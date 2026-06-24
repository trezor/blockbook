//go:build unittest

package eth

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/trezor/blockbook/bchain"
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
