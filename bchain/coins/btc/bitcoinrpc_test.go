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

func getRPCClient(chain string) func(json.RawMessage) (bchain.BlockChain, error) {
	return func(cfg json.RawMessage) (bchain.BlockChain, error) {
		c, err := NewBitcoinRPC(cfg, nil)
		if err != nil {
			return nil, err
		}
		cli := c.(*BitcoinRPC)
		cli.Parser = NewBitcoinParser(GetChainParams(chain), cli.ChainConfig)
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
	t, err := rpc.NewTest("Bitcoin", getRPCClient("main"))
	if err != nil {
		panic(err)
	}

	tests.mainnet = t

	t, err = rpc.NewTest("Bitcoin Testnet", getRPCClient("test"))
	if err != nil {
		panic(err)
	}

	tests.testnet = t

	os.Exit(m.Run())
}

func TestBitcoinRPC_GetBlockHash(t *testing.T) {
	tests.mainnet.TestGetBlockHash(t)
}

func TestBitcoinRPC_GetBlock(t *testing.T) {
	tests.mainnet.TestGetBlock(t)
}

func TestBitcoinRPC_GetTransaction(t *testing.T) {
	tests.mainnet.TestGetTransaction(t)
}

func TestBitcoinRPC_GetTransactionForMempool(t *testing.T) {
	tests.mainnet.TestGetTransactionForMempool(t)
}

func TestBitcoinRPC_MempoolSync(t *testing.T) {
	tests.mainnet.TestMempoolSync(t)
}

func TestBitcoinRPC_GetMempoolEntry(t *testing.T) {
	tests.mainnet.TestGetMempoolEntry(t)
}

func TestBitcoinRPC_SendRawTransaction(t *testing.T) {
	tests.mainnet.TestSendRawTransaction(t)
}

func TestBitcoinRPC_EstimateSmartFee(t *testing.T) {
	tests.mainnet.TestEstimateSmartFee(t)
}

func TestBitcoinRPC_EstimateFee(t *testing.T) {
	tests.mainnet.TestEstimateFee(t)
}

func TestBitcoinRPC_GetBestBlockHash(t *testing.T) {
	tests.mainnet.TestGetBestBlockHash(t)
}

func TestBitcoinRPC_GetBestBlockHeight(t *testing.T) {
	tests.mainnet.TestGetBestBlockHeight(t)
}

func TestBitcoinRPC_GetBlockHeader(t *testing.T) {
	tests.mainnet.TestGetBlockHeader(t)
}

func TestBitcoinTestnetRPC_GetBlockHash(t *testing.T) {
	tests.testnet.TestGetBlockHash(t)
}

func TestBitcoinTestnetRPC_GetBlock(t *testing.T) {
	tests.testnet.TestGetBlock(t)
}

func TestBitcoinTestnetRPC_GetTransaction(t *testing.T) {
	tests.testnet.TestGetTransaction(t)
}

func TestBitcoinTestnetRPC_GetTransactionForMempool(t *testing.T) {
	tests.testnet.TestGetTransactionForMempool(t)
}

func TestBitcoinTestnetRPC_MempoolSync(t *testing.T) {
	tests.testnet.TestMempoolSync(t)
}

func TestBitcoinTestnetRPC_GetMempoolEntry(t *testing.T) {
	tests.testnet.TestGetMempoolEntry(t)
}

func TestBitcoinTestnetRPC_SendRawTransaction(t *testing.T) {
	tests.testnet.TestSendRawTransaction(t)
}

func TestBitcoinTestnetRPC_EstimateSmartFee(t *testing.T) {
	tests.testnet.TestEstimateSmartFee(t)
}

func TestBitcoinTestnetRPC_EstimateFee(t *testing.T) {
	tests.testnet.TestEstimateFee(t)
}

func TestBitcoinTestnetRPC_GetBestBlockHash(t *testing.T) {
	tests.testnet.TestGetBestBlockHash(t)
}

func TestBitcoinTestnetRPC_GetBestBlockHeight(t *testing.T) {
	tests.testnet.TestGetBestBlockHeight(t)
}

func TestBitcoinTestnetRPC_GetBlockHeader(t *testing.T) {
	tests.testnet.TestGetBlockHeader(t)
}
