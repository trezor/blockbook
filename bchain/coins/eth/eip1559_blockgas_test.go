//go:build unittest

package eth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// feeHistoryRPCStub serves a canned eth_feeHistory response; any other method errors so the test
// also asserts the on-chain path no longer issues eth_maxPriorityFeePerGas.
type feeHistoryRPCStub struct {
	raw  string
	args []interface{} // the last eth_feeHistory arguments, so tests can assert the window and percentiles
}

func (s *feeHistoryRPCStub) EthSubscribe(context.Context, interface{}, ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, errors.New("not implemented")
}

func (s *feeHistoryRPCStub) Close() {}

func (s *feeHistoryRPCStub) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	if method != "eth_feeHistory" {
		return errors.New("unexpected RPC method: " + method)
	}
	s.args = args
	return json.Unmarshal([]byte(s.raw), result)
}

// feeHistoryBaseFees is the 9-element baseFeePerGas array geth and Erigon return for an 8-block
// "latest" window: one per window block plus the projected next-block base fee, 0x64=100, last.
const feeHistoryBaseFees = `"baseFeePerGas":["0x10","0x20","0x30","0x40","0x50","0x60","0x70","0x80","0x64"],` +
	`"gasUsedRatio":[0.5,0.5,0.5,0.5,0.5,0.5,0.5,0.5]`

func onChainFees(t *testing.T, reward string) (*bchain.Eip1559Fees, *feeHistoryRPCStub) {
	t.Helper()
	stub := &feeHistoryRPCStub{raw: `{"oldestBlock":"0x1","reward":` + reward + `,` + feeHistoryBaseFees + `}`}
	b := &EthereumRPC{
		RPC:         stub,
		Timeout:     time.Second,
		ChainConfig: &Configuration{Eip1559Fees: true},
	}
	fees, err := b.EthereumTypeGetEip1559Fees()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fees == nil || fees.BaseFeePerGas == nil {
		t.Fatal("expected fees with baseFeePerGas")
	}
	return fees, stub
}

// checkTiers asserts the tips of the ladder and that maxFeePerGas = 2*baseFee + tip for every tier.
func checkTiers(t *testing.T, fees *bchain.Eip1559Fees, low, medium, high, instant string) {
	t.Helper()
	tiers := []struct {
		name string
		fee  *bchain.Eip1559Fee
		want string
	}{{"low", fees.Low, low}, {"medium", fees.Medium, medium}, {"high", fees.High, high}, {"instant", fees.Instant, instant}}
	for _, c := range tiers {
		t.Run(c.name, func(t *testing.T) {
			if c.fee == nil {
				t.Fatal("nil tier")
			}
			wantTip, _ := new(big.Int).SetString(c.want, 10)
			if c.fee.MaxPriorityFeePerGas.Cmp(wantTip) != 0 {
				t.Errorf("MaxPriorityFeePerGas = %v, want %v", c.fee.MaxPriorityFeePerGas, wantTip)
			}
			wantMax := new(big.Int).Add(new(big.Int).Mul(fees.BaseFeePerGas, big.NewInt(eip1559BaseFeeMultiplier)), wantTip)
			if c.fee.MaxFeePerGas.Cmp(wantMax) != 0 {
				t.Errorf("MaxFeePerGas = %v, want %v", c.fee.MaxFeePerGas, wantMax)
			}
			// core invariant the pre-fix code violated: maxFeePerGas must cover the base fee.
			if c.fee.MaxFeePerGas.Cmp(fees.BaseFeePerGas) <= 0 {
				t.Errorf("MaxFeePerGas %v must exceed baseFee %v", c.fee.MaxFeePerGas, fees.BaseFeePerGas)
			}
		})
	}
}

func TestEthereumTypeGetEip1559FeesOnChain(t *testing.T) {
	// Three blocks of (p20, p50, p99) rewards. Each tier is the window MAX of its column, not a mean:
	// low = max p20 = 4, medium = 1.5*low = 6, high = max p50 = 8, instant = max p99 = 0x30 = 48.
	fees, stub := onChainFees(t, `[["0x2","0x5","0x9"],["0x4","0x3","0x30"],["0x1","0x8","0x7"]]`)
	if fees.BaseFeePerGas.Int64() != 100 {
		t.Errorf("BaseFeePerGas = %v, want 100", fees.BaseFeePerGas)
	}
	checkTiers(t, fees, "4", "6", "8", "48")

	// the request itself: an 8-block window of mined blocks (latest, not the inflated pending row),
	// percentiles 20/50/99
	if len(stub.args) != 3 || stub.args[0] != eip1559FeeHistoryBlocks || stub.args[1] != "latest" {
		t.Fatalf("eth_feeHistory args = %v, want [%d latest [20 50 99]]", stub.args, eip1559FeeHistoryBlocks)
	}
	if got := fmt.Sprint(stub.args[2]); got != "[20 50 99]" {
		t.Errorf("percentiles = %s, want [20 50 99]", got)
	}
	for _, f := range []*bchain.Eip1559Fee{fees.Low, fees.Medium, fees.High} {
		if f.MaxPriorityFeePerGas.Cmp(fees.Instant.MaxPriorityFeePerGas) > 0 {
			t.Errorf("ladder must be non-decreasing: %v above instant %v", f.MaxPriorityFeePerGas, fees.Instant.MaxPriorityFeePerGas)
		}
	}
}

// TestEthereumTypeGetEip1559FeesOnChainLadderClamp: when the busiest block's p50 (or p99) is below
// the tier under it, the tier is lifted to it so the ladder never inverts (medium 1.5*low can exceed
// a quiet p50).
func TestEthereumTypeGetEip1559FeesOnChainLadderClamp(t *testing.T) {
	fees, _ := onChainFees(t, `[["0x10","0x1","0x2"],["0x8","0x11","0x3"]]`)
	// low = 16, medium = 24; max p50 = 17 < 24 -> high = 24; max p99 = 3 < 24 -> instant = 24
	checkTiers(t, fees, "16", "24", "24", "24")
	if fees.Medium.MaxPriorityFeePerGas == fees.High.MaxPriorityFeePerGas {
		t.Error("clamped tiers must not share one big.Int")
	}
}

// TestEthereumTypeGetEip1559FeesOnChainShortRewardRow asserts the reducer tolerates a non-compliant
// eth_feeHistory whose reward rows are shorter than the requested percentile count: it must not panic
// and must take the column max over the rows that carry the column.
func TestEthereumTypeGetEip1559FeesOnChainShortRewardRow(t *testing.T) {
	// Row 1 has only the p20 column; p50 and p99 come from row 0 alone. An undecodable cell is skipped.
	fees, _ := onChainFees(t, `[["0x1","0x2","0x3"],["0x4"],["zz","0x9"]]`)
	// low = max(1,4) = 4, medium = 6, high = max(2,9)=9, instant = max(3, 9) = 9
	checkTiers(t, fees, "4", "6", "9", "9")
}

// TestEthereumTypeGetEip1559FeesOnChainIdle: no rewards at all (quiet testnet, backend without reward
// support) prices every tier at a zero tip; maxFeePerGas still covers 2*baseFee so the tx is mineable.
func TestEthereumTypeGetEip1559FeesOnChainIdle(t *testing.T) {
	fees, _ := onChainFees(t, `[]`)
	checkTiers(t, fees, "0", "0", "0", "0")
	fees, _ = onChainFees(t, `[["0x0","0x0","0x0"],[]]`)
	checkTiers(t, fees, "0", "0", "0", "0")
}

// TestEthereumTypeGetEip1559FeesOnChainBigReward: rewards are wei and may exceed int64 (a 2^64-1
// bribe would have wrapped negative through the old int64 sum); the reducer must carry them intact.
func TestEthereumTypeGetEip1559FeesOnChainBigReward(t *testing.T) {
	fees, _ := onChainFees(t, `[["0x2","0x3","0xffffffffffffffff"],["0x1","0x10000000000000000","0x4"]]`)
	checkTiers(t, fees, "2", "3", "18446744073709551616", "18446744073709551616")
}

// TestEthereumTypeGetEip1559FeesOnChainShortHistory: a backend that returns fewer base fees than the
// window asked for (a chain younger than 8 blocks, or a non-compliant node) yields no on-chain estimate
// rather than a quote read off the wrong index.
// TestEthereumTypeGetEip1559FeesOnChainBaseFeeIsLast: the projected base fee is read from the end of
// the array, so a backend that returns one base fee per block with the projection last (Erigon's
// shape for a shortened window) and one that returns blocks+1 agree, and a malformed value is an error
// rather than a silent zero.
func TestEthereumTypeGetEip1559FeesOnChainBaseFeeIsLast(t *testing.T) {
	newRPC := func(baseFees string) *EthereumRPC {
		return &EthereumRPC{
			RPC: &feeHistoryRPCStub{raw: `{"oldestBlock":"0x1","reward":[["0x1","0x2","0x3"]],` +
				`"baseFeePerGas":` + baseFees + `,"gasUsedRatio":[0.5,0.5,0.5,0.5,0.5,0.5,0.5,0.5]}`},
			Timeout:     time.Second,
			ChainConfig: &Configuration{Eip1559Fees: true},
		}
	}
	for _, baseFees := range []string{
		`["0x10","0x20","0x30","0x40","0x50","0x60","0x70","0x64"]`,
		`["0x10","0x20","0x30","0x40","0x50","0x60","0x70","0x80","0x64"]`,
	} {
		fees, err := newRPC(baseFees).EthereumTypeGetEip1559Fees()
		if err != nil || fees == nil || fees.BaseFeePerGas.Int64() != 100 {
			t.Errorf("baseFeePerGas %s: fees, err = %v, %v; want base fee 100", baseFees, fees, err)
		}
	}
	if _, err := newRPC(`["0x10","0x20","0x30","0x40","0x50","0x60","0x70","0x80","zz"]`).EthereumTypeGetEip1559Fees(); err == nil {
		t.Error("undecodable base fee: expected an error")
	}
}

func TestEthereumTypeGetEip1559FeesOnChainShortHistory(t *testing.T) {
	b := &EthereumRPC{
		RPC: &feeHistoryRPCStub{raw: `{"oldestBlock":"0x1","reward":[["0x1","0x2","0x3"]],` +
			`"baseFeePerGas":["0x10","0x20","0x30","0x64"],"gasUsedRatio":[0.5,0.5,0.5]}`},
		Timeout:     time.Second,
		ChainConfig: &Configuration{Eip1559Fees: true},
	}
	fees, err := b.EthereumTypeGetEip1559Fees()
	if err != nil || fees != nil {
		t.Fatalf("fees, err = %v, %v; want nil, nil", fees, err)
	}
}

// gaugeVecSeriesCount reports how many label series a GaugeVec currently holds, using a throwaway
// registry (same approach as gaugeValue, no prometheus/testutil dependency).
func gaugeVecSeriesCount(t *testing.T, gv *prometheus.GaugeVec) int {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(gv); err != nil {
		t.Fatalf("register gauge vec: %v", err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather gauge vec: %v", err)
	}
	n := 0
	for _, mf := range families {
		n += len(mf.GetMetric())
	}
	return n
}

func TestObserveEip1559Fees(t *testing.T) {
	m := &common.Metrics{
		EthEip1559Fee: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{Name: "test_eth_eip1559_fee"}, []string{"tier", "kind"}),
		EthEip1559BaseFee: prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_eth_eip1559_base_fee"}),
	}
	b := &EthereumRPC{}
	b.metrics = m
	// only low and high tiers populated; medium/instant left nil
	b.observeEip1559Fees(&bchain.Eip1559Fees{
		BaseFeePerGas: big.NewInt(100),
		Low:           &bchain.Eip1559Fee{MaxFeePerGas: big.NewInt(250), MaxPriorityFeePerGas: big.NewInt(2)},
		High:          &bchain.Eip1559Fee{MaxFeePerGas: big.NewInt(260), MaxPriorityFeePerGas: big.NewInt(4)},
	})

	if got := gaugeValue(t, m.EthEip1559BaseFee); got != 100 {
		t.Errorf("base fee gauge = %v, want 100", got)
	}
	for _, c := range []struct {
		tier, kind string
		want       float64
	}{
		{"low", "max_fee", 250}, {"low", "priority_fee", 2},
		{"high", "max_fee", 260}, {"high", "priority_fee", 4},
	} {
		got := gaugeValue(t, m.EthEip1559Fee.With(common.Labels{"tier": c.tier, "kind": c.kind}))
		if got != c.want {
			t.Errorf("eip1559_fee{tier=%s,kind=%s} = %v, want %v", c.tier, c.kind, got, c.want)
		}
	}
	// nil tiers (medium, instant) must not create series: exactly 4 (=2 tiers x 2 kinds) expected
	if n := gaugeVecSeriesCount(t, m.EthEip1559Fee); n != 4 {
		t.Errorf("expected 4 tier series, got %d (nil tiers must be skipped)", n)
	}

	// nil metrics must be a no-op (matches the unit-test path with no metrics wired)
	(&EthereumRPC{}).observeEip1559Fees(&bchain.Eip1559Fees{BaseFeePerGas: big.NewInt(1)})
}

func TestObserveSyncMetric(t *testing.T) {
	gv := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{Name: "test_alt_fee_provider_last_sync"}, []string{"provider"})
	p := &alternativeFeeProvider{
		metrics: &common.Metrics{AlternativeFeeProviderLastSync: gv},
		name:    "infura",
	}
	ts := time.Unix(1700000000, 0)
	p.observeSync(ts)

	if !p.lastSync.Equal(ts) {
		t.Errorf("lastSync = %v, want %v (must use the same instant as the metric)", p.lastSync, ts)
	}
	if got := gaugeValue(t, gv.With(common.Labels{"provider": "infura"})); got != 1700000000 {
		t.Errorf("last_sync gauge = %v, want 1700000000", got)
	}

	// nil metrics must still advance lastSync without panicking
	p2 := &alternativeFeeProvider{name: "1inch"}
	p2.observeSync(ts)
	if !p2.lastSync.Equal(ts) {
		t.Errorf("nil-metrics observeSync must still set lastSync")
	}
}

func TestEip1559FeeSourceMetric(t *testing.T) {
	m := &common.Metrics{
		EthEip1559FeeSource: prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: "test_eth_eip1559_fee_source_total"}, []string{"source"}),
		EthEip1559Fee: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{Name: "test_eth_eip1559_fee_src"}, []string{"tier", "kind"}),
		EthEip1559BaseFee: prometheus.NewGauge(prometheus.GaugeOpts{Name: "test_eth_eip1559_base_fee_src"}),
	}
	raw := `{"oldestBlock":"0x1","reward":[["0x1","0x2","0x3"]],` + feeHistoryBaseFees + `}`
	b := &EthereumRPC{
		RPC:         &feeHistoryRPCStub{raw: raw},
		Timeout:     time.Second,
		ChainConfig: &Configuration{Eip1559Fees: true},
	}
	b.metrics = m
	if _, err := b.EthereumTypeGetEip1559Fees(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// no alternative provider configured -> the on-chain estimate is source=onchain (not a fallback)
	if got := counterVecValue(t, m.EthEip1559FeeSource, "source", "onchain"); got != 1 {
		t.Errorf("source=onchain counter = %v, want 1", got)
	}
	if got := counterVecValue(t, m.EthEip1559FeeSource, "source", "onchain_fallback"); got != 0 {
		t.Errorf("source=onchain_fallback counter = %v, want 0", got)
	}
}

func equalBigInt(a, b *big.Int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Cmp(b) == 0
}

func TestAttachBlockGas(t *testing.T) {
	t.Run("post-London allocates and sets", func(t *testing.T) {
		bsd := attachBlockGas(&rpcHeader{GasUsed: "0x5208", GasLimit: "0x3938700", BaseFeePerGas: "0x7"}, nil)
		if bsd == nil {
			t.Fatal("expected allocated EthereumBlockSpecificData, got nil")
		}
		if !equalBigInt(bsd.BaseFeePerGas, big.NewInt(7)) {
			t.Errorf("BaseFeePerGas = %v, want 7", bsd.BaseFeePerGas)
		}
		if !equalBigInt(bsd.GasUsed, big.NewInt(21000)) {
			t.Errorf("GasUsed = %v, want 21000", bsd.GasUsed)
		}
		if !equalBigInt(bsd.GasLimit, big.NewInt(60000000)) {
			t.Errorf("GasLimit = %v, want 60000000", bsd.GasLimit)
		}
	})

	t.Run("pre-London with nil existing returns nil", func(t *testing.T) {
		if got := attachBlockGas(&rpcHeader{GasUsed: "0x5208", GasLimit: "0x1c9c380"}, nil); got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("pre-London preserves existing struct unchanged", func(t *testing.T) {
		existing := &bchain.EthereumBlockSpecificData{InternalDataError: "boom"}
		got := attachBlockGas(&rpcHeader{}, existing)
		if got != existing {
			t.Fatal("expected existing struct returned unchanged")
		}

		if got.BaseFeePerGas != nil {
			t.Errorf("expected no gas set pre-London, got %v", got.BaseFeePerGas)
		}
	})

	t.Run("post-London preserves existing fields and adds gas", func(t *testing.T) {
		existing := &bchain.EthereumBlockSpecificData{InternalDataError: "err"}
		got := attachBlockGas(&rpcHeader{GasUsed: "0x1", GasLimit: "0x2", BaseFeePerGas: "0x7"}, existing)
		if got.InternalDataError != "err" {
			t.Errorf("InternalDataError = %q, want preserved \"err\"", got.InternalDataError)
		}
		if !equalBigInt(got.BaseFeePerGas, big.NewInt(7)) {
			t.Errorf("BaseFeePerGas = %v, want 7", got.BaseFeePerGas)
		}
		if !equalBigInt(got.GasUsed, big.NewInt(1)) {
			t.Errorf("GasUsed = %v, want 1", got.GasUsed)
		}
		if !equalBigInt(got.GasLimit, big.NewInt(2)) {
			t.Errorf("GasLimit = %v, want 2", got.GasLimit)
		}
	})
}
