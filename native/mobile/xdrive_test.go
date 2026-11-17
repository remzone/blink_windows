// SPDX-License-Identifier: GPL-3.0-only
package mobile

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func xdriveTestConfig(t *testing.T, service string) string {
	t.Helper()
	settings := map[string]any{"service": service, "remoteFolder": "blink-test", "secrets": []string{"ACCESS", "SECRET"}}
	switch service {
	case "s3":
		settings["s3"] = map[string]any{"endpoint": "https://storage.yandexcloud.net", "region": "ru-central1", "bucket": "blink-test", "pathStyle": false}
	case "google-drive":
		settings["secrets"] = []string{"CLIENT_ID", "CLIENT_SECRET", "REFRESH_TOKEN"}
	case "template":
		raw, err := os.ReadFile("testdata/xdrive-yandex-template.json")
		if err != nil {
			t.Fatal(err)
		}
		var tmpl any
		if err := json.Unmarshal(raw, &tmpl); err != nil {
			t.Fatal(err)
		}
		settings["template"] = tmpl
		settings["remoteFolder"] = "disk:/blink-xdrive-test"
		settings["secrets"] = []string{"TEST_OAUTH_TOKEN"}
	}
	raw, err := json.Marshal(map[string]any{
		"remarks": "XDRIVE " + service,
		"outbounds": []any{map[string]any{
			"tag": "proxy", "protocol": "vless",
			"settings":       map[string]any{"address": "logical-server.invalid", "port": 443, "id": "00000000-0000-4000-8000-000000000001", "encryption": "mlkem768x25519plus.native.0rtt.VVrR-37dPZgOD9AWCl9ZBpWpeK21U1_qmW_8EmstOS0"},
			"streamSettings": map[string]any{"network": "xdrive", "security": "none", "xdriveSettings": settings},
			"mux":            map[string]any{"enabled": true, "concurrency": 128, "xudpConcurrency": 128, "xudpProxyUDP443": "reject"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestXDriveJSONSubscriptionImport(t *testing.T) {
	for _, service := range []string{"s3", "google-drive", "template"} {
		t.Run(service, func(t *testing.T) {
			source := xdriveTestConfig(t, service)
			raw, _, err := prepareXrayJSON(source)
			if err != nil {
				t.Fatal(err)
			}
			prepared, err := prepareJSONEndpoints(raw)
			if err != nil {
				t.Fatalf("XDRIVE must not resolve logical VLESS endpoint: %v", err)
			}
			var before, after map[string]any
			_ = json.Unmarshal([]byte(source), &before)
			_ = json.Unmarshal(prepared, &after)
			if !reflect.DeepEqual(before["outbounds"], after["outbounds"]) {
				t.Fatal("XDRIVE settings lost during preparation")
			}
			imported, err := ImportProfiles("[" + source + "]")
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Profiles []ImportedProfile
				Rejected []any
			}
			if err := json.Unmarshal([]byte(imported), &result); err != nil {
				t.Fatal(err)
			}
			if len(result.Profiles) != 1 || len(result.Rejected) != 0 || result.Profiles[0].Kind != "xray-json" || result.Profiles[0].Source != source {
				t.Fatal("JSON subscription import lost XDRIVE")
			}
			alias := strings.Replace(source, `"network":"xdrive"`, `"method":"xdrive"`, 1)
			if _, err := prepareJSONEndpoints([]byte(alias)); err != nil {
				t.Fatal("method alias resolved logical endpoint")
			}
		})
	}
}

func TestXDriveRejectsIncompleteRequirements(t *testing.T) {
	source := xdriveTestConfig(t, "s3")
	for _, bad := range []string{
		strings.Replace(source, `"enabled":true`, `"enabled":false`, 1),
		strings.Replace(source, `"concurrency":128`, `"concurrency":-1`, 1),
		strings.Replace(source, `"xudpConcurrency":128`, `"xudpConcurrency":-1`, 1),
		strings.Replace(source, `"xudpProxyUDP443":"reject"`, `"xudpProxyUDP443":"skip"`, 1),
		strings.Replace(source, `"encryption":"mlkem768x25519plus.native.0rtt.VVrR-37dPZgOD9AWCl9ZBpWpeK21U1_qmW_8EmstOS0"`, `"encryption":"none"`, 1),
		strings.Replace(source, `"bucket":"blink-test"`, `"bucket":"bad/bucket"`, 1),
	} {
		if _, err := ImportProfile(bad); err == nil {
			t.Fatal("invalid XDRIVE requirements accepted")
		}
	}
}
