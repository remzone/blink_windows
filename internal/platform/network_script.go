// SPDX-License-Identifier: GPL-3.0-only
package platform

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/remzone/blink_windows/internal/client"
)

// buildNetworkScript is called only after validateNetwork. IPv6 filtering is
// installed through the tunnel-owned dynamic WFP session, before installing OS routes.
func buildNetworkScript(p client.Profile) string {
	script := `$blinkStep='adapter';$a=Get-NetAdapter -Name 'Blink';$i=$a.ifIndex;Get-NetRoute -InterfaceIndex $i -ErrorAction SilentlyContinue | Remove-NetRoute -Confirm:$false -ErrorAction SilentlyContinue;Get-NetIPAddress -InterfaceIndex $i -ErrorAction SilentlyContinue | Remove-NetIPAddress -Confirm:$false -ErrorAction SilentlyContinue;`
	if hasIPv6(p) {
		script += `$blinkStep='binding';Enable-NetAdapterBinding -Name 'Blink' -ComponentID ms_tcpip6 | Out-Null;Set-NetIPInterface -InterfaceIndex $i -AddressFamily IPv6 -NlMtuBytes ` + fmt.Sprint(p.MTU) + ` -InterfaceMetric 5;`
	} else {
		script += `$blinkStep='binding';Disable-NetAdapterBinding -Name 'Blink' -ComponentID ms_tcpip6 | Out-Null;`
	}
	script += `$blinkStep='mtu';Set-NetIPInterface -InterfaceIndex $i -AddressFamily IPv4 -NlMtuBytes ` + fmt.Sprint(p.MTU) + ` -InterfaceMetric 5;`
	for _, raw := range p.Addresses {
		prefix, _ := netip.ParsePrefix(raw)
		family := "IPv4"
		if prefix.Addr().Is6() {
			family = "IPv6"
		}
		script += fmt.Sprintf("$blinkStep='address';New-NetIPAddress -InterfaceIndex $i -IPAddress '%s' -PrefixLength %d -AddressFamily %s | Out-Null;", prefix.Addr(), prefix.Bits(), family)
	}
	dns := []string{}
	for _, d := range p.DNS {
		dns = append(dns, "'"+d+"'")
	}
	script += "$blinkStep='dns';Set-DnsClientServerAddress -InterfaceIndex $i -ServerAddresses @(" + strings.Join(dns, ",") + ");"
	for _, r := range p.Routes {
		routes := []string{r}
		if r == "0.0.0.0/0" {
			routes = []string{"0.0.0.0/1", "128.0.0.0/1"}
		} else if r == "::/0" {
			routes = []string{"::/1", "8000::/1"}
		}
		for _, route := range routes {
			nextHop := "0.0.0.0"
			prefix, _ := netip.ParsePrefix(route)
			if prefix.Addr().Is6() {
				nextHop = "::"
			}
			script += "$blinkStep='route';New-NetRoute -InterfaceIndex $i -DestinationPrefix '" + route + "' -NextHop '" + nextHop + "' -RouteMetric 1 -PolicyStore ActiveStore | Out-Null;"
		}
	}

	full := false
	for _, r := range p.Routes {
		if r == "0.0.0.0/0" || r == "::/0" {
			full = true
		}
	}
	if full {
		script += "$blinkStep='nrpt';Add-DnsClientNrptRule -Namespace '.' -NameServers @(" + strings.Join(dns, ",") + ") -Comment 'Blink.Windows temporary';Clear-DnsClientCache;"
	}
	script = shellErrorFunction + "try {" + script + `} catch {Write-BlinkError 'BLINK_NETWORK' $blinkStep $_;[Console]::Error.WriteLine(('BLINK_STEP='+$blinkStep+';BLINK_HRESULT='+$_.Exception.HResult));exit 1}`
	return script
}

func hasIPv6(p client.Profile) bool {
	for _, s := range p.Addresses {
		a, e := netip.ParsePrefix(s)
		if e == nil && a.Addr().Is6() {
			return true
		}
	}
	return false
}

func needsIPv6Guard(p client.Profile) bool {
	if hasIPv6(p) {
		return false
	}
	for _, route := range p.Routes {
		if route == "0.0.0.0/0" {
			return true
		}
	}
	return false
}
