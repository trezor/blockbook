//go:build unittest

package server

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

func newTestSentTxTracker(t *testing.T) (*sentTxTracker, *common.Metrics, *time.Time) {
	t.Helper()
	m := &common.Metrics{
		SentTxPending:       prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_sent_tx_pending"}),
		SentTxPendingOldest: prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_sent_tx_pending_oldest"}),
		SentTxOutcome: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_sent_tx_outcome_total"}, []string{"result"}),
		SentTxInclusionSeconds: prometheus.NewHistogram(
			prometheus.HistogramOpts{Name: "test_sent_tx_inclusion_seconds", Buckets: []float64{10, 60, 600}}),
	}
	now := time.Unix(1_700_000_000, 0)
	tr := newSentTxTracker(m)
	tr.now = func() time.Time { return now }
	return tr, m, &now
}

func blockWith(txids ...string) *bchain.Block {
	b := &bchain.Block{}
	for _, id := range txids {
		b.Txs = append(b.Txs, bchain.Tx{Txid: id})
	}
	return b
}

func TestSentTxTrackerMinedAndPending(t *testing.T) {
	tr, m, now := newTestSentTxTracker(t)
	tr.track("a")
	*now = now.Add(30 * time.Second)
	tr.track("b")
	tr.track("b") // a repeated broadcast of the same txid is one pending send
	if got := gaugeValue(t, m.SentTxPending); got != 2 {
		t.Fatalf("pending = %v, want 2", got)
	}
	if got := gaugeValue(t, m.SentTxPendingOldest); got != float64(now.Add(-30*time.Second).Unix()) {
		t.Errorf("oldest timestamp = %v, want the broadcast time of a", got)
	}

	*now = now.Add(60 * time.Second)
	tr.OnNewBlock(blockWith("x", "a", "y"))
	if got := gaugeValue(t, m.SentTxPending); got != 1 {
		t.Errorf("pending after a mined = %v, want 1", got)
	}
	if got := gaugeValue(t, m.SentTxPendingOldest); got != float64(now.Add(-60*time.Second).Unix()) {
		t.Errorf("oldest timestamp after a mined = %v, want the broadcast time of b", got)
	}
	if got := counterVecValue(t, m.SentTxOutcome, "result", "mined"); got != 1 {
		t.Errorf("mined = %v, want 1", got)
	}
	if got := counterVecValue(t, m.SentTxOutcome, "result", "expired"); got != 0 {
		t.Errorf("expired = %v, want 0", got)
	}

	tr.OnNewBlock(blockWith("b"))
	if got := gaugeValue(t, m.SentTxPending); got != 0 {
		t.Errorf("pending after all mined = %v, want 0", got)
	}
	if got := gaugeValue(t, m.SentTxPendingOldest); got != 0 {
		t.Errorf("oldest timestamp with nothing pending = %v, want 0", got)
	}
	// a block with nothing of ours is a no-op, and blocks keep arriving after the set is empty
	tr.OnNewBlock(blockWith("z"))
	tr.OnNewBlock(nil)
	if got := counterVecValue(t, m.SentTxOutcome, "result", "mined"); got != 2 {
		t.Errorf("mined = %v, want 2", got)
	}
}

func TestSentTxTrackerExpiry(t *testing.T) {
	tr, m, now := newTestSentTxTracker(t)
	tr.track("old")
	*now = now.Add(sentTxMaxAge + time.Second)
	tr.track("fresh") // a track sweeps too, so a stale entry does not need a block to expire
	if got := gaugeValue(t, m.SentTxPending); got != 1 {
		t.Errorf("pending = %v, want 1 (old expired)", got)
	}
	if got := counterVecValue(t, m.SentTxOutcome, "result", "expired"); got != 1 {
		t.Errorf("expired = %v, want 1", got)
	}
	// an expired txid that shows up in a block later is no longer ours to count
	tr.OnNewBlock(blockWith("old"))
	if got := counterVecValue(t, m.SentTxOutcome, "result", "mined"); got != 0 {
		t.Errorf("mined = %v, want 0", got)
	}
}

func TestSentTxTrackerBound(t *testing.T) {
	tr, m, now := newTestSentTxTracker(t)
	for i := 0; i < sentTxMaxTracked; i++ {
		tr.track(string(rune('a'+i%26)) + string(rune('A'+i/26%26)) + string(rune('0'+i/676)))
		*now = now.Add(time.Millisecond)
	}
	if got := tr.pendingCount(); got != sentTxMaxTracked {
		t.Fatalf("pending = %d, want %d", got, sentTxMaxTracked)
	}
	tr.track("one-more")
	if got := tr.pendingCount(); got != sentTxMaxTracked {
		t.Errorf("pending = %d, want the cap %d", got, sentTxMaxTracked)
	}
	if got := counterVecValue(t, m.SentTxOutcome, "result", "expired"); got != 1 {
		t.Errorf("expired = %v, want 1 (the oldest evicted)", got)
	}
}

func TestSentTxTrackerNilSafe(t *testing.T) {
	var tr *sentTxTracker
	tr.track("a")
	tr.OnNewBlock(blockWith("a"))
	noMetrics := newSentTxTracker(nil)
	noMetrics.track("a")
	noMetrics.OnNewBlock(blockWith("a"))
	if noMetrics.pendingCount() != 0 {
		t.Error("tracking must work without metrics wired")
	}
}

func gaugeValue(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	return collectorValue(t, g)
}

func counterVecValue(t *testing.T, cv *prometheus.CounterVec, label, value string) float64 {
	t.Helper()
	return collectorValue(t, cv.With(prometheus.Labels{label: value}))
}

func collectorValue(t *testing.T, c prometheus.Collector) float64 {
	t.Helper()
	reg := prometheus.NewPedanticRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("register: %v", err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range families {
		for _, m := range mf.GetMetric() {
			if m.GetGauge() != nil {
				return m.GetGauge().GetValue()
			}
			if m.GetCounter() != nil {
				return m.GetCounter().GetValue()
			}
		}
	}
	t.Fatal("no gauge or counter value gathered")
	return 0
}
