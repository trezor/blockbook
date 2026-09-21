//go:build unittest

package api

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

// ignoringParser marks one contract as ignored, the way PolygonParser does for the native token
// contract; the test data has no Polygon address, so the rule is applied to a contract it has.
type ignoringParser struct {
	*eth.EthereumParser
	ignored bchain.AddressDescriptor
}

func (p *ignoringParser) EthereumTypeIsIgnoredContract(contract bchain.AddressDescriptor) bool {
	return bytes.Equal(contract, p.ignored)
}

// balanceRecordingChain records which contracts the token loop asks balances for.
type balanceRecordingChain struct {
	bchain.BlockChain
	parser   bchain.BlockChainParser
	balances []bchain.AddressDescriptor
}

func (c *balanceRecordingChain) GetChainParser() bchain.BlockChainParser {
	return c.parser
}

func (c *balanceRecordingChain) EthereumTypeGetErc20ContractBalance(addrDesc, contractDesc bchain.AddressDescriptor) (*big.Int, error) {
	c.balances = append(c.balances, contractDesc)
	return c.BlockChain.EthereumTypeGetErc20ContractBalance(addrDesc, contractDesc)
}

func (c *balanceRecordingChain) EthereumTypeGetErc20ContractBalances(addrDesc bchain.AddressDescriptor, contractDescs []bchain.AddressDescriptor) ([]*big.Int, error) {
	c.balances = append(c.balances, contractDescs...)
	return c.BlockChain.EthereumTypeGetErc20ContractBalances(addrDesc, contractDescs)
}

// Holdings of an ignored contract indexed before its transfers were dropped are neither listed
// nor looked up, so the fix does not need a reindex to reach the address token list.
func TestGetAddress_SkipsIgnoredContractHoldings(t *testing.T) {
	ethParser := eth.NewEthereumParser(1, true)
	ignored := addrDesc(t, ethParser, dbtestdata.EthAddrContract4a)
	fake, err := dbtestdata.NewFakeBlockChainEthereumType(ethParser)
	require.NoError(t, err)
	chain := &balanceRecordingChain{BlockChain: fake, parser: &ignoringParser{EthereumParser: ethParser, ignored: ignored}}
	w, _, _ := setupContractProbeWorker(t, chain)

	addr, err := w.GetAddress(dbtestdata.EthAddr7bEIP55, 1, 25, AccountDetailsTokenBalances, &AddressFilter{Vout: AddressFilterVoutOff, OnlyConfirmed: true}, "")
	require.NoError(t, err)

	require.NotEmpty(t, addr.Tokens, "the other contracts of the address are still listed")
	for _, token := range addr.Tokens {
		require.NotEqual(t, eth.EIP55Address(ignored), token.Contract)
	}
	for _, contract := range chain.balances {
		require.False(t, bytes.Equal(ignored, contract), "balance looked up for the ignored contract")
	}
	require.NotEmpty(t, chain.balances, "balances of the other contracts are still looked up")
}
