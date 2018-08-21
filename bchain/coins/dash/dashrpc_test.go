// +build integration

package dash

import (
	"encoding/json"
	"flag"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/tests/rpc"
	"os"
	"testing"
)

func getRPCClient(chain string) func(json.RawMessage) (bchain.BlockChain, error) {
	return func(cfg json.RawMessage) (bchain.BlockChain, error) {
		c, err := NewDashRPC(cfg, nil)
		if err != nil {
			return nil, err
		}
		cli := c.(*DashRPC)
		cli.Parser = NewDashParser(GetChainParams(chain), cli.ChainConfig)
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
	t, err := rpc.NewTest("Dash", getRPCClient("main"))
	if err != nil {
		panic(err)
	}

	tests.mainnet = t

	t, err = rpc.NewTest("Dash Testnet", getRPCClient("test"))
	if err != nil {
		panic(err)
	}

	tests.testnet = t

	os.Exit(m.Run())
}

func TestDashRPC_GetBlockHash(t *testing.T) {
	tests.mainnet.TestGetBlockHash(t)
}

func TestDashRPC_GetBlock(t *testing.T) {
	tests.mainnet.TestGetBlock(t)
}

func TestDashRPC_GetTransaction(t *testing.T) {
	tests.mainnet.TestGetTransaction(t)
}

func TestDashRPC_GetTransactionForMempool(t *testing.T) {
	tests.mainnet.TestGetTransactionForMempool(t)
}

func TestDashRPC_MempoolSync(t *testing.T) {
	tests.mainnet.TestMempoolSync(t)
}

func TestDashRPC_EstimateSmartFee(t *testing.T) {
	tests.mainnet.TestEstimateSmartFee(t)
}

func TestDashRPC_EstimateFee(t *testing.T) {
	tests.mainnet.TestEstimateFee(t)
}

func TestDashRPC_GetBestBlockHash(t *testing.T) {
	tests.mainnet.TestGetBestBlockHash(t)
}

func TestDashRPC_GetBestBlockHeight(t *testing.T) {
	tests.mainnet.TestGetBestBlockHeight(t)
}

func TestDashRPC_GetBlockHeader(t *testing.T) {
	tests.mainnet.TestGetBlockHeader(t)
}

func TestDashTestnetRPC_GetBlockHash(t *testing.T) {
	tests.testnet.TestGetBlockHash(t)
}

func TestDashTestnetRPC_GetBlock(t *testing.T) {
	tests.testnet.TestGetBlock(t)
}

func TestDashTestnetRPC_GetTransaction(t *testing.T) {
	tests.testnet.TestGetTransaction(t)
}

func TestDashTestnetRPC_GetTransactionForMempool(t *testing.T) {
	tests.testnet.TestGetTransactionForMempool(t)
}

func TestDashTestnetRPC_MempoolSync(t *testing.T) {
	tests.testnet.TestMempoolSync(t)
}

func TestDashTestnetRPC_EstimateSmartFee(t *testing.T) {
	tests.testnet.TestEstimateSmartFee(t)
}

func TestDashTestnetRPC_EstimateFee(t *testing.T) {
	tests.testnet.TestEstimateFee(t)
}

func TestDashTestnetRPC_GetBestBlockHash(t *testing.T) {
	tests.testnet.TestGetBestBlockHash(t)
}

func TestDashTestnetRPC_GetBestBlockHeight(t *testing.T) {
	tests.testnet.TestGetBestBlockHeight(t)
}

func TestDashTestnetRPC_GetBlockHeader(t *testing.T) {
	tests.testnet.TestGetBlockHeader(t)
}
