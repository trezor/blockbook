package ecash

import (
	"fmt"

	"github.com/pirk/ecashutil"
	"github.com/martinboehm/btcutil"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/martinboehm/btcutil/txscript"
	"github.com/pirk/ecashaddr-converter/address"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// AddressFormat type is used to specify different formats of address
type AddressFormat = uint8

const (
	// Legacy AddressFormat is the same as Bitcoin
	Legacy AddressFormat = iota
	// CashAddr AddressFormat is new eCash standard
	CashAddr
)

const (
	// MainNetPrefix is CashAddr prefix for mainnet
	MainNetPrefix = "ecash:"
	// TestNetPrefix is CashAddr prefix for testnet
	TestNetPrefix = "ectest:"
	// RegTestPrefix is CashAddr prefix for regtest
	RegTestPrefix = "ecreg:"
)

var (
	// MainNetParams are parser parameters for mainnet
	MainNetParams chaincfg.Params
	// TestNetParams are parser parameters for testnet
	TestNetParams chaincfg.Params
	// RegtestParams are parser parameters for regtest
	RegtestParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = ecashutil.MainnetMagic

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = ecashutil.TestnetMagic

	RegtestParams = chaincfg.RegressionNetParams
	RegtestParams.Net = ecashutil.Regtestmagic
}

// ECashParser handle
type ECashParser struct {
	*btc.BitcoinLikeParser
	AddressFormat AddressFormat
}

// NewECashParser returns new ECashParser instance
func NewECashParser(params *chaincfg.Params, c *btc.Configuration) (*ECashParser, error) {
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
	p := &ECashParser{
		BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c),
		AddressFormat:     format,
	}
	p.OutputScriptToAddressesFunc = p.outputScriptToAddresses
	p.AmountDecimalPoint = 2
	return p, nil
}

// GetChainParams contains network parameters for the main eCash network,
// the regression test eCash network, the test eCash network and
// the simulation test eCash network, in this order
func GetChainParams(chain string) *chaincfg.Params {
	if !chaincfg.IsRegistered(&MainNetParams) {
		err := chaincfg.Register(&MainNetParams)
		if err == nil {
			err = chaincfg.Register(&TestNetParams)
		}
		if err == nil {
			err = chaincfg.Register(&RegtestParams)
		}
		if err != nil {
			panic(err)
		}
	}
	switch chain {
	case "test":
		return &TestNetParams
	case "regtest":
		return &RegtestParams
	default:
		return &MainNetParams
	}
}

// GetAddrDescFromAddress returns internal address representation of given address
func (p *ECashParser) GetAddrDescFromAddress(address string) (bchain.AddressDescriptor, error) {
	return p.addressToOutputScript(address)
}

// addressToOutputScript converts address to ScriptPubKey
func (p *ECashParser) addressToOutputScript(address string) ([]byte, error) {
	if isCashAddr(address) {
		da, err := ecashutil.DecodeAddress(address, p.Params)
		if err != nil {
			return nil, err
		}
		script, err := ecashutil.PayToAddrScript(da)
		if err != nil {
			return nil, err
		}
		return script, nil
	}
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
func (p *ECashParser) outputScriptToAddresses(script []byte) ([]string, bool, error) {
	// convert possible P2PK script to P2PK, which ecashutil can process
	var err error
	script, err = txscript.ConvertP2PKtoP2PKH(p.Params.Base58CksumHasher, script)
	if err != nil {
		return nil, false, err
	}
	a, err := ecashutil.ExtractPkScriptAddrs(script, p.Params)
	if err != nil {
		// do not return unknown script type error as error
		if err.Error() == "unknown script type" {
			// try OP_RETURN script
			or := p.TryParseOPReturn(script)
			if or != "" {
				return []string{or}, false, nil
			}
			return []string{}, false, nil
		}
		return nil, false, err
	}
	// EncodeAddress returns CashAddr address
	addr := a.EncodeAddress()
	if p.AddressFormat == Legacy {
		da, err := address.NewFromString(addr)
		if err != nil {
			return nil, false, err
		}
		ca, err := da.Legacy()
		if err != nil {
			return nil, false, err
		}
		addr, err = ca.Encode()
		if err != nil {
			return nil, false, err
		}
	}
	return []string{addr}, len(addr) > 0, nil
}
