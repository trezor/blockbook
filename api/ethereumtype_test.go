//go:build unittest

package api

import "testing"

func Test_healDue(t *testing.T) {
	tests := []struct {
		name     string
		failures uint8
		pass     uint64
		want     bool
	}{
		// a restart resets the pass counter, so the whole queue gets one immediate attempt
		{name: "first pass attempts a fresh block", failures: 0, pass: 0, want: true},
		{name: "first pass attempts a long failing block", failures: 200, pass: 0, want: true},
		{name: "no failures is attempted every pass", failures: 0, pass: 7, want: true},
		{name: "one failure skips the next pass", failures: 1, pass: 1, want: false},
		{name: "one failure is attempted every second pass", failures: 1, pass: 2, want: true},
		{name: "three failures wait eight passes", failures: 3, pass: 4, want: false},
		{name: "three failures are attempted every eighth pass", failures: 3, pass: 8, want: true},
		// the shift saturates, so the slowest schedule stays at 64 passes
		{name: "the cap is attempted every 64th pass", failures: maxHealBackoffShift, pass: 64, want: true},
		{name: "above the cap keeps the capped schedule", failures: 200, pass: 64, want: true},
		{name: "above the cap is not attempted in between", failures: 200, pass: 65, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := healDue(tt.failures, tt.pass); got != tt.want {
				t.Errorf("healDue(%d, %d) = %v, want %v", tt.failures, tt.pass, got, tt.want)
			}
		})
	}
}
