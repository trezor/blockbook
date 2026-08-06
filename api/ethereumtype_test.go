//go:build unittest

package api

import (
	"os"
	"testing"
	"time"

	"github.com/juju/errors"
	"github.com/linxGnu/grocksdb"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

// The delay between heals must not hold up shutdown - a pass over a large queue sleeps
// most of its wall-clock time, so the sleep has to give up as soon as stop closes.
func Test_sleepBetweenHeals_Shutdown(t *testing.T) {
	stop := make(chan os.Signal)
	close(stop)
	start := time.Now()
	if sleepBetweenHeals(stop) {
		t.Error("sleepBetweenHeals() = true on a closed stop channel, want false")
	}
	if elapsed := time.Since(start); elapsed >= healInterBlockDelay {
		t.Errorf("sleepBetweenHeals waited %v on a closed stop channel", elapsed)
	}
}

func Test_healDue(t *testing.T) {
	tests := []struct {
		name     string
		failures uint8
		pass     uint64
		want     bool
	}{
		// a restart resets the pass counter, so the whole queue gets one immediate attempt
		{name: "first pass attempts a fresh block", failures: 0, pass: 0, want: true},
		{name: "first pass attempts a long failing block", failures: 200, pass: 0, want: true},
		{name: "no failures is attempted every pass", failures: 0, pass: 7, want: true},
		{name: "one failure skips the next pass", failures: 1, pass: 1, want: false},
		{name: "one failure is attempted every second pass", failures: 1, pass: 2, want: true},
		{name: "three failures wait eight passes", failures: 3, pass: 4, want: false},
		{name: "three failures are attempted every eighth pass", failures: 3, pass: 8, want: true},
		// the shift saturates, so the slowest schedule stays at 64 passes
		{name: "the cap is attempted every 64th pass", failures: maxHealBackoffShift, pass: 64, want: true},
		{name: "above the cap keeps the capped schedule", failures: 200, pass: 64, want: true},
		{name: "above the cap is not attempted in between", failures: 200, pass: 65, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := healDue(tt.failures, tt.pass); got != tt.want {
				t.Errorf("healDue(%d, %d) = %v, want %v", tt.failures, tt.pass, got, tt.want)
			}
		})
	}
}

// healTestChain stubs only GetBlock - HealInternalData touches nothing else on the
// chain, and the embedded nil interface panics loudly if that ever changes.
type healTestChain struct {
	bchain.BlockChain
	calls    int
	getBlock func() (*bchain.Block, error)
}

func (c *healTestChain) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	c.calls++
	return c.getBlock()
}

// failedInternalDataBlock2 returns test block 2 the way a failed internal data fetch
// leaves it: no per-tx internal data, only the error marker and the log-derived aliases.
func failedInternalDataBlock2(parser bchain.BlockChainParser) *bchain.Block {
	block := dbtestdata.GetTestEthereumTypeBlock2(parser)
	for i := range block.Txs {
		csd, _ := block.Txs[i].CoinSpecificData.(bchain.EthereumSpecificData)
		csd.InternalData = nil
		block.Txs[i].CoinSpecificData = csd
	}
	block.CoinSpecificData = &bchain.EthereumBlockSpecificData{
		InternalDataError:   dbtestdata.Block2SpecificData.InternalDataError,
		AddressAliasRecords: dbtestdata.Block2SpecificData.AddressAliasRecords,
	}
	return block
}

// healedInternalDataBlock2 returns test block 2 as a successful refetch delivers it.
func healedInternalDataBlock2(parser bchain.BlockChainParser) *bchain.Block {
	block := dbtestdata.GetTestEthereumTypeBlock2(parser)
	block.CoinSpecificData = &bchain.EthereumBlockSpecificData{
		AddressAliasRecords: dbtestdata.Block2SpecificData.AddressAliasRecords,
		Contracts:           dbtestdata.Block2SpecificData.Contracts,
	}
	return block
}

func queuedInternalDataError(t *testing.T, d *db.RocksDB, height uint32) *db.BlockInternalDataError {
	t.Helper()
	queue, err := d.GetBlockInternalDataErrorsEthereumType()
	if err != nil {
		t.Fatal(err)
	}
	for i := range queue {
		if queue[i].Height == height {
			return &queue[i]
		}
	}
	return nil
}

// HealInternalData decides what a failure means - only a definitive answer about the
// block itself spends the backoff budget - and that rule lives nowhere else, so it is
// pinned here with a stubbed chain against a real database.
func TestWorker_HealInternalData(t *testing.T) {
	parser := eth.NewEthereumParser(1, true)
	d, err := db.NewRocksDB(t.TempDir(), 100000, -1, parser, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	is, err := d.LoadInternalState(&common.Config{CoinName: "coin-unittest"})
	if err != nil {
		t.Fatal(err)
	}
	d.SetInternalState(is)

	// GetMetrics registers in the global prometheus registry, so it can run only once
	// per test binary; this is currently the only api test that needs it
	metrics, err := common.GetMetrics("coin-unittest")
	if err != nil {
		t.Fatal(err)
	}
	chain := &healTestChain{}
	w := &Worker{db: d, chain: chain, metrics: metrics}
	stop := make(chan os.Signal)
	metricValue := func(m prometheus.Metric) float64 {
		var pb dto.Metric
		if err := m.Write(&pb); err != nil {
			t.Fatal(err)
		}
		if pb.Counter != nil {
			return pb.Counter.GetValue()
		}
		return pb.Gauge.GetValue()
	}
	heals := func(result string) float64 {
		return metricValue(metrics.InternalDataHeals.With(common.Labels{"result": result}))
	}
	assertOutcomes := func(stage string, healed, failed, fetchErr, queueGauge float64) {
		t.Helper()
		if got := heals("healed"); got != healed {
			t.Errorf("%s: healed = %v, want %v", stage, got, healed)
		}
		if got := heals("failed"); got != failed {
			t.Errorf("%s: failed = %v, want %v", stage, got, failed)
		}
		if got := heals("fetch_error"); got != fetchErr {
			t.Errorf("%s: fetch_error = %v, want %v", stage, got, fetchErr)
		}
		if got := heals("reconnect_error"); got != 0 {
			t.Errorf("%s: reconnect_error = %v, want 0", stage, got)
		}
		if got := metricValue(metrics.InternalDataErrorQueue); got != queueGauge {
			t.Errorf("%s: queue gauge = %v, want %v", stage, got, queueGauge)
		}
	}

	// sync: block 2 fails its internal data fetch and is queued
	if err := d.ConnectBlock(dbtestdata.GetTestEthereumTypeBlock1(parser)); err != nil {
		t.Fatal(err)
	}
	if err := d.ConnectBlock(failedInternalDataBlock2(parser)); err != nil {
		t.Fatal(err)
	}
	block2 := dbtestdata.GetTestEthereumTypeBlock2(parser)
	if entry := queuedInternalDataError(t, d, block2.Height); entry == nil {
		t.Fatal("block 2 was not queued by the failed sync")
	}

	// a transport error is backend-wide: retried once within the pass, left uncharged,
	// so an outage cannot back off the queue
	chain.getBlock = func() (*bchain.Block, error) {
		return nil, errors.New("Post \"http://backend\": context deadline exceeded")
	}
	w.HealInternalData(0, stop)
	if chain.calls != 2 {
		t.Errorf("transport error: %d fetches, want 2 (the warm-cache retry)", chain.calls)
	}
	entry := queuedInternalDataError(t, d, block2.Height)
	if entry == nil || entry.Retries != 0 {
		t.Errorf("transport error: entry = %+v, want retries 0 - outages must not be charged", entry)
	}
	assertOutcomes("transport error", 0, 0, 1, 1)

	// a backend without the block has answered definitively: charged, and the
	// warm-cache retry is skipped because it cannot help. The juju annotation mimics
	// what chain.GetBlock produces, exercising the Cause unwrapping.
	chain.calls = 0
	chain.getBlock = func() (*bchain.Block, error) {
		return nil, errors.Annotatef(bchain.ErrBlockNotFound, "hash %v, height %v", block2.Hash, block2.Height)
	}
	w.HealInternalData(0, stop)
	if chain.calls != 1 {
		t.Errorf("block not found: %d fetches, want 1 (no retry for a missing block)", chain.calls)
	}
	entry = queuedInternalDataError(t, d, block2.Height)
	if entry == nil || entry.Retries != 1 {
		t.Errorf("block not found: entry = %+v, want retries 1", entry)
	}
	assertOutcomes("block not found", 0, 1, 1, 1)

	// the trace failed again on the refetched block: about the block, charged
	chain.calls = 0
	chain.getBlock = func() (*bchain.Block, error) {
		return &bchain.Block{
			BlockHeader:      bchain.BlockHeader{Hash: block2.Hash, Height: block2.Height},
			CoinSpecificData: &bchain.EthereumBlockSpecificData{InternalDataError: "trace failed again"},
		}, nil
	}
	w.HealInternalData(2, stop) // retries 1 is due on even passes
	if chain.calls != 2 {
		t.Errorf("internal data error: %d fetches, want 2 (the warm-cache retry)", chain.calls)
	}
	entry = queuedInternalDataError(t, d, block2.Height)
	if entry == nil || entry.Retries != 2 || entry.ErrorMessage != "trace failed again" {
		t.Errorf("internal data error: entry = %+v, want retries 2 and the new message", entry)
	}
	assertOutcomes("internal data error", 0, 2, 1, 1)

	// the refetch succeeds: the block is reconnected and leaves the queue
	chain.calls = 0
	chain.getBlock = func() (*bchain.Block, error) {
		return healedInternalDataBlock2(parser), nil
	}
	w.HealInternalData(4, stop) // retries 2 is due on passes divisible by 4
	if entry = queuedInternalDataError(t, d, block2.Height); entry != nil {
		t.Errorf("heal: entry still queued: %+v", entry)
	}
	if internalData, err := d.GetEthereumInternalData(block2.Txs[0].Txid); err != nil || internalData == nil {
		t.Errorf("heal: internal data of %s not stored (%v)", block2.Txs[0].Txid, err)
	}
	assertOutcomes("heal", 1, 2, 1, 0)

	// a backed off block is not attempted before its pass is due
	wb := grocksdb.NewWriteBatch()
	defer wb.Destroy()
	if err := d.StoreBlockInternalDataErrorEthereumType(wb, &bchain.Block{
		BlockHeader: bchain.BlockHeader{Hash: block2.Hash, Height: block2.Height},
	}, "trace failed", 3); err != nil {
		t.Fatal(err)
	}
	if err := d.WriteBatch(wb); err != nil {
		t.Fatal(err)
	}
	chain.calls = 0
	w.HealInternalData(1, stop) // retries 3 is due only on passes divisible by 8
	if chain.calls != 0 {
		t.Errorf("backed off: %d fetches, want 0", chain.calls)
	}
	assertOutcomes("backed off", 1, 2, 1, 1)
}
