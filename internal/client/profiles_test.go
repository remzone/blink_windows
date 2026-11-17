// SPDX-License-Identifier: GPL-3.0-only
package client

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T) string {
	t.Helper()
	b, e := os.ReadFile("../../fixtures/vokop.subscription.v1.json")
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func TestMobileSubscriptionFixtures(t *testing.T) {
	for _, name := range []string{"standard-subscription.txt", "standard-subscription.base64"} {
		t.Run(name, func(t *testing.T) {
			b, e := os.ReadFile("../../fixtures/" + name)
			if e != nil {
				t.Fatal(e)
			}
			r, e := Parse(string(b), time.Now(), 0)
			if e != nil {
				t.Fatal(e)
			}
			if len(r.Profiles) != 4 {
				t.Fatalf("got %d profiles", len(r.Profiles))
			}
			for _, p := range r.Profiles {
				if len(p.Raw) == 0 || len(p.Addresses) == 0 || len(p.DNS) == 0 || len(p.Routes) == 0 {
					t.Fatal("missing connection data")
				}
			}
		})
	}
}
func TestVokopCallAliasesPreserveURL(t *testing.T) {
	now := time.Unix(1793300100, 0)
	for _, host := range []string{"vk.com", "vk.ru", "www.vk.com", "www.vk.ru"} {
		t.Run(host, func(t *testing.T) {
			url := "https://" + host + "/call/join/DEMO_TOKEN"
			raw := strings.ReplaceAll(fixture(t), "https://vk.com/call/join/DEMO_TOKEN", url)
			r, e := Parse(raw, now, 0)
			if e != nil {
				t.Fatal(e)
			}
			var p map[string]any
			_ = json.Unmarshal(r.Profiles[0].Raw, &p)
			if p["vkCallURL"] != url || p["manualCaptcha"] != true || p["mode"] != "vk" || p["dnsServer"] != "1.1.1.1" {
				t.Fatal("native handoff altered")
			}
		})
	}
}
func TestVokopRejectsExpiredFutureRollbackAndMalformed(t *testing.T) {
	raw := fixture(t)
	now := time.Unix(1793300100, 0)
	cases := []struct {
		name, raw string
		now       time.Time
		minimum   int64
	}{
		{"expired", raw, time.Unix(1793303600, 0), 0},
		{"future", raw, time.Unix(1793299600, 0), 0},
		{"rollback", raw, now, 2},
		{"duplicate", strings.Replace(raw, `"revision":1`, `"revision":2,"revision":1`, 1), now, 0},
		{"v3", strings.Replace(raw, "vokop.subscription.v1", "vokop.subscription.v3", 1), now, 0},
		{"routing", strings.Replace(raw, `"revision":1`, `"routing":{"mode":"selected"},"revision":1`, 1), now, 0},
		{"hostile host", strings.Replace(raw, "vk.com/call/", "vk.com.evil.invalid/call/", 1), now, 0},
		{"trailing", raw + " {}", now, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, e := Parse(c.raw, c.now, c.minimum); e == nil {
				t.Fatal("accepted invalid subscription")
			}
		})
	}
}
func TestFetchRejectsUnsafeURLs(t *testing.T) {
	for _, u := range []string{"http://example.org/sub", "https://user:pass@example.org/sub", "https://example.org/sub#token", "file:///etc/passwd", "https:///sub"} {
		if _, e := Fetch(context.Background(), u); e == nil {
			t.Fatalf("accepted %s", u)
		}
	}
}
func TestUniqueJSONBounded(t *testing.T) {
	for _, s := range []string{`{"a":1,"a":2}`, `{"a":{"b":1,"b":2}}`, strings.Repeat("[", 65) + "0" + strings.Repeat("]", 65)} {
		if uniqueJSON(s) {
			t.Fatal("ambiguous or unbounded JSON accepted")
		}
	}
	if !uniqueJSON(`{"a":[1,{"b":true}],"c":null}`) {
		t.Fatal("valid JSON rejected")
	}
}
