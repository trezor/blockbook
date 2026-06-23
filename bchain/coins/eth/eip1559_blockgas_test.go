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

// blockGasRPCStub serves a single CallContext response (raw JSON) or a transport error,
// recording the method and args so tests can assert the "latest" block tag is requested.
type blockGasRPCStub struct {
	raw     string
	err     error
	method  string
	gotArgs []interface{}
}

func (s *blockGasRPCStub) EthSubscribe(context.Context, interface{}, ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, errors.New("not implemented")
}

func (s *blockGasRPCStub) Close() {}

func (s *blockGasRPCStub) CallContext(ctx context.Context, result interface{}, method string, args ...interface{}) error {
	s.method = method
	s.gotArgs = args
	if s.err != nil {
		return s.err
	}
	return json.Unmarshal([]byte(s.raw), result)
}

func equalBigInt(a, b *big.Int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Cmp(b) == 0
}

func TestGetLatestBlockGas(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		err       error
		wantUsed  *big.Int
		wantLimit *big.Int
		wantErr   bool
	}{
		{
			name:      "post-London populates gas",
			raw:       `{"gasUsed":"0x5208","gasLimit":"0x1c9c380","baseFeePerGas":"0x7"}`,
			wantUsed:  big.NewInt(21000),
			wantLimit: big.NewInt(30000000),
		},
		{
			name: "pre-London (no baseFee) omits gas",
			raw:  `{"gasUsed":"0x5208","gasLimit":"0x1c9c380"}`,
		},
		{
			name:    "rpc error propagates",
			err:     errors.New("boom"),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &blockGasRPCStub{raw: tt.raw, err: tt.err}
			b := &EthereumRPC{RPC: stub, Timeout: time.Second}
			gu, gl, err := b.getLatestBlockGas()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !equalBigInt(gu, tt.wantUsed) {
				t.Errorf("gasUsed = %v, want %v", gu, tt.wantUsed)
			}
			if !equalBigInt(gl, tt.wantLimit) {
				t.Errorf("gasLimit = %v, want %v", gl, tt.wantLimit)
			}
			if stub.method != "eth_getBlockByNumber" {
				t.Errorf("method = %q, want eth_getBlockByNumber", stub.method)
			}
			if len(stub.gotArgs) < 1 || stub.gotArgs[0] != "latest" {
				t.Errorf("args = %v, want first arg \"latest\"", stub.gotArgs)
			}
		})
	}
}

func TestSetLatestBlockGas(t *testing.T) {
	t.Run("success sets fields", func(t *testing.T) {
		b := &EthereumRPC{RPC: &blockGasRPCStub{raw: `{"gasUsed":"0x5208","gasLimit":"0x3938700","baseFeePerGas":"0x7"}`}, Timeout: time.Second}
		fees := &bchain.Eip1559Fees{}
		b.setLatestBlockGas(fees)
		if !equalBigInt(fees.BlockGasUsed, big.NewInt(21000)) {
			t.Errorf("BlockGasUsed = %v, want 21000", fees.BlockGasUsed)
		}
		if !equalBigInt(fees.BlockGasLimit, big.NewInt(60000000)) {
			t.Errorf("BlockGasLimit = %v, want 60000000", fees.BlockGasLimit)
		}
	})

	t.Run("rpc error leaves fields nil (non-fatal)", func(t *testing.T) {
		b := &EthereumRPC{RPC: &blockGasRPCStub{err: errors.New("boom")}, Timeout: time.Second}
		fees := &bchain.Eip1559Fees{}
		b.setLatestBlockGas(fees)
		if fees.BlockGasUsed != nil || fees.BlockGasLimit != nil {
			t.Errorf("expected nil gas fields on error, got used=%v limit=%v", fees.BlockGasUsed, fees.BlockGasLimit)
		}
	})

	t.Run("pre-London leaves fields nil", func(t *testing.T) {
		b := &EthereumRPC{RPC: &blockGasRPCStub{raw: `{"gasUsed":"0x5208","gasLimit":"0x1c9c380"}`}, Timeout: time.Second}
		fees := &bchain.Eip1559Fees{}
		b.setLatestBlockGas(fees)
		if fees.BlockGasUsed != nil || fees.BlockGasLimit != nil {
			t.Errorf("expected nil gas fields pre-London, got used=%v limit=%v", fees.BlockGasUsed, fees.BlockGasLimit)
		}
	})
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
	})
}
