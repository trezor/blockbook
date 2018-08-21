// +build integration

package zec

import (
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/tests/rpc"
	"encoding/json"
	"flag"
	"os"
	"testing"
)

func getRPCClient(cfg json.RawMessage) (bchain.BlockChain, error) {
	c, err := NewZCashRPC(cfg, nil)
	if err != nil {
		return nil, err
	}
	cli := c.(*ZCashRPC)
	cli.Parser = NewZCashParser(cli.ChainConfig)
	cli.Mempool = bchain.NewUTXOMempool(cli, cli.ChainConfig.MempoolWorkers, cli.ChainConfig.MempoolSubWorkers)
	return cli, nil
}

var tests struct {
	mainnet *rpc.Test
	testnet *rpc.Test
}

func TestMain(m *testing.M) {
	flag.Parse()

	t, err := rpc.NewTest("Zcash", getRPCClient)
	if err != nil {
		panic(err)
	}

	tests.mainnet = t

	t, err = rpc.NewTest("Zcash Testnet", getRPCClient)
	if err != nil {
		panic(err)
	}

	tests.testnet = t

	os.Exit(m.Run())
}

func TestZCashRPC_GetBlockHash(t *testing.T) {
	tests.mainnet.TestGetBlockHash(t)
}

func TestZCashRPC_GetBlock(t *testing.T) {
	tests.mainnet.TestGetBlock(t)
}

func TestZCashRPC_GetTransaction(t *testing.T) {
	tests.mainnet.TestGetTransaction(t)
}

func TestZCashRPC_GetTransactionForMempool(t *testing.T) {
	tests.mainnet.TestGetTransactionForMempool(t)
}

func TestZCashRPC_MempoolSync(t *testing.T) {
	tests.mainnet.TestMempoolSync(t)
}

func TestZCashRPC_EstimateSmartFee(t *testing.T) {
	tests.mainnet.TestEstimateSmartFee(t)
}

func TestZCashRPC_EstimateFee(t *testing.T) {
	tests.mainnet.TestEstimateFee(t)
}

func TestZCashRPC_GetBestBlockHash(t *testing.T) {
	tests.mainnet.TestGetBestBlockHash(t)
}

func TestZCashRPC_GetBestBlockHeight(t *testing.T) {
	tests.mainnet.TestGetBestBlockHeight(t)
}

func TestZCashRPC_GetBlockHeader(t *testing.T) {
	tests.mainnet.TestGetBlockHeader(t)
}

func TestZCashTestnetRPC_GetBlockHash(t *testing.T) {
	tests.testnet.TestGetBlockHash(t)
}

func TestZCashTestnetRPC_GetBlock(t *testing.T) {
	tests.testnet.TestGetBlock(t)
}

func TestZCashTestnetRPC_GetTransaction(t *testing.T) {
	tests.testnet.TestGetTransaction(t)
}

func TestZCashTestnetRPC_GetTransactionForMempool(t *testing.T) {
	tests.testnet.TestGetTransactionForMempool(t)
}

func TestZCashTestnetRPC_MempoolSync(t *testing.T) {
	tests.testnet.TestMempoolSync(t)
}

func TestZCashTestnetRPC_EstimateSmartFee(t *testing.T) {
	tests.testnet.TestEstimateSmartFee(t)
}

func TestZCashTestnetRPC_EstimateFee(t *testing.T) {
	tests.testnet.TestEstimateFee(t)
}

func TestZCashTestnetRPC_GetBestBlockHash(t *testing.T) {
	tests.mainnet.TestGetBestBlockHash(t)
}

func TestZCashTestnetRPC_GetBestBlockHeight(t *testing.T) {
	tests.mainnet.TestGetBestBlockHeight(t)
}

func TestZCashTestnetRPC_GetBlockHeader(t *testing.T) {
	tests.mainnet.TestGetBlockHeader(t)
}
