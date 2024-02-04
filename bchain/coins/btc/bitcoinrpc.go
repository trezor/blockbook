package btc

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/martinboehm/btcd/wire"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
)

// BitcoinRPC is an interface to JSON-RPC bitcoind service.
type BitcoinRPC struct {
	*bchain.BaseChain
	client                 http.Client
	rpcURL                 string
	user                   string
	password               string
	Mempool                *bchain.MempoolBitcoinType
	ParseBlocks            bool
	pushHandler            func(bchain.NotificationType)
	mq                     *bchain.MQ
	ChainConfig            *Configuration
	RPCMarshaler           RPCMarshaler
	mempoolGolombFilterP   uint8
	mempoolFilterScripts   string
	mempoolUseZeroedKey    bool
	alternativeFeeProvider alternativeFeeProviderInterface
}

// Configuration represents json config file
type Configuration struct {
	CoinName                     string `json:"coin_name"`
	CoinShortcut                 string `json:"coin_shortcut"`
	RPCURL                       string `json:"rpc_url"`
	RPCUser                      string `json:"rpc_user"`
	RPCPass                      string `json:"rpc_pass"`
	RPCTimeout                   int    `json:"rpc_timeout"`
	AddressAliases               bool   `json:"address_aliases,omitempty"`
	Parse                        bool   `json:"parse"`
	MessageQueueBinding          string `json:"message_queue_binding"`
	Subversion                   string `json:"subversion"`
	BlockAddressesToKeep         int    `json:"block_addresses_to_keep"`
	MempoolWorkers               int    `json:"mempool_workers"`
	MempoolSubWorkers            int    `json:"mempool_sub_workers"`
	AddressFormat                string `json:"address_format"`
	SupportsEstimateFee          bool   `json:"supports_estimate_fee"`
	SupportsEstimateSmartFee     bool   `json:"supports_estimate_smart_fee"`
	XPubMagic                    uint32 `json:"xpub_magic,omitempty"`
	XPubMagicSegwitP2sh          uint32 `json:"xpub_magic_segwit_p2sh,omitempty"`
	XPubMagicSegwitNative        uint32 `json:"xpub_magic_segwit_native,omitempty"`
	Slip44                       uint32 `json:"slip44,omitempty"`
	AlternativeEstimateFee       string `json:"alternative_estimate_fee,omitempty"`
	AlternativeEstimateFeeParams string `json:"alternative_estimate_fee_params,omitempty"`
	MinimumCoinbaseConfirmations int    `json:"minimumCoinbaseConfirmations,omitempty"`
	MempoolGolombFilterP         uint8  `json:"mempool_golomb_filter_p,omitempty"`
	MempoolFilterScripts         string `json:"mempool_filter_scripts,omitempty"`
	MempoolFilterUseZeroedKey    bool   `json:"mempool_filter_use_zeroed_key,omitempty"`
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
	// default MinimumCoinbaseConfirmations is 100
	if c.MinimumCoinbaseConfirmations == 0 {
		c.MinimumCoinbaseConfirmations = 100
	}
	// at least 1 mempool worker/subworker for synchronous mempool synchronization
	if c.MempoolWorkers < 1 {
		c.MempoolWorkers = 1
	}
	if c.MempoolSubWorkers < 1 {
		c.MempoolSubWorkers = 1
	}
	// btc supports both calls, other coins overriding BitcoinRPC can change this
	c.SupportsEstimateFee = true
	c.SupportsEstimateSmartFee = true

	transport := &http.Transport{
		Dial:                (&net.Dialer{KeepAlive: 600 * time.Second}).Dial,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100, // necessary to not to deplete ports
	}

	s := &BitcoinRPC{
		BaseChain:            &bchain.BaseChain{},
		client:               http.Client{Timeout: time.Duration(c.RPCTimeout) * time.Second, Transport: transport},
		rpcURL:               c.RPCURL,
		user:                 c.RPCUser,
		password:             c.RPCPass,
		ParseBlocks:          c.Parse,
		ChainConfig:          &c,
		pushHandler:          pushHandler,
		RPCMarshaler:         JSONMarshalerV2{},
		mempoolGolombFilterP: c.MempoolGolombFilterP,
		mempoolFilterScripts: c.MempoolFilterScripts,
		mempoolUseZeroedKey:  c.MempoolFilterUseZeroedKey,
	}

	return s, nil
}

// Initialize initializes BitcoinRPC instance.
func (b *BitcoinRPC) Initialize() error {
	b.ChainConfig.SupportsEstimateFee = false

	ci, err := b.GetChainInfo()
	if err != nil {
		return err
	}
	chainName := ci.Chain

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

	if b.ChainConfig.AlternativeEstimateFee == "whatthefee" {
		if b.alternativeFeeProvider, err = NewWhatTheFee(b, b.ChainConfig.AlternativeEstimateFeeParams); err != nil {
			glog.Error("NewWhatTheFee error ", err, " Reverting to default estimateFee functionality")
			// disable AlternativeEstimateFee logic
			b.alternativeFeeProvider = nil
		}
	} else if b.ChainConfig.AlternativeEstimateFee == "mempoolspace" {
		if b.alternativeFeeProvider, err = NewMempoolSpaceFee(b, b.ChainConfig.AlternativeEstimateFeeParams); err != nil {
			glog.Error("MempoolSpaceFee error ", err, " Reverting to default estimateFee functionality")
			// disable AlternativeEstimateFee logic
			b.alternativeFeeProvider = nil
		}
	}

	return nil
}

// CreateMempool creates mempool if not already created, however does not initialize it
func (b *BitcoinRPC) CreateMempool(chain bchain.BlockChain) (bchain.Mempool, error) {
	if b.Mempool == nil {
		b.Mempool = bchain.NewMempoolBitcoinType(chain, b.ChainConfig.MempoolWorkers, b.ChainConfig.MempoolSubWorkers, b.mempoolGolombFilterP, b.mempoolFilterScripts, b.mempoolUseZeroedKey)
	}
	return b.Mempool, nil
}

// InitializeMempool creates ZeroMQ subscription and sets AddrDescForOutpointFunc to the Mempool
func (b *BitcoinRPC) InitializeMempool(addrDescForOutpoint bchain.AddrDescForOutpointFunc, onNewTxAddr bchain.OnNewTxAddrFunc, onNewTx bchain.OnNewTxFunc) error {
	if b.Mempool == nil {
		return errors.New("Mempool not created")
	}
	b.Mempool.AddrDescForOutpoint = addrDescForOutpoint
	b.Mempool.OnNewTxAddr = onNewTxAddr
	b.Mempool.OnNewTx = onNewTx
	if b.mq == nil {
		mq, err := bchain.NewMQ(b.ChainConfig.MessageQueueBinding, b.pushHandler)
		if err != nil {
			glog.Error("mq: ", err)
			return err
		}
		b.mq = mq
	}
	return nil
}

// Shutdown ZeroMQ and other resources
func (b *BitcoinRPC) Shutdown(ctx context.Context) error {
	if b.mq != nil {
		if err := b.mq.Shutdown(ctx); err != nil {
			glog.Error("MQ.Shutdown error: ", err)
			return err
		}
	}
	return nil
}

// GetCoinName returns the coin name
func (b *BitcoinRPC) GetCoinName() string {
	return b.ChainConfig.CoinName
}

// GetSubversion returns the backend subversion
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
		Chain         string            `json:"chain"`
		Blocks        int               `json:"blocks"`
		Headers       int               `json:"headers"`
		Bestblockhash string            `json:"bestblockhash"`
		Difficulty    common.JSONNumber `json:"difficulty"`
		SizeOnDisk    int64             `json:"size_on_disk"`
		Warnings      string            `json:"warnings"`
	} `json:"result"`
}

// getnetworkinfo

type CmdGetNetworkInfo struct {
	Method string `json:"method"`
}

type ResGetNetworkInfo struct {
	Error  *bchain.RPCError `json:"error"`
	Result struct {
		Version         common.JSONNumber `json:"version"`
		Subversion      common.JSONNumber `json:"subversion"`
		ProtocolVersion common.JSONNumber `json:"protocolversion"`
		Timeoffset      float64           `json:"timeoffset"`
		Warnings        string            `json:"warnings"`
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

type BlockThin struct {
	bchain.BlockHeader
	Txids []string `json:"tx"`
}

type ResGetBlockThin struct {
	Error  *bchain.RPCError `json:"error"`
	Result BlockThin        `json:"result"`
}

type ResGetBlockFull struct {
	Error  *bchain.RPCError `json:"error"`
	Result bchain.Block     `json:"result"`
}

type ResGetBlockInfo struct {
	Error  *bchain.RPCError `json:"error"`
	Result bchain.BlockInfo `json:"result"`
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
		Feerate common.JSONNumber `json:"feerate"`
		Blocks  int               `json:"blocks"`
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
	Error  *bchain.RPCError  `json:"error"`
	Result common.JSONNumber `json:"result"`
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

// GetChainInfo returns information about the connected backend
func (b *BitcoinRPC) GetChainInfo() (*bchain.ChainInfo, error) {
	glog.V(1).Info("rpc: getblockchaininfo")

	resCi := ResGetBlockChainInfo{}
	err := b.Call(&CmdGetBlockChainInfo{Method: "getblockchaininfo"}, &resCi)
	if err != nil {
		return nil, err
	}
	if resCi.Error != nil {
		return nil, resCi.Error
	}

	glog.V(1).Info("rpc: getnetworkinfo")
	resNi := ResGetNetworkInfo{}
	err = b.Call(&CmdGetNetworkInfo{Method: "getnetworkinfo"}, &resNi)
	if err != nil {
		return nil, err
	}
	if resNi.Error != nil {
		return nil, resNi.Error
	}

	rv := &bchain.ChainInfo{
		Bestblockhash: resCi.Result.Bestblockhash,
		Blocks:        resCi.Result.Blocks,
		Chain:         resCi.Result.Chain,
		Difficulty:    string(resCi.Result.Difficulty),
		Headers:       resCi.Result.Headers,
		SizeOnDisk:    resCi.Result.SizeOnDisk,
		Subversion:    string(resNi.Result.Subversion),
		Timeoffset:    resNi.Result.Timeoffset,
	}
	rv.Version = string(resNi.Result.Version)
	rv.ProtocolVersion = string(resNi.Result.ProtocolVersion)
	if len(resCi.Result.Warnings) > 0 {
		rv.Warnings = resCi.Result.Warnings + " "
	}
	if resCi.Result.Warnings != resNi.Result.Warnings {
		rv.Warnings += resNi.Result.Warnings
	}
	return rv, nil
}

// IsErrBlockNotFound returns true if error means block was not found
func IsErrBlockNotFound(err *bchain.RPCError) bool {
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
		if IsErrBlockNotFound(res.Error) {
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
		if IsErrBlockNotFound(res.Error) {
			return nil, bchain.ErrBlockNotFound
		}
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}
	return &res.Result, nil
}

// GetBlock returns block with given hash.
func (b *BitcoinRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	var err error
	if hash == "" {
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
	data, err := b.GetBlockBytes(hash)
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

// GetBlockInfo returns extended header (more info than in bchain.BlockHeader) with a list of txids
func (b *BitcoinRPC) GetBlockInfo(hash string) (*bchain.BlockInfo, error) {
	glog.V(1).Info("rpc: getblock (verbosity=1) ", hash)

	res := ResGetBlockInfo{}
	req := CmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = 1
	err := b.Call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		if IsErrBlockNotFound(res.Error) {
			return nil, bchain.ErrBlockNotFound
		}
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}
	return &res.Result, nil
}

// GetBlockWithoutHeader is an optimization - it does not call GetBlockHeader to get prev, next hashes
// instead it sets to header only block hash and height passed in parameters
func (b *BitcoinRPC) GetBlockWithoutHeader(hash string, height uint32) (*bchain.Block, error) {
	data, err := b.GetBlockBytes(hash)
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

// GetBlockRaw returns block with given hash as hex string
func (b *BitcoinRPC) GetBlockRaw(hash string) (string, error) {
	glog.V(1).Info("rpc: getblock (verbosity=0) ", hash)

	res := ResGetBlockRaw{}
	req := CmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = 0
	err := b.Call(&req, &res)

	if err != nil {
		return "", errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		if IsErrBlockNotFound(res.Error) {
			return "", bchain.ErrBlockNotFound
		}
		return "", errors.Annotatef(res.Error, "hash %v", hash)
	}
	return res.Result, nil
}

// GetBlockBytes returns block with given hash as bytes
func (b *BitcoinRPC) GetBlockBytes(hash string) ([]byte, error) {
	block, err := b.GetBlockRaw(hash)
	if err != nil {
		return nil, err
	}
	return hex.DecodeString(block)
}

// GetBlockFull returns block with given hash
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
		if IsErrBlockNotFound(res.Error) {
			return nil, bchain.ErrBlockNotFound
		}
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}

	for i := range res.Result.Txs {
		tx := &res.Result.Txs[i]
		for j := range tx.Vout {
			vout := &tx.Vout[j]
			// convert vout.JsonValue to big.Int and clear it, it is only temporary value used for unmarshal
			vout.ValueSat, err = b.Parser.AmountToBigInt(vout.JsonValue)
			if err != nil {
				return nil, err
			}
			vout.JsonValue = ""
		}
	}

	return &res.Result, nil
}

// GetMempoolTransactions returns transactions in mempool
func (b *BitcoinRPC) GetMempoolTransactions() ([]string, error) {
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

// IsMissingTx return true if error means missing tx
func IsMissingTx(err *bchain.RPCError) bool {
	// err.Code == -5 "No such mempool or blockchain transaction"
	return err.Code == -5
}

// GetTransactionForMempool returns a transaction by the transaction ID
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
		if IsMissingTx(res.Error) {
			return nil, bchain.ErrTxNotFound
		}
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

// GetTransaction returns a transaction by the transaction ID
func (b *BitcoinRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	r, err := b.getRawTransaction(txid)
	if err != nil {
		return nil, err
	}
	tx, err := b.Parser.ParseTxFromJson(r)
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	tx.CoinSpecificData = r
	return tx, nil
}

// GetTransactionSpecific returns json as returned by backend, with all coin specific data
func (b *BitcoinRPC) GetTransactionSpecific(tx *bchain.Tx) (json.RawMessage, error) {
	if csd, ok := tx.CoinSpecificData.(json.RawMessage); ok {
		return csd, nil
	}
	return b.getRawTransaction(tx.Txid)
}

// getRawTransaction returns json as returned by backend, with all coin specific data
func (b *BitcoinRPC) getRawTransaction(txid string) (json.RawMessage, error) {
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
		if IsMissingTx(res.Error) {
			return nil, bchain.ErrTxNotFound
		}
		return nil, errors.Annotatef(res.Error, "txid %v", txid)
	}
	return res.Result, nil
}

func (b *BitcoinRPC) blockchainEstimateSmartFee(blocks int, conservative bool) (big.Int, error) {
	// use EstimateFee if EstimateSmartFee is not supported
	if !b.ChainConfig.SupportsEstimateSmartFee && b.ChainConfig.SupportsEstimateFee {
		return b.EstimateFee(blocks)
	}

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
	var r big.Int
	if err != nil {
		return r, err
	}
	if res.Error != nil {
		return r, res.Error
	}
	r, err = b.Parser.AmountToBigInt(res.Result.Feerate)
	if err != nil {
		return r, err
	}
	return r, nil
}

// EstimateSmartFee returns fee estimation
func (b *BitcoinRPC) EstimateSmartFee(blocks int, conservative bool) (big.Int, error) {
	// use alternative estimator if enabled
	if b.alternativeFeeProvider != nil {
		r, err := b.alternativeFeeProvider.estimateFee(blocks)
		// in case of error, fallback to default estimator
		if err == nil {
			return r, nil
		}
	}
	return b.blockchainEstimateSmartFee(blocks, conservative)
}

// EstimateFee returns fee estimation.
func (b *BitcoinRPC) EstimateFee(blocks int) (big.Int, error) {
	var r big.Int
	var err error
	// use alternative estimator if enabled
	if b.alternativeFeeProvider != nil {
		r, err = b.alternativeFeeProvider.estimateFee(blocks)
		// in case of error, fallback to default estimator
		if err == nil {
			return r, nil
		}
	}
	// use EstimateSmartFee if EstimateFee is not supported
	if !b.ChainConfig.SupportsEstimateFee && b.ChainConfig.SupportsEstimateSmartFee {
		return b.EstimateSmartFee(blocks, true)
	}

	glog.V(1).Info("rpc: estimatefee ", blocks)

	res := ResEstimateFee{}
	req := CmdEstimateFee{Method: "estimatefee"}
	req.Params.Blocks = blocks
	err = b.Call(&req, &res)

	if err != nil {
		return r, err
	}
	if res.Error != nil {
		return r, res.Error
	}
	r, err = b.Parser.AmountToBigInt(res.Result)
	if err != nil {
		return r, err
	}
	return r, nil
}

// SendRawTransaction sends raw transaction
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
	res.Result.FeeSat, err = b.Parser.AmountToBigInt(res.Result.Fee)
	if err != nil {
		return nil, err
	}
	res.Result.ModifiedFeeSat, err = b.Parser.AmountToBigInt(res.Result.ModifiedFee)
	if err != nil {
		return nil, err
	}
	return res.Result, nil
}

func safeDecodeResponse(body io.ReadCloser, res interface{}) (err error) {
	var data []byte
	defer func() {
		if r := recover(); r != nil {
			glog.Error("unmarshal json recovered from panic: ", r, "; data: ", string(data))
			debug.PrintStack()
			if len(data) > 0 && len(data) < 2048 {
				err = errors.Errorf("Error: %v", string(data))
			} else {
				err = errors.New("Internal error")
			}
		}
	}()
	data, err = io.ReadAll(body)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &res)
}

// Call calls Backend RPC interface, using RPCMarshaler interface to marshall the request
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
