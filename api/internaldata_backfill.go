package api

import (
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// InternalDataHealingRoutine keeps the internal data of the index complete. It
// combines two mechanisms: a periodic, always-on retry of the internal data error
// queue (blocks whose internal data fetch failed during sync or healing), and a
// downward sweep that backfills blocks synced before processInternalTransactions was
// enabled, without a full reindex. The sweep runs only when an operator seeded the
// watermark (the -healinternaldata flag, see RocksDB.ResolveInternalDataFrom) or when
// a previous sweep was interrupted; its range is the internal state watermark, and it
// sweeps downward - healing the most recent history first and moving the watermark
// after every block. Progress is persisted with the periodic internal state store, so
// an interrupted run resumes after restart; re-healing a block is safe - transactions
// whose stored internal data already match the recomputed data are skipped.
func (w *Worker) InternalDataHealingRoutine() {
	if w.chainType != bchain.ChainEthereumType || !bchain.ProcessInternalTransactions {
		return
	}
	// blocks whose internal data fetch failed - during sync or during the healing
	// sweep below - wait in the internal data error queue; retry them periodically
	// so that gaps close without operator action
	go w.internalDataErrorRetryLoop()
	from, resolved := w.is.GetInternalDataFrom()
	if !resolved || from == 0 {
		return
	}
	glog.Infof("internalDataHealing: index is missing internal data below height %d, backfilling from the backend", from)
	start := time.Now()
	var healed, errorCount uint64
	for h := from; h > 0; {
		if common.IsInShutdown() {
			glog.Infof("internalDataHealing: shutdown at height %d, healing resumes after restart", h)
			return
		}
		// do not compete with the sync for the backend while catching up
		if synced, _, _, _ := w.is.GetSyncState(); !synced {
			time.Sleep(10 * time.Second)
			continue
		}
		height := h - 1
		// resolve the hash up front: a real read error must not advance the watermark
		// past a block we could neither heal nor enqueue - that would drop it from
		// healing with no way to re-seed - so keep the watermark and retry the height
		hash, err := w.db.GetBlockHash(height)
		if err != nil {
			glog.Errorf("internalDataHealing: height %d: GetBlockHash: %v", height, err)
			time.Sleep(10 * time.Second)
			continue
		}
		if hash != "" {
			if err := w.backfillBlockInternalData(height, hash); err != nil {
				errorCount++
				glog.Errorf("internalDataHealing: height %d: %v", height, err)
				// enqueue with the resolved hash so the periodic retry can heal it
				// later, without a second lookup that could itself fail
				w.storeBlockInternalDataError(hash, height, err.Error(), 0)
			}
		}
		h = height
		w.is.SetInternalDataFrom(h)
		healed++
		if healed%10000 == 0 {
			rate := float64(healed) / time.Since(start).Seconds()
			glog.Infof("internalDataHealing: %d blocks healed, at height %d, %.1f blocks/s, %d errors", healed, h, rate, errorCount)
		}
	}
	glog.Infof("internalDataHealing: finished, %d blocks healed in %v, %d errors", healed, time.Since(start), errorCount)
}

const internalDataErrorRetryPeriod = time.Hour

// internalDataErrorRetryLoop periodically retries the blocks in the internal data
// error queue. The underlying refetch routine runs at most once at a time, is a
// no-op on an empty queue and gives up on a block after its retry limit, so the
// periodic trigger is safe. The first pass runs shortly after startup to pick up
// failures from the previous run while the backend cache is warm.
func (w *Worker) internalDataErrorRetryLoop() {
	period := time.Minute
	for {
		time.Sleep(period)
		period = internalDataErrorRetryPeriod
		if common.IsInShutdown() {
			return
		}
		// periodic auto-retry keeps the retry cap so it never hammers a permanently
		// unfetchable block; an operator can force a reset via the admin page
		if err := w.RefetchInternalData(false); err != nil {
			glog.Errorf("internalDataErrorRetryLoop: %v", err)
		}
	}
}

// backfillBlockInternalData fetches the internal data for the already-indexed block
// at height (whose current hash the caller resolved) and reconnects it. The caller
// passes the resolved hash so a failure can be enqueued without a second lookup that
// could itself fail and lose the block.
func (w *Worker) backfillBlockInternalData(height uint32, hash string) error {
	block, err := w.getBlockRetryInternalData(hash, height)
	if err != nil {
		return err
	}
	if block == nil {
		return errors.New("block not found on backend")
	}
	blockSpecificData, _ := block.CoinSpecificData.(*bchain.EthereumBlockSpecificData)
	if blockSpecificData != nil && blockSpecificData.InternalDataError != "" {
		return errors.New(blockSpecificData.InternalDataError)
	}
	// guard against a reorg between reading the indexed hash and the backend fetch
	if currentHash, err := w.db.GetBlockHash(height); err != nil || currentHash != hash {
		return errors.New("block hash changed, possible reorg, skipping")
	}
	if err := w.db.ReconnectInternalDataToBlockEthereumType(block); err != nil {
		return err
	}
	if blockSpecificData != nil {
		w.storeBackfilledContracts(blockSpecificData)
	}
	return nil
}

// storeBackfilledContracts persists the contract lifecycle discovered during the
// backfill (creations and destructions); ReconnectInternalDataToBlockEthereumType
// does not store them. Because the sweep runs downward, a contract's destruction is
// visited before its creation, so BackfillContractInfo records a destruction even
// when no registry row exists yet and merges the creation height into it later,
// preserving any name/symbol/standard enrichment fetched on demand after sync.
func (w *Worker) storeBackfilledContracts(blockSpecificData *bchain.EthereumBlockSpecificData) {
	for i := range blockSpecificData.Contracts {
		ci := &blockSpecificData.Contracts[i]
		if err := w.db.BackfillContractInfo(ci); err != nil {
			glog.Errorf("storeBackfilledContracts: BackfillContractInfo %s: %v", ci.Contract, err)
		}
	}
}
