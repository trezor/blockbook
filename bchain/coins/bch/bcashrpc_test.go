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
	return cli, nil
}

var rpcTest *rpc.Test

func init() {
	t, err := rpc.NewTest("bch", getRPCClient)
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
