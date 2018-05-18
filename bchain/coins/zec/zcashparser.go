package zec

import (
	"blockbook/bchain"
)

// ZCashParser handle
type ZCashParser struct {
	*bchain.BaseParser
}

// NewZCAshParser returns new ZCAshParser instance
func NewZCashParser() *ZCashParser {
	return &ZCashParser{&bchain.BaseParser{AddressFactory: bchain.NewBaseAddress}}
}

// GetAddrIDFromVout returns internal address representation of given transaction output
func (p *ZCashParser) GetAddrIDFromVout(output *bchain.Vout) ([]byte, error) {
	if len(output.ScriptPubKey.Addresses) != 1 {
		return nil, nil
	}
	hash, _, err := CheckDecode(output.ScriptPubKey.Addresses[0])
	return hash, err
}

// GetAddrIDFromAddress returns internal address representation of given address
func (p *ZCashParser) GetAddrIDFromAddress(address string) ([]byte, error) {
	hash, _, err := CheckDecode(address)
	return hash, err
}
