// SPDX-License-Identifier: GPL-3.0-only
package platform

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDPAPIStateRoundTripAndTampering(t *testing.T) {
	t.Setenv("APPDATA", t.TempDir())
	raw := []byte(`{"subscriptionURL":"https://example.org/private-token","profiles":[]}`)
	if err := SaveState(raw); err != nil {
		t.Fatal(err)
	}
	dir, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	disk, err := os.ReadFile(filepath.Join(dir, "profiles.dpapi"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(disk, []byte("private-token")) {
		t.Fatal("secret stored unencrypted")
	}
	got, err := LoadState()
	if err != nil || !bytes.Equal(got, raw) {
		t.Fatal("DPAPI roundtrip failed", err)
	}
	updated := []byte(`{"profiles":[],"revision":2}`)
	if err = SaveState(updated); err != nil {
		t.Fatal("replacing existing state", err)
	}
	got, err = LoadState()
	if err != nil || !bytes.Equal(got, updated) {
		t.Fatal("replacement failed", err)
	}
	disk, err = os.ReadFile(filepath.Join(dir, "profiles.dpapi"))
	if err != nil {
		t.Fatal(err)
	}
	disk[len(disk)-1] ^= 1
	if err = os.WriteFile(filepath.Join(dir, "profiles.dpapi"), disk, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadState(); err == nil {
		t.Fatal("tampered DPAPI accepted")
	}
}
