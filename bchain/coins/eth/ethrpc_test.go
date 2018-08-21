// +build integration

package eth

import (
	"encoding/json"
	"flag"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/tests/rpc"
	"os"
	"testing"
)

func getRPCClient(cfg json.RawMessage) (bchain.BlockChain, error) {
	c, err := NewEthereumRPC(cfg, nil)
	if err != nil {
		return nil, err
	}
	return c, nil
}

var rpcTest *rpc.Test

func TestMain(m *testing.M) {
	flag.Parse()
	t, err := rpc.NewTest("Ethereum Testnet", getRPCClient)
	if err != nil {
		panic(err)
	}

	rpcTest = t

	os.Exit(m.Run())
}

func TestEthRPC_GetBlockHash(t *testing.T) {
	rpcTest.TestGetBlockHash(t)
}

func TestEthRPC_GetBlock(t *testing.T) {
	rpcTest.TestGetBlock(t)
}

func TestEthRPC_GetTransaction(t *testing.T) {
	rpcTest.TestGetTransaction(t)
}

func TestEthRPC_GetBestBlockHash(t *testing.T) {
	rpcTest.TestGetBestBlockHash(t)
}

func TestEthRPC_GetBestBlockHeight(t *testing.T) {
	rpcTest.TestGetBestBlockHeight(t)
}

func TestEthRPC_GetBlockHeader(t *testing.T) {
	rpcTest.TestGetBlockHeader(t)
}

func TestEthRPC_EstimateFee(t *testing.T) {
	rpcTest.TestEstimateFee(t)
}
