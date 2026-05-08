package tron

import (
	"testing"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

func TestTronChainTipSyncIntentUsesTronPath(t *testing.T) {
	b := &TronRPC{
		EthereumRPC: &eth.EthereumRPC{},
		ChainConfig: &TronConfiguration{},
	}

	if got := b.BlockChainForSyncIntent(bchain.SyncIntentChainTip); got != b {
		t.Fatal("Tron chain-tip sync intent did not return the Tron chain")
	}
}
