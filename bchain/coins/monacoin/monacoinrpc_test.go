// +build integration

package monacoin

import (
	"blockbook/bchain"
	"blockbook/bchain/tests/rpc"
	"encoding/json"
	"flag"
	"os"
	"testing"
)

func getRPCClient(cfg json.RawMessage) (bchain.BlockChain, error) {
	c, err := NewMonacoinRPC(cfg, nil)
	if err != nil {
		return nil, err
	}
	cli := c.(*MonacoinRPC)
	cli.Parser = NewMonacoinParser(GetChainParams("main"), cli.ChainConfig)
	if err != nil {
		return nil, err
	}
	cli.Mempool = bchain.NewUTXOMempool(cli, cli.ChainConfig.MempoolWorkers, cli.ChainConfig.MempoolSubWorkers)
	return cli, nil
}

var rpcTest *rpc.Test

func TestMain(m *testing.M) {
	flag.Parse()
	t, err := rpc.NewTest("Monacoin", getRPCClient)
	if err != nil {
		panic(err)
	}

	rpcTest = t

	os.Exit(m.Run())
}

func TestMonacoinRPC_GetBlockHash(t *testing.T) {
	rpcTest.TestGetBlockHash(t)
}

func TestMonacoinRPC_GetBlock(t *testing.T) {
	rpcTest.TestGetBlock(t)
}

func TestMonacoinRPC_GetTransaction(t *testing.T) {
	rpcTest.TestGetTransaction(t)
}

func TestMonacoinRPC_GetTransactionForMempool(t *testing.T) {
	rpcTest.TestGetTransactionForMempool(t)
}

func TestMonacoinRPC_MempoolSync(t *testing.T) {
	rpcTest.TestMempoolSync(t)
}

func TestMonacoinRPC_GetMempoolEntry(t *testing.T) {
	rpcTest.TestGetMempoolEntry(t)
}

func TestMonacoinRPC_EstimateSmartFee(t *testing.T) {
	rpcTest.TestEstimateSmartFee(t)
}

func TestMonacoinRPC_EstimateFee(t *testing.T) {
	rpcTest.TestEstimateFee(t)
}
