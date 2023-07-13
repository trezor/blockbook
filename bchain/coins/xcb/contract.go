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
	"github.com/core-coin/go-core/v2/common/hexutil"
	"github.com/golang/glog"
	"github.com/juju/errors"

	"github.com/cryptohub-digital/blockbook-fork/bchain"
)

const tokenTransferEventSignature = "0xc17a9d92b89f27cb79cc390f23a1a5d302fefab8c7911075ede952ac2b5607a1"

// doing the parsing/processing without using go-core/accounts/abi library, it is simple to get data from Transfer event
const crc20TransferMethodSignature = "0x4b40e901"

const nameSignature = "0x07ba2a17"
const symbolSignature = "0x231782d8"
const decimalsSignature = "0x5d1fb5f9"
const balanceOfSignature = "0x1d7976f3"

const crc721TransferFromMethodSignature = "0x31f2e679"             // transferFrom(address,address,uint256)
const crc721SafeTransferFromMethodSignature = "0x3453ba4a"         // safeTransferFrom(address,address,uint256)
const crc721SafeTransferFromWithDataMethodSignature = "0xf3d63809" // safeTransferFrom(address,address,uint256,bytes)

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

func getTokenTransfersFromLog(logs []*RpcLog) (bchain.TokenTransfers, error) {
	var r bchain.TokenTransfers
	var tt *bchain.TokenTransfer
	var err error
	for _, l := range logs {
		tl := len(l.Topics)
		if tl > 0 {
			signature := l.Topics[0]
			if signature == tokenTransferEventSignature {
				tt, err = processtokenTransferEventFromLogs(l)
			} else {
				continue
			}
			if err != nil {
				return nil, err
			}
			if tt != nil {
				r = append(r, tt)
			}
		}
	}
	return r, nil
}

func processtokenTransferEventFromLogs(log *RpcLog) (*bchain.TokenTransfer, error) {
	tl := len(log.Topics)
	var ttt bchain.TokenType
	var value big.Int
	if tl == 3 {
		ttt = bchain.FungibleToken
		_, ok := value.SetString(log.Data, 0)
		if !ok {
			return nil, errors.New("CRC20 log Data is not a number")
		}
	} else if tl == 4 {
		ttt = bchain.NonFungibleToken
		_, ok := value.SetString(log.Topics[3], 0)
		if !ok {
			return nil, errors.New("CRC721 log Topics[3] is not a number")
		}
	} else {
		return nil, nil
	}

	from, err := addressFromPaddedHex(log.Topics[1])
	if err != nil {
		return nil, err
	}
	to, err := addressFromPaddedHex(log.Topics[2])
	if err != nil {
		return nil, err
	}
	return &bchain.TokenTransfer{
		Type:     ttt,
		Contract: log.Address,
		From:     from,
		To:       to,
		Value:    value,
	}, nil
}

func getTokenTransfersFromTx(tx *RpcTransaction) (bchain.TokenTransfers, error) {
	var r bchain.TokenTransfers
	if len(tx.Payload)%(128+len(crc20TransferMethodSignature)) == 0 && strings.HasPrefix(tx.Payload, crc20TransferMethodSignature) {
		to, err := addressFromPaddedHex(tx.Payload[len(crc20TransferMethodSignature) : 64+len(crc20TransferMethodSignature)])
		if err != nil {
			return nil, err
		}
		var t big.Int
		_, ok := t.SetString(tx.Payload[len(crc20TransferMethodSignature)+64:], 16)
		if !ok {
			return nil, errors.New("Data is not a number")
		}
		r = append(r, &bchain.TokenTransfer{
			Contract: tx.To,
			From:     tx.From,
			To:       to,
			Value:    t,
			Type:     bchain.FungibleToken,
		})
	} else if len(tx.Payload) >= 10+192 &&
		(strings.HasPrefix(tx.Payload, crc721TransferFromMethodSignature) ||
			strings.HasPrefix(tx.Payload, crc721SafeTransferFromMethodSignature) ||
			strings.HasPrefix(tx.Payload, crc721SafeTransferFromWithDataMethodSignature)) {
		from, err := addressFromPaddedHex(tx.Payload[10 : 10+64])
		if err != nil {
			return nil, err
		}
		to, err := addressFromPaddedHex(tx.Payload[10+64 : 10+128])
		if err != nil {
			return nil, err
		}
		var t big.Int
		_, ok := t.SetString(tx.Payload[10+128:10+192], 16)
		if !ok {
			return nil, errors.New("Data is not a number")
		}
		r = append(r, &bchain.TokenTransfer{
			Type:     bchain.NonFungibleToken,
			Contract: tx.To,
			From:     from,
			To:       to,
			Value:    t,
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

func parseCRC20NumericProperty(contractDesc bchain.AddressDescriptor, data string) *big.Int {
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

func parseCRC20StringProperty(contractDesc bchain.AddressDescriptor, data string) string {
	if has0xPrefix(data) {
		data = data[2:]
	}
	if len(data) > 128 {
		n := parseCRC20NumericProperty(contractDesc, data[64:128])
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

// GetContractInfo returns information about smart contract
func (b *CoreblockchainRPC) GetContractInfo(contractDesc bchain.AddressDescriptor) (*bchain.ContractInfo, error) {
	cds, err := b.Parser.GetAddrDescFromAddress(common.Bytes2Hex(contractDesc[:]))
	if err != nil {
		return nil, err
	}
	cachedContractsMux.Lock()
	contract, found := cachedContracts[common.Bytes2Hex(cds)]
	cachedContractsMux.Unlock()

	if !found {
		address, err := common.HexToAddress(common.Bytes2Hex(cds))
		if err != nil {
			return nil, err
		}
		data, err := b.xcbCall(nameSignature, address.Hex())
		if err != nil {
			if strings.Contains(err.Error(), "execution reverted") {
				// if execution reverted -> it is not crc20 smart contract
				return &bchain.ContractInfo{
					Contract: address.Hex(),
					Type:     CRC721TokenType,
				}, nil
			}
			return nil, nil
		}
		name := parseCRC20StringProperty(contractDesc, data)
		if name != "" {
			data, err = b.xcbCall(symbolSignature, address.Hex())
			if err != nil {
				glog.Warning(errors.Annotatef(err, "crc20SymbolSignature %v", address))
				return nil, nil
				// return nil, errors.Annotatef(err, "crc20SymbolSignature %v", address)
			}
			symbol := parseCRC20StringProperty(contractDesc, data)
			data, err = b.xcbCall(decimalsSignature, address.Hex())
			if err != nil {
				glog.Warning(errors.Annotatef(err, "crc20DecimalsSignature %v", address))
				// return nil, errors.Annotatef(err, "crc20DecimalsSignature %v", address)
			}
			contract = &bchain.ContractInfo{
				Contract: address.Hex(),
				Name:     name,
				Symbol:   symbol,
				Type:     CRC20TokenType,
			}
			d := parseCRC20NumericProperty(contractDesc, data)
			if d != nil {
				contract.Decimals = int(uint8(d.Uint64()))
			} else {
				contract.Decimals = CoreAmountDecimalPoint
			}
		} else {
			contract = nil
		}
		cachedContractsMux.Lock()
		cachedContracts[common.Bytes2Hex(cds)] = contract
		cachedContractsMux.Unlock()
	}
	return contract, nil
}

// CoreCoinTypeGetCrc20ContractBalance returns balance of crc20 contract for given address
func (b *CoreblockchainRPC) CoreCoinTypeGetCrc20ContractBalance(addrDesc, contractDesc bchain.AddressDescriptor) (*big.Int, error) {
	addr := cutAddress(addrDesc)
	contract := "0x" + cutAddress(contractDesc)

	req := balanceOfSignature + "0000000000000000000000000000000000000000000000000000000000000000"[len(addr):] + addr
	data, err := b.xcbCall(req, contract)
	if err != nil {
		return nil, err
	}
	r := parseCRC20NumericProperty(contractDesc, data)
	if r == nil {
		return nil, errors.New("Invalid balance")
	}
	return r, nil
}

func cutAddress(addrDesc bchain.AddressDescriptor) string {
	raw := hexutil.Encode(addrDesc)

	if len(raw) > 2 {
		raw = raw[2:]
	}

	return raw
}
