//go:build integration

package btc

import (
	"encoding/json"
	"testing"

	"github.com/trezor/blockbook/bchain"
)

const blockHeightLag = 100

func newTestBitcoinRPC(t *testing.T) *BitcoinRPC {
	t.Helper()

	cfg := bchain.LoadBlockchainCfg(t, "bitcoin")
	config := Configuration{
		RPCURL:     cfg.RpcUrl,
		RPCUser:    cfg.RpcUser,
		RPCPass:    cfg.RpcPass,
		RPCTimeout: cfg.RpcTimeout,
		Parse:      cfg.Parse,
	}
	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	chain, err := NewBitcoinRPC(raw, nil)
	if err != nil {
		t.Fatalf("new bitcoin rpc: %v", err)
	}
	rpcClient, ok := chain.(*BitcoinRPC)
	if !ok {
		t.Fatalf("unexpected rpc client type %T", chain)
	}
	if err := rpcClient.Initialize(); err != nil {
		t.Skipf("skipping: cannot connect to RPC at %s: %v", cfg.RpcUrl, err)
		return nil
	}
	return rpcClient
}

func assertBlockBasics(t *testing.T, block *bchain.Block, hash string, height uint32) {
	t.Helper()
	if block.Hash != hash {
		t.Fatalf("hash mismatch: got %s want %s", block.Hash, hash)
	}
	if block.Height != height {
		t.Fatalf("height mismatch: got %d want %d", block.Height, height)
	}
	if block.Time <= 0 {
		t.Fatalf("expected block time > 0, got %d", block.Time)
	}
}

// TestBitcoinRPCGetBlockIntegration validates GetBlock by hash/height and checks
// previous hash availability for fork detection.
func TestBitcoinRPCGetBlockIntegration(t *testing.T) {
	rpcClient := newTestBitcoinRPC(t)
	if rpcClient == nil {
		return
	}

	best, err := rpcClient.GetBestBlockHeight()
	if err != nil {
		t.Fatalf("GetBestBlockHeight: %v", err)
	}
	if best <= blockHeightLag {
		t.Skipf("best height %d too low for lag %d", best, blockHeightLag)
		return
	}
	height := best - blockHeightLag
	if height == 0 {
		t.Skip("block height is zero, cannot validate previous hash")
		return
	}

	hash, err := rpcClient.GetBlockHash(height)
	if err != nil {
		t.Fatalf("GetBlockHash height %d: %v", height, err)
	}
	prevHash, err := rpcClient.GetBlockHash(height - 1)
	if err != nil {
		t.Fatalf("GetBlockHash height %d: %v", height-1, err)
	}

	blockByHash, err := rpcClient.GetBlock(hash, 0)
	if err != nil {
		t.Fatalf("GetBlock by hash: %v", err)
	}
	assertBlockBasics(t, blockByHash, hash, height)
	if blockByHash.Confirmations <= 0 {
		t.Fatalf("expected confirmations > 0, got %d", blockByHash.Confirmations)
	}
	if blockByHash.Prev != prevHash {
		t.Fatalf("previous hash mismatch: got %s want %s", blockByHash.Prev, prevHash)
	}

	blockByHeight, err := rpcClient.GetBlock("", height)
	if err != nil {
		t.Fatalf("GetBlock by height: %v", err)
	}
	assertBlockBasics(t, blockByHeight, hash, height)
	if blockByHeight.Prev != prevHash {
		t.Fatalf("previous hash mismatch by height: got %s want %s", blockByHeight.Prev, prevHash)
	}
	if len(blockByHeight.Txs) != len(blockByHash.Txs) {
		t.Fatalf("tx count mismatch: by hash %d vs by height %d", len(blockByHash.Txs), len(blockByHeight.Txs))
	}
}
