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
	raw := `{"oldestBlock":"0x1",` +
		`"reward":[["0x1","0x2","0x3","0x4"],["0x3","0x4","0x5","0x6"]],` +
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
	// tip = average of the tier's reward percentile; maxFeePerGas = 2*baseFee + tip.
	cases := []struct {
		name    string
		fee     *bchain.Eip1559Fee
		wantTip int64
	}{
		{"low", fees.Low, 2},         // avg(1,3)
		{"medium", fees.Medium, 3},   // avg(2,4)
		{"high", fees.High, 4},       // avg(3,5)
		{"instant", fees.Instant, 5}, // avg(4,6)
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

// counterVecValue reads the counter series carrying label=value, via a throwaway registry.
func counterVecValue(t *testing.T, cv *prometheus.CounterVec, label, value string) float64 {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(cv); err != nil {
		t.Fatalf("register counter vec: %v", err)
	}
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather counter vec: %v", err)
	}
	for _, mf := range families {
		for _, mm := range mf.GetMetric() {
			for _, lp := range mm.GetLabel() {
				if lp.GetName() == label && lp.GetValue() == value {
					return mm.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
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
