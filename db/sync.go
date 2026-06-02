package db

import (
	"context"
	stdErrors "errors"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// SyncWorker is handle to SyncWorker
type SyncWorker struct {
	db                     *RocksDB
	chain                  bchain.BlockChain
	syncWorkers, syncChunk int
	dryRun                 bool
	startHeight            uint32
	startHash              string
	chanOsSignal           chan os.Signal
	missingBlockRetry      MissingBlockRetryConfig
	metrics                *common.Metrics
	is                     *common.InternalState
}

// MissingBlockRetryConfig controls how long we retry a missing block before re-checking chain state.
type MissingBlockRetryConfig struct {
	// RecheckThreshold is the number of consecutive ErrBlockNotFound retries
	// before re-checking the tip/hash for a reorg or rollback.
	RecheckThreshold int
	// TipRecheckThreshold is a lower threshold used once the hash queue is
	// closed (we are at the tail of the requested range).
	TipRecheckThreshold int
	// RetryDelay keeps retry pressure low while still reacting quickly to transient backend gaps.
	RetryDelay time.Duration
	// MaxStallDuration caps the wall-clock time a single block fetch may spend in
	// the retry loop before yielding errResync. Liveness invariant: since lagging
	// probes report "no reorg" and known hashes get retried, a genuinely-behind
	// backend or chain-shortening reorg relies on this cap. Must stay > 0
	// (ApplyMissingBlockRetryOverride enforces it).
	MaxStallDuration time.Duration
}

// SyncWorkerConfig bundles optional tuning knobs for SyncWorker.
type SyncWorkerConfig struct {
	MissingBlockRetry MissingBlockRetryConfig
}

// DefaultMissingBlockRetryConfig returns the built-in defaults used when no
// per-chain override is supplied. Exported so blockbook.go can overlay
// optional bchain.MissingBlockRetry fields onto known-good defaults.
func DefaultMissingBlockRetryConfig() MissingBlockRetryConfig {
	return MissingBlockRetryConfig{
		RecheckThreshold:    10,
		RetryDelay:          1 * time.Second,
		TipRecheckThreshold: 3,
		MaxStallDuration:    60 * time.Second,
	}
}

func defaultSyncWorkerConfig() SyncWorkerConfig {
	return SyncWorkerConfig{
		MissingBlockRetry: DefaultMissingBlockRetryConfig(),
	}
}

// ApplyMissingBlockRetryOverride overlays the optional bchain.MissingBlockRetry
// onto the defaults. Zero / unset wire fields keep their default; explicitly set
// but invalid values (negative, or a TipRecheckThreshold above RecheckThreshold)
// keep the default and log a warning.
func ApplyMissingBlockRetryOverride(o *bchain.MissingBlockRetry) MissingBlockRetryConfig {
	cfg := DefaultMissingBlockRetryConfig()
	if o == nil {
		return cfg
	}
	apply := func(field string, v int, set func(int)) {
		if v == 0 {
			return // unset: keep default
		}
		if v < 0 {
			glog.Warningf("sync: missingBlockRetry.%s=%d is invalid, keeping default", field, v)
			return
		}
		set(v)
	}
	apply("retryDelayMs", o.RetryDelayMs, func(v int) { cfg.RetryDelay = time.Duration(v) * time.Millisecond })
	apply("recheckThreshold", o.RecheckThreshold, func(v int) { cfg.RecheckThreshold = v })
	apply("tipRecheckThreshold", o.TipRecheckThreshold, func(v int) { cfg.TipRecheckThreshold = v })
	apply("maxStallMs", o.MaxStallMs, func(v int) { cfg.MaxStallDuration = time.Duration(v) * time.Millisecond })
	if cfg.TipRecheckThreshold > cfg.RecheckThreshold {
		glog.Warningf("sync: missingBlockRetry.tipRecheckThreshold=%d exceeds recheckThreshold=%d, clamping to %d",
			cfg.TipRecheckThreshold, cfg.RecheckThreshold, cfg.RecheckThreshold)
		cfg.TipRecheckThreshold = cfg.RecheckThreshold
	}
	return cfg
}

// NewSyncWorker creates new SyncWorker and returns its handle
func NewSyncWorker(db *RocksDB, chain bchain.BlockChain, syncWorkers, syncChunk int, minStartHeight int, dryRun bool, chanOsSignal chan os.Signal, metrics *common.Metrics, is *common.InternalState) (*SyncWorker, error) {
	return NewSyncWorkerWithConfig(db, chain, syncWorkers, syncChunk, minStartHeight, dryRun, chanOsSignal, metrics, is, nil)
}

// NewSyncWorkerWithConfig allows tests or callers to override SyncWorker defaults.
func NewSyncWorkerWithConfig(db *RocksDB, chain bchain.BlockChain, syncWorkers, syncChunk int, minStartHeight int, dryRun bool, chanOsSignal chan os.Signal, metrics *common.Metrics, is *common.InternalState, cfg *SyncWorkerConfig) (*SyncWorker, error) {
	if minStartHeight < 0 {
		minStartHeight = 0
	}
	effectiveCfg := defaultSyncWorkerConfig()
	if cfg != nil {
		effectiveCfg = *cfg
	}
	// MaxStallDuration is the load-bearing liveness cap (see its doc): the retry
	// loops disable the cap when it's <= 0, which would let a chain-shortening
	// reorg spin forever. Enforce the invariant structurally here so every caller
	// (including tests passing a partial cfg) gets a safe value, not just the
	// ApplyMissingBlockRetryOverride path.
	if effectiveCfg.MissingBlockRetry.MaxStallDuration <= 0 {
		effectiveCfg.MissingBlockRetry.MaxStallDuration = DefaultMissingBlockRetryConfig().MaxStallDuration
	}
	return &SyncWorker{
		db:                db,
		chain:             chain,
		syncWorkers:       syncWorkers,
		syncChunk:         syncChunk,
		dryRun:            dryRun,
		startHeight:       uint32(minStartHeight),
		chanOsSignal:      chanOsSignal,
		missingBlockRetry: effectiveCfg.MissingBlockRetry,
		metrics:           metrics,
		is:                is,
	}, nil
}

// syncNotNeeded is returned by resyncIndex when the local tip already matches
// the backend tip. ResyncIndex treats it as a successful no-op.
var syncNotNeeded = errors.New("sync not needed")
var errFork = errors.New("fork")

// errResync signals that the parallel/bulk sync should restart because the
// target block hash no longer matches the chain (likely reorg/rollback).
var errResync = errors.New("resync")

// ErrOperationInterrupted is returned when operation is interrupted by OS signal
var ErrOperationInterrupted = errors.New("ErrOperationInterrupted")

func (w *SyncWorker) updateBackendInfo() {
	ci, err := w.chain.GetChainInfo()
	var backendError string
	if err != nil {
		glog.Error("GetChainInfo error ", err)
		backendError = errors.Annotatef(err, "GetChainInfo").Error()
		ci = &bchain.ChainInfo{}
	}
	w.is.SetBackendInfo(&common.BackendInfo{
		BackendError:     backendError,
		BestBlockHash:    ci.Bestblockhash,
		Blocks:           ci.Blocks,
		Chain:            ci.Chain,
		Difficulty:       ci.Difficulty,
		Headers:          ci.Headers,
		ProtocolVersion:  ci.ProtocolVersion,
		SizeOnDisk:       ci.SizeOnDisk,
		Subversion:       ci.Subversion,
		Timeoffset:       ci.Timeoffset,
		Version:          ci.Version,
		Warnings:         ci.Warnings,
		ConsensusVersion: ci.ConsensusVersion,
		Consensus:        ci.Consensus,
	})
	// Refresh tip-age on every resync outcome (nil/syncNotNeeded/error), not only on a
	// successful connect: during a silent stall resyncIndex returns syncNotNeeded each
	// run, so without this the gauge would only be refreshed by the ~15-minute app-info
	// loop. A climbing blockbook_tip_age_seconds is the primary stall signal.
	w.metrics.BackendTipAgeSeconds.Set(time.Since(w.is.GetBackendTipLastAdvance()).Seconds())
}

// ResyncIndex synchronizes index to the top of the blockchain
// onNewBlock is called when new block is connected, but not in initial parallel sync
func (w *SyncWorker) ResyncIndex(onNewBlock bchain.OnNewBlockFunc, initialSync bool) error {
	start := time.Now()
	w.is.StartedSync()

	err := w.resyncIndex(onNewBlock, initialSync)

	// update backend info after each resync
	w.updateBackendInfo()

	switch err {
	case nil:
		d := time.Since(start)
		glog.Info("resync: finished in ", d)
		w.metrics.IndexResyncDuration.Observe(float64(d) / 1e6) // in milliseconds
		w.metrics.IndexDBSize.Set(float64(w.db.DatabaseSizeOnDisk()))
		bh, _, err := w.db.GetBestBlock()
		if err == nil {
			w.is.FinishedSync(bh)
		}
		w.metrics.BackendBestHeight.Set(float64(w.is.BackendInfo.Blocks))
		w.metrics.BlockbookBestHeight.Set(float64(bh))
		return err
	case syncNotNeeded:
		w.is.FinishedSyncNoChange()
		w.metrics.IndexDBSize.Set(float64(w.db.DatabaseSizeOnDisk()))
		if initialSync {
			d := time.Since(start)
			glog.Info("resync: finished in ", d)
		}
		return nil
	}

	w.metrics.IndexResyncErrors.With(common.Labels{"error": "failure"}).Inc()

	return err
}

func (w *SyncWorker) resyncIndex(onNewBlock bchain.OnNewBlockFunc, initialSync bool) error {
	remoteBestHash, err := w.chain.GetBestBlockHash()
	if err != nil {
		return err
	}
	localBestHeight, localBestHash, err := w.db.GetBestBlock()
	if err != nil {
		return err
	}
	// If the locally indexed block is the same as the best block on the network, we're done.
	if localBestHash == remoteBestHash {
		glog.Infof("resync: synced at %d %s", localBestHeight, localBestHash)
		return syncNotNeeded
	}
	if localBestHash != "" {
		remoteHash, err := w.chain.GetBlockHash(localBestHeight)
		// for some coins (eth) remote can be at lower best height after rollback
		if err != nil && !stdErrors.Is(err, bchain.ErrBlockNotFound) {
			return err
		}
		if remoteHash != localBestHash {
			// forked - the remote hash differs from the local hash at the same height
			glog.Info("resync: local is forked at height ", localBestHeight, ", local hash ", localBestHash, ", remote hash ", remoteHash)
			return w.handleFork(localBestHeight, localBestHash, onNewBlock, initialSync)
		}
		glog.Info("resync: local at ", localBestHeight, " is behind")
		w.startHeight = localBestHeight + 1
	} else {
		// database is empty, start genesis
		glog.Info("resync: genesis from block ", w.startHeight)
	}
	w.startHash, err = w.chain.GetBlockHash(w.startHeight)
	if err != nil {
		return err
	}
	// if parallel operation is enabled and the number of blocks to be connected is large,
	// use parallel routine to load majority of blocks
	// use parallel sync only in case of initial sync because it puts the db to inconsistent state
	// or in case of ChainEthereumType if the tip is farther
	if w.syncWorkers > 1 && (initialSync || w.chain.GetChainParser().GetChainType() == bchain.ChainEthereumType) {
		remoteBestHeight, err := w.chain.GetBestBlockHeight()
		if err != nil {
			return err
		}
		if remoteBestHeight < w.startHeight {
			glog.Warning("resync: observed remote best height ", remoteBestHeight, " less than sync start height ", w.startHeight, ", falling back to sequential sync")
		} else {
			if initialSync {
				if remoteBestHeight-w.startHeight > uint32(w.syncChunk) {
					glog.Infof("resync: bulk sync of blocks %d-%d, using %d workers", w.startHeight, remoteBestHeight, w.syncWorkers)
					// Bulk sync can encounter a disappearing block hash during reorgs.
					// When that happens, it returns errResync to trigger a full restart.
					err = w.BulkConnectBlocks(w.startHeight, remoteBestHeight)
					if err != nil {
						if stdErrors.Is(err, errResync) {
							// block hash changed during parallel sync, restart the full resync
							return w.resyncIndex(onNewBlock, initialSync)
						}
						return err
					}
					// after parallel load finish the sync using standard way,
					// new blocks may have been created in the meantime
					return w.resyncIndex(onNewBlock, initialSync)
				}
			}
			if w.chain.GetChainParser().GetChainType() == bchain.ChainEthereumType {
				syncWorkers := uint32(4)
				if remoteBestHeight-w.startHeight >= syncWorkers {
					glog.Infof("resync: parallel sync of blocks %d-%d, using %d workers", w.startHeight, remoteBestHeight, syncWorkers)
					// Parallel sync also returns errResync when a requested hash no longer
					// exists at its height; restart to realign with the canonical chain.
					err = w.ParallelConnectBlocks(onNewBlock, w.startHeight, remoteBestHeight, syncWorkers)
					if err != nil {
						if stdErrors.Is(err, errResync) {
							// block hash changed during parallel sync, restart the full resync
							return w.resyncIndex(onNewBlock, initialSync)
						}
						return err
					}
					// after parallel load finish the sync using standard way,
					// new blocks may have been created in the meantime
					return w.resyncIndex(onNewBlock, initialSync)
				}
			}
		}
	}
	err = w.connectBlocks(onNewBlock, initialSync)
	if stdErrors.Is(err, errFork) || stdErrors.Is(err, errResync) {
		return w.resyncIndex(onNewBlock, initialSync)
	}
	return err
}

func (w *SyncWorker) handleFork(localBestHeight uint32, localBestHash string, onNewBlock bchain.OnNewBlockFunc, initialSync bool) error {
	// find forked blocks, disconnect them and then synchronize again
	var height uint32
	hashes := []string{localBestHash}
	for height = localBestHeight - 1; ; height-- {
		local, err := w.db.GetBlockHash(height)
		if err != nil {
			return err
		}
		if local == "" {
			break
		}
		remote, err := w.chain.GetBlockHash(height)
		// A tolerated ErrBlockNotFound leaves remote == "", which (local is non-empty
		// here) counts this height as forked and disconnects it. That is intended: for
		// EVM the backend can sit at a lower height after a rollback, and those blocks
		// must be disconnected to realign with the chain. The tradeoff is that on a
		// load-balanced backend a transient lagging node can answer NotFound for a block
		// that is still canonical, over-disconnecting — bounded and self-healing, since
		// the resyncIndex below re-connects them. Treating NotFound as a stop instead
		// would be worse: genuinely orphaned blocks would stay connected after a real
		// rollback, leaving the index wedged ahead of the backend.
		if err != nil && !stdErrors.Is(err, bchain.ErrBlockNotFound) {
			return err
		}
		if local == remote {
			break
		}
		hashes = append(hashes, local)
	}
	w.metrics.IndexReorgEvents.With(common.Labels{"type": "disconnect"}).Inc()
	if err := w.DisconnectBlocks(height+1, localBestHeight, hashes); err != nil {
		return err
	}
	return w.resyncIndex(onNewBlock, initialSync)
}

func (w *SyncWorker) connectBlocks(onNewBlock bchain.OnNewBlockFunc, initialSync bool) error {
	bch := make(chan blockResult, 8)
	done := make(chan struct{})
	defer close(done)

	go w.getBlockChain(bch, done)

	var lastRes blockResult

	connect := func(res blockResult) error {
		lastRes = res
		if res.err != nil {
			return res.err
		}
		err := w.db.ConnectBlock(res.block)
		if err != nil {
			return err
		}
		if onNewBlock != nil {
			onNewBlock(res.block)
		}
		w.metrics.BlockbookBestHeight.Set(float64(res.block.Height))
		if res.block.Height > 0 && res.block.Height%1000 == 0 {
			glog.Info("connected block ", res.block.Height, " ", res.block.Hash)
		}

		return nil
	}

	logInterrupted := func() {
		if lastRes.block != nil {
			glog.Info("connectBlocks interrupted at height ", lastRes.block.Height)
		} else {
			glog.Info("connectBlocks interrupted")
		}
	}
	// During regular sync, shutdown is now signaled by closing chanOsSignal,
	// so we honor it here to avoid leaving RocksDB in an open state.
	// Initial sync uses the same shutdown-aware loop.
ConnectLoop:
	for {
		select {
		case <-w.chanOsSignal:
			logInterrupted()
			return ErrOperationInterrupted
		case res, ok := <-bch:
			if !ok {
				select {
				case <-w.chanOsSignal:
					logInterrupted()
					return ErrOperationInterrupted
				default:
				}
				break ConnectLoop
			}
			err := connect(res)
			if err != nil {
				return err
			}
		}
	}

	if lastRes.block != nil {
		glog.Infof("resync: synced at %d %s", lastRes.block.Height, lastRes.block.Hash)
	}

	return nil
}

type hashHeight struct {
	hash   string
	height uint32
}

// sendHashHeight queues hh but stays abort-aware: if a full hch made this a blocking
// send, the coordinator could never read abortCh and sync would wedge. On abort hh is
// intentionally dropped since the round is being torn down anyway.
func (w *SyncWorker) sendHashHeight(hch chan<- hashHeight, abortCh <-chan error, hh hashHeight) error {
	select {
	case hch <- hh:
		return nil
	case abortErr := <-abortCh:
		return abortErr
	case <-w.chanOsSignal:
		return ErrOperationInterrupted
	}
}

func (w *SyncWorker) shouldRestartSyncOnMissingBlock(height uint32, expectedHash string) (bool, error) {
	// When a block hash disappears at a given height, it can indicate a
	// reorg/rollback, but on load-balanced EVM RPCs a single lagging backend can
	// also report an older tip. Only restart immediately when another probe can
	// prove the height exists with a different hash; otherwise let the retry
	// loop or wall-clock cap yield control to the outer resync.
	bestHeight, err := w.chain.GetBestBlockHeight()
	if err != nil {
		return false, err
	}
	if bestHeight < height {
		return false, nil
	}
	currentHash, err := w.chain.GetBlockHash(height)
	if err != nil {
		if stdErrors.Is(err, bchain.ErrBlockNotFound) {
			return false, nil
		}
		return false, err
	}
	return currentHash != expectedHash, nil
}

// onRetryableMiss bumps the retry count and, once at threshold, rechecks chain
// state. It emits a single warning at the crossover; below threshold it stays
// quiet because transient backend lag (e.g. load-balanced RPC routing skew)
// is expected. The per-error signal is preserved via the IndexResyncErrors metric.
func (w *SyncWorker) onRetryableMiss(retries *int, threshold int, label string, height uint32, hash string, err error) (bool, error) {
	(*retries)++
	if *retries < threshold {
		return false, nil
	}
	if *retries == threshold {
		glog.Warningf("%s: block %d %s still missing after %d retries (last: %v); rechecking chain state",
			label, height, hash, *retries, err)
	}
	return w.shouldRestartSyncOnMissingBlock(height, hash)
}

func isRetryableGetBlockError(err error) bool {
	if err == nil {
		return false
	}
	isRetryable := func(e error) bool {
		if stdErrors.Is(e, bchain.ErrBlockNotFound) ||
			stdErrors.Is(e, context.DeadlineExceeded) ||
			stdErrors.Is(e, io.ErrUnexpectedEOF) ||
			stdErrors.Is(e, io.EOF) ||
			stdErrors.Is(e, net.ErrClosed) ||
			stdErrors.Is(e, syscall.ECONNRESET) ||
			stdErrors.Is(e, syscall.ECONNREFUSED) ||
			stdErrors.Is(e, syscall.ECONNABORTED) ||
			stdErrors.Is(e, syscall.EPIPE) ||
			stdErrors.Is(e, syscall.ETIMEDOUT) {
			return true
		}

		var netErr net.Error
		if stdErrors.As(e, &netErr) && netErr.Timeout() {
			return true
		}

		msg := strings.ToLower(e.Error())
		switch {
		case strings.Contains(msg, "connection reset by peer"),
			strings.Contains(msg, "connection refused"),
			strings.Contains(msg, "broken pipe"),
			strings.Contains(msg, "connection lost"),
			strings.Contains(msg, "client is closed"),
			strings.Contains(msg, "i/o timeout"),
			strings.Contains(msg, "request timed out"),
			strings.Contains(msg, "429 too many requests"),
			strings.Contains(msg, "502 bad gateway"),
			strings.Contains(msg, "503 service unavailable"),
			strings.Contains(msg, "504 gateway timeout"),
			strings.Contains(msg, "header not found"),
			strings.Contains(msg, "block not found"):
			return true
		default:
			return false
		}
	}
	if isRetryable(err) {
		return true
	}
	cause := errors.Cause(err)
	return cause != nil && isRetryable(cause)
}

// ParallelConnectBlocks uses parallel goroutines to get data from blockchain daemon but keeps Blockbook in
func (w *SyncWorker) ParallelConnectBlocks(onNewBlock bchain.OnNewBlockFunc, lower, higher uint32, syncWorkers uint32) error {
	var err error
	var wg sync.WaitGroup
	bch := make([]chan *bchain.Block, syncWorkers)
	for i := 0; i < int(syncWorkers); i++ {
		bch[i] = make(chan *bchain.Block)
	}
	hch := make(chan hashHeight, syncWorkers)
	hchClosed := atomic.Value{}
	hchClosed.Store(false)
	writeBlockDone := make(chan struct{})
	terminating := make(chan struct{})
	// abortCh is used by workers to signal a resync-worthy reorg or a terminal worker error.
	// Keep it buffered so the first worker can report without blocking while the
	// coordinator is closing channels/terminating.
	abortCh := make(chan error, 1)
	writeBlockWorker := func() {
		defer close(writeBlockDone)
		lastBlock := lower - 1
	WriteBlockLoop:
		for {
			select {
			case b := <-bch[(lastBlock+1)%syncWorkers]:
				if b == nil {
					// channel is closed and empty - work is done
					break WriteBlockLoop
				}
				if b.Height != lastBlock+1 {
					glog.Fatal("writeBlockWorker skipped block, expected block ", lastBlock+1, ", new block ", b.Height)
				}
				err := w.db.ConnectBlock(b)
				if err != nil {
					glog.Fatal("writeBlockWorker ", b.Height, " ", b.Hash, " error ", err)
				}

				if onNewBlock != nil {
					onNewBlock(b)
				}
				w.metrics.BlockbookBestHeight.Set(float64(b.Height))

				if b.Height > 0 && b.Height%1000 == 0 {
					glog.Info("connected block ", b.Height, " ", b.Hash)
				}

				lastBlock = b.Height
			case <-terminating:
				break WriteBlockLoop
			}
		}
		if err != nil {
			glog.Error("sync: ParallelConnectBlocks.Close error ", err)
		}
		glog.Info("WriteBlock exiting...")
	}
	for i := 0; i < int(syncWorkers); i++ {
		wg.Add(1)
		go w.getBlockWorker(i, syncWorkers, &wg, hch, bch, &hchClosed, terminating, abortCh)
	}
	go writeBlockWorker()
	var hash string
ConnectLoop:
	for h := lower; h <= higher; {
		select {
		case abortErr := <-abortCh:
			if stdErrors.Is(abortErr, errResync) {
				glog.Warning("sync: parallel connect aborted, restarting sync")
			} else {
				glog.Error("sync: parallel connect aborted, worker error ", abortErr)
			}
			err = abortErr
			close(terminating)
			break ConnectLoop
		case <-w.chanOsSignal:
			glog.Info("connectBlocksParallel interrupted at height ", h)
			err = ErrOperationInterrupted
			// signal all workers to terminate their loops (error loops are interrupted below)
			close(terminating)
			break ConnectLoop
		default:
			hash, err = w.chain.GetBlockHash(h)
			if err != nil {
				glog.Error("GetBlockHash error ", err)
				w.metrics.IndexResyncErrors.With(common.Labels{"error": "failure"}).Inc()
				time.Sleep(time.Millisecond * 500)
				continue
			}
			if err = w.sendHashHeight(hch, abortCh, hashHeight{hash, h}); err != nil {
				if stdErrors.Is(err, errResync) {
					glog.Warning("sync: parallel connect aborted while queueing block hash, restarting sync")
				} else if stdErrors.Is(err, ErrOperationInterrupted) {
					glog.Info("connectBlocksParallel interrupted at height ", h)
				} else {
					glog.Error("sync: parallel connect aborted while queueing block hash, worker error ", err)
				}
				close(terminating)
				break ConnectLoop
			}
			h++
		}
	}
	close(hch)
	// signal stop to workers that are in a error loop
	hchClosed.Store(true)
	// wait for workers and close bch that will stop writer loop
	wg.Wait()
	// Hardening: a worker can report a terminal tail error after ConnectLoop has
	// already ended (for example once hchClosed=true). Drain once so we return
	// that error instead of silently succeeding.
	select {
	case abortErr := <-abortCh:
		if err == nil {
			err = abortErr
		}
	default:
	}
	for i := 0; i < int(syncWorkers); i++ {
		close(bch[i])
	}
	<-writeBlockDone
	return err
}

func (w *SyncWorker) getBlockWorker(i int, syncWorkers uint32, wg *sync.WaitGroup, hch chan hashHeight, bch []chan *bchain.Block, hchClosed *atomic.Value, terminating chan struct{}, abortCh chan error) {
	defer wg.Done()
	var err error
	var block *bchain.Block
	cfg := w.missingBlockRetry
	label := "getBlockWorker " + strconv.Itoa(i)
	const checkErrStreakLimit = 3
GetBlockLoop:
	for hh := range hch {
		// Track consecutive retryable errors per block so we only re-check the
		// chain once the backend has had a chance to catch up.
		retries := 0
		checkErrStreak := 0
		loopStart := time.Now()
		for {
			// Allow global shutdown or an abort to stop the retry loop promptly.
			select {
			case <-terminating:
				return
			case <-w.chanOsSignal:
				return
			default:
			}
			block, err = w.chain.GetBlock(hh.hash, hh.height)
			if err != nil {
				if stdErrors.Is(err, bchain.ErrBlockNotFound) {
					w.metrics.IndexBlockNotFoundRetries.Inc()
				}
				if isRetryableGetBlockError(err) {
					threshold := cfg.RecheckThreshold
					// Once the hash queue is closed we are at the tail of the range; use
					// a smaller threshold to avoid stalling on a missing tip block.
					if hchClosed.Load() == true {
						threshold = cfg.TipRecheckThreshold
					}
					restart, checkErr := w.onRetryableMiss(&retries, threshold, label, hh.height, hh.hash, err)
					if checkErr != nil {
						checkErrStreak++
						if checkErrStreak == 1 {
							glog.Warningf("%s: chain-state probe failed for block %d %s (last: %v); will abort after %d consecutive failures",
								label, hh.height, hh.hash, checkErr, checkErrStreakLimit)
						}
						if checkErrStreak >= checkErrStreakLimit {
							// Backend cannot answer chain-state probes either; surface so the
							// outer loop can decide how to recover instead of spinning silently.
							glog.Errorf("%s: aborting after %d consecutive chain-state probe failures (last: %v)",
								label, checkErrStreak, checkErr)
							w.metrics.IndexSyncYields.With(common.Labels{"reason": "probe_failed"}).Inc()
							select {
							case abortCh <- checkErr:
							default:
							}
							return
						}
					} else {
						checkErrStreak = 0
						if restart {
							// The block hash at this height no longer exists; restart sync to realign.
							glog.Warning("sync: block ", hh.height, " ", hh.hash, " no longer on chain, restarting sync")
							w.metrics.IndexReorgEvents.With(common.Labels{"type": "resync"}).Inc()
							select {
							case abortCh <- errResync:
							default:
							}
							return
						}
					}
					if cfg.MaxStallDuration > 0 && time.Since(loopStart) >= cfg.MaxStallDuration {
						glog.Warningf("%s: block %d %s stall deadline %s exceeded after %d retries (last: %v); yielding to resync",
							label, hh.height, hh.hash, cfg.MaxStallDuration, retries, err)
						w.metrics.IndexSyncYields.With(common.Labels{"reason": "deadline"}).Inc()
						select {
						case abortCh <- errResync:
						default:
						}
						return
					}
				} else {
					// When the hash queue is closed, stop retrying non-retryable errors.
					if hchClosed.Load() == true {
						glog.Error("getBlockWorker ", i, " connect block error ", err, ". Exiting...")
						// Hardening: without surfacing this tail failure, the worker could
						// exit and leave the sync loop stuck until manual restart.
						select {
						case abortCh <- err:
						default:
						}
						return
					}
					retries = 0
					checkErrStreak = 0
					loopStart = time.Now()
					glog.Error("getBlockWorker ", i, " connect block error ", err, ". Retrying...")
				}
				w.metrics.IndexResyncErrors.With(common.Labels{"error": "failure"}).Inc()
				select {
				case <-terminating:
					return
				case <-w.chanOsSignal:
					return
				case <-time.After(cfg.RetryDelay):
				}
			} else {
				break
			}
		}
		if w.dryRun {
			continue
		}
		select {
		case bch[hh.height%syncWorkers] <- block:
		case <-terminating:
			break GetBlockLoop
		}
	}
	glog.Info("getBlockWorker ", i, " exiting...")
}

// BulkConnectBlocks uses parallel goroutines to get data from blockchain daemon
func (w *SyncWorker) BulkConnectBlocks(lower, higher uint32) error {
	var err error
	var wg sync.WaitGroup
	bch := make([]chan *bchain.Block, w.syncWorkers)
	for i := 0; i < w.syncWorkers; i++ {
		bch[i] = make(chan *bchain.Block)
	}
	hch := make(chan hashHeight, w.syncWorkers)
	hchClosed := atomic.Value{}
	hchClosed.Store(false)
	writeBlockDone := make(chan struct{})
	terminating := make(chan struct{})
	// abortCh is used by workers to signal a resync-worthy reorg or a terminal worker error.
	// Keep it buffered so the first worker can report without blocking while the
	// coordinator is closing channels/terminating.
	abortCh := make(chan error, 1)
	writeBlockWorker := func() {
		defer close(writeBlockDone)
		bc, err := w.db.InitBulkConnect()
		if err != nil {
			glog.Error("sync: InitBulkConnect error ", err)
		}
		lastBlock := lower - 1
		keep := uint32(w.chain.GetChainParser().KeepBlockAddresses())
	WriteBlockLoop:
		for {
			select {
			case b := <-bch[(lastBlock+1)%uint32(w.syncWorkers)]:
				if b == nil {
					// channel is closed and empty - work is done
					break WriteBlockLoop
				}
				if b.Height != lastBlock+1 {
					glog.Fatal("writeBlockWorker skipped block, expected block ", lastBlock+1, ", new block ", b.Height)
				}
				err := bc.ConnectBlock(b, b.Height+keep > higher)
				if err != nil {
					glog.Fatal("writeBlockWorker ", b.Height, " ", b.Hash, " error ", err)
				}
				lastBlock = b.Height
			case <-terminating:
				break WriteBlockLoop
			}
		}
		err = bc.Close()
		if err != nil {
			glog.Error("sync: bulkconnect.Close error ", err)
		}
		glog.Info("WriteBlock exiting...")
	}
	for i := 0; i < w.syncWorkers; i++ {
		wg.Add(1)
		go w.getBlockWorker(i, uint32(w.syncWorkers), &wg, hch, bch, &hchClosed, terminating, abortCh)
	}
	go writeBlockWorker()
	var hash string
	start := time.Now()
	msTime := time.Now().Add(1 * time.Minute)
ConnectLoop:
	for h := lower; h <= higher; {
		select {
		case abortErr := <-abortCh:
			if stdErrors.Is(abortErr, errResync) {
				// Another worker observed a missing block that no longer matches the chain.
				glog.Warning("sync: bulk connect aborted, restarting sync")
			} else {
				glog.Error("sync: bulk connect aborted, worker error ", abortErr)
			}
			err = abortErr
			close(terminating)
			break ConnectLoop
		case <-w.chanOsSignal:
			glog.Info("BulkConnectBlocks interrupted at height ", h)
			err = ErrOperationInterrupted
			// signal all workers to terminate their loops (error loops are interrupted below)
			close(terminating)
			break ConnectLoop
		default:
			hash, err = w.chain.GetBlockHash(h)
			if err != nil {
				glog.Error("GetBlockHash error ", err)
				w.metrics.IndexResyncErrors.With(common.Labels{"error": "failure"}).Inc()
				time.Sleep(time.Millisecond * 500)
				continue
			}
			if err = w.sendHashHeight(hch, abortCh, hashHeight{hash, h}); err != nil {
				if stdErrors.Is(err, errResync) {
					glog.Warning("sync: bulk connect aborted while queueing block hash, restarting sync")
				} else if stdErrors.Is(err, ErrOperationInterrupted) {
					glog.Info("BulkConnectBlocks interrupted at height ", h)
				} else {
					glog.Error("sync: bulk connect aborted while queueing block hash, worker error ", err)
				}
				close(terminating)
				break ConnectLoop
			}
			if h > 0 && h%1000 == 0 {
				w.metrics.BlockbookBestHeight.Set(float64(h))
				glog.Info("connecting block ", h, " ", hash, ", elapsed ", time.Since(start), " ", w.db.GetAndResetConnectBlockStats())
				start = time.Now()
			}
			if msTime.Before(time.Now()) {
				if glog.V(1) {
					glog.Info(w.db.GetMemoryStats())
				}
				w.metrics.IndexDBSize.Set(float64(w.db.DatabaseSizeOnDisk()))
				msTime = time.Now().Add(10 * time.Minute)
			}
			h++
		}
	}
	close(hch)
	// signal stop to workers that are in a error loop
	hchClosed.Store(true)
	// wait for workers and close bch that will stop writer loop
	wg.Wait()
	// Hardening: capture a late worker error reported after the connect loop
	// exits so the caller can retry instead of treating sync as successful.
	select {
	case abortErr := <-abortCh:
		if err == nil {
			err = abortErr
		}
	default:
	}
	for i := 0; i < w.syncWorkers; i++ {
		close(bch[i])
	}
	<-writeBlockDone
	return err
}

type blockResult struct {
	block *bchain.Block
	err   error
}

func (w *SyncWorker) getBlockChain(out chan blockResult, done chan struct{}) {
	defer close(out)

	hash := w.startHash
	height := w.startHeight
	prevHash := ""
	cfg := w.missingBlockRetry
	retryDelay := cfg.RetryDelay
	if retryDelay <= 0 || retryDelay > 250*time.Millisecond {
		retryDelay = 250 * time.Millisecond
	}
	recheckThreshold := cfg.TipRecheckThreshold
	if recheckThreshold <= 0 {
		recheckThreshold = 1
	}
	maxStall := cfg.MaxStallDuration
	// loop until error ErrBlockNotFound
	for {
		select {
		case <-done:
			return
		case <-w.chanOsSignal:
			return
		default:
		}
		retries := 0
		loopStart := time.Now()
		var block *bchain.Block
		var err error
		for {
			block, err = w.chain.GetBlock(hash, height)
			if err == nil {
				break
			}
			// On the first ErrBlockNotFound, check whether we are past the backend tip
			// so we exit cleanly at end-of-chain. Subsequent retries skip this RPC and
			// defer to shouldRestartSyncOnMissingBlock at the threshold tick.
			gotNotFound := stdErrors.Is(err, bchain.ErrBlockNotFound)
			if retries == 0 && gotNotFound {
				bestHeight, bestErr := w.chain.GetBestBlockHeight()
				if bestErr != nil {
					out <- blockResult{err: bestErr}
					return
				}
				if height > bestHeight {
					if hash == "" {
						return
					}
					glog.Warningf("getBlockChain: block %d %s is above observed backend height %d; retrying because the block hash was already observed", height, hash, bestHeight)
				}
			}
			if gotNotFound {
				w.metrics.IndexBlockNotFoundRetries.Inc()
			}
			if !isRetryableGetBlockError(err) {
				out <- blockResult{err: err}
				return
			}
			resync, checkErr := w.onRetryableMiss(&retries, recheckThreshold, "getBlockChain", height, hash, err)
			if checkErr != nil {
				out <- blockResult{err: checkErr}
				return
			}
			if resync {
				w.metrics.IndexReorgEvents.With(common.Labels{"type": "resync"}).Inc()
				out <- blockResult{err: errResync}
				return
			}
			if maxStall > 0 && time.Since(loopStart) >= maxStall {
				glog.Warningf("getBlockChain: block %d %s stall deadline %s exceeded after %d retries (last: %v); yielding to resync",
					height, hash, maxStall, retries, err)
				w.metrics.IndexSyncYields.With(common.Labels{"reason": "deadline"}).Inc()
				select {
				case out <- blockResult{err: errResync}:
				case <-done:
				case <-w.chanOsSignal:
				}
				return
			}
			w.metrics.IndexResyncErrors.With(common.Labels{"error": "failure"}).Inc()
			select {
			case <-done:
				return
			case <-w.chanOsSignal:
				return
			case <-time.After(retryDelay):
			}
		}
		if block.Prev != "" && prevHash != "" && prevHash != block.Prev {
			glog.Infof("sync: fork detected at height %d %s, local prevHash %s, remote prevHash %s", height, block.Hash, prevHash, block.Prev)
			w.metrics.IndexReorgEvents.With(common.Labels{"type": "fork"}).Inc()
			out <- blockResult{err: errFork}
			return
		}
		prevHash = block.Hash
		hash = block.Next
		height++
		out <- blockResult{block: block}
	}
}

// DisconnectBlocks removes all data belonging to blocks in range lower-higher,
func (w *SyncWorker) DisconnectBlocks(lower uint32, higher uint32, hashes []string) error {
	glog.Infof("sync: disconnecting blocks %d-%d", lower, higher)
	ct := w.chain.GetChainParser().GetChainType()
	if ct == bchain.ChainBitcoinType {
		return w.db.DisconnectBlockRangeBitcoinType(lower, higher)
	} else if ct == bchain.ChainEthereumType {
		return w.db.DisconnectBlockRangeEthereumType(lower, higher)
	}
	return errors.New("Unknown chain type")
}
