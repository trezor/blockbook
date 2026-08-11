//go:build unittest

package api

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

// setupEnsAliasDB indexes the two EthereumType test blocks, which between them store an
// ENS reverse label (cfAddressAliases) and a named ERC20 contract (cfContracts) - the two
// alias sources getAddressAliases must tell apart when the ENS gate is closed.
func setupEnsAliasDB(t *testing.T) (*db.RocksDB, *eth.EthereumParser) {
	t.Helper()
	parser := eth.NewEthereumParser(1, true)
	tmp, err := os.MkdirTemp("", "ens-aliases")
	require.NoError(t, err)
	database, err := db.NewRocksDB(tmp, 100000, -1, parser, nil, false)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, database.Close())
		require.NoError(t, os.RemoveAll(tmp))
	})
	is, err := database.LoadInternalState(&common.Config{CoinName: "eth-unittest"})
	require.NoError(t, err)
	database.SetInternalState(is)
	require.NoError(t, database.ConnectBlock(dbtestdata.GetTestEthereumTypeBlock1(parser)))
	require.NoError(t, database.ConnectBlock(dbtestdata.GetTestEthereumTypeBlock2(parser)))
	return database, parser
}

// TestGetAddressAliases_EnsReverseGate locks in the read-path half of the ENS reverse
// opt-out: closing the gate must drop only the ENS label, never the contract/token name
// that shares the addressAliases response.
func TestGetAddressAliases_EnsReverseGate(t *testing.T) {
	database, parser := setupEnsAliasDB(t)

	const contractAddr = "0x" + dbtestdata.EthAddrContract4a
	ensAddr := dbtestdata.EthAddr7bEIP55
	addresses := map[string]struct{}{contractAddr: {}, ensAddr: {}}

	contractAlias := AddressAlias{Type: "Contract", Alias: "Contract 74"}
	ensAlias := AddressAlias{Type: "ENS", Alias: parser.FormatAddressAlias(ensAddr, "address7b")}

	newWorker := func(useEnsReverseAliases bool) *Worker {
		return &Worker{
			db:                   database,
			chainParser:          parser,
			chainType:            bchain.ChainEthereumType,
			useAddressAliases:    true,
			useEnsReverseAliases: useEnsReverseAliases,
		}
	}

	t.Run("enabled serves both alias kinds", func(t *testing.T) {
		got := newWorker(true).getAddressAliases(addresses)
		require.Equal(t, AddressAliasesMap{contractAddr: contractAlias, ensAddr: ensAlias}, got)
	})

	t.Run("disabled keeps the contract name and drops the ENS label", func(t *testing.T) {
		got := newWorker(false).getAddressAliases(addresses)
		require.Equal(t, AddressAliasesMap{contractAddr: contractAlias}, got)
	})
}
