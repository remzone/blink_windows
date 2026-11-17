// SPDX-License-Identifier: GPL-3.0-only
package platform

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/remzone/blink_windows/internal/diagnostics"
)

type shellEvent struct {
	Step         string `json:"step"`
	OK           bool   `json:"ok"`
	Count        int    `json:"count"`
	HRESULT      int64  `json:"hresult"`
	NativeError  int64  `json:"nativeError,omitempty"`
	InnerHRESULT int64  `json:"innerHresult,omitempty"`
	ErrorID      string `json:"errorId,omitempty"`
	Category     string `json:"category,omitempty"`
	Exception    string `json:"exception,omitempty"`
}

var cleanupLabels = map[string]string{"nrpt": "правила DNS (NRPT)", "cache": "кэш DNS", "firewall": "брандмауэр", "adapter": "поиск адаптера", "adapter_dns": "DNS адаптера", "addresses": "адреса адаптера", "routes": "маршруты адаптера"}
var technicalID = regexp.MustCompile(`^[A-Za-z0-9_.,:+ -]{1,192}$`)

// Only our ASCII JSON markers are accepted. PowerShell's raw CLIXML and error
// messages may contain arbitrary profile data and are never written to disk.
func cleanupEvents(raw []byte) []shellEvent {
	return parseShellEvents(raw, "BLINK_CLEANUP ", cleanupLabels)
}

func networkEvents(raw []byte) []shellEvent {
	return parseShellEvents(raw, "BLINK_NETWORK ", networkLabels)
}

var networkLabels = map[string]string{"adapter": "сетевой адаптер", "binding": "привязки IPv4/IPv6", "mtu": "MTU", "address": "адрес интерфейса", "dns": "DNS", "route": "маршруты", "nrpt": "правило системного DNS", "firewall": "правило IPv6-фильтра"}

func parseShellEvents(raw []byte, prefix string, labels map[string]string) []shellEvent {
	var events []shellEvent
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) || len(line) > 4096 {
			continue
		}
		var e shellEvent
		if json.Unmarshal([]byte(strings.TrimPrefix(line, prefix)), &e) != nil || labels[e.Step] == "" {
			continue
		}
		for _, id := range []*string{&e.ErrorID, &e.Category, &e.Exception} {
			if !technicalID.MatchString(*id) {
				*id = ""
			}
		}
		events = append(events, e)
		if len(events) == 32 {
			break
		}
	}
	return events
}

func recordCleanup(raw []byte) {
	for _, e := range cleanupEvents(raw) {
		diagnostics.Event("windows.cleanup.step", e)
	}
}

func cleanupFailure(raw []byte) error {
	var parts []string
	for _, e := range cleanupEvents(raw) {
		if !e.OK {
			parts = append(parts, fmt.Sprintf("%s (0x%08X)", cleanupLabels[e.Step], uint32(e.HRESULT)))
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "запуск PowerShell или сетевого модуля")
	}
	return fmt.Errorf("Не удалось завершить очистку Blink: %s. Подробности — в файле журнала в разделе «Диагностика»", strings.Join(parts, "; "))
}

// Empty selections are successful. Missing named CIM objects must not become
// terminating errors; enumerate and filter rather than query by a missing name.
// Every independent step still runs after a failure. No physical adapter changes.
const shellErrorFunction = `
function Write-BlinkError($prefix,$step,$record) {
 $native=0;$inner=0;
 if($record.Exception.PSObject.Properties['NativeErrorCode']){$native=[long]$record.Exception.NativeErrorCode};
 if($record.Exception.InnerException){$inner=[long]$record.Exception.InnerException.HResult};
 [Console]::Error.WriteLine(($prefix+' '+(@{step=$step;ok=$false;count=0;hresult=[long]$record.Exception.HResult;nativeError=$native;innerHresult=$inner;errorId=$record.FullyQualifiedErrorId;category=[string]$record.CategoryInfo.Category;exception=$record.Exception.GetType().FullName}|ConvertTo-Json -Compress)));
};
`

const cleanupScript = shellErrorFunction + `
$script:blinkFailures=0;
function Invoke-BlinkCleanup($step, [scriptblock]$action) {
 try {
  $n=& $action;
  [Console]::Error.WriteLine(('BLINK_CLEANUP '+(@{step=$step;ok=$true;count=[int]$n;hresult=0}|ConvertTo-Json -Compress)));
 } catch {
  $script:blinkFailures++;
  Write-BlinkError 'BLINK_CLEANUP' $step $_;
 }
};
$script:blinkRulesRemoved=0;
Invoke-BlinkCleanup 'nrpt' {
 $r=@(Get-DnsClientNrptRule | Where-Object {$_.Comment -eq 'Blink.Windows temporary'});
 foreach($rule in $r){$rule | Remove-DnsClientNrptRule -Force -ErrorAction Stop;$script:blinkRulesRemoved++};
 return $r.Count;
};
Invoke-BlinkCleanup 'cache' {if($script:blinkRulesRemoved -gt 0){Clear-DnsClientCache -ErrorAction Stop;return 1};return 0};
Invoke-BlinkCleanup 'firewall' {
 $r=@(Get-NetFirewallRule -PolicyStore PersistentStore | Where-Object {$_.Name -eq 'Blink-IPv6'});
 foreach($rule in $r){$rule | Remove-NetFirewallRule -ErrorAction Stop};return $r.Count;
};
$script:blinkAdapters=@();
Invoke-BlinkCleanup 'adapter' {$script:blinkAdapters=@(Get-NetAdapter -IncludeHidden | Where-Object {$_.Name -eq 'Blink'});return $script:blinkAdapters.Count};
foreach($a in $script:blinkAdapters){
 Invoke-BlinkCleanup 'adapter_dns' {Set-DnsClientServerAddress -InterfaceIndex $a.ifIndex -ResetServerAddresses -ErrorAction Stop;return 1};
 Invoke-BlinkCleanup 'addresses' {
  $r=@(Get-NetIPAddress -ErrorAction Stop | Where-Object {$_.InterfaceIndex -eq $a.ifIndex});
  foreach($address in $r){$address | Remove-NetIPAddress -Confirm:$false -ErrorAction Stop};return $r.Count;
 };
 Invoke-BlinkCleanup 'routes' {
  $r=@(Get-NetRoute -ErrorAction Stop | Where-Object {$_.InterfaceIndex -eq $a.ifIndex -and $_.Protocol -eq 'NetMgmt'});
  foreach($route in $r){$route | Remove-NetRoute -Confirm:$false -ErrorAction Stop};return $r.Count;
 };
};
if($script:blinkFailures -gt 0){exit 1}
`
