//go:build unittest

package api

import (
	"math/big"
	"testing"

	"github.com/trezor/blockbook/bchain"
)

func TestHasEthereumTokenNonzeroHoldings(t *testing.T) {
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
			name:  "erc20 zero balance",
			token: &Token{BalanceSat: (*Amount)(big.NewInt(0))},
			want:  false,
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
			name:  "erc1155 zero value only",
			token: &Token{MultiTokenValues: []MultiTokenValue{{Value: (*Amount)(big.NewInt(0))}}},
			want:  false,
		},
		{
			name:  "erc1155 nonzero value",
			token: &Token{MultiTokenValues: []MultiTokenValue{{Value: (*Amount)(big.NewInt(7))}}},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasEthereumTokenNonzeroHoldings(tt.token)
			if got != tt.want {
				t.Fatalf("hasEthereumTokenNonzeroHoldings() = %v, want %v", got, tt.want)
			}
		})
	}
}

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
			name:  "erc20 zero balance",
			token: &Token{BalanceSat: (*Amount)(big.NewInt(0))},
			want:  true,
		},
		{
			name:  "erc721 ids",
			token: &Token{Ids: []Amount{Amount(*big.NewInt(0))}},
			want:  true,
		},
		{
			name:  "erc1155 zero value field",
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

func TestShouldIncludeEthereumAddressToken(t *testing.T) {
	tests := []struct {
		name    string
		token   *Token
		details AccountDetails
		mode    TokensToReturn
		want    bool
	}{
		{
			name:    "nonzero keeps metadata token for tokens detail",
			token:   &Token{},
			details: AccountDetailsTokens,
			mode:    TokensToReturnNonzeroBalance,
			want:    true,
		},
		{
			name:    "nonzero drops metadata token for token balances",
			token:   &Token{},
			details: AccountDetailsTokenBalances,
			mode:    TokensToReturnNonzeroBalance,
			want:    false,
		},
		{
			name:    "nonzero drops zero erc20 balance",
			token:   &Token{BalanceSat: (*Amount)(big.NewInt(0))},
			details: AccountDetailsTokenBalances,
			mode:    TokensToReturnNonzeroBalance,
			want:    false,
		},
		{
			name:    "nonzero keeps nonzero erc20 balance",
			token:   &Token{BalanceSat: (*Amount)(big.NewInt(5))},
			details: AccountDetailsTokenBalances,
			mode:    TokensToReturnNonzeroBalance,
			want:    true,
		},
		{
			name:    "nonzero keeps erc721 ids",
			token:   &Token{Ids: []Amount{Amount(*big.NewInt(1))}},
			details: AccountDetailsTokenBalances,
			mode:    TokensToReturnNonzeroBalance,
			want:    true,
		},
		{
			name: "nonzero keeps erc1155 nonzero value",
			token: &Token{MultiTokenValues: []MultiTokenValue{
				{Value: (*Amount)(big.NewInt(9))},
			}},
			details: AccountDetailsTokenBalances,
			mode:    TokensToReturnNonzeroBalance,
			want:    true,
		},
		{
			name:    "used keeps zero erc20 balance",
			token:   &Token{Standard: bchain.ERC20TokenStandard, BalanceSat: (*Amount)(big.NewInt(0))},
			details: AccountDetailsTokenBalances,
			mode:    TokensToReturnUsed,
			want:    true,
		},
		{
			name:    "derived keeps zero erc20 balance",
			token:   &Token{Standard: bchain.ERC20TokenStandard, BalanceSat: (*Amount)(big.NewInt(0))},
			details: AccountDetailsTokenBalances,
			mode:    TokensToReturnDerived,
			want:    true,
		},
		{
			name:    "used keeps metadata-only erc20 token",
			token:   &Token{Standard: bchain.ERC20TokenStandard},
			details: AccountDetailsTokenBalances,
			mode:    TokensToReturnUsed,
			want:    true,
		},
		{
			name:    "used drops empty erc721 contract",
			token:   &Token{Standard: bchain.ERC771TokenStandard},
			details: AccountDetailsTokenBalances,
			mode:    TokensToReturnUsed,
			want:    false,
		},
		{
			name:    "derived drops empty erc1155 contract",
			token:   &Token{Standard: bchain.ERC1155TokenStandard},
			details: AccountDetailsTokenBalances,
			mode:    TokensToReturnDerived,
			want:    false,
		},
		{
			name: "derived keeps erc1155 holdings field",
			token: &Token{
				Standard:         bchain.ERC1155TokenStandard,
				MultiTokenValues: []MultiTokenValue{{Value: (*Amount)(big.NewInt(0))}},
			},
			details: AccountDetailsTokenBalances,
			mode:    TokensToReturnDerived,
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldIncludeEthereumAddressToken(tt.token, tt.details, tt.mode)
			if got != tt.want {
				t.Fatalf("shouldIncludeEthereumAddressToken() = %v, want %v", got, tt.want)
			}
		})
	}
}
