//go:build integration

package eth

import (
	"time"

	"github.com/trezor/blockbook/bchain"
)

// NewERC20BatchIntegrationClient builds an ERC20-capable RPC client for integration tests.
// EVM chains share ERC20 balanceOf semantics (eth_call) and coin wrappers embed EthereumRPC.
func NewERC20BatchIntegrationClient(rpcURL, rpcURLWS string, batchSize int) (bchain.ERC20BatchClient, func(), error) {
	rc, ec, err := OpenRPC(rpcURL, rpcURLWS)
	if err != nil {
		return nil, nil, err
	}
	client := &EthereumRPC{
		Client:      ec,
		RPC:         rc,
		Timeout:     15 * time.Second,
		ChainConfig: &Configuration{RPCURL: rpcURL, RPCURLWS: rpcURLWS, Erc20BatchSize: batchSize},
	}
	return client, func() { rc.Close() }, nil
}
