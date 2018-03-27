package coins

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/bchain/coins/eth"
	"blockbook/bchain/coins/zec"
	"blockbook/common"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"reflect"
	"time"

	"github.com/juju/errors"
)

type blockChainFactory func(config json.RawMessage, pushHandler func(*bchain.MQMessage)) (bchain.BlockChain, error)

var blockChainFactories = make(map[string]blockChainFactory)

func init() {
	blockChainFactories["btc"] = btc.NewBitcoinRPC
	blockChainFactories["btc-testnet"] = btc.NewBitcoinRPC
	blockChainFactories["zec"] = zec.NewZCashRPC
	blockChainFactories["eth"] = eth.NewEthRPC
	blockChainFactories["eth-testnet"] = eth.NewEthRPC
}

// NewBlockChain creates bchain.BlockChain of type defined by parameter coin
func NewBlockChain(coin string, configfile string, pushHandler func(*bchain.MQMessage), metrics *common.Metrics) (bchain.BlockChain, error) {
	bcf, ok := blockChainFactories[coin]
	if !ok {
		return nil, errors.New(fmt.Sprint("Unsupported coin ", coin, ". Must be one of ", reflect.ValueOf(blockChainFactories).MapKeys()))
	}
	data, err := ioutil.ReadFile(configfile)
	if err != nil {
		return nil, errors.Annotatef(err, "Error reading file %v", configfile)
	}
	var config json.RawMessage
	err = json.Unmarshal(data, &config)
	if err != nil {
		return nil, errors.Annotatef(err, "Error parsing file %v", configfile)
	}
	bc, err := bcf(config, pushHandler)
	if err != nil {
		return nil, err
	}
	err = bc.Initialize()
	if err != nil {
		return nil, err
	}
	return &blockChainWithMetrics{b: bc, m: metrics}, nil
}

type blockChainWithMetrics struct {
	b bchain.BlockChain
	m *common.Metrics
}

func (c *blockChainWithMetrics) observeRPCLatency(method string, start time.Time, err error) {
	var e string
	if err != nil {
		e = err.Error()
	}
	c.m.RPCLatency.With(common.Labels{"method": method, "error": e}).Observe(float64(time.Since(start)) / 1e6) // in milliseconds
}

func (c *blockChainWithMetrics) Initialize() error {
	return c.b.Initialize()
}

func (c *blockChainWithMetrics) Shutdown() error {
	return c.b.Shutdown()
}

func (c *blockChainWithMetrics) IsTestnet() bool {
	return c.b.IsTestnet()
}

func (c *blockChainWithMetrics) GetNetworkName() string {
	return c.b.GetNetworkName()
}

func (c *blockChainWithMetrics) GetBestBlockHash() (v string, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetBestBlockHash", s, err) }(time.Now())
	return c.b.GetBestBlockHash()
}

func (c *blockChainWithMetrics) GetBestBlockHeight() (v uint32, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetBestBlockHeight", s, err) }(time.Now())
	return c.b.GetBestBlockHeight()
}

func (c *blockChainWithMetrics) GetBlockHash(height uint32) (v string, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetBlockHash", s, err) }(time.Now())
	return c.b.GetBlockHash(height)
}

func (c *blockChainWithMetrics) GetBlockHeader(hash string) (v *bchain.BlockHeader, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetBlockHeader", s, err) }(time.Now())
	return c.b.GetBlockHeader(hash)
}

func (c *blockChainWithMetrics) GetBlock(hash string, height uint32) (v *bchain.Block, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetBlock", s, err) }(time.Now())
	return c.b.GetBlock(hash, height)
}

func (c *blockChainWithMetrics) GetMempool() (v []string, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetMempool", s, err) }(time.Now())
	return c.b.GetMempool()
}

func (c *blockChainWithMetrics) GetTransaction(txid string) (v *bchain.Tx, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetTransaction", s, err) }(time.Now())
	return c.b.GetTransaction(txid)
}

func (c *blockChainWithMetrics) EstimateSmartFee(blocks int, conservative bool) (v float64, err error) {
	defer func(s time.Time) { c.observeRPCLatency("EstimateSmartFee", s, err) }(time.Now())
	return c.b.EstimateSmartFee(blocks, conservative)
}

func (c *blockChainWithMetrics) EstimateFee(blocks int) (v float64, err error) {
	defer func(s time.Time) { c.observeRPCLatency("EstimateFee", s, err) }(time.Now())
	return c.b.EstimateFee(blocks)
}

func (c *blockChainWithMetrics) SendRawTransaction(tx string) (v string, err error) {
	defer func(s time.Time) { c.observeRPCLatency("SendRawTransaction", s, err) }(time.Now())
	return c.b.SendRawTransaction(tx)
}

func (c *blockChainWithMetrics) ResyncMempool(onNewTxAddr func(txid string, addr string)) (err error) {
	defer func(s time.Time) { c.observeRPCLatency("ResyncMempool", s, err) }(time.Now())
	return c.b.ResyncMempool(onNewTxAddr)
}

func (c *blockChainWithMetrics) GetMempoolTransactions(address string) (v []string, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetMempoolTransactions", s, err) }(time.Now())
	return c.b.GetMempoolTransactions(address)
}

func (c *blockChainWithMetrics) GetMempoolSpentOutput(outputTxid string, vout uint32) (v string) {
	return c.b.GetMempoolSpentOutput(outputTxid, vout)
}

func (c *blockChainWithMetrics) GetMempoolEntry(txid string) (v *bchain.MempoolEntry, err error) {
	defer func(s time.Time) { c.observeRPCLatency("GetMempoolEntry", s, err) }(time.Now())
	return c.b.GetMempoolEntry(txid)
}

func (c *blockChainWithMetrics) GetChainParser() bchain.BlockChainParser {
	return c.b.GetChainParser()
}
