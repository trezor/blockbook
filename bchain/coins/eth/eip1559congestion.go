package eth

import (
	"context"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/glog"
	"github.com/juju/errors"
)

// eip1559CongestionBlocks is the trailing window of mined base fees the projected base fee is ranked
// in for networkCongestion. 200 blocks is ~40 min on mainnet: long enough that the rank reads as
// "expensive compared to the last while" rather than "up since the previous block". The window is
// fetched without reward percentiles, a header-only read on geth and Erigon.
const eip1559CongestionBlocks = 200

// baseFeeWindow caches the congestion window per tip block, so the pull path adds at most one
// 200-block eth_feeHistory per new block however often estimateFee is called.
type baseFeeWindow struct {
	mu   sync.Mutex
	key  string // oldestBlock of the 8-block tier window; it moves with every new tip block
	fees []*big.Int
}

// eip1559BaseFeePercentile is the networkCongestion score: the fraction of window base fees strictly
// below the projected base fee. 0 means the next block is the cheapest in the window, or the base fee
// is flat (rollups at their floor); 1 means it is the most expensive. An empty window scores 0, which
// omitempty drops from the response. The ratio of full blocks is deliberately not used: EIP-1559 holds
// it at 0.5 on average whatever the price level (see docs/fees.md).
func eip1559BaseFeePercentile(window []*big.Int, baseFee *big.Int) float64 {
	if len(window) == 0 || baseFee == nil {
		return 0
	}
	below := 0
	for _, f := range window {
		if f.Cmp(baseFee) < 0 {
			below++
		}
	}
	return float64(below) / float64(len(window))
}

// congestionWindow returns the mined base fees of the last eip1559CongestionBlocks blocks, from the
// cache while key matches the last call. A failed fetch keeps the previous window (one block stale
// is still a fair rank) and warns once per tip block.
func (b *EthereumRPC) congestionWindow(key string) []*big.Int {
	w := &b.baseFeeWindow
	w.mu.Lock()
	if w.key == key {
		fees := w.fees
		w.mu.Unlock()
		return fees
	}
	w.mu.Unlock()
	// Fetched outside the lock: concurrent callers at a block boundary may fetch twice, but waiting
	// on another caller's RPC would stall every estimateFee for up to b.Timeout.
	fees, err := b.fetchBaseFeeWindow()
	w.mu.Lock()
	defer w.mu.Unlock()
	if err != nil {
		if w.key != key {
			glog.Warning("eth_feeHistory congestion window: ", err)
			w.key = key
		}
		return w.fees
	}
	w.key, w.fees = key, fees
	return fees
}

func (b *EthereumRPC) fetchBaseFeeWindow() ([]*big.Int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var h struct {
		BaseFeePerGas []string `json:"baseFeePerGas"`
	}
	if err := b.RPC.CallContext(ctx, &h, "eth_feeHistory", eip1559CongestionBlocks, "latest", []int{}); err != nil {
		return nil, err
	}
	// the last element is the projection for the block after the window, not a mined base fee
	if len(h.BaseFeePerGas) < 2 {
		return nil, errors.Errorf("eth_feeHistory returned %d base fees", len(h.BaseFeePerGas))
	}
	mined := h.BaseFeePerGas[:len(h.BaseFeePerGas)-1]
	fees := make([]*big.Int, 0, len(mined))
	for _, s := range mined {
		v, err := hexutil.DecodeBig(s)
		if err != nil {
			return nil, errors.Annotate(err, "eth_feeHistory baseFeePerGas")
		}
		fees = append(fees, v)
	}
	return fees, nil
}
