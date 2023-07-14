package nuls

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/ioutil"
	"math/big"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// NulsRPC is an interface to JSON-RPC bitcoind service
type NulsRPC struct {
	*btc.BitcoinRPC
	client   http.Client
	rpcURL   string
	user     string
	password string
}

// NewNulsRPC returns new NulsRPC instance
func NewNulsRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	var c btc.Configuration
	err = json.Unmarshal(config, &c)
	if err != nil {
		return nil, errors.Annotatef(err, "Invalid configuration file")
	}

	transport := &http.Transport{
		Dial:                (&net.Dialer{KeepAlive: 600 * time.Second}).Dial,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100, // necessary to not to deplete ports
	}

	s := &NulsRPC{
		BitcoinRPC: b.(*btc.BitcoinRPC),
		client:     http.Client{Timeout: time.Duration(c.RPCTimeout) * time.Second, Transport: transport},
		rpcURL:     c.RPCURL,
		user:       c.RPCUser,
		password:   c.RPCPass,
	}
	s.BitcoinRPC.RPCMarshaler = btc.JSONMarshalerV1{}
	s.BitcoinRPC.ChainConfig.SupportsEstimateSmartFee = false

	return s, nil
}

// Initialize initializes GincoinRPC instance.
func (n *NulsRPC) Initialize() error {
	chainName := ""
	params := GetChainParams(chainName)

	// always create parser
	n.BitcoinRPC.Parser = NewNulsParser(params, n.BitcoinRPC.ChainConfig)

	// parameters for getInfo request
	if params.Net == MainnetMagic {
		n.BitcoinRPC.Testnet = false
		n.BitcoinRPC.Network = "livenet"
	} else {
		n.BitcoinRPC.Testnet = true
		n.BitcoinRPC.Network = "testnet"
	}

	glog.Info("rpc: block chain ", params.Name)

	return nil
}

type CmdGetNetworkInfo struct {
	Success bool `json:"success"`
	Data    struct {
		LocalBestHeight int64  `json:"localBestHeight"`
		NetBestHeight   int    `json:"netBestHeight"`
		TimeOffset      string `json:"timeOffset"`
		InCount         int8   `json:"inCount"`
		OutCount        int8   `json:"outCount"`
	} `json:"data"`
}

type CmdGetVersionInfo struct {
	Success bool `json:"success"`
	Data    struct {
		MyVersion      string `json:"myVersion"`
		NewestVersion  string `json:"newestVersion"`
		NetworkVersion int    `json:"networkVersion"`
		Information    string `json:"information"`
	} `json:"data"`
}

type CmdGetBestBlockHash struct {
	Success bool `json:"success"`
	Data    struct {
		Value string `json:"value"`
	} `json:"data"`
}

type CmdGetBestBlockHeight struct {
	Success bool `json:"success"`
	Data    struct {
		Value uint32 `json:"value"`
	} `json:"data"`
}

type CmdTxBroadcast struct {
	Success bool `json:"success"`
	Data    struct {
		Value string `json:"value"`
	} `json:"data"`
}

type CmdGetBlockHeader struct {
	Success bool `json:"success"`
	Data    struct {
		Hash           string  `json:"hash"`
		PreHash        string  `json:"preHash"`
		MerkleHash     string  `json:"merkleHash"`
		StateRoot      string  `json:"stateRoot"`
		Time           int64   `json:"time"`
		Height         int64   `json:"height"`
		TxCount        int     `json:"txCount"`
		PackingAddress string  `json:"packingAddress"`
		ConfirmCount   int     `json:"confirmCount"`
		ScriptSig      string  `json:"scriptSig"`
		Size           int     `json:"size"`
		Reward         float64 `json:"reward"`
		Fee            float64 `json:"fee"`
	} `json:"data"`
}

type CmdGetBlock struct {
	Success bool `json:"success"`
	Data    struct {
		Hash           string  `json:"hash"`
		PreHash        string  `json:"preHash"`
		MerkleHash     string  `json:"merkleHash"`
		StateRoot      string  `json:"stateRoot"`
		Time           int64   `json:"time"`
		Height         int64   `json:"height"`
		TxCount        int     `json:"txCount"`
		PackingAddress string  `json:"packingAddress"`
		ConfirmCount   int     `json:"confirmCount"`
		ScriptSig      string  `json:"scriptSig"`
		Size           int     `json:"size"`
		Reward         float64 `json:"reward"`
		Fee            float64 `json:"fee"`

		TxList []Tx `json:"txList"`
	} `json:"data"`
}

type CmdGetTx struct {
	Success bool `json:"success"`
	Tx      Tx   `json:"data"`
}

type Tx struct {
	Hash         string  `json:"hash"`
	Type         int     `json:"type"`
	Time         int64   `json:"time"`
	BlockHeight  int64   `json:"blockHeight"`
	Fee          float64 `json:"fee"`
	Value        float64 `json:"value"`
	Remark       string  `json:"remark"`
	ScriptSig    string  `json:"scriptSig"`
	Status       int     `json:"status"`
	ConfirmCount int     `json:"confirmCount"`
	Size         int     `json:"size"`
	Inputs       []struct {
		FromHash  string  `json:"fromHash"`
		FromIndex uint32  `json:"fromIndex"`
		Address   string  `json:"address"`
		Value     float64 `json:"value"`
	} `json:"inputs"`
	Outputs []struct {
		Address  string `json:"address"`
		Value    int64  `json:"value"`
		LockTime int64  `json:"lockTime"`
	} `json:"outputs"`
}

type CmdGetTxBytes struct {
	Success bool `json:"success"`
	Data    struct {
		Value string `json:"value"`
	} `json:"data"`
}

func (n *NulsRPC) GetChainInfo() (*bchain.ChainInfo, error) {
	networkInfo := CmdGetNetworkInfo{}
	error := n.Call("/api/network/info", &networkInfo)
	if error != nil {
		return nil, error
	}

	versionInfo := CmdGetVersionInfo{}
	error = n.Call("/api/client/version", &versionInfo)
	if error != nil {
		return nil, error
	}

	chainInfo := &bchain.ChainInfo{
		Chain:           "nuls",
		Blocks:          networkInfo.Data.NetBestHeight,
		Headers:         networkInfo.Data.NetBestHeight,
		Bestblockhash:   "",
		Difficulty:      networkInfo.Data.TimeOffset,
		SizeOnDisk:      networkInfo.Data.LocalBestHeight,
		Version:         versionInfo.Data.MyVersion,
		Subversion:      versionInfo.Data.NewestVersion,
		ProtocolVersion: strconv.Itoa(versionInfo.Data.NetworkVersion),
		Timeoffset:      0,
		Warnings:        versionInfo.Data.Information,
	}
	return chainInfo, nil
}

func (n *NulsRPC) GetBestBlockHash() (string, error) {
	bestBlockHash := CmdGetBestBlockHash{}
	error := n.Call("/api/block/newest/hash", &bestBlockHash)
	if error != nil {
		return "", error
	}
	return bestBlockHash.Data.Value, nil
}

func (n *NulsRPC) GetBestBlockHeight() (uint32, error) {
	bestBlockHeight := CmdGetBestBlockHeight{}
	error := n.Call("/api/block/newest/height", &bestBlockHeight)
	if error != nil {
		return 0, error
	}
	return bestBlockHeight.Data.Value, nil
}

func (n *NulsRPC) GetBlockHash(height uint32) (string, error) {
	blockHeader := CmdGetBlockHeader{}
	error := n.Call("/api/block/header/height/"+strconv.Itoa(int(height)), &blockHeader)
	if error != nil {
		return "", error
	}

	if !blockHeader.Success {
		return "", bchain.ErrBlockNotFound
	}

	return blockHeader.Data.Hash, nil
}

func (n *NulsRPC) GetBlockHeader(hash string) (*bchain.BlockHeader, error) {
	uri := "/api/block/header/hash/" + hash
	return n.getBlobkHeader(uri)
}

func (n *NulsRPC) GetBlockHeaderByHeight(height uint32) (*bchain.BlockHeader, error) {
	uri := "/api/block/header/height/" + strconv.Itoa(int(height))
	return n.getBlobkHeader(uri)
}

func (n *NulsRPC) getBlobkHeader(uri string) (*bchain.BlockHeader, error) {
	blockHeader := CmdGetBlockHeader{}
	error := n.Call(uri, &blockHeader)
	if error != nil {
		return nil, error
	}

	if !blockHeader.Success {
		return nil, bchain.ErrBlockNotFound
	}

	nexHash, _ := n.GetBlockHash(uint32(blockHeader.Data.Height + 1))

	header := &bchain.BlockHeader{
		Hash:          blockHeader.Data.Hash,
		Prev:          blockHeader.Data.PreHash,
		Next:          nexHash,
		Height:        uint32(blockHeader.Data.Height),
		Confirmations: blockHeader.Data.ConfirmCount,
		Size:          blockHeader.Data.Size,
		Time:          blockHeader.Data.Time / 1000,
	}
	return header, nil
}

func (n *NulsRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {

	url := "/api/block/hash/" + hash

	if hash == "" {
		url = "/api/block/height/" + strconv.Itoa(int(height))
	}

	getBlock := CmdGetBlock{}
	error := n.Call(url, &getBlock)
	if error != nil {
		return nil, error
	}

	if !getBlock.Success {
		return nil, bchain.ErrBlockNotFound
	}

	nexHash, _ := n.GetBlockHash(uint32(getBlock.Data.Height + 1))

	header := bchain.BlockHeader{
		Hash:          getBlock.Data.Hash,
		Prev:          getBlock.Data.PreHash,
		Next:          nexHash,
		Height:        uint32(getBlock.Data.Height),
		Confirmations: getBlock.Data.ConfirmCount,
		Size:          getBlock.Data.Size,
		Time:          getBlock.Data.Time / 1000,
	}

	var txs []bchain.Tx

	for _, rawTx := range getBlock.Data.TxList {
		tx, err := converTx(rawTx)
		if err != nil {
			return nil, err
		}
		tx.Blocktime = header.Time
		txs = append(txs, *tx)
	}

	block := &bchain.Block{
		BlockHeader: header,
		Txs:         txs,
	}
	return block, nil
}

func (n *NulsRPC) GetBlockInfo(hash string) (*bchain.BlockInfo, error) {
	if hash == "" {
		return nil, bchain.ErrBlockNotFound
	}

	getBlock := CmdGetBlock{}
	error := n.Call("/api/block/hash/"+hash, &getBlock)
	if error != nil {
		return nil, error
	}

	if !getBlock.Success {
		return nil, bchain.ErrBlockNotFound
	}

	nexHash, _ := n.GetBlockHash(uint32(getBlock.Data.Height + 1))

	header := bchain.BlockHeader{
		Hash:          getBlock.Data.Hash,
		Prev:          getBlock.Data.PreHash,
		Next:          nexHash,
		Height:        uint32(getBlock.Data.Height),
		Confirmations: getBlock.Data.ConfirmCount,
		Size:          getBlock.Data.Size,
		Time:          getBlock.Data.Time / 1000,
	}

	var txIds []string

	for _, rawTx := range getBlock.Data.TxList {
		txIds = append(txIds, rawTx.Hash)
	}

	blockInfo := &bchain.BlockInfo{
		BlockHeader: header,
		MerkleRoot:  getBlock.Data.MerkleHash,
		//Version:	getBlock.Data.StateRoot,
		Txids: txIds,
	}
	return blockInfo, nil
}

func (n *NulsRPC) GetMempoolTransactions() ([]string, error) {
	return nil, nil
}

func (n *NulsRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	if txid == "" {
		return nil, bchain.ErrTxidMissing
	}

	getTx := CmdGetTx{}
	error := n.Call("/api/tx/hash/"+txid, &getTx)
	if error != nil {
		return nil, error
	}

	if !getTx.Success {
		return nil, bchain.ErrTxNotFound
	}

	tx, err := converTx(getTx.Tx)
	if err != nil {
		return nil, err
	}

	blockHeaderHeight := getTx.Tx.BlockHeight
	// shouldn't it check the error here?
	blockHeader, _ := n.GetBlockHeaderByHeight(uint32(blockHeaderHeight))
	if blockHeader != nil {
		tx.Blocktime = blockHeader.Time
	}

	hexBytys, e := n.GetTransactionSpecific(tx)
	if e == nil {
		var hex string
		json.Unmarshal(hexBytys, &hex)
		tx.Hex = hex
		tx.CoinSpecificData = hex
	}
	return tx, nil
}

func (n *NulsRPC) GetTransactionForMempool(txid string) (*bchain.Tx, error) {
	return nil, nil
}

func (n *NulsRPC) GetTransactionSpecific(tx *bchain.Tx) (json.RawMessage, error) {
	if tx == nil {
		return nil, bchain.ErrTxNotFound
	}

	if csd, ok := tx.CoinSpecificData.(json.RawMessage); ok {
		return csd, nil
	}

	getTxBytes := CmdGetTxBytes{}
	error := n.Call("/api/tx/bytes?hash="+tx.Txid, &getTxBytes)
	if error != nil {
		return nil, error
	}

	if !getTxBytes.Success {
		return nil, bchain.ErrTxNotFound
	}

	txBytes, byErr := base64.StdEncoding.DecodeString(getTxBytes.Data.Value)
	if byErr != nil {
		return nil, byErr
	}
	hexBytes := make([]byte, len(txBytes)*2)
	hex.Encode(hexBytes, txBytes)

	m, err := json.Marshal(string(hexBytes))
	return json.RawMessage(m), err
}

func (n *NulsRPC) EstimateSmartFee(blocks int, conservative bool) (big.Int, error) {
	return n.EstimateFee(blocks)
}

func (n *NulsRPC) EstimateFee(blocks int) (big.Int, error) {
	return *big.NewInt(100000), nil
}

func (n *NulsRPC) SendRawTransaction(tx string) (string, error) {
	broadcast := CmdTxBroadcast{}
	req := struct {
		TxHex string `json:"txHex"`
	}{
		TxHex: tx,
	}

	error := n.Post("/api/accountledger/transaction/broadcast", req, &broadcast)
	if error != nil {
		return "", error
	}

	if !broadcast.Success {
		return "", bchain.ErrTxidMissing
	}

	return broadcast.Data.Value, nil
}

// Call calls Backend RPC interface, using RPCMarshaler interface to marshall the request
func (b *NulsRPC) Call(uri string, res interface{}) error {
	url := b.rpcURL + uri
	httpReq, err := http.NewRequest("GET", url, nil)
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

func (b *NulsRPC) Post(uri string, req interface{}, res interface{}) error {
	url := b.rpcURL + uri
	httpData, err := b.RPCMarshaler.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(httpData))
	if err != nil {
		return err
	}
	httpReq.SetBasicAuth(b.user, b.password)
	httpReq.Header.Set("Content-Type", "application/json")
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

func safeDecodeResponse(body io.ReadCloser, res *interface{}) (err error) {
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
	data, err = ioutil.ReadAll(body)
	if err != nil {
		return err
	}

	//fmt.Println(string(data))
	error := json.Unmarshal(data, res)
	return error
}

func converTx(rawTx Tx) (*bchain.Tx, error) {
	var lockTime int64 = 0
	var vins = make([]bchain.Vin, 0)
	var vouts []bchain.Vout

	for _, input := range rawTx.Inputs {
		vin := bchain.Vin{
			Coinbase:  "",
			Txid:      input.FromHash,
			Vout:      input.FromIndex,
			ScriptSig: bchain.ScriptSig{},
			Sequence:  0,
			Addresses: []string{input.Address},
		}
		vins = append(vins, vin)
	}

	for index, output := range rawTx.Outputs {
		vout := bchain.Vout{
			ValueSat: *big.NewInt(output.Value),
			//JsonValue: 	"",
			//LockTime:	output.LockTime,
			N: uint32(index),
			ScriptPubKey: bchain.ScriptPubKey{
				Hex: output.Address,
				Addresses: []string{
					output.Address,
				},
			},
		}
		vouts = append(vouts, vout)

		if lockTime < output.LockTime {
			lockTime = output.LockTime
		}
	}

	tx := &bchain.Tx{
		Hex:           "",
		Txid:          rawTx.Hash,
		Version:       0,
		LockTime:      uint32(lockTime),
		Vin:           vins,
		Vout:          vouts,
		Confirmations: uint32(rawTx.ConfirmCount),
		Time:          rawTx.Time / 1000,
	}

	return tx, nil
}
