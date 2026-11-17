// SPDX-License-Identifier: GPL-3.0-only
package client

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/remzone/blink_windows/native/mobile"
)

func TestVokopV3RoutingAndPanelOptions(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal([]byte(fixture(t)), &doc); err != nil {
		t.Fatal(err)
	}
	doc["schema"] = "vokop.subscription.v3"
	doc["routing"] = map[string]any{"schema": "blink.routing.v1", "mode": "selected", "revision": 3, "updated_at": 1793300000, "domains": []string{"example.org"}, "ips": []string{"203.0.113.0/24"}}
	turn := doc["profiles"].([]any)[0].(map[string]any)["turn"].(map[string]any)
	turn["options"] = map[string]any{"transport": "tcp", "preset": "custom", "dns": "9.9.9.9", "relay": "relay.example.org:3478", "connections": 4, "mtu": 1100, "pace": 128, "window": 32, "interval": 30, "congestion": true}
	raw, _ := json.Marshal(doc)
	result, err := Parse(string(raw), time.Unix(1793300100, 0), 0)
	if err != nil {
		t.Fatal(err)
	}
	var vk mobile.VKProfile
	if err = json.Unmarshal(result.Profiles[0].Raw, &vk); err != nil {
		t.Fatal(err)
	}
	if vk.TURNTransport != "tcp" || vk.DNSServer != "9.9.9.9" || vk.Tuning.PaceKiB != 128 || vk.Tuning.Window != 32 || vk.Tuning.IntervalMS != 30 || !vk.Tuning.Congestion || vk.Streams != 2 || vk.Routing == nil || vk.Routing.Mode != "selected" {
		t.Fatalf("panel contract lost: %+v", vk)
	}
	p, err := ConnectionProfile(result.Profiles[0], DefaultVKOptions(), RoutingOptions{Mode: "inherit", Exclude: "local.example.org"})
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(p.Raw, &vk)
	if vk.Routing.Mode != "selected" || len(vk.Routing.ExcludeDomains) != 1 || vk.TURNTransport != "tcp" {
		t.Fatal("inherited routing/options overwritten")
	}
	local := DefaultVKOptions()
	local.ServerDefaults = false
	local.Transport = "udp"
	local.Connections = 1
	p, err = ConnectionProfile(result.Profiles[0], local, RoutingOptions{Mode: "all"})
	if err != nil {
		t.Fatal(err)
	}
	_ = json.Unmarshal(p.Raw, &vk)
	if vk.TURNTransport != "udp" || vk.Streams != 1 || vk.Routing.Mode != "all" {
		t.Fatal("explicit overrides not applied")
	}
	// Unknown panel tuning must still fail rather than silently drift.
	raw = []byte(strings.Replace(string(raw), `"pace":128`, `"unsupported":true,"pace":128`, 1))
	if _, err = Parse(string(raw), time.Unix(1793300100, 0), 0); err == nil {
		t.Fatal("unknown panel option accepted")
	}
}
func TestVKOptionsRejectUnsafeAndOutOfRange(t *testing.T) {
	cases := []func(*VKOptions){func(v *VKOptions) { v.Transport = "quic" }, func(v *VKOptions) { v.DNS = "dns.example.org" }, func(v *VKOptions) { v.Connections = 17 }, func(v *VKOptions) { v.MTU = 1400 }, func(v *VKOptions) { v.Pace = 1 }, func(v *VKOptions) { v.Window = 512 }, func(v *VKOptions) { v.Relay = "host:0" }}
	for _, change := range cases {
		v := DefaultVKOptions()
		change(&v)
		if v.Validate() == nil {
			t.Fatalf("invalid settings accepted: %+v", v)
		}
	}
}
func TestSelectedRoutingRequiresDestinations(t *testing.T) {
	if _, err := (RoutingOptions{Mode: "selected"}).Policy(nil); err == nil {
		t.Fatal("empty selected policy accepted")
	}
}

func TestAllWithExclusionsIsNotSilentlyIgnored(t *testing.T) {
	raw, err := (RoutingOptions{Mode: "all", Exclude: "local.example.org\n192.168.0.0/16"}).Policy(nil)
	if err != nil {
		t.Fatal(err)
	}
	var p mobile.RoutingPolicy
	if json.Unmarshal([]byte(raw), &p) != nil {
		t.Fatal("invalid policy")
	}
	if p.Mode != "selected" || len(p.IPs) != 2 || len(p.ExcludeDomains) != 1 || len(p.ExcludeIPs) != 1 || p.Validate() != nil {
		t.Fatal("all except exclusions were lost")
	}
}
