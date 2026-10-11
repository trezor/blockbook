//go:build unittest

package api

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

func setFilteredScanBudget(t *testing.T, budget int) {
	t.Helper()
	prev := maxFilteredAddressScanNonMatching
	maxFilteredAddressScanNonMatching = budget
	t.Cleanup(func() { maxFilteredAddressScanNonMatching = prev })
}

// TestGetAddressSparseFilterStopsAtScanBudget: a filter that matches nothing must not walk the
// whole address index, and the caller must get a public error instead of an empty history.
func TestGetAddressSparseFilterStopsAtScanBudget(t *testing.T) {
	// every indexed tx only pays to Addr1, so filter=inputs never matches
	w, _, _ := buildPagingWorker(t, 10, 0)
	setFilteredScanBudget(t, 5)

	_, err := w.GetAddress(dbtestdata.Addr1, 1, 25, AccountDetailsTxidHistory, &AddressFilter{Vout: AddressFilterVoutInputs}, "")
	require.Error(t, err)
	apiErr, ok := err.(*APIError)
	require.True(t, ok, "error must stay a plain *APIError so the servers return it to the client, got %T", err)
	require.True(t, apiErr.Public)
	require.Equal(t, errFilteredScanLimit, apiErr)
}

// TestGetAddressSparseFilterWithinScanBudget: a sparse walk that fits the budget still succeeds.
func TestGetAddressSparseFilterWithinScanBudget(t *testing.T) {
	w, _, _ := buildPagingWorker(t, 10, 0)
	setFilteredScanBudget(t, 10)

	addr, err := w.GetAddress(dbtestdata.Addr1, 1, 25, AccountDetailsTxidHistory, &AddressFilter{Vout: AddressFilterVoutInputs}, "")
	require.NoError(t, err)
	require.Empty(t, addr.Txids)
}

// TestGetAddressDenseFilterIgnoresScanBudget: matching entries do not count against the budget,
// so a filter that matches often pages as deep as an unfiltered walk.
func TestGetAddressDenseFilterIgnoresScanBudget(t *testing.T) {
	w, _, confirmed := buildPagingWorker(t, 10, 0)
	setFilteredScanBudget(t, 1)

	addr, err := w.GetAddress(dbtestdata.Addr1, 1, 25, AccountDetailsTxidHistory, &AddressFilter{Vout: AddressFilterVoutOutputs}, "")
	require.NoError(t, err)
	require.Equal(t, confirmed, addr.Txids)
}
