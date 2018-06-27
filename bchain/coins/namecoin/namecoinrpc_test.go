// +build integration

package namecoin

import (
	"blockbook/bchain"
	"blockbook/bchain/tests/rpc"
	"encoding/json"
	"flag"
	"os"
	"testing"
)

func getRPCClient(cfg json.RawMessage) (bchain.BlockChain, error) {
	c, err := NewNamecoinRPC(cfg, nil)
	if err != nil {
		return nil, err
	}
	cli := c.(*NamecoinRPC)
	cli.Parser = NewNamecoinParser(GetChainParams("main"), cli.ChainConfig)
	if err != nil {
		return nil, err
	}
	cli.Mempool = bchain.NewUTXOMempool(cli, cli.ChainConfig.MempoolWorkers, cli.ChainConfig.MempoolSubWorkers)
	return cli, nil
}

var rpcTest *rpc.Test

func TestMain(m *testing.M) {
	flag.Parse()
	t, err := rpc.NewTest("Namecoin", getRPCClient)
	if err != nil {
		panic(err)
	}
	t.TryConnect()

	rpcTest = t

	os.Exit(m.Run())
}

func TestNamecoinRPC_GetBlockHash(t *testing.T) {
	rpcTest.TestGetBlockHash(t)
}

func TestNamecoinRPC_GetBlock(t *testing.T) {
	rpcTest.TestGetBlock(t)
}

func TestNamecoinRPC_GetTransaction(t *testing.T) {
	rpcTest.TestGetTransaction(t)
}

func TestNamecoinRPC_GetTransactionForMempool(t *testing.T) {
	// extra opcodes (name_new, name_firstupdate, name_update) aren't supported, so some transactions
	// in mempool can't be parsed correctly
	t.Skipf("Skipped because of instability")
}

func TestNamecoinRPC_MempoolSync(t *testing.T) {
	rpcTest.TestMempoolSync(t)
}

func TestNamecoinRPC_GetMempoolEntry(t *testing.T) {
	rpcTest.TestGetMempoolEntry(t)
}

func TestNamecoinRPC_SendRawTransaction(t *testing.T) {
	rpcTest.TestSendRawTransaction(t)
}

func TestNamecoinRPC_EstimateSmartFee(t *testing.T) {
	rpcTest.TestEstimateSmartFee(t)
}

func TestNamecoinRPC_EstimateFee(t *testing.T) {
	rpcTest.TestEstimateFee(t)
}
