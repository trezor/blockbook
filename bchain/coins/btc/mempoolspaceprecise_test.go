//go:build unittest

package btc

import (
	"encoding/json"
	"math"
	"math/big"
	"strconv"
	"strings"
	"testing"
)

func newTestMempoolSpacePreciseFeeProvider(t *testing.T) *mempoolSpacePreciseFeeProvider {
	m, err := NewMempoolSpacePreciseFeeProviderFromParamsWithoutChain(mempoolSpacePreciseFeeParams{
		URL:           "https://mempool.space/api/v1/fees/precise",
		PeriodSeconds: 20,
	})
	if err != nil {
		t.Fatalf("NewMempoolSpacePreciseFeeProviderFromParamsWithoutChain returned error: %v", err)
	}
	return m
}

func Test_mempoolSpacePreciseFeeResultParsing(t *testing.T) {
	// Both the float /fees/precise and the integer /fees/recommended responses must parse
	tests := []struct {
		name string
		json string
		want mempoolSpacePreciseFeeResult
	}{
		{
			name: "precise",
			json: `{"fastestFee":2.605,"halfHourFee":1.603,"hourFee":0.843,"economyFee":0.2,"minimumFee":0.1}`,
			want: mempoolSpacePreciseFeeResult{FastestFee: 2.605, HalfHourFee: 1.603, HourFee: 0.843, EconomyFee: 0.2, MinimumFee: 0.1},
		},
		{
			name: "recommended",
			json: `{"fastestFee":41,"halfHourFee":39,"hourFee":36,"economyFee":36,"minimumFee":20}`,
			want: mempoolSpacePreciseFeeResult{FastestFee: 41, HalfHourFee: 39, HourFee: 36, EconomyFee: 36, MinimumFee: 20},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got mempoolSpacePreciseFeeResult
			if err := json.Unmarshal([]byte(tt.json), &got); err != nil {
				t.Fatalf("json.Unmarshal returned error: %v", err)
			}
			if got != tt.want {
				t.Errorf("json.Unmarshal = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func Test_mempoolSpacePreciseFeeProvider(t *testing.T) {
	m := newTestMempoolSpacePreciseFeeProvider(t)
	if !m.processData(&mempoolSpacePreciseFeeResult{
		FastestFee:  2.605,
		HalfHourFee: 1.603,
		HourFee:     0.843,
		EconomyFee:  0.2,
		MinimumFee:  0.1,
	}) {
		t.Fatal("expected data to be processed successfully")
	}

	// 2605 (not 2610) proves the conversion does not round to significant digits
	tests := []struct {
		blocks int
		want   big.Int
	}{
		{0, *big.NewInt(2605)},
		{1, *big.NewInt(2605)},
		{2, *big.NewInt(1603)},
		{3, *big.NewInt(1603)},
		{4, *big.NewInt(843)},
		{6, *big.NewInt(843)},
		{7, *big.NewInt(200)},
		{36, *big.NewInt(200)},
		{100, *big.NewInt(200)},
		{500, *big.NewInt(200)},
		{501, *big.NewInt(100)},
		{1008, *big.NewInt(100)},
		{5000000, *big.NewInt(100)},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.blocks), func(t *testing.T) {
			got, err := m.estimateFee(tt.blocks)
			if err != nil {
				t.Errorf("estimateFee returned error: %v", err)
			}
			if got.Cmp(&tt.want) != 0 {
				t.Errorf("estimateFee(%d) = %v, want %v", tt.blocks, got, tt.want)
			}
		})
	}
}

func Test_feePerVBToFeePerKB(t *testing.T) {
	tests := []struct {
		fee  float64
		want int
	}{
		{2.605, 2605},
		{0.1, 100},
		{0.001, 1},
		// not sane positive fees, must convert to 0
		{0.0004, 0},
		{0, 0},
		{-1, 0},
		{1e100, 0},
		{math.NaN(), 0},
		{math.Inf(1), 0},
	}
	for _, tt := range tests {
		if got := feePerVBToFeePerKB(tt.fee); got != tt.want {
			t.Errorf("feePerVBToFeePerKB(%v) = %d, want %d", tt.fee, got, tt.want)
		}
	}
}

func Test_mempoolSpacePreciseFeeProviderInvalidData(t *testing.T) {
	tests := []struct {
		name string
		data mempoolSpacePreciseFeeResult
	}{
		{
			name: "zero field",
			data: mempoolSpacePreciseFeeResult{FastestFee: 2.605, HalfHourFee: 1.603, HourFee: 0.843, EconomyFee: 0.2},
		},
		{
			name: "negative field",
			data: mempoolSpacePreciseFeeResult{FastestFee: 2.605, HalfHourFee: -1, HourFee: 0.843, EconomyFee: 0.2, MinimumFee: 0.1},
		},
		{
			name: "huge field overflowing int",
			data: mempoolSpacePreciseFeeResult{FastestFee: 1e100, HalfHourFee: 1.603, HourFee: 0.843, EconomyFee: 0.2, MinimumFee: 0.1},
		},
		{
			name: "field rounding to zero",
			data: mempoolSpacePreciseFeeResult{FastestFee: 2.605, HalfHourFee: 1.603, HourFee: 0.843, EconomyFee: 0.2, MinimumFee: 0.0004},
		},
		{
			name: "NaN field",
			data: mempoolSpacePreciseFeeResult{FastestFee: math.NaN(), HalfHourFee: 1.603, HourFee: 0.843, EconomyFee: 0.2, MinimumFee: 0.1},
		},
		{
			name: "Inf field",
			data: mempoolSpacePreciseFeeResult{FastestFee: math.Inf(1), HalfHourFee: 1.603, HourFee: 0.843, EconomyFee: 0.2, MinimumFee: 0.1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestMempoolSpacePreciseFeeProvider(t)
			if m.processData(&tt.data) {
				t.Error("expected processData to reject invalid data")
			}
			if _, err := m.estimateFee(1); err == nil {
				t.Error("expected estimateFee to return error when no data was processed")
			}

			// a rejected poll must keep the previously processed table
			if !m.processData(&mempoolSpacePreciseFeeResult{FastestFee: 2.605, HalfHourFee: 1.603, HourFee: 0.843, EconomyFee: 0.2, MinimumFee: 0.1}) {
				t.Fatal("expected valid data to be processed successfully")
			}
			if m.processData(&tt.data) {
				t.Error("expected processData to reject invalid data")
			}
			got, err := m.estimateFee(1)
			if err != nil {
				t.Errorf("estimateFee returned error: %v", err)
			}
			if want := big.NewInt(2605); got.Cmp(want) != 0 {
				t.Errorf("estimateFee(1) = %v, want %v", got, want)
			}
		})
	}
}

func Test_mempoolSpacePreciseFeeProviderMissingUrl(t *testing.T) {
	_, err := NewMempoolSpacePreciseFeeProviderFromParamsWithoutChain(mempoolSpacePreciseFeeParams{
		PeriodSeconds: 20,
	})
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	expectedSubstring := "Missing url"
	if !strings.Contains(err.Error(), expectedSubstring) {
		t.Errorf("expected error message to contain %q, got: %v", expectedSubstring, err)
	}
}

func Test_mempoolSpacePreciseFeeProviderInvalidPeriodSeconds(t *testing.T) {
	for _, periodSeconds := range []int{0, -5} {
		_, err := NewMempoolSpacePreciseFeeProviderFromParamsWithoutChain(mempoolSpacePreciseFeeParams{
			URL:           "https://mempool.space/api/v1/fees/precise",
			PeriodSeconds: periodSeconds,
		})
		if err == nil {
			t.Fatalf("expected error for periodSeconds=%d, got nil", periodSeconds)
		}
		expectedSubstring := "Missing periodSeconds"
		if !strings.Contains(err.Error(), expectedSubstring) {
			t.Errorf("expected error message to contain %q, got: %v", expectedSubstring, err)
		}
	}
}
