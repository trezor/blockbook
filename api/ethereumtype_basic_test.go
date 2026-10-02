//go:build unittest

package api

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/db"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

// details=basic reads only the header of the address-contracts record; every counter it exposes
// must match the full decode, while the contract array stays untouched.
func TestGetEthereumTypeAddressBalances_BasicMatchesFullDecode(t *testing.T) {
	w, _, parser := setupContractProbeWorker(t, newContractProbeChain(t))
	for _, address := range []string{dbtestdata.EthAddr4b, dbtestdata.EthAddr7b} {
		ad := addrDesc(t, parser, address)
		baBasic, dBasic, err := w.getEthereumTypeAddressBalances(ad, AccountDetailsBasic, &AddressFilter{Vout: AddressFilterVoutOff}, "")
		require.NoError(t, err)
		baFull, dFull, err := w.getEthereumTypeAddressBalances(ad, AccountDetailsTokens, &AddressFilter{Vout: AddressFilterVoutOff}, "")
		require.NoError(t, err)

		require.Equal(t, baFull.Txs, baBasic.Txs, address)
		require.Equal(t, baFull.BalanceSat, baBasic.BalanceSat, address)
		require.Equal(t, dFull.nonContractTxs, dBasic.nonContractTxs, address)
		require.Equal(t, dFull.internalTxs, dBasic.internalTxs, address)
		require.Equal(t, dFull.totalResults, dBasic.totalResults, address)
		require.Equal(t, dFull.nonce, dBasic.nonce, address)
		require.Empty(t, dBasic.tokens, address)
		require.NotEmpty(t, dFull.tokens, address)
	}
}

// A fresh address has no record on either path; basic must keep the "never sent" branch intact.
func TestGetEthereumTypeAddressBalances_BasicFreshAddress(t *testing.T) {
	w, _, parser := setupContractProbeWorker(t, newContractProbeChain(t))
	fresh := addrDesc(t, parser, "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee")
	ba, d, err := w.getEthereumTypeAddressBalances(fresh, AccountDetailsBasic, &AddressFilter{Vout: AddressFilterVoutOff}, "")
	require.NoError(t, err)
	require.NotNil(t, ba)
	require.Equal(t, uint32(0), ba.Txs)
	require.Equal(t, -1, d.totalResults)
	require.Equal(t, 0, d.nonContractTxs)
	require.Equal(t, 0, d.internalTxs)
	require.Equal(t, "0", d.nonce)
}

// Per-contract paging and a contract filter can reach the contract array even at basic, so both
// must fall back to the full decode.
func TestGetEthereumTypeAddressBalances_BasicFallsBackForContractFilters(t *testing.T) {
	w, database, parser := setupContractProbeWorker(t, newContractProbeChain(t))
	ad := addrDesc(t, parser, dbtestdata.EthAddr4b)
	full, err := database.GetAddrDescContracts(ad)
	require.NoError(t, err)
	require.NotEmpty(t, full.Contracts)

	_, d, err := w.getEthereumTypeAddressBalances(ad, AccountDetailsBasic, &AddressFilter{Vout: db.ContractIndexOffset}, "")
	require.NoError(t, err)
	require.Equal(t, int(full.Contracts[0].Txs), d.totalResults)

	_, d, err = w.getEthereumTypeAddressBalances(ad, AccountDetailsBasic, &AddressFilter{Vout: AddressFilterVoutOff, Contract: dbtestdata.EthAddrContract4a}, "")
	require.NoError(t, err)
	require.Equal(t, int(full.TotalTxs), d.totalResults)
	require.Empty(t, d.tokens)
}
