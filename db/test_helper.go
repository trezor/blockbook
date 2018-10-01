// +build integration

package db

import (
	"blockbook/bchain"
)

func ConnectBlocks(w *SyncWorker, onNewBlock bchain.OnNewBlockFunc, initialSync bool) error {
	return w.connectBlocks(onNewBlock, initialSync)
}

func HandleFork(w *SyncWorker, localBestHeight uint32, localBestHash string, onNewBlock bchain.OnNewBlockFunc, initialSync bool) error {
	return w.handleFork(localBestHeight, localBestHash, onNewBlock, initialSync)
}
