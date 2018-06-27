// +build integration

package btc

import (
	"blockbook/bchain"
	"blockbook/bchain/tests/rpc"
	"encoding/json"
	"flag"
	"os"
	"testing"
)

func getRPCClient(cfg json.RawMessage) (bchain.BlockChain, error) {
	c, err := NewBitcoinRPC(cfg, nil)
	if err != nil {
		return nil, err
	}
	cli := c.(*BitcoinRPC)
	cli.Parser = NewBitcoinParser(GetChainParams("test"), cli.ChainConfig)
	if err != nil {
		return nil, err
	}
	cli.Mempool = bchain.NewUTXOMempool(cli, cli.ChainConfig.MempoolWorkers, cli.ChainConfig.MempoolSubWorkers)
	return cli, nil
}

var rpcTest *rpc.Test

func TestMain(m *testing.M) {
	flag.Parse()
	t, err := rpc.NewTest("Bitcoin Testnet", getRPCClient)
	if err != nil {
		panic(err)
	}
	t.TryConnect()

	rpcTest = t

	os.Exit(m.Run())
}

func TestBitcoinRPC_GetBlockHash(t *testing.T) {
	rpcTest.TestGetBlockHash(t)
}

func TestBitcoinRPC_GetBlock(t *testing.T) {
	rpcTest.TestGetBlock(t)
}

func TestBitcoinRPC_GetTransaction(t *testing.T) {
	rpcTest.TestGetTransaction(t)
}

func TestBitcoinRPC_TestGetTransactionForMempool(t *testing.T) {
	rpcTest.TestGetTransactionForMempool(t)
}

func TestBitcoinRPC_TestMempoolSync(t *testing.T) {
	rpcTest.TestMempoolSync(t)
}

func TestBitcoinRPC_GetMempoolEntry(t *testing.T) {
	rpcTest.TestGetMempoolEntry(t)
}

func TestBitcoinRPC_SendRawTransaction(t *testing.T) {
	rpcTest.TestSendRawTransaction(t)
}

func TestBitcoinRPC_EstimateSmartFee(t *testing.T) {
	rpcTest.TestEstimateSmartFee(t)
}

func TestBitcoinRPC_EstimateFee(t *testing.T) {
	rpcTest.TestEstimateFee(t)
}
