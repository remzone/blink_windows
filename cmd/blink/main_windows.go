// SPDX-License-Identifier: GPL-3.0-only
//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/remzone/blink_windows/internal/client"
	"github.com/remzone/blink_windows/internal/platform"
	"github.com/remzone/blink_windows/native/mobile"
	"golang.org/x/sys/windows"
)

var user = windows.NewLazySystemDLL("user32.dll")
var kernel = windows.NewLazySystemDLL("kernel32.dll")
var gdi = windows.NewLazySystemDLL("gdi32.dll")
var createWindow = user.NewProc("CreateWindowExW")
var send = user.NewProc("SendMessageW")
var setText = user.NewProc("SetWindowTextW")
var getText = user.NewProc("GetWindowTextW")
var getTextLen = user.NewProc("GetWindowTextLengthW")
var post = user.NewProc("PostMessageW")

const wmCommand = 0x111
const wmClose = 0x10
const wmUpdate = 0x8001
const wmDestroy = 2
const idImport = 101
const idConnect = 102
const idRefresh = 103
const idDelete = 104
const idProbe = 105
const idRecover = 106
const idCaptcha = 107
const idOpenCaptcha = 108
const idFile = 109

type point struct{ X, Y int32 }
type message struct {
	Hwnd    uintptr
	Msg     uint32
	Wparam  uintptr
	Lparam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}
type windowClass struct {
	Size, Style                        uint32
	Proc                               uintptr
	ClsExtra, WndExtra                 int32
	Instance, Icon, Cursor, Background uintptr
	Menu, Name                         *uint16
	SmallIcon                          uintptr
}

type app struct {
	exiting                                             bool
	disconnecting                                       bool
	hwnd, input, list, status, detail, captcha, connect uintptr
	state                                               client.State
	tunnel                                              *platform.Tunnel
	busy                                                bool
	connected                                           bool
	updates                                             chan func()
	pendingChallenge                                    string
	font                                                uintptr
}

var ui app

func ptr(s string) *uint16     { p, _ := windows.UTF16PtrFromString(s); return p }
func text(h uintptr, s string) { setText.Call(h, uintptr(unsafe.Pointer(ptr(s)))) }
func value(h uintptr) string {
	n, _, _ := getTextLen.Call(h)
	b := make([]uint16, n+1)
	getText.Call(h, uintptr(unsafe.Pointer(&b[0])), n+1)
	return windows.UTF16ToString(b)
}
func (a *app) later(f func()) { a.updates <- f; post.Call(a.hwnd, wmUpdate, 0, 0) }
func (a *app) control(class, label string, style uintptr, x, y, w, h int32, id int) uintptr {
	wnd, _, _ := createWindow.Call(0, uintptr(unsafe.Pointer(ptr(class))), uintptr(unsafe.Pointer(ptr(label))), 0x40000000|0x10000000|style, uintptr(x), uintptr(y), uintptr(w), uintptr(h), a.hwnd, uintptr(id), 0, 0)
	send.Call(wnd, 0x30, a.font, 1)
	return wnd
}
func (a *app) save() error {
	b, err := json.Marshal(a.state)
	if err != nil {
		return err
	}
	return platform.SaveState(b)
}
func (a *app) render() {
	send.Call(a.list, 0x184, 0, 0)
	for _, p := range a.state.Profiles {
		send.Call(a.list, 0x180, 0, uintptr(unsafe.Pointer(ptr(p.Name+"  ·  "+p.Kind))))
	}
	send.Call(a.list, 0x186, uintptr(a.state.Selected), 0)
}
func (a *app) selected() int {
	n, _, _ := send.Call(a.list, 0x188, 0, 0)
	i := int(int32(n))
	if i < 0 || i >= len(a.state.Profiles) {
		return -1
	}
	return i
}
func (a *app) notice(s string) { text(a.status, s) }
func (a *app) begin() bool {
	if a.busy {
		a.notice("Дождитесь завершения текущей операции")
		return false
	}
	a.busy = true
	return true
}
func (a *app) importSource(raw, link string) {
	if a.tunnel != nil || !a.begin() {
		return
	}
	minimum := int64(0)
	if link != "" && link == a.state.SubscriptionURL {
		minimum = a.state.Revision
	}
	a.notice("Проверка и импорт профилей…")
	go func() {
		var err error
		if link != "" {
			raw, err = client.Fetch(context.Background(), link)
		}
		var result client.ImportResult
		if err == nil {
			result, err = client.Parse(raw, time.Now(), minimum)
		}
		a.later(func() {
			a.busy = false
			if err != nil {
				a.notice(err.Error())
				return
			}
			previous := a.state
			a.state = client.State{Profiles: result.Profiles, Selected: 0, SubscriptionURL: link, Revision: result.Revision}
			if err = a.save(); err != nil {
				a.state = previous
				a.notice(err.Error())
				return
			}
			a.render()
			text(a.input, "")
			a.notice(fmt.Sprintf("Импортировано: %d. Отклонено: %d. %s", len(result.Profiles), result.Rejected, result.Notice))
		})
	}()
}
func (a *app) toggle() {
	if a.busy {
		return
	}
	if a.tunnel != nil {
		a.disconnect("Отключено")
		return
	}
	i := a.selected()
	if i < 0 {
		a.notice("Сначала импортируйте и выберите профиль")
		return
	}
	if !a.begin() {
		return
	}
	p := a.state.Profiles[i]
	a.state.Selected = i
	_ = a.save()
	a.notice("Подготовка Wintun и транспорта…")
	go func() {
		t, err := platform.Start(p)
		a.later(func() {
			a.busy = false
			if err != nil {
				a.notice(err.Error())
				return
			}
			a.tunnel = t
			text(a.connect, "Отключить")
			a.notice("Ожидание готовности транспорта; маршруты ещё не установлены")
			go a.monitor(t, p)
		})
	}()
}
func (a *app) monitor(t *platform.Tunnel, p client.Profile) {
	deadline := time.Now().Add(5 * time.Minute)
	configured := false
	lastChallenge := ""
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-t.Failed:
			a.later(func() {
				if a.tunnel == t {
					a.disconnect("Пакетный интерфейс остановлен")
				}
			})
			return
		case <-t.Done:
			return
		case <-ticker.C:
		}
		status := t.Engine.Status()
		if status == "stopped" || time.Now().After(deadline) && !configured {
			a.later(func() {
				if a.tunnel == t {
					a.disconnect("Транспорт остановлен или превышено время ожидания")
				}
			})
			return
		}
		var challenge struct {
			ID, URL, Image string
			Expires        int64 `json:"expires_at"`
		}
		_ = json.Unmarshal([]byte(t.Engine.CaptchaJSON()), &challenge)
		if challenge.ID != "" && challenge.ID != lastChallenge {
			lastChallenge = challenge.ID
			a.later(func() {
				a.pendingChallenge = challenge.ID
				a.notice("VK требует ручную CAPTCHA. Современный SDK в этой версии Windows не встроен; можно отменить подключение.")
				if challenge.Image != "" {
					a.notice("VK требует код с изображения: нажмите «Открыть проверку», введите код и нажмите «Отправить код»")
				}
			})
		}
		ready := status == "running" || status == "ready"
		if ready && !configured {
			if err := t.Configure(p); err != nil {
				a.later(func() {
					if a.tunnel == t {
						a.disconnect(err.Error())
					}
				})
				return
			}
			configured = true
			a.later(func() {
				if a.tunnel == t && !a.disconnecting {
					a.connected = true
					a.notice("Подключено · " + p.Name)
				}
			})
		}
		stats := t.Stats()
		a.later(func() {
			if a.tunnel == t {
				text(a.detail, "Ядро: "+status+"\r\n"+stats)
			}
		})
	}
}
func (a *app) disconnect(reason string) {
	if a.tunnel == nil || a.disconnecting {
		return
	}
	a.disconnecting = true
	t := a.tunnel
	a.busy = true
	a.connected = false
	a.notice("Отключение и восстановление сети…")
	go func() {
		err := t.Close()
		a.later(func() {
			a.busy = false
			a.tunnel = nil
			a.disconnecting = false
			a.pendingChallenge = ""
			text(a.connect, "Подключить")
			text(a.detail, "")
			text(a.captcha, "")
			if err != nil {
				a.notice("Не удалось завершить очистку сети. Нажмите «Восстановить сеть»")
			} else {
				a.notice(reason)
				if a.exiting {
					user.NewProc("DestroyWindow").Call(a.hwnd)
				}
			}
		})
	}()
}
func (a *app) probe() {
	if !a.connected || a.tunnel == nil {
		a.notice("Проверка IP доступна после подключения туннеля")
		return
	}
	if !a.begin() {
		return
	}
	t := a.tunnel
	a.notice("Проверка HTTPS через туннель…")
	go func() {
		result, err := mobile.ProbeInternet("ip", false, nil)
		a.later(func() {
			if a.tunnel != t || a.disconnecting {
				return
			}
			a.busy = false
			if a.tunnel != t {
				return
			}
			if err != nil {
				a.notice(err.Error())
			} else {
				text(a.detail, result)
				a.notice("HTTPS через выбранные маршруты: успешно")
			}
		})
	}()
}
func (a *app) openCaptcha() {
	if a.tunnel == nil {
		return
	}
	var c struct{ Image string }
	_ = json.Unmarshal([]byte(a.tunnel.Engine.CaptchaJSON()), &c)
	if c.Image == "" {
		a.notice("Современная VK CAPTCHA требует SDK; эта версия Windows его пока не содержит")
		return
	}
	if !strings.HasPrefix(c.Image, "https://") {
		a.notice("Разрешены только HTTPS-изображения CAPTCHA")
		return
	}
	shell := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteW")
	shell.Call(a.hwnd, uintptr(unsafe.Pointer(ptr("open"))), uintptr(unsafe.Pointer(ptr(c.Image))), 0, 0, 1)
}
func (a *app) command(id int) {
	switch id {
	case idImport:
		raw := strings.TrimSpace(value(a.input))
		if strings.HasPrefix(raw, "https://") {
			a.importSource("", raw)
		} else {
			a.importSource(raw, "")
		}
	case idRefresh:
		if a.state.SubscriptionURL == "" {
			a.notice("В сохранённых профилях нет ссылки подписки")
		} else {
			a.importSource("", a.state.SubscriptionURL)
		}
	case idFile:
		a.importFile()
	case idConnect:
		a.toggle()
	case idDelete:
		if a.tunnel != nil || a.busy {
			return
		}
		i := a.selected()
		if i < 0 {
			return
		}
		previous := a.state
		next := append([]client.Profile(nil), a.state.Profiles[:i]...)
		next = append(next, a.state.Profiles[i+1:]...)
		a.state.Profiles = next
		a.state.Selected = 0
		if err := a.save(); err != nil {
			a.state = previous
			a.notice(err.Error())
		} else {
			a.render()
			a.notice("Профиль удалён. Обновление подписки восстановит её список")
		}
	case idProbe:
		a.probe()
	case idRecover:
		if a.tunnel != nil || !a.begin() {
			return
		}
		go func() {
			err := platform.Recover()
			a.later(func() {
				a.busy = false
				if err != nil {
					a.notice(err.Error())
				} else {
					a.notice("Маршруты Blink, DNS и временный IPv6-фильтр очищены")
				}
			})
		}()
	case idOpenCaptcha:
		a.openCaptcha()
	case idCaptcha:
		if a.tunnel != nil && a.pendingChallenge != "" {
			ok := a.tunnel.Engine.SubmitCaptcha(a.pendingChallenge, "key", value(a.captcha))
			text(a.captcha, "")
			if ok {
				a.notice("Код отправлен VK, ожидание ответа…")
			} else {
				a.notice("Проверка истекла или код не принят")
			}
		}
	}
}
func wndProc(h uintptr, msg uint32, w, l uintptr) uintptr {
	switch msg {
	case wmCommand:
		if l != ui.list {
			ui.command(int(w & 0xffff))
		}
		return 0
	case wmUpdate:
		for {
			select {
			case f := <-ui.updates:
				f()
			default:
				return 0
			}
		}
	case wmClose:
		if ui.busy {
			ui.notice("Дождитесь завершения операции, затем закройте окно")
			return 0
		}
		if ui.tunnel != nil {
			ui.exiting = true
			ui.disconnect("Отключено")
			return 0
		}
		user.NewProc("DestroyWindow").Call(h)
		return 0
	case wmDestroy:
		user.NewProc("PostQuitMessage").Call(0)
		return 0
	}
	r, _, _ := user.NewProc("DefWindowProcW").Call(h, uintptr(msg), w, l)
	return r
}
func main() {
	runtime.LockOSThread()
	// One UI owns the adapter and saved state; a second instance must not clean its routes.
	mutex, err := windows.CreateMutex(nil, false, ptr("Global\\Blink.Windows.Client"))
	if err == windows.ERROR_ALREADY_EXISTS {
		windows.CloseHandle(mutex)
		return
	}
	if err != nil {
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
		dialog("Не удалось подготовить geodata")
		return
	}
	var loadErr error
	if raw, e := platform.LoadState(); e == nil {
		loadErr = json.Unmarshal(raw, &ui.state)
	} else if !errors.Is(e, os.ErrNotExist) {
		loadErr = e
	}
	ui.updates = make(chan func(), 64)
	user.NewProc("SetProcessDPIAware").Call()
	instance, _, _ := kernel.NewProc("GetModuleHandleW").Call(0)
	name := ptr("BlinkWindows")
	cursor, _, _ := user.NewProc("LoadCursorW").Call(0, 32512)
	wc := windowClass{Size: uint32(unsafe.Sizeof(windowClass{})), Proc: syscall.NewCallback(wndProc), Instance: instance, Cursor: cursor, Background: 6, Name: name}
	user.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&wc)))
	ui.hwnd, _, _ = createWindow.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(ptr("Blink · Windows · 0.1.0"))), 0x00ca0000|0x00010000, 100, 100, 800, 790, 0, 0, instance, 0)
	if ui.hwnd == 0 {
		return
	}
	ui.font, _, _ = gdi.NewProc("CreateFontW").Call(18, 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(ptr("Segoe UI"))))
	defer gdi.NewProc("DeleteObject").Call(ui.font)
	ui.control("STATIC", "BLINK  /  WINDOWS", 0, 24, 18, 730, 26, 0)
	ui.control("STATIC", "Вставьте HTTPS-подписку, URI, Xray JSON или WireGuard/AmneziaWG .conf", 0, 24, 52, 730, 24, 0)
	ui.input = ui.control("EDIT", "", 0x00800000|0x00200000|0x00001000|0x00000004|0x00000040|0x00010000, 24, 80, 730, 108, 0)
	send.Call(ui.input, 0xc5, client.MaxDocument, 0)
	ui.control("BUTTON", "Импортировать", 0x10000, 24, 200, 150, 34, idImport)
	ui.control("BUTTON", "Открыть файл", 0x10000, 184, 200, 150, 34, idFile)
	ui.control("BUTTON", "Обновить подписку", 0x10000, 344, 200, 200, 34, idRefresh)
	ui.control("STATIC", "Профили  ·  импорт заменяет текущий список", 0, 24, 248, 730, 24, 0)
	ui.list = ui.control("LISTBOX", "", 0x00800000|0x00200000|0x00010000|1, 24, 276, 730, 150, 0)
	ui.connect = ui.control("BUTTON", "Подключить", 0x10000, 24, 438, 180, 40, idConnect)
	ui.control("BUTTON", "Проверить IP", 0x10000, 214, 438, 170, 40, idProbe)
	ui.control("BUTTON", "Удалить", 0x10000, 394, 438, 140, 40, idDelete)
	ui.control("BUTTON", "Восстановить сеть", 0x10000, 544, 438, 210, 40, idRecover)
	ui.status = ui.control("STATIC", "Отключено. Для подключения запустите от имени администратора.", 0, 24, 492, 730, 62, 0)
	ui.detail = ui.control("EDIT", "", 0x00000800|0x00000004, 24, 558, 730, 62, 0)
	ui.captcha = ui.control("EDIT", "", 0x00800000|0x00010000|0x20, 24, 638, 230, 30, 0)
	ui.control("BUTTON", "Отправить код", 0x10000, 266, 636, 180, 34, idCaptcha)
	ui.control("BUTTON", "Открыть проверку", 0x10000, 458, 636, 210, 34, idOpenCaptcha)
	ui.control("STATIC", "IPv4 · Wintun · без автозапуска · профили защищены Windows DPAPI", 0, 24, 688, 730, 24, 0)
	ui.render()
	if loadErr != nil {
		ui.notice("Не удалось прочитать сохранённые профили: " + loadErr.Error())
	}
	user.NewProc("ShowWindow").Call(ui.hwnd, 1)
	var m message
	for {
		r, _, _ := user.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		handled, _, _ := user.NewProc("IsDialogMessageW").Call(ui.hwnd, uintptr(unsafe.Pointer(&m)))
		if handled == 0 {
			user.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&m)))
			user.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&m)))
		}
	}
}
func dialog(s string) {
	user.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(ptr(s))), uintptr(unsafe.Pointer(ptr("Blink"))), 0x10)
}
