package eth

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
	"golang.org/x/sync/singleflight"
)

// Canonical Multicall3 deployment, identical address on every major EVM chain.
// See https://github.com/mds1/multicall.
const multicall3Address = "0xcA11bde05977b3631167028862bE2a173976CA11"

// Function selector for aggregate3((address,bool,bytes)[]).
// Verified: keccak256("aggregate3((address,bool,bytes)[])")[:4].
var multicall3Aggregate3Selector = []byte{0x82, 0xad, 0x56, 0xcb}

// multicall3Gate.state values; Unprobed is the zero value.
const (
	multicall3Unprobed    int32 = 0
	multicall3Deployed    int32 = 1
	multicall3NotDeployed int32 = 2
)

// multicall3MaxProbeFailures is the consecutive transient probe failures before probing pauses.
const multicall3MaxProbeFailures = 5

// multicall3ProbeSuspendInterval is the probing pause: no per-request probes on a restricted
// provider, yet multicall is never disabled for good.
const multicall3ProbeSuspendInterval = 10 * time.Minute

// multicall3MaxCallsPerAggregate bounds one aggregate3 eth_call by its shared gas budget;
// deliberately independent of erc20BatchSize, which exists for JSON-RPC request-count limits.
const multicall3MaxCallsPerAggregate = 100

// errMulticall3NotDeployed is returned on chains where Multicall3 is not deployed
// at the probed address (canonical, or a per-chain override); cached for the process lifetime.
var errMulticall3NotDeployed = errors.New("multicall3 not deployed on this chain")

// multicall3Gate holds whether Multicall3 is usable on this chain: the probe verdict plus the
// consecutive-failure count and suspension deadline that keep a provider rejecting eth_getCode
// from being re-probed on every request. One field on EthereumRPC instead of four.
type multicall3Gate struct {
	state          atomic.Int32
	failures       atomic.Int32
	suspendedUntil atomic.Int64 // unix nanos; 0 means not suspended
	sf             singleflight.Group
}

// multicall3ContractAddress returns Multicall3AddressOverride when set (e.g. Tron's
// non-canonical deployment), otherwise the canonical const.
func (b *EthereumRPC) multicall3ContractAddress() string {
	if b.Multicall3AddressOverride != "" {
		return b.Multicall3AddressOverride
	}
	return multicall3Address
}

// multicall3MaxCalls returns the configured sub-calls per aggregate3, else the default.
// Lower it for a node that aborts a long constant call before it runs out of gas.
func (b *EthereumRPC) multicall3MaxCalls() int {
	if b.ChainConfig != nil && b.ChainConfig.Multicall3MaxCalls > 0 {
		return b.ChainConfig.Multicall3MaxCalls
	}
	return multicall3MaxCallsPerAggregate
}

// EthereumTypeMulticallAggregate3 issues an aggregate3 batch as one eth_call,
// observing all sub-calls at the same block (pinned to blockNumber, or
// "latest" if nil). The first call probes deployment with one eth_getCode;
// the deterministic result is cached.
func (b *EthereumRPC) EthereumTypeMulticallAggregate3(calls []bchain.EthereumMulticallCall, blockNumber *big.Int) ([]bchain.EthereumMulticallResult, error) {
	if len(calls) == 0 {
		return nil, nil
	}
	deployed, err := b.probeMulticall3()
	if err != nil {
		// Transient probe failure — surface as-is so callers can retry rather
		// than treat the chain as permanently unsupported.
		return nil, fmt.Errorf("multicall3 probe: %w", err)
	}
	if !deployed {
		return nil, errMulticall3NotDeployed
	}
	encoded, err := encodeAggregate3(calls)
	if err != nil {
		return nil, fmt.Errorf("multicall3 encode: %w", err)
	}
	// coalescing metrics: one physical request carrying len(calls) reads
	b.observeEthCallBatch(len(calls))
	b.observeEthCallMulticallRequest()
	resp, err := b.ethCallAtBlock(encoded, b.multicall3ContractAddress(), "", blockNumber, "multicall", len(calls))
	if err != nil {
		return nil, err
	}
	results, err := decodeAggregate3Result(resp, len(calls))
	if err != nil {
		return nil, err
	}
	return results, nil
}

// probeMulticall3 reports whether Multicall3 is deployed at the probed address
// (canonical, or a per-chain override). Three outcomes:
//
//   - (true, nil)  — deployed; deterministic, cached for the process lifetime.
//   - (false, nil) — not deployed; deterministic, cached. Also returned while
//     probing is suspended after repeated transient failures.
//   - (false, err) — transient probe failure (RPC down, timeout). NOT cached;
//     the next call retries. Returned to callers so they can distinguish
//     "this chain has no Multicall3" from "RPC is having a moment."
//
// Concurrent probers are collapsed via singleflight, so a thundering herd
// at process start performs at most one eth_getCode.
func (b *EthereumRPC) probeMulticall3() (bool, error) {
	switch b.multicall3.state.Load() {
	case multicall3Deployed:
		return true, nil
	case multicall3NotDeployed:
		return false, nil
	}

	// while suspended, report unavailable (not an error); retried after the window
	if until := b.multicall3.suspendedUntil.Load(); until != 0 && time.Now().UnixNano() < until {
		return false, nil
	}

	type probeResult struct {
		deployed bool
		err      error
	}
	v, _, _ := b.multicall3.sf.Do("multicall3", func() (interface{}, error) {
		// Re-check: a peer may have completed before we entered Do.
		if state := b.multicall3.state.Load(); state != multicall3Unprobed {
			return probeResult{deployed: state == multicall3Deployed}, nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
		defer cancel()
		addr := b.multicall3ContractAddress()
		// Probe at "latest": below the deployment height aggregate3 returns "0x", which
		// decodes as an error and falls back safely.
		var code string
		if err := b.RPC.CallContext(ctx, &code, "eth_getCode", addr, "latest"); err != nil {
			// repeated failures: pause probing for a while instead of disabling multicall for good
			if b.multicall3.failures.Add(1) >= multicall3MaxProbeFailures {
				glog.Warningf("multicall3 probe at %s failed %d times; suspending probing for %s, using JSON-RPC batch: %v", addr, multicall3MaxProbeFailures, multicall3ProbeSuspendInterval, err)
				b.multicall3.suspendedUntil.Store(time.Now().Add(multicall3ProbeSuspendInterval).UnixNano())
				b.multicall3.failures.Store(0)
				return probeResult{}, nil
			}
			glog.Warningf("multicall3 probe at %s failed: %v (will retry on next call)", addr, err)
			return probeResult{err: err}, nil
		}
		// "0x" means no code at the address.
		if len(code) <= 2 {
			glog.Infof("multicall3 not deployed at %s on this chain; multicall enrichments will be disabled", addr)
			b.multicall3.state.Store(multicall3NotDeployed)
			return probeResult{}, nil
		}
		b.multicall3.state.Store(multicall3Deployed)
		return probeResult{deployed: true}, nil
	})
	r := v.(probeResult)
	return r.deployed, r.err
}

// encodeAggregate3 hand-rolls the ABI encoding for aggregate3((address,bool,bytes)[]).
// Layout (after the 4-byte selector):
//
//	0x20                                <- offset to outer array
//	N                                   <- array length
//	headOff[0..N-1]                     <- N words; offsets to each tuple, relative to start of heads
//	tail[0..N-1]                        <- per-tuple encoding
//
// Each tuple `(address,bool,bytes)` is itself dynamic and encodes as:
//
//	address (32 bytes, left-padded)
//	bool    (32 bytes)
//	0x60                                <- offset to bytes data within the tuple
//	bytesLen (32 bytes)
//	bytesData (padded up to 32-byte boundary)
func encodeAggregate3(calls []bchain.EthereumMulticallCall) (string, error) {
	type tuple struct {
		target  []byte // 20 bytes
		bool32  byte   // 0 or 1
		payload []byte
	}
	tuples := make([]tuple, len(calls))
	for i, c := range calls {
		addr, err := hexToAddressBytes(c.Target)
		if err != nil {
			return "", fmt.Errorf("call %d target: %w", i, err)
		}
		payload, err := hexToBytes(c.CallData)
		if err != nil {
			return "", fmt.Errorf("call %d callData: %w", i, err)
		}
		tuples[i].target = addr
		if c.AllowFailure {
			tuples[i].bool32 = 1
		}
		tuples[i].payload = payload
	}

	// Compute offset words first (relative to the start of the heads block). Each tuple encodes
	// as 3 head words (address, bool, bytes-offset) + 1 length word + padded data.
	n := len(tuples)
	offsets := make([]int, n)
	cursor := n * 32
	for i, t := range tuples {
		offsets[i] = cursor
		cursor += 32*4 + paddedLen(len(t.payload))
	}

	// Total payload size after the selector: 0x20 word + length word + heads + tails.
	totalAfterSelector := 32 + 32 + cursor
	out := make([]byte, 0, 4+totalAfterSelector)

	out = append(out, multicall3Aggregate3Selector...)
	// Outer offset: array starts immediately after this word.
	out = appendWord(out, 0x20)
	// Array length.
	out = appendWord(out, uint64(n))
	// Heads.
	for _, off := range offsets {
		out = appendWord(out, uint64(off))
	}
	// Tails.
	for _, t := range tuples {
		var word [32]byte
		copy(word[12:], t.target)
		out = append(out, word[:]...) // address
		word = [32]byte{}
		word[31] = t.bool32
		out = append(out, word[:]...) // bool
		// offset to bytes within tuple = 0x60 (3 head words)
		out = appendWord(out, 0x60)
		out = appendWord(out, uint64(len(t.payload)))
		// bytes data, zero-padded up to the word boundary
		out = append(out, t.payload...)
		if pad := paddedLen(len(t.payload)) - len(t.payload); pad > 0 {
			out = append(out, make([]byte, pad)...)
		}
	}

	return hexutil.Encode(out), nil
}

// decodeAggregate3Result inverts encodeAggregate3's return encoding for (bool,bytes)[].
// Layout:
//
//	0x20                                <- outer offset to array
//	N                                   <- array length
//	headOff[0..N-1]                     <- offsets to tuples, relative to heads start
//	tail[0..N-1]                        <- per-tuple (bool, bytes-offset, bytesLen, bytesData)
//
// expectedCalls is the number of sub-calls sent; a response claiming any other count is
// rejected before any returndata is materialized.
func decodeAggregate3Result(data string, expectedCalls int) ([]bchain.EthereumMulticallResult, error) {
	raw, err := hexToBytes(data)
	if err != nil {
		return nil, fmt.Errorf("decode hex: %w", err)
	}
	if len(raw) < 64 {
		return nil, fmt.Errorf("multicall3 response too short: %d bytes", len(raw))
	}
	// Top-level offset word; in well-formed responses always 0x20.
	if v, ok := wordAsIndex(raw, 0); !ok || v != 0x20 {
		return nil, fmt.Errorf("multicall3 unexpected outer offset")
	}
	headsStart := 64
	n, ok := wordAsIndex(raw, 32)
	if !ok {
		return nil, fmt.Errorf("multicall3 array length out of range")
	}
	// Checked before the tail loop: a response is otherwise free to claim len(raw)/32
	// elements and have every one of them decode a large tuple.
	if n != expectedCalls {
		return nil, fmt.Errorf("multicall3 returned %d results for %d calls", n, expectedCalls)
	}
	if len(raw) < headsStart+n*32 {
		return nil, fmt.Errorf("multicall3 response truncated in heads")
	}

	// Head offsets may alias, so the per-element bounds below do not limit the total.
	// A canonical response lays the tuples out disjointly, so their returndata can never
	// sum past the response itself; that keeps the decode linear in len(raw).
	decoded := 0
	results := make([]bchain.EthereumMulticallResult, n)
	for i := 0; i < n; i++ {
		offset, ok := wordAsIndex(raw, headsStart+i*32)
		if !ok {
			return nil, fmt.Errorf("multicall3 element %d offset out of range", i)
		}
		tupleStart := headsStart + offset
		// Tuple shape: bool (32) | bytesOffsetInTuple (32) | bytesLen (32) | bytesData...
		if len(raw) < tupleStart+96 {
			return nil, fmt.Errorf("multicall3 element %d truncated", i)
		}
		// success is rightmost byte of the bool word.
		results[i].Success = raw[tupleStart+31] == 1

		bytesOffset, ok := wordAsIndex(raw, tupleStart+32)
		if !ok {
			return nil, fmt.Errorf("multicall3 element %d bytes offset out of range", i)
		}
		bytesPos := tupleStart + bytesOffset
		if len(raw) < bytesPos+32 {
			return nil, fmt.Errorf("multicall3 element %d truncated at bytes length", i)
		}
		bl, ok := wordAsIndex(raw, bytesPos)
		if !ok {
			return nil, fmt.Errorf("multicall3 element %d bytes length out of range", i)
		}
		if len(raw) < bytesPos+32+bl {
			return nil, fmt.Errorf("multicall3 element %d truncated at bytes data", i)
		}
		decoded += bl
		if decoded > len(raw) {
			return nil, fmt.Errorf("multicall3 returndata exceeds the response at element %d", i)
		}
		results[i].Data = hexutil.Encode(raw[bytesPos+32 : bytesPos+32+bl])
	}
	return results, nil
}

// hexToBytes accepts either a "0x"-prefixed or bare hex string and returns its bytes.
// Empty input is allowed and yields an empty slice (callers may pass empty calldata).
func hexToBytes(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	if has0xPrefix(s) {
		s = s[2:]
	}
	if s == "" {
		return nil, nil
	}
	return hex.DecodeString(s)
}

// hexToAddressBytes decodes an EIP-55 / lowercase hex address into 20 bytes.
func hexToAddressBytes(s string) ([]byte, error) {
	addr, err := hexutil.Decode(s)
	if err != nil {
		return nil, err
	}
	if len(addr) != 20 {
		return nil, fmt.Errorf("address must be 20 bytes, got %d", len(addr))
	}
	return addr, nil
}

// appendWord appends v as a 32-byte big-endian ABI word.
func appendWord(out []byte, v uint64) []byte {
	var word [32]byte
	binary.BigEndian.PutUint64(word[24:], v)
	return append(out, word[:]...)
}

// wordAsIndex reads the ABI word at offset as a length or offset into buf. Anything that
// cannot index buf is rejected, so callers never narrow a word that would wrap negative.
func wordAsIndex(buf []byte, offset int) (int, bool) {
	word := buf[offset : offset+32]
	for _, c := range word[:24] {
		if c != 0 {
			return 0, false
		}
	}
	v := binary.BigEndian.Uint64(word[24:])
	if v > uint64(len(buf)) {
		return 0, false
	}
	return int(v), true
}

// paddedLen rounds n up to the next 32-byte word boundary.
func paddedLen(n int) int {
	return (n + 31) &^ 31
}
