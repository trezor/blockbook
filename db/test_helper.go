//go:build integration

package db

import (
	"github.com/trezor/blockbook/bchain"
)

func SetBlockChain(w *SyncWorker, chain bchain.BlockChain) {
	w.chain = chain
}

func ConnectBlocks(w *SyncWorker, onNewBlock bchain.OnNewBlockFunc, initialSync bool) error {
	return w.connectBlocks(onNewBlock, initialSync)
}

func HandleFork(w *SyncWorker, localBestHeight uint32, localBestHash string, onNewBlock bchain.OnNewBlockFunc, initialSync bool) error {
	return w.handleFork(localBestHeight, localBestHash, onNewBlock, initialSync)
}

// ConnectBlocksParallel keeps legacy integration tests compiling against the new API.
func (w *SyncWorker) ConnectBlocksParallel(lower, higher uint32) error {
	workers := w.syncWorkers
	if workers < 1 {
		workers = 1
	}
	return w.ParallelConnectBlocks(nil, lower, higher, uint32(workers))
}
