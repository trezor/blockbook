package api

import (
	"errors"
	"os"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
)

// refetch internal data
var (
	refetchInternalDataMux sync.Mutex
	refetchingInternalData bool
	// refetchInternalDataStopped is set by StopInternalDataRefetch. It is read under
	// refetchInternalDataMux, the same lock that guards the WaitGroup Add below, so no
	// pass can start after the barrier has begun waiting.
	refetchInternalDataStopped bool
	refetchInternalDataWG      sync.WaitGroup
)

func (w *Worker) IsRefetchingInternalData() bool {
	refetchInternalDataMux.Lock()
	defer refetchInternalDataMux.Unlock()
	return refetchingInternalData
}

const (
	internalDataErrorRetryPeriod       = time.Hour
	internalDataErrorRetryStartupDelay = time.Minute
)

// InternalDataErrorRetryLoop drains the internal data error queue without operator
// action. The first pass runs shortly after startup, while the backend cache is still
// warm from the failed sync fetches, then hourly. It returns when stop is closed or the
// application enters shutdown. RefetchInternalData is a no-op while a pass is already
// running or the queue is empty.
func (w *Worker) InternalDataErrorRetryLoop(stop <-chan os.Signal) {
	if w.chainType != bchain.ChainEthereumType || !bchain.ProcessInternalTransactions {
		return
	}
	glog.Info("InternalDataErrorRetryLoop starting")
	defer glog.Info("InternalDataErrorRetryLoop stopped")
	timer := time.NewTimer(internalDataErrorRetryStartupDelay)
	defer timer.Stop()
	for {
		select {
		case <-stop:
			return
		case <-timer.C:
		}
		timer.Reset(internalDataErrorRetryPeriod)
		if common.IsInShutdown() {
			return
		}
		// keep the retry cap here so a permanently unfetchable block is not hammered
		// forever; only the operator's admin button revives an exhausted block
		if err := w.RefetchInternalData(false); err != nil {
			glog.Errorf("InternalDataErrorRetryLoop: %v", err)
		}
	}
}

// RefetchInternalData starts a pass over the internal data error queue unless one is
// already running or the application is shutting down. resetRetries clears exhausted
// retry counts - set by the admin "Retry fetch" button so abandoned blocks can be
// revived, false for the auto-retry.
func (w *Worker) RefetchInternalData(resetRetries bool) error {
	refetchInternalDataMux.Lock()
	defer refetchInternalDataMux.Unlock()
	if refetchInternalDataStopped || common.IsInShutdown() {
		return nil
	}
	if !refetchingInternalData {
		refetchingInternalData = true
		refetchInternalDataWG.Add(1)
		go func() {
			defer refetchInternalDataWG.Done()
			w.RefetchInternalDataRoutine(resetRetries)
		}()
	}
	return nil
}

// internalDataRefetchStopTimeout bounds how long shutdown waits for a healing pass. The
// pass gives up as soon as it regains control, but a fetch already in flight cannot be
// aborted - go-ethereum's rpc client Close is a no-op over HTTP - so the wait lasts as
// long as the backend takes to answer.
const internalDataRefetchStopTimeout = 15 * time.Second

// StopInternalDataRefetch blocks further refetch passes and waits for one in flight to
// finish. It must run before the database is closed: a pass reads and writes RocksDB
// from its own goroutine, and Close destroys the column family handles and clears the
// database handle, so a pass racing the close dereferences freed memory. It reports
// whether the pass finished; on timeout the caller is better off closing anyway, which
// is no worse than not having waited at all.
func StopInternalDataRefetch() bool {
	refetchInternalDataMux.Lock()
	refetchInternalDataStopped = true
	running := refetchingInternalData
	refetchInternalDataMux.Unlock()
	if running {
		glog.Info("shutdown: waiting for the internal data refetch pass to finish")
	}
	done := make(chan struct{})
	go func() {
		refetchInternalDataWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(internalDataRefetchStopTimeout):
		glog.Error("shutdown: internal data refetch pass still running after ", internalDataRefetchStopTimeout, ", continuing")
		return false
	}
}

// internalDataRefetchStopping reports whether a running pass should abandon the rest of
// the queue, either because the application is shutting down or because
// StopInternalDataRefetch is waiting for it.
func internalDataRefetchStopping() bool {
	if common.IsInShutdown() {
		return true
	}
	refetchInternalDataMux.Lock()
	defer refetchInternalDataMux.Unlock()
	return refetchInternalDataStopped
}

const maxNumberOfRetires = 25

// incrementRefetchInternalDataRetryCount charges the block one retry and records the
// failure that caused it. The update is dropped if the queue no longer holds this block.
func (w *Worker) incrementRefetchInternalDataRetryCount(ie *db.BlockInternalDataError, message string) {
	if err := w.db.UpdateBlockInternalDataErrorEthereumType(ie.Height, ie.Hash, message, ie.Retries+1); err != nil {
		glog.Errorf("UpdateBlockInternalDataErrorEthereumType %d %s, error %v", ie.Height, ie.Hash, err)
	}
}

// RefetchInternalDataRoutine walks the internal data error queue once. Start it through
// RefetchInternalData, which registers the pass with the shutdown barrier.
func (w *Worker) RefetchInternalDataRoutine(resetRetries bool) {
	defer func() {
		refetchInternalDataMux.Lock()
		refetchingInternalData = false
		refetchInternalDataMux.Unlock()
	}()
	internalErrors, err := w.db.GetBlockInternalDataErrorsEthereumType()
	if err != nil {
		glog.Errorf("GetBlockInternalDataErrorsEthereumType, error %v", err)
		return
	}
	exceeded := 0
	for i := range internalErrors {
		// give up between blocks so StopInternalDataRefetch waits for at most one block
		if internalDataRefetchStopping() {
			glog.Info("Refetching internal data interrupted by shutdown")
			break
		}
		ie := &internalErrors[i]
		// an operator-triggered refetch revives only the blocks that already burned
		// their budget; entries still under the cap keep their real count, so the
		// periodic auto-retry can still give up on them and the admin table keeps
		// reporting how many attempts a block has actually cost
		if resetRetries && ie.Retries >= maxNumberOfRetires {
			ie.Retries = 0
		}
		if ie.Retries >= maxNumberOfRetires {
			exceeded++
			continue
		}
		glog.Infof("Refetching internal data for %d %s, retries %d", ie.Height, ie.Hash, ie.Retries)
		block, err := w.getBlockRetryInternalData(ie.Hash, ie.Height)
		// chain.Shutdown does not abort the fetch above - go-ethereum's rpc client Close
		// is a no-op over HTTP - so check again here, before any database access, to keep
		// the shutdown barrier bounded by one write instead of an RPC timeout
		if internalDataRefetchStopping() {
			glog.Info("Refetching internal data interrupted by shutdown")
			break
		}
		if err != nil || block == nil {
			glog.Errorf("Refetching internal data for %d %s, error %v", ie.Height, ie.Hash, err)
			// a backend that does not have the block has given a definitive answer about
			// this block, so charge a retry - otherwise the entry never reaches
			// maxNumberOfRetires and the hourly loop refetches it forever. Every other
			// error here (transport, eth_getLogs) is backend-wide and usually transient,
			// and must not spend the budget of every queued block during an outage.
			if errors.Is(err, bchain.ErrBlockNotFound) {
				w.incrementRefetchInternalDataRetryCount(ie, err.Error())
			}
			continue
		}
		blockSpecificData, _ := block.CoinSpecificData.(*bchain.EthereumBlockSpecificData)
		if blockSpecificData != nil && blockSpecificData.InternalDataError != "" {
			glog.Errorf("Refetching internal data for %d %s, internal data error %v", ie.Height, ie.Hash, blockSpecificData.InternalDataError)
			w.incrementRefetchInternalDataRetryCount(ie, blockSpecificData.InternalDataError)
		} else if err = w.db.ReconnectInternalDataToBlockEthereumType(block); err != nil {
			// a reconnect failure is local - rocksdb or the disk - rather than a property
			// of the block, so retrying it is right and it does not consume the budget
			glog.Errorf("ReconnectInternalDataToBlockEthereumType %d %s, error %v", ie.Height, ie.Hash, err)
		} else {
			if blockSpecificData != nil {
				w.storeHealedContracts(blockSpecificData.Contracts)
			}
			glog.Infof("Refetching internal data for %d %s, success", ie.Height, ie.Hash)
		}
	}
	// one line per pass - the blocks that ran out of budget stay queued, and the admin
	// page lists them with their retry counts
	if exceeded > 0 {
		glog.Infof("Refetching internal data, %d blocks skipped with retries exceeded", exceeded)
	}
}

// storeHealedContracts registers the contracts a healed block created or destroyed.
// The failed fetch is what produced them, so sync stored none, and the reconnect stores
// internal data only - without this the block regains its internal data but its
// contracts stay unregistered.
func (w *Worker) storeHealedContracts(contracts []bchain.ContractInfo) {
	for i := range contracts {
		ci := &contracts[i]
		if err := w.db.BackfillContractInfo(ci); err != nil {
			glog.Errorf("storeHealedContracts: BackfillContractInfo %s: %v", ci.Contract, err)
		}
	}
}

// getBlockRetryInternalData fetches a block, retrying once when the internal data are
// missing or errored - the second attempt usually succeeds, apparently because the
// first warms the backend cache. A block the backend does not have is reported straight
// away, since warming its cache cannot conjure one up. The returned block may still
// carry an InternalDataError; the caller decides how to handle that.
func (w *Worker) getBlockRetryInternalData(hash string, height uint32) (*bchain.Block, error) {
	block, err := w.chain.GetBlock(hash, height)
	if err == nil && block != nil {
		if bsd, _ := block.CoinSpecificData.(*bchain.EthereumBlockSpecificData); bsd == nil || bsd.InternalDataError == "" {
			return block, nil
		}
	}
	if errors.Is(err, bchain.ErrBlockNotFound) {
		// the backend does not have the block at all; a second attempt cannot change that
		return nil, err
	}
	glog.Errorf("getBlockRetryInternalData %d %s, first attempt failed (error %v), retrying", height, hash, err)
	return w.chain.GetBlock(hash, height)
}
