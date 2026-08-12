package api

import (
	stdErrors "errors"
	"math"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
)

// maxHealBackoffShift caps the backoff at 2^6 passes - about 2.7 days at the hourly heal
// cadence - so a block that keeps failing is slowed down but never abandoned.
const maxHealBackoffShift = 6

// healInterBlockDelay spaces consecutive heal attempts. After a long backend outage the
// queue can hold tens of thousands of blocks, and pass 0 retries all of them after a
// restart - back-to-back traces would compete with live sync for the backend.
const healInterBlockDelay = time.Second

// HealInternalData walks the internal data error queue once and heals the blocks whose
// backoff schedule is due in this pass. Blocks are handled one at a time, spaced by
// healInterBlockDelay, to bound the trace load healing adds to the backend. Called
// serially from a single loop, so it needs no single-flight guard; stop cuts the pass
// short on shutdown.
func (w *Worker) HealInternalData(pass uint64, stop <-chan os.Signal) {
	internalErrors, err := w.db.GetBlockInternalDataErrorsEthereumType()
	if err != nil {
		glog.Error("HealInternalData: GetBlockInternalDataErrorsEthereumType ", err)
		return
	}
	var healed, failed, fetchErrors, reconnectErrors, postponed int
	attempted := false
	for i := range internalErrors {
		// give up between blocks so shutdown waits for at most one of them
		if common.IsInShutdown() {
			glog.Info("HealInternalData: interrupted by shutdown")
			break
		}
		ie := &internalErrors[i]
		if !healDue(ie.Retries, pass) {
			postponed++
			continue
		}
		// rate-limit only actual attempts - postponed entries cost the backend nothing
		if attempted && !sleepBetweenHeals(stop) {
			glog.Info("HealInternalData: interrupted by shutdown")
			break
		}
		attempted = true
		glog.Infof("HealInternalData: healing internal data of %d %s, previous failures %d", ie.Height, ie.Hash, ie.Retries)
		block, err := w.fetchBlockForHealing(ie.Hash, ie.Height)
		// the fetch above cannot be aborted - go-ethereum's rpc client Close is a no-op
		// over HTTP - so check again before touching the database, to keep shutdown
		// bounded by one write instead of an RPC timeout
		if common.IsInShutdown() {
			glog.Info("HealInternalData: interrupted by shutdown")
			break
		}
		if err != nil || block == nil {
			glog.Errorf("HealInternalData: %d %s, fetch error %v", ie.Height, ie.Hash, err)
			// a backend that does not have the block has answered definitively about this
			// block, so charge a failure. Every other error is backend-wide and usually
			// transient, and must not back off every queued block during an outage.
			if stdErrors.Is(errors.Cause(err), bchain.ErrBlockNotFound) {
				failed++
				w.chargeInternalDataFailure(ie, err.Error())
			} else {
				// uncharged, but counted - a pass lost to an outage must not look like
				// a pass that attempted nothing
				fetchErrors++
			}
			continue
		}
		blockSpecificData, _ := block.CoinSpecificData.(*bchain.EthereumBlockSpecificData)
		if blockSpecificData != nil && blockSpecificData.InternalDataError != "" {
			glog.Errorf("HealInternalData: %d %s, internal data error %v", ie.Height, ie.Hash, blockSpecificData.InternalDataError)
			failed++
			w.chargeInternalDataFailure(ie, blockSpecificData.InternalDataError)
			continue
		}
		if err = w.db.ReconnectInternalDataToBlockEthereumType(block); err != nil {
			// a reconnect failure is local - rocksdb or the disk - or a reorg that moved
			// the block out of the index, rather than a property of the block, so it does
			// not spend the backoff budget; a block stuck here stays visible through the
			// reconnect_error series of the heals metric
			glog.Errorf("HealInternalData: ReconnectInternalDataToBlockEthereumType %d %s, error %v", ie.Height, ie.Hash, err)
			reconnectErrors++
			continue
		}
		healed++
		glog.Infof("HealInternalData: healed internal data of %d %s", ie.Height, ie.Hash)
	}
	if healed > 0 || failed > 0 || fetchErrors > 0 || reconnectErrors > 0 {
		glog.Infof("HealInternalData: pass %d done, %d healed, %d failed, %d fetch errors, %d reconnect errors, %d backed off", pass, healed, failed, fetchErrors, reconnectErrors, postponed)
	}
	if w.metrics != nil {
		// zero Adds still create the series, so dashboards see the counters from the first pass
		w.metrics.InternalDataHeals.With(common.Labels{"result": "healed"}).Add(float64(healed))
		w.metrics.InternalDataHeals.With(common.Labels{"result": "failed"}).Add(float64(failed))
		w.metrics.InternalDataHeals.With(common.Labels{"result": "fetch_error"}).Add(float64(fetchErrors))
		w.metrics.InternalDataHeals.With(common.Labels{"result": "reconnect_error"}).Add(float64(reconnectErrors))
		// healed blocks left the queue with their reconnect; new sync failures show up next pass
		w.metrics.InternalDataErrorQueue.Set(float64(len(internalErrors) - healed))
	}
}

// sleepBetweenHeals rate-limits a healing pass to one block per healInterBlockDelay,
// reporting false when the wait is cut short by shutdown.
func sleepBetweenHeals(stop <-chan os.Signal) bool {
	select {
	case <-stop:
		return false
	case <-time.After(healInterBlockDelay):
		return true
	}
}

// healDue spaces out repeated failures: a block that has failed n times is attempted on
// passes divisible by 2^n (n capped by maxHealBackoffShift), so the wait after a failure
// is up to 2^n passes, not exactly 2^n - the schedule is aligned to the pass counter, not
// to the block. Pass 0 matches every schedule, so a restart gives the whole queue one
// immediate attempt.
func healDue(failures uint8, pass uint64) bool {
	shift := uint(failures)
	if shift > maxHealBackoffShift {
		shift = maxHealBackoffShift
	}
	return pass%(1<<shift) == 0
}

// chargeInternalDataFailure records a failure of the block itself, which both keeps the
// reason visible on the admin page and doubles the block's backoff.
func (w *Worker) chargeInternalDataFailure(ie *db.BlockInternalDataError, message string) {
	failures := ie.Retries
	if failures < math.MaxUint8 {
		failures++
	}
	if err := w.db.UpdateBlockInternalDataErrorEthereumType(ie.Height, ie.Hash, message, failures); err != nil {
		glog.Errorf("HealInternalData: UpdateBlockInternalDataErrorEthereumType %d %s, error %v", ie.Height, ie.Hash, err)
	}
}

// fetchBlockForHealing fetches a block, retrying once when its internal data are still
// missing - the second attempt succeeds many times more often, apparently because the
// first one warms the backend's trace cache. The returned block may still carry an
// InternalDataError; the caller decides what that means.
func (w *Worker) fetchBlockForHealing(hash string, height uint32) (*bchain.Block, error) {
	block, err := w.chain.GetBlock(hash, height)
	if err == nil && block != nil {
		if bsd, _ := block.CoinSpecificData.(*bchain.EthereumBlockSpecificData); bsd == nil || bsd.InternalDataError == "" {
			return block, nil
		}
	}
	if stdErrors.Is(errors.Cause(err), bchain.ErrBlockNotFound) {
		// the backend does not have the block at all, warming its cache cannot help
		return nil, err
	}
	glog.Errorf("HealInternalData: %d %s, first fetch failed (error %v), retrying", height, hash, err)
	return w.chain.GetBlock(hash, height)
}
