package btc

import (
	"blockbook/bchain"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	"github.com/btcsuite/btcd/wire"

	"github.com/golang/glog"
	"github.com/juju/errors"
)

// BitcoinRPC is an interface to JSON-RPC bitcoind service.
type BitcoinRPC struct {
	client      http.Client
	rpcURL      string
	user        string
	password    string
	Parser      *BitcoinBlockParser
	Testnet     bool
	Network     string
	Mempool     *bchain.Mempool
	ParseBlocks bool
}

// NewBitcoinRPC returns new BitcoinRPC instance.
func NewBitcoinRPC(url string, user string, password string, timeout time.Duration, parse bool) (bchain.BlockChain, error) {
	transport := &http.Transport{
		Dial:                (&net.Dialer{KeepAlive: 600 * time.Second}).Dial,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100, // necessary to not to deplete ports
	}
	s := &BitcoinRPC{
		client:      http.Client{Timeout: timeout, Transport: transport},
		rpcURL:      url,
		user:        user,
		password:    password,
		ParseBlocks: parse,
	}
	chainName, err := s.GetBlockChainInfo()
	if err != nil {
		return nil, err
	}

	// always create parser
	s.Parser = &BitcoinBlockParser{
		Params: GetChainParams(chainName),
	}

	// parameters for getInfo request
	if s.Parser.Params.Net == wire.MainNet {
		s.Testnet = false
		s.Network = "livenet"
	} else {
		s.Testnet = true
		s.Network = "testnet"
	}

	s.Mempool = bchain.NewMempool(s)

	glog.Info("rpc: block chain ", s.Parser.Params.Name)
	return s, nil
}

func (b *BitcoinRPC) IsTestnet() bool {
	return b.Testnet
}

func (b *BitcoinRPC) GetNetworkName() string {
	return b.Network
}

// getblockhash

type cmdGetBlockHash struct {
	Method string `json:"method"`
	Params struct {
		Height uint32 `json:"height"`
	} `json:"params"`
}

type resGetBlockHash struct {
	Error  *bchain.RPCError `json:"error"`
	Result string           `json:"result"`
}

// getbestblockhash

type cmdGetBestBlockHash struct {
	Method string `json:"method"`
}

type resGetBestBlockHash struct {
	Error  *bchain.RPCError `json:"error"`
	Result string           `json:"result"`
}

// getblockcount

type cmdGetBlockCount struct {
	Method string `json:"method"`
}

type resGetBlockCount struct {
	Error  *bchain.RPCError `json:"error"`
	Result uint32           `json:"result"`
}

// getblockchaininfo

type cmdGetBlockChainInfo struct {
	Method string `json:"method"`
}

type resGetBlockChainInfo struct {
	Error  *bchain.RPCError `json:"error"`
	Result struct {
		Chain         string `json:"chain"`
		Blocks        int    `json:"blocks"`
		Headers       int    `json:"headers"`
		Bestblockhash string `json:"bestblockhash"`
	} `json:"result"`
}

// getrawmempool

type cmdGetMempool struct {
	Method string `json:"method"`
}

type resGetMempool struct {
	Error  *bchain.RPCError `json:"error"`
	Result []string         `json:"result"`
}

// getblockheader

type cmdGetBlockHeader struct {
	Method string `json:"method"`
	Params struct {
		BlockHash string `json:"blockhash"`
		Verbose   bool   `json:"verbose"`
	} `json:"params"`
}

type resGetBlockHeaderRaw struct {
	Error  *bchain.RPCError `json:"error"`
	Result string           `json:"result"`
}

type resGetBlockHeaderVerbose struct {
	Error  *bchain.RPCError   `json:"error"`
	Result bchain.BlockHeader `json:"result"`
}

// getblock

type cmdGetBlock struct {
	Method string `json:"method"`
	Params struct {
		BlockHash string `json:"blockhash"`
		Verbosity int    `json:"verbosity"`
	} `json:"params"`
}

type resGetBlockRaw struct {
	Error  *bchain.RPCError `json:"error"`
	Result string           `json:"result"`
}

type resGetBlockThin struct {
	Error  *bchain.RPCError `json:"error"`
	Result bchain.ThinBlock `json:"result"`
}

type resGetBlockFull struct {
	Error  *bchain.RPCError `json:"error"`
	Result bchain.Block     `json:"result"`
}

// getrawtransaction

type cmdGetRawTransaction struct {
	Method string `json:"method"`
	Params struct {
		Txid    string `json:"txid"`
		Verbose bool   `json:"verbose"`
	} `json:"params"`
}

type resGetRawTransactionRaw struct {
	Error  *bchain.RPCError `json:"error"`
	Result string           `json:"result"`
}

type resGetRawTransactionVerbose struct {
	Error  *bchain.RPCError `json:"error"`
	Result bchain.Tx        `json:"result"`
}

// estimatesmartfee

type cmdEstimateSmartFee struct {
	Method string `json:"method"`
	Params struct {
		ConfTarget   int    `json:"conf_target"`
		EstimateMode string `json:"estimate_mode"`
	} `json:"params"`
}

type resEstimateSmartFee struct {
	Error  *bchain.RPCError `json:"error"`
	Result struct {
		Feerate float64 `json:"feerate"`
		Blocks  int     `json:"blocks"`
	} `json:"result"`
}

// sendrawtransaction

type cmdSendRawTransaction struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
}

type resSendRawTransaction struct {
	Error  *bchain.RPCError `json:"error"`
	Result string           `json:"result"`
}

// getmempoolentry

type cmdGetMempoolEntry struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
}

type resGetMempoolEntry struct {
	Error  *bchain.RPCError     `json:"error"`
	Result *bchain.MempoolEntry `json:"result"`
}

// GetBestBlockHash returns hash of the tip of the best-block-chain.
func (b *BitcoinRPC) GetBestBlockHash() (string, error) {

	glog.V(1).Info("rpc: getbestblockhash")

	res := resGetBestBlockHash{}
	req := cmdGetBestBlockHash{Method: "getbestblockhash"}
	err := b.call(&req, &res)

	if err != nil {
		return "", err
	}
	if res.Error != nil {
		return "", res.Error
	}
	return res.Result, nil
}

// GetBestBlockHeight returns height of the tip of the best-block-chain.
func (b *BitcoinRPC) GetBestBlockHeight() (uint32, error) {
	glog.V(1).Info("rpc: getblockcount")

	res := resGetBlockCount{}
	req := cmdGetBlockCount{Method: "getblockcount"}
	err := b.call(&req, &res)

	if err != nil {
		return 0, err
	}
	if res.Error != nil {
		return 0, res.Error
	}
	return res.Result, nil
}

// GetBlockChainInfo returns the name of the block chain: main/test/regtest.
func (b *BitcoinRPC) GetBlockChainInfo() (string, error) {
	glog.V(1).Info("rpc: getblockchaininfo")

	res := resGetBlockChainInfo{}
	req := cmdGetBlockChainInfo{Method: "getblockchaininfo"}
	err := b.call(&req, &res)

	if err != nil {
		return "", err
	}
	if res.Error != nil {
		return "", res.Error
	}
	return res.Result.Chain, nil
}

// GetBlockHash returns hash of block in best-block-chain at given height.
func (b *BitcoinRPC) GetBlockHash(height uint32) (string, error) {
	glog.V(1).Info("rpc: getblockhash ", height)

	res := resGetBlockHash{}
	req := cmdGetBlockHash{Method: "getblockhash"}
	req.Params.Height = height
	err := b.call(&req, &res)

	if err != nil {
		return "", errors.Annotatef(err, "height %v", height)
	}
	if res.Error != nil {
		return "", errors.Annotatef(res.Error, "height %v", height)
	}
	return res.Result, nil
}

// GetBlockHeader returns header of block with given hash.
func (b *BitcoinRPC) GetBlockHeader(hash string) (*bchain.BlockHeader, error) {
	glog.V(1).Info("rpc: getblockheader")

	res := resGetBlockHeaderVerbose{}
	req := cmdGetBlockHeader{Method: "getblockheader"}
	req.Params.BlockHash = hash
	req.Params.Verbose = true
	err := b.call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}
	return &res.Result, nil
}

// GetBlock returns block with given hash.
func (b *BitcoinRPC) GetBlock(hash string) (*bchain.Block, error) {
	if !b.ParseBlocks {
		return b.GetBlockFull(hash)
	}
	header, err := b.GetBlockHeader(hash)
	if err != nil {
		return nil, err
	}
	data, err := b.GetBlockRaw(hash)
	if err != nil {
		return nil, err
	}
	block, err := b.Parser.ParseBlock(data)
	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	block.BlockHeader = *header
	return block, nil
}

// GetBlockWithoutHeader is an optimization - it does not call GetBlockHeader to get prev, next hashes
// instead it sets to header only block hash and height passed in parameters
func (b *BitcoinRPC) GetBlockWithoutHeader(hash string, height uint32) (*bchain.Block, error) {
	if !b.ParseBlocks {
		return b.GetBlockFull(hash)
	}
	data, err := b.GetBlockRaw(hash)
	if err != nil {
		return nil, err
	}
	block, err := b.Parser.ParseBlock(data)
	if err != nil {
		return nil, errors.Annotatef(err, "%v %v", height, hash)
	}
	block.BlockHeader.Hash = hash
	block.BlockHeader.Height = height
	return block, nil
}

// GetBlockRaw returns block with given hash as bytes.
func (b *BitcoinRPC) GetBlockRaw(hash string) ([]byte, error) {
	glog.V(1).Info("rpc: getblock (verbosity=0) ", hash)

	res := resGetBlockRaw{}
	req := cmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = 0
	err := b.call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}
	return hex.DecodeString(res.Result)
}

// GetBlockList returns block with given hash by downloading block
// transactions one by one.
func (b *BitcoinRPC) GetBlockList(hash string) (*bchain.Block, error) {
	glog.V(1).Info("rpc: getblock (verbosity=1) ", hash)

	res := resGetBlockThin{}
	req := cmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = 1
	err := b.call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}

	txs := make([]bchain.Tx, len(res.Result.Txids))
	for i, txid := range res.Result.Txids {
		tx, err := b.GetTransaction(txid)
		if err != nil {
			return nil, err
		}
		txs[i] = *tx
	}
	block := &bchain.Block{
		BlockHeader: res.Result.BlockHeader,
		Txs:         txs,
	}
	return block, nil
}

// GetBlockFull returns block with given hash.
func (b *BitcoinRPC) GetBlockFull(hash string) (*bchain.Block, error) {
	glog.V(1).Info("rpc: getblock (verbosity=2) ", hash)

	res := resGetBlockFull{}
	req := cmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = 2
	err := b.call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}
	return &res.Result, nil
}

// GetMempool returns transactions in mempool.
func (b *BitcoinRPC) GetMempool() ([]string, error) {
	glog.V(1).Info("rpc: getrawmempool")

	res := resGetMempool{}
	req := cmdGetMempool{Method: "getrawmempool"}
	err := b.call(&req, &res)

	if err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, res.Error
	}
	return res.Result, nil
}

// GetTransaction returns a transaction by the transaction ID.
func (b *BitcoinRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	glog.V(1).Info("rpc: getrawtransaction ", txid)

	res := resGetRawTransactionVerbose{}
	req := cmdGetRawTransaction{Method: "getrawtransaction"}
	req.Params.Txid = txid
	req.Params.Verbose = true
	err := b.call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "txid %v", txid)
	}
	return &res.Result, nil
}

// ResyncMempool gets mempool transactions and maps output scripts to transactions.
// ResyncMempool is not reentrant, it should be called from a single thread.
func (b *BitcoinRPC) ResyncMempool(onNewTxAddr func(txid string, addr string)) error {
	return b.Mempool.Resync(onNewTxAddr)
}

// GetMempoolTransactions returns slice of mempool transactions for given output script.
func (b *BitcoinRPC) GetMempoolTransactions(outputScript []byte) ([]string, error) {
	return b.Mempool.GetTransactions(outputScript)
}

// GetMempoolSpentOutput returns transaction in mempool which spends given outpoint
func (b *BitcoinRPC) GetMempoolSpentOutput(outputTxid string, vout uint32) string {
	return b.Mempool.GetSpentOutput(outputTxid, vout)
}

// EstimateSmartFee returns fee estimation.
func (b *BitcoinRPC) EstimateSmartFee(blocks int, conservative bool) (float64, error) {
	glog.V(1).Info("rpc: estimatesmartfee ", blocks)

	res := resEstimateSmartFee{}
	req := cmdEstimateSmartFee{Method: "estimatesmartfee"}
	req.Params.ConfTarget = blocks
	if conservative {
		req.Params.EstimateMode = "CONSERVATIVE"
	} else {
		req.Params.EstimateMode = "ECONOMICAL"
	}
	err := b.call(&req, &res)

	if err != nil {
		return 0, err
	}
	if res.Error != nil {
		return 0, res.Error
	}
	return res.Result.Feerate, nil
}

// SendRawTransaction sends raw transaction.
func (b *BitcoinRPC) SendRawTransaction(tx string) (string, error) {
	glog.V(1).Info("rpc: sendrawtransaction")

	res := resSendRawTransaction{}
	req := cmdSendRawTransaction{Method: "sendrawtransaction"}
	req.Params = []string{tx}
	err := b.call(&req, &res)

	if err != nil {
		return "", err
	}
	if res.Error != nil {
		return "", res.Error
	}
	return res.Result, nil
}

func (b *BitcoinRPC) GetMempoolEntry(txid string) (*bchain.MempoolEntry, error) {
	glog.V(1).Info("rpc: getmempoolentry")

	res := resGetMempoolEntry{}
	req := cmdGetMempoolEntry{
		Method: "getmempoolentry",
		Params: []string{txid},
	}
	err := b.call(&req, &res)

	if err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, res.Error
	}
	return res.Result, nil
}

func (b *BitcoinRPC) call(req interface{}, res interface{}) error {
	httpData, err := json.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequest("POST", b.rpcURL, bytes.NewBuffer(httpData))
	if err != nil {
		return err
	}
	httpReq.SetBasicAuth(b.user, b.password)
	httpRes, err := b.client.Do(httpReq)
	// in some cases the httpRes can contain data even if it returns error
	// see http://devs.cloudimmunity.com/gotchas-and-common-mistakes-in-go-golang/
	if httpRes != nil {
		defer httpRes.Body.Close()
	}
	if err != nil {
		return err
	}
	// read the entire response body until the end to avoid memory leak when reusing http connection
	// see http://devs.cloudimmunity.com/gotchas-and-common-mistakes-in-go-golang/
	defer io.Copy(ioutil.Discard, httpRes.Body)
	return json.NewDecoder(httpRes.Body).Decode(&res)
}

// GetChainParser returns BlockChainParser
func (b *BitcoinRPC) GetChainParser() bchain.BlockChainParser {
	return b.Parser
}
