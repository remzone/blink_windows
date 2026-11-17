// SPDX-License-Identifier: GPL-3.0-only
package platform

import (
	"github.com/remzone/blink_windows/internal/client"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
	"strings"
	"testing"
)

func TestRejectMalformedNetworkBeforePowerShell(t *testing.T) {
	p := client.Profile{Addresses: []string{"10.77.0.2/32"}, Routes: []string{"0.0.0.0/0"}, DNS: []string{"1.1.1.1"}, MTU: 1280}
	if err := validateNetwork(p); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"1.1.1.1';exit;#", "dns.example.org", "::1"} {
		bad := p
		bad.DNS = []string{s}
		if validateNetwork(bad) == nil {
			t.Fatalf("accepted DNS %q", s)
		}
	}
	bad := p
	bad.Routes = []string{"0.0.0.0/0';exit;#"}
	if validateNetwork(bad) == nil {
		t.Fatal("unsafe route accepted")
	}
}

func TestDualStackWGRequiresExplicitAddressAndFullCoreRouting(t *testing.T) {
	p := client.Profile{Kind: "wireguard", Addresses: []string{"10.77.0.2/32", "fd00::2/128"}, Routes: []string{"0.0.0.0/0", "::/0"}, DNS: []string{"1.1.1.1", "2606:4700:4700::1111"}, MTU: 1280}
	if err := validateNetwork(p); err != nil {
		t.Fatal(err)
	}
	p.Routing = []byte(`{"mode":"selected"}`)
	if validateNetwork(p) == nil {
		t.Fatal("unsupported IPv6 selected routing accepted")
	}
	p.Routing = nil
	p.Kind = "xray"
	if validateNetwork(p) == nil {
		t.Fatal("unsupported Xray IPv6 accepted")
	}
}

// Read-only Windows regression: choosing the upstream must not parse PowerShell.
func TestDefaultInterfaceUsesWindowsRoutingTable(t *testing.T) {
	index, err := defaultInterface(windows.AF_INET)
	if err != nil {
		t.Fatal(err)
	}
	if index == 0 {
		t.Skip("No active IPv4 default route on this machine")
	}
	luid, err := winipcfg.LUIDFromIndex(index)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := luid.Interface()
	if err != nil {
		t.Fatal(err)
	}
	if strings.EqualFold(adapter.Alias(), "Blink") || adapter.OperStatus != winipcfg.IfOperStatusUp {
		t.Fatalf("invalid upstream: %s", adapter.Alias())
	}
}

func TestPowerShellStderrCannotCorruptStdout(t *testing.T) {
	out, err := powershell(`[Console]::Error.WriteLine('diagnostic');Write-Output 42`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "42" {
		t.Fatalf("unexpected stdout: %q", out)
	}
}
