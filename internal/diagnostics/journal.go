// SPDX-License-Identifier: GPL-3.0-only
// Package diagnostics records explicitly selected technical fields, never raw
// errors, commands, profiles, URLs or provider payloads.
package diagnostics

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const maxSize = 1024 * 1024

type Journal struct {
	mu    sync.Mutex
	path  string
	file  *os.File
	size  int64
	limit int64
	err   error
}

var active struct {
	sync.RWMutex
	j *Journal
}

func Open(path string) (*Journal, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	s, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &Journal{path: path, file: f, size: s.Size(), limit: maxSize}, nil
}

func Set(j *Journal) { active.Lock(); active.j = j; active.Unlock() }

// Event accepts only fields selected by the caller. Never pass err.Error(),
// State, Profile, PowerShell scripts or unfiltered native output here.
func Event(event string, fields any) {
	active.RLock()
	defer active.RUnlock()
	if active.j != nil {
		active.j.Write(event, fields)
	}
}

func Status() (string, error) {
	active.RLock()
	defer active.RUnlock()
	if active.j == nil {
		return "", errors.New("journal is not initialized")
	}
	active.j.mu.Lock()
	defer active.j.mu.Unlock()
	return active.j.path, active.j.err
}

func (j *Journal) Write(event string, fields any) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.err != nil {
		return
	}
	line, err := json.Marshal(struct {
		Time   string `json:"time"`
		Event  string `json:"event"`
		Fields any    `json:"fields,omitempty"`
	}{time.Now().UTC().Format(time.RFC3339Nano), event, fields})
	if err == nil && len(line) > 16*1024 {
		err = errors.New("journal record exceeds limit")
	}
	if err != nil {
		j.err = err
		return
	}
	line = append(line, '\n')
	if j.size+int64(len(line)) > j.limit {
		if err = j.file.Close(); err == nil {
			if e := os.Remove(j.path + ".1"); e != nil && !errors.Is(e, os.ErrNotExist) {
				err = e
			}
		}
		if err == nil {
			err = os.Rename(j.path, j.path+".1")
		}
		if err == nil {
			j.file, err = os.OpenFile(j.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
			j.size = 0
		}
	}
	if err == nil {
		var n int
		n, err = j.file.Write(line)
		j.size += int64(n)
	}
	// A failed connection or a forced process exit must leave usable evidence.
	if err == nil {
		err = j.file.Sync()
	}
	j.err = err
}

func (j *Journal) Close() error { j.mu.Lock(); defer j.mu.Unlock(); return j.file.Close() }
