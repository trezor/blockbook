//go:build unittest

package api

import (
	"math/big"
	"testing"

	"github.com/trezor/blockbook/db"
)

func TestHasEthereumTokenHoldingsField(t *testing.T) {
	tests := []struct {
		name  string
		token *Token
		want  bool
	}{
		{
			name:  "nil token",
			token: nil,
			want:  false,
		},
		{
			name:  "metadata only",
			token: &Token{},
			want:  false,
		},
		{
			name:  "erc20 zero balance still has field",
			token: &Token{BalanceSat: (*Amount)(big.NewInt(0))},
			want:  true,
		},
		{
			name:  "erc20 nonzero balance",
			token: &Token{BalanceSat: (*Amount)(big.NewInt(42))},
			want:  true,
		},
		{
			name:  "erc721 ids",
			token: &Token{Ids: []Amount{Amount(*big.NewInt(0))}},
			want:  true,
		},
		{
			name:  "erc1155 multi token values",
			token: &Token{MultiTokenValues: []MultiTokenValue{{Value: (*Amount)(big.NewInt(0))}}},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasEthereumTokenHoldingsField(tt.token)
			if got != tt.want {
				t.Fatalf("hasEthereumTokenHoldingsField() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMultiTokenHoldingsSkipsZeroValues(t *testing.T) {
	values := db.MultiTokenValues{
		{Id: *big.NewInt(1), Value: *big.NewInt(0)},
		{Id: *big.NewInt(2), Value: *big.NewInt(5)},
	}
	got := multiTokenHoldings(values)
	if len(got) != 1 || (*big.Int)(got[0].Id).Cmp(big.NewInt(2)) != 0 || (*big.Int)(got[0].Value).Cmp(big.NewInt(5)) != 0 {
		t.Fatalf("multiTokenHoldings() = %+v, want [{2 5}]", got)
	}
	// phantom-only holdings drop the field, so tokenBalances treats the collection as emptied
	if got := multiTokenHoldings(db.MultiTokenValues{{Id: *big.NewInt(1)}}); got != nil {
		t.Fatalf("multiTokenHoldings(phantoms) = %+v, want nil", got)
	}
}
