package eth

import (
	"blockbook/bchain"
	"errors"

	ethcommon "github.com/ethereum/go-ethereum/common"
)

type EthParser struct {
}

func (p *EthParser) GetAddrIDFromVout(output *bchain.Vout) ([]byte, error) {
	if len(output.ScriptPubKey.Addresses) != 1 {
		return nil, nil
	}
	return p.GetAddrIDFromAddress(output.ScriptPubKey.Addresses[0])
}

func (p *EthParser) GetAddrIDFromAddress(address string) ([]byte, error) {
	a := ethcommon.HexToAddress(address)
	return a.Bytes(), nil
}

func (p *EthParser) AddressToOutputScript(address string) ([]byte, error) {
	return nil, errors.New("AddressToOutputScript: not implemented")
}

func (p *EthParser) OutputScriptToAddresses(script []byte) ([]string, error) {
	return nil, errors.New("OutputScriptToAddresses: not implemented")
}

func (p *EthParser) ParseTx(b []byte) (*bchain.Tx, error) {
	return nil, errors.New("ParseTx: not implemented")
}

func (p *EthParser) ParseBlock(b []byte) (*bchain.Block, error) {
	return nil, errors.New("ParseBlock: not implemented")
}

func (p *EthParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	return nil, errors.New("PackTx: not implemented")
}

func (p *EthParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	return nil, 0, errors.New("UnpackTx: not implemented")
}

func (p *EthParser) IsUTXOChain() bool {
	return false
}
