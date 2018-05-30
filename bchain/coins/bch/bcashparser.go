package bch

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"fmt"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcutil"
	"github.com/cpacia/bchutil"
	"github.com/schancel/cashaddr-converter/address"
)

type AddressFormat = uint8

const (
	Legacy AddressFormat = iota
	CashAddr
)

const (
	MainNetPrefix = "bitcoincash:"
	TestNetPrefix = "bchtest:"
	RegTestPrefix = "bchreg:"
)

// BCashParser handle
type BCashParser struct {
	*btc.BitcoinParser
	AddressFormat AddressFormat
}

// NewBCashParser returns new BCashParser instance
func NewBCashParser(params *chaincfg.Params, c *btc.Configuration) (*BCashParser, error) {
	var format AddressFormat
	switch c.AddressFormat {
	case "":
		fallthrough
	case "cashaddr":
		format = CashAddr
	case "legacy":
		format = Legacy
	default:
		return nil, fmt.Errorf("Unknown address format: %s", c.AddressFormat)
	}
	p := &BCashParser{
		BitcoinParser: &btc.BitcoinParser{
			BaseParser: &bchain.BaseParser{
				AddressFactory:       func(addr string) (bchain.Address, error) { return newBCashAddress(addr, format) },
				BlockAddressesToKeep: c.BlockAddressesToKeep,
			},
			Params:                  params,
			OutputScriptToAddresses: outputScriptToAddresses,
		},
		AddressFormat: format,
	}
	return p, nil
}

// GetChainParams contains network parameters for the main Bitcoin Cash network,
// the regression test Bitcoin Cash network, the test Bitcoin Cash network and
// the simulation test Bitcoin Cash network, in this order
func GetChainParams(chain string) *chaincfg.Params {
	var params *chaincfg.Params
	switch chain {
	case "test":
		params = &chaincfg.TestNet3Params
		params.Net = bchutil.TestnetMagic
	case "regtest":
		params = &chaincfg.RegressionNetParams
		params.Net = bchutil.Regtestmagic
	default:
		params = &chaincfg.MainNetParams
		params.Net = bchutil.MainnetMagic
	}

	return params
}

// GetAddrIDFromAddress returns internal address representation of given address
func (p *BCashParser) GetAddrIDFromAddress(address string) ([]byte, error) {
	return p.AddressToOutputScript(address)
}

// AddressToOutputScript converts bitcoin address to ScriptPubKey
func (p *BCashParser) AddressToOutputScript(address string) ([]byte, error) {
	if isCashAddr(address) {
		da, err := bchutil.DecodeAddress(address, p.Params)
		if err != nil {
			return nil, err
		}
		script, err := bchutil.PayToAddrScript(da)
		if err != nil {
			return nil, err
		}
		return script, nil
	} else {
		da, err := btcutil.DecodeAddress(address, p.Params)
		if err != nil {
			return nil, err
		}
		script, err := txscript.PayToAddrScript(da)
		if err != nil {
			return nil, err
		}
		return script, nil
	}
}

func isCashAddr(addr string) bool {
	n := len(addr)
	switch {
	case n > len(MainNetPrefix) && addr[0:len(MainNetPrefix)] == MainNetPrefix:
		return true
	case n > len(TestNetPrefix) && addr[0:len(TestNetPrefix)] == TestNetPrefix:
		return true
	case n > len(RegTestPrefix) && addr[0:len(RegTestPrefix)] == RegTestPrefix:
		return true
	}

	return false
}

// outputScriptToAddresses converts ScriptPubKey to bitcoin addresses
func outputScriptToAddresses(script []byte, params *chaincfg.Params) ([]string, error) {
	a, err := bchutil.ExtractPkScriptAddrs(script, params)
	if err != nil {
		return nil, err
	}
	return []string{a.EncodeAddress()}, nil
}

type bcashAddress struct {
	addr string
}

func newBCashAddress(addr string, format AddressFormat) (*bcashAddress, error) {
	if isCashAddr(addr) && format == CashAddr {
		return &bcashAddress{addr: addr}, nil
	}

	da, err := address.NewFromString(addr)
	if err != nil {
		return nil, err
	}
	var ea string
	switch format {
	case CashAddr:
		if a, err := da.CashAddress(); err != nil {
			return nil, err
		} else {
			ea, err = a.Encode()
			if err != nil {
				return nil, err
			}
		}

	case Legacy:
		if a, err := da.Legacy(); err != nil {
			return nil, err
		} else {
			ea, err = a.Encode()
			if err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("Unknown address format: %d", format)
	}
	return &bcashAddress{addr: ea}, nil
}

func (a *bcashAddress) String() string {
	return a.addr
}

func (a *bcashAddress) AreEqual(addr string) bool {
	return a.String() == addr
}

func (a *bcashAddress) InSlice(addrs []string) bool {
	ea := a.String()
	for _, addr := range addrs {
		if ea == addr {
			return true
		}
	}
	return false
}
