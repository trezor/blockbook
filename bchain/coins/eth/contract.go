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
	return b.ethCallAtBlock(data, to, from, blockNumber, "single", 1)
}

// ethCallAtBlock is the shared single-eth_call primitive. mode labels the call in the eth_call
// metrics and subCalls is how many logical reads it carries (1 for direct reads, the batch size
// for a coalescing call), so no caller can opt out of the instrumentation.
func (b *EthereumRPC) ethCallAtBlock(data, to, from string, blockNumber *big.Int, mode string, subCalls int) (string, error) {
	args := map[string]interface{}{
		"data": data,
		"to":   to,
	}
	if from != "" {
		args["from"] = from
	}
	b.observeEthCall(mode, subCalls)
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()
	var r string
	blockArg := bchain.ToBlockNumArg(blockNumber)
	err := b.RPC.CallContext(ctx, &r, "eth_call", args, blockArg)
	if err != nil {
		b.observeEthCallError(mode, "rpc")
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

// fetchContractInfo reads name/symbol/decimals, short-circuiting as soon as name() shows
// the address holds no token.
//
// The returned pair is a verdict, not just a result: (nil, nil) means the chain
// conclusively reports no token at the address, a non-nil error means the read itself
// failed and says nothing about the contract. Callers must not persist or cache a
// "not a token" conclusion drawn from an error.
func (b *EthereumRPC) fetchContractInfo(address string) (*bchain.ContractInfo, error) {
	b.observeEthCallContractInfo("name")
	data, err := b.EthereumTypeRpcCall(contractNameSignature, address, "")
	if err != nil {
		// since geth v1.9.15 a call to a non-contract address reverts instead of returning
		// empty data, so a deterministic error is the usual "not a token" signal
		// https://github.com/ethereum/go-ethereum/issues/21249#issuecomment-648647672
		if isNonRetriableEthCallError(err) {
			return nil, nil
		}
		return nil, errors.Annotatef(err, "contract name() %v", address)
	}
	name := strings.TrimSpace(parseSimpleStringProperty(data))
	if name == "" {
		return nil, nil
	}
	b.observeEthCallContractInfo("symbol")
	data, err = b.EthereumTypeRpcCall(contractSymbolSignature, address, "")
	if err != nil {
		// a contract whose symbol() reverts has never been surfaced as a token
		if isNonRetriableEthCallError(err) {
			return nil, nil
		}
		return nil, errors.Annotatef(err, "contract symbol() %v", address)
	}
	symbol := strings.TrimSpace(parseSimpleStringProperty(data))
	b.observeEthCallContractInfo("decimals")
	// decimals() is optional; an unreadable value falls back to the coin default
	data, _ = b.EthereumTypeRpcCall(contractDecimalsSignature, address, "")
	return &bchain.ContractInfo{
		Contract: address,
		Name:     name,
		Symbol:   symbol,
		Decimals: contractDecimalsOrDefault(parseSimpleNumericProperty(data)),
	}, nil
}

// contractDecimalsOrDefault narrows a decimals() return to the stored width, falling back
// to the coin default when the call yielded nothing usable.
func contractDecimalsOrDefault(d *big.Int) int {
	if d == nil {
		return EtherAmountDecimalPoint
	}
	return int(uint8(d.Uint64()))
}

// GetContractInfo returns information about a contract
func (b *EthereumRPC) GetContractInfo(contractDesc bchain.AddressDescriptor) (*bchain.ContractInfo, error) {
	address := EIP55Address(contractDesc)
	return b.fetchContractInfo(address)
}

// contractInfoSignatures are the metadata reads issued per contract, in the order the
// aggregate3 sub-calls are laid out.
var contractInfoSignatures = [...]string{contractNameSignature, contractSymbolSignature, contractDecimalsSignature}

// contractInfoCallsPerContract is how many aggregate3 slots one contract occupies.
const contractInfoCallsPerContract = len(contractInfoSignatures)

// multicall3StarvationCanary closes every aggregate3 chunk. A call to a codeless address
// succeeds for almost no gas, so it can only fail once the chunk ran out - which is what
// tells a sub-call's bare revert (a real "not a token") apart from a starved one. The
// element itself cannot: both come back as a failure with empty returndata.
const multicall3StarvationCanary = "0x0000000000000000000000000000000000000000"

// EthereumTypeGetContractInfos resolves metadata for many contracts at once, in input order.
// One aggregate3 eth_call carries name/symbol/decimals for a chunk of contracts; without
// Multicall3 each contract falls back to the sequential path, which bills a single call for
// the common "not a token" case instead of three.
func (b *EthereumRPC) EthereumTypeGetContractInfos(contractDescs []bchain.AddressDescriptor) []bchain.EthereumContractInfoResult {
	if len(contractDescs) == 0 {
		return nil
	}
	results := make([]bchain.EthereumContractInfoResult, len(contractDescs))
	deployed, err := b.probeMulticall3()
	if err != nil {
		// visible so an eth_getCode-restricting provider is not silent
		b.ObserveChainDataFallback("contract_info_multicall", "probe_error")
	}
	if !deployed {
		for i := range contractDescs {
			results[i] = b.contractInfoSequential(contractDescs[i])
		}
		return results
	}
	for _, i := range b.contractInfosMulticall3(contractDescs, results) {
		results[i] = b.contractInfoSequential(contractDescs[i])
	}
	return results
}

// contractInfoSequential resolves one contract with the short-circuiting eth_calls - the
// path for chains without Multicall3 and for elements aggregate3 left ambiguous.
func (b *EthereumRPC) contractInfoSequential(contractDesc bchain.AddressDescriptor) bchain.EthereumContractInfoResult {
	info, err := b.fetchContractInfo(EIP55Address(contractDesc))
	if info != nil {
		info.Contract = b.contractAddress(contractDesc)
	}
	return bchain.EthereumContractInfoResult{Info: info, Err: err}
}

// contractAddress renders the descriptor in the coin's own address form (EIP-55 on
// Ethereum, Base58 on Tron) so a batched result carries the same address the
// single-contract path stores.
func (b *EthereumRPC) contractAddress(contractDesc bchain.AddressDescriptor) string {
	if b.Parser != nil {
		if addresses, _, err := b.Parser.GetAddressesFromAddrDesc(contractDesc); err == nil && len(addresses) > 0 {
			return addresses[0]
		}
	}
	return EIP55Address(contractDesc)
}

// contractInfosMulticall3 fills results from gas-bounded aggregate3 chunks and returns the
// indexes it could not settle; the caller re-reads those with independent eth_calls.
func (b *EthereumRPC) contractInfosMulticall3(contractDescs []bchain.AddressDescriptor, results []bchain.EthereumContractInfoResult) (holes []int) {
	valid := make([]int, 0, len(contractDescs))
	for i := range contractDescs {
		if len(contractDescs[i]) == EthereumTypeAddressDescriptorLen {
			valid = append(valid, i)
			continue
		}
		// a malformed descriptor would fail the whole chunk at encode time
		holes = append(holes, i)
	}
	// the canary takes a slot of its own, so the chunk still fits the configured budget
	perChunk := (b.multicall3MaxCalls() - 1) / contractInfoCallsPerContract
	if perChunk < 1 {
		perChunk = 1
	}
	starved := 0
	for start := 0; start < len(valid); start += perChunk {
		end := start + perChunk
		if end > len(valid) {
			end = len(valid)
		}
		chunk := valid[start:end]
		res, err := b.EthereumTypeMulticallAggregate3(contractInfoCalls(contractDescs, chunk), nil)
		if err != nil {
			// chunk failures are usually systemic: stop at the first and leave the rest to the
			// sequential path, one fallback event and one log line per request
			holes = append(holes, valid[start:]...)
			b.ObserveChainDataFallback("contract_info_multicall", "error")
			glog.Warningf("contract info multicall3 failed at chunk [%d:%d), falling back for %d contract(s): %v", start, end, len(valid)-start, err)
			break
		}
		if len(res) != len(chunk)*contractInfoCallsPerContract+1 {
			// aggregate3 verifies the count itself; refuse to index into a short response
			holes = append(holes, chunk...)
			continue
		}
		b.observeEthCallContractInfos(len(chunk))
		// the canary is the last slot; see multicall3StarvationCanary
		gasStarved := !res[len(res)-1].Success
		for j, i := range chunk {
			slots := res[j*contractInfoCallsPerContract : (j+1)*contractInfoCallsPerContract]
			if gasStarved && (emptyAggregate3Failure(slots[0]) || emptyAggregate3Failure(slots[1])) {
				// re-read rather than record a verdict the caller would go on to cache
				holes = append(holes, i)
				starved++
				continue
			}
			results[i] = contractInfoFromAggregate3(b.contractAddress(contractDescs[i]), slots)
		}
	}
	// element-level holes, counted apart from a failing chunk's one "error" event
	b.observeChainDataFallback("contract_info_multicall", "elem_fallback", starved)
	return holes
}

// contractInfoCalls lays out the sub-calls for one chunk: the metadata reads of every
// contract in order, closed by the starvation canary.
func contractInfoCalls(contractDescs []bchain.AddressDescriptor, chunk []int) []bchain.EthereumMulticallCall {
	calls := make([]bchain.EthereumMulticallCall, 0, len(chunk)*contractInfoCallsPerContract+1)
	for _, i := range chunk {
		target := hexutil.Encode(contractDescs[i])
		for _, signature := range contractInfoSignatures {
			// a reverting metadata getter yields Success=false, not a failed batch
			calls = append(calls, bchain.EthereumMulticallCall{Target: target, CallData: signature, AllowFailure: true})
		}
	}
	return append(calls, bchain.EthereumMulticallCall{Target: multicall3StarvationCanary, AllowFailure: true})
}

// contractInfoFromAggregate3 turns one contract's slots into the verdict fetchContractInfo
// would have reached from the same three eth_calls.
func contractInfoFromAggregate3(address string, slots []bchain.EthereumMulticallResult) bchain.EthereumContractInfoResult {
	var name string
	if slots[0].Success {
		name = strings.TrimSpace(parseSimpleStringProperty(slots[0].Data))
	}
	// a failed or blank name() is how the chain reports that there is no token here, and a
	// reverting symbol() has never been surfaced as one either
	if name == "" || !slots[1].Success {
		return bchain.EthereumContractInfoResult{}
	}
	var decimals *big.Int
	if slots[2].Success {
		decimals = parseSimpleNumericProperty(slots[2].Data)
	}
	return bchain.EthereumContractInfoResult{Info: &bchain.ContractInfo{
		Contract: address,
		Name:     name,
		Symbol:   strings.TrimSpace(parseSimpleStringProperty(slots[1].Data)),
		Decimals: contractDecimalsOrDefault(decimals),
	}}
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
// address; see EthereumTypeGetErc20ContractBalancesAtBlock.
func (b *EthereumRPC) EthereumTypeGetErc20ContractBalances(addrDesc bchain.AddressDescriptor, contractDescs []bchain.AddressDescriptor) ([]*big.Int, error) {
	return b.EthereumTypeGetErc20ContractBalancesAtBlock(addrDesc, contractDescs, nil)
}

// EthereumTypeGetErc20ContractBalancesAtBlock returns balances for multiple ERC20 contracts at a
// block: Multicall3 aggregate3 when available (contracts it leaves unsettled go through the
// JSON-RPC batch), else the JSON-RPC batch alone. Nil entries mark failed/invalid results,
// in input order.
func (b *EthereumRPC) EthereumTypeGetErc20ContractBalancesAtBlock(addrDesc bchain.AddressDescriptor, contractDescs []bchain.AddressDescriptor, blockNumber *big.Int) ([]*big.Int, error) {
	if len(contractDescs) == 0 {
		return nil, nil
	}
	// Same calldata for all balanceOf calls; only the contract address varies per element.
	callData := erc20BalanceOfCallData(addrDesc)
	batcher, hasBatcher := b.RPC.(batchCaller)
	if !hasBatcher {
		// Some RPC clients do not support batching; caller will fall back to single calls.
		// Multicall3 needs no batcher itself, but settling its holes does.
		return nil, errors.New("BatchCallContext not supported")
	}

	// Gate on the cached probe so non-multicall chains don't build calls just to discard them.
	deployed, probeErr := b.probeMulticall3()
	if probeErr != nil {
		// Transient probe failure — visible so an eth_getCode-restricting provider is not silent.
		b.ObserveChainDataFallback("erc20_multicall", "probe_error")
	}
	if !deployed {
		return b.erc20BalancesBatchChunked(batcher, callData, contractDescs, blockNumber)
	}
	balances, holes := b.erc20BalancesMulticall3(callData, contractDescs, blockNumber)
	if len(holes) == 0 {
		return balances, nil
	}
	// Callers treat a present entry — even nil — as authoritative, so settle holes with
	// independent eth_calls (full gas cap each) instead of reporting them as nil.
	sub := make([]bchain.AddressDescriptor, len(holes))
	for j, i := range holes {
		sub[j] = contractDescs[i]
	}
	subBalances, err := b.erc20BalancesBatchChunked(batcher, callData, sub, blockNumber)
	if err != nil {
		return nil, err
	}
	for j, i := range holes {
		balances[i] = subBalances[j]
	}
	return balances, nil
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

// emptyAggregate3Failure reports an element that failed with no returndata: a bare revert()
// and gas starvation are indistinguishable from the element alone.
func emptyAggregate3Failure(r bchain.EthereumMulticallResult) bool {
	return !r.Success && len(r.Data) <= 2
}

// erc20BalancesMulticall3 fetches balanceOf via gas-bounded aggregate3 chunks. holes are the
// indexes it could not settle; the caller must re-read those with independent eth_calls.
func (b *EthereumRPC) erc20BalancesMulticall3(callData string, contractDescs []bchain.AddressDescriptor, blockNumber *big.Int) (balances []*big.Int, holes []int) {
	balances = make([]*big.Int, len(contractDescs))
	// Tallied per element and emitted once: this loop runs per held token, and every
	// observe* call would otherwise rebuild a label map and re-resolve the child counter.
	starved, elemErrors, invalid := 0, 0, 0
	valid := make([]int, 0, len(contractDescs))
	for i := range contractDescs {
		if len(contractDescs[i]) == EthereumTypeAddressDescriptorLen {
			valid = append(valid, i)
		}
	}
	if skipped := len(contractDescs) - len(valid); skipped > 0 {
		glog.V(2).Infof("erc20 balances multicall3: excluding %d malformed contract descriptor(s)", skipped)
	}
	maxCalls := b.multicall3MaxCalls()
	for start := 0; start < len(valid); start += maxCalls {
		end := start + maxCalls
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
			// Chunk failures are usually systemic: stop at the first, keep decoded chunks and
			// leave the rest to the caller — one fallback event and one log line per request.
			holes = append(holes, valid[start:]...)
			b.ObserveChainDataFallback("erc20_multicall", "error")
			glog.Warningf("erc20 balances multicall3 failed at chunk [%d:%d), falling back for %d contract(s): %v", start, end, len(valid)-start, err)
			break
		}
		// Starvation usually shows up as a trailing run: a sub-call that exhausts the gas
		// leaves aggregate3 only the 1/64 the EVM withholds, rarely enough for the calls
		// after it. A heuristic, not an invariant — it assumes comparably priced sub-calls,
		// and a mid-chunk gas hog can spare enough for the rest to succeed, leaving its own
		// empty failure read as a bare revert and its balance nil. Accepted because treating
		// every empty failure as starved costs a metered call per dead token per request.
		tailStarved := len(results) > 0 && emptyAggregate3Failure(results[len(results)-1])
		for j := range results {
			i := chunk[j]
			if !results[j].Success {
				elemErrors++
				// Once the tail is starved, earlier empty failures may be starved too.
				if tailStarved && emptyAggregate3Failure(results[j]) {
					holes = append(holes, i)
					starved++
				}
				continue
			}
			// nil on unparseable output (empty/short), matching the batch path
			balances[i] = parseSimpleNumericProperty(results[j].Data)
			if balances[i] == nil {
				invalid++
			}
		}
	}
	b.observeEthCallErrors("multicall", "elem", elemErrors)
	b.observeEthCallErrors("multicall", "invalid", invalid)
	// element-level holes, counted apart from a failing chunk's one "error" event
	b.observeChainDataFallback("erc20_multicall", "elem_fallback", starved)
	return balances, holes
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
		// Tron's phrasing for no code at the address; a single-call retry cannot succeed either.
		strings.Contains(msg, "smart contract is not exist") ||
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
