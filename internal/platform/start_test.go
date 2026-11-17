// SPDX-License-Identifier: GPL-3.0-only
package platform

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCancelledStartCleansLateAdapterBeforeRetry(t *testing.T) {
	gate := make(chan struct{}, 1)
	unblock := make(chan struct{})
	entered := make(chan struct{})
	disposed := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		_, err := awaitStart(ctx, gate, func() (int, error) { close(entered); <-unblock; return 42, nil }, func(v int) {
			if v != 42 {
				t.Errorf("wrong adapter %d", v)
			}
			close(disposed)
		})
		done <- err
	}()
	<-entered
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel blocked on driver")
	}
	if _, err := awaitStart(context.Background(), gate, func() (int, error) { t.Fatal("overlapping start"); return 0, nil }, func(int) {}); err == nil {
		t.Fatal("retry raced old driver")
	}
	close(unblock)
	select {
	case <-disposed:
	case <-time.After(time.Second):
		t.Fatal("late adapter leaked")
	}
	for len(gate) > 0 {
		time.Sleep(time.Millisecond)
	}
	v, err := awaitStart(context.Background(), gate, func() (int, error) { return 7, nil }, func(int) { t.Error("accepted result disposed") })
	if err != nil || v != 7 {
		t.Fatalf("retry: %v %d", err, v)
	}
}

func TestStartupDeadlineReturnsBeforeBlockedDriver(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	gate := make(chan struct{}, 1)
	unblock := make(chan struct{})
	disposed := make(chan struct{})
	start := time.Now()
	_, err := awaitStart(ctx, gate, func() (int, error) { <-unblock; return 1, nil }, func(int) { close(disposed) })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("deadline did not bound startup")
	}
	close(unblock)
	select {
	case <-disposed:
	case <-time.After(time.Second):
		t.Fatal("late startup result leaked")
	}
}
