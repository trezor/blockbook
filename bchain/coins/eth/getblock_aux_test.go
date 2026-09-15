//go:build unittest

package eth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/trezor/blockbook/bchain"
)

// recordingBlockRPC serves one block and records every JSON-RPC method GetBlock issues.
type recordingBlockRPC struct {
	mu      sync.Mutex
	block   json.RawMessage
	txCount int
	methods []string
}

func (m *recordingBlockRPC) EthSubscribe(context.Context, interface{}, ...interface{}) (bchain.EVMClientSubscription, error) {
	return nil, errors.New("not implemented")
}

func (m *recordingBlockRPC) Close() {}

func (m *recordingBlockRPC) BatchCallContext(context.Context, []rpc.BatchElem) error {
	return errors.New("not implemented")
}

func (m *recordingBlockRPC) CallContext(_ context.Context, result interface{}, method string, _ ...interface{}) error {
	m.mu.Lock()
	m.methods = append(m.methods, method)
	m.mu.Unlock()
	switch method {
	case "eth_getBlockByHash":
		*(result.(*json.RawMessage)) = m.block
	case "debug_traceBlockByHash":
		*(result.(*[]rpcTraceResult)) = make([]rpcTraceResult, m.txCount)
	}
	return nil
}

func (m *recordingBlockRPC) calls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.methods...)
}

func newGetBlockTestRPC(t *testing.T, txs []bchain.RpcTransaction) (*EthereumRPC, *recordingBlockRPC) {
	t.Helper()
	block := struct {
		rpcHeader
		rpcBlockTransactions
	}{
		rpcHeader: rpcHeader{
			Hash:       "0x" + strings.Repeat("aa", 32),
			ParentHash: "0x" + strings.Repeat("bb", 32),
			Difficulty: "0x1",
			Number:     "0x41eee8",
			Time:       "0x5b7c9f26",
			Size:       "0x200",
		},
		rpcBlockTransactions: rpcBlockTransactions{Transactions: txs},
	}
	raw, err := json.Marshal(block)
	if err != nil {
		t.Fatal(err)
	}
	mock := &recordingBlockRPC{block: raw, txCount: len(txs)}
	b := &EthereumRPC{
		RPC:         mock,
		Timeout:     time.Second,
		Parser:      NewEthereumParser(1, false),
		ChainConfig: &Configuration{},
		bestHeader:  stubHeader{n: 0x41eee8},
	}
	return b, mock
}

func countCalls(calls []string, method string) int {
	n := 0
	for _, c := range calls {
		if c == method {
			n++
		}
	}
	return n
}

func TestGetBlockEmptyBlockSkipsLogsAndTrace(t *testing.T) {
	prev := bchain.ProcessInternalTransactions
	bchain.ProcessInternalTransactions = true
	defer func() { bchain.ProcessInternalTransactions = prev }()

	b, mock := newGetBlockTestRPC(t, nil)
	block, err := b.GetBlock("0x"+strings.Repeat("aa", 32), 0)
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if len(block.Txs) != 0 || block.Height != 0x41eee8 {
		t.Fatalf("unexpected block: height %d, %d txs", block.Height, len(block.Txs))
	}
	calls := mock.calls()
	if len(calls) != 1 || calls[0] != "eth_getBlockByHash" {
		t.Fatalf("RPC calls = %v, want only eth_getBlockByHash for an empty block", calls)
	}
}

func TestGetBlockWithTransactionsFetchesLogsAndTrace(t *testing.T) {
	prev := bchain.ProcessInternalTransactions
	bchain.ProcessInternalTransactions = true
	defer func() { bchain.ProcessInternalTransactions = prev }()

	tx := *testTx1.CoinSpecificData.(bchain.EthereumSpecificData).Tx
	b, mock := newGetBlockTestRPC(t, []bchain.RpcTransaction{tx})
	block, err := b.GetBlock("0x"+strings.Repeat("aa", 32), 0)
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if len(block.Txs) != 1 {
		t.Fatalf("got %d txs, want 1", len(block.Txs))
	}
	calls := mock.calls()
	for _, method := range []string{"eth_getBlockByHash", "eth_getLogs", "debug_traceBlockByHash"} {
		if countCalls(calls, method) != 1 {
			t.Fatalf("RPC calls = %v, want exactly one %s", calls, method)
		}
	}
}
