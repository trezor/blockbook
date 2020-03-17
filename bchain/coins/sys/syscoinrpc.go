package syscoin

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"encoding/json"
	"encoding/hex"
	"github.com/juju/errors"
	"github.com/golang/glog"
)

// SyscoinRPC is an interface to JSON-RPC bitcoind service
type SyscoinRPC struct {
	*btc.BitcoinRPC
}

// NewSyscoinRPC returns new SyscoinRPC instance
func NewSyscoinRPC(config json.RawMessage, pushHandler func(notificationType bchain.NotificationType)) (bchain.BlockChain, error) {
	b, err := btc.NewBitcoinRPC(config, pushHandler)
	if err != nil {
		return nil, err
	}

	s := &SyscoinRPC{
		b.(*btc.BitcoinRPC),
	}
	s.RPCMarshaler = btc.JSONMarshalerV2{}
	s.ChainConfig.SupportsEstimateFee = false

	return s, nil
}

// Initialize initializes SyscoinRPC instance.
func (b *SyscoinRPC) Initialize() error {
	ci, err := b.GetChainInfo()
	if err != nil {
		return err
	}
	chainName := ci.Chain

	glog.Info("Chain name ", chainName)
	params := GetChainParams(chainName)

	// always create parser
	b.Parser = NewSyscoinParser(params, b.ChainConfig)

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

// GetBlock returns block with given hash
func (b *SyscoinRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
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
	return b.GetBlockWithoutHeader(hash, height)
}

type CmdAssetAllocationSend struct {
	Method string `json:"method"`
	Params struct {
		Asset    int    `json:"asset_guid"`
		Sender   string `json:"address_sender"`
		Receiver string `json:"address_receiver"`
		Amount   string `json:"amount"`
	} `json:"params"`
}
type CmdSendFrom struct {
	Method string `json:"method"`
	Params struct {
		Sender   string `json:"funding_address"`
		Receiver string `json:"address"`
		Amount   string `json:"amount"`
	} `json:"params"`
}
type ResSyscoinSend struct {
	Error  *bchain.RPCError `json:"error"`
	Result json.RawMessage      `json:"result"`
}
type GetSyscoinTxHex struct {
	Hex string `json:"hex"`
}
type CmdDecodeRawTransaction struct {
	Method string `json:"method"`
	Params struct {
		Hexstring    string `json:"hexstring"`
	} `json:"params"`
}
func (b *SyscoinRPC) AssetAllocationSend(asset int, sender string, receiver string, amount string) (*bchain.Tx, error) {
	glog.V(1).Info("rpc: assetallocationsend ", asset)

	res := ResSyscoinSend{}
	req := CmdAssetAllocationSend{Method: "assetallocationsend"}
	req.Params.Asset = asset
	req.Params.Sender = sender
	req.Params.Receiver = receiver
	req.Params.Amount = amount
	err := b.Call(&req, &res)

	if err != nil {
		return nil, errors.Annotatef(err, "asset %v", asset)
	}
	if res.Error != nil {
		return nil, errors.Annotatef(res.Error, "asset %v", asset)
	}
	var resHex GetSyscoinTxHex
	err = json.Unmarshal(res.Result, &resHex)
	if err != nil {
		return nil, errors.Annotatef(err, "Unmarshal")
	}


	data, err := hex.DecodeString(resHex.Hex)
	if err != nil {
		return nil, errors.Annotatef(err, "asset %v", asset)
	}
	tx, err := b.Parser.ParseTx(data)
	if err != nil {
		return nil, errors.Annotatef(err, "asset %v", asset)
	}

	req = CmdDecodeRawTransaction{Method: "decoderawtransaction"}
	req.Params.Hexstring = resHex.Hex
	err = b.Call(&req, &tx.CoinSpecificData)
	if err != nil {
		return nil, errors.Annotatef(err, "decoderawtransaction for asset %v", asset)
	}
	return tx, nil
}
func (b *SyscoinRPC) SendFrom(sender string, receiver string, amount string) (*bchain.Tx, error) {
	glog.V(1).Info("rpc: sendfrom ", sender)

	res := ResSyscoinSend{}
	req := CmdSendFrom{Method: "sendfrom"}
	req.Params.Sender = sender
	req.Params.Receiver = receiver
	req.Params.Amount = amount
	err := b.Call(&req, &res)

	if err != nil {
		return nil, err
	}
	if res.Error != nil {
		return nil, res.Error
	}
	var resHex GetSyscoinTxHex
	err = json.Unmarshal(res.Result, &resHex)
	if err != nil {
		return nil, err
	}
	data, err := hex.DecodeString(resHex.Hex)
	if err != nil {
		return nil, err
	}
	tx, err := b.Parser.ParseTx(data)
	if err != nil {
		return nil, err
	}
	req = CmdDecodeRawTransaction{Method: "decoderawtransaction"}
	req.Params.Hexstring = resHex.Hex
	err = b.Call(&req, &tx.CoinSpecificData)
	if err != nil {
		return nil, err
	}
	return tx, nil
}