// SPDX-License-Identifier: GPL-3.0-only
package platform

import (
	"github.com/remzone/blink_windows/internal/client"
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
