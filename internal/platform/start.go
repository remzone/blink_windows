// SPDX-License-Identifier: GPL-3.0-only
package platform

import (
	"context"
	"errors"
)

// Gate remains held until a cancelled driver/core operation actually exits and its
// late result is cleaned up. A retry must never race an old adapter's cleanup.
var startGate = make(chan struct{}, 1)

func awaitStart[T any](ctx context.Context, gate chan struct{}, start func() (T, error), dispose func(T)) (T, error) {
	var zero T
	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	default:
	}
	select {
	case gate <- struct{}{}:
	default:
		return zero, errors.New("Предыдущая операция драйвера ещё завершается. Повторите позже или перезапустите Blink")
	}
	type result struct {
		value T
		err   error
	}
	results := make(chan result)
	accepted := make(chan bool, 1)
	go func() {
		defer func() { <-gate }()
		value, err := start()
		select {
		case results <- result{value, err}:
			if !<-accepted {
				dispose(value)
			}
		case <-ctx.Done():
			dispose(value)
		}
	}()
	select {
	case r := <-results:
		if ctx.Err() != nil {
			accepted <- false
			return zero, ctx.Err()
		}
		accepted <- true
		return r.value, r.err
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}
