// +build integration

package db

func ConnectBlocks(w *SyncWorker, onNewBlock func(hash string), initialSync bool) error {
	return w.connectBlocks(onNewBlock, initialSync)
}

func HandleFork(w *SyncWorker, localBestHeight uint32, localBestHash string, onNewBlock func(hash string), initialSync bool) error {
	return w.handleFork(localBestHeight, localBestHash, onNewBlock, initialSync)
}
