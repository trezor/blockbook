package eth

import (
	"math/big"
	"strings"
	"testing"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/trezor/blockbook/bchain"
)

// visibilityHarness wires a provider over a relay that accepts every send and never surfaces it, a
// public node that knows nothing, and a real MempoolEthereumType whose notifications are recorded.
type visibilityHarness struct {
	rpc      *EthereumRPC
	provider *AlternativeSendTxProvider
	pushed   []*bchain.MempoolTx
}

func newVisibilityHarness(t *testing.T, tx *types.Transaction) *visibilityHarness {
	t.Helper()
	relay := newMethodAwareTxProviderTestServer(t, map[string]string{
		"eth_sendRawTransaction":   `{"jsonrpc":"2.0","id":1,"result":"` + tx.Hash().Hex() + `"}`,
		"eth_getTransactionByHash": `{"jsonrpc":"2.0","id":1,"result":null}`,
	})
	provider := &AlternativeSendTxProvider{
		urls:              []string{relay.URL},
		onlyAlternative:   true,
		fetchMempoolTx:    true,
		mempoolTxs:        map[string]storedTx{},
		mempoolTxsTimeout: time.Hour,
		rpcTimeout:        time.Second,
		metrics:           newReconcileTestMetrics(),
	}
	b := &EthereumRPC{RPC: &pendingTxRPC{txs: map[string]*bchain.RpcTransaction{}}, Parser: NewEthereumParser(1, false), Timeout: time.Second, mempoolInitialized: true}
	b.alternativeSendTxProvider = provider
	b.Mempool = bchain.NewMempoolEthereumType(b, time.Hour, false)
	h := &visibilityHarness{rpc: b, provider: provider}
	b.Mempool.OnNewTx = func(mtx *bchain.MempoolTx) { h.pushed = append(h.pushed, mtx) }
	provider.SetupMempool(b.Mempool, b.removeTransactionFromMempool)
	return h
}

func (h *visibilityHarness) send(t *testing.T, rawTx string) string {
	t.Helper()
	txid, err := h.provider.SendRawTransaction(rawTx)
	if err != nil {
		t.Fatalf("SendRawTransaction() error = %v", err)
	}
	h.provider.waitForRefreshes()
	return txid
}

func (h *visibilityHarness) assertIndexed(t *testing.T, txid string, indexed []ethcommon.Address, notIndexed []ethcommon.Address) {
	t.Helper()
	for _, addr := range indexed {
		if got := mempoolTxids(t, h.rpc, addr.Hex()); !sameTxids(got, txid) {
			t.Errorf("mempool txids for %s = %v, want [%s]", addr.Hex(), got, txid)
		}
	}
	for _, addr := range notIndexed {
		if got := mempoolTxids(t, h.rpc, addr.Hex()); len(got) != 0 {
			t.Errorf("mempool txids for %s = %v, want none", addr.Hex(), got)
		}
	}
	if len(h.pushed) != 1 {
		t.Fatalf("OnNewTx calls = %d, want 1", len(h.pushed))
	}
	mtx := h.pushed[0]
	if !mtx.Private {
		t.Error("OnNewTx tx is not marked private")
	}
	var want []string
	for _, addr := range indexed {
		addrDesc, _ := h.rpc.Parser.GetAddrDescFromAddress(addr.Hex())
		want = append(want, string(addrDesc))
	}
	var got []string
	for _, addrDesc := range mtx.NotifyAddrDescs {
		got = append(got, string(addrDesc))
	}
	if !sameTxids(got, want...) {
		t.Errorf("NotifyAddrDescs = %x, want %x", got, want)
	}
	for _, e := range h.rpc.Mempool.GetAllEntries() {
		if e.Txid == txid {
			t.Error("private send is listed by GetAllEntries")
		}
	}
}

// A relay-accepted contract call is indexed and announced under its sender only: a subscription to
// the called contract (a DEX router) must not receive it. The body stays readable by txid.
func TestAlternativeSendContractCallIsVisibleUnderSenderOnly(t *testing.T) {
	router := ethcommon.HexToAddress("0x3fC91A3afd70395Cd496C647d5a6CC9D4B2b7FAD")
	calldata := ethcommon.FromHex("0x3593564c" + strings.Repeat("ab", 96))
	rawTx, sender, tx := signTestTx(t, &types.DynamicFeeTx{
		ChainID:   big.NewInt(1),
		Nonce:     7,
		GasTipCap: big.NewInt(2_000_000_000),
		GasFeeCap: big.NewInt(50_000_000_000),
		Gas:       250000,
		To:        &router,
		Value:     new(big.Int).Mul(big.NewInt(10), big.NewInt(1_000_000_000_000_000_000)),
		Data:      calldata,
	})
	h := newVisibilityHarness(t, tx)
	txid := h.send(t, rawTx)

	h.assertIndexed(t, txid, []ethcommon.Address{sender}, []ethcommon.Address{router})

	got, err := h.rpc.GetTransaction(txid)
	if err != nil {
		t.Fatalf("GetTransaction(%s) error = %v", txid, err)
	}
	if csd, ok := got.CoinSpecificData.(bchain.EthereumSpecificData); !ok || csd.Tx.Payload != "0x"+ethcommon.Bytes2Hex(calldata) {
		t.Errorf("GetTransaction by txid does not return the cached body")
	}
}

// A plain value transfer is hidden from its recipient too: a watched recipient address would
// otherwise reveal the send before inclusion.
func TestAlternativeSendValueTransferIsHiddenFromRecipient(t *testing.T) {
	recipient := ethcommon.HexToAddress("0x3333333333333333333333333333333333333333")
	rawTx, sender, tx := signTestTx(t, &types.DynamicFeeTx{
		ChainID:   big.NewInt(1),
		Nonce:     8,
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(2),
		Gas:       21000,
		To:        &recipient,
		Value:     big.NewInt(1),
	})
	h := newVisibilityHarness(t, tx)
	txid := h.send(t, rawTx)

	h.assertIndexed(t, txid, []ethcommon.Address{sender}, []ethcommon.Address{recipient})
}

// A token transfer is indexed neither under the token contract nor under the counterparty parsed
// from the calldata; the parsed transfer still rides on the sender's notification.
func TestAlternativeSendTokenTransferIsHiddenFromCounterparty(t *testing.T) {
	token := ethcommon.HexToAddress("0x4af4114F73d1c1C903aC9E0361b379D1291808A2")
	recipient := ethcommon.HexToAddress("0x555ee11fbddc0e49a9bab358a8941ad95ffdb48f")
	calldata := ethcommon.FromHex("0xa9059cbb000000000000000000000000555ee11fbddc0e49a9bab358a8941ad95ffdb48f00000000000000000000000000000000000000000000021e19e0c9bab2400000")
	rawTx, sender, tx := signTestTx(t, &types.DynamicFeeTx{
		ChainID:   big.NewInt(1),
		Nonce:     9,
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(2),
		Gas:       60000,
		To:        &token,
		Data:      calldata,
	})
	h := newVisibilityHarness(t, tx)
	txid := h.send(t, rawTx)

	h.assertIndexed(t, txid, []ethcommon.Address{sender}, []ethcommon.Address{token, recipient})
	if tt := h.pushed[0].TokenTransfers; len(tt) != 1 || tt[0].To != recipient.Hex() {
		t.Errorf("sender notification token transfers = %+v, want one to %s", tt, recipient.Hex())
	}
}
