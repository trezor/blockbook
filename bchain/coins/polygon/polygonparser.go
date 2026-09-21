package polygon

import (
	"bytes"
	"strings"

	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

// nativeTokenContract is the MRC20 system contract through which native POL moves. It emits an
// ERC-20 Transfer for every such move, which Blockbook already indexes as an internal transfer,
// so treating it as a token would count the same POL twice.
const nativeTokenContract = "0x0000000000000000000000000000000000001010"

var nativeTokenContractDesc = mustAddrDesc(nativeTokenContract)

func mustAddrDesc(address string) bchain.AddressDescriptor {
	desc, err := eth.NewEthereumParser(1, false).GetAddrDescFromAddress(address)
	if err != nil {
		panic(err)
	}
	return desc
}

// PolygonParser is the Ethereum parser without the native token pseudo-contract.
type PolygonParser struct {
	*eth.EthereumParser
}

// EthereumTypeGetTokenTransfersFromTx drops the native token contract's transfers.
func (p *PolygonParser) EthereumTypeGetTokenTransfersFromTx(tx *bchain.Tx) (bchain.TokenTransfers, error) {
	transfers, err := p.EthereumParser.EthereumTypeGetTokenTransfersFromTx(tx)
	if err != nil {
		return nil, err
	}
	// copy on the first hit only, the common case has nothing to drop
	var filtered bchain.TokenTransfers
	for i, t := range transfers {
		if strings.EqualFold(t.Contract, nativeTokenContract) {
			if filtered == nil {
				filtered = append(make(bchain.TokenTransfers, 0, len(transfers)-1), transfers[:i]...)
			}
			continue
		}
		if filtered != nil {
			filtered = append(filtered, t)
		}
	}
	if filtered == nil {
		return transfers, nil
	}
	return filtered, nil
}

// EthereumTypeIsIgnoredContract reports the native token contract, whose holdings indexed before
// its transfers were dropped must not be served as a token.
func (p *PolygonParser) EthereumTypeIsIgnoredContract(contract bchain.AddressDescriptor) bool {
	return bytes.Equal(contract, nativeTokenContractDesc)
}
