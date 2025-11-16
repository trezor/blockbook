package eth

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
)

type storedTx struct {
	tx   *bchain.RpcTransaction
	time uint32
}

// AlternativeSendTxProvider handles sending transactions to alternative providers
type AlternativeSendTxProvider struct {
	urls                         []string
	onlyAlternative              bool
	fetchMempoolTx               bool
	mempoolTxs                   map[string]storedTx
	mempoolTxsMux                sync.Mutex
	mempoolTxsTimeout            time.Duration
	rpcTimeout                   time.Duration
	mempool                      *bchain.MempoolEthereumType
	removeTransactionFromMempool func(string)
}

// NewAlternativeSendTxProvider creates a new alternative send tx provider if enabled
func NewAlternativeSendTxProvider(network string, rpcTimeout int, mempoolTxsTimeout int) *AlternativeSendTxProvider {
	urls := strings.Split(os.Getenv(strings.ToUpper(network)+"_ALTERNATIVE_SENDTX_URLS"), ",")
	onlyAlternative := strings.ToUpper(os.Getenv(strings.ToUpper(network)+"_ALTERNATIVE_SENDTX_ONLY")) == "TRUE"
	fetchMempoolTx := strings.ToUpper(os.Getenv(strings.ToUpper(network)+"_ALTERNATIVE_FETCH_MEMPOOL_TX")) == "TRUE"
	if len(urls) == 0 || urls[0] == "" {
		return nil
	}

	provider := &AlternativeSendTxProvider{
		urls:              urls,
		onlyAlternative:   onlyAlternative,
		fetchMempoolTx:    fetchMempoolTx,
		rpcTimeout:        time.Duration(rpcTimeout) * time.Second,
		mempoolTxsTimeout: time.Duration(mempoolTxsTimeout) * time.Hour,
		mempoolTxs:        make(map[string]storedTx),
	}

	glog.Infof("Using alternative send transaction providers %v. Only alternative providers %v", urls, onlyAlternative)
	if fetchMempoolTx {
		glog.Infof("Alternative fetch mempool tx %v", fetchMempoolTx)
	}

	return provider
}

// SetupMempool sets up connection to the mempool
func (p *AlternativeSendTxProvider) SetupMempool(mempool *bchain.MempoolEthereumType, removeTransactionFromMempool func(string)) {
	p.mempool = mempool
	p.removeTransactionFromMempool = removeTransactionFromMempool
}

// SendRawTransaction sends raw transaction to alternative providers
func (p *AlternativeSendTxProvider) SendRawTransaction(hex string) (string, error) {
	var txid string
	var retErr error

	for i := range p.urls {
		r, err := p.callHttpStringResult(p.urls[i], "eth_sendRawTransaction", hex)
		glog.Infof("eth_sendRawTransaction to %s, txid %s", p.urls[i], r)
		// set success return value; or error only if there was no previous success
		if err == nil || len(txid) == 0 {
			txid = r
			retErr = err
		}
	}

	if p.onlyAlternative && p.fetchMempoolTx {
		p.handleMempoolTransaction(txid)
	}

	return txid, retErr
}

// handleMempoolTransaction handles the transaction when using only alternative providers
func (p *AlternativeSendTxProvider) handleMempoolTransaction(txid string) (string, error) {
	hash := ethcommon.HexToHash(txid)
	raw, err := p.callHttpRawResult(p.urls[0], "eth_getTransactionByHash", hash)
	if err != nil || raw == nil {
		glog.Errorf("eth_getTransactionByHash from %s returned error %v", p.urls[0], err)
		return txid, err
	}

	var tx bchain.RpcTransaction
	if err := json.Unmarshal(raw, &tx); err != nil {
		glog.Errorf("eth_getTransactionByHash from %s unmarshal returned error %v", p.urls[0], err)
		return txid, err
	}

	p.mempoolTxsMux.Lock()
	// remove potential RBF transactions - with equal from and nonce
	var rbfTxid string
	for rbf, storedTx := range p.mempoolTxs {
		if storedTx.tx.From == tx.From && storedTx.tx.AccountNonce == tx.AccountNonce {
			rbfTxid = rbf
			break
		}
	}
	p.mempoolTxs[txid] = storedTx{tx: &tx, time: uint32(time.Now().Unix())}
	p.mempoolTxsMux.Unlock()

	if rbfTxid != "" {
		glog.Infof("eth_sendRawTransaction replacing txid %s by %s", rbfTxid, txid)
		if p.removeTransactionFromMempool != nil {
			p.removeTransactionFromMempool(rbfTxid)
		}
	}

	if p.mempool != nil {
		p.mempool.AddTransactionToMempool(txid)
	}

	return txid, nil
}

// GetTransaction gets a transaction from alternative mempool cache
func (p *AlternativeSendTxProvider) GetTransaction(txid string) (*bchain.RpcTransaction, bool) {
	if !p.fetchMempoolTx {
		return nil, false
	}

	var storedTx storedTx
	var found bool

	p.mempoolTxsMux.Lock()
	storedTx, found = p.mempoolTxs[txid]
	p.mempoolTxsMux.Unlock()

	if found {
		if time.Unix(int64(storedTx.time), 0).Before(time.Now().Add(-p.mempoolTxsTimeout)) {
			p.mempoolTxsMux.Lock()
			delete(p.mempoolTxs, txid)
			p.mempoolTxsMux.Unlock()
			return nil, false
		}
		return storedTx.tx, true
	}

	return nil, false
}

// RemoveTransaction removes a transaction from alternative mempool cache
func (p *AlternativeSendTxProvider) RemoveTransaction(txid string) {
	if !p.fetchMempoolTx {
		return
	}

	p.mempoolTxsMux.Lock()
	delete(p.mempoolTxs, txid)
	p.mempoolTxsMux.Unlock()
}

// UseOnlyAlternativeProvider returns true if only alternative providers should be used
func (p *AlternativeSendTxProvider) UseOnlyAlternativeProvider() bool {
	return p.onlyAlternative
}

// Helper function for calling ETH RPC over http with parameters. Creates and closes a new client for every call.
func (p *AlternativeSendTxProvider) callHttpRawResult(url string, rpcMethod string, args ...interface{}) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.rpcTimeout)
	defer cancel()
	client, err := rpc.DialContext(ctx, url)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	var raw json.RawMessage
	err = client.CallContext(ctx, &raw, rpcMethod, args...)
	if err != nil {
		return nil, err
	} else if len(raw) == 0 {
		return nil, errors.New(url + " " + rpcMethod + " : failed")
	}
	return raw, nil
}

// Helper function for calling ETH RPC over http with parameters and getting string result. Creates and closes a new client for every call.
func (p *AlternativeSendTxProvider) callHttpStringResult(url string, rpcMethod string, args ...interface{}) (string, error) {
	raw, err := p.callHttpRawResult(url, rpcMethod, args...)
	if err != nil {
		return "", err
	}
	var result string
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", errors.Annotatef(err, "%s %s raw result %v", url, rpcMethod, raw)
	}
	if result == "" {
		return "", errors.New(url + " " + rpcMethod + " : failed, empty result")
	}
	return result, nil
}
