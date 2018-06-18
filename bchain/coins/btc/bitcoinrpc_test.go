// +build integration

package btc

import (
	"blockbook/bchain"
	"blockbook/bchain/tests/rpc"
	"encoding/json"
	"testing"
)

func getRPCClient(cfg json.RawMessage) (bchain.BlockChain, error) {
	c, err := NewBitcoinRPC(cfg, nil)
	if err != nil {
		return nil, err
	}
	cli := c.(*BitcoinRPC)
	cli.Parser = NewBitcoinParser(GetChainParams("test"), cli.ChainConfig)
	if err != nil {
		return nil, err
	}
	return cli, nil
}

var rpcTest *rpc.Test

func init() {
	t, err := rpc.NewTest("Bitcoin Testnet", getRPCClient)
	if err != nil {
		panic(err)
	}
	rpcTest = t
}

func TestBitcoinRPC_GetBlockHash(t *testing.T) {
	rpcTest.TestGetBlockHash(t)
}

func TestBitcoinRPC_GetBlock(t *testing.T) {
	rpcTest.TestGetBlock(t)
}

func TestBitcoinRPC_GetTransaction(t *testing.T) {
	rpcTest.TestGetTransaction(t)
}
