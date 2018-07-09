// +build integration

package digibyte

import (
	"blockbook/bchain"
	"blockbook/bchain/tests/rpc"
	"encoding/json"
	"flag"
	"os"
	"testing"
)

func getRPCClient(cfg json.RawMessage) (bchain.BlockChain, error) {
	c, err := NewDigiByteRPC(cfg, nil)
	if err != nil {
		return nil, err
	}
	cli := c.(*DigiByteRPC)
	cli.Parser = NewDigiByteParser(GetChainParams("main"), cli.ChainConfig)
	if err != nil {
		return nil, err
	}
	cli.Mempool = bchain.NewUTXOMempool(cli, cli.ChainConfig.MempoolWorkers, cli.ChainConfig.MempoolSubWorkers)
	return cli, nil
}

var rpcTest *rpc.Test

func TestMain(m *testing.M) {
	flag.Parse()
	t, err := rpc.NewTest("DigiByte", getRPCClient)
	if err != nil {
		panic(err)
	}

	rpcTest = t

	os.Exit(m.Run())
}

func TestDigiByteRPC_GetBlockHash(t *testing.T) {
	rpcTest.TestGetBlockHash(t)
}

func TestDigiByteRPC_GetBlock(t *testing.T) {
	rpcTest.TestGetBlock(t)
}

func TestDigiByteRPC_GetTransaction(t *testing.T) {
	rpcTest.TestGetTransaction(t)
}

func TestDigiByteRPC_GetTransactionForMempool(t *testing.T) {
	rpcTest.TestGetTransactionForMempool(t)
}

func TestDigiByteRPC_MempoolSync(t *testing.T) {
	rpcTest.TestMempoolSync(t)
}

func TestDigiByteRPC_GetMempoolEntry(t *testing.T) {
	rpcTest.TestGetMempoolEntry(t)
}

func TestDigiByteRPC_SendRawTransaction(t *testing.T) {
	rpcTest.TestSendRawTransaction(t)
}

func TestDigiByteRPC_EstimateSmartFee(t *testing.T) {
	rpcTest.TestEstimateSmartFee(t)
}

func TestDigiByteRPC_EstimateFee(t *testing.T) {
	rpcTest.TestEstimateFee(t)
}
