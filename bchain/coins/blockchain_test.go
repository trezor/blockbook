//go:build unittest

package coins

import (
	"testing"

	"github.com/trezor/blockbook/bchain"
)

// ensRebuilderIface mirrors db.EnsAliasRebuilder's method set. The blockbook.go
// -rebuildensaliases path reaches the chain through exactly this duck-typed
// assertion, so the metrics wrapper must expose the method (defining it here
// avoids importing db and the resulting import cycle).
type ensRebuilderIface interface {
	RebuildEnsAliases(fromBlock, toBlock uint32, chunkBlocks int, isInterrupted func() bool, store func([]bchain.AddressAliasRecord) error) error
}

type fakeEnsChain struct {
	bchain.BlockChain
	called bool
}

func (f *fakeEnsChain) RebuildEnsAliases(_, _ uint32, _ int, _ func() bool, _ func([]bchain.AddressAliasRecord) error) error {
	f.called = true
	return nil
}

// Test_blockChainWithMetrics_ForwardsRebuildEnsAliases guards the regression
// where the metrics wrapper hid the EthereumType RebuildEnsAliases method, so
// running -rebuildensaliases failed with "coin does not support ENS alias
// rebuild" even on Ethereum.
func Test_blockChainWithMetrics_ForwardsRebuildEnsAliases(t *testing.T) {
	inner := &fakeEnsChain{}
	var chain bchain.BlockChain = &blockChainWithMetrics{b: inner}

	rebuilder, ok := chain.(ensRebuilderIface)
	if !ok {
		t.Fatal("metrics wrapper does not expose RebuildEnsAliases; blockbook.go assertion would fail")
	}
	if err := rebuilder.RebuildEnsAliases(0, 1, 0, nil, nil); err != nil {
		t.Fatal(err)
	}
	if !inner.called {
		t.Fatal("RebuildEnsAliases was not forwarded to the underlying chain")
	}
}
