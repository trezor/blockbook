//go:build unittest

package api

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

// A pending contract creation has no "to", so the calldata parser emits a transfer whose contract is
// "0x"; its metadata lookup fails. The transfer must still be returned with its addresses and value
// instead of a zero-valued placeholder (trezor/blockbook#1715).
func TestGetEthereumTokensTransfers_KeepsTransferWhenContractInfoFails(t *testing.T) {
	w, _, parser := setupContractProbeWorker(t, newContractProbeChain(t))
	var one, two big.Int
	one.SetInt64(1)
	two.SetInt64(2)
	transfers := bchain.TokenTransfers{
		{Standard: bchain.FungibleToken, Contract: "0x", From: "0x" + dbtestdata.EthAddr3e, To: "0x" + dbtestdata.EthAddr55, Value: one},
		{Standard: bchain.FungibleToken, Contract: "0x" + dbtestdata.EthAddrContract4a, From: "0x" + dbtestdata.EthAddr55, To: "0x" + dbtestdata.EthAddr3e, Value: two},
	}
	addresses := map[string]struct{}{}

	tokens := w.getEthereumTokensTransfers(transfers, addresses)

	require.Len(t, tokens, 2)
	byContract := map[string]TokenTransfer{}
	for _, tt := range tokens {
		byContract[tt.Contract] = tt
	}
	broken, ok := byContract["0x"]
	require.True(t, ok, "the transfer with unreadable contract metadata must not be dropped")
	require.Equal(t, bchain.ERC20TokenStandard, broken.Standard)
	require.Equal(t, "0x"+dbtestdata.EthAddr3e, broken.From)
	require.Equal(t, "0x"+dbtestdata.EthAddr55, broken.To)
	require.Equal(t, "1", broken.Value.String())
	require.Equal(t, parser.AmountDecimals(), broken.Decimals)
	require.Empty(t, broken.Name)
	require.Empty(t, broken.Symbol)

	known := byContract["0x"+dbtestdata.EthAddrContract4a]
	require.Equal(t, "Contract 74", known.Name)
	require.Equal(t, "S74", known.Symbol)
	require.Equal(t, 12, known.Decimals)
	require.Equal(t, "2", known.Value.String())

	require.Contains(t, addresses, "0x"+dbtestdata.EthAddr3e)
	require.Contains(t, addresses, "0x"+dbtestdata.EthAddr55)
}
