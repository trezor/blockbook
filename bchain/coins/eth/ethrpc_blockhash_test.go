package eth

import (
	"context"
	stdErrors "errors"
	"math/big"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/trezor/blockbook/bchain"
)

type stubEVMHeader struct {
	hash string
	num  *big.Int
}

func (h *stubEVMHeader) Hash() string         { return h.hash }
func (h *stubEVMHeader) Number() *big.Int     { return h.num }
func (h *stubEVMHeader) Difficulty() *big.Int { return big.NewInt(0) }

type stubEVMClient struct {
	header bchain.EVMHeader
	err    error
}

func (s *stubEVMClient) NetworkID(ctx context.Context) (*big.Int, error) { return nil, nil }
func (s *stubEVMClient) HeaderByNumber(ctx context.Context, number *big.Int) (bchain.EVMHeader, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.header, nil
}
func (s *stubEVMClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) { return nil, nil }
func (s *stubEVMClient) EstimateGas(ctx context.Context, msg interface{}) (uint64, error) {
	return 0, nil
}
func (s *stubEVMClient) BalanceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (*big.Int, error) {
	return nil, nil
}
func (s *stubEVMClient) NonceAt(ctx context.Context, addrDesc bchain.AddressDescriptor, blockNumber *big.Int) (uint64, error) {
	return 0, nil
}

func newTestEthereumRPC(client bchain.EVMClient) *EthereumRPC {
	return &EthereumRPC{Client: client, Timeout: time.Second}
}

func TestGetBlockHashMapsErrBlockNotFound(t *testing.T) {
	// AvalancheClient.HeaderByNumber returns bchain.ErrBlockNotFound directly
	// after the unfinalized-data fix. GetBlockHash must recognize that and
	// surface ErrBlockNotFound to sync.go's stdErrors.Is check; otherwise
	// fork detection breaks because juju/errors v0.0.0-2017 has no Unwrap.
	b := newTestEthereumRPC(&stubEVMClient{err: bchain.ErrBlockNotFound})
	_, err := b.GetBlockHash(123)
	if !stdErrors.Is(err, bchain.ErrBlockNotFound) {
		t.Fatalf("GetBlockHash() error = %v, want ErrBlockNotFound", err)
	}
}

func TestGetBlockHashMapsEthereumNotFound(t *testing.T) {
	b := newTestEthereumRPC(&stubEVMClient{err: ethereum.NotFound})
	_, err := b.GetBlockHash(123)
	if !stdErrors.Is(err, bchain.ErrBlockNotFound) {
		t.Fatalf("GetBlockHash() error = %v, want ErrBlockNotFound", err)
	}
}

func TestGetBlockHashPropagatesOtherErrors(t *testing.T) {
	other := stdErrors.New("rpc connection refused")
	b := newTestEthereumRPC(&stubEVMClient{err: other})
	_, err := b.GetBlockHash(123)
	if err == nil {
		t.Fatal("GetBlockHash() error = nil, want propagated error")
	}
	if stdErrors.Is(err, bchain.ErrBlockNotFound) {
		t.Fatalf("GetBlockHash() error = %v, must not match ErrBlockNotFound", err)
	}
}

func TestGetBlockHashReturnsHash(t *testing.T) {
	header := &stubEVMHeader{hash: "0xdeadbeef", num: big.NewInt(123)}
	b := newTestEthereumRPC(&stubEVMClient{header: header})
	got, err := b.GetBlockHash(123)
	if err != nil {
		t.Fatalf("GetBlockHash() error = %v", err)
	}
	if got != "0xdeadbeef" {
		t.Fatalf("GetBlockHash() = %q, want %q", got, "0xdeadbeef")
	}
}
