//go:build unittest

package eth

import (
	"context"
	"encoding/json"
	"errors"
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
	raw string
}

func (s *feeHistoryRPCStub) EthSubscribe(context.Context, interface{}, ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, errors.New("not implemented")
}

func (s *feeHistoryRPCStub) Close() {}

func (s *feeHistoryRPCStub) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	if method != "eth_feeHistory" {
		return errors.New("unexpected RPC method: " + method)
	}
	return json.Unmarshal([]byte(s.raw), result)
}

func TestEthereumTypeGetEip1559FeesOnChain(t *testing.T) {
	// baseFeePerGas[blocks-1]=[3]=0x64=100 is the projected next-block base fee (4-element array, as a
	// no-distinct-pending-block backend returns). Per-tier reward percentiles over 2 blocks.
	// Three reward columns, matching eip1559RewardPercentiles {20, 70, 99}.
	raw := `{"oldestBlock":"0x1",` +
		`"reward":[["0x1","0x2","0x4"],["0x2","0x6","0x8"]],` +
		`"baseFeePerGas":["0x10","0x20","0x30","0x64"],` +
		`"gasUsedRatio":[0.5,0.5,0.5]}`
	b := &EthereumRPC{
		RPC:         &feeHistoryRPCStub{raw: raw},
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
	if fees.BaseFeePerGas.Int64() != 100 {
		t.Errorf("BaseFeePerGas = %v, want 100", fees.BaseFeePerGas)
	}
	// tip = the tier's reward column reduced over the window; maxFeePerGas = 2*baseFee + tip.
	cases := []struct {
		name    string
		fee     *bchain.Eip1559Fee
		wantTip int64
	}{
		{"low", fees.Low, 2},         // p20 column (1,2), window max
		{"medium", fees.Medium, 4},   // p70 column (2,6), window median
		{"high", fees.High, 6},       // p70 column (2,6), window max
		{"instant", fees.Instant, 8}, // p99 column (4,8), window max
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.fee == nil {
				t.Fatal("nil tier")
			}
			if c.fee.MaxPriorityFeePerGas.Int64() != c.wantTip {
				t.Errorf("MaxPriorityFeePerGas = %v, want %d", c.fee.MaxPriorityFeePerGas, c.wantTip)
			}
			wantMax := int64(eip1559BaseFeeMultiplier)*100 + c.wantTip
			if c.fee.MaxFeePerGas.Int64() != wantMax {
				t.Errorf("MaxFeePerGas = %v, want %d", c.fee.MaxFeePerGas, wantMax)
			}
			// core invariant the previous code violated: maxFeePerGas must cover the base fee.
			if c.fee.MaxFeePerGas.Cmp(fees.BaseFeePerGas) <= 0 {
				t.Errorf("MaxFeePerGas %v must exceed baseFee %v", c.fee.MaxFeePerGas, fees.BaseFeePerGas)
			}
		})
	}
}

// TestEthereumTypeGetEip1559FeesOnChainShortRewardRow asserts the on-chain tier loop tolerates a
// non-compliant eth_feeHistory whose reward rows are shorter than the requested percentile count: it
// must not panic indexing the row, and a tier whose column is missing reduces over the rows that
// did carry it. It also covers the monotonic clamp, since dropping the row leaves instant below high.
func TestEthereumTypeGetEip1559FeesOnChainShortRewardRow(t *testing.T) {
	// Row 0 has all 3 percentiles; row 1 has only 2, so the instant tier's p99 column comes from
	// row 0 alone - which lands it below high and must then be lifted by the clamp.
	raw := `{"oldestBlock":"0x1",` +
		`"reward":[["0x1","0x2","0x4"],["0x2","0x6"]],` +
		`"baseFeePerGas":["0x10","0x20","0x30","0x64"],` +
		`"gasUsedRatio":[0.5,0.5,0.5]}`
	b := &EthereumRPC{
		RPC:         &feeHistoryRPCStub{raw: raw},
		Timeout:     time.Second,
		ChainConfig: &Configuration{Eip1559Fees: true},
	}
	fees, err := b.EthereumTypeGetEip1559Fees()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fees == nil {
		t.Fatal("expected fees")
	}
	cases := []struct {
		name    string
		fee     *bchain.Eip1559Fee
		wantTip int64
	}{
		{"low", fees.Low, 2},         // p20 (1,2), max
		{"medium", fees.Medium, 4},   // p70 (2,6), median
		{"high", fees.High, 6},       // p70 (2,6), max
		{"instant", fees.Instant, 6}, // p99 present only in row 0 (4); clamped up to high
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.fee == nil {
				t.Fatal("nil tier")
			}
			if c.fee.MaxPriorityFeePerGas.Int64() != c.wantTip {
				t.Errorf("MaxPriorityFeePerGas = %v, want %d", c.fee.MaxPriorityFeePerGas, c.wantTip)
			}
		})
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
	raw := `{"oldestBlock":"0x1",` +
		`"reward":[["0x1","0x2","0x3","0x4"]],` +
		`"baseFeePerGas":["0x10","0x20","0x30","0x64"],` +
		`"gasUsedRatio":[0.5,0.5,0.5]}`
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

// TestEthereumTypeGetEip1559FeesLadderMonotonic covers the case the clamp exists for: mixing
// reducers across tiers can invert the ladder on a spiky window, because a window maximum of the
// 20th percentile can exceed a window median of the 70th. Economy must never be quoted above Normal.
func TestEthereumTypeGetEip1559FeesLadderMonotonic(t *testing.T) {
	// One spiky block dominates the p20 column while the p70 column stays flat and low.
	raw := `{"oldestBlock":"0x1",` +
		`"reward":[["0x9","0x1","0x1"],["0x1","0x1","0x1"]],` +
		`"baseFeePerGas":["0x10","0x20","0x30","0x64"],` +
		`"gasUsedRatio":[0.5,0.5,0.5]}`
	b := &EthereumRPC{
		RPC:         &feeHistoryRPCStub{raw: raw},
		Timeout:     time.Second,
		ChainConfig: &Configuration{Eip1559Fees: true},
	}
	fees, err := b.EthereumTypeGetEip1559Fees()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tiers := []*bchain.Eip1559Fee{fees.Low, fees.Medium, fees.High, fees.Instant}
	names := []string{"low", "medium", "high", "instant"}
	for i, f := range tiers {
		if f == nil {
			t.Fatalf("%s tier is nil", names[i])
		}
		// low is the unclamped window max of 9; every tier above is lifted to match it.
		if f.MaxPriorityFeePerGas.Int64() != 9 {
			t.Errorf("%s tip = %v, want 9", names[i], f.MaxPriorityFeePerGas)
		}
		if i > 0 && f.MaxPriorityFeePerGas.Cmp(tiers[i-1].MaxPriorityFeePerGas) < 0 {
			t.Errorf("%s tip %v below %s %v: ladder inverted",
				names[i], f.MaxPriorityFeePerGas, names[i-1], tiers[i-1].MaxPriorityFeePerGas)
		}
		// Clamped tiers must not share a big.Int with the tier they were lifted to.
		if i > 0 && f.MaxPriorityFeePerGas == tiers[i-1].MaxPriorityFeePerGas {
			t.Errorf("%s and %s share a MaxPriorityFeePerGas pointer", names[i], names[i-1])
		}
	}
}

func TestMedianAndMaxBigInt(t *testing.T) {
	bi := func(v ...int64) []*big.Int {
		out := make([]*big.Int, len(v))
		for i, x := range v {
			out[i] = big.NewInt(x)
		}
		return out
	}
	// A reward above math.MaxInt64 must survive: the old int64 accumulator wrapped negative here.
	huge, _ := new(big.Int).SetString("18446744073709551615", 10) // 2^64-1
	cases := []struct {
		name string
		in   []*big.Int
		med  string
		max  string
	}{
		{"empty", nil, "0", "0"},
		{"single", bi(7), "7", "7"},
		{"odd", bi(5, 1, 9), "5", "9"},
		{"even averages the middles", bi(1, 2, 6, 8), "4", "8"},
		{"above int64", []*big.Int{huge, big.NewInt(1)}, "9223372036854775808", "18446744073709551615"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := medianBigInt(c.in).String(); got != c.med {
				t.Errorf("medianBigInt = %s, want %s", got, c.med)
			}
			if got := maxBigInt(c.in).String(); got != c.max {
				t.Errorf("maxBigInt = %s, want %s", got, c.max)
			}
		})
	}
	// The reducers must not reorder or otherwise disturb the caller's slice.
	in := bi(3, 1, 2)
	medianBigInt(in)
	if in[0].Int64() != 3 || in[1].Int64() != 1 || in[2].Int64() != 2 {
		t.Errorf("medianBigInt mutated its input: %v", in)
	}
}
