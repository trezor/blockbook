package api

import (
	stdErrors "errors"
	"math"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
)

// maxHealBackoffShift caps the backoff at 2^6 passes - about 2.7 days at the hourly heal
// cadence - so a block that keeps failing is slowed down but never abandoned.
const maxHealBackoffShift = 6

// HealInternalData walks the internal data error queue once and heals the blocks whose
// backoff schedule is due in this pass. Blocks are handled one at a time to bound the
// trace load healing adds to the backend. Called serially from a single loop, so it needs
// no single-flight guard.
func (w *Worker) HealInternalData(pass uint64) {
	internalErrors, err := w.db.GetBlockInternalDataErrorsEthereumType()
	if err != nil {
		glog.Error("HealInternalData: GetBlockInternalDataErrorsEthereumType ", err)
		return
	}
	var healed, failed, postponed int
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
			// not spend the backoff budget
			glog.Errorf("HealInternalData: ReconnectInternalDataToBlockEthereumType %d %s, error %v", ie.Height, ie.Hash, err)
			continue
		}
		healed++
		glog.Infof("HealInternalData: healed internal data of %d %s", ie.Height, ie.Hash)
	}
	if healed > 0 || failed > 0 {
		glog.Infof("HealInternalData: pass %d done, %d healed, %d failed, %d backed off", pass, healed, failed, postponed)
	}
}

// healDue spaces out repeated failures: a block that has failed n times is attempted
// every 2^n passes, capped by maxHealBackoffShift. Pass 0 matches every schedule, so a
// restart gives the whole queue one immediate attempt.
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
