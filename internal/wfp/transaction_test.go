// SPDX-License-Identifier: GPL-3.0-only
package wfp

import (
	"errors"
	"reflect"
	"testing"
)

func TestDynamicPolicyCommitsOrRollsBackEveryFailure(t *testing.T) {
	denied := errors.New("denied")
	for _, fail := range []string{"", "open", "transaction", "install", "commit"} {
		t.Run(fail, func(t *testing.T) {
			var calls []string
			step := func(name string) func() error {
				return func() error {
					calls = append(calls, name)
					if name == fail {
						return denied
					}
					return nil
				}
			}
			err := installDynamic(operations{step("open"), step("transaction"), step("install"), step("commit"), step("abort"), step("close")})
			want := map[string][]string{"": {"open", "transaction", "install", "commit"}, "open": {"open"}, "transaction": {"open", "transaction", "close"}, "install": {"open", "transaction", "install", "abort", "close"}, "commit": {"open", "transaction", "install", "commit", "abort", "close"}}[fail]
			if !reflect.DeepEqual(calls, want) || (fail != "" && !errors.Is(err, denied)) || (fail == "" && err != nil) {
				t.Fatalf("calls=%v error=%v", calls, err)
			}
		})
	}
}

func TestDynamicPolicyCloseRunsAfterAbortFailure(t *testing.T) {
	closed := false
	denied := errors.New("installation failed")
	ok := func() error { return nil }
	err := installDynamic(operations{ok, ok, func() error { return denied }, ok, func() error { return errors.New("abort failed") }, func() error { closed = true; return nil }})
	if !closed || !errors.Is(err, denied) {
		t.Fatal("failed transaction leaked session or hid original failure")
	}
}
