// +build integration

package vertcoin

import (
	"blockbook/bchain"
	"blockbook/bchain/tests/rpc"
	"encoding/json"
	"flag"
	"os"
	"testing"
)

func getRPCClient(cfg json.RawMessage) (bchain.BlockChain, error) {
	c, err := NewVertcoinRPC(cfg, nil)
	if err != nil {
		return nil, err
	}
	cli := c.(*VertcoinRPC)
	cli.Parser = NewVertcoinParser(GetChainParams("main"), cli.ChainConfig)
	if err != nil {
		return nil, err
	}
	cli.Mempool = bchain.NewUTXOMempool(cli, cli.ChainConfig.MempoolWorkers, cli.ChainConfig.MempoolSubWorkers)
	return cli, nil
}

var rpcTest *rpc.Test

func TestMain(m *testing.M) {
	flag.Parse()
	t, err := rpc.NewTest("Vertcoin", getRPCClient)
	if err != nil {
		panic(err)
	}
	t.TryConnect()

	rpcTest = t

	os.Exit(m.Run())
}

func TestVertcoinRPC_GetBlockHash(t *testing.T) {
	rpcTest.TestGetBlockHash(t)
}

func TestVertcoinRPC_GetBlock(t *testing.T) {
	rpcTest.TestGetBlock(t)
}

func TestVertcoinRPC_GetTransaction(t *testing.T) {
	rpcTest.TestGetTransaction(t)
}

func TestVertcoinRPC_TestGetTransactionForMempool(t *testing.T) {
	rpcTest.TestGetTransactionForMempool(t)
}

func TestVertcoinRPC_TestMempoolSync(t *testing.T) {
	rpcTest.TestMempoolSync(t)
}

func TestVertcoinRPC_GetMempoolEntry(t *testing.T) {
	rpcTest.TestGetMempoolEntry(t)
}

func TestVertcoinRPC_SendRawTransaction(t *testing.T) {
	rpcTest.TestSendRawTransaction(t)
}

func TestVertcoinRPC_EstimateSmartFee(t *testing.T) {
	rpcTest.TestEstimateSmartFee(t)
}

func TestVertcoinRPC_EstimateFee(t *testing.T) {
	rpcTest.TestEstimateFee(t)
}
