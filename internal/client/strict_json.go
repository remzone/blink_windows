// SPDX-License-Identifier: GPL-3.0-only
package client

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
)

// JSON duplicate fields have ambiguous security semantics across clients.
func uniqueJSON(raw string) bool {
	d := json.NewDecoder(strings.NewReader(raw))
	d.UseNumber()
	var scan func(int) error
	scan = func(depth int) error {
		if depth > 64 {
			return errors.New("JSON nesting limit")
		}
		token, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		if delim == '{' {
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				k, ok := key.(string)
				if !ok || seen[k] {
					return errors.New("duplicate JSON field")
				}
				seen[k] = true
				if err = scan(depth + 1); err != nil {
					return err
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim('}') {
				return errors.New("invalid JSON object")
			}
		} else if delim == '[' {
			for d.More() {
				if err := scan(depth + 1); err != nil {
					return err
				}
			}
			end, err := d.Token()
			if err != nil || end != json.Delim(']') {
				return errors.New("invalid JSON array")
			}
		} else {
			return errors.New("invalid JSON delimiter")
		}
		return nil
	}
	if scan(0) != nil {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}
