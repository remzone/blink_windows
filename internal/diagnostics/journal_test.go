// SPDX-License-Identifier: GPL-3.0-only
package diagnostics

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestJournalPersistsConcurrentEventsAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "blink.log")
	j, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); j.Write("test.event", map[string]int{"index": i}) }(i)
	}
	wg.Wait()
	// Read before Close: a failed connection must already be on disk.
	check := func(want int) {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		s := bufio.NewScanner(f)
		count := 0
		for s.Scan() {
			var v map[string]any
			if json.Unmarshal(s.Bytes(), &v) != nil || v["event"] != "test.event" || v["time"] == nil {
				t.Fatalf("invalid record %s", s.Bytes())
			}
			count++
		}
		if s.Err() != nil || count != want {
			t.Fatalf("count=%d error=%v", count, s.Err())
		}
	}
	check(12)
	j.Close()
	j, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	j.Write("test.event", nil)
	check(13)
}

func TestJournalRotationIsBoundedAndFailureIsVisible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "blink.log")
	j, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	j.limit = 300
	for i := 0; i < 50; i++ {
		j.Write("test.event", map[string]int{"index": i})
	}
	for _, p := range []string{path, path + ".1"} {
		s, err := os.Stat(p)
		if err != nil || s.Size() > 300 {
			t.Fatalf("rotation: %v %v", s, err)
		}
	}
	Set(j)
	defer Set(nil)
	j.file.Close()
	j.Write("test.event", nil)
	got, err := Status()
	if got != path || err == nil {
		t.Fatal("write failure was hidden")
	}
}
