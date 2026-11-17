// SPDX-License-Identifier: GPL-3.0-only
package platform

import "testing"

func TestChooseDefaultInterface(t *testing.T) {
	tests := []struct {
		name   string
		routes []routeCandidate
		want   uint32
	}{
		{"offline", nil, 0},
		{"ignore Blink and inactive", []routeCandidate{{1, "bLiNk", true, 0, 0}, {2, "Wi-Fi", false, 0, 0}, {0, "invalid", true, 0, 0}, {9, "Ethernet", true, 20, 25}}, 9},
		{"combined metric", []routeCandidate{{3, "Wi-Fi", true, 1, 100}, {4, "Ethernet", true, 20, 10}}, 4},
		{"stable tie", []routeCandidate{{8, "Wi-Fi", true, 10, 10}, {5, "Ethernet", true, 15, 5}}, 5},
		{"no metric overflow", []routeCandidate{{2, "Wi-Fi", true, ^uint32(0), 10}, {6, "Ethernet", true, 1, 10}}, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chooseDefaultInterface(tt.routes); got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}
