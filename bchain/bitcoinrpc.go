package bchain

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"time"

	vlq "github.com/bsm/go-vlq"
	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"

	"github.com/golang/glog"
	"github.com/juju/errors"
)

// BitcoinRPC is an interface to JSON-RPC bitcoind service.
type BitcoinRPC struct {
	client      http.Client
	rpcURL      string
	user        string
	password    string
	parser      *BitcoinBlockParser
	testnet     bool
	network     string
	mempool     *Mempool
	parseBlocks bool
}

// NewBitcoinRPC returns new BitcoinRPC instance.
func NewBitcoinRPC(url string, user string, password string, timeout time.Duration, parse bool) (BlockChain, error) {
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
		parseBlocks: parse,
	}
	chainName, err := s.GetBlockChainInfo()
	if err != nil {
		return nil, err
	}

	// always create parser
	s.parser = &BitcoinBlockParser{
		Params: GetChainParams(chainName),
	}

	// parameters for getInfo request
	if s.parser.Params.Net == wire.MainNet {
		s.testnet = false
		s.network = "livenet"
	} else {
		s.testnet = true
		s.network = "testnet"
	}

	s.mempool = NewMempool(s)

	glog.Info("rpc: block chain ", s.parser.Params.Name)
	return s, nil
}

func (b *BitcoinRPC) IsTestnet() bool {
	return b.testnet
}

func (b *BitcoinRPC) GetNetworkName() string {
	return b.network
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

// getblockhash

type cmdGetBlockHash struct {
	Method string `json:"method"`
	Params struct {
		Height uint32 `json:"height"`
	} `json:"params"`
}

type resGetBlockHash struct {
	Error  *RPCError `json:"error"`
	Result string    `json:"result"`
}

// getbestblockhash

type cmdGetBestBlockHash struct {
	Method string `json:"method"`
}

type resGetBestBlockHash struct {
	Error  *RPCError `json:"error"`
	Result string    `json:"result"`
}

// getblockcount

type cmdGetBlockCount struct {
	Method string `json:"method"`
}

type resGetBlockCount struct {
	Error  *RPCError `json:"error"`
	Result uint32    `json:"result"`
}

// getblockchaininfo

type cmdGetBlockChainInfo struct {
	Method string `json:"method"`
}

type resGetBlockChainInfo struct {
	Error  *RPCError `json:"error"`
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
	Error  *RPCError `json:"error"`
	Result []string  `json:"result"`
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
	Error  *RPCError `json:"error"`
	Result string    `json:"result"`
}

type resGetBlockHeaderVerbose struct {
	Error  *RPCError   `json:"error"`
	Result BlockHeader `json:"result"`
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
	Error  *RPCError `json:"error"`
	Result string    `json:"result"`
}

type resGetBlockThin struct {
	Error  *RPCError `json:"error"`
	Result ThinBlock `json:"result"`
}

type resGetBlockFull struct {
	Error  *RPCError `json:"error"`
	Result Block     `json:"result"`
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
	Error  *RPCError `json:"error"`
	Result string    `json:"result"`
}

type resGetRawTransactionVerbose struct {
	Error  *RPCError `json:"error"`
	Result Tx        `json:"result"`
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
	Error  *RPCError `json:"error"`
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
	Error  *RPCError `json:"error"`
	Result string    `json:"result"`
}

// getmempoolentry

type cmdGetMempoolEntry struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
}

type resGetMempoolEntry struct {
	Error  *RPCError     `json:"error"`
	Result *MempoolEntry `json:"result"`
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
func (b *BitcoinRPC) GetBlockHeader(hash string) (*BlockHeader, error) {
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
func (b *BitcoinRPC) GetBlock(hash string) (*Block, error) {
	if !b.parseBlocks {
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
	block, err := b.parser.ParseBlock(data)
	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	block.BlockHeader = *header
	return block, nil
}

// GetBlockWithoutHeader is an optimization - it does not call GetBlockHeader to get prev, next hashes
// instead it sets to header only block hash and height passed in parameters
func (b *BitcoinRPC) GetBlockWithoutHeader(hash string, height uint32) (*Block, error) {
	if !b.parseBlocks {
		return b.GetBlockFull(hash)
	}
	data, err := b.GetBlockRaw(hash)
	if err != nil {
		return nil, err
	}
	block, err := b.parser.ParseBlock(data)
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
func (b *BitcoinRPC) GetBlockList(hash string) (*Block, error) {
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

	txs := make([]Tx, len(res.Result.Txids))
	for i, txid := range res.Result.Txids {
		tx, err := b.GetTransaction(txid)
		if err != nil {
			return nil, err
		}
		txs[i] = *tx
	}
	block := &Block{
		BlockHeader: res.Result.BlockHeader,
		Txs:         txs,
	}
	return block, nil
}

// GetBlockFull returns block with given hash.
func (b *BitcoinRPC) GetBlockFull(hash string) (*Block, error) {
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
func (b *BitcoinRPC) GetTransaction(txid string) (*Tx, error) {
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
	return b.mempool.Resync(onNewTxAddr)
}

// GetMempoolTransactions returns slice of mempool transactions for given output script.
func (b *BitcoinRPC) GetMempoolTransactions(outputScript []byte) ([]string, error) {
	return b.mempool.GetTransactions(outputScript)
}

// GetMempoolSpentOutput returns transaction in mempool which spends given outpoint
func (b *BitcoinRPC) GetMempoolSpentOutput(outputTxid string, vout uint32) string {
	return b.mempool.GetSpentOutput(outputTxid, vout)
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

func (b *BitcoinRPC) GetMempoolEntry(txid string) (*MempoolEntry, error) {
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
func (b *BitcoinRPC) GetChainParser() BlockChainParser {
	return b.parser
}

// bitcoinwire parsing

type BitcoinBlockParser struct {
	Params *chaincfg.Params
}

// getChainParams contains network parameters for the main Bitcoin network,
// the regression test Bitcoin network, the test Bitcoin network and
// the simulation test Bitcoin network, in this order
func GetChainParams(chain string) *chaincfg.Params {
	switch chain {
	case "test":
		return &chaincfg.TestNet3Params
	case "regtest":
		return &chaincfg.RegressionNetParams
	}
	return &chaincfg.MainNetParams
}

// AddressToOutputScript converts bitcoin address to ScriptPubKey
func (p *BitcoinBlockParser) AddressToOutputScript(address string) ([]byte, error) {
	da, err := btcutil.DecodeAddress(address, p.Params)
	if err != nil {
		return nil, err
	}
	script, err := txscript.PayToAddrScript(da)
	if err != nil {
		return nil, err
	}
	return script, nil
}

// OutputScriptToAddresses converts ScriptPubKey to bitcoin addresses
func (p *BitcoinBlockParser) OutputScriptToAddresses(script []byte) ([]string, error) {
	_, addresses, _, err := txscript.ExtractPkScriptAddrs(script, p.Params)
	if err != nil {
		return nil, err
	}
	rv := make([]string, len(addresses))
	for i, a := range addresses {
		rv[i] = a.EncodeAddress()
	}
	return rv, nil
}

func (p *BitcoinBlockParser) txFromMsgTx(t *wire.MsgTx, parseAddresses bool) Tx {
	vin := make([]Vin, len(t.TxIn))
	for i, in := range t.TxIn {
		if blockchain.IsCoinBaseTx(t) {
			vin[i] = Vin{
				Coinbase: hex.EncodeToString(in.SignatureScript),
				Sequence: in.Sequence,
			}
			break
		}
		s := ScriptSig{
			Hex: hex.EncodeToString(in.SignatureScript),
			// missing: Asm,
		}
		vin[i] = Vin{
			Txid:      in.PreviousOutPoint.Hash.String(),
			Vout:      in.PreviousOutPoint.Index,
			Sequence:  in.Sequence,
			ScriptSig: s,
		}
	}
	vout := make([]Vout, len(t.TxOut))
	for i, out := range t.TxOut {
		addrs := []string{}
		if parseAddresses {
			addrs, _ = p.OutputScriptToAddresses(out.PkScript)
		}
		s := ScriptPubKey{
			Hex:       hex.EncodeToString(out.PkScript),
			Addresses: addrs,
			// missing: Asm,
			// missing: Type,
		}
		vout[i] = Vout{
			Value:        float64(out.Value) / 1E8,
			N:            uint32(i),
			ScriptPubKey: s,
		}
	}
	tx := Tx{
		Txid: t.TxHash().String(),
		// skip: Version,
		LockTime: t.LockTime,
		Vin:      vin,
		Vout:     vout,
		// skip: BlockHash,
		// skip: Confirmations,
		// skip: Time,
		// skip: Blocktime,
	}
	return tx
}

// ParseTx parses byte array containing transaction and returns Tx struct
func (p *BitcoinBlockParser) ParseTx(b []byte) (*Tx, error) {
	t := wire.MsgTx{}
	r := bytes.NewReader(b)
	if err := t.Deserialize(r); err != nil {
		return nil, err
	}
	tx := p.txFromMsgTx(&t, true)
	tx.Hex = hex.EncodeToString(b)
	return &tx, nil
}

// ParseBlock parses raw block to our Block struct
func (p *BitcoinBlockParser) ParseBlock(b []byte) (*Block, error) {
	w := wire.MsgBlock{}
	r := bytes.NewReader(b)

	if err := w.Deserialize(r); err != nil {
		return nil, err
	}

	txs := make([]Tx, len(w.Transactions))
	for ti, t := range w.Transactions {
		txs[ti] = p.txFromMsgTx(t, false)
	}

	return &Block{Txs: txs}, nil
}

// PackTx packs transaction to byte array
func (p *BitcoinBlockParser) PackTx(tx *Tx, height uint32, blockTime int64) ([]byte, error) {
	buf := make([]byte, 4+vlq.MaxLen64+len(tx.Hex)/2)
	binary.BigEndian.PutUint32(buf[0:4], height)
	vl := vlq.PutInt(buf[4:4+vlq.MaxLen64], blockTime)
	hl, err := hex.Decode(buf[4+vl:], []byte(tx.Hex))
	return buf[0 : 4+vl+hl], err
}

// UnpackTx unpacks transaction from byte array
func (p *BitcoinBlockParser) UnpackTx(buf []byte) (*Tx, uint32, error) {
	height := binary.BigEndian.Uint32(buf)
	bt, l := vlq.Int(buf[4:])
	tx, err := p.ParseTx(buf[4+l:])
	if err != nil {
		return nil, 0, err
	}
	tx.Blocktime = bt
	return tx, height, nil
}
