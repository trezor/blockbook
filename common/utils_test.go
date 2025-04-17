//go:build unittest

package common

import (
	"math"
	"strconv"
	"testing"
)

func Test_RoundToSignificantDigits(t *testing.T) {
	type testCase struct {
		input  float64
		digits int
		want   float64
	}

	tests := []testCase{
		{input: 1234.5678, digits: 3, want: 1230},
		{input: 1234.5678, digits: 4, want: 1235},
		{input: 1234.5678, digits: 5, want: 1234.6},
		{input: 0.0123456, digits: 3, want: 0.0123},
		{input: 98765.4321, digits: 3, want: 98800},
		{input: 1.99999, digits: 3, want: 2.00},
		{input: 999.999, digits: 3, want: 1000},
		{input: 0.0006789, digits: 3, want: 0.000679},
		{input: 5.123456, digits: 3, want: 5.12},
		{input: 4.456789, digits: 3, want: 4.46},
		{input: 3.789012, digits: 3, want: 3.79},
		{input: 2.012345, digits: 3, want: 2.01},
	}

	for _, tt := range tests {
		t.Run(strconv.FormatFloat(tt.input, 'f', -1, 64), func(t *testing.T) {
			got := RoundToSignificantDigits(tt.input, tt.digits)

			// Use relative epsilon for float comparison
			epsilon := 1e-9
			if math.Abs(got-tt.want) > epsilon {
				t.Errorf("RoundToSignificantDigits(%v, %d) = %v, want %v", tt.input, tt.digits, got, tt.want)
			}
		})
	}
}
