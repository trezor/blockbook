package eth

import (
	"blockbook/bchain"
	"blockbook/common"
	"context"
	"encoding/json"
	"math/big"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"

	ethcommon "github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

type EthereumNet uint32

const (
	MainNet EthereumNet = 1
	TestNet EthereumNet = 3
)

// EthRPC is an interface to JSON-RPC eth service.
type EthRPC struct {
	client     *ethclient.Client
	timeout    time.Duration
	rpcURL     string
	Parser     *EthParser
	Testnet    bool
	Network    string
	Mempool    *bchain.Mempool
	metrics    *common.Metrics
	bestHeader *ethtypes.Header
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
	s := &EthRPC{
		client:  ec,
		rpcURL:  c.RPCURL,
		metrics: metrics,
	}

	// always create parser
	s.Parser = &EthParser{}
	s.timeout = time.Duration(c.RPCTimeout) * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	id, err := ec.NetworkID(ctx)
	if err != nil {
		return nil, err
	}

	// parameters for getInfo request
	switch EthereumNet(id.Uint64()) {
	case MainNet:
		s.Testnet = false
		s.Network = "livenet"
		break
	case TestNet:
		s.Testnet = true
		s.Network = "testnet"
		break
	default:
		return nil, errors.Errorf("Unknown network id %v", id)
	}
	glog.Info("rpc: block chain ", s.Network)

	// s.Mempool = bchain.NewMempool(s, metrics)

	return s, nil
}

func (b *EthRPC) Shutdown() error {
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
