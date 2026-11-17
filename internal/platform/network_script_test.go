// SPDX-License-Identifier: GPL-3.0-only
package platform

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/remzone/blink_windows/internal/client"
)

func TestDynamicGuardOnlyForFullIPv4Profiles(t *testing.T) {
	for _, kind := range []string{"xray", "wireguard", "vk"} {
		p := client.Profile{Kind: kind, Addresses: []string{"10.77.0.2/32"}, Routes: []string{"0.0.0.0/0"}, DNS: []string{"1.1.1.1"}, MTU: 1280}
		script := buildNetworkScript(p)
		if !needsIPv6Guard(p) || strings.Contains(script, "New-NetFirewallRule") || strings.Contains(script, "-RemoteAddress") {
			t.Fatalf("%s: full IPv4 must use native guard without NetSecurity address parameters", kind)
		}
		if !strings.Contains(script, "'0.0.0.0/1'") || !strings.Contains(script, "'128.0.0.0/1'") {
			t.Fatal("IPv4 split routes missing")
		}
		p.Routes = []string{"10.20.0.0/16"}
		if needsIPv6Guard(p) || strings.Contains(buildNetworkScript(p), "New-NetFirewallRule") || strings.Contains(buildNetworkScript(p), "Add-DnsClientNrptRule") {
			t.Fatal("selective profile must not install global filters")
		}
		p.Addresses = append(p.Addresses, "fd00::2/128")
		p.Routes = []string{"0.0.0.0/0", "::/0"}
		if needsIPv6Guard(p) || strings.Contains(buildNetworkScript(p), "New-NetFirewallRule") || !strings.Contains(buildNetworkScript(p), "Enable-NetAdapterBinding") {
			t.Fatal("dual-stack profile would lose IPv6")
		}
	}
}

func TestNetworkExceptionExportsCIMMetadataWithoutRawMessage(t *testing.T) {
	raw := []byte(`private-key https://secret.invalid/token
BLINK_NETWORK {"step":"firewall","ok":false,"hresult":-2146233088,"nativeError":4,"innerHresult":-2147024809,"errorId":"Windows System Error 87,New-NetFirewallRule","category":"InvalidArgument","exception":"Microsoft.Management.Infrastructure.CimException","message":"private-key","target":"secret.invalid"}
BLINK_NETWORK {"step":"secret.invalid","ok":false,"hresult":1}`)
	e := networkEvents(raw)
	if len(e) != 1 || e[0].SystemHRESULT != 0 || e[0].NativeError != 4 || e[0].InnerHRESULT != -2147024809 || e[0].ErrorID != "Windows System Error 87,New-NetFirewallRule" {
		t.Fatalf("lost useful CIM fields: %+v", e)
	}
	encoded, _ := json.Marshal(e)
	if strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "secret") {
		t.Fatal("raw data leaked")
	}
}

func TestCIMWrapperDoesNotHideWindowsInvalidArgument(t *testing.T) {
	events := networkEvents([]byte(`BLINK_NETWORK {"step":"firewall","ok":false,"hresult":-2146233088,"nativeError":1,"errorId":"HRESULT 0x80070057,New-NetFirewallRule","category":"InvalidArgument","exception":"Microsoft.Management.Infrastructure.CimException"}`))
	if len(events) != 1 || events[0].SystemHRESULT != 0x80070057 {
		t.Fatalf("inner Windows code missing: %+v", events)
	}
}
