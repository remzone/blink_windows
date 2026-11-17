// SPDX-License-Identifier: GPL-3.0-only
package client

import (
	"encoding/json"
	"testing"
	"time"
)

func imported(kind, name string, revision int64) ImportResult {
	raw, _ := json.Marshal(map[string]string{"fixture": name})
	return ImportResult{Profiles: []Profile{{Kind: kind, Name: name, Raw: raw}}, Revision: revision, ExpiresAt: time.Now().Add(time.Hour).Unix()}
}
func TestIndependentSubscriptionsAndRefresh(t *testing.T) {
	now := time.Now()
	s, err := (State{}).MergeImport(imported("vk", "TURN one", 3), "https://turn.example/sub/fixture", now)
	if err != nil {
		t.Fatal(err)
	}
	turnID := s.Profiles[0].ID
	s, err = s.MergeImport(imported("wireguard", "VPN one", 0), "https://vpn.example/sub/fixture", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Sources) != 2 || len(s.Profiles) != 2 || s.Profiles[0].ID != turnID {
		t.Fatal("import overwrote independent source")
	}
	if s.MinimumRevision("https://turn.example/sub/fixture") != 3 || s.MinimumRevision("https://vpn.example/sub/fixture") != 0 {
		t.Fatal("revision leaked between sources")
	}
	old := s.Clone()
	s, err = s.MergeImport(imported("vk", "TURN updated", 4), "https://turn.example/sub/fixture", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Profiles) != 2 || s.Profiles[0].Name != "VPN one" || s.Profiles[s.Selected].Name != "VPN one" {
		t.Fatal("refresh changed independent VPN selection")
	}
	if old.Sources[0].Revision != 3 || old.Profiles[0].Name != "TURN one" {
		t.Fatal("merge mutated previous state before save")
	}
	s.SelectProfile(s.PreferredIndex("turn"))
	turnSelected := s.Profiles[s.Selected].ID
	s.SelectProfile(s.PreferredIndex("vpn"))
	b, _ := json.Marshal(s)
	var restored State
	if json.Unmarshal(b, &restored) != nil {
		t.Fatal("round trip")
	}
	restored.Normalize()
	if restored.Profiles[restored.PreferredIndex("turn")].ID != turnSelected {
		t.Fatal("TURN selection lost after restart")
	}
}
func TestMixedPanelKeepsIndependentManualAndVPN(t *testing.T) {
	now := time.Now()
	s, _ := (State{}).MergeImport(imported("xray", "manual", 0), "", now)
	s, _ = s.MergeImport(imported("xray", "external", 0), "https://vpn.example/sub/fixture", now)
	mixed := imported("vk", "relay", 2)
	mixed.Profiles = append(mixed.Profiles, imported("xray", "managed", 0).Profiles...)
	s, err := s.MergeImport(mixed, "https://panel.example/sub/fixture", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Profiles) != 4 || len(s.Sources) != 3 {
		t.Fatal("mixed import dropped independent profiles")
	}
	s, err = s.MergeImport(imported("vk", "relay update", 3), "https://panel.example/sub/fixture", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Profiles) != 3 || s.Profiles[0].Name != "manual" || s.Profiles[1].Name != "external" {
		t.Fatal("source replacement crossed ownership boundary")
	}
}
func TestLegacyMigrationAndRejectedImportPreserveState(t *testing.T) {
	s := State{Profiles: imported("vk", "legacy", 0).Profiles, Selected: 0, SubscriptionURL: "https://panel.example/fixture", Revision: 9, ExpiresAt: 123}
	s.Normalize()
	if len(s.Sources) != 1 || s.Sources[0].Revision != 9 || s.Profiles[0].ID == "" || s.Profiles[0].SourceID != s.Sources[0].ID {
		t.Fatal("legacy migration")
	}
	before, _ := json.Marshal(s)
	_, err := s.MergeImport(ImportResult{}, "https://empty.example/fixture", time.Now())
	if err == nil {
		t.Fatal("empty source accepted")
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("failure modified old data")
	}
}

func TestParsedMobileSubscriptionsCoexist(t *testing.T) {
	now := time.Unix(1793300100, 0)
	turn, err := Parse(fixture(t), now, 0)
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := Parse("vless://00000000-0000-4000-8000-000000000001@vpn.example.org:443?type=tcp&security=tls&sni=vpn.example.org#Ordinary", now, 0)
	if err != nil {
		t.Fatal(err)
	}
	s, err := (State{}).MergeImport(turn, "https://panel.example/turn-fixture", now)
	if err != nil {
		t.Fatal(err)
	}
	s, err = s.MergeImport(ordinary, "https://panel.example/vpn-fixture", now)
	if err != nil {
		t.Fatal(err)
	}
	if s.PreferredIndex("turn") < 0 || s.PreferredIndex("vpn") < 0 || len(s.Sources) != 2 {
		t.Fatal("one mode erased the other")
	}
	s.SelectProfile(s.PreferredIndex("turn"))
	oldID := s.Profiles[s.Selected].ID
	turn.Profiles[0].Raw = append(turn.Profiles[0].Raw, ' ')
	s, err = s.MergeImport(turn, "https://panel.example/turn-fixture", now)
	if err != nil {
		t.Fatal(err)
	}
	if s.Profiles[s.Selected].ID != oldID {
		t.Fatal("panel profile selection changed despite stable panel id")
	}
}
