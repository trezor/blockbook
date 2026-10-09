//go:build unittest

package api

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

// Address 7b holds ERC721 id 1 on contract cd and balances on the ERC20 contracts 0d and 4a.
// Below tokenBalances the holdings are not emitted, so the row is read without decoding them;
// the response must be identical apart from the ids themselves.
func TestGetEthereumTypeAddressBalances_HoldingsFollowDetailsLevel(t *testing.T) {
	chain, err := dbtestdata.NewFakeBlockChainEthereumType(eth.NewEthereumParser(1, true))
	require.NoError(t, err)
	w, _, parser := setupContractProbeWorker(t, chain)
	addr := addrDesc(t, parser, dbtestdata.EthAddr7b)
	nft := eth.EIP55Address(addrDesc(t, parser, dbtestdata.EthAddrContractCd))

	get := func(details AccountDetails, contract string) *ethereumTypeAddressData {
		_, d, err := w.getEthereumTypeAddressBalances(addr, details, &AddressFilter{Vout: AddressFilterVoutOff, Contract: contract}, "")
		require.NoError(t, err)
		return d
	}
	tokenOf := func(tokens []Token, contract string) Token {
		for _, tk := range tokens {
			if tk.Contract == contract {
				return tk
			}
		}
		t.Fatalf("token %s not found", contract)
		return Token{}
	}

	basic := get(AccountDetailsBasic, "")
	require.Empty(t, basic.tokens)
	require.Equal(t, 2, basic.totalResults, "totals are still read without holdings")

	tokens := get(AccountDetailsTokens, "")
	require.Len(t, tokens.tokens, 3)
	require.Empty(t, tokenOf(tokens.tokens, nft).Ids, "ids are not emitted at details=tokens")

	balances := get(AccountDetailsTokenBalances, "")
	require.Len(t, balances.tokens, 3)
	require.Equal(t, "1", tokenOf(balances.tokens, nft).Ids[0].String(), "ids are emitted at details=tokenBalances")
	for _, tk := range tokens.tokens {
		require.Equal(t, tk.Transfers, tokenOf(balances.tokens, tk.Contract).Transfers, "skipping holdings must not change the rest of the token")
	}

	// a contract filter decodes holdings for that contract only
	filtered := get(AccountDetailsTokenBalances, "0x"+dbtestdata.EthAddrContractCd)
	require.Len(t, filtered.tokens, 1)
	require.Equal(t, nft, filtered.tokens[0].Contract)
	require.Len(t, filtered.tokens[0].Ids, 1)

	erc20 := get(AccountDetailsTokenBalances, "0x"+dbtestdata.EthAddrContract0d)
	require.Len(t, erc20.tokens, 1)
	require.NotNil(t, erc20.tokens[0].BalanceSat, "ERC20 balance comes from the backend, not the index")
}
