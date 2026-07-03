package bchutil

import (
	"errors"
	"github.com/martinboehm/btcutil"
	"github.com/martinboehm/btcutil/txscript"
)

func PayToAddrScript(addr btcutil.Address) ([]byte, error) {
	var script []byte
	var err error
	script, err = txscript.PayToAddrScript(addr)
	if err == nil {
		return script, nil
	}
	script, err = cashPayToAddrScript(addr)
	if err == nil {
		return script, nil
	}
	script, err = bitpayPayToAddrScript(addr)
	if err == nil {
		return script, nil
	}
	return script, errors.New("Unrecognized address format")
}
