package monetaryunit

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/hex"
	"encoding/json"

	"github.com/golang/glog"
	"github.com/juju/errors"
)

// MonetaryUnitRPC is an interface to JSON-RPC bitcoind service.
type MonetaryUnitRPC struct {
	*btc.BitcoinRPC
}

// NewMonetaryUnitRPC returns new MonetaryUnitRPC instance.
func NewMonetaryUnitRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &MonetaryUnitRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV1{}
	s.ChainConfig.SupportsEstimateFee = true
	s.ChainConfig.SupportsEstimateSmartFee = false

	return s, nil
}

// Initialize initializes MonetaryUnitRPC instance.
func (b *MonetaryUnitRPC) Initialize() error {
	ci, err := b.GetChainInfo()
	if err != nil {
		return err
	}
	chainName := ci.Chain

	glog.Info("Chain name ", chainName)
	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewMonetaryUnitParser(params, b.ChainConfig)

	// parameters for getInfo request
	if params.Net == MainnetMagic {
		b.Testnet = false
		b.Network = "livenet"
	} else {
		b.Testnet = true
		b.Network = "testnet"
	}

	glog.Info("rpc: block chain ", params.Name)

	return nil
}

// Get Block
func (b *MonetaryUnitRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	var err error

	if hash == "" {
		hash, err = b.GetBlockHash(height)
		if err != nil {
			return nil, err
		}
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

func (b *MonetaryUnitRPC) GetBlockInfo(hash string) (*bchain.BlockInfo, error) {
	glog.V(1).Info("rpc: getblock (verbosity=true) ", hash)

	res := btc.ResGetBlockInfo{}
	req := cmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = true
	err := b.Call(&req, &res)

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

func (b *MonetaryUnitRPC) GetBlockWithoutHeader(hash string, height uint32) (*bchain.Block, error) {
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

func (b *MonetaryUnitRPC) GetBlockRaw(hash string) ([]byte, error) {
	glog.V(1).Info("rpc: getblock (verbosity=false) ", hash)

	res := btc.ResGetBlockRaw{}
	req := cmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbosity = false
	err := b.Call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		if btc.IsErrBlockNotFound(res.Error) {
			return nil, bchain.ErrBlockNotFound
		}
		return nil, errors.Annotatef(res.Error, "hash %v", hash)
	}
	return hex.DecodeString(res.Result)
}

func (b *MonetaryUnitRPC) GetTransactionForMempool(txid string) (*bchain.Tx, error) {
	glog.V(1).Info("rpc: getrawtransaction nonverbose ", txid)

	res := btc.ResGetRawTransactionNonverbose{}
	req := cmdGetRawTransaction{Method: "getrawtransaction"}
	req.Params.Txid = txid
	req.Params.Verbose = 0
	err := b.Call(&req, &res)
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
	tx, err := b.Parser.ParseTx(data)
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}
	return tx, nil
}

func (b *MonetaryUnitRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	r, err := b.getRawTransaction(txid)
	if err != nil {
		return nil, err
	}

	tx, err := b.Parser.ParseTxFromJson(r)
	tx.CoinSpecificData = r
	if err != nil {
		return nil, errors.Annotatef(err, "txid %v", txid)
	}

	return tx, nil
}

func (b *MonetaryUnitRPC) GetTransactionSpecific(tx *bchain.Tx) (json.RawMessage, error) {
	if csd, ok := tx.CoinSpecificData.(json.RawMessage); ok {
		return csd, nil
	}
	return b.getRawTransaction(tx.Txid)
}

func (b *MonetaryUnitRPC) getRawTransaction(txid string) (json.RawMessage, error) {
	glog.V(1).Info("rpc: getrawtransaction ", txid)

	res := btc.ResGetRawTransaction{}
	req := cmdGetRawTransaction{Method: "getrawtransaction"}
	req.Params.Txid = txid
	req.Params.Verbose = 1
	err := b.Call(&req, &res)

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
