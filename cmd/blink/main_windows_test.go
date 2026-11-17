// SPDX-License-Identifier: GPL-3.0-only
package main

import (
	"encoding/json"
	"github.com/remzone/blink_windows/internal/client"
	"github.com/remzone/blink_windows/internal/platform"
	"strings"
	"testing"
	"time"
)

func TestSnapshotSeparatesSourcesWithoutPublishingCredentials(t *testing.T) {
	now := time.Now()
	s := client.State{}
	for _, kind := range []string{"vk", "xray"} {
		source := "https://example.invalid/" + kind + "/PRIVATE_SUBSCRIPTION_TOKEN"
		result := client.ImportResult{Profiles: []client.Profile{{Kind: kind, Name: "Server", Raw: json.RawMessage(`{"password":"PRIVATE_PROFILE_KEY"}`)}}}
		var err error
		s, err = s.MergeImport(result, source, now)
		if err != nil {
			t.Fatal(err)
		}
	}
	app := Application{state: s, phase: "disconnected"}
	snapshot := app.Snapshot()
	raw, _ := json.Marshal(snapshot)
	for _, secret := range []string{"PRIVATE_SUBSCRIPTION_TOKEN", "PRIVATE_PROFILE_KEY", "subscriptionURL", "\"raw\""} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("snapshot leaked %s", secret)
		}
	}
	if len(snapshot.Sources) != 2 || len(snapshot.Profiles) != 2 || snapshot.Selections["vpn"] < 0 || snapshot.Selections["turn"] < 0 {
		t.Fatal("missing independent modes")
	}
	if snapshot.Profiles[0].Kind != "turn" {
		t.Fatal("provider identifier in public kind")
	}
}

func TestFailureFromOldTunnelCannotCancelCurrentConnection(t *testing.T) {
	old, current := &platform.Tunnel{}, &platform.Tunnel{}
	cancelled := false
	app := Application{tunnel: current, phase: "connected", connectionCancel: func() { cancelled = true }}
	app.failConnection(old, "OLD_FAILURE", "old attempt failed")
	if cancelled || app.tunnel != current || app.phase != "connected" || app.connection.Error != "" {
		t.Fatal("old attempt affected current connection")
	}
}
