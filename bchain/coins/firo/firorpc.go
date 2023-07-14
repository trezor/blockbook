package firo

import (
	"encoding/hex"
	"encoding/json"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

type FiroRPC struct {
	*btc.BitcoinRPC
}

func NewFiroRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	// init base implementation
	bc, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	// init firo implementation
	zc := &FiroRPC{
		BitcoinRPC: bc.(*btc.BitcoinRPC),
	}

	zc.ChainConfig.Parse = true
	zc.ChainConfig.SupportsEstimateFee = true
	zc.ChainConfig.SupportsEstimateSmartFee = false
	zc.ParseBlocks = true
	zc.RPCMarshaler = btc.JSONMarshalerV1{}

	return zc, nil
}

func (zc *FiroRPC) Initialize() error {
	ci, err := zc.GetChainInfo()
	if err != nil {
		return err
	}
	chainName := ci.Chain

	params := GetChainParams(chainName)

	// always create parser
	zc.Parser = NewFiroParser(params, zc.ChainConfig)

	// parameters for getInfo request
	if params.Net == MainnetMagic {
		zc.Testnet = false
		zc.Network = "livenet"
	} else {
		zc.Testnet = true
		zc.Network = "testnet"
	}

	glog.Info("rpc: block chain ", params.Name)

	return nil
}

func (zc *FiroRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	var err error

	if hash == "" {
		hash, err = zc.GetBlockHash(height)
		if err != nil {
			return nil, err
		}
	}

	// optimization
	if height > 0 {
		return zc.GetBlockWithoutHeader(hash, height)
	}

	header, err := zc.GetBlockHeader(hash)
	if err != nil {
		return nil, err
	}

	data, err := zc.GetBlockBytes(hash)
	if err != nil {
		return nil, err
	}

	block, err := zc.Parser.ParseBlock(data)
	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}

	block.BlockHeader = *header

	return block, nil
}

func (zc *FiroRPC) GetBlockInfo(hash string) (*bchain.BlockInfo, error) {
	glog.V(1).Info("rpc: getblock (verbosity=true) ", hash)

	res := btc.ResGetBlockInfo{}
	req := cmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = true
	err := zc.Call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		if btc.IsErrBlockNotFound(res.Error) {
			return nil, bchain.ErrBlockNotFound
		}
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}
	return &res.Result, nil
}

func (zc *FiroRPC) GetBlockWithoutHeader(hash string, height uint32) (*bchain.Block, error) {
	data, err := zc.GetBlockBytes(hash)
	if err != nil {
		return nil, err
	}

	block, err := zc.Parser.ParseBlock(data)
	if err != nil {
		return nil, errors.Annotatef(err, "%v %v", height, hash)
	}

	block.BlockHeader.Hash = hash
	block.BlockHeader.Height = height

	return block, nil
}

// GetBlockRaw returns block with given hash as hex string
func (zc *FiroRPC) GetBlockRaw(hash string) (string, error) {
	glog.V(1).Info("rpc: getblock (verbosity=false) ", hash)

	res := btc.ResGetBlockRaw{}
	req := cmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = false
	err := zc.Call(&req, &res)

	if err != nil {
		return "", errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		if btc.IsErrBlockNotFound(res.Error) {
			return "", bchain.ErrBlockNotFound
		}
		return "", errors.Annotatef(res.Error, "hash %v", hash)
	}
	return res.Result, nil
}

// GetBlockBytes returns block with given hash as bytes
func (zc *FiroRPC) GetBlockBytes(hash string) ([]byte, error) {
	block, err := zc.GetBlockRaw(hash)
	if err != nil {
		return nil, err
	}
	return hex.DecodeString(block)
}

func (zc *FiroRPC) GetTransactionForMempool(txid string) (*bchain.Tx, error) {
	glog.V(1).Info("rpc: getrawtransaction nonverbose ", txid)

	res := btc.ResGetRawTransactionNonverbose{}
	req := cmdGetRawTransaction{Method: "getrawtransaction"}
	req.Params.Txid = txid
	req.Params.Verbose = 0
	err := zc.Call(&req, &res)
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	if res.Error != nil {
		if btc.IsMissingTx(res.Error) {
			return nil, bchain.ErrTxNotFound
		}
		return nil, errors.Annotatef(res.Error, "txid %v", txid)
	}
	data, err := hex.DecodeString(res.Result)
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	tx, err := zc.Parser.ParseTx(data)
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	return tx, nil
}

func (zc *FiroRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	r, err := zc.getRawTransaction(txid)
	if err != nil {
		return nil, err
	}

	tx, err := zc.Parser.ParseTxFromJson(r)
	tx.CoinSpecificData = r
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}

	return tx, nil
}

func (zc *FiroRPC) GetTransactionSpecific(tx *bchain.Tx) (json.RawMessage, error) {
	if csd, ok := tx.CoinSpecificData.(json.RawMessage); ok {
		return csd, nil
	}
	return zc.getRawTransaction(tx.Txid)
}

func (zc *FiroRPC) getRawTransaction(txid string) (json.RawMessage, error) {
	glog.V(1).Info("rpc: getrawtransaction ", txid)

	res := btc.ResGetRawTransaction{}
	req := cmdGetRawTransaction{Method: "getrawtransaction"}
	req.Params.Txid = txid
	req.Params.Verbose = 1
	err := zc.Call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	if res.Error != nil {
		if btc.IsMissingTx(res.Error) {
			return nil, bchain.ErrTxNotFound
		}
		return nil, errors.Annotatef(res.Error, "txid %v", txid)
	}
	return res.Result, nil
}

type cmdGetBlock struct {
	Method string `json:"method"`
	Params struct {
		BlockHash string `json:"blockhash"`
		Verbosity bool   `json:"verbosity"`
	} `json:"params"`
}

type cmdGetRawTransaction struct {
	Method string `json:"method"`
	Params struct {
		Txid    string `json:"txid"`
		Verbose int    `json:"verbose"`
	} `json:"params"`
}
