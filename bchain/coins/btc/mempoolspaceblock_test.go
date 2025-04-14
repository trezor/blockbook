//go:build unittest

package btc

import (
	"math/big"
	"strconv"
	"strings"
	"testing"
)

var testBlocks = []mempoolSpaceBlockFeeResult{
	{
		BlockSize:  1800000,
		BlockVSize: 997931,
		NTx:        2500,
		TotalFees:  6000000,
		MedianFee:  25.1,
		FeeRange:   []float64{1, 5, 10, 20, 30, 50, 300},
	},
	{
		BlockSize:  1750000,
		BlockVSize: 997930,
		NTx:        2200,
		TotalFees:  4500000,
		MedianFee:  7.31,
		FeeRange:   []float64{1, 2, 5, 10, 15, 20, 150},
	},
	{
		BlockSize:  1700000,
		BlockVSize: 997929,
		NTx:        2000,
		TotalFees:  3000000,
		MedianFee:  3.14,
		FeeRange:   []float64{1, 1.5, 2, 5, 7, 10, 100},
	},
	{
		BlockSize:  1650000,
		BlockVSize: 997928,
		NTx:        1800,
		TotalFees:  2000000,
		MedianFee:  1.34,
		FeeRange:   []float64{1, 1.2, 1.5, 3, 4, 5, 50},
	},
	{
		BlockSize:  1600000,
		BlockVSize: 997927,
		NTx:        1500,
		TotalFees:  1500000,
		MedianFee:  1.11,
		FeeRange:   []float64{1, 1.05, 1.1, 1.5, 1.8, 2, 20},
	},
}

var estimateFeeTestCasesMedian = []struct {
	blocks int
	want   big.Int
}{
	{0, *big.NewInt(25100)},
	{1, *big.NewInt(25100)},
	{2, *big.NewInt(7310)},
	{3, *big.NewInt(3140)},
	{4, *big.NewInt(1340)},
	{5, *big.NewInt(1110)},
	{6, *big.NewInt(1110)},
	{7, *big.NewInt(1110)},
	{10, *big.NewInt(1110)},
	{36, *big.NewInt(1110)},
	{100, *big.NewInt(1110)},
	{201, *big.NewInt(1110)},
	{501, *big.NewInt(1110)},
	{5000000, *big.NewInt(1110)},
}

var estimateFeeTestCasesFeeRangeIndex5FallbackSet = []struct {
	blocks int
	want   big.Int
}{
	{0, *big.NewInt(50000)},
	{1, *big.NewInt(50000)},
	{2, *big.NewInt(20000)},
	{3, *big.NewInt(10000)},
	{4, *big.NewInt(5000)},
	{5, *big.NewInt(2000)},
	{6, *big.NewInt(1000)},
	{7, *big.NewInt(1000)},
	{10, *big.NewInt(1000)},
	{36, *big.NewInt(1000)},
	{100, *big.NewInt(1000)},
	{201, *big.NewInt(1000)},
	{501, *big.NewInt(1000)},
	{5000000, *big.NewInt(1000)},
}

func runEstimateFeeTest(t *testing.T, testName string, m *mempoolSpaceBlockFeeProvider, expected []struct {
	blocks int
	want   big.Int
}) {
	success := m.processData(&testBlocks)
	if !success {
		t.Fatalf("[%s] Expected data to be processed successfully", testName)
	}

	for _, tt := range expected {
		t.Run(testName+"_"+strconv.Itoa(tt.blocks), func(t *testing.T) {
			got, err := m.estimateFee(tt.blocks)
			if err != nil {
				t.Errorf("[%s] estimateFee returned error: %v", testName, err)
			}
			if got.Cmp(&tt.want) != 0 {
				t.Errorf("[%s] estimateFee(%d) = %v, want %v", testName, tt.blocks, got, tt.want)
			}
		})
	}
}

func Test_mempoolSpaceBlockFeeProviderMedian(t *testing.T) {
	// Taking the median explicitly
	m, err :=
		NewMempoolSpaceBlockFeeProviderFromParamsWithoutChain(mempoolSpaceBlockFeeParams{
			URL:           "https://mempool.space/api/v1/fees/mempool-blocks",
			PeriodSeconds: 20,
			FeeRangeIndex: nil,
		})
	if err != nil {
		t.Fatalf("NewMempoolSpaceBlockFeeProviderFromParamsWithoutChain returned error: %v", err)
	}
	runEstimateFeeTest(t, "median", m, estimateFeeTestCasesMedian)
}

func Test_mempoolSpaceBlockFeeProviderSecondLargestIndex(t *testing.T) {
	// Taking the valid index
	index := 5
	m, err :=
		NewMempoolSpaceBlockFeeProviderFromParamsWithoutChain(mempoolSpaceBlockFeeParams{
			URL:              "https://mempool.space/api/v1/fees/mempool-blocks",
			PeriodSeconds:    20,
			FeeRangeIndex:    &index,
			FallbackFeePerKB: 1000,
		})
	if err != nil {
		t.Fatalf("NewMempoolSpaceBlockFeeProviderFromParamsWithoutChain returned error: %v", err)
	}
	runEstimateFeeTest(t, "feeRangeIndex_5", m, estimateFeeTestCasesFeeRangeIndex5FallbackSet)
}

func Test_mempoolSpaceBlockFeeProviderInvalidIndexTooHigh(t *testing.T) {
	index := 555
	_, err :=
		NewMempoolSpaceBlockFeeProviderFromParamsWithoutChain(mempoolSpaceBlockFeeParams{
			URL:           "https://mempool.space/api/v1/fees/mempool-blocks",
			PeriodSeconds: 20,
			FeeRangeIndex: &index,
		})

	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	expectedSubstring := "feeRangeIndex must be between 0 and 6"
	if !strings.Contains(err.Error(), expectedSubstring) {
		t.Errorf("expected error message to contain %q, got: %v", expectedSubstring, err)
	}
}

func Test_mempoolSpaceBlockFeeProviderMissingUrl(t *testing.T) {
	_, err :=
		NewMempoolSpaceBlockFeeProviderFromParamsWithoutChain(mempoolSpaceBlockFeeParams{
			PeriodSeconds: 20,
			FeeRangeIndex: nil,
		})

	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	expectedSubstring := "Missing url"
	if !strings.Contains(err.Error(), expectedSubstring) {
		t.Errorf("expected error message to contain %q, got: %v", expectedSubstring, err)
	}
}

func Test_mempoolSpaceBlockFeeProviderMissingPeriodSeconds(t *testing.T) {
	_, err :=
		NewMempoolSpaceBlockFeeProviderFromParamsWithoutChain(mempoolSpaceBlockFeeParams{
			URL:           "https://mempool.space/api/v1/fees/mempool-blocks",
			FeeRangeIndex: nil,
		})

	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	expectedSubstring := "Missing periodSeconds"
	if !strings.Contains(err.Error(), expectedSubstring) {
		t.Errorf("expected error message to contain %q, got: %v", expectedSubstring, err)
	}
}
