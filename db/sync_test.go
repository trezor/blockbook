//go:build unittest

package db

import (
	"context"
	"encoding/json"
	stdErrors "errors"
	"io"
	"math/big"
	"net"
	"net/url"
	"syscall"
	"testing"

	jujuErrors "github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

func TestIsRetryableGetBlockError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil",
			err:  nil,
			want: false,
		},
		{
			name: "block not found",
			err:  bchain.ErrBlockNotFound,
			want: true,
		},
		{
			name: "deadline exceeded",
			err:  context.DeadlineExceeded,
			want: true,
		},
		{
			name: "unexpected EOF",
			err:  io.ErrUnexpectedEOF,
			want: true,
		},
		{
			name: "EOF",
			err:  io.EOF,
			want: true,
		},
		{
			name: "annotated deadline exceeded",
			err:  jujuErrors.Annotatef(context.DeadlineExceeded, "eth_getLogs blockNumber %v", "0x1"),
			want: true,
		},
		{
			name: "annotated unexpected EOF",
			err:  jujuErrors.Annotatef(io.ErrUnexpectedEOF, "eth_getLogs blockNumber %v", "0x1"),
			want: true,
		},
		{
			name: "network timeout",
			err: &net.DNSError{
				Err:       "i/o timeout",
				Name:      "example.org",
				IsTimeout: true,
			},
			want: true,
		},
		{
			name: "connection reset by peer",
			err: &url.Error{
				Op:  "Post",
				URL: "http://127.0.0.1:8545",
				Err: syscall.ECONNRESET,
			},
			want: true,
		},
		{
			name: "connection refused",
			err: &url.Error{
				Op:  "Post",
				URL: "http://127.0.0.1:8545",
				Err: syscall.ECONNREFUSED,
			},
			want: true,
		},
		{
			name: "rpc 503",
			err:  stdErrors.New("503 Service Unavailable: backend overloaded"),
			want: true,
		},
		{
			name: "rpc 429",
			err:  stdErrors.New("429 Too Many Requests"),
			want: true,
		},
		{
			name: "header not found",
			err:  stdErrors.New("header not found"),
			want: true,
		},
		{
			name: "other error",
			err:  stdErrors.New("boom"),
			want: false,
		},
		{
			name: "annotated other error",
			err:  jujuErrors.Annotatef(stdErrors.New("boom"), "eth_getLogs blockNumber %v", "0x1"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isRetryableGetBlockError(tt.err)
			if got != tt.want {
				t.Fatalf("isRetryableGetBlockError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

type syncIntentTestChain struct {
	parser  bchain.BlockChainParser
	tip     bchain.BlockChain
	intents []bchain.SyncIntent
}

func (c *syncIntentTestChain) BlockChainForSyncIntent(intent bchain.SyncIntent) bchain.BlockChain {
	c.intents = append(c.intents, intent)
	if intent == bchain.SyncIntentChainTip && c.tip != nil {
		return c.tip
	}
	return c
}

func (c *syncIntentTestChain) Initialize() error { return nil }
func (c *syncIntentTestChain) CreateMempool(bchain.BlockChain) (bchain.Mempool, error) {
	return nil, nil
}
func (c *syncIntentTestChain) InitializeMempool(bchain.AddrDescForOutpointFunc, bchain.OnNewTxFunc) error {
	return nil
}
func (c *syncIntentTestChain) Shutdown(ctx context.Context) error         { return nil }
func (c *syncIntentTestChain) IsTestnet() bool                            { return false }
func (c *syncIntentTestChain) GetNetworkName() string                     { return "" }
func (c *syncIntentTestChain) GetSubversion() string                      { return "" }
func (c *syncIntentTestChain) GetCoinName() string                        { return "" }
func (c *syncIntentTestChain) GetChainInfo() (*bchain.ChainInfo, error)   { return nil, nil }
func (c *syncIntentTestChain) GetBestBlockHash() (string, error)          { return "", nil }
func (c *syncIntentTestChain) GetBestBlockHeight() (uint32, error)        { return 0, nil }
func (c *syncIntentTestChain) GetBlockHash(height uint32) (string, error) { return "", nil }
func (c *syncIntentTestChain) GetBlockHeader(hash string) (*bchain.BlockHeader, error) {
	return nil, nil
}
func (c *syncIntentTestChain) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	return nil, nil
}
func (c *syncIntentTestChain) GetBlockInfo(hash string) (*bchain.BlockInfo, error) { return nil, nil }
func (c *syncIntentTestChain) GetBlockRaw(hash string) (string, error)             { return "", nil }
func (c *syncIntentTestChain) GetMempoolTransactions() ([]string, error)           { return nil, nil }
func (c *syncIntentTestChain) GetTransaction(txid string) (*bchain.Tx, error)      { return nil, nil }
func (c *syncIntentTestChain) GetTransactionForMempool(txid string) (*bchain.Tx, error) {
	return nil, nil
}
func (c *syncIntentTestChain) GetTransactionSpecific(tx *bchain.Tx) (json.RawMessage, error) {
	return nil, nil
}
func (c *syncIntentTestChain) GetAddressChainExtraData(addrDesc bchain.AddressDescriptor) (json.RawMessage, error) {
	return nil, nil
}
func (c *syncIntentTestChain) EstimateSmartFee(blocks int, conservative bool) (big.Int, error) {
	return big.Int{}, nil
}
func (c *syncIntentTestChain) EstimateFee(blocks int) (big.Int, error)           { return big.Int{}, nil }
func (c *syncIntentTestChain) LongTermFeeRate() (*bchain.LongTermFeeRate, error) { return nil, nil }
func (c *syncIntentTestChain) SendRawTransaction(tx string, disableAlternativeRPC bool) (string, error) {
	return "", nil
}
func (c *syncIntentTestChain) GetMempoolEntry(txid string) (*bchain.MempoolEntry, error) {
	return nil, nil
}
func (c *syncIntentTestChain) GetContractInfo(contractDesc bchain.AddressDescriptor) (*bchain.ContractInfo, error) {
	return nil, nil
}
func (c *syncIntentTestChain) GetChainParser() bchain.BlockChainParser { return c.parser }
func (c *syncIntentTestChain) EthereumTypeGetBalance(addrDesc bchain.AddressDescriptor) (*big.Int, error) {
	return nil, nil
}
func (c *syncIntentTestChain) EthereumTypeGetNonce(addrDesc bchain.AddressDescriptor) (uint64, error) {
	return 0, nil
}
func (c *syncIntentTestChain) EthereumTypeEstimateGas(params map[string]interface{}) (uint64, error) {
	return 0, nil
}
func (c *syncIntentTestChain) EthereumTypeGetEip1559Fees() (*bchain.Eip1559Fees, error) {
	return nil, nil
}
func (c *syncIntentTestChain) EthereumTypeGetErc20ContractBalance(addrDesc, contractDesc bchain.AddressDescriptor) (*big.Int, error) {
	return nil, nil
}
func (c *syncIntentTestChain) EthereumTypeGetErc20ContractBalances(addrDesc bchain.AddressDescriptor, contractDescs []bchain.AddressDescriptor) ([]*big.Int, error) {
	return nil, nil
}
func (c *syncIntentTestChain) EthereumTypeGetSupportedStakingPools() []string { return nil }
func (c *syncIntentTestChain) EthereumTypeGetStakingPoolsData(addrDesc bchain.AddressDescriptor) ([]bchain.StakingPoolData, error) {
	return nil, nil
}
func (c *syncIntentTestChain) EthereumTypeRpcCall(data, to, from string) (string, error) {
	return "", nil
}
func (c *syncIntentTestChain) EthereumTypeGetRawTransaction(txid string) (string, error) {
	return "", nil
}
func (c *syncIntentTestChain) EthereumTypeGetTransactionReceipt(txid string) (*bchain.RpcReceipt, error) {
	return nil, nil
}
func (c *syncIntentTestChain) GetTokenURI(contractDesc bchain.AddressDescriptor, tokenID *big.Int) (string, error) {
	return "", nil
}

func TestSyncWorkerChainForSyncIntent(t *testing.T) {
	parser := eth.NewEthereumParser(1, false)
	tip := &syncIntentTestChain{parser: parser}
	base := &syncIntentTestChain{parser: parser, tip: tip}
	w := &SyncWorker{chain: base}

	if got := w.chainForSyncIntent(bchain.SyncIntentDefault); got != base {
		t.Fatal("default sync intent did not use base chain")
	}
	if got := w.chainForSyncIntent(bchain.SyncIntentChainTip); got != tip {
		t.Fatal("chain-tip sync intent did not use chain-tip view")
	}
	if len(base.intents) != 2 || base.intents[0] != bchain.SyncIntentDefault || base.intents[1] != bchain.SyncIntentChainTip {
		t.Fatalf("intents = %v, want [default chain-tip]", base.intents)
	}
}

func TestSyncWorkerChainTipIntentSkipsParallelSync(t *testing.T) {
	chain := &syncIntentTestChain{parser: eth.NewEthereumParser(1, false)}
	tip := &syncIntentTestChain{parser: eth.NewEthereumParser(1, false)}
	w := &SyncWorker{chain: chain, syncWorkers: 4}

	if !w.canUseParallelSync(bchain.SyncIntentDefault, false, chain) {
		t.Fatal("default Ethereum sync should allow parallel sync")
	}
	if !w.canUseParallelSync(bchain.SyncIntentChainTip, false, chain) {
		t.Fatal("chain-tip sync on the default chain should keep normal parallel behavior")
	}
	if w.canUseParallelSync(bchain.SyncIntentChainTip, false, tip) {
		t.Fatal("chain-tip sync on a specialized chain view should stay sequential")
	}
}
