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
		{0, *big.NewInt(5123)},
		{1, *big.NewInt(5123)},
		{2, *big.NewInt(4456)},
		{3, *big.NewInt(3789)},
		{4, *big.NewInt(2012)},
		{5, *big.NewInt(1345)},
		{6, *big.NewInt(1345)},
		{7, *big.NewInt(1345)},
		{10, *big.NewInt(1345)},
		{18, *big.NewInt(1345)},
		{19, *big.NewInt(1345)},
		{36, *big.NewInt(1345)},
		{37, *big.NewInt(1345)},
		{100, *big.NewInt(1345)},
		{101, *big.NewInt(1345)},
		{200, *big.NewInt(1345)},
		{201, *big.NewInt(1345)},
		{500, *big.NewInt(1345)},
		{501, *big.NewInt(1345)},
		{5000000, *big.NewInt(1345)},
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
