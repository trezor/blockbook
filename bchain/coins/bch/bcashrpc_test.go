// +build integration

package bch

import (
	"blockbook/bchain"
	"blockbook/bchain/tests/rpc"
	"encoding/json"
	"flag"
	"os"
	"testing"
)

func getRPCClient(chain string) func(json.RawMessage) (bchain.BlockChain, error) {
	return func(cfg json.RawMessage) (bchain.BlockChain, error) {
		c, err := NewBCashRPC(cfg, nil)
		if err != nil {
			return nil, err
		}
		cli := c.(*BCashRPC)
		cli.Parser, err = NewBCashParser(GetChainParams(chain), cli.ChainConfig)
		if err != nil {
			return nil, err
		}
		cli.Mempool = bchain.NewUTXOMempool(cli, cli.ChainConfig.MempoolWorkers, cli.ChainConfig.MempoolSubWorkers)
		return cli, nil
	}
}

var tests struct {
	mainnet *rpc.Test
	testnet *rpc.Test
}

func TestMain(m *testing.M) {
	flag.Parse()

	t, err := rpc.NewTest("Bcash", getRPCClient("main"))
	if err != nil {
		panic(err)
	}

	tests.mainnet = t

	t, err = rpc.NewTest("Bcash Testnet", getRPCClient("test"))
	if err != nil {
		panic(err)
	}

	tests.testnet = t

	os.Exit(m.Run())
}

func TestBCashRPC_GetBlockHash(t *testing.T) {
	tests.mainnet.TestGetBlockHash(t)
}

func TestBCashRPC_GetBlock(t *testing.T) {
	tests.mainnet.TestGetBlock(t)
}

func TestBCashRPC_GetTransaction(t *testing.T) {
	tests.mainnet.TestGetTransaction(t)
}

func TestBCashRPC_GetTransactionForMempool(t *testing.T) {
	tests.mainnet.TestGetTransactionForMempool(t)
}

func TestBCashRPC_MempoolSync(t *testing.T) {
	tests.mainnet.TestMempoolSync(t)
}

func TestBCashRPC_GetMempoolEntry(t *testing.T) {
	tests.mainnet.TestGetMempoolEntry(t)
}

func TestBCashRPC_EstimateSmartFee(t *testing.T) {
	tests.mainnet.TestEstimateSmartFee(t)
}

func TestBCashRPC_EstimateFee(t *testing.T) {
	tests.mainnet.TestEstimateFee(t)
}

func TestBCashRPC_GetBestBlockHash(t *testing.T) {
	tests.mainnet.TestGetBestBlockHash(t)
}

func TestBCashRPC_GetBestBlockHeight(t *testing.T) {
	tests.mainnet.TestGetBestBlockHeight(t)
}

func TestBCashRPC_GetBlockHeader(t *testing.T) {
	tests.mainnet.TestGetBlockHeader(t)
}

func TestBCashTestnetRPC_GetBlockHash(t *testing.T) {
	tests.testnet.TestGetBlockHash(t)
}

func TestBCashTestnetRPC_GetBlock(t *testing.T) {
	tests.testnet.TestGetBlock(t)
}

func TestBCashTestnetRPC_GetTransaction(t *testing.T) {
	tests.testnet.TestGetTransaction(t)
}

func TestBCashTestnetRPC_GetTransactionForMempool(t *testing.T) {
	tests.testnet.TestGetTransactionForMempool(t)
}

func TestBCashTestnetRPC_MempoolSync(t *testing.T) {
	tests.testnet.TestMempoolSync(t)
}

func TestBCashTestnetRPC_GetMempoolEntry(t *testing.T) {
	tests.testnet.TestGetMempoolEntry(t)
}

func TestBCashTestnetRPC_EstimateSmartFee(t *testing.T) {
	tests.testnet.TestEstimateSmartFee(t)
}

func TestBCashTestnetRPC_EstimateFee(t *testing.T) {
	tests.testnet.TestEstimateFee(t)
}

func TestBCashTestnetRPC_GetBestBlockHash(t *testing.T) {
	tests.testnet.TestGetBestBlockHash(t)
}

func TestBCashTestnetRPC_GetBestBlockHeight(t *testing.T) {
	tests.testnet.TestGetBestBlockHeight(t)
}

func TestBCashTestnetRPC_GetBlockHeader(t *testing.T) {
	tests.testnet.TestGetBlockHeader(t)
}
