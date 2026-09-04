//go:build unittest

package db

import (
	"context"
	stdErrors "errors"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	jujuErrors "github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/common"
)

var (
	testMetricsOnce sync.Once
	testMetrics     *common.Metrics
	testMetricsErr  error
)

func getTestMetrics(t *testing.T) *common.Metrics {
	testMetricsOnce.Do(func() {
		testMetrics, testMetricsErr = common.GetMetrics("test")
	})
	if testMetricsErr != nil {
		t.Fatalf("GetMetrics: %v", testMetricsErr)
	}
	return testMetrics
}

func TestIsRetryableGetBlockError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil",
			err:  nil,
			want: false,
		},
		{
			name: "block not found",
			err:  bchain.ErrBlockNotFound,
			want: true,
		},
		{
			name: "deadline exceeded",
			err:  context.DeadlineExceeded,
			want: true,
		},
		{
			name: "unexpected EOF",
			err:  io.ErrUnexpectedEOF,
			want: true,
		},
		{
			name: "EOF",
			err:  io.EOF,
			want: true,
		},
		{
			name: "annotated deadline exceeded",
			err:  jujuErrors.Annotatef(context.DeadlineExceeded, "eth_getLogs blockNumber %v", "0x1"),
			want: true,
		},
		{
			name: "annotated unexpected EOF",
			err:  jujuErrors.Annotatef(io.ErrUnexpectedEOF, "eth_getLogs blockNumber %v", "0x1"),
			want: true,
		},
		{
			name: "network timeout",
			err: &net.DNSError{
				Err:       "i/o timeout",
				Name:      "example.org",
				IsTimeout: true,
			},
			want: true,
		},
		{
			name: "connection reset by peer",
			err: &url.Error{
				Op:  "Post",
				URL: "http://127.0.0.1:8545",
				Err: syscall.ECONNRESET,
			},
			want: true,
		},
		{
			name: "connection refused",
			err: &url.Error{
				Op:  "Post",
				URL: "http://127.0.0.1:8545",
				Err: syscall.ECONNREFUSED,
			},
			want: true,
		},
		{
			name: "rpc 503",
			err:  stdErrors.New("503 Service Unavailable: backend overloaded"),
			want: true,
		},
		{
			name: "rpc 429",
			err:  stdErrors.New("429 Too Many Requests"),
			want: true,
		},
		{
			name: "header not found",
			err:  stdErrors.New("header not found"),
			want: true,
		},
		{
			name: "other error",
			err:  stdErrors.New("boom"),
			want: false,
		},
		{
			name: "annotated other error",
			err:  jujuErrors.Annotatef(stdErrors.New("boom"), "eth_getLogs blockNumber %v", "0x1"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableGetBlockError(tt.err)
			if got != tt.want {
				t.Fatalf("isRetryableGetBlockError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestConnectBlocksHonorsClosedShutdownBeforeStart(t *testing.T) {
	for i := 0; i < 100; i++ {
		ch := make(chan os.Signal)
		close(ch)

		w := &SyncWorker{
			chanOsSignal: ch,
		}

		if err := w.connectBlocks(nil, false); !stdErrors.Is(err, ErrOperationInterrupted) {
			t.Fatalf("connectBlocks error = %v, want %v", err, ErrOperationInterrupted)
		}
	}
}

type getBlockChainTestChain struct {
	bchain.BlockChain
	chainType         bchain.ChainType // zero value keeps existing tests on the bitcoin-type path
	bestHeight        uint32
	bestHash          string
	tip               *bchain.EVMTip // nil: snapshot is derived from bestHash/bestHeight with no parent
	bestHeightErr     error
	bestHeightCalls   int
	hashes            map[uint32]string
	blocks            map[uint32]*bchain.Block
	blockErrors       map[uint32][]error
	getBlockCalls     map[uint32]int
	getBlockHashCalls map[uint32]int
	getBlockHashErr   error
}

type chainTypeTestParser struct {
	bchain.BlockChainParser
	chainType bchain.ChainType
}

func (p *chainTypeTestParser) GetChainType() bchain.ChainType {
	return p.chainType
}

func (c *getBlockChainTestChain) GetChainParser() bchain.BlockChainParser {
	return &chainTypeTestParser{chainType: c.chainType}
}

func (c *getBlockChainTestChain) GetBestBlockHash() (string, error) {
	return c.bestHash, nil
}

func (c *getBlockChainTestChain) EthereumTypeGetBestTip() (*bchain.EVMTip, error) {
	if c.tip != nil {
		return c.tip, nil
	}
	return &bchain.EVMTip{Hash: c.bestHash, Height: c.bestHeight}, nil
}

func (c *getBlockChainTestChain) GetBestBlockHeight() (uint32, error) {
	c.bestHeightCalls++
	if c.bestHeightErr != nil {
		return 0, c.bestHeightErr
	}
	return c.bestHeight, nil
}

func (c *getBlockChainTestChain) GetBlockHash(height uint32) (string, error) {
	if c.getBlockHashCalls != nil {
		c.getBlockHashCalls[height]++
	}
	if c.getBlockHashErr != nil {
		return "", c.getBlockHashErr
	}
	if hash, ok := c.hashes[height]; ok {
		return hash, nil
	}
	return "", bchain.ErrBlockNotFound
}

func (c *getBlockChainTestChain) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	c.getBlockCalls[height]++
	if errs := c.blockErrors[height]; len(errs) > 0 {
		err := errs[0]
		c.blockErrors[height] = errs[1:]
		return nil, err
	}
	if block := c.blocks[height]; block != nil {
		copy := *block
		return &copy, nil
	}
	return nil, bchain.ErrBlockNotFound
}

func newGetBlockChainTestWorker(t *testing.T, chain *getBlockChainTestChain, startHash string, startHeight uint32) *SyncWorker {
	return &SyncWorker{
		chain:       chain,
		startHash:   startHash,
		startHeight: startHeight,
		missingBlockRetry: MissingBlockRetryConfig{
			TipRecheckThreshold: 2,
			RetryDelay:          time.Millisecond,
		},
		metrics: getTestMetrics(t),
	}
}

func runGetBlockChain(w *SyncWorker) []blockResult {
	out := make(chan blockResult)
	done := make(chan struct{})
	go w.getBlockChain(out, done)
	var results []blockResult
	for res := range out {
		results = append(results, res)
	}
	return results
}

func TestGetBlockChainRetriesSequentialTipBlock(t *testing.T) {
	chain := &getBlockChainTestChain{
		bestHeight: 1,
		hashes:     map[uint32]string{1: "h1"},
		blocks: map[uint32]*bchain.Block{
			1: {BlockHeader: bchain.BlockHeader{Hash: "h1", Height: 1}},
		},
		blockErrors: map[uint32][]error{
			1: {bchain.ErrBlockNotFound, bchain.ErrBlockNotFound},
		},
		getBlockCalls: map[uint32]int{},
	}
	w := newGetBlockChainTestWorker(t, chain, "h1", 1)

	results := runGetBlockChain(w)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].err != nil {
		t.Fatalf("unexpected error: %v", results[0].err)
	}
	if results[0].block == nil || results[0].block.Hash != "h1" {
		t.Fatalf("unexpected block: %+v", results[0].block)
	}
	if calls := chain.getBlockCalls[1]; calls != 3 {
		t.Fatalf("GetBlock height 1 calls = %d, want 3", calls)
	}
}

func TestGetBlockChainStopsAboveBestHeight(t *testing.T) {
	chain := &getBlockChainTestChain{
		bestHeight:    0,
		hashes:        map[uint32]string{},
		blocks:        map[uint32]*bchain.Block{},
		blockErrors:   map[uint32][]error{},
		getBlockCalls: map[uint32]int{},
	}
	w := newGetBlockChainTestWorker(t, chain, "", 1)

	results := runGetBlockChain(w)
	if len(results) != 0 {
		t.Fatalf("got %d results, want 0: %+v", len(results), results)
	}
	if calls := chain.getBlockCalls[1]; calls != 1 {
		t.Fatalf("GetBlock height 1 calls = %d, want 1", calls)
	}
}

func TestGetBlockChainRetriesKnownHashAboveObservedBestHeight(t *testing.T) {
	chain := &getBlockChainTestChain{
		bestHeight: 0,
		hashes:     map[uint32]string{1: "h1"},
		blocks: map[uint32]*bchain.Block{
			1: {BlockHeader: bchain.BlockHeader{Hash: "h1", Height: 1}},
		},
		blockErrors: map[uint32][]error{
			1: {bchain.ErrBlockNotFound},
		},
		getBlockCalls: map[uint32]int{},
	}
	w := newGetBlockChainTestWorker(t, chain, "h1", 1)

	results := runGetBlockChain(w)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].err != nil {
		t.Fatalf("unexpected error: %v", results[0].err)
	}
	if results[0].block == nil || results[0].block.Hash != "h1" {
		t.Fatalf("unexpected block: %+v", results[0].block)
	}
	if calls := chain.getBlockCalls[1]; calls != 2 {
		t.Fatalf("GetBlock height 1 calls = %d, want 2", calls)
	}
}

func TestGetBlockChainEthereumTypeSkipsTailProbe(t *testing.T) {
	chain := &getBlockChainTestChain{
		chainType:     bchain.ChainEthereumType,
		bestHeight:    0,
		hashes:        map[uint32]string{},
		blocks:        map[uint32]*bchain.Block{},
		blockErrors:   map[uint32][]error{},
		getBlockCalls: map[uint32]int{},
	}
	w := newGetBlockChainTestWorker(t, chain, "", 1)

	results := runGetBlockChain(w)
	if len(results) != 0 {
		t.Fatalf("got %d results, want 0: %+v", len(results), results)
	}
	if calls := chain.getBlockCalls[1]; calls != 0 {
		t.Fatalf("GetBlock height 1 calls = %d, want 0 (probe above cached tip must be skipped)", calls)
	}
	if chain.bestHeightCalls != 1 {
		t.Fatalf("GetBestBlockHeight calls = %d, want 1", chain.bestHeightCalls)
	}
}

func TestGetBlockChainEthereumTypeRetriesKnownHashAboveObservedBestHeight(t *testing.T) {
	chain := &getBlockChainTestChain{
		chainType:  bchain.ChainEthereumType,
		bestHeight: 0,
		hashes:     map[uint32]string{1: "h1"},
		blocks: map[uint32]*bchain.Block{
			1: {BlockHeader: bchain.BlockHeader{Hash: "h1", Height: 1}},
		},
		blockErrors: map[uint32][]error{
			1: {bchain.ErrBlockNotFound},
		},
		getBlockCalls: map[uint32]int{},
	}
	w := newGetBlockChainTestWorker(t, chain, "h1", 1)

	results := runGetBlockChain(w)
	if len(results) != 1 || results[0].err != nil || results[0].block == nil || results[0].block.Hash != "h1" {
		t.Fatalf("unexpected results: %+v", results)
	}
	if calls := chain.getBlockCalls[1]; calls != 2 {
		t.Fatalf("GetBlock height 1 calls = %d, want 2 (known hash above tip is still retried)", calls)
	}
}

func TestGetBlockChainEthereumTypeRetriesGenuineMissBelowTip(t *testing.T) {
	chain := &getBlockChainTestChain{
		chainType:  bchain.ChainEthereumType,
		bestHeight: 2,
		hashes:     map[uint32]string{1: "h1", 2: "h2"},
		blocks: map[uint32]*bchain.Block{
			1: {BlockHeader: bchain.BlockHeader{Hash: "h1", Height: 1}},
			2: {BlockHeader: bchain.BlockHeader{Hash: "h2", Prev: "h1", Height: 2}},
		},
		blockErrors: map[uint32][]error{
			2: {bchain.ErrBlockNotFound},
		},
		getBlockCalls: map[uint32]int{},
	}
	w := newGetBlockChainTestWorker(t, chain, "h1", 1)

	results := runGetBlockChain(w)
	if len(results) != 2 || results[0].err != nil || results[1].err != nil {
		t.Fatalf("unexpected results: %+v", results)
	}
	if results[1].block == nil || results[1].block.Hash != "h2" {
		t.Fatalf("unexpected second block: %+v", results[1].block)
	}
	if calls := chain.getBlockCalls[2]; calls != 2 {
		t.Fatalf("GetBlock height 2 calls = %d, want 2 (miss at or below tip keeps retrying)", calls)
	}
	if calls := chain.getBlockCalls[3]; calls != 0 {
		t.Fatalf("GetBlock height 3 calls = %d, want 0", calls)
	}
}

func TestStartHashForHeight(t *testing.T) {
	tests := []struct {
		name      string
		tipCached bool
		tipHeight uint32
		wantHash  string
		wantCalls int
	}{
		{name: "cached tip is the start block", tipCached: true, tipHeight: 5, wantHash: "tip", wantCalls: 0},
		{name: "cached tip ahead of start block", tipCached: true, tipHeight: 6, wantHash: "h5", wantCalls: 1},
		{name: "tip not cached", tipCached: false, tipHeight: 5, wantHash: "h5", wantCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain := &getBlockChainTestChain{
				hashes:            map[uint32]string{5: "h5"},
				getBlockHashCalls: map[uint32]int{},
			}
			w := &SyncWorker{chain: chain}
			got, err := w.startHashForHeight(5, "tip", tt.tipHeight, tt.tipCached)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.wantHash {
				t.Fatalf("startHash = %q, want %q", got, tt.wantHash)
			}
			if calls := chain.getBlockHashCalls[5]; calls != tt.wantCalls {
				t.Fatalf("GetBlockHash calls = %d, want %d", calls, tt.wantCalls)
			}
		})
	}
}

// evmTestHash builds a valid 32-byte hex block hash from a one-byte pattern.
func evmTestHash(b string) string {
	return "0x" + strings.Repeat(b, 32)
}

func evmTestBlock(hash, prev string, height uint32) *bchain.Block {
	return &bchain.Block{BlockHeader: bchain.BlockHeader{Hash: hash, Prev: prev, Height: height, Time: int64(height)}}
}

func newResyncTestChain(tip *bchain.EVMTip, blocks ...*bchain.Block) *getBlockChainTestChain {
	c := &getBlockChainTestChain{
		chainType:         bchain.ChainEthereumType,
		tip:               tip,
		hashes:            map[uint32]string{},
		blocks:            map[uint32]*bchain.Block{},
		blockErrors:       map[uint32][]error{},
		getBlockCalls:     map[uint32]int{},
		getBlockHashCalls: map[uint32]int{},
	}
	for _, b := range blocks {
		c.hashes[b.Height] = b.Hash
		c.blocks[b.Height] = b
		if b.Height > c.bestHeight {
			c.bestHeight, c.bestHash = b.Height, b.Hash
		}
	}
	return c
}

func newResyncTestWorker(t *testing.T, d *RocksDB, chain *getBlockChainTestChain) *SyncWorker {
	return &SyncWorker{
		db:           d,
		chain:        chain,
		syncWorkers:  1,
		chanOsSignal: make(chan os.Signal),
		metrics:      getTestMetrics(t),
		missingBlockRetry: MissingBlockRetryConfig{
			TipRecheckThreshold: 2,
			RetryDelay:          time.Millisecond,
		},
	}
}

func assertBestBlock(t *testing.T, d *RocksDB, wantHeight uint32, wantHash string) {
	t.Helper()
	height, hash, err := d.GetBestBlock()
	if err != nil {
		t.Fatalf("GetBestBlock: %v", err)
	}
	if height != wantHeight || hash != wantHash {
		t.Fatalf("best block = %d %s, want %d %s", height, hash, wantHeight, wantHash)
	}
}

func assertCalls(t *testing.T, what string, calls map[uint32]int, height uint32, want int) {
	t.Helper()
	if got := calls[height]; got != want {
		t.Fatalf("%s(%d) calls = %d, want %d", what, height, got, want)
	}
}

// One steady-state EVM cycle with a tip snapshot that carries no parent hash: the fork check
// is the only header lookup, the tip hash comes from the cache and the tail probe is not sent.
func TestResyncIndexEthereumTypeUsesCachedTipHash(t *testing.T) {
	d := setupRocksDB(t, eth.NewEthereumParser(1, false))
	defer closeAndDestroyRocksDB(t, d)

	h1, h2 := evmTestHash("11"), evmTestHash("22")
	block1, block2 := evmTestBlock(h1, "", 1), evmTestBlock(h2, h1, 2)
	if err := d.ConnectBlock(block1); err != nil {
		t.Fatalf("ConnectBlock: %v", err)
	}
	chain := newResyncTestChain(nil, block1, block2)
	w := newResyncTestWorker(t, d, chain)

	if err := w.resyncIndex(nil, false); err != nil {
		t.Fatalf("resyncIndex: %v", err)
	}
	assertBestBlock(t, d, 2, h2)
	assertCalls(t, "GetBlockHash", chain.getBlockHashCalls, 1, 1) // fork check kept without parent linkage
	assertCalls(t, "GetBlockHash", chain.getBlockHashCalls, 2, 0) // tip hash from the cached header
	assertCalls(t, "GetBlock", chain.getBlockCalls, 2, 1)
	assertCalls(t, "GetBlock", chain.getBlockCalls, 3, 0) // tail probe skipped
}

// The pushed tip links to the local best block: no header lookup at all, only the block fetch.
func TestResyncIndexEthereumTypeLinkedTipSkipsForkCheck(t *testing.T) {
	d := setupRocksDB(t, eth.NewEthereumParser(1, false))
	defer closeAndDestroyRocksDB(t, d)

	h1, h2 := evmTestHash("11"), evmTestHash("22")
	block1, block2 := evmTestBlock(h1, "", 1), evmTestBlock(h2, h1, 2)
	if err := d.ConnectBlock(block1); err != nil {
		t.Fatalf("ConnectBlock: %v", err)
	}
	chain := newResyncTestChain(&bchain.EVMTip{Hash: h2, ParentHash: h1, Height: 2}, block1, block2)
	w := newResyncTestWorker(t, d, chain)

	if err := w.resyncIndex(nil, false); err != nil {
		t.Fatalf("resyncIndex: %v", err)
	}
	assertBestBlock(t, d, 2, h2)
	assertCalls(t, "GetBlockHash", chain.getBlockHashCalls, 1, 0)
	assertCalls(t, "GetBlockHash", chain.getBlockHashCalls, 2, 0)
	assertCalls(t, "GetBlock", chain.getBlockCalls, 2, 1)
	assertCalls(t, "GetBlock", chain.getBlockCalls, 3, 0)
}

// A tip more than one block ahead cannot prove linkage, so both header lookups stay.
func TestResyncIndexEthereumTypeGapAboveOneKeepsForkCheck(t *testing.T) {
	d := setupRocksDB(t, eth.NewEthereumParser(1, false))
	defer closeAndDestroyRocksDB(t, d)

	h1, h2, h3 := evmTestHash("11"), evmTestHash("22"), evmTestHash("33")
	block1, block2, block3 := evmTestBlock(h1, "", 1), evmTestBlock(h2, h1, 2), evmTestBlock(h3, h2, 3)
	if err := d.ConnectBlock(block1); err != nil {
		t.Fatalf("ConnectBlock: %v", err)
	}
	chain := newResyncTestChain(&bchain.EVMTip{Hash: h3, ParentHash: h2, Height: 3}, block1, block2, block3)
	w := newResyncTestWorker(t, d, chain)

	if err := w.resyncIndex(nil, false); err != nil {
		t.Fatalf("resyncIndex: %v", err)
	}
	assertBestBlock(t, d, 3, h3)
	assertCalls(t, "GetBlockHash", chain.getBlockHashCalls, 1, 1)
	assertCalls(t, "GetBlockHash", chain.getBlockHashCalls, 2, 1)
	assertCalls(t, "GetBlock", chain.getBlockCalls, 2, 1)
	assertCalls(t, "GetBlock", chain.getBlockCalls, 3, 1)
	assertCalls(t, "GetBlock", chain.getBlockCalls, 4, 0)
}

// The tip is one block ahead but its parent is not our best block: the fork check runs,
// the orphaned local block is disconnected and the canonical branch is connected.
func TestResyncIndexEthereumTypeParentMismatchHandlesFork(t *testing.T) {
	d := setupRocksDB(t, eth.NewEthereumParser(1, false))
	defer closeAndDestroyRocksDB(t, d)

	h1, h2, h2b, h3b := evmTestHash("11"), evmTestHash("22"), evmTestHash("bb"), evmTestHash("cc")
	block1, block2 := evmTestBlock(h1, "", 1), evmTestBlock(h2, h1, 2)
	block2b, block3b := evmTestBlock(h2b, h1, 2), evmTestBlock(h3b, h2b, 3)
	for _, b := range []*bchain.Block{block1, block2} {
		if err := d.ConnectBlock(b); err != nil {
			t.Fatalf("ConnectBlock: %v", err)
		}
	}
	chain := newResyncTestChain(&bchain.EVMTip{Hash: h3b, ParentHash: h2b, Height: 3}, block1, block2b, block3b)
	w := newResyncTestWorker(t, d, chain)

	if err := w.resyncIndex(nil, false); err != nil {
		t.Fatalf("resyncIndex: %v", err)
	}
	assertBestBlock(t, d, 3, h3b)
	if calls := chain.getBlockHashCalls[2]; calls == 0 {
		t.Fatal("GetBlockHash(2) was not called: fork check must run when the parent does not link")
	}
}

func TestGetBlockChainMissingBlockChangedHashResyncs(t *testing.T) {
	chain := &getBlockChainTestChain{
		bestHeight:    1,
		hashes:        map[uint32]string{1: "real-hash"},
		blocks:        map[uint32]*bchain.Block{},
		blockErrors:   map[uint32][]error{},
		getBlockCalls: map[uint32]int{},
	}
	w := newGetBlockChainTestWorker(t, chain, "fake-hash", 1)

	results := runGetBlockChain(w)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if !stdErrors.Is(results[0].err, errResync) {
		t.Fatalf("error = %v, want errResync", results[0].err)
	}
	if calls := chain.getBlockCalls[1]; calls != 2 {
		t.Fatalf("GetBlock height 1 calls = %d, want 2", calls)
	}
}

func TestShouldRestartSyncOnMissingBlockIgnoresLaggingBestHeight(t *testing.T) {
	chain := &getBlockChainTestChain{
		bestHeight: 9,
		hashes:     map[uint32]string{},
	}
	w := newGetBlockChainTestWorker(t, chain, "h10", 10)

	restart, err := w.shouldRestartSyncOnMissingBlock(10, "h10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if restart {
		t.Fatal("restart = true, want false for a single lagging best-height probe")
	}
}

func TestShouldRestartSyncOnMissingBlockIgnoresMissingHashProbe(t *testing.T) {
	chain := &getBlockChainTestChain{
		bestHeight: 10,
		hashes:     map[uint32]string{},
	}
	w := newGetBlockChainTestWorker(t, chain, "h10", 10)

	restart, err := w.shouldRestartSyncOnMissingBlock(10, "h10")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if restart {
		t.Fatal("restart = true, want false for a single missing hash probe")
	}
}

func TestGetBlockChainNonRetryableErrorReturns(t *testing.T) {
	boom := stdErrors.New("boom")
	chain := &getBlockChainTestChain{
		bestHeight: 1,
		hashes:     map[uint32]string{1: "h1"},
		blocks:     map[uint32]*bchain.Block{},
		blockErrors: map[uint32][]error{
			1: {boom},
		},
		getBlockCalls: map[uint32]int{},
	}
	w := newGetBlockChainTestWorker(t, chain, "h1", 1)

	results := runGetBlockChain(w)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if !stdErrors.Is(results[0].err, boom) {
		t.Fatalf("error = %v, want %v", results[0].err, boom)
	}
	if calls := chain.getBlockCalls[1]; calls != 1 {
		t.Fatalf("GetBlock height 1 calls = %d, want 1", calls)
	}
}

func TestGetBlockChainWallClockCap(t *testing.T) {
	// Block 1 exists on chain (so first ErrBlockNotFound does not short-circuit
	// to "above best height") but GetBlock never produces it. TipRecheckThreshold
	// is set high enough that the recheck path cannot fire before the cap.
	chain := &getBlockChainTestChain{
		bestHeight:    1,
		hashes:        map[uint32]string{1: "h1"},
		blocks:        map[uint32]*bchain.Block{},
		blockErrors:   map[uint32][]error{},
		getBlockCalls: map[uint32]int{},
	}
	w := &SyncWorker{
		chain:       chain,
		startHash:   "h1",
		startHeight: 1,
		missingBlockRetry: MissingBlockRetryConfig{
			TipRecheckThreshold: 1_000_000,
			RetryDelay:          time.Millisecond,
			MaxStallDuration:    50 * time.Millisecond,
		},
		metrics: getTestMetrics(t),
	}

	start := time.Now()
	results := runGetBlockChain(w)
	elapsed := time.Since(start)

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if !stdErrors.Is(results[0].err, errResync) {
		t.Fatalf("error = %v, want errResync", results[0].err)
	}
	if elapsed < 50*time.Millisecond {
		t.Fatalf("wall-clock cap returned in %v, expected at least 50ms", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("wall-clock cap took %v, expected to return shortly after 50ms", elapsed)
	}
	if calls := chain.getBlockCalls[1]; calls < 2 {
		t.Fatalf("GetBlock height 1 calls = %d, want at least 2", calls)
	}
}

func TestGetBlockWorkerCheckErrAbortsAfterStreak(t *testing.T) {
	// GetBlock keeps returning ErrBlockNotFound (retryable). GetBestBlockHeight
	// fails too, so onRetryableMiss returns (false, checkErr) on every call past
	// the threshold. After three consecutive checkErrs the worker must surface
	// the error via abortCh instead of spinning silently.
	probeErr := stdErrors.New("backend unreachable")
	chain := &getBlockChainTestChain{
		bestHeight:    1,
		bestHeightErr: probeErr,
		hashes:        map[uint32]string{1: "h1"},
		blocks:        map[uint32]*bchain.Block{},
		blockErrors:   map[uint32][]error{},
		getBlockCalls: map[uint32]int{},
	}
	w := &SyncWorker{
		chain: chain,
		missingBlockRetry: MissingBlockRetryConfig{
			RecheckThreshold:    1,
			TipRecheckThreshold: 1,
			RetryDelay:          time.Millisecond,
			MaxStallDuration:    10 * time.Second, // do not let the wall-clock cap fire first
		},
		metrics: getTestMetrics(t),
	}

	const workers = 1
	hch := make(chan hashHeight, workers)
	bch := make([]chan *bchain.Block, workers)
	for i := range bch {
		bch[i] = make(chan *bchain.Block, 1)
	}
	var hchClosed atomic.Value
	hchClosed.Store(true)
	terminating := make(chan struct{})
	abortCh := make(chan error, 1)
	hch <- hashHeight{hash: "h1", height: 1}
	close(hch)

	var wg sync.WaitGroup
	wg.Add(1)
	go w.getBlockWorker(0, workers, &wg, hch, bch, &hchClosed, terminating, abortCh)

	select {
	case err := <-abortCh:
		if !stdErrors.Is(err, probeErr) {
			t.Fatalf("abortCh got %v, want %v", err, probeErr)
		}
	case <-time.After(2 * time.Second):
		close(terminating)
		t.Fatalf("worker did not abort after consecutive checkErrs")
	}

	wg.Wait()
	if chain.bestHeightCalls < 3 {
		t.Fatalf("GetBestBlockHeight calls = %d, want at least 3", chain.bestHeightCalls)
	}
}

func TestParallelConnectBlocksReturnsWorkerAbortWhenHashQueueFull(t *testing.T) {
	hashes := make(map[uint32]string)
	for h := uint32(1); h <= 10; h++ {
		hashes[h] = "h" + strconv.Itoa(int(h))
	}
	chain := &getBlockChainTestChain{
		bestHeight:    10,
		hashes:        hashes,
		blocks:        map[uint32]*bchain.Block{},
		blockErrors:   map[uint32][]error{},
		getBlockCalls: map[uint32]int{},
	}
	w := &SyncWorker{
		chain: chain,
		missingBlockRetry: MissingBlockRetryConfig{
			RecheckThreshold:    1,
			TipRecheckThreshold: 1,
			RetryDelay:          time.Millisecond,
			MaxStallDuration:    30 * time.Millisecond,
		},
		metrics: getTestMetrics(t),
	}

	done := make(chan error, 1)
	go func() {
		done <- w.ParallelConnectBlocks(nil, 1, 10, 1)
	}()

	select {
	case err := <-done:
		if !stdErrors.Is(err, errResync) {
			t.Fatalf("ParallelConnectBlocks error = %v, want errResync", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ParallelConnectBlocks did not return after worker abort")
	}
}

// MaxStallDuration is the load-bearing liveness cap: the retry loops disable the
// cap when it is <= 0, so construction must clamp it to a safe default regardless
// of which caller (or partial test cfg) supplied the config.
func TestNewSyncWorkerClampsMaxStallDuration(t *testing.T) {
	def := DefaultMissingBlockRetryConfig().MaxStallDuration
	cases := []struct {
		name string
		cfg  *SyncWorkerConfig
		want time.Duration
	}{
		{name: "nil cfg keeps default", cfg: nil, want: def},
		{
			name: "zero stall clamped to default",
			cfg:  &SyncWorkerConfig{MissingBlockRetry: MissingBlockRetryConfig{MaxStallDuration: 0}},
			want: def,
		},
		{
			name: "negative stall clamped to default",
			cfg:  &SyncWorkerConfig{MissingBlockRetry: MissingBlockRetryConfig{MaxStallDuration: -time.Second}},
			want: def,
		},
		{
			name: "explicit positive stall preserved",
			cfg:  &SyncWorkerConfig{MissingBlockRetry: MissingBlockRetryConfig{MaxStallDuration: 5 * time.Second}},
			want: 5 * time.Second,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, err := NewSyncWorkerWithConfig(nil, nil, 1, 0, 0, false, nil, getTestMetrics(t), nil, tc.cfg)
			if err != nil {
				t.Fatalf("NewSyncWorkerWithConfig: %v", err)
			}
			if got := w.missingBlockRetry.MaxStallDuration; got != tc.want {
				t.Fatalf("MaxStallDuration = %s, want %s", got, tc.want)
			}
		})
	}
}
