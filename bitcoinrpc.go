package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"time"

	jsoniter "github.com/json-iterator/go"
)

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
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

// BitcoinRPC is an interface to JSON-RPC bitcoind service.
type BitcoinRPC struct {
	client   http.Client
	URL      string
	User     string
	Password string
	Parser   BlockParser
}

// NewBitcoinRPC returns new BitcoinRPC instance.
func NewBitcoinRPC(url string, user string, password string, timeout time.Duration) *BitcoinRPC {
	return &BitcoinRPC{
		client:   http.Client{Timeout: timeout},
		URL:      url,
		User:     user,
		Password: password,
	}
}

// GetBestBlockHash returns hash of the tip of the best-block-chain.
func (b *BitcoinRPC) GetBestBlockHash() (string, error) {
	log.Printf("rpc: getbestblockhash")

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

// GetBlockHash returns hash of block in best-block-chain at given height.
func (b *BitcoinRPC) GetBlockHash(height uint32) (string, error) {
	log.Printf("rpc: getblockhash %v", height)

	res := resGetBlockHash{}
	req := cmdGetBlockHash{Method: "getblockhash"}
	req.Params.Height = height
	err := b.call(&req, &res)

	if err != nil {
		return "", err
	}
	if res.Error != nil {
		return "", res.Error
	}
	return res.Result, nil
}

// GetBlockHeader returns header of block with given hash.
func (b *BitcoinRPC) GetBlockHeader(hash string) (*BlockHeader, error) {
	log.Printf("rpc: getblockheader")

	res := resGetBlockHeaderVerbose{}
	req := cmdGetBlockHeader{Method: "getblockheader"}
	req.Params.BlockHash = hash
	req.Params.Verbose = true
	err := b.call(&req, &res)

	if err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, res.Error
	}
	return &res.Result, nil
}

// GetBlock returns block with given hash.
func (b *BitcoinRPC) GetBlock(hash string) (*Block, error) {
	if b.Parser == nil {
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
		return nil, err
	}
	block.BlockHeader = *header
	return block, nil
}

// GetBlockRaw returns block with given hash as bytes.
func (b *BitcoinRPC) GetBlockRaw(hash string) ([]byte, error) {
	log.Printf("rpc: getblock (verbosity=0) %v", hash)

	res := resGetBlockRaw{}
	req := cmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = 0
	err := b.call(&req, &res)

	if err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, res.Error
	}
	return hex.DecodeString(res.Result)
}

// GetBlockList returns block with given hash by downloading block
// transactions one by one.
func (b *BitcoinRPC) GetBlockList(hash string) (*Block, error) {
	log.Printf("rpc: getblock (verbosity=1) %v", hash)

	res := resGetBlockThin{}
	req := cmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = 1
	err := b.call(&req, &res)

	if err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, res.Error
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
	log.Printf("rpc: getblock (verbosity=2) %v", hash)

	res := resGetBlockFull{}
	req := cmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = 2
	err := b.call(&req, &res)

	if err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, res.Error
	}
	return &res.Result, nil
}

// GetTransaction returns a transaction by the transaction ID.
func (b *BitcoinRPC) GetTransaction(txid string) (*Tx, error) {
	log.Printf("rpc: getrawtransaction %v", txid)

	res := resGetRawTransactionVerbose{}
	req := cmdGetRawTransaction{Method: "getrawtransaction"}
	req.Params.Txid = txid
	req.Params.Verbose = true
	err := b.call(&req, &res)

	if err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, res.Error
	}
	return &res.Result, nil
}

// GetAddress returns address of transaction output.
func (b *BitcoinRPC) GetAddress(txid string, vout uint32) (string, error) {
	tx, err := b.GetTransaction(txid)
	if err != nil {
		return "", err
	}
	return tx.GetAddress(vout), nil
}

func (b *BitcoinRPC) call(req interface{}, res interface{}) error {
	httpData, err := jsoniter.Marshal(req)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequest("POST", b.URL, bytes.NewBuffer(httpData))
	if err != nil {
		return err
	}
	httpReq.SetBasicAuth(b.User, b.Password)
	httpRes, err := b.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpRes.Body.Close()
	return jsoniter.NewDecoder(httpRes.Body).Decode(res)
}
