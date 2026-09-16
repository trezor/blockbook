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

	idsByContract := func(tokens []Token) map[string]int {
		rv := map[string]int{}
		for _, tk := range tokens {
			rv[tk.Contract] = len(tk.Ids)
		}
		return rv
	}
	transfersByContract := func(tokens []Token) map[string]int {
		rv := map[string]int{}
		for _, tk := range tokens {
			rv[tk.Contract] = tk.Transfers
		}
		return rv
	}

	_, basic, err := w.getEthereumTypeAddressBalances(addr, AccountDetailsBasic, &AddressFilter{Vout: AddressFilterVoutOff}, "")
	require.NoError(t, err)
	require.Empty(t, basic.tokens)
	require.Equal(t, 2, basic.totalResults, "totals are still read without holdings")

	_, tokens, err := w.getEthereumTypeAddressBalances(addr, AccountDetailsTokens, &AddressFilter{Vout: AddressFilterVoutOff}, "")
	require.NoError(t, err)
	require.Len(t, tokens.tokens, 3)
	require.Equal(t, 0, idsByContract(tokens.tokens)[nft], "ids are not emitted at details=tokens")

	_, balances, err := w.getEthereumTypeAddressBalances(addr, AccountDetailsTokenBalances, &AddressFilter{Vout: AddressFilterVoutOff}, "")
	require.NoError(t, err)
	require.Len(t, balances.tokens, 3)
	require.Equal(t, 1, idsByContract(balances.tokens)[nft], "ids are emitted at details=tokenBalances")
	require.Equal(t, "1", balances.tokens[idxOf(balances.tokens, nft)].Ids[0].String())
	require.Equal(t, transfersByContract(tokens.tokens), transfersByContract(balances.tokens), "skipping holdings must not change the rest of the token")

	_, history, err := w.getEthereumTypeAddressBalances(addr, AccountDetailsTxidHistory, &AddressFilter{Vout: AddressFilterVoutOff}, "")
	require.NoError(t, err)
	require.Equal(t, 1, idsByContract(history.tokens)[nft], "the REST default level still carries ids")

	// a contract filter decodes holdings for that contract only
	_, filtered, err := w.getEthereumTypeAddressBalances(addr, AccountDetailsTokenBalances, &AddressFilter{Vout: AddressFilterVoutOff, Contract: "0x" + dbtestdata.EthAddrContractCd}, "")
	require.NoError(t, err)
	require.Len(t, filtered.tokens, 1)
	require.Equal(t, nft, filtered.tokens[0].Contract)
	require.Len(t, filtered.tokens[0].Ids, 1)

	_, erc20, err := w.getEthereumTypeAddressBalances(addr, AccountDetailsTokenBalances, &AddressFilter{Vout: AddressFilterVoutOff, Contract: "0x" + dbtestdata.EthAddrContract0d}, "")
	require.NoError(t, err)
	require.Len(t, erc20.tokens, 1)
	require.NotNil(t, erc20.tokens[0].BalanceSat)
	require.Empty(t, erc20.tokens[0].Ids)

	// an invalid filter still fails before anything is read
	_, _, err = w.getEthereumTypeAddressBalances(addr, AccountDetailsTokenBalances, &AddressFilter{Vout: AddressFilterVoutOff, Contract: "not-an-address"}, "")
	require.Error(t, err)
}

func idxOf(tokens []Token, contract string) int {
	for i, tk := range tokens {
		if tk.Contract == contract {
			return i
		}
	}
	return -1
}
