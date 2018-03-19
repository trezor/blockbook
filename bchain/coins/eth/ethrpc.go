package eth

import (
	"blockbook/bchain"
	"blockbook/common"
	"context"
	"encoding/json"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"

	"github.com/ethereum/go-ethereum/ethclient"
)

// EthRPC is an interface to JSON-RPC eth service.
type EthRPC struct {
	client    *ethclient.Client
	ctx       context.Context
	ctxCancel context.CancelFunc
	rpcURL    string
	Parser    *EthParser
	Testnet   bool
	Network   string
	Mempool   *bchain.Mempool
	metrics   *common.Metrics
}

type configuration struct {
	RPCURL     string `json:"rpcURL"`
	RPCTimeout int    `json:"rpcTimeout"`
}

// NewEthRPC returns new EthRPC instance.
func NewEthRPC(config json.RawMessage, pushHandler func(*bchain.MQMessage), metrics *common.Metrics) (bchain.BlockChain, error) {
	var err error
	var c configuration
	err = json.Unmarshal(config, &c)
	if err != nil {
		return nil, errors.Annotatef(err, "Invalid configuration file")
	}
	ec, err := ethclient.Dial(c.RPCURL)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.RPCTimeout)*time.Second)
	s := &EthRPC{
		client:    ec,
		ctx:       ctx,
		ctxCancel: cancel,
		rpcURL:    c.RPCURL,
		metrics:   metrics,
	}

	// always create parser
	s.Parser = &EthParser{}

	h, err := ec.HeaderByNumber(s.ctx, nil)
	if err != nil {
		return nil, err
	}
	glog.Info("best block ", h.Number)

	// // parameters for getInfo request
	// if s.Parser.Params.Net == wire.MainNet {
	// 	s.Testnet = false
	// 	s.Network = "livenet"
	// } else {
	// 	s.Testnet = true
	// 	s.Network = "testnet"
	// }

	// s.Mempool = bchain.NewMempool(s, metrics)

	// glog.Info("rpc: block chain ", s.Parser.Params.Name)
	return s, nil
}

func (b *EthRPC) Shutdown() error {
	if b.ctxCancel != nil {
		b.ctxCancel()
	}
	return nil
}

func (b *EthRPC) IsTestnet() bool {
	panic("not implemented")
}

func (b *EthRPC) GetNetworkName() string {
	panic("not implemented")
}

func (b *EthRPC) GetBestBlockHash() (string, error) {
	panic("not implemented")
}

func (b *EthRPC) GetBestBlockHeight() (uint32, error) {
	panic("not implemented")
}

func (b *EthRPC) GetBlockHash(height uint32) (string, error) {
	panic("not implemented")
}

func (b *EthRPC) GetBlockHeader(hash string) (*bchain.BlockHeader, error) {
	panic("not implemented")
}

func (b *EthRPC) GetBlock(hash string, height uint32) (*bchain.Block, error) {
	panic("not implemented")
}

func (b *EthRPC) GetMempool() ([]string, error) {
	panic("not implemented")
}

func (b *EthRPC) GetTransaction(txid string) (*bchain.Tx, error) {
	panic("not implemented")
}

func (b *EthRPC) EstimateSmartFee(blocks int, conservative bool) (float64, error) {
	panic("not implemented")
}

func (b *EthRPC) SendRawTransaction(tx string) (string, error) {
	panic("not implemented")
}

func (b *EthRPC) ResyncMempool(onNewTxAddr func(txid string, addr string)) error {
	panic("not implemented")
}

func (b *EthRPC) GetMempoolTransactions(outputScript []byte) ([]string, error) {
	panic("not implemented")
}

func (b *EthRPC) GetMempoolSpentOutput(outputTxid string, vout uint32) string {
	panic("not implemented")
}

func (b *EthRPC) GetMempoolEntry(txid string) (*bchain.MempoolEntry, error) {
	panic("not implemented")
}

func (b *EthRPC) GetChainParser() bchain.BlockChainParser {
	panic("not implemented")
}
