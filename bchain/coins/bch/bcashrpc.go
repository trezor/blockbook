package bch

import (
	"encoding/hex"
	"encoding/json"
	"math/big"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/martinboehm/bchutil"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// BCashRPC is an interface to JSON-RPC bitcoind service.
type BCashRPC struct {
	*btc.BitcoinRPC
}

// NewBCashRPC returns new BCashRPC instance.
func NewBCashRPC(config json.RawMessage, pushHandler func(bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &BCashRPC{
		b.(*btc.BitcoinRPC),
	}
	s.ChainConfig.SupportsEstimateSmartFee = false

	return s, nil
}

// Initialize initializes BCashRPC instance.
func (b *BCashRPC) Initialize() error {
	ci, err := b.GetChainInfo()
	if err != nil {
		return err
	}
	chainName := ci.Chain

	params := GetChainParams(chainName)

	// always create parser
	b.Parser, err = NewBCashParser(params, b.ChainConfig)

	if err != nil {
		return err
	}

	// parameters for getInfo request
	if params.Net == bchutil.MainnetMagic {
		b.Testnet = false
		b.Network = "livenet"
	} else {
		b.Testnet = true
		b.Network = "testnet"
	}

	glog.Info("rpc: block chain ", params.Name)

	return nil
}

// getblock

type cmdGetBlock struct {
	Method string `json:"method"`
	Params struct {
		BlockHash string `json:"blockhash"`
		Verbose   bool   `json:"verbose"`
	} `json:"params"`
}

// estimatesmartfee

type cmdEstimateSmartFee struct {
	Method string `json:"method"`
	Params struct {
		Blocks int `json:"nblocks"`
	} `json:"params"`
}

// GetBlock returns block with given hash.
func (b *BCashRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	var err error
	if hash == "" && height > 0 {
		hash, err = b.GetBlockHash(height)
		if err != nil {
			return nil, err
		}
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
	// size is not returned by GetBlockHeader and would be overwritten
	size := block.Size
	block.BlockHeader = *header
	block.Size = size
	return block, nil
}

// GetBlockRaw returns block with given hash as bytes.
func (b *BCashRPC) GetBlockRaw(hash string) (string, error) {
	glog.V(1).Info("rpc: getblock (verbose=0) ", hash)

	res := btc.ResGetBlockRaw{}
	req := cmdGetBlock{Method: "getblock"}
	req.Params.BlockHash = hash
	req.Params.Verbose = false
	err := b.Call(&req, &res)

	if err != nil {
		return "", errors.Annotatef(err, "hash %v", hash)
	}
	if res.Error != nil {
		if isErrBlockNotFound(res.Error) {
			return "", bchain.ErrBlockNotFound
		}
		return "", errors.Annotatef(res.Error, "hash %v", hash)
	}
	return res.Result, nil
}

// GetBlockBytes returns block with given hash as bytes
func (b *BCashRPC) GetBlockBytes(hash string) ([]byte, error) {
	block, err := b.GetBlockRaw(hash)
	if err != nil {
		return nil, err
	}
	return hex.DecodeString(block)
}

// GetBlockInfo returns extended header (more info than in bchain.BlockHeader) with a list of txids
func (b *BCashRPC) GetBlockInfo(hash string) (*bchain.BlockInfo, error) {
	glog.V(1).Info("rpc: getblock (verbosity=1) ", hash)

	res := btc.ResGetBlockInfo{}
	req := cmdGetBlock{Method: "getblock"}
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

// GetBlockFull returns block with given hash.
func (b *BCashRPC) GetBlockFull(hash string) (*bchain.Block, error) {
	return nil, errors.New("Not implemented")
}

func isErrBlockNotFound(err *bchain.RPCError) bool {
	return err.Message == "Block not found" ||
		err.Message == "Block height out of range"
}

// EstimateFee returns fee estimation
func (b *BCashRPC) EstimateFee(blocks int) (big.Int, error) {
	//  from version BitcoinABC version 0.19.1 EstimateFee does not support parameter Blocks
	if b.ChainConfig.CoinShortcut == "BCHSV" {
		return b.BitcoinRPC.EstimateFee(blocks)
	}

	glog.V(1).Info("rpc: estimatefee ", blocks)

	res := btc.ResEstimateFee{}
	req := struct {
		Method string `json:"method"`
	}{
		Method: "estimatefee",
	}

	err := b.Call(&req, &res)

	var r big.Int
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

// EstimateSmartFee returns fee estimation
func (b *BCashRPC) EstimateSmartFee(blocks int, conservative bool) (big.Int, error) {
	// EstimateSmartFee is not supported by bcash
	return b.EstimateFee(blocks)
}
