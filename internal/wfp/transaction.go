// SPDX-License-Identifier: GPL-3.0-only
package wfp

import "fmt"

type operations struct{ open, begin, install, commit, abort, close func() error }
type operationError struct {
	step string
	err  error
}

func (e *operationError) Error() string { return fmt.Sprintf("%s: %v", e.step, e.err) }
func (e *operationError) Unwrap() error { return e.err }

// Every partially installed policy is aborted, then its dynamic session closed.
// Closing is attempted even when aborting fails. Ownership transfers only after
// a successful commit. The callbacks also let portable tests exercise failures.
func installDynamic(op operations) error {
	if err := op.open(); err != nil {
		return &operationError{"open", err}
	}
	accepted := false
	defer func() {
		if !accepted {
			_ = op.close()
		}
	}()
	if err := op.begin(); err != nil {
		return &operationError{"transaction", err}
	}
	committed := false
	defer func() {
		if !committed {
			_ = op.abort()
		}
	}()
	if err := op.install(); err != nil {
		return err
	}
	if err := op.commit(); err != nil {
		return &operationError{"commit", err}
	}
	committed = true
	accepted = true
	return nil
}
