// +build integration

package myriad

import (
	"blockbook/bchain"
	"blockbook/bchain/tests/rpc"
	"encoding/json"
	"flag"
	"os"
	"testing"
)

func getRPCClient(cfg json.RawMessage) (bchain.BlockChain, error) {
	c, err := NewMyriadRPC(cfg, nil)
	if err != nil {
		return nil, err
	}
	cli := c.(*MyriadRPC)
	cli.Parser = NewMyriadParser(GetChainParams("main"), cli.ChainConfig)
	if err != nil {
		return nil, err
	}
	cli.Mempool = bchain.NewUTXOMempool(cli, cli.ChainConfig.MempoolWorkers, cli.ChainConfig.MempoolSubWorkers)
	return cli, nil
}

var rpcTest *rpc.Test

func TestMain(m *testing.M) {
	flag.Parse()
	t, err := rpc.NewTest("Myriad", getRPCClient)
	if err != nil {
		panic(err)
	}

	rpcTest = t

	os.Exit(m.Run())
}

func TestMyriadRPC_GetBlockHash(t *testing.T) {
	rpcTest.TestGetBlockHash(t)
}

func TestMyriadRPC_GetBlock(t *testing.T) {
	rpcTest.TestGetBlock(t)
}

func TestMyriadRPC_GetTransaction(t *testing.T) {
	rpcTest.TestGetTransaction(t)
}

func TestMyriadRPC_GetTransactionForMempool(t *testing.T) {
	rpcTest.TestGetTransactionForMempool(t)
}

func TestMyriadRPC_MempoolSync(t *testing.T) {
	rpcTest.TestMempoolSync(t)
}

func TestMyriadRPC_GetMempoolEntry(t *testing.T) {
	rpcTest.TestGetMempoolEntry(t)
}

func TestMyriadRPC_EstimateSmartFee(t *testing.T) {
	rpcTest.TestEstimateSmartFee(t)
}

func TestMyriadRPC_EstimateFee(t *testing.T) {
	rpcTest.TestEstimateFee(t)
}
