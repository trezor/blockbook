package xcb

import (
	"bytes"
	"context"
	"encoding/hex"
	"math/big"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/core-coin/go-core/v2/common"
	"github.com/golang/glog"
	"github.com/juju/errors"

	"github.com/cryptohub-digital/blockbook-fork/bchain"
)

var xrc20abi = `[{"constant":true,"inputs":[],"name":"name","outputs":[{"name":"","type":"string"}],"payable":false,"type":"function","signature":"0x06fdde03"},
{"constant":true,"inputs":[],"name":"symbol","outputs":[{"name":"","type":"string"}],"payable":false,"type":"function","signature":"0x95d89b41"},
{"constant":true,"inputs":[],"name":"decimals","outputs":[{"name":"","type":"uint8"}],"payable":false,"type":"function","signature":"0x313ce567"},
{"constant":true,"inputs":[],"name":"totalSupply","outputs":[{"name":"","type":"uint256"}],"payable":false,"type":"function","signature":"0x18160ddd"},
{"constant":true,"inputs":[{"name":"_owner","type":"address"}],"name":"balanceOf","outputs":[{"name":"balance","type":"uint256"}],"payable":false,"type":"function","signature":"0x70a08231"},
{"constant":false,"inputs":[{"name":"_to","type":"address"},{"name":"_value","type":"uint256"}],"name":"transfer","outputs":[{"name":"success","type":"bool"}],"payable":false,"type":"function","signature":"0xa9059cbb"},
{"constant":false,"inputs":[{"name":"_from","type":"address"},{"name":"_to","type":"address"},{"name":"_value","type":"uint256"}],"name":"transferFrom","outputs":[{"name":"success","type":"bool"}],"payable":false,"type":"function","signature":"0x23b872dd"},
{"constant":false,"inputs":[{"name":"_spender","type":"address"},{"name":"_value","type":"uint256"}],"name":"approve","outputs":[{"name":"success","type":"bool"}],"payable":false,"type":"function","signature":"0x095ea7b3"},
{"constant":true,"inputs":[{"name":"_owner","type":"address"},{"name":"_spender","type":"address"}],"name":"allowance","outputs":[{"name":"remaining","type":"uint256"}],"payable":false,"type":"function","signature":"0xdd62ed3e"},
{"anonymous":false,"inputs":[{"indexed":true,"name":"_from","type":"address"},{"indexed":true,"name":"_to","type":"address"},{"indexed":false,"name":"_value","type":"uint256"}],"name":"Transfer","type":"event","signature":"0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"},
{"anonymous":false,"inputs":[{"indexed":true,"name":"_owner","type":"address"},{"indexed":true,"name":"_spender","type":"address"},{"indexed":false,"name":"_value","type":"uint256"}],"name":"Approval","type":"event","signature":"0x8c5be1e5ebec7d5bd14f71427d1e84f3dd0314c0f7b2291e5b200ac8c7c3b925"},
{"inputs":[{"name":"_initialAmount","type":"uint256"},{"name":"_tokenName","type":"string"},{"name":"_decimalUnits","type":"uint8"},{"name":"_tokenSymbol","type":"string"}],"payable":false,"type":"constructor"},
{"constant":false,"inputs":[{"name":"_spender","type":"address"},{"name":"_value","type":"uint256"},{"name":"_extraData","type":"bytes"}],"name":"approveAndCall","outputs":[{"name":"success","type":"bool"}],"payable":false,"type":"function","signature":"0xcae9ca51"},
{"constant":true,"inputs":[],"name":"version","outputs":[{"name":"","type":"string"}],"payable":false,"type":"function","signature":"0x54fd4d50"}]`


// doing the parsing/processing without using go-core/accounts/abi library, it is simple to get data from Transfer event
const xrc20TransferMethodSignature = "0x4b40e901"
const xrc20TransferEventSignature = "0xc17a9d92b89f27cb79cc390f23a1a5d302fefab8c7911075ede952ac2b5607a1"
const xrc20NameSignature = "0x07ba2a17"
const xrc20SymbolSignature = "0x231782d8"
const xrc20DecimalsSignature = "0x5d1fb5f9"
const xrc20BalanceOf = "0x1d7976f3"

var cachedContracts = make(map[string]*bchain.ContractInfo)
var cachedContractsMux sync.Mutex

func addressFromPaddedHex(s string) (string, error) {
	var t big.Int
	var ok bool
	if has0xPrefix(s) {
		_, ok = t.SetString(s[2:], 16)
	} else {
		_, ok = t.SetString(s, 16)
	}
	if !ok {
		return "", errors.New("Data is not a number")
	}
	a := common.BigToAddress(&t)
	return a.String(), nil
}

func xrc20GetTransfersFromLog(logs []*RpcLog) (bchain.TokenTransfers, error) {
	var r bchain.TokenTransfers
	for _, l := range logs {
		if len(l.Topics) == 3 && l.Topics[0] == xrc20TransferEventSignature {
			var t big.Int
			_, ok := t.SetString(l.Data, 0)
			if !ok {
				return nil, errors.New("Data is not a number")
			}
			from, err := addressFromPaddedHex(l.Topics[1])
			if err != nil {
				return nil, err
			}
			to, err := addressFromPaddedHex(l.Topics[2])
			if err != nil {
				return nil, err
			}
			r = append(r, &bchain.TokenTransfer{
				Contract: l.Address,
				From:     from,
				To:       to,
				Value:   t,
				Type: bchain.FungibleToken,
			})
		}
	}
	return r, nil
}

func xrc20GetTransfersFromTx(tx *RpcTransaction) (bchain.TokenTransfers, error) {
	var r bchain.TokenTransfers
	if len(tx.Payload) % (128+len(xrc20TransferMethodSignature)) == 0 && strings.HasPrefix(tx.Payload, xrc20TransferMethodSignature) {
		to, err := addressFromPaddedHex(tx.Payload[len(xrc20TransferMethodSignature) : 64+len(xrc20TransferMethodSignature)])
		if err != nil {
			return nil, err
		}
		var t big.Int
		_, ok := t.SetString(tx.Payload[len(xrc20TransferMethodSignature)+64:], 16)
		if !ok {
			return nil, errors.New("Data is not a number")
		}
		r = append(r, &bchain.TokenTransfer{
			Contract: tx.To,
			From:     tx.From,
			To:       to,
			Value:   t,
			Type: bchain.FungibleToken,
		})
	}
	return r, nil
}

func (b *CoreblockchainRPC) xcbCall(data, to string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var r string
	err := b.RPC.CallContext(ctx, &r, "xcb_call", map[string]interface{}{
		"data": data,
		"to":   to,
	}, "latest")
	if err != nil {
		return "", err
	}
	return r, nil
}

func parsexrc20NumericProperty(contractDesc bchain.AddressDescriptor, data string) *big.Int {
	if has0xPrefix(data) {
		data = data[2:]
	}
	if len(data) > 64 {
		data = data[:64]
	}
	if len(data) == 64 {
		var n big.Int
		_, ok := n.SetString(data, 16)
		if ok {
			return &n
		}
	}
	if glog.V(1) {
		glog.Warning("Cannot parse '", data, "' for contract ", contractDesc)
	}
	return nil
}

func parsexrc20StringProperty(contractDesc bchain.AddressDescriptor, data string) string {
	if has0xPrefix(data) {
		data = data[2:]
	}
	if len(data) > 128 {
		n := parsexrc20NumericProperty(contractDesc, data[64:128])
		if n != nil {
			l := n.Uint64()
			if l > 0 && 2*int(l) <= len(data)-128 {
				b, err := hex.DecodeString(data[128 : 128+2*l])
				if err == nil {
					return string(b)
				}
			}
		}
	}
	// allow string properties as UTF-8 data
	b, err := hex.DecodeString(data)
	if err == nil {
		i := bytes.Index(b, []byte{0})
		if i > 32 {
			i = 32
		}
		if i > 0 {
			b = b[:i]
		}
		if utf8.Valid(b) {
			return string(b)
		}
	}
	if glog.V(1) {
		glog.Warning("Cannot parse '", data, "' for contract ", contractDesc)
	}
	return ""
}

// CoreCoinTypeGetXrc20ContractInfo returns information about xrc20 contract
func (b *CoreblockchainRPC) CoreCoinTypeGetXrc20ContractInfo(contractDesc bchain.AddressDescriptor) (*bchain.ContractInfo, error) {
	cds := string(contractDesc)
	cachedContractsMux.Lock()
	contract, found := cachedContracts[cds]
	cachedContractsMux.Unlock()
	if !found {
		address, err := common.HexToAddress(cds)
		if err != nil {
			return nil, err
		}
		data, err := b.xcbCall(xrc20NameSignature, address.Hex())
		if err != nil {
			// ignore the error from the xcb_call - since geth v1.9.15 they changed the behavior
			// and returning error "execution reverted" for some non contract addresses
			// https://github.com/core-coin/go-core/v2/issues/21249#issuecomment-648647672
			glog.Warning(errors.Annotatef(err, "xrc20NameSignature %v", address))
			return nil, nil
			// return nil, errors.Annotatef(err, "xrc20NameSignature %v", address)
		}
		name := parsexrc20StringProperty(contractDesc, data)
		if name != "" {
			data, err = b.xcbCall(xrc20SymbolSignature, address.Hex())
			if err != nil {
				glog.Warning(errors.Annotatef(err, "xrc20SymbolSignature %v", address))
				return nil, nil
				// return nil, errors.Annotatef(err, "xrc20SymbolSignature %v", address)
			}
			symbol := parsexrc20StringProperty(contractDesc, data)
			data, err = b.xcbCall(xrc20DecimalsSignature, address.Hex())
			if err != nil {
				glog.Warning(errors.Annotatef(err, "xrc20DecimalsSignature %v", address))
				// return nil, errors.Annotatef(err, "xrc20DecimalsSignature %v", address)
			}
			contract = &bchain.ContractInfo{
				Contract: address.Hex(),
				Name:     name,
				Symbol:   symbol,
				Type: bchain.XRC20TokenType,
			}
			d := parsexrc20NumericProperty(contractDesc, data)
			if d != nil {
				contract.Decimals = int(uint8(d.Uint64()))
			} else {
				contract.Decimals = CoreAmountDecimalPoint
			}
		} else {
			contract = nil
		}
		cachedContractsMux.Lock()
		cachedContracts[cds] = contract
		cachedContractsMux.Unlock()
	}
	return contract, nil
}

// CoreCoinTypeGetXrc20ContractBalance returns balance of xrc20 contract for given address
func (b *CoreblockchainRPC) CoreCoinTypeGetXrc20ContractBalance(addrDesc, contractDesc bchain.AddressDescriptor) (*big.Int, error) {
	addr, err := common.HexToAddress(string(addrDesc))
	if err != nil {
		return nil, err
	}
	contract, err := common.HexToAddress("0x" + string(contractDesc))
	if err != nil {
		return nil, err
	}
	req := xrc20BalanceOf + "0000000000000000000000000000000000000000000000000000000000000000"[len(addr):] + addr.Hex()
	data, err := b.xcbCall(req, contract.String())
	if err != nil {
		return nil, err
	}
	r := parsexrc20NumericProperty(contractDesc, data)
	if r == nil {
		return nil, errors.New("Invalid balance")
	}
	return r, nil
}
