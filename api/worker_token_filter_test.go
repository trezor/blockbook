//go:build unittest

package api

import (
	"math/big"
	"testing"
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
