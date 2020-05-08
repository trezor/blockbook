package eth

import (
	"bytes"
	"context"
	"encoding/hex"
	"math/big"
	"strings"
	"sync"
	"unicode/utf8"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
)

var erc20abi = `[{"constant":true,"inputs":[],"name":"name","outputs":[{"name":"","type":"string"}],"payable":false,"type":"function","signature":"0x06fdde03"},
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

// doing the parsing/processing without using go-ethereum/accounts/abi library, it is simple to get data from Transfer event
const erc20TransferMethodSignature = "0xa9059cbb"
const erc20TransferEventSignature = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
const erc20NameSignature = "0x06fdde03"
const erc20SymbolSignature = "0x95d89b41"
const erc20DecimalsSignature = "0x313ce567"
const erc20BalanceOf = "0x70a08231"

var cachedContracts = make(map[string]*bchain.Erc20Contract)
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
	a := ethcommon.BigToAddress(&t)
	return a.String(), nil
}

func erc20GetTransfersFromLog(logs []*rpcLog) ([]bchain.Erc20Transfer, error) {
	var r []bchain.Erc20Transfer
	for _, l := range logs {
		if len(l.Topics) == 3 && l.Topics[0] == erc20TransferEventSignature {
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
			r = append(r, bchain.Erc20Transfer{
				Contract: EIP55AddressFromAddress(l.Address),
				From:     EIP55AddressFromAddress(from),
				To:       EIP55AddressFromAddress(to),
				Tokens:   t,
			})
		}
	}
	return r, nil
}

func erc20GetTransfersFromTx(tx *rpcTransaction) ([]bchain.Erc20Transfer, error) {
	var r []bchain.Erc20Transfer
	if len(tx.Payload) == 128+len(erc20TransferMethodSignature) && strings.HasPrefix(tx.Payload, erc20TransferMethodSignature) {
		to, err := addressFromPaddedHex(tx.Payload[len(erc20TransferMethodSignature) : 64+len(erc20TransferMethodSignature)])
		if err != nil {
			return nil, err
		}
		var t big.Int
		_, ok := t.SetString(tx.Payload[len(erc20TransferMethodSignature)+64:], 16)
		if !ok {
			return nil, errors.New("Data is not a number")
		}
		r = append(r, bchain.Erc20Transfer{
			Contract: EIP55AddressFromAddress(tx.To),
			From:     EIP55AddressFromAddress(tx.From),
			To:       EIP55AddressFromAddress(to),
			Tokens:   t,
		})
	}
	return r, nil
}

func (b *EthereumRPC) ethCall(data, to string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	var r string
	err := b.rpc.CallContext(ctx, &r, "eth_call", map[string]interface{}{
		"data": data,
		"to":   to,
	}, "latest")
	if err != nil {
		return "", err
	}
	return r, nil
}

func parseErc20NumericProperty(contractDesc bchain.AddressDescriptor, data string) *big.Int {
	if has0xPrefix(data) {
		data = data[2:]
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

func parseErc20StringProperty(contractDesc bchain.AddressDescriptor, data string) string {
	if has0xPrefix(data) {
		data = data[2:]
	}
	if len(data) > 128 {
		n := parseErc20NumericProperty(contractDesc, data[64:128])
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

// EthereumTypeGetErc20ContractInfo returns information about ERC20 contract
func (b *EthereumRPC) EthereumTypeGetErc20ContractInfo(contractDesc bchain.AddressDescriptor) (*bchain.Erc20Contract, error) {
	cds := string(contractDesc)
	cachedContractsMux.Lock()
	contract, found := cachedContracts[cds]
	cachedContractsMux.Unlock()
	if !found {
		address := EIP55Address(contractDesc)
		data, err := b.ethCall(erc20NameSignature, address)
		if err != nil {
			return nil, err
		}
		name := parseErc20StringProperty(contractDesc, data)
		if name != "" {
			data, err = b.ethCall(erc20SymbolSignature, address)
			if err != nil {
				return nil, err
			}
			symbol := parseErc20StringProperty(contractDesc, data)
			data, err = b.ethCall(erc20DecimalsSignature, address)
			if err != nil {
				return nil, err
			}
			contract = &bchain.Erc20Contract{
				Contract: address,
				Name:     name,
				Symbol:   symbol,
			}
			d := parseErc20NumericProperty(contractDesc, data)
			if d != nil {
				contract.Decimals = int(uint8(d.Uint64()))
			} else {
				contract.Decimals = EtherAmountDecimalPoint
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

// EthereumTypeGetErc20ContractBalance returns balance of ERC20 contract for given address
func (b *EthereumRPC) EthereumTypeGetErc20ContractBalance(addrDesc, contractDesc bchain.AddressDescriptor) (*big.Int, error) {
	addr := EIP55Address(addrDesc)
	contract := EIP55Address(contractDesc)
	req := erc20BalanceOf + "0000000000000000000000000000000000000000000000000000000000000000"[len(addr)-2:] + addr[2:]
	data, err := b.ethCall(req, contract)
	if err != nil {
		return nil, err
	}
	r := parseErc20NumericProperty(contractDesc, data)
	if r == nil {
		return nil, errors.New("Invalid balance")
	}
	return r, nil
}
