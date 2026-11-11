package eth

import (
	"context"
	"math"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/glog"
	"github.com/juju/errors"
)

// eip1559CongestionWindow is the history networkCongestion ranks against: a week, so a short bump that
// is large only against a quiet day or two (as with a 3-day window) does not rank as busy, while a
// new fee level is absorbed within a week (unlike 14 or 30 days), see docs/fees.md.
const eip1559CongestionWindow = 7 * 24 * time.Hour

// eip1559CongestionMaxBlocks caps the window at ~4.6 MiB of base fees and ~590 eth_feeHistory calls to
// fill; chains faster than one block per second rank against less than a week.
const eip1559CongestionMaxBlocks = 604800

// feeHistoryMaxBlocks is the most blocks geth and Erigon serve from one eth_feeHistory call.
const feeHistoryMaxBlocks = 1024

// baseFeeWindowRetry spaces fill attempts after a failure, so a backend without deep fee history is
// not asked again on every quote.
const baseFeeWindowRetry = time.Minute

// baseFeeWindow is a ring of mined base fees indexed by height, filled once in the background and then
// extended from each quote's 8-block tier window, so a quote costs no extra RPC once it is full.
type baseFeeWindow struct {
	mu          sync.Mutex
	fees        []uint64
	has         bool
	first, last uint64 // stored heights, empty while last < first
	filling     bool
	retryAt     time.Time
	key         [2]uint64 // tip height and projected base fee of the cached score
	score       float64
	lo, hi      uint64
}

// put stores height h, which must be at most one past the newest stored height.
func (w *baseFeeWindow) put(h, fee uint64) {
	size := uint64(len(w.fees))
	w.fees[h%size] = fee
	if h == w.last+1 {
		w.last = h
		if w.last-w.first+1 > size {
			w.first = w.last - size + 1
		}
	}
}

// eip1559CongestionScore is networkCongestion: for each of the last k quotes, the fraction of the n base
// fees before it strictly below its next-block base fee, averaged over the k quotes. seq holds n+k-1
// mined base fees, oldest first, then the projection. A flat base fee (a rollup at its floor) scores 0.
// Averaging over the tier window's 8 blocks cuts how often the score crosses a threshold about 3x,
// because EIP-1559 makes the base fee swing block to block.
func eip1559CongestionScore(seq []uint64, n int) float64 {
	k := len(seq) - n
	if n <= 0 || k <= 0 {
		return 0
	}
	below := 0
	for j := 0; j < k; j++ {
		next := seq[j+n]
		for _, f := range seq[j : j+n] {
			if f < next {
				below++
			}
		}
	}
	return float64(below) / float64(n*k)
}

// baseFeeUint64 saturates, so a base fee above 2^64-1 wei ranks as the highest rather than wrapping.
func baseFeeUint64(v *big.Int) uint64 {
	if v.Sign() <= 0 {
		return 0
	}
	if !v.IsUint64() {
		return math.MaxUint64
	}
	return v.Uint64()
}

// eip1559BaseFeeTrend is baseFeeTrend: "up" when the projected base fee is above the oldest base
// fee of the tier window (8 blocks, ~100 s on mainnet), "down" below it, "" when equal (a flat
// chain), which omitempty drops. A one-block comparison would flip every block as EIP-1559 hunts
// its target; eight blocks is the shortest span that reads as a direction.
func eip1559BaseFeeTrend(from, to *big.Int) string {
	if from == nil || to == nil {
		return ""
	}
	switch to.Cmp(from) {
	case 1:
		return "up"
	case -1:
		return "down"
	}
	return ""
}

// congestionBlocks is the window length in blocks for this chain's block time, 0 when unknown.
func (b *EthereumRPC) congestionBlocks() int {
	avg, err := b.ChainConfig.AverageBlockTimeDuration()
	if err != nil {
		return 0
	}
	return int(min(eip1559CongestionWindow/avg, eip1559CongestionMaxBlocks))
}

// congestion returns networkCongestion and historicalBaseFeeRange for the tier window starting at
// height oldest, whose mined base fees are mined and projection next. Both stay empty (0, nil) until
// the window is full; the first call starts filling it in the background.
func (b *EthereumRPC) congestion(oldest uint64, mined []uint64, next uint64) (float64, []*big.Int) {
	n, k := b.congestionBlocks(), len(mined)
	if n == 0 || k == 0 || k > eip1559FeeHistoryBlocks {
		return 0, nil
	}
	w := &b.baseFeeWindow
	w.mu.Lock()
	defer w.mu.Unlock()
	if size := n + eip1559FeeHistoryBlocks - 1; len(w.fees) != size {
		w.fees, w.has = make([]uint64, size), false
	}
	// the tier window extends the ring and rewrites the heights a shallow reorg changed
	if w.has && oldest >= w.first && oldest <= w.last+1 {
		for i, f := range mined {
			w.put(oldest+uint64(i), f)
		}
	}
	tip, span := oldest+uint64(k)-1, uint64(n+k-1)
	if tip+1 < span {
		return 0, nil
	}
	from := tip - span + 1
	if !w.has || w.last < w.first || w.first > from || w.last < tip {
		if !w.filling && time.Now().After(w.retryAt) {
			start := from
			if w.has && w.last+1 > from && w.first <= from {
				start = w.last + 1
			}
			w.filling = true
			go b.fillBaseFeeWindow(start, tip)
		}
		return 0, nil
	}
	if w.key != [2]uint64{tip, next} {
		size := uint64(len(w.fees))
		seq := make([]uint64, 0, span+1)
		for h := from; h <= tip; h++ {
			seq = append(seq, w.fees[h%size])
		}
		w.lo, w.hi = math.MaxUint64, 0
		for _, f := range seq[k-1:] {
			w.lo, w.hi = min(w.lo, f), max(w.hi, f)
		}
		w.score = eip1559CongestionScore(append(seq, next), n)
		w.key = [2]uint64{tip, next}
	}
	return w.score, []*big.Int{new(big.Int).SetUint64(w.lo), new(big.Int).SetUint64(w.hi)}
}

// fillBaseFeeWindow loads heights from..to into the window, continuing it when from follows the newest
// stored height and starting it over otherwise.
func (b *EthereumRPC) fillBaseFeeWindow(from, to uint64) {
	fees, err := b.fetchBaseFees(from, to)
	w := &b.baseFeeWindow
	w.mu.Lock()
	defer w.mu.Unlock()
	w.filling = false
	if err != nil {
		glog.Warning("eth_feeHistory congestion window: ", err)
		w.retryAt = time.Now().Add(baseFeeWindowRetry)
		return
	}
	if !w.has || from != w.last+1 {
		w.has, w.first, w.last = true, from, from-1
	}
	for i, f := range fees {
		w.put(from+uint64(i), f)
	}
}

// fetchBaseFees reads the base fees of heights from..to, oldest first, feeHistoryMaxBlocks per call and
// without reward percentiles, a header-only read on geth and Erigon.
func (b *EthereumRPC) fetchBaseFees(from, to uint64) ([]uint64, error) {
	out := make([]uint64, 0, to-from+1)
	for start := from; start <= to; start += feeHistoryMaxBlocks {
		end := min(start+feeHistoryMaxBlocks-1, to)
		count := int(end - start + 1)
		var h struct {
			OldestBlock   string   `json:"oldestBlock"`
			BaseFeePerGas []string `json:"baseFeePerGas"`
		}
		ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
		err := b.RPC.CallContext(ctx, &h, "eth_feeHistory", count, hexutil.EncodeUint64(end), []int{})
		cancel()
		if err != nil {
			return nil, err
		}
		if oldest, err := hexutil.DecodeUint64(h.OldestBlock); err != nil || oldest != start || len(h.BaseFeePerGas) < count {
			return nil, errors.Errorf("eth_feeHistory %d..%d returned oldestBlock %q with %d base fees", start, end, h.OldestBlock, len(h.BaseFeePerGas))
		}
		for _, s := range h.BaseFeePerGas[:count] {
			v, err := hexutil.DecodeBig(s)
			if err != nil {
				return nil, errors.Annotate(err, "eth_feeHistory baseFeePerGas")
			}
			out = append(out, baseFeeUint64(v))
		}
	}
	return out, nil
}
