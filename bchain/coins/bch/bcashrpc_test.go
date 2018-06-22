// +build integration

package bch

import (
	"blockbook/bchain"
	"blockbook/bchain/tests/rpc"
	"encoding/json"
	"testing"
)

func getRPCClient(cfg json.RawMessage) (bchain.BlockChain, error) {
	c, err := NewBCashRPC(cfg, nil)
	if err != nil {
		return nil, err
	}
	cli := c.(*BCashRPC)
	cli.Parser, err = NewBCashParser(GetChainParams("test"), cli.ChainConfig)
	if err != nil {
		return nil, err
	}
	cli.Mempool = bchain.NewUTXOMempool(cli, cli.ChainConfig.MempoolWorkers, cli.ChainConfig.MempoolSubWorkers)
	return cli, nil
}

var rpcTest *rpc.Test

func init() {
	t, err := rpc.NewTest("Bcash Testnet", getRPCClient)
	if err != nil {
		panic(err)
	}
	rpcTest = t
}

func TestBCashRPC_GetBlockHash(t *testing.T) {
	rpcTest.TestGetBlockHash(t)
}

func TestBCashRPC_GetBlock(t *testing.T) {
	rpcTest.TestGetBlock(t)
}

func TestBCashRPC_GetTransaction(t *testing.T) {
	rpcTest.TestGetTransaction(t)
}

func TestBCashRPC_TestGetTransactionForMempool(t *testing.T) {
	rpcTest.TestGetTransactionForMempool(t)
}

func TestBCashRPC_TestMempoolSync(t *testing.T) {
	rpcTest.TestMempoolSync(t)
}

func TestBCashRPC_GetMempoolEntry(t *testing.T) {
	rpcTest.TestGetMempoolEntry(t)
}

func TestBCashRPC_SendRawTransaction(t *testing.T) {
	rpcTest.TestSendRawTransaction(t)
}

func TestBCashRPC_EstimateSmartFee(t *testing.T) {
	rpcTest.TestEstimateSmartFee(t)
}

func TestBCashRPC_EstimateFee(t *testing.T) {
	rpcTest.TestEstimateFee(t)
}
