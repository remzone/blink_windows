// SPDX-License-Identifier: GPL-3.0-only
package platform

import (
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
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"

	"github.com/remzone/blink_windows/internal/client"
	"github.com/remzone/blink_windows/native/mobile"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/tun"
)

// Only the Blink interface is changed; the physical adapter is never reconfigured.
func powershell(script string) ([]byte, error) {
	codes := utf16.Encode([]rune("$ErrorActionPreference='Stop';" + script))
	b := make([]byte, 2*len(codes))
	for i, c := range codes {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	systemDir, err := windows.GetSystemDirectory()
	if err != nil {
		return nil, err
	}
	exe := filepath.Join(systemDir, "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.CommandContext(ctx, exe, "-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(b))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return cmd.CombinedOutput()
}
func Admin() bool {
	var sid *windows.SID
	sid, _ = windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	ok, _ := windows.Token(0).IsMember(sid)
	return ok
}

type Protector struct{ Index uint32 }

func (p Protector) Protect(fd int64) bool {
	// IP_UNICAST_IF takes network byte order; IPV6_UNICAST_IF takes host order.
	index := p.Index
	swapped := ((index & 255) << 24) | ((index & 65280) << 8) | ((index >> 8) & 65280) | (index >> 24)
	e4 := windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IP, 31, int(swapped))
	e6 := windows.SetsockoptInt(windows.Handle(fd), windows.IPPROTO_IPV6, 31, int(index))
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
	for _, s := range append(append([]string{}, p.Addresses...), p.Routes...) {
		a, e := netip.ParsePrefix(s)
		if e != nil || !a.Addr().Is4() {
			return errors.New("Первая версия Windows поддерживает только IPv4-конфигурации")
		}
	}
	for _, s := range p.DNS {
		a, e := netip.ParseAddr(s)
		if e != nil || !a.Is4() {
			return errors.New("Требуется IPv4 DNS")
		}
	}
	return nil
}
func Start(p client.Profile) (*Tunnel, error) {
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
	out, err := powershell(`$r=Get-NetRoute -AddressFamily IPv4 -DestinationPrefix '0.0.0.0/0' | Where-Object {$_.InterfaceAlias -ne 'Blink'} | Sort-Object @{Expression={$_.RouteMetric+(Get-NetIPInterface -InterfaceIndex $_.InterfaceIndex -AddressFamily IPv4).InterfaceMetric}} | Select-Object -First 1;if(!$r){throw 'No physical IPv4 default route'};$r.InterfaceIndex`)
	if err != nil {
		return nil, errors.New("Не найден физический IPv4-маршрут")
	}
	var index uint32
	if _, err = fmt.Sscan(strings.TrimSpace(string(out)), &index); err != nil || index == 0 {
		return nil, errors.New("Не найден индекс физического интерфейса")
	}
	d, err := tun.CreateTUN("Blink", p.MTU)
	if err != nil {
		return nil, errors.New("Не удалось создать Wintun; проверьте DLL, права и конфликт имени Blink")
	}
	t := &Tunnel{Device: d, Protector: Protector{Index: index}, Done: make(chan struct{}), Failed: make(chan struct{})}
	fail := func(err error) (*Tunnel, error) { t.Close(); return nil, err }
	if p.Kind == "vk" {
		t.Engine, err = mobile.StartVK(string(p.Raw), t, t.Protector)
	} else {
		t.Engine, err = mobile.StartProfile(string(p.Raw), t, t.Protector)
	}
	if err != nil {
		return fail(err)
	}
	go t.readPackets()
	// VK must be ready before installing any VPN routes. CAPTCHA is available meanwhile.
	return t, nil
}
func (t *Tunnel) Configure(p client.Profile) error {
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
	script := `$a=Get-NetAdapter -Name 'Blink';$i=$a.ifIndex;Get-NetRoute -InterfaceIndex $i -ErrorAction SilentlyContinue | Remove-NetRoute -Confirm:$false -ErrorAction SilentlyContinue;Get-NetIPAddress -InterfaceIndex $i -ErrorAction SilentlyContinue | Remove-NetIPAddress -Confirm:$false -ErrorAction SilentlyContinue;Set-NetIPInterface -InterfaceIndex $i -AddressFamily IPv4 -NlMtuBytes ` + fmt.Sprint(p.MTU) + ` -InterfaceMetric 5;`
	for _, raw := range p.Addresses {
		prefix, _ := netip.ParsePrefix(raw)
		script += fmt.Sprintf("New-NetIPAddress -InterfaceIndex $i -IPAddress '%s' -PrefixLength %d -AddressFamily IPv4 | Out-Null;", prefix.Addr(), prefix.Bits())
	}
	dns := []string{}
	for _, d := range p.DNS {
		dns = append(dns, "'"+d+"'")
	}
	script += "Set-DnsClientServerAddress -InterfaceIndex $i -ServerAddresses @(" + strings.Join(dns, ",") + ");"
	for _, r := range p.Routes {
		routes := []string{r}
		if r == "0.0.0.0/0" {
			routes = []string{"0.0.0.0/1", "128.0.0.0/1"}
		}
		for _, route := range routes {
			script += "New-NetRoute -InterfaceIndex $i -DestinationPrefix '" + route + "' -NextHop '0.0.0.0' -RouteMetric 1 -PolicyStore ActiveStore | Out-Null;"
		}
	}
	// IPv6 is disabled only on Blink. Full tunnel IPv6 is blocked by a dedicated firewall rule.
	script += `Disable-NetAdapterBinding -Name 'Blink' -ComponentID ms_tcpip6 | Out-Null;`
	full := false
	for _, r := range p.Routes {
		if r == "0.0.0.0/0" {
			full = true
		}
	}
	if full {
		script += "Add-DnsClientNrptRule -Namespace '.' -NameServers @(" + strings.Join(dns, ",") + ") -Comment 'Blink.Windows temporary';Clear-DnsClientCache;"
		script += `New-NetFirewallRule -Name 'Blink-IPv6' -DisplayName 'Blink IPv6 while connected' -Direction Outbound -Action Block -RemoteAddress '::/0' -PolicyStore PersistentStore | Out-Null;`
	}
	if _, err := powershell(script); err != nil {
		return errors.New("Не удалось настроить адреса, DNS, маршруты или IPv6-фильтр; подключение отменено")
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

// Cleanup attempts every independent operation, even if a policy denies one of them.
const cleanupScript = `
$failures=@();
try {Get-DnsClientNrptRule | Where-Object {$_.Comment -eq 'Blink.Windows temporary'} | Remove-DnsClientNrptRule -Force;Clear-DnsClientCache} catch {$failures+='DNS'};
try {$r=Get-NetFirewallRule -Name 'Blink-IPv6' -PolicyStore PersistentStore -ErrorAction SilentlyContinue;if($r){$r | Remove-NetFirewallRule}} catch {$failures+='Firewall'};
$a=Get-NetAdapter -Name 'Blink' -ErrorAction SilentlyContinue;
if($a){
 try {Set-DnsClientServerAddress -InterfaceIndex $a.ifIndex -ResetServerAddresses} catch {$failures+='Adapter DNS'};
 try {Get-NetIPAddress -InterfaceIndex $a.ifIndex -ErrorAction SilentlyContinue | Remove-NetIPAddress -Confirm:$false} catch {$failures+='Addresses'};
 try {Get-NetRoute -InterfaceIndex $a.ifIndex -ErrorAction SilentlyContinue | Where-Object {$_.Protocol -eq 'NetMgmt'} | Remove-NetRoute -Confirm:$false} catch {$failures+='Routes'};
};
if($failures.Count){throw ('Blink cleanup failed: '+($failures -join ', '))}
`

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
		return errors.New("Не удалось очистить все настройки Blink; проверьте права администратора и сетевые политики Windows")
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
