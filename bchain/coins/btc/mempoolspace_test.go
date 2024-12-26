package btc

import (
	"math/big"
	"strconv"
	"testing"
)

func Test_mempoolSpaceFeeProvider(t *testing.T) {
	m := &mempoolSpaceFeeProvider{alternativeFeeProvider: &alternativeFeeProvider{}}
	m.mempoolSpaceFeeProcessData(&mempoolSpaceFeeResult{
		MinimumFee:  10,
		EconomyFee:  20,
		HourFee:     30,
		HalfHourFee: 40,
		FastestFee:  50,
	})

	tests := []struct {
		blocks int
		want   big.Int
	}{
		{0, *big.NewInt(50000)},
		{1, *big.NewInt(50000)},
		{2, *big.NewInt(40000)},
		{5, *big.NewInt(40000)},
		{6, *big.NewInt(40000)},
		{7, *big.NewInt(30000)},
		{10, *big.NewInt(30000)},
		{18, *big.NewInt(30000)},
		{19, *big.NewInt(30000)},
		{36, *big.NewInt(30000)},
		{37, *big.NewInt(20000)},
		{100, *big.NewInt(20000)},
		{101, *big.NewInt(20000)},
		{200, *big.NewInt(20000)},
		{201, *big.NewInt(20000)},
		{500, *big.NewInt(20000)},
		{501, *big.NewInt(10000)},
		{5000000, *big.NewInt(10000)},
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
