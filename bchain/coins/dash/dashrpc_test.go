// +build integration

package dash

import (
	"blockbook/bchain"
	"blockbook/bchain/tests/rpc"
	"encoding/json"
	"testing"
)

func getRPCClient(cfg json.RawMessage) (bchain.BlockChain, error) {
	c, err := NewDashRPC(cfg, nil)
	if err != nil {
		return nil, err
	}
	cli := c.(*DashRPC)
	cli.Parser = NewDashParser(GetChainParams("test"), cli.ChainConfig)
	return cli, nil
}

var rpcTest *rpc.Test

func init() {
	t, err := rpc.NewTest("Dash Testnet", getRPCClient)
	if err != nil {
		panic(err)
	}
	rpcTest = t
}

func TestDashRPC_GetBlockHash(t *testing.T) {
	rpcTest.TestGetBlockHash(t)
}

func TestDashRPC_GetBlock(t *testing.T) {
	rpcTest.TestGetBlock(t)
}

func TestDashRPC_GetTransaction(t *testing.T) {
	rpcTest.TestGetTransaction(t)
}
