// +build integration

package zec

import (
	"blockbook/bchain"
	"blockbook/bchain/tests/rpc"
	"encoding/json"
	"testing"
)

func getRPCClient(cfg json.RawMessage) (bchain.BlockChain, error) {
	c, err := NewZCashRPC(cfg, nil)
	if err != nil {
		return nil, err
	}
	cli := c.(*ZCashRPC)
	cli.Parser = NewZCashParser(cli.ChainConfig)
	return cli, nil
}

var rpcTest *rpc.Test

func init() {
	t, err := rpc.NewTest("Zcash Testnet", getRPCClient)
	if err != nil {
		panic(err)
	}
	rpcTest = t
}

func TestZCashRPC_GetBlockHash(t *testing.T) {
	rpcTest.TestGetBlockHash(t)
}

func TestZCashRPC_GetBlock(t *testing.T) {
	rpcTest.TestGetBlock(t)
}

func TestZCashRPC_GetTransaction(t *testing.T) {
	rpcTest.TestGetTransaction(t)
}
