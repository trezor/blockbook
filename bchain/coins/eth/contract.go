package eth

import (
	"context"
	"math/big"
	"strings"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
)

const erc20TransferMethodSignature = "0xa9059cbb"                  // transfer(address,uint256)
const erc721TransferFromMethodSignature = "0x23b872dd"             // transferFrom(address,address,uint256)
const erc721SafeTransferFromMethodSignature = "0x42842e0e"         // safeTransferFrom(address,address,uint256)
const erc721SafeTransferFromWithDataMethodSignature = "0xb88d4fde" // safeTransferFrom(address,address,uint256,bytes)
const erc721TokenURIMethodSignature = "0xc87b56dd"                 // tokenURI(uint256)
const erc1155URIMethodSignature = "0x0e89341c"                     // uri(uint256)

const tokenTransferEventSignature = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"
const tokenERC1155TransferSingleEventSignature = "0xc3d58168c5ae7397731d063d5bbf3d657854427343f4c083240f7aacaa2d0f62"
const tokenERC1155TransferBatchEventSignature = "0x4a39dc06d4c0dbc64b70af90fd698a233a518aa5d07e595d983b8c0526c8f7fb"

const contractNameSignature = "0x06fdde03"
const contractSymbolSignature = "0x95d89b41"
const contractDecimalsSignature = "0x313ce567"
const contractBalanceOfSignature = "0x70a08231"

const (
	evmWordBytes = 32
	evmWordHex   = evmWordBytes * 2
)

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

func processTransferEvent(l *bchain.RpcLog) (transfer *bchain.TokenTransfer, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("processTransferEvent recovered from panic %v", r)
		}
	}()
	tl := len(l.Topics)
	var standard bchain.TokenStandard
	var value big.Int
	if tl == 3 {
		standard = bchain.FungibleToken
		_, ok := value.SetString(l.Data, 0)
		if !ok {
			return nil, errors.New("ERC20 log Data is not a number")
		}
	} else if tl == 4 {
		standard = bchain.NonFungibleToken
		_, ok := value.SetString(l.Topics[3], 0)
		if !ok {
			return nil, errors.New("ERC721 log Topics[3] is not a number")
		}
	} else {
		return nil, nil
	}
	var from, to string
	from, err = addressFromPaddedHex(l.Topics[1])
	if err != nil {
		return nil, err
	}
	to, err = addressFromPaddedHex(l.Topics[2])
	if err != nil {
		return nil, err
	}
	return &bchain.TokenTransfer{
		Standard: standard,
		Contract: EIP55AddressFromAddress(l.Address),
		From:     EIP55AddressFromAddress(from),
		To:       EIP55AddressFromAddress(to),
		Value:    value,
	}, nil
}

func processERC1155TransferSingleEvent(l *bchain.RpcLog) (transfer *bchain.TokenTransfer, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("processERC1155TransferSingleEvent recovered from panic %v", r)
		}
	}()
	tl := len(l.Topics)
	if tl != 4 {
		return nil, nil
	}
	var from, to string
	from, err = addressFromPaddedHex(l.Topics[2])
	if err != nil {
		return nil, err
	}
	to, err = addressFromPaddedHex(l.Topics[3])
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
		Standard:         bchain.MultiToken,
		Contract:         EIP55AddressFromAddress(l.Address),
		From:             EIP55AddressFromAddress(from),
		To:               EIP55AddressFromAddress(to),
		MultiTokenValues: []bchain.MultiTokenValue{{Id: id, Value: value}},
	}, nil
}

func parseEVMLogWordUint64(data string, offset int) (uint64, error) {
	if offset < 0 || offset > len(data) || len(data)-offset < evmWordHex {
		return 0, errors.New("ERC1155 TransferBatch, invalid data length")
	}
	var b big.Int
	_, ok := b.SetString(data[offset:offset+evmWordHex], 16)
	if !ok || !b.IsUint64() {
		return 0, errors.New("ERC1155 TransferBatch, not a number")
	}
	return b.Uint64(), nil
}

func erc1155BatchOffsetHex(offsetBytes uint64) (int, error) {
	if offsetBytes < 2*evmWordBytes {
		return 0, errors.New("ERC1155 TransferBatch, invalid offset")
	}
	if offsetBytes%evmWordBytes != 0 {
		return 0, errors.New("ERC1155 TransferBatch, invalid offset")
	}
	maxInt := uint64(^uint(0) >> 1)
	if offsetBytes > maxInt/2 {
		return 0, errors.New("ERC1155 TransferBatch, invalid offset")
	}
	return int(offsetBytes * 2), nil
}

func erc1155BatchArrayEnd(offset int, count uint64) (int, error) {
	if offset < 0 {
		return 0, errors.New("ERC1155 TransferBatch, invalid offset")
	}
	maxInt := uint64(^uint(0) >> 1)
	base := uint64(offset) + evmWordHex
	if base > maxInt || count > (maxInt-base)/evmWordHex {
		return 0, errors.New("ERC1155 TransferBatch, invalid data length")
	}
	return int(base + count*evmWordHex), nil
}

func processERC1155TransferBatchEvent(l *bchain.RpcLog) (transfer *bchain.TokenTransfer, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Errorf("processERC1155TransferBatchEvent recovered from panic %v", r)
		}
	}()
	tl := len(l.Topics)
	if tl < 4 {
		return nil, nil
	}
	var from, to string
	from, err = addressFromPaddedHex(l.Topics[2])
	if err != nil {
		return nil, err
	}
	to, err = addressFromPaddedHex(l.Topics[3])
	if err != nil {
		return nil, err
	}
	data := l.Data
	if has0xPrefix(l.Data) {
		data = data[2:]
	}
	if len(data) < 2*evmWordHex || len(data)%evmWordHex != 0 {
		return nil, errors.New("ERC1155 TransferBatch, invalid data length")
	}
	offsetIdsBytes, err := parseEVMLogWordUint64(data, 0)
	if err != nil {
		return nil, err
	}
	offsetIds, err := erc1155BatchOffsetHex(offsetIdsBytes)
	if err != nil {
		return nil, err
	}
	offsetValuesBytes, err := parseEVMLogWordUint64(data, evmWordHex)
	if err != nil {
		return nil, err
	}
	offsetValues, err := erc1155BatchOffsetHex(offsetValuesBytes)
	if err != nil {
		return nil, err
	}
	countIds, err := parseEVMLogWordUint64(data, offsetIds)
	if err != nil {
		return nil, err
	}
	countValues, err := parseEVMLogWordUint64(data, offsetValues)
	if err != nil {
		return nil, err
	}
	if countIds != countValues {
		return nil, errors.New("ERC1155 TransferBatch, count values and ids does not match")
	}
	endIds, err := erc1155BatchArrayEnd(offsetIds, countValues)
	if err != nil {
		return nil, err
	}
	if endIds > len(data) {
		return nil, errors.New("ERC1155 TransferBatch, invalid ids data length")
	}
	endValues, err := erc1155BatchArrayEnd(offsetValues, countValues)
	if err != nil {
		return nil, err
	}
	if endValues > len(data) {
		return nil, errors.New("ERC1155 TransferBatch, invalid values data length")
	}
	// countValues cannot truncate: the endValues <= len(data) check above bounds
	// it to len(data)/evmWordHex.
	count := int(countValues)
	idValues := make([]bchain.MultiTokenValue, count)
	for i := 0; i < count; i++ {
		var id, value big.Int
		o := offsetIds + evmWordHex + evmWordHex*i
		_, ok := id.SetString(data[o:o+evmWordHex], 16)
		if !ok {
			return nil, errors.New("ERC1155 log Data id is not a number")
		}
		o = offsetValues + evmWordHex + evmWordHex*i
		_, ok = value.SetString(data[o:o+evmWordHex], 16)
		if !ok {
			return nil, errors.New("ERC1155 log Data value is not a number")
		}
		idValues[i] = bchain.MultiTokenValue{Id: id, Value: value}
	}
	return &bchain.TokenTransfer{
		Standard:         bchain.MultiToken,
		Contract:         EIP55AddressFromAddress(l.Address),
		From:             EIP55AddressFromAddress(from),
		To:               EIP55AddressFromAddress(to),
		MultiTokenValues: idValues,
	}, nil
}

// contractGetTransfersFromLog extracts token transfers from receipt logs.
// An unparseable log is skipped with a warning so that one malformed event
// does not discard the valid transfers of the transaction.
func contractGetTransfersFromLog(logs []*bchain.RpcLog, txid string) bchain.TokenTransfers {
	var r bchain.TokenTransfers
	for _, l := range logs {
		tl := len(l.Topics)
		if tl > 0 {
			signature := l.Topics[0]
			var tt *bchain.TokenTransfer
			var err error
			if signature == tokenTransferEventSignature {
				tt, err = processTransferEvent(l)
			} else if signature == tokenERC1155TransferSingleEventSignature {
				tt, err = processERC1155TransferSingleEvent(l)
			} else if signature == tokenERC1155TransferBatchEventSignature {
				tt, err = processERC1155TransferBatchEvent(l)
			} else {
				continue
			}
			if err != nil {
				glog.Warningf("contractGetTransfersFromLog: skipping unparseable log of contract %s, tx %s: %v", l.Address, txid, err)
				continue
			}
			if tt != nil {
				r = append(r, tt)
			}
		}
	}
	return r
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
			Standard: bchain.FungibleToken,
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
			Standard: bchain.NonFungibleToken,
			Contract: EIP55AddressFromAddress(tx.To),
			From:     EIP55AddressFromAddress(from),
			To:       EIP55AddressFromAddress(to),
			Value:    t,
		})
	}
	return r, nil
}

// EthereumTypeRpcCall calls eth_call with given data and to address
func (b *EthereumRPC) EthereumTypeRpcCall(data, to, from string) (string, error) {
	return b.EthereumTypeRpcCallAtBlock(data, to, from, nil)
}

// EthereumTypeRpcCallAtBlock calls eth_call with given data and to address at a specific block.
func (b *EthereumRPC) EthereumTypeRpcCallAtBlock(data, to, from string, blockNumber *big.Int) (string, error) {
	return b.ethCallAtBlock(data, to, from, blockNumber, "single")
}

// ethCallAtBlock is the shared single-eth_call primitive. mode labels the call in the
// eth_call request/error metrics ("single" for direct reads); "" emits nothing, for
// callers that instrument themselves (e.g. aggregate3, counted per sub-call).
func (b *EthereumRPC) ethCallAtBlock(data, to, from string, blockNumber *big.Int, mode string) (string, error) {
	args := map[string]interface{}{
		"data": data,
		"to":   to,
	}
	if from != "" {
		args["from"] = from
	}
	if mode != "" {
		b.observeEthCall(mode, 1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var r string
	blockArg := bchain.ToBlockNumArg(blockNumber)
	err := b.RPC.CallContext(ctx, &r, "eth_call", args, blockArg)
	if err != nil {
		if mode != "" {
			b.observeEthCallError(mode, "rpc")
		}
		return "", err
	}
	return r, nil
}

// EthereumTypeRpcCallBatch executes multiple eth_call requests in one JSON-RPC batch.
func (b *EthereumRPC) EthereumTypeRpcCallBatch(calls []bchain.EthereumTypeRPCCall) ([]bchain.EthereumTypeRPCCallResult, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	batcher, ok := b.RPC.(batchCaller)
	if !ok {
		return nil, errors.New("BatchCallContext not supported")
	}

	results := make([]string, len(calls))
	batch := make([]rpc.BatchElem, len(calls))
	blockArg := bchain.ToBlockNumArg(nil)
	for i := range calls {
		args := map[string]interface{}{
			"data": calls[i].Data,
			"to":   calls[i].To,
		}
		if calls[i].From != "" {
			args["from"] = calls[i].From
		}
		batch[i] = rpc.BatchElem{
			Method: "eth_call",
			Args:   []interface{}{args, blockArg},
			Result: &results[i],
		}
	}

	b.observeEthCall("batch", len(calls))
	b.observeEthCallBatch(len(calls))

	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	if err := batcher.BatchCallContext(ctx, batch); err != nil {
		b.observeEthCallError("batch", "rpc")
		return nil, err
	}

	out := make([]bchain.EthereumTypeRPCCallResult, len(calls))
	for i := range calls {
		out[i].Data = results[i]
		if batch[i].Error == nil {
			continue
		}
		b.observeEthCallError("batch", "elem")
		if isNonRetriableEthCallError(batch[i].Error) {
			out[i].Error = batch[i].Error
			continue
		}
		// Retry failed elements using single eth_call to avoid losing data on partial batch failures.
		data, err := b.EthereumTypeRpcCall(calls[i].Data, calls[i].To, calls[i].From)
		if err != nil {
			out[i].Error = err
			continue
		}
		out[i].Data = data
	}
	return out, nil
}

func erc20BalanceOfCallData(addrDesc bchain.AddressDescriptor) string {
	addr := hexutil.Encode(addrDesc)
	if len(addr) > 1 {
		addr = addr[2:]
	}
	padded := "0000000000000000000000000000000000000000000000000000000000000000"
	return contractBalanceOfSignature + padded[len(addr):] + addr
}

func (b *EthereumRPC) fetchContractInfo(address string) (*bchain.ContractInfo, error) {
	var contract bchain.ContractInfo
	b.observeEthCallContractInfo("name")
	data, err := b.EthereumTypeRpcCall(contractNameSignature, address, "")
	if err != nil {
		// ignore the error from the eth_call - since geth v1.9.15 they changed the behavior
		// and returning error "execution reverted" for some non contract addresses
		// https://github.com/ethereum/go-ethereum/issues/21249#issuecomment-648647672
		// glog.Warning(errors.Annotatef(err, "Contract NameSignature %v", address))
		return nil, nil
		// return nil, errors.Annotatef(err, "erc20NameSignature %v", address)
	}
	name := strings.TrimSpace(parseSimpleStringProperty(data))
	if name != "" {
		b.observeEthCallContractInfo("symbol")
		data, err = b.EthereumTypeRpcCall(contractSymbolSignature, address, "")
		if err != nil {
			// glog.Warning(errors.Annotatef(err, "Contract SymbolSignature %v", address))
			return nil, nil
			// return nil, errors.Annotatef(err, "erc20SymbolSignature %v", address)
		}
		symbol := strings.TrimSpace(parseSimpleStringProperty(data))
		b.observeEthCallContractInfo("decimals")
		data, _ = b.EthereumTypeRpcCall(contractDecimalsSignature, address, "")
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

// ErrInvalidErc20Balance means a balanceOf eth_call yielded no usable balance: either it returned
// unparseable data (empty "0x" or non-32-byte output) or it reverted deterministically. Both are
// benign and common for dead/non-conforming/rebasing tokens; callers treat it as "no balance" and
// must not log it at warning level (tracked via the observeEthCallError "invalid"/"reverted" metrics).
var ErrInvalidErc20Balance = errors.New("Invalid balance")

// EthereumTypeGetErc20ContractBalance returns balance of ERC20 contract for given address
func (b *EthereumRPC) EthereumTypeGetErc20ContractBalance(addrDesc, contractDesc bchain.AddressDescriptor) (*big.Int, error) {
	return b.EthereumTypeGetErc20ContractBalanceAtBlock(addrDesc, contractDesc, nil)
}

// EthereumTypeGetErc20ContractBalanceAtBlock returns balance of ERC20 contract for given address at a specific block.
func (b *EthereumRPC) EthereumTypeGetErc20ContractBalanceAtBlock(addrDesc, contractDesc bchain.AddressDescriptor, blockNumber *big.Int) (*big.Int, error) {
	contract := hexutil.Encode(contractDesc)
	req := erc20BalanceOfCallData(addrDesc)
	data, err := b.EthereumTypeRpcCallAtBlock(req, contract, "", blockNumber)
	if err != nil {
		// A deterministic revert (balanceOf reverts for this holder) is benign: no usable
		// balance. Match the batch path and treat it as ErrInvalidErc20Balance
		if isNonRetriableEthCallError(err) {
			b.observeEthCallError("single", "reverted")
			return nil, ErrInvalidErc20Balance
		}
		return nil, err
	}
	r := parseSimpleNumericProperty(data)
	if r == nil {
		b.observeEthCallError("single", "invalid")
		return nil, ErrInvalidErc20Balance
	}
	return r, nil
}

type batchCaller interface {
	BatchCallContext(context.Context, []rpc.BatchElem) error
}

func (b *EthereumRPC) erc20BatchSize() int {
	if b.ChainConfig != nil && b.ChainConfig.Erc20BatchSize > 0 {
		return b.ChainConfig.Erc20BatchSize
	}
	return defaultErc20BatchSize
}

// EthereumTypeGetErc20ContractBalances returns balances of multiple ERC20 contracts for a given
// address. It prefers Multicall3 aggregate3 (falling back to the JSON-RPC batch) and returns nil
// entries for failed/invalid results.
func (b *EthereumRPC) EthereumTypeGetErc20ContractBalances(addrDesc bchain.AddressDescriptor, contractDescs []bchain.AddressDescriptor) ([]*big.Int, error) {
	return b.EthereumTypeGetErc20ContractBalancesAtBlock(addrDesc, contractDescs, nil)
}

// EthereumTypeGetErc20ContractBalancesAtBlock returns balances for multiple ERC20 contracts at a
// block: Multicall3 aggregate3 when available, else the JSON-RPC batch, with only the contracts
// aggregate3 could not settle falling back. Both return nil entries for failed/invalid results,
// in input order.
func (b *EthereumRPC) EthereumTypeGetErc20ContractBalancesAtBlock(addrDesc bchain.AddressDescriptor, contractDescs []bchain.AddressDescriptor, blockNumber *big.Int) ([]*big.Int, error) {
	if len(contractDescs) == 0 {
		return nil, nil
	}
	// Same calldata for all balanceOf calls; only the contract address varies per element.
	callData := erc20BalanceOfCallData(addrDesc)
	// Multicall3 needs no batcher (single eth_call); the batcher is only for the batch path.
	batcher, hasBatcher := b.RPC.(batchCaller)

	// Prefer Multicall3, but only when the cached probe reports it deployed — otherwise a
	// non-multicall chain would build and hex-encode calls on every request just to discard them.
	deployed, probeErr := b.probeMulticall3()
	if probeErr != nil {
		// Transient probe failure — visible so an eth_getCode-restricting provider is not silent.
		b.ObserveChainDataFallback("erc20_multicall", "probe_error")
	}
	if deployed {
		mcBalances, reresolve, unresolved := b.erc20BalancesMulticall3(callData, contractDescs, blockNumber)
		anyResolved := len(unresolved) < len(contractDescs)
		// Keep what aggregate3 resolved unless every chunk failed (then the plain batch path
		// below is the same work with a propagatable error), or chunk-failure holes remain that
		// no batcher can fill — a nil entry for an unknown balance reads as "no balance" to
		// callers that treat a present entry as authoritative, so let them fall back to single
		// calls. Reresolve holes never block: single eth_calls settle them without a batcher.
		canFillHoles := len(unresolved) == 0 || hasBatcher
		if anyResolved && canFillHoles {
			// aggregate3 shares one gas budget, so a Success=false element can be a gas-starved
			// (not truly empty) balance, and a failed chunk leaves its elements unknown. Settle
			// both with independent eth_calls (full gas cap each): one JSON-RPC batch when the
			// client supports it, else individual calls (canFillHoles guarantees only reresolve
			// holes remain then). A real revert stays nil.
			if pending := len(reresolve) + len(unresolved); pending > 0 {
				idx := make([]int, 0, pending)
				idx = append(idx, reresolve...)
				idx = append(idx, unresolved...)
				// Count every contract settled by independent calls, whichever transport and
				// whichever hole put it there; counting only reresolve would report zero for a
				// chunk failure that re-fetched contracts, hiding the extra round trip from
				// cost dashboards.
				b.observeChainDataFallback("erc20_multicall", "elem_fallback", len(idx))
				if hasBatcher {
					sub := make([]bchain.AddressDescriptor, len(idx))
					for j, i := range idx {
						sub[j] = contractDescs[i]
					}
					if subBalances, berr := b.erc20BalancesBatchChunked(batcher, callData, sub, blockNumber); berr == nil {
						for j, i := range idx {
							mcBalances[i] = subBalances[j]
						}
					} else if len(unresolved) > 0 {
						// Chunk-failure holes are unknown balances; returning them as nil would
						// read as authoritative "no balance", so propagate the error and let the
						// caller fall back to single calls. (Unreachable today —
						// erc20BalancesBatchChunked settles element failures internally — but
						// kept so a future error path cannot leak unknowns as nil.)
						return nil, berr
					} else {
						// Only best-effort reresolve holes remain: keep what aggregate3 resolved.
						glog.Warningf("erc20 multicall3 fallback failed for %d contract(s): %v", len(idx), berr)
					}
				} else {
					for _, i := range idx {
						contract := hexutil.Encode(contractDescs[i])
						data, cerr := b.EthereumTypeRpcCallAtBlock(callData, contract, "", blockNumber)
						if cerr != nil {
							glog.Warningf("erc20 single eth_call fallback failed for %s: %v", contract, cerr)
							continue
						}
						mcBalances[i] = parseSimpleNumericProperty(data)
						if mcBalances[i] == nil {
							b.observeEthCallError("single", "invalid")
							// Benign and high-volume: see erc20BalancesBatchAtBlock's single-call
							// fallback, which this mirrors.
							glog.V(2).Infof("erc20 single eth_call invalid result for %s: %q", contract, data)
						}
					}
				}
			}
			return mcBalances, nil
		}
		if !hasBatcher && len(unresolved) > 0 {
			// Multicall3 is deployed but aggregate3 left holes nothing here can fill. Name the
			// real failure instead of the generic no-batcher error below; the caller only logs
			// the message and falls back to single calls either way.
			return nil, errors.Errorf("multicall3 left %d of %d balances unresolved and BatchCallContext not supported", len(unresolved), len(contractDescs))
		}
	}
	if !hasBatcher {
		// Some RPC clients do not support batching; caller will fall back to single calls.
		return nil, errors.New("BatchCallContext not supported")
	}
	return b.erc20BalancesBatchChunked(batcher, callData, contractDescs, blockNumber)
}

// erc20BalancesBatchChunked fetches balances via the JSON-RPC batch path, split into
// erc20BatchSize chunks to keep each batch request within provider limits.
func (b *EthereumRPC) erc20BalancesBatchChunked(batcher batchCaller, callData string, contractDescs []bchain.AddressDescriptor, blockNumber *big.Int) ([]*big.Int, error) {
	balances := make([]*big.Int, len(contractDescs))
	batchSize := b.erc20BatchSize()
	for start := 0; start < len(contractDescs); start += batchSize {
		end := start + batchSize
		if end > len(contractDescs) {
			end = len(contractDescs)
		}
		chunkBalances, err := b.erc20BalancesBatchAtBlock(batcher, callData, contractDescs[start:end], blockNumber)
		if err != nil {
			return nil, err
		}
		copy(balances[start:end], chunkBalances)
	}
	return balances, nil
}

// multicall3MaxCallsPerAggregate is the aggregate3 chunk size: how many balanceOf sub-calls go
// into one aggregate3 eth_call. Unlike a JSON-RPC batch, whose size (erc20BatchSize) is bounded
// by provider request-count limits, aggregate3 is a single request whose only constraint is the
// node's eth_call gas budget shared by all sub-calls — hence a bound of its own, independent of
// erc20_batch_size.
const multicall3MaxCallsPerAggregate = 100

// erc20BalancesMulticall3 fetches balanceOf for all contracts via aggregate3, sub-chunked to
// bound each eth_call's gas. Malformed (non-address) descriptors are excluded up front and stay
// nil — matching the batch path, where a bogus `to` yields an element error and thus nil — so one
// bad element cannot fail its whole chunk's encoding. Success=true results are decoded (nil on
// unparseable data, as in the batch path). It returns two index sets the caller settles with
// independent eth_calls:
//
//   - reresolve: Success=false with empty returndata, possibly gas-starved under the shared
//     aggregate3 gas budget, so a real balance must not be silently lost. Genuine reverts
//     (returndata present) stay nil and are not listed.
//   - unresolved: elements from the first failing aggregate3 chunk onwards, about which nothing
//     is known. Chunks decoded before it are kept, so one bad chunk never costs a re-fetch of
//     the whole list; the loop stops there because chunk failures are usually systemic.
func (b *EthereumRPC) erc20BalancesMulticall3(callData string, contractDescs []bchain.AddressDescriptor, blockNumber *big.Int) (balances []*big.Int, reresolve []int, unresolved []int) {
	balances = make([]*big.Int, len(contractDescs))
	valid := make([]int, 0, len(contractDescs))
	for i := range contractDescs {
		if len(contractDescs[i]) == ethcommon.AddressLength {
			valid = append(valid, i)
		}
	}
	if skipped := len(contractDescs) - len(valid); skipped > 0 {
		glog.V(2).Infof("erc20 balances multicall3: excluding %d malformed contract descriptor(s)", skipped)
	}
	for start := 0; start < len(valid); start += multicall3MaxCallsPerAggregate {
		end := start + multicall3MaxCallsPerAggregate
		if end > len(valid) {
			end = len(valid)
		}
		chunk := valid[start:end]
		calls := make([]bchain.EthereumMulticallCall, len(chunk))
		for j, i := range chunk {
			calls[j] = bchain.EthereumMulticallCall{
				Target:   hexutil.Encode(contractDescs[i]),
				CallData: callData,
				// a reverting balanceOf yields Success=false, not a failed batch
				AllowFailure: true,
			}
		}
		results, err := b.EthereumTypeMulticallAggregate3(calls, blockNumber)
		if err != nil {
			// A chunk failure is usually systemic (eth_call gas cap, decode, transport), so stop
			// at the first one instead of paying a doomed aggregate3 for every chunk left: mark
			// this chunk and the remaining valid elements unresolved for the caller to settle.
			// Chunks already decoded are kept, so the failure costs only the remainder. Breaking
			// also keeps this one fallback event and one log line per request.
			unresolved = append(unresolved, valid[start:]...)
			b.ObserveChainDataFallback("erc20_multicall", "error")
			glog.Warningf("erc20 balances multicall3 failed at chunk [%d:%d), falling back for %d contract(s): %v", start, end, len(valid)-start, err)
			break
		}
		for j := range results {
			i := chunk[j]
			if !results[j].Success {
				b.observeEthCallError("multicall", "elem")
				// Non-empty returndata is a genuine revert (Error/Panic data), never an
				// out-of-gas truncation, so leave it nil. Only empty returndata may be gas
				// starvation under the shared budget — re-resolve those (see the caller).
				if len(results[j].Data) <= 2 {
					reresolve = append(reresolve, i)
				}
				continue
			}
			// nil on unparseable output (empty/short), matching the batch path
			balances[i] = parseSimpleNumericProperty(results[j].Data)
			if balances[i] == nil {
				b.observeEthCallError("multicall", "invalid")
			}
		}
	}
	return balances, reresolve, unresolved
}

func (b *EthereumRPC) erc20BalancesBatch(batcher batchCaller, callData string, contractDescs []bchain.AddressDescriptor) ([]*big.Int, error) {
	return b.erc20BalancesBatchAtBlock(batcher, callData, contractDescs, nil)
}

func (b *EthereumRPC) erc20BalancesBatchAtBlock(batcher batchCaller, callData string, contractDescs []bchain.AddressDescriptor, blockNumber *big.Int) ([]*big.Int, error) {
	results := make([]string, len(contractDescs))
	batch := make([]rpc.BatchElem, len(contractDescs))
	blockArg := bchain.ToBlockNumArg(blockNumber)
	for i, contractDesc := range contractDescs {
		args := map[string]interface{}{
			"data": callData,
			"to":   hexutil.Encode(contractDesc),
		}
		batch[i] = rpc.BatchElem{
			Method: "eth_call",
			Args:   []interface{}{args, blockArg},
			Result: &results[i],
		}
	}
	b.observeEthCall("batch", len(contractDescs))
	b.observeEthCallBatch(len(contractDescs))
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	if err := batcher.BatchCallContext(ctx, batch); err != nil {
		b.observeEthCallError("batch", "rpc")
		// Distinct fallback metric so monitoring can alert on this path even
		// though we suppress the error to keep callers (e.g. account info)
		// usable on transient batch-level RPC failures.
		b.ObserveChainDataFallback("erc20_batch", "rpc")
		glog.Warningf("erc20 batch eth_call failed: %v, falling back to single calls", err)
		balances := make([]*big.Int, len(contractDescs))
		for i, contractDesc := range contractDescs {
			data, err := b.EthereumTypeRpcCallAtBlock(callData, hexutil.Encode(contractDesc), "", blockNumber)
			if err != nil {
				glog.Warningf("erc20 single eth_call fallback failed for %s: %v", hexutil.Encode(contractDesc), err)
				continue
			}
			balances[i] = parseSimpleNumericProperty(data)
			if balances[i] == nil {
				b.observeEthCallError("single", "invalid")
				// Benign and high-volume: a successful eth_call returning empty/non-32-byte data, typical of
				// dead (self-destructed) or non-ERC20-conforming tokens that linger in holders' contract lists.
				// Tracked via the "invalid" metric; logged at V(2) to avoid flooding (one line per holder request).
				glog.V(2).Infof("erc20 single eth_call invalid result for %s: %q", hexutil.Encode(contractDesc), data)
			}
		}
		return balances, nil
	}
	balances := make([]*big.Int, len(contractDescs))
	for i := range batch {
		if batch[i].Error != nil {
			b.observeEthCallError("batch", "elem")
			if isNonRetriableEthCallError(batch[i].Error) {
				continue
			}
			glog.Warningf("erc20 batch eth_call failed for %s: %v", hexutil.Encode(contractDescs[i]), batch[i].Error)
			// In case of individual element failure in a successful batch, retry it as a single call.
			data, err := b.EthereumTypeRpcCallAtBlock(callData, hexutil.Encode(contractDescs[i]), "", blockNumber)
			if err != nil {
				glog.Warningf("erc20 single eth_call fallback failed for %s: %v", hexutil.Encode(contractDescs[i]), err)
				continue
			}
			balances[i] = parseSimpleNumericProperty(data)
			if balances[i] == nil {
				b.observeEthCallError("single", "invalid")
				glog.V(2).Infof("erc20 single eth_call invalid result for %s: %q", hexutil.Encode(contractDescs[i]), data)
			}
			continue
		}
		// Leave nil on parse failures; retrying as a single call is unlikely to help
		// as malformed returns usually indicate non-conforming contract implementations.
		balances[i] = parseSimpleNumericProperty(results[i])
		if balances[i] == nil {
			b.observeEthCallError("batch", "invalid")
			// Benign and high-volume: see the single-call note above. Same event on the batch success path,
			// dominated by widely-airdropped dead/non-conforming tokens. Tracked via the "invalid" metric.
			glog.V(2).Infof("erc20 batch eth_call invalid result for %s: %q", hexutil.Encode(contractDescs[i]), results[i])
		}
	}
	return balances, nil
}

func isNonRetriableEthCallError(err error) bool {
	if err == nil {
		return false
	}
	// These errors are deterministic for the given call data and won't succeed on retry.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "execution reverted") ||
		strings.Contains(msg, "invalid opcode") ||
		strings.Contains(msg, "out of gas") ||
		strings.Contains(msg, "stack underflow") ||
		strings.Contains(msg, "revert")
}

// GetTokenURI returns URI of non fungible or multi token defined by token id
func (b *EthereumRPC) GetTokenURI(contractDesc bchain.AddressDescriptor, tokenID *big.Int) (string, error) {
	address := hexutil.Encode(contractDesc)
	// CryptoKitties do not fully support ERC721 standard, do not have tokenURI method
	if address == "0x06012c8cf97bead5deae237070f9587f8e7a266d" {
		return "https://api.cryptokitties.co/kitties/" + tokenID.Text(10), nil
	}
	id := tokenID.Text(16)
	if len(id) < 64 {
		id = "0000000000000000000000000000000000000000000000000000000000000000"[len(id):] + id
	}
	// try ERC721 tokenURI method and  ERC1155 uri method
	for _, method := range []string{erc721TokenURIMethodSignature, erc1155URIMethodSignature} {
		if method == erc721TokenURIMethodSignature {
			b.observeEthCallTokenURI("erc721_token_uri")
		} else {
			b.observeEthCallTokenURI("erc1155_uri")
		}
		data, err := b.EthereumTypeRpcCall(method+id, address, "")
		if err == nil && data != "" {
			uri := parseSimpleStringProperty(data)
			// try to sanitize the URI returned from the contract
			i := strings.LastIndex(uri, "ipfs://")
			if i >= 0 {
				uri = strings.Replace(uri[i:], "ipfs://", "https://ipfs.io/ipfs/", 1)
				// some contracts return uri ipfs://ifps/abcdef instead of ipfs://abcdef
				uri = strings.Replace(uri, "https://ipfs.io/ipfs/ipfs/", "https://ipfs.io/ipfs/", 1)
			}
			i = strings.LastIndex(uri, "https://")
			// allow only https:// URIs
			if i >= 0 {
				uri = strings.ReplaceAll(uri[i:], "{id}", id)
				return uri, nil
			}
		}
	}
	return "", nil
}
