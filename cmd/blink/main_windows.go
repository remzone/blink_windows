// SPDX-License-Identifier: GPL-3.0-only
//go:build windows

package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/remzone/blink_windows/internal/client"
	"github.com/remzone/blink_windows/internal/platform"
	"github.com/remzone/blink_windows/native/mobile"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wwindows "github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

//go:embed ui/*
var assets embed.FS

const version = "0.2.1"

type ProfileView struct {
	Index    int    `json:"index"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Selected bool   `json:"selected"`
	Routing  bool   `json:"routing"`
}
type View struct {
	ExpiresAt       int64                 `json:"expiresAt"`
	Version         string                `json:"version"`
	Arch            string                `json:"arch"`
	Profiles        []ProfileView         `json:"profiles"`
	Selected        int                   `json:"selected"`
	Busy            bool                  `json:"busy"`
	Phase           string                `json:"phase"`
	Message         string                `json:"message"`
	Stats           json.RawMessage       `json:"stats"`
	Diagnostic      json.RawMessage       `json:"diagnostic"`
	HasSubscription bool                  `json:"hasSubscription"`
	Revision        int64                 `json:"revision"`
	VK              client.VKOptions      `json:"vk"`
	Routing         client.RoutingOptions `json:"routing"`
	Captcha         *ChallengeView        `json:"captcha,omitempty"`
}
type ChallengeView struct {
	ID      string `json:"id"`
	URL     string `json:"url,omitempty"`
	Image   string `json:"image,omitempty"`
	Expires int64  `json:"expires_at"`
}
type Application struct {
	captchaSubmitted                            string
	captchaDeadline                             time.Time
	mu                                          sync.Mutex
	ctx                                         context.Context
	state                                       client.State
	tunnel                                      *platform.Tunnel
	busy, closing, cancelPending, disconnecting bool
	phase, message                              string
	stats, diagnostic                           json.RawMessage
	challenge                                   *ChallengeView
	cancel                                      context.CancelFunc
	started                                     time.Time
}

func (a *Application) startup(ctx context.Context) { a.mu.Lock(); a.ctx = ctx; a.mu.Unlock() }
func (a *Application) Snapshot() View {
	a.mu.Lock()
	defer a.mu.Unlock()
	vk := client.DefaultVKOptions()
	if a.state.VKOptions != nil {
		vk = *a.state.VKOptions
	}
	route := client.DefaultRouting()
	if a.state.Routing != nil {
		route = *a.state.Routing
	}
	v := View{ExpiresAt: a.state.ExpiresAt, Version: version, Arch: goruntime.GOARCH, Profiles: []ProfileView{}, Selected: a.state.Selected, Busy: a.busy, Phase: a.phase, Message: a.message, Stats: a.stats, Diagnostic: a.diagnostic, HasSubscription: a.state.SubscriptionURL != "", Revision: a.state.Revision, VK: vk, Routing: route, Captcha: a.challenge}
	for i, p := range a.state.Profiles {
		v.Profiles = append(v.Profiles, ProfileView{Index: i, Name: p.Name, Kind: p.Kind, Selected: i == a.state.Selected, Routing: len(p.Routing) > 0 && string(p.Routing) != "null"})
	}
	return v
}
func (a *Application) emit() {
	a.mu.Lock()
	ctx := a.ctx
	a.mu.Unlock()
	if ctx != nil {
		runtime.EventsEmit(ctx, "blink:state", a.Snapshot())
	}
}
func (a *Application) saveLocked() error {
	b, err := json.Marshal(a.state)
	if err != nil {
		return err
	}
	return platform.SaveState(b)
}
func (a *Application) finish(message string) {
	a.mu.Lock()
	a.busy = false
	a.message = message
	quit := a.closing && a.tunnel == nil
	ctx := a.ctx
	a.cancel = nil
	a.mu.Unlock()
	a.emit()
	if quit {
		runtime.Quit(ctx)
	}
}
func (a *Application) begin(message string, offline bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.busy || a.disconnecting || offline && a.tunnel != nil {
		return errors.New("Сначала завершите текущую операцию или отключите соединение")
	}
	a.busy = true
	a.cancelPending = false
	a.message = message
	return nil
}
func (a *Application) Import(source string) error {
	if err := a.begin("Проверяем профили…", true); err != nil {
		return err
	}
	a.emit()
	a.mu.Lock()
	old := a.state
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancel = cancel
	a.mu.Unlock()
	defer cancel()
	raw, link := strings.TrimSpace(source), ""
	var err error
	if strings.HasPrefix(raw, "https://") {
		link = raw
		raw, err = client.Fetch(ctx, link)
	}
	minimum := int64(0)
	if link != "" && link == old.SubscriptionURL {
		minimum = old.Revision
	}
	var result client.ImportResult
	if err == nil {
		result, err = client.Parse(raw, time.Now(), minimum)
	}
	if err != nil {
		a.finish(err.Error())
		return err
	}
	a.mu.Lock()
	if a.cancelPending || a.closing {
		a.mu.Unlock()
		a.finish("Импорт отменён")
		return nil
	}
	a.state = client.State{ExpiresAt: result.ExpiresAt, Profiles: result.Profiles, Selected: 0, SubscriptionURL: link, Revision: result.Revision, VKOptions: old.VKOptions, Routing: old.Routing}
	err = a.saveLocked()
	if err != nil {
		a.state = old
	}
	a.mu.Unlock()
	if err != nil {
		a.finish("Не удалось сохранить профили")
		return errors.New("Не удалось сохранить профили")
	}
	a.finish(fmt.Sprintf("Добавлено профилей: %d. Отклонено: %d. %s", len(result.Profiles), result.Rejected, result.Notice))
	return nil
}
func (a *Application) ImportFile() error {
	a.mu.Lock()
	ctx := a.ctx
	blocked := a.busy || a.tunnel != nil
	a.mu.Unlock()
	if blocked {
		return errors.New("Сначала отключите соединение")
	}
	name, err := runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{Title: "Добавить профили Blink", Filters: []runtime.FileFilter{{DisplayName: "Конфигурации VPN", Pattern: "*.conf;*.json;*.txt;*.yaml;*.yml"}, {DisplayName: "Все файлы", Pattern: "*"}}})
	if err != nil || name == "" {
		return err
	}
	f, err := os.Open(name)
	if err != nil {
		return errors.New("Не удалось открыть файл")
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, client.MaxDocument+1))
	if err != nil || len(b) > client.MaxDocument {
		return errors.New("Файл не прочитан или превышает 1 МиБ")
	}
	return a.Import(string(b))
}
func (a *Application) Refresh() error {
	a.mu.Lock()
	link := a.state.SubscriptionURL
	a.mu.Unlock()
	if link == "" {
		return errors.New("Сначала добавьте HTTPS-подписку")
	}
	return a.Import(link)
}
func (a *Application) Select(index int) error {
	a.mu.Lock()
	if a.busy || a.tunnel != nil || index < 0 || index >= len(a.state.Profiles) {
		a.mu.Unlock()
		return errors.New("Отключитесь, чтобы сменить профиль")
	}
	old := a.state.Selected
	a.state.Selected = index
	err := a.saveLocked()
	if err != nil {
		a.state.Selected = old
	}
	a.mu.Unlock()
	a.emit()
	return err
}
func (a *Application) Delete(index int) error {
	a.mu.Lock()
	if a.busy || a.tunnel != nil || index < 0 || index >= len(a.state.Profiles) {
		a.mu.Unlock()
		return errors.New("Отключитесь, чтобы изменить профили")
	}
	old := a.state
	profiles := append([]client.Profile(nil), old.Profiles[:index]...)
	profiles = append(profiles, old.Profiles[index+1:]...)
	a.state.Profiles = profiles
	a.state.Selected = 0
	err := a.saveLocked()
	if err != nil {
		a.state = old
	}
	a.mu.Unlock()
	a.emit()
	return err
}
func (a *Application) SaveSettings(vk client.VKOptions, routing client.RoutingOptions) error {
	if err := vk.Validate(); err != nil {
		return err
	}
	if _, err := routing.Policy(nil); err != nil {
		return err
	}
	a.mu.Lock()
	if a.busy || a.tunnel != nil {
		a.mu.Unlock()
		return errors.New("Отключитесь перед изменением настроек")
	}
	old := a.state
	a.state.VKOptions = &vk
	a.state.Routing = &routing
	err := a.saveLocked()
	if err != nil {
		a.state = old
	}
	a.mu.Unlock()
	a.emit()
	return err
}
func (a *Application) Connect() error {
	if err := a.begin("Готовим защищённое соединение…", true); err != nil {
		return err
	}
	a.mu.Lock()
	i := a.state.Selected
	if i < 0 || i >= len(a.state.Profiles) {
		a.mu.Unlock()
		a.finish("Добавьте первый профиль")
		return errors.New("Добавьте и выберите профиль")
	}
	p := a.state.Profiles[i]
	vk := client.DefaultVKOptions()
	if a.state.VKOptions != nil {
		vk = *a.state.VKOptions
	}
	routing := client.DefaultRouting()
	if a.state.Routing != nil {
		routing = *a.state.Routing
	}
	a.phase = "connecting"
	a.diagnostic = nil
	a.mu.Unlock()
	a.emit()
	p, err := client.ConnectionProfile(p, vk, routing)
	var t *platform.Tunnel
	if err == nil {
		t, err = platform.Start(p)
	}
	a.mu.Lock()
	cancelled := a.cancelPending || a.closing
	if err != nil || cancelled {
		a.mu.Unlock()
		if t != nil {
			_ = t.Close()
		}
		a.mu.Lock()
		a.phase = "disconnected"
		a.mu.Unlock()
		if cancelled {
			a.finish("Подключение отменено")
			return nil
		}
		a.finish(err.Error())
		return err
	}
	a.tunnel = t
	a.captchaSubmitted = ""
	a.captchaDeadline = time.Time{}
	a.started = time.Now()
	a.mu.Unlock()
	a.finish("Ожидаем готовности транспорта…")
	go a.monitor(t, p)
	return nil
}
func (a *Application) monitor(t *platform.Tunnel, p client.Profile) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	deadline := time.Now().Add(5 * time.Minute)
	configured := false
	for {
		select {
		case <-t.Done:
			return
		case <-t.Failed:
			_ = a.Disconnect()
			return
		case <-ticker.C:
		}
		status := t.Engine.Status()
		a.mu.Lock()
		answerExpired := !a.captchaDeadline.IsZero() && time.Now().After(a.captchaDeadline) && !configured
		a.mu.Unlock()
		if answerExpired {
			_ = a.Disconnect()
			a.mu.Lock()
			a.message = "Транспорт не подтвердил готовность за 60 секунд. Подключитесь заново."
			a.mu.Unlock()
			a.emit()
			return
		}
		if status == "stopped" || !configured && time.Now().After(deadline) {
			_ = a.Disconnect()
			return
		}
		var challenge ChallengeView
		_ = json.Unmarshal([]byte(t.Engine.CaptchaJSON()), &challenge)
		if !configured && (status == "ready" || status == "running") {
			if err := t.Configure(p); err != nil {
				_ = a.Disconnect()
				a.mu.Lock()
				a.message = err.Error()
				a.mu.Unlock()
				a.emit()
				return
			}
			configured = true
		}
		a.mu.Lock()
		if a.tunnel != t || a.disconnecting {
			a.mu.Unlock()
			return
		}
		a.stats = json.RawMessage(t.Engine.StatsJSON())
		a.challenge = nil
		if challenge.ID != "" && challenge.ID != a.captchaSubmitted {
			a.challenge = &challenge
			a.phase = "captcha"
			a.message = "Один небольшой шаг. Пройдите проверку — Blink продолжит подключение."
		} else if configured {
			a.phase = "connected"
			a.message = "Соединение активно. Ваш профиль готов к работе."
		} else {
			a.phase = "connecting"
		}
		a.mu.Unlock()
		a.emit()
	}
}
func (a *Application) Disconnect() error {
	a.mu.Lock()
	if a.disconnecting {
		a.mu.Unlock()
		return nil
	}
	t := a.tunnel
	if t == nil {
		a.cancelPending = true
		if a.cancel != nil {
			a.cancel()
		}
		a.mu.Unlock()
		return nil
	}
	a.disconnecting = true
	a.busy = true
	a.phase = "disconnecting"
	a.challenge = nil
	a.mu.Unlock()
	a.emit()
	err := t.Close()
	a.mu.Lock()
	a.tunnel = nil
	a.stats = nil
	a.challenge = nil
	a.phase = "disconnected"
	a.disconnecting = false
	if err != nil {
		a.closing = false
	}
	a.mu.Unlock()
	if err != nil {
		a.finish("Проверьте восстановление сети в настройках")
		return errors.New("Не все сетевые правила очищены. Используйте восстановление сети")
	}
	a.finish("Отключено. Вы готовы к следующему подключению.")
	return nil
}
func (a *Application) Probe(kind string) error {
	a.mu.Lock()
	t := a.tunnel
	connected := a.phase == "connected"
	a.mu.Unlock()
	if t == nil || !connected {
		return errors.New("Сначала подключите туннель")
	}
	if err := a.begin("Проверяем HTTPS через соединение…", false); err != nil {
		return err
	}
	a.emit()
	result, err := mobile.ProbeInternet(kind, false, nil)
	a.mu.Lock()
	same := a.tunnel == t && !a.disconnecting
	if same && err == nil {
		a.diagnostic = json.RawMessage(result)
	}
	a.mu.Unlock()
	if !same {
		return nil
	}
	if err != nil {
		a.finish(err.Error())
		return err
	}
	a.finish("Проверка завершена")
	return nil
}
func (a *Application) Recover() error {
	if err := a.begin("Восстанавливаем настройки сети…", true); err != nil {
		return err
	}
	a.emit()
	err := platform.Recover()
	if err != nil {
		a.finish(err.Error())
		return err
	}
	a.finish("Сеть восстановлена")
	return nil
}
func (a *Application) SubmitCaptcha(id, kind, answer string) error {
	a.mu.Lock()
	t := a.tunnel
	c := a.challenge
	a.mu.Unlock()
	if t == nil || c == nil || c.ID != id || c.Expires <= time.Now().Unix() || !t.Engine.SubmitCaptcha(id, kind, answer) {
		return errors.New("Проверка истекла или ответ не принят")
	}
	a.mu.Lock()
	a.challenge = nil
	a.captchaSubmitted = id
	a.captchaDeadline = time.Now().Add(60 * time.Second)
	a.phase = "connecting"
	a.message = "Ответ отправлен. Ожидаем подтверждения…"
	a.mu.Unlock()
	a.emit()
	return nil
}
func (a *Application) beforeClose(ctx context.Context) bool {
	a.mu.Lock()
	if a.tunnel == nil && !a.busy {
		a.mu.Unlock()
		return false
	}
	a.closing = true
	a.mu.Unlock()
	go a.Disconnect()
	return true
}
func dialog(s string) {
	title, _ := windows.UTF16PtrFromString("Blink")
	body, _ := windows.UTF16PtrFromString(s)
	windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(body)), uintptr(unsafe.Pointer(title)), 0x10)
}
func setWindowIcon() {
	user := windows.NewLazySystemDLL("user32.dll")
	kernel := windows.NewLazySystemDLL("kernel32.dll")
	module, _, _ := kernel.NewProc("GetModuleHandleW").Call(0)
	hwnd, _, _ := user.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("BlinkWindows"))), 0)
	if hwnd == 0 {
		return
	}
	for _, v := range []struct{ size, kind uintptr }{{32, 1}, {16, 0}} {
		icon, _, _ := user.NewProc("LoadImageW").Call(module, 2, 1, v.size, v.size, 0x8000)
		if icon != 0 {
			user.NewProc("SendMessageW").Call(hwnd, 0x80, v.kind, icon)
		}
	}
}
func main() {
	mutex, err := windows.CreateMutex(nil, false, windows.StringToUTF16Ptr("Global\\Blink.Windows.Client"))
	if err == windows.ERROR_ALREADY_EXISTS {
		if mutex != 0 {
			windows.CloseHandle(mutex)
		}
		return
	}
	if err != nil {
		dialog("Не удалось проверить уже запущенный Blink")
		return
	}
	defer windows.CloseHandle(mutex)
	if len(os.Args) > 1 && os.Args[1] == "--recover" {
		if err := platform.Recover(); err != nil {
			dialog(err.Error())
		}
		return
	}
	dir, err := platform.DataDir()
	if err != nil {
		dialog(err.Error())
		return
	}
	if err = mobile.PrepareAssets(filepath.Join(dir, "assets")); err != nil {
		dialog("Не удалось подготовить ядро Blink")
		return
	}
	app := &Application{phase: "disconnected", message: "Всё начинается с одного подключения."}
	if b, e := platform.LoadState(); e == nil {
		if json.Unmarshal(b, &app.state) != nil {
			app.message = "Сохранённые профили повреждены"
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		app.message = "Не удалось расшифровать профили текущего пользователя"
	}
	// Browser state is transient and never shared with the system browser.
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "webview-session-") {
			_ = os.RemoveAll(filepath.Join(dir, entry.Name()))
		}
	}
	webviewDir, e := os.MkdirTemp(dir, "webview-session-")
	if e != nil {
		dialog("Не удалось создать временное хранилище интерфейса")
		return
	}
	defer os.RemoveAll(webviewDir)
	files, _ := fs.Sub(assets, "ui")
	err = wails.Run(&options.App{Title: "Blink", Width: 1160, Height: 820, MinWidth: 760, MinHeight: 660, BackgroundColour: options.NewRGB(8, 13, 24), AssetServer: &assetserver.Options{Assets: files}, Bind: []interface{}{app}, OnStartup: app.startup, OnDomReady: func(context.Context) { setWindowIcon(); app.emit() }, OnBeforeClose: app.beforeClose, OnShutdown: func(context.Context) {
		app.mu.Lock()
		t := app.tunnel
		app.mu.Unlock()
		if t != nil {
			_ = t.Close()
		}
	}, Windows: &wwindows.Options{Theme: wwindows.Dark, WebviewUserDataPath: webviewDir, DisablePinchZoom: true, WindowClassName: "BlinkWindows", DLLSearchPaths: wwindows.DLLSearchApplicationDir | wwindows.DLLSearchSystem32}, EnableDefaultContextMenu: false})
	if err != nil {
		dialog("Не удалось открыть Blink. Проверьте установку Microsoft Edge WebView2 Runtime.")
	}
}
