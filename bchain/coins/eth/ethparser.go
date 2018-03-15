package eth

import "blockbook/bchain"

type EthParser struct {
}

func (p *EthParser) AddressToOutputScript(address string) ([]byte, error) {
	panic("not implemented")
}

func (p *EthParser) OutputScriptToAddresses(script []byte) ([]string, error) {
	panic("not implemented")
}

func (p *EthParser) ParseTx(b []byte) (*bchain.Tx, error) {
	panic("not implemented")
}

func (p *EthParser) ParseBlock(b []byte) (*bchain.Block, error) {
	panic("not implemented")
}

func (p *EthParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	panic("not implemented")
}

func (p *EthParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	panic("not implemented")
}
