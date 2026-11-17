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
	"github.com/remzone/blink_windows/internal/diagnostics"
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

const version = "0.3.0-preview.2"

type SourceView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Count       int    `json:"count"`
	Refreshable bool   `json:"refreshable"`
	ExpiresAt   int64  `json:"expiresAt,omitempty"`
}
type ProfileView struct {
	SourceID  string `json:"sourceId"`
	ExpiresAt int64  `json:"expiresAt,omitempty"`
	Index     int    `json:"index"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Selected  bool   `json:"selected"`
	Routing   bool   `json:"routing"`
}
type View struct {
	LogPath         string                 `json:"logPath"`
	LogError        string                 `json:"logError"`
	Sources         []SourceView           `json:"sources"`
	Selections      map[string]int         `json:"selections"`
	Connection      client.ConnectionTrace `json:"connection"`
	ExpiresAt       int64                  `json:"expiresAt"`
	Version         string                 `json:"version"`
	Arch            string                 `json:"arch"`
	Profiles        []ProfileView          `json:"profiles"`
	Selected        int                    `json:"selected"`
	Busy            bool                   `json:"busy"`
	Phase           string                 `json:"phase"`
	Message         string                 `json:"message"`
	Stats           json.RawMessage        `json:"stats"`
	Diagnostic      json.RawMessage        `json:"diagnostic"`
	HasSubscription bool                   `json:"hasSubscription"`
	Revision        int64                  `json:"revision"`
	TURN            client.VKOptions       `json:"turn"`
	Routing         client.RoutingOptions  `json:"routing"`
	Captcha         *ChallengeView         `json:"captcha,omitempty"`
}
type ChallengeView struct {
	ID      string `json:"id"`
	URL     string `json:"url,omitempty"`
	Image   string `json:"image,omitempty"`
	Expires int64  `json:"expires_at"`
}
type Application struct {
	connectionCancel                            context.CancelFunc
	connection                                  client.ConnectionTrace
	attempt                                     uint64
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
	trace := a.connection
	trace.Events = append([]client.ConnectionEvent(nil), trace.Events...)
	if trace.StartedAt > 0 && trace.Stage != "failed" && trace.Stage != "disconnected" {
		trace.ElapsedSeconds = (time.Now().UnixMilli() - trace.StartedAt) / 1000
	}
	logPath, logErr := diagnostics.Status()
	logError := ""
	if logErr != nil {
		logError = "Не удалось записать журнал. Проверьте доступ к папке Blink и свободное место."
	}
	v := View{LogPath: logPath, LogError: logError, Sources: []SourceView{}, Connection: trace, Selections: map[string]int{"vpn": a.state.PreferredIndex("vpn"), "turn": a.state.PreferredIndex("turn")}, ExpiresAt: a.state.ExpiresAt, Version: version, Arch: goruntime.GOARCH, Profiles: []ProfileView{}, Selected: a.state.Selected, Busy: a.busy, Phase: a.phase, Message: a.message, Stats: a.stats, Diagnostic: a.diagnostic, HasSubscription: a.state.SubscriptionURL != "", Revision: a.state.Revision, TURN: vk, Routing: route, Captcha: a.challenge}
	for i, p := range a.state.Profiles {
		kind := p.Kind
		if kind == "vk" {
			kind = "turn"
		}
		source, _ := a.state.Source(p.SourceID)
		v.Profiles = append(v.Profiles, ProfileView{SourceID: p.SourceID, ExpiresAt: source.ExpiresAt, Index: i, Name: p.Name, Kind: kind, Selected: i == a.state.Selected, Routing: len(p.Routing) > 0 && string(p.Routing) != "null"})
	}
	for _, source := range a.state.Sources {
		count := 0
		for _, p := range a.state.Profiles {
			if p.SourceID == source.ID {
				count++
			}
		}
		v.Sources = append(v.Sources, SourceView{ID: source.ID, Name: source.Name, Kind: source.Kind, Count: count, Refreshable: source.URL != "", ExpiresAt: source.ExpiresAt})
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
	old := a.state.Clone()
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
	minimum := old.MinimumRevision(link)
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
	next, mergeErr := old.MergeImport(result, link, time.Now())
	if mergeErr != nil {
		a.mu.Unlock()
		a.finish(mergeErr.Error())
		return mergeErr
	}
	a.state = next
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
func (a *Application) RefreshSource(id string) error {
	a.mu.Lock()
	source, ok := a.state.Source(id)
	a.mu.Unlock()
	if !ok || source.URL == "" {
		return errors.New("У этого источника нет ссылки обновления")
	}
	return a.Import(source.URL)
}
func (a *Application) DeleteSource(id string) error {
	a.mu.Lock()
	if a.busy || a.tunnel != nil {
		a.mu.Unlock()
		return errors.New("Отключитесь перед удалением источника")
	}
	old := a.state.Clone()
	next := a.state.Clone()
	next.Sources = nil
	next.Profiles = nil
	selectedID := ""
	if old.Selected >= 0 && old.Selected < len(old.Profiles) {
		selectedID = old.Profiles[old.Selected].ID
	}
	for _, source := range old.Sources {
		if source.ID != id {
			next.Sources = append(next.Sources, source)
		}
	}
	for _, p := range old.Profiles {
		if p.SourceID != id {
			next.Profiles = append(next.Profiles, p)
		}
	}
	next.Selected = 0
	for i, p := range next.Profiles {
		if p.ID == selectedID {
			next.Selected = i
			break
		}
	}
	if len(next.Profiles) > 0 {
		next.SelectProfile(next.Selected)
	} else {
		next.SubscriptionURL = ""
		next.ExpiresAt = 0
		next.Revision = 0
	}
	a.state = next
	err := a.saveLocked()
	if err != nil {
		a.state = old
	}
	a.mu.Unlock()
	a.emit()
	return err
}
func (a *Application) Select(index int) error {
	a.mu.Lock()
	if a.busy || a.tunnel != nil || index < 0 || index >= len(a.state.Profiles) {
		a.mu.Unlock()
		return errors.New("Отключитесь, чтобы сменить профиль")
	}
	old := a.state.Clone()
	a.state.SelectProfile(index)
	err := a.saveLocked()
	if err != nil {
		a.state = old
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
	old := a.state.Clone()
	a.state = a.state.RemoveProfile(index)
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
	old := a.state.Clone()
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
func (a *Application) reportStage(attempt uint64, stage string) {
	a.mu.Lock()
	if a.attempt != attempt || a.cancelPending || a.closing || a.connectionCancel == nil {
		a.mu.Unlock()
		return
	}
	a.connection.SetStage(stage, time.Now())
	a.message = a.connection.Detail
	a.mu.Unlock()
	a.emit()
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
	a.attempt++
	attempt := a.attempt
	ctx, cancel := context.WithCancel(a.ctx)
	a.connectionCancel = cancel
	a.connection = client.NewConnectionTrace(time.Now())
	a.connection.SetStage("adapter", time.Now())
	a.phase = "connecting"
	a.diagnostic = nil
	a.started = time.Now()
	a.mu.Unlock()
	a.emit()
	_ = mobile.DrainDiagnosticEventsJSON()
	p, err := client.ConnectionProfile(p, vk, routing)
	var t *platform.Tunnel
	startCtx, startCancel := context.WithTimeout(ctx, 45*time.Second)
	if err == nil {
		t, err = platform.Start(startCtx, p, func(stage string) { a.reportStage(attempt, stage) })
	}
	startCancel()
	a.mu.Lock()
	cancelled := a.cancelPending || a.closing || ctx.Err() != nil
	if err != nil || cancelled {
		a.mu.Unlock()
		cancel()
		if t != nil {
			_ = t.Close()
		}
		a.mu.Lock()
		a.phase = "disconnected"
		a.connectionCancel = nil
		message := "Подключение отменено"
		if cancelled {
			a.connection.SetStage("disconnected", time.Now())
		} else {
			message = err.Error()
			code := "START_FAILED"
			if errors.Is(err, context.DeadlineExceeded) {
				message = a.connection.TimeoutMessage()
				code = "START_TIMEOUT"
			}
			a.connection.Fail(code, message, time.Now())
		}
		a.mu.Unlock()
		a.finish(message)
		if cancelled {
			return nil
		}
		return errors.New(message)
	}
	a.tunnel = t
	a.captchaSubmitted = ""
	a.captchaDeadline = time.Time{}
	if p.Kind == "vk" {
		a.connection.SetStage("transport", time.Now())
	} else {
		a.connection.SetStage("network", time.Now())
	}
	a.mu.Unlock()
	a.finish("Ожидаем готовности транспорта…")
	go a.monitor(ctx, t, p, attempt)
	return nil
}
func (a *Application) failConnection(t *platform.Tunnel, code, message string) {
	_ = a.disconnect(t, code, message)
}
func (a *Application) monitor(ctx context.Context, t *platform.Tunnel, p client.Profile, attempt uint64) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	deadline := time.Now().Add(90 * time.Second)
	configured := false
	recovering := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.Done:
			return
		case <-t.Failed:
			a.failConnection(t, "PACKET_IO_FAILED", "Сетевой адаптер остановил обработку пакетов. Сохраните отчёт подключения.")
			return
		case <-ticker.C:
		}
		status := t.Engine.Status()
		ready := status == "ready" || status == "running"
		if configured && !ready && !recovering {
			deadline = time.Now().Add(90 * time.Second)
			recovering = true
			a.reportStage(attempt, "transport")
		}
		if ready {
			recovering = false
		}
		var challenge ChallengeView
		_ = json.Unmarshal([]byte(t.Engine.CaptchaJSON()), &challenge)
		events := mobile.DrainDiagnosticEventsJSON()
		a.mu.Lock()
		if a.tunnel != t || a.disconnecting || a.attempt != attempt {
			a.mu.Unlock()
			return
		}
		a.connection.Consume(events, time.Now())
		if ready {
			a.captchaDeadline = time.Time{}
		}
		pending := challenge.ID != "" && challenge.ID != a.captchaSubmitted
		answerExpired := !a.captchaDeadline.IsZero() && time.Now().After(a.captchaDeadline) && (!configured || recovering)
		problem := a.connection.TimeoutMessage()
		problemCode := a.connection.SafeProblemCode()
		a.mu.Unlock()
		if answerExpired {
			a.failConnection(t, "CHECK_RESPONSE_TIMEOUT", "После ручной проверки транспорт не подтвердил готовность за 60 секунд.")
			return
		}
		if pending {
			deadline = time.Now().Add(90 * time.Second)
			if challenge.Expires > 0 && time.Now().Unix() >= challenge.Expires {
				a.failConnection(t, "CHECK_EXPIRED", "Истекло время ручной проверки. Подключитесь заново.")
				return
			}
		} else if (!configured || recovering) && time.Now().After(deadline) {
			a.failConnection(t, problemCode, problem)
			return
		}
		if status == "stopped" {
			a.failConnection(t, "CORE_STOPPED", "VPN-ядро остановилось. Сохраните отчёт подключения.")
			return
		}
		if !configured && (status == "ready" || status == "running") {
			a.reportStage(attempt, "network")
			if err := t.Configure(ctx, p); err != nil {
				if ctx.Err() == nil {
					a.failConnection(t, "NETWORK_CONFIG_FAILED", err.Error())
				}
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
		if pending {
			a.challenge = &challenge
			a.phase = "captcha"
			a.connection.SetStage("captcha", time.Now())
		} else if configured && ready {
			a.phase = "connected"
			a.connection.SetStage("connected", time.Now())
		} else {
			a.phase = "connecting"
			if a.connection.Stage == "captcha" {
				a.connection.SetStage("transport", time.Now())
			}
		}
		a.message = a.connection.Detail
		a.mu.Unlock()
		a.emit()
	}
}
func (a *Application) CopyConnectionReport() error {
	a.mu.Lock()
	report := a.connection.Report(version, goruntime.GOARCH)
	ctx := a.ctx
	a.mu.Unlock()
	return runtime.ClipboardSetText(ctx, report)
}

// Open only our local log directory, never an imported path or external URL.
func (a *Application) OpenLogFolder() error {
	path, err := diagnostics.Status()
	if path == "" {
		return errors.New("Журнал не создан. Перезапустите Blink")
	}
	folder, _ := windows.UTF16PtrFromString(filepath.Dir(path))
	if e := windows.ShellExecute(0, nil, folder, nil, nil, windows.SW_SHOWNORMAL); e != nil {
		return errors.New("Не удалось открыть папку журнала. Откройте %APPDATA%\\Blink\\logs вручную")
	}
	if err != nil {
		return errors.New("В журнале произошла ошибка записи. Проверьте свободное место в папке Blink")
	}
	return nil
}

func (a *Application) Disconnect() error { return a.disconnect(nil, "", "") }
func (a *Application) disconnect(expected *platform.Tunnel, code, message string) error {
	a.mu.Lock()
	if expected != nil && a.tunnel != expected {
		a.mu.Unlock()
		return nil
	}
	if a.disconnecting {
		a.mu.Unlock()
		return nil
	}
	if a.connectionCancel != nil {
		a.connectionCancel()
		a.connectionCancel = nil
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
	a.connection.SetStage("stopping", time.Now())
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
	a.connection.ElapsedSeconds = (time.Now().UnixMilli() - a.connection.StartedAt) / 1000
	if code != "" {
		if err != nil {
			message += " Не все правила сняты; используйте восстановление сети."
		}
		a.connection.Fail(code, message, time.Now())
	} else {
		a.connection.SetStage("disconnected", time.Now())
		message = "Отключено. Вы готовы к следующему подключению."
	}
	if err != nil {
		a.closing = false
		if code == "" {
			message = "Не все сетевые правила очищены. Используйте восстановление сети"
		}
	}
	a.mu.Unlock()
	a.finish(message)
	if err != nil {
		return errors.New("Не все сетевые правила очищены. Используйте восстановление сети")
	}
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
	diagnostics.Event("recovery.manual", nil)
	err := platform.Recover()
	diagnostics.Event("recovery.result", map[string]any{"ok": err == nil})
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
	dir, err := platform.DataDir()
	if err != nil {
		dialog(err.Error())
		return
	}
	journal, err := diagnostics.Open(filepath.Join(dir, "logs", "blink.log"))
	if err != nil {
		dialog("Не удалось создать журнал %APPDATA%\\Blink\\logs\\blink.log. Проверьте доступ к папке и свободное место.")
		return
	}
	diagnostics.Set(journal)
	defer func() { diagnostics.Event("app.stop", nil); diagnostics.Set(nil); _ = journal.Close() }()
	osv := windows.RtlGetVersion()
	diagnostics.Event("app.start", map[string]any{"version": version, "arch": goruntime.GOARCH, "pid": os.Getpid(), "admin": platform.Admin(), "windowsMajor": osv.MajorVersion, "windowsMinor": osv.MinorVersion, "windowsBuild": osv.BuildNumber})
	if len(os.Args) > 1 && os.Args[1] == "--recover" {
		diagnostics.Event("recovery.command", nil)
		err := platform.Recover()
		diagnostics.Event("recovery.result", map[string]any{"ok": err == nil})
		if err != nil {
			dialog(err.Error())
		}
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
	app.state.Normalize()
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
