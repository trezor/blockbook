//go:build unittest

package btc

import (
	"math/big"
	"strconv"
	"testing"
)

func Test_mempoolSpaceMedianFeeProvider(t *testing.T) {
	m := &mempoolSpaceMedianFeeProvider{alternativeFeeProvider: &alternativeFeeProvider{}}
	testBlocks := []mempoolSpaceMedianFeeResult{
		{MedianFee: 5.123456},
		{MedianFee: 4.456789},
		{MedianFee: 3.789012},
		{MedianFee: 2.012345},
		{MedianFee: 1.345678},
	}

	success := m.mempoolSpaceMedianFeeProcessData(&testBlocks)
	if !success {
		t.Fatal("Expected data to be processed successfully")
	}

	tests := []struct {
		blocks int
		want   big.Int
	}{
		{0, *big.NewInt(5120)},
		{1, *big.NewInt(5120)},
		{2, *big.NewInt(4460)},
		{3, *big.NewInt(3790)},
		{4, *big.NewInt(2010)},
		{5, *big.NewInt(1350)},
		{6, *big.NewInt(1350)},
		{7, *big.NewInt(1350)},
		{10, *big.NewInt(1350)},
		{18, *big.NewInt(1350)},
		{19, *big.NewInt(1350)},
		{36, *big.NewInt(1350)},
		{37, *big.NewInt(1350)},
		{100, *big.NewInt(1350)},
		{101, *big.NewInt(1350)},
		{200, *big.NewInt(1350)},
		{201, *big.NewInt(1350)},
		{500, *big.NewInt(1350)},
		{501, *big.NewInt(1350)},
		{5000000, *big.NewInt(1350)},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.blocks), func(t *testing.T) {
			got, err := m.estimateFee(tt.blocks)
			if err != nil {
				t.Error("estimateFee returned error ", err)
			}
			if got.Cmp(&tt.want) != 0 {
				t.Errorf("estimateFee(%d) = %v, want %v", tt.blocks, got, tt.want)
			}
		})
	}
}
