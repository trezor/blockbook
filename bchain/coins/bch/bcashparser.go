package bch

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"fmt"
	"strings"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcutil"
	"github.com/cpacia/bchutil"
)

type AddressFormat = uint8

const (
	Legacy AddressFormat = iota
	CashAddr
)

var prefixes = []string{"bitcoincash", "bchtest", "bchreg"}

// BCashParser handle
type BCashParser struct {
	*btc.BitcoinParser
	AddressFormat AddressFormat
}

// NewBCashParser returns new BCashParser instance
func NewBCashParser(params *chaincfg.Params, c *btc.Configuration) *BCashParser {
	var format AddressFormat
	switch c.AddressFormat {
	case "":
		fallthrough
	case "cashaddr":
		format = CashAddr
	case "legacy":
		format = Legacy
	default:
		// XXX
		e := fmt.Errorf("Unknown address format: %s", c.AddressFormat)
		panic(e)
	}
	return &BCashParser{
		BitcoinParser: &btc.BitcoinParser{
			BaseParser: &bchain.BaseParser{
				AddressFactory:       func(addr string) (bchain.Address, error) { return newBCashAddress(addr, params, format) },
				BlockAddressesToKeep: c.BlockAddressesToKeep,
			},
			Params: params,
		},
		AddressFormat: format,
	}
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
	slice := strings.Split(addr, ":")
	if len(slice) != 2 {
		return false
	}
	for _, prefix := range prefixes {
		if slice[0] == prefix {
			return true
		}
	}
	return false
}

func (p *BCashParser) UnpackTx(buf []byte) (tx *bchain.Tx, height uint32, err error) {
	tx, height, err = p.BitcoinParser.UnpackTx(buf)
	if err != nil {
		return
	}

	for i, vout := range tx.Vout {
		if len(vout.ScriptPubKey.Addresses) == 1 {
			a, err := newBCashAddress(vout.ScriptPubKey.Addresses[0], p.Params, p.AddressFormat)
			if err != nil {
				return nil, 0, err
			}
			tx.Vout[i].Address = a
		}
	}

	return
}

type bcashAddress struct {
	addr   btcutil.Address
	net    *chaincfg.Params
	format AddressFormat
}

func newBCashAddress(addr string, net *chaincfg.Params, format AddressFormat) (*bcashAddress, error) {
	var (
		da  btcutil.Address
		err error
	)
	if isCashAddr(addr) {
		// for cashaddr we need to convert it to the legacy form (i.e. to btcutil's Address)
		// because bchutil doesn't allow later conversions
		da, err = bchutil.DecodeAddress(addr, net)
		if err != nil {
			return nil, err
		}
		switch ca := da.(type) {
		case *bchutil.CashAddressPubKeyHash:
			da, err = btcutil.NewAddressPubKeyHash(ca.Hash160()[:], net)
		case *bchutil.CashAddressScriptHash:
			da, err = btcutil.NewAddressScriptHash(ca.Hash160()[:], net)
		default:
			err = fmt.Errorf("Unknown address type: %T", da)
		}
		if err != nil {
			return nil, err
		}
	} else {
		da, err = btcutil.DecodeAddress(addr, net)
		if err != nil {
			return nil, err
		}
	}
	switch format {
	case Legacy, CashAddr:
	default:
		return nil, fmt.Errorf("Unknown address format: %d", format)
	}
	return &bcashAddress{addr: da, net: net, format: format}, nil
}

func (a *bcashAddress) String() string {
	return a.addr.String()
}

func (a *bcashAddress) EncodeAddress() (string, error) {
	switch a.format {
	case Legacy:
		return a.String(), nil
	case CashAddr:
		var (
			ca  btcutil.Address
			err error
		)
		switch da := a.addr.(type) {
		case *btcutil.AddressPubKeyHash:
			ca, err = bchutil.NewCashAddressPubKeyHash(da.Hash160()[:], a.net)
		case *btcutil.AddressScriptHash:
			ca, err = bchutil.NewCashAddressScriptHash(da.Hash160()[:], a.net)
		default:
			err = fmt.Errorf("Unknown address type: %T", da)
		}
		if err != nil {
			return "", err
		}
		return ca.String(), nil

	default:
		return "", fmt.Errorf("Unknown address format: %d", a.format)
	}
}

func (a *bcashAddress) AreEqual(addr string) (bool, error) {
	ea, err := a.EncodeAddress()
	if err != nil {
		return false, err
	}
	return ea == addr, nil
}

func (a *bcashAddress) InSlice(addrs []string) (bool, error) {
	ea, err := a.EncodeAddress()
	if err != nil {
		return false, err
	}
	for _, addr := range addrs {
		if ea == addr {
			return true, nil
		}
	}
	return false, nil
}
