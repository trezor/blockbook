package eth

import (
	"context"
	"math/big"
	"strings"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
)

const erc20TransferMethodSignature = "0xa9059cbb"                  // transfer(address,uint256)
const erc721TransferFromMethodSignature = "0x23b872dd"             // transferFrom(address,address,uint256)
const erc721SafeTransferFromMethodSignature = "0x42842e0e"         // safeTransferFrom(address,address,uint256)
const erc721SafeTransferFromWithDataMethodSignature = "0xb88d4fde" // safeTransferFrom(address,address,uint256,bytes)

const tokenTransferEventSignature = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
const tokenERC1155TransferSingleEventSignature = "0xc3d58168c5ae7397731d063d5bbf3d657854427343f4c083240f7aacaa2d0f62"
const tokenERC1155TransferBatchEventSignature = "0x4a39dc06d4c0dbc64b70af90fd698a233a518aa5d07e595d983b8c0526c8f7fb"

const nameRegisteredEventSignature = "0xca6abbe9d7f11422cb6ca7629fbf6fe9efb1c621f71ce8f02b9f2a230097404f"

const contractNameSignature = "0x06fdde03"
const contractSymbolSignature = "0x95d89b41"
const contractDecimalsSignature = "0x313ce567"
const contractBalanceOf = "0x70a08231"

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

func processTransferEvent(l *bchain.RpcLog) (*bchain.TokenTransfer, error) {
	tl := len(l.Topics)
	var ttt bchain.TokenType
	var value big.Int
	if tl == 3 {
		ttt = bchain.FungibleToken
		_, ok := value.SetString(l.Data, 0)
		if !ok {
			return nil, errors.New("ERC20 log Data is not a number")
		}
	} else if tl == 4 {
		ttt = bchain.NonFungibleToken
		_, ok := value.SetString(l.Topics[3], 0)
		if !ok {
			return nil, errors.New("ERC721 log Topics[3] is not a number")
		}
	} else {
		return nil, nil
	}
	from, err := addressFromPaddedHex(l.Topics[1])
	if err != nil {
		return nil, err
	}
	to, err := addressFromPaddedHex(l.Topics[2])
	if err != nil {
		return nil, err
	}
	return &bchain.TokenTransfer{
		Type:     ttt,
		Contract: EIP55AddressFromAddress(l.Address),
		From:     EIP55AddressFromAddress(from),
		To:       EIP55AddressFromAddress(to),
		Value:    value,
	}, nil
}

func processERC1155TransferSingleEvent(l *bchain.RpcLog) (*bchain.TokenTransfer, error) {
	from, err := addressFromPaddedHex(l.Topics[2])
	if err != nil {
		return nil, err
	}
	to, err := addressFromPaddedHex(l.Topics[3])
	if err != nil {
		return nil, err
	}
	var id, value big.Int
	data := l.Data
	if has0xPrefix(l.Data) {
		data = data[2:]
	}
	_, ok := id.SetString(data[:64], 16)
	if !ok {
		return nil, errors.New("ERC1155 log Data id is not a number")
	}
	_, ok = value.SetString(data[64:128], 16)
	if !ok {
		return nil, errors.New("ERC1155 log Data value is not a number")
	}
	return &bchain.TokenTransfer{
		Type:             bchain.MultiToken,
		Contract:         EIP55AddressFromAddress(l.Address),
		From:             EIP55AddressFromAddress(from),
		To:               EIP55AddressFromAddress(to),
		MultiTokenValues: []bchain.MultiTokenValue{{Id: id, Value: value}},
	}, nil
}

func processERC1155TransferBatchEvent(l *bchain.RpcLog) (*bchain.TokenTransfer, error) {
	from, err := addressFromPaddedHex(l.Topics[2])
	if err != nil {
		return nil, err
	}
	to, err := addressFromPaddedHex(l.Topics[3])
	if err != nil {
		return nil, err
	}
	data := l.Data
	if has0xPrefix(l.Data) {
		data = data[2:]
	}
	var b big.Int
	_, ok := b.SetString(data[:64], 16)
	if !ok || !b.IsInt64() {
		return nil, errors.New("ERC1155 TransferBatch, not a number")
	}
	offsetIds := int(b.Int64()) * 2
	_, ok = b.SetString(data[64:128], 16)
	if !ok || !b.IsInt64() {
		return nil, errors.New("ERC1155 TransferBatch, not a number")
	}
	offsetValues := int(b.Int64()) * 2
	_, ok = b.SetString(data[offsetIds:offsetIds+64], 16)
	if !ok || !b.IsInt64() {
		return nil, errors.New("ERC1155 TransferBatch, not a number")
	}
	countIds := int(b.Int64())
	_, ok = b.SetString(data[offsetValues:offsetValues+64], 16)
	if !ok || !b.IsInt64() {
		return nil, errors.New("ERC1155 TransferBatch, not a number")
	}
	countValues := int(b.Int64())
	if countIds != countValues {
		return nil, errors.New("ERC1155 TransferBatch, count values and ids does not match")
	}
	idValues := make([]bchain.MultiTokenValue, countValues)
	for i := 0; i < countValues; i++ {
		var id, value big.Int
		o := offsetIds + 64 + 64*i
		_, ok := id.SetString(data[o:o+64], 16)
		if !ok {
			return nil, errors.New("ERC1155 log Data id is not a number")
		}
		o = offsetValues + 64 + 64*i
		_, ok = value.SetString(data[o:o+64], 16)
		if !ok {
			return nil, errors.New("ERC1155 log Data value is not a number")
		}
		idValues[i] = bchain.MultiTokenValue{Id: id, Value: value}
	}
	return &bchain.TokenTransfer{
		Type:             bchain.MultiToken,
		Contract:         EIP55AddressFromAddress(l.Address),
		From:             EIP55AddressFromAddress(from),
		To:               EIP55AddressFromAddress(to),
		MultiTokenValues: idValues,
	}, nil
}
func contractGetTransfersFromLog(logs []*bchain.RpcLog) (bchain.TokenTransfers, error) {
	var r bchain.TokenTransfers
	var tt *bchain.TokenTransfer
	var err error
	for _, l := range logs {
		tl := len(l.Topics)
		if tl > 0 {
			signature := l.Topics[0]
			if signature == tokenTransferEventSignature {
				tt, err = processTransferEvent(l)
			} else if signature == tokenERC1155TransferSingleEventSignature && tl == 4 {
				tt, err = processERC1155TransferSingleEvent(l)
			} else if signature == tokenERC1155TransferBatchEventSignature {
				tt, err = processERC1155TransferBatchEvent(l)
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

func contractGetTransfersFromTx(tx *bchain.RpcTransaction) (bchain.TokenTransfers, error) {
	var r bchain.TokenTransfers
	if len(tx.Payload) == 10+128 && strings.HasPrefix(tx.Payload, erc20TransferMethodSignature) {
		to, err := addressFromPaddedHex(tx.Payload[10 : 10+64])
		if err != nil {
			return nil, err
		}
		var t big.Int
		_, ok := t.SetString(tx.Payload[10+64:], 16)
		if !ok {
			return nil, errors.New("Data is not a number")
		}
		r = append(r, &bchain.TokenTransfer{
			Type:     bchain.FungibleToken,
			Contract: EIP55AddressFromAddress(tx.To),
			From:     EIP55AddressFromAddress(tx.From),
			To:       EIP55AddressFromAddress(to),
			Value:    t,
		})
	} else if len(tx.Payload) >= 10+192 &&
		(strings.HasPrefix(tx.Payload, erc721TransferFromMethodSignature) ||
			strings.HasPrefix(tx.Payload, erc721SafeTransferFromMethodSignature) ||
			strings.HasPrefix(tx.Payload, erc721SafeTransferFromWithDataMethodSignature)) {
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
			Contract: EIP55AddressFromAddress(tx.To),
			From:     EIP55AddressFromAddress(from),
			To:       EIP55AddressFromAddress(to),
			Value:    t,
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

func (b *EthereumRPC) fetchContractInfo(address string) (*bchain.ContractInfo, error) {
	var contract bchain.ContractInfo
	data, err := b.ethCall(contractNameSignature, address)
	if err != nil {
		// ignore the error from the eth_call - since geth v1.9.15 they changed the behavior
		// and returning error "execution reverted" for some non contract addresses
		// https://github.com/ethereum/go-ethereum/issues/21249#issuecomment-648647672
		// glog.Warning(errors.Annotatef(err, "Contract NameSignature %v", address))
		return nil, nil
		// return nil, errors.Annotatef(err, "erc20NameSignature %v", address)
	}
	name := parseSimpleStringProperty(data)
	if name != "" {
		data, err = b.ethCall(contractSymbolSignature, address)
		if err != nil {
			// glog.Warning(errors.Annotatef(err, "Contract SymbolSignature %v", address))
			return nil, nil
			// return nil, errors.Annotatef(err, "erc20SymbolSignature %v", address)
		}
		symbol := parseSimpleStringProperty(data)
		data, _ = b.ethCall(contractDecimalsSignature, address)
		// if err != nil {
		// 	glog.Warning(errors.Annotatef(err, "Contract DecimalsSignature %v", address))
		// 	// return nil, errors.Annotatef(err, "erc20DecimalsSignature %v", address)
		// }
		contract = bchain.ContractInfo{
			Contract: address,
			Name:     name,
			Symbol:   symbol,
		}
		d := parseSimpleNumericProperty(data)
		if d != nil {
			contract.Decimals = int(uint8(d.Uint64()))
		} else {
			contract.Decimals = EtherAmountDecimalPoint
		}
	} else {
		return nil, nil
	}
	return &contract, nil
}

// GetContractInfo returns information about a contract
func (b *EthereumRPC) GetContractInfo(contractDesc bchain.AddressDescriptor) (*bchain.ContractInfo, error) {
	address := EIP55Address(contractDesc)
	return b.fetchContractInfo(address)
}

// EthereumTypeGetErc20ContractBalance returns balance of ERC20 contract for given address
func (b *EthereumRPC) EthereumTypeGetErc20ContractBalance(addrDesc, contractDesc bchain.AddressDescriptor) (*big.Int, error) {
	addr := EIP55Address(addrDesc)
	contract := EIP55Address(contractDesc)
	req := contractBalanceOf + "0000000000000000000000000000000000000000000000000000000000000000"[len(addr)-2:] + addr[2:]
	data, err := b.ethCall(req, contract)
	if err != nil {
		return nil, err
	}
	r := parseSimpleNumericProperty(data)
	if r == nil {
		return nil, errors.New("Invalid balance")
	}
	return r, nil
}
