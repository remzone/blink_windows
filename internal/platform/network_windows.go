// SPDX-License-Identifier: GPL-3.0-only
package platform

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"

	"github.com/remzone/blink_windows/internal/client"
	"github.com/remzone/blink_windows/internal/diagnostics"
	"github.com/remzone/blink_windows/native/mobile"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

// Only the Blink interface is changed; the physical adapter is never reconfigured.
func powershell(script string) ([]byte, error) {
	return powershellContext(context.Background(), script)
}
func powershellContext(parent context.Context, script string) ([]byte, error) {
	codes := utf16.Encode([]rune("$ErrorActionPreference='Stop';$ProgressPreference='SilentlyContinue';" + script))
	b := make([]byte, 2*len(codes))
	for i, c := range codes {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	systemDir, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	exe := filepath.Join(systemDir, "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.CommandContext(ctx, exe, "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(b))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	// Keep PowerShell progress/CLIXML on stderr out of successful stdout.
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = 2 * time.Second
	operation := "configure"
	if script == cleanupScript {
		operation = "cleanup"
	}
	started := time.Now()
	diagnostics.Event("windows.powershell.start", map[string]any{"operation": operation})
	err = cmd.Run()
	out := stdout.Bytes()
	if failure, ok := err.(*exec.ExitError); ok {
		failure.Stderr = stderr.Bytes()
	}
	if script == cleanupScript {
		recordCleanup(stderr.Bytes())
	}
	fields := map[string]any{"operation": operation, "elapsedMs": time.Since(started).Milliseconds(), "ok": err == nil, "timeout": ctx.Err() == context.DeadlineExceeded, "cancelled": ctx.Err() == context.Canceled}
	if cmd.ProcessState != nil {
		fields["exitCode"] = cmd.ProcessState.ExitCode()
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		fields["win32"] = uint32(errno)
	}
	if script != cleanupScript {
		for _, e := range networkEvents(stderr.Bytes()) {
			diagnostics.Event("windows.network.step", e)
		}
		if match := networkMarker.FindSubmatch(stderr.Bytes()); len(match) == 3 {
			fields["step"] = string(match[1])
			fields["hresult"], _ = strconv.ParseInt(string(match[2]), 10, 64)
		}
	}
	diagnostics.Event("windows.powershell.finish", fields)
	if ctx.Err() != nil {
		return out, ctx.Err()
	}
	return out, err
}
func Admin() bool {
	var sid *windows.SID
	sid, _ = windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	ok, _ := windows.Token(0).IsMember(sid)
	return ok
}

type Protector struct {
	Index  uint32
	Index6 uint32
}

func (p Protector) Protect(fd int64) bool {
	// IP_UNICAST_IF takes network byte order; IPV6_UNICAST_IF takes host order.
	index := p.Index
	swapped := ((index & 255) << 24) | ((index & 65280) << 8) | ((index >> 8) & 65280) | (index >> 24)
	e4 := windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IP, 31, int(swapped))
	index6 := p.Index6
	if index6 == 0 {
		index6 = index
	}
	e6 := windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IPV6, 31, int(index6))
	return e4 == nil || e6 == nil
}

type Tunnel struct {
	mu        sync.Mutex
	Device    tun.Device
	Engine    *mobile.Engine
	Protector Protector
	once      sync.Once
	Done      chan struct{}
	Failed    chan struct{}
}

func (t *Tunnel) WritePacket(b []byte) bool {
	_, err := t.Device.Write([][]byte{b}, 0)
	return err == nil
}

func validateNetwork(p client.Profile) error {
	if p.MTU < 576 || p.MTU > 9000 || len(p.Addresses) == 0 || len(p.Routes) == 0 || len(p.DNS) == 0 {
		return errors.New("Профиль должен задавать адреса, маршруты и DNS")
	}
	v6 := hasIPv6(p)
	if v6 {
		wg := p.Kind == "wireguard" || p.Kind == "amneziawg"
		if p.Kind == "vk" {
			var v mobile.VKProfile
			_ = json.Unmarshal(p.Raw, &v)
			wg = v.Backend == "wireguard"
		}
		if !wg || p.MTU < 1280 {
			return errors.New("IPv6 требует WireGuard/AmneziaWG и MTU не меньше 1280")
		}
		var route mobile.RoutingPolicy
		_ = json.Unmarshal(p.Routing, &route)
		if route.Mode == "selected" {
			return errors.New("Выборочная маршрутизация общего ядра использует IPv4; для dual-stack WG выберите полный профиль")
		}
	}
	for _, s := range append(append([]string{}, p.Addresses...), p.Routes...) {
		a, e := netip.ParsePrefix(s)
		if e != nil || a.Addr().Is6() && !v6 {
			return errors.New("Некорректный маршрут или отсутствует IPv6-адрес интерфейса")
		}
	}
	for _, s := range p.DNS {
		a, e := netip.ParseAddr(s)
		if e != nil || a.Is6() && !v6 {
			return errors.New("DNS должен быть IP-адресом поддерживаемого семейства")
		}
	}
	return nil
}
func Start(ctx context.Context, p client.Profile, report func(string)) (*Tunnel, error) {
	return awaitStart(ctx, startGate, func() (*Tunnel, error) { return start(ctx, p, report) }, func(t *Tunnel) {
		if t != nil {
			_ = t.Close()
		}
	})
}
func start(ctx context.Context, p client.Profile, report func(string)) (*Tunnel, error) {
	report("adapter")
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !Admin() {
		return nil, errors.New("Запустите Blink от имени администратора для Wintun и маршрутов")
	}
	if err := validateNetwork(p); err != nil {
		return nil, err
	}
	// Do not silently load arbitrary DLLs from the working directory.
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	dll := filepath.Join(filepath.Dir(exe), "wintun.dll")
	handle, err := windows.LoadLibraryEx(dll, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_SYSTEM32)
	if err != nil {
		return nil, errors.New("Нет подходящего подписанного wintun.dll рядом с Blink.exe")
	}
	defer windows.FreeLibrary(handle)
	// A killed previous process may leave NRPT pointing at a dead tunnel. Clear
	// only Blink-owned state before resolving any endpoint or creating a new core.
	report("recovery")
	if _, err := powershellContext(ctx, cleanupScript); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, cleanupError(err)
	}
	report("adapter")
	index, err := defaultInterface(windows.AF_INET)
	if err != nil {
		return nil, fmt.Errorf("Не удалось определить сетевой адаптер: %w", err)
	}
	if index == 0 {
		return nil, errors.New("Нет активного IPv4-маршрута в интернет. Проверьте Wi-Fi или Ethernet и повторите подключение")
	}
	index6, _ := defaultInterface(windows.AF_INET6)
	if index6 == 0 {
		index6 = index
	}
	d, err := tun.CreateTUN("Blink", p.MTU)
	if err != nil {
		var number syscall.Errno
		if errors.As(err, &number) {
			return nil, fmt.Errorf("Не удалось создать Wintun (Windows %d); проверьте драйвер, права и конфликт имени Blink", uint32(number))
		}
		return nil, errors.New("Не удалось создать Wintun; проверьте драйвер, права и конфликт имени Blink")
	}
	if err := ctx.Err(); err != nil {
		_ = d.Close()
		return nil, err
	}
	report("engine")
	t := &Tunnel{Device: d, Protector: Protector{Index: index, Index6: index6}, Done: make(chan struct{}), Failed: make(chan struct{})}
	fail := func(err error) (*Tunnel, error) { t.Close(); return nil, err }
	if p.Kind == "vk" {
		t.Engine, err = mobile.StartVK(string(p.Raw), t, t.Protector)
	} else {
		if len(p.Routing) > 0 {
			t.Engine, err = mobile.StartProfileRouted(string(p.Raw), string(p.Routing), t, t.Protector)
		} else {
			t.Engine, err = mobile.StartProfile(string(p.Raw), t, t.Protector)
		}
	}
	if err != nil {
		return fail(err)
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	go t.readPackets()
	// VK must be ready before installing any VPN routes. CAPTCHA is available meanwhile.
	return t, nil
}
func (t *Tunnel) Configure(ctx context.Context, p client.Profile) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	select {
	case <-t.Done:
		return errors.New("Подключение отменено")
	default:
	}

	if err := validateNetwork(p); err != nil {
		return err
	}
	script := buildNetworkScript(p)
	if _, err := powershellContext(ctx, script); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return errors.New("Windows не применила адреса, DNS, маршруты или IPv6-фильтр за 30 секунд. Подключение отменено")
		}
		return networkConfigurationError(err)
	}
	return nil
}
func (t *Tunnel) readPackets() {
	defer close(t.Failed)
	bufs := [][]byte{make([]byte, 65535)}
	sizes := []int{0}
	for {
		n, err := t.Device.Read(bufs, sizes, 0)
		if err != nil {
			return
		}
		for i := 0; i < n; i++ {
			if err = t.Engine.Input(bufs[i][:sizes[i]]); err != nil {
				return
			}
		}
	}
}

func (t *Tunnel) Close() error {
	var result error
	t.once.Do(func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		close(t.Done)
		_, result = powershell(cleanupScript)
		_ = t.Device.Close()
		if t.Engine != nil {
			t.Engine.Stop()
		}
	})
	return result
}
func Recover() error {
	if !Admin() {
		return errors.New("Восстановление требует запуска от администратора")
	}
	_, err := powershell(cleanupScript)
	if err != nil {
		return cleanupError(err)
	}
	return nil
}
func (t *Tunnel) Stats() string {
	var s struct {
		TX uint64 `json:"tx"`
		RX uint64 `json:"rx"`
	}
	_ = json.Unmarshal([]byte(t.Engine.StatsJSON()), &s)
	return fmt.Sprintf("Отправлено %.2f МиБ   ·   Получено %.2f МиБ", float64(s.TX)/(1<<20), float64(s.RX)/(1<<20))
}

// Read the routing table directly: console encoding, CLIXML and localization
// must never participate in selecting an adapter for protected transport sockets.
func defaultInterface(family winipcfg.AddressFamily) (uint32, error) {
	routes, err := winipcfg.GetIPForwardTable2(family)
	if err != nil {
		return 0, err
	}
	var candidates []routeCandidate
	for _, r := range routes {
		prefix := r.DestinationPrefix.Prefix()
		if !prefix.IsValid() || prefix.Bits() != 0 || !prefix.Addr().IsUnspecified() || r.Loopback || r.ValidLifetime == 0 {
			continue
		}
		adapter, err := r.InterfaceLUID.Interface()
		if err != nil {
			continue
		}
		ip, err := r.InterfaceLUID.IPInterface(family)
		if err != nil {
			continue
		}
		candidates = append(candidates, routeCandidate{
			Index: r.InterfaceIndex, Alias: adapter.Alias(), Up: adapter.OperStatus == winipcfg.IfOperStatusUp && ip.Connected,
			RouteMetric: r.Metric, InterfaceMetric: ip.Metric,
		})
	}
	return chooseDefaultInterface(candidates), nil
}

// Never surface arbitrary PowerShell stderr: only our fixed stage marker and a
// numeric HRESULT are allowed into a support report.
var networkMarker = regexp.MustCompile(`BLINK_STEP=(adapter|binding|mtu|address|dns|route|nrpt|firewall);BLINK_HRESULT=(-?[0-9]+)`)

func networkConfigurationError(err error) error {
	var failure *exec.ExitError
	if errors.As(err, &failure) {
		for _, e := range networkEvents(failure.Stderr) {
			if !e.OK {
				code := fmt.Sprintf("HRESULT 0x%08X", uint32(e.HRESULT))
				if e.NativeError != 0 {
					code += fmt.Sprintf(", CIM %d", e.NativeError)
				}
				if e.InnerHRESULT != 0 {
					code += fmt.Sprintf(", внутренний HRESULT 0x%08X", uint32(e.InnerHRESULT))
				}
				return fmt.Errorf("Windows не настроила %s (%s). Подключение отменено; подробности — в файле журнала", networkLabels[e.Step], code)
			}
		}
		match := networkMarker.FindSubmatch(failure.Stderr)
		if len(match) == 3 {
			number, e := strconv.ParseInt(string(match[2]), 10, 64)
			if e == nil {

				return fmt.Errorf("Windows не настроила %s (HRESULT 0x%08X). Подключение отменено", networkLabels[string(match[1])], uint32(number))
			}
		}
	}
	return errors.New("Не удалось настроить адреса, DNS, маршруты или IPv6-фильтр; подключение отменено")
}

func cleanupError(err error) error {
	var failure *exec.ExitError
	if errors.As(err, &failure) {
		return cleanupFailure(failure.Stderr)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("Очистка Blink не завершилась за 30 секунд. Подробности — в файле журнала в разделе «Диагностика»")
	}
	return cleanupFailure(nil)
}
