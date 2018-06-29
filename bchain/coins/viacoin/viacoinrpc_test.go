// +build integration

package viacoin

import (
	"blockbook/bchain"
	"blockbook/bchain/tests/rpc"
	"encoding/json"
	"flag"
	"os"
	"testing"
)

func getRPCClient(cfg json.RawMessage) (bchain.BlockChain, error) {
	c, err := NewViacoinRPC(cfg, nil)
	if err != nil {
		return nil, err
	}
	cli := c.(*ViacoinRPC)
	cli.Parser = NewViacoinParser(GetChainParams("main"), cli.ChainConfig)
	if err != nil {
		return nil, err
	}
	cli.Mempool = bchain.NewUTXOMempool(cli, cli.ChainConfig.MempoolWorkers, cli.ChainConfig.MempoolSubWorkers)
	return cli, nil
}

var rpcTest *rpc.Test

func TestMain(m *testing.M) {
	flag.Parse()
	t, err := rpc.NewTest("Viacoin", getRPCClient)
	if err != nil {
		panic(err)
	}

	rpcTest = t

	os.Exit(m.Run())
}

func TestViacoinRPC_GetBlockHash(t *testing.T) {
	rpcTest.TestGetBlockHash(t)
}

func TestViacoinRPC_GetBlock(t *testing.T) {
	rpcTest.TestGetBlock(t)
}

func TestViacoinRPC_GetTransaction(t *testing.T) {
	rpcTest.TestGetTransaction(t)
}

func TestViacoinRPC_GetTransactionForMempool(t *testing.T) {
	// extra opcodes (name_new, name_firstupdate, name_update) aren't supported, so some transactions
	// in mempool can't be parsed correctly
	t.Skipf("Skipped because of instability")
}

func TestViacoinRPC_MempoolSync(t *testing.T) {
	rpcTest.TestMempoolSync(t)
}

func TestViacoinRPC_GetMempoolEntry(t *testing.T) {
	rpcTest.TestGetMempoolEntry(t)
}

func TestViacoinRPC_SendRawTransaction(t *testing.T) {
	rpcTest.TestSendRawTransaction(t)
}

func TestViacoinRPC_EstimateSmartFee(t *testing.T) {
	rpcTest.TestEstimateSmartFee(t)
}

func TestViacoinRPC_EstimateFee(t *testing.T) {
	rpcTest.TestEstimateFee(t)
}
