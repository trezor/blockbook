package btc

import (
	"blockbook/bchain"
	"bytes"
	"context"
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
	client       http.Client
	rpcURL       string
	user         string
	password     string
	Parser       bchain.BlockChainParser
	Testnet      bool
	Network      string
	Mempool      *bchain.UTXOMempool
	ParseBlocks  bool
	pushHandler  func(bchain.NotificationType)
	mq           *bchain.MQ
	ChainConfig  *Configuration
	RPCMarshaler RPCMarshaler
}

type Configuration struct {
	CoinName             string `json:"coin_name"`
	RPCURL               string `json:"rpc_url"`
	RPCUser              string `json:"rpc_user"`
	RPCPass              string `json:"rpc_pass"`
	RPCTimeout           int    `json:"rpc_timeout"`
	Parse                bool   `json:"parse"`
	MessageQueueBinding  string `json:"message_queue_binding"`
	Subversion           string `json:"subversion"`
	BlockAddressesToKeep int    `json:"block_addresses_to_keep"`
	MempoolWorkers       int    `json:"mempool_workers"`
	MempoolSubWorkers    int    `json:"mempool_sub_workers"`
	AddressFormat        string `json:"address_format"`
}

// NewBitcoinRPC returns new BitcoinRPC instance.
func NewBitcoinRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	var err error
	var c Configuration
	err = json.Unmarshal(config, &c)
	if err != nil {
		return nil, errors.Annotatef(err, "Invalid configuration file")
	}
	// keep at least 100 mappings block->addresses to allow rollback
	if c.BlockAddressesToKeep < 100 {
		c.BlockAddressesToKeep = 100
	}
	// at least 1 mempool worker/subworker for synchronous mempool synchronization
	if c.MempoolWorkers < 1 {
		c.MempoolWorkers = 1
	}
	if c.MempoolSubWorkers < 1 {
		c.MempoolSubWorkers = 1
	}

	transport := &http.Transport{
		Dial:                (&net.Dialer{KeepAlive: 600 * time.Second}).Dial,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100, // necessary to not to deplete ports
	}

	s := &BitcoinRPC{
		client:       http.Client{Timeout: time.Duration(c.RPCTimeout) * time.Second, Transport: transport},
		rpcURL:       c.RPCURL,
		user:         c.RPCUser,
		password:     c.RPCPass,
		ParseBlocks:  c.Parse,
		ChainConfig:  &c,
		pushHandler:  pushHandler,
		RPCMarshaler: JSONMarshalerV2{},
	}

	return s, nil
}

// GetChainInfoAndInitializeMempool is called by Initialize and reused by other coins
// it contacts the blockchain rpc interface for the first time
// and if successful it connects to ZeroMQ and creates mempool handler
func (b *BitcoinRPC) GetChainInfoAndInitializeMempool(bc bchain.BlockChain) (string, error) {
	// try to connect to block chain and get some info
	chainName, err := bc.GetBlockChainInfo()
	if err != nil {
		return "", err
	}

	mq, err := bchain.NewMQ(b.ChainConfig.MessageQueueBinding, b.pushHandler)
	if err != nil {
		glog.Error("mq: ", err)
		return "", err
	}
	b.mq = mq

	b.Mempool = bchain.NewUTXOMempool(bc, b.ChainConfig.MempoolWorkers, b.ChainConfig.MempoolSubWorkers)

	return chainName, nil
}

// Initialize initializes BitcoinRPC instance.
func (b *BitcoinRPC) Initialize() error {

	chainName, err := b.GetChainInfoAndInitializeMempool(b)
	if err != nil {
		return err
	}

	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewBitcoinParser(params, b.ChainConfig)

	// parameters for getInfo request
	if params.Net == wire.MainNet {
		b.Testnet = false
		b.Network = "livenet"
	} else {
		b.Testnet = true
		b.Network = "testnet"
	}

	glog.Info("rpc: block chain ", params.Name)

	return nil
}

func (b *BitcoinRPC) Shutdown(ctx context.Context) error {
	if b.mq != nil {
		if err := b.mq.Shutdown(ctx); err != nil {
			glog.Error("MQ.Shutdown error: ", err)
			return err
		}
	}
	return nil
}

func (b *BitcoinRPC) IsTestnet() bool {
	return b.Testnet
}

func (b *BitcoinRPC) GetNetworkName() string {
	return b.Network
}

func (b *BitcoinRPC) GetCoinName() string {
	return b.ChainConfig.CoinName
}

func (b *BitcoinRPC) GetSubversion() string {
	return b.ChainConfig.Subversion
}

// getblockhash

type CmdGetBlockHash struct {
	Method string `json:"method"`
	Params struct {
		Height uint32 `json:"height"`
	} `json:"params"`
}

type ResGetBlockHash struct {
	Error  *bchain.RPCError `json:"error"`
	Result string           `json:"result"`
}

// getbestblockhash

type CmdGetBestBlockHash struct {
	Method string `json:"method"`
}

type ResGetBestBlockHash struct {
	Error  *bchain.RPCError `json:"error"`
	Result string           `json:"result"`
}

// getblockcount

type CmdGetBlockCount struct {
	Method string `json:"method"`
}

type ResGetBlockCount struct {
	Error  *bchain.RPCError `json:"error"`
	Result uint32           `json:"result"`
}

// getblockchaininfo

type CmdGetBlockChainInfo struct {
	Method string `json:"method"`
}

type ResGetBlockChainInfo struct {
	Error  *bchain.RPCError `json:"error"`
	Result struct {
		Chain         string `json:"chain"`
		Blocks        int    `json:"blocks"`
		Headers       int    `json:"headers"`
		Bestblockhash string `json:"bestblockhash"`
	} `json:"result"`
}

// getrawmempool

type CmdGetMempool struct {
	Method string `json:"method"`
}

type ResGetMempool struct {
	Error  *bchain.RPCError `json:"error"`
	Result []string         `json:"result"`
}

// getblockheader

type CmdGetBlockHeader struct {
	Method string `json:"method"`
	Params struct {
		BlockHash string `json:"blockhash"`
		Verbose   bool   `json:"verbose"`
	} `json:"params"`
}

type ResGetBlockHeader struct {
	Error  *bchain.RPCError   `json:"error"`
	Result bchain.BlockHeader `json:"result"`
}

// getblock

type CmdGetBlock struct {
	Method string `json:"method"`
	Params struct {
		BlockHash string `json:"blockhash"`
		Verbosity int    `json:"verbosity"`
	} `json:"params"`
}

type ResGetBlockRaw struct {
	Error  *bchain.RPCError `json:"error"`
	Result string           `json:"result"`
}

type ResGetBlockThin struct {
	Error  *bchain.RPCError `json:"error"`
	Result bchain.ThinBlock `json:"result"`
}

type ResGetBlockFull struct {
	Error  *bchain.RPCError `json:"error"`
	Result bchain.Block     `json:"result"`
}

// getrawtransaction

type CmdGetRawTransaction struct {
	Method string `json:"method"`
	Params struct {
		Txid    string `json:"txid"`
		Verbose bool   `json:"verbose"`
	} `json:"params"`
}

type ResGetRawTransaction struct {
	Error  *bchain.RPCError `json:"error"`
	Result json.RawMessage  `json:"result"`
}

type ResGetRawTransactionNonverbose struct {
	Error  *bchain.RPCError `json:"error"`
	Result string           `json:"result"`
}

// estimatesmartfee

type CmdEstimateSmartFee struct {
	Method string `json:"method"`
	Params struct {
		ConfTarget   int    `json:"conf_target"`
		EstimateMode string `json:"estimate_mode"`
	} `json:"params"`
}

type ResEstimateSmartFee struct {
	Error  *bchain.RPCError `json:"error"`
	Result struct {
		Feerate float64 `json:"feerate"`
		Blocks  int     `json:"blocks"`
	} `json:"result"`
}

// estimatefee

type CmdEstimateFee struct {
	Method string `json:"method"`
	Params struct {
		Blocks int `json:"nblocks"`
	} `json:"params"`
}

type ResEstimateFee struct {
	Error  *bchain.RPCError `json:"error"`
	Result float64          `json:"result"`
}

// sendrawtransaction

type CmdSendRawTransaction struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
}

type ResSendRawTransaction struct {
	Error  *bchain.RPCError `json:"error"`
	Result string           `json:"result"`
}

// getmempoolentry

type CmdGetMempoolEntry struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
}

type ResGetMempoolEntry struct {
	Error  *bchain.RPCError     `json:"error"`
	Result *bchain.MempoolEntry `json:"result"`
}

// GetBestBlockHash returns hash of the tip of the best-block-chain.
func (b *BitcoinRPC) GetBestBlockHash() (string, error) {

	glog.V(1).Info("rpc: getbestblockhash")

	res := ResGetBestBlockHash{}
	req := CmdGetBestBlockHash{Method: "getbestblockhash"}
	err := b.Call(&req, &res)

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

	res := ResGetBlockCount{}
	req := CmdGetBlockCount{Method: "getblockcount"}
	err := b.Call(&req, &res)

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

	res := ResGetBlockChainInfo{}
	req := CmdGetBlockChainInfo{Method: "getblockchaininfo"}
	err := b.Call(&req, &res)

	if err != nil {
		return "", err
	}
	if res.Error != nil {
		return "", res.Error
	}
	return res.Result.Chain, nil
}

func isErrBlockNotFound(err *bchain.RPCError) bool {
	return err.Message == "Block not found" ||
		err.Message == "Block height out of range"
}

// GetBlockHash returns hash of block in best-block-chain at given height.
func (b *BitcoinRPC) GetBlockHash(height uint32) (string, error) {
	glog.V(1).Info("rpc: getblockhash ", height)

	res := ResGetBlockHash{}
	req := CmdGetBlockHash{Method: "getblockhash"}
	req.Params.Height = height
	err := b.Call(&req, &res)

	if err != nil {
		return "", errors.Annotatef(err, "height %v", height)
	}
	if res.Error != nil {
		if isErrBlockNotFound(res.Error) {
			return "", bchain.ErrBlockNotFound
		}
		return "", errors.Annotatef(res.Error, "height %v", height)
	}
	return res.Result, nil
}

// GetBlockHeader returns header of block with given hash.
func (b *BitcoinRPC) GetBlockHeader(hash string) (*bchain.BlockHeader, error) {
	glog.V(1).Info("rpc: getblockheader")

	res := ResGetBlockHeader{}
	req := CmdGetBlockHeader{Method: "getblockheader"}
	req.Params.BlockHash = hash
	req.Params.Verbose = true
	err := b.Call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		if isErrBlockNotFound(res.Error) {
			return nil, bchain.ErrBlockNotFound
		}
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}
	return &res.Result, nil
}

// GetBlock returns block with given hash.
func (b *BitcoinRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	var err error
	if hash == "" && height > 0 {
		hash, err = b.GetBlockHash(height)
		if err != nil {
			return nil, err
		}
	}
	if !b.ParseBlocks {
		return b.GetBlockFull(hash)
	}
	// optimization
	if height > 0 {
		return b.GetBlockWithoutHeader(hash, height)
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

// getBlockWithoutHeader is an optimization - it does not call GetBlockHeader to get prev, next hashes
// instead it sets to header only block hash and height passed in parameters
func (b *BitcoinRPC) GetBlockWithoutHeader(hash string, height uint32) (*bchain.Block, error) {
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

	res := ResGetBlockRaw{}
	req := CmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = 0
	err := b.Call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		if isErrBlockNotFound(res.Error) {
			return nil, bchain.ErrBlockNotFound
		}
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}
	return hex.DecodeString(res.Result)
}

// GetBlockFull returns block with given hash.
func (b *BitcoinRPC) GetBlockFull(hash string) (*bchain.Block, error) {
	glog.V(1).Info("rpc: getblock (verbosity=2) ", hash)

	res := ResGetBlockFull{}
	req := CmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = 2
	err := b.Call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		if isErrBlockNotFound(res.Error) {
			return nil, bchain.ErrBlockNotFound
		}
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}
	return &res.Result, nil
}

// GetMempool returns transactions in mempool.
func (b *BitcoinRPC) GetMempool() ([]string, error) {
	glog.V(1).Info("rpc: getrawmempool")

	res := ResGetMempool{}
	req := CmdGetMempool{Method: "getrawmempool"}
	err := b.Call(&req, &res)

	if err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, res.Error
	}
	return res.Result, nil
}

// GetTransactionForMempool returns a transaction by the transaction ID.
// It could be optimized for mempool, i.e. without block time and confirmations
func (b *BitcoinRPC) GetTransactionForMempool(txid string) (*bchain.Tx, error) {
	glog.V(1).Info("rpc: getrawtransaction nonverbose ", txid)

	res := ResGetRawTransactionNonverbose{}
	req := CmdGetRawTransaction{Method: "getrawtransaction"}
	req.Params.Txid = txid
	req.Params.Verbose = false
	err := b.Call(&req, &res)
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "txid %v", txid)
	}
	data, err := hex.DecodeString(res.Result)
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	tx, err := b.Parser.ParseTx(data)
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	return tx, nil
}

// GetTransaction returns a transaction by the transaction ID.
func (b *BitcoinRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	glog.V(1).Info("rpc: getrawtransaction ", txid)

	res := ResGetRawTransaction{}
	req := CmdGetRawTransaction{Method: "getrawtransaction"}
	req.Params.Txid = txid
	req.Params.Verbose = true
	err := b.Call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "txid %v", txid)
	}
	tx, err := b.Parser.ParseTxFromJson(res.Result)
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	return tx, nil
}

// ResyncMempool gets mempool transactions and maps output scripts to transactions.
// ResyncMempool is not reentrant, it should be called from a single thread.
// It returns number of transactions in mempool
func (b *BitcoinRPC) ResyncMempool(onNewTxAddr func(txid string, addr string)) (int, error) {
	return b.Mempool.Resync(onNewTxAddr)
}

// GetMempoolTransactions returns slice of mempool transactions for given address.
func (b *BitcoinRPC) GetMempoolTransactions(address string) ([]string, error) {
	return b.Mempool.GetTransactions(address)
}

// EstimateSmartFee returns fee estimation.
func (b *BitcoinRPC) EstimateSmartFee(blocks int, conservative bool) (float64, error) {
	glog.V(1).Info("rpc: estimatesmartfee ", blocks)

	res := ResEstimateSmartFee{}
	req := CmdEstimateSmartFee{Method: "estimatesmartfee"}
	req.Params.ConfTarget = blocks
	if conservative {
		req.Params.EstimateMode = "CONSERVATIVE"
	} else {
		req.Params.EstimateMode = "ECONOMICAL"
	}
	err := b.Call(&req, &res)

	if err != nil {
		return 0, err
	}
	if res.Error != nil {
		return 0, res.Error
	}
	return res.Result.Feerate, nil
}

// EstimateFee returns fee estimation.
func (b *BitcoinRPC) EstimateFee(blocks int) (float64, error) {
	glog.V(1).Info("rpc: estimatefee ", blocks)

	res := ResEstimateFee{}
	req := CmdEstimateFee{Method: "estimatefee"}
	req.Params.Blocks = blocks
	err := b.Call(&req, &res)

	if err != nil {
		return 0, err
	}
	if res.Error != nil {
		return 0, res.Error
	}
	return res.Result, nil
}

// SendRawTransaction sends raw transaction.
func (b *BitcoinRPC) SendRawTransaction(tx string) (string, error) {
	glog.V(1).Info("rpc: sendrawtransaction")

	res := ResSendRawTransaction{}
	req := CmdSendRawTransaction{Method: "sendrawtransaction"}
	req.Params = []string{tx}
	err := b.Call(&req, &res)

	if err != nil {
		return "", err
	}
	if res.Error != nil {
		return "", res.Error
	}
	return res.Result, nil
}

// GetMempoolEntry returns mempool data for given transaction
func (b *BitcoinRPC) GetMempoolEntry(txid string) (*bchain.MempoolEntry, error) {
	glog.V(1).Info("rpc: getmempoolentry")

	res := ResGetMempoolEntry{}
	req := CmdGetMempoolEntry{
		Method: "getmempoolentry",
		Params: []string{txid},
	}
	err := b.Call(&req, &res)

	if err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, res.Error
	}
	return res.Result, nil
}

func safeDecodeResponse(body io.ReadCloser, res interface{}) (err error) {
	var data []byte
	defer func() {
		if r := recover(); r != nil {
			glog.Error("unmarshal json recovered from panic: ", r, "; data: ", string(data))
			if len(data) > 0 && len(data) < 2048 {
				err = errors.Errorf("Error: ", string(data))
			} else {
				err = errors.New("Internal error")
			}
		}
	}()
	data, err = ioutil.ReadAll(body)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &res)
}

func (b *BitcoinRPC) Call(req interface{}, res interface{}) error {
	httpData, err := b.RPCMarshaler.Marshal(req)
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
	// if server returns HTTP error code it might not return json with response
	// handle both cases
	if httpRes.StatusCode != 200 {
		err = safeDecodeResponse(httpRes.Body, &res)
		if err != nil {
			return errors.Errorf("%v %v", httpRes.Status, err)
		}
		return nil
	}
	return safeDecodeResponse(httpRes.Body, &res)
}

// GetChainParser returns BlockChainParser
func (b *BitcoinRPC) GetChainParser() bchain.BlockChainParser {
	return b.Parser
}
