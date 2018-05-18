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

var prefixes = []string{"bitcoincash", "bchtest", "bchreg"}

// BCashParser handle
type BCashParser struct {
	*btc.BitcoinParser
}

// NewBCashParser returns new BCashParser instance
func NewBCashParser(params *chaincfg.Params) *BCashParser {
	return &BCashParser{
		&btc.BitcoinParser{
			&bchain.BaseParser{
				AddressFactory: func(addr string) bchain.Address {
					return &bcashAddress{addr: addr, net: params}
				},
			},
			params,
		},
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
			tx.Vout[i].Address = &bcashAddress{
				addr: vout.ScriptPubKey.Addresses[0],
				net:  p.Params,
			}
		}
	}

	return
}

type bcashAddress struct {
	addr string
	net  *chaincfg.Params
}

func (a *bcashAddress) String() string {
	return a.addr
}

func (a *bcashAddress) EncodeAddress(format bchain.AddressFormat) (string, error) {
	switch format {
	case bchain.DefaultAddress:
		return a.String(), nil
	case bchain.BCashAddress:
		da, err := btcutil.DecodeAddress(a.addr, a.net)
		if err != nil {
			return "", err
		}
		var ca btcutil.Address
		switch da := da.(type) {
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
		return "", fmt.Errorf("Unknown address format: %d", format)
	}
}

func (a *bcashAddress) AreEqual(addr string) (bool, error) {
	var format bchain.AddressFormat
	if isCashAddr(addr) {
		format = bchain.BCashAddress
	} else {
		format = bchain.DefaultAddress
	}
	ea, err := a.EncodeAddress(format)
	if err != nil {
		return false, err
	}
	return ea == addr, nil
}

func (a *bcashAddress) InSlice(addrs []string) (bool, error) {
	for _, addr := range addrs {
		eq, err := a.AreEqual(addr)
		if err != nil {
			return false, err
		}
		if eq {
			return true, nil
		}
	}
	return false, nil
}
