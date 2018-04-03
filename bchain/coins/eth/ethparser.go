package eth

import (
	"blockbook/bchain"
	"encoding/hex"
	"errors"
)

type EthereumParser struct {
}

func (p *EthereumParser) GetAddrIDFromVout(output *bchain.Vout) ([]byte, error) {
	if len(output.ScriptPubKey.Addresses) != 1 {
		return nil, bchain.ErrAddressMissing
	}
	return p.GetAddrIDFromAddress(output.ScriptPubKey.Addresses[0])
}

func (p *EthereumParser) GetAddrIDFromAddress(address string) ([]byte, error) {
	// github.com/ethereum/go-ethereum/common.HexToAddress does not handle address errors, using own decoding
	if len(address) > 1 {
		if address[0:2] == "0x" || address[0:2] == "0X" {
			address = address[2:]
		}
	} else {
		if len(address) == 0 {
			return nil, bchain.ErrAddressMissing
		}
		return nil, errors.New("Invalid address")
	}
	if len(address)&1 == 1 {
		address = "0" + address
	}
	return hex.DecodeString(address)
}

func (p *EthereumParser) AddressToOutputScript(address string) ([]byte, error) {
	return nil, errors.New("AddressToOutputScript: not implemented")
}

func (p *EthereumParser) OutputScriptToAddresses(script []byte) ([]string, error) {
	return nil, errors.New("OutputScriptToAddresses: not implemented")
}

func (p *EthereumParser) ParseTx(b []byte) (*bchain.Tx, error) {
	return nil, errors.New("ParseTx: not implemented")
}

func (p *EthereumParser) ParseBlock(b []byte) (*bchain.Block, error) {
	return nil, errors.New("ParseBlock: not implemented")
}

func (p *EthereumParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return nil, errors.New("PackTx: not implemented")
}

func (p *EthereumParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return nil, 0, errors.New("UnpackTx: not implemented")
}

func (p *EthereumParser) IsUTXOChain() bool {
	return false
}
