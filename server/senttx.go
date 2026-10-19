package server

import (
	"sync"
	"time"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// sentTxMaxAge is how long an own broadcast is watched for before it is written off as expired.
// Long enough to outlive any fee-estimation ETA by two orders of magnitude, short enough that a
// replaced or dropped transaction cannot pin the pending gauge for a day.
const sentTxMaxAge = time.Hour

// sentTxMaxTracked bounds the watch list; the oldest entries expire first when it is full.
const sentTxMaxTracked = 10000

// sentTxTracker follows the transactions this instance broadcast until a connected block carries
// them. It is the one signal that measures what a fee estimate is for: a wallet's transaction
// getting mined in the time the quote promised. Every API path (websocket, REST, explorer form)
// records its successful sends here, and OnNewBlock drains the ones that landed.
type sentTxTracker struct {
	mu      sync.Mutex
	pending map[string]time.Time // txid -> broadcast time
	metrics *common.Metrics
	now     func() time.Time
}

func newSentTxTracker(metrics *common.Metrics) *sentTxTracker {
	return &sentTxTracker{pending: make(map[string]time.Time), metrics: metrics, now: time.Now}
}

// track records a transaction the backend or relay accepted for broadcast.
func (t *sentTxTracker) track(txid string) {
	if t == nil || txid == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if _, known := t.pending[txid]; !known && len(t.pending) >= sentTxMaxTracked {
		t.expireOldestLocked(now)
	}
	if _, known := t.pending[txid]; !known {
		t.pending[txid] = now
	}
	t.sweepLocked(now)
	t.exportLocked()
}

// OnNewBlock settles every tracked transaction the block carries and expires the ones that have
// waited past sentTxMaxAge.
func (t *sentTxTracker) OnNewBlock(block *bchain.Block) {
	if t == nil || block == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if len(t.pending) > 0 {
		for i := range block.Txs {
			if sentAt, ok := t.pending[block.Txs[i].Txid]; ok {
				delete(t.pending, block.Txs[i].Txid)
				t.observeOutcome("mined", now.Sub(sentAt))
			}
		}
	}
	t.sweepLocked(now)
	t.exportLocked()
}

func (t *sentTxTracker) sweepLocked(now time.Time) {
	for txid, sentAt := range t.pending {
		if now.Sub(sentAt) > sentTxMaxAge {
			delete(t.pending, txid)
			t.observeOutcome("expired", now.Sub(sentAt))
		}
	}
}

func (t *sentTxTracker) expireOldestLocked(now time.Time) {
	var oldest string
	var oldestAt time.Time
	for txid, sentAt := range t.pending {
		if oldest == "" || sentAt.Before(oldestAt) {
			oldest, oldestAt = txid, sentAt
		}
	}
	if oldest != "" {
		delete(t.pending, oldest)
		t.observeOutcome("expired", now.Sub(oldestAt))
	}
}

func (t *sentTxTracker) observeOutcome(result string, waited time.Duration) {
	if t.metrics == nil {
		return
	}
	if t.metrics.SentTxOutcome != nil {
		t.metrics.SentTxOutcome.With(common.Labels{"result": result}).Inc()
	}
	if result == "mined" && t.metrics.SentTxInclusionSeconds != nil {
		t.metrics.SentTxInclusionSeconds.Observe(waited.Seconds())
	}
}

// exportLocked publishes the pending count and the broadcast time of the oldest pending send (0
// when nothing is pending, so time() - it is only meaningful alongside a non-zero count).
func (t *sentTxTracker) exportLocked() {
	if t.metrics == nil || t.metrics.SentTxPending == nil || t.metrics.SentTxPendingOldest == nil {
		return
	}
	t.metrics.SentTxPending.Set(float64(len(t.pending)))
	var oldest time.Time
	for _, sentAt := range t.pending {
		if oldest.IsZero() || sentAt.Before(oldest) {
			oldest = sentAt
		}
	}
	if oldest.IsZero() {
		t.metrics.SentTxPendingOldest.Set(0)
	} else {
		t.metrics.SentTxPendingOldest.Set(float64(oldest.Unix()))
	}
}

// pendingCount is for tests.
func (t *sentTxTracker) pendingCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.pending)
}
