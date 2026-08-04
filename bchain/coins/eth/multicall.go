package eth

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
)

// Canonical Multicall3 deployment, identical address on every major EVM chain.
// See https://github.com/mds1/multicall.
const multicall3Address = "0xcA11bde05977b3631167028862bE2a173976CA11"

// Function selector for aggregate3((address,bool,bytes)[]).
// Verified: keccak256("aggregate3((address,bool,bytes)[])")[:4].
const multicall3Aggregate3Signature = "0x82ad56cb"

// multicall3Probe states; Unprobed is the zero value.
const (
	multicall3Unprobed    int32 = 0
	multicall3Deployed    int32 = 1
	multicall3NotDeployed int32 = 2
)

// multicall3MaxProbeFailures suspends probing for multicall3ProbeSuspendInterval after this many
// consecutive transient eth_getCode errors. The batch path still serves balances meanwhile.
const multicall3MaxProbeFailures = 5

// multicall3ProbeSuspendInterval pauses probing after multicall3MaxProbeFailures consecutive
// transient failures, so a provider that permanently restricts eth_getCode does not cost an
// extra probe on every balances request, while a node that is merely down (e.g. at startup)
// does not disable multicall for the process lifetime — probing resumes after the window.
const multicall3ProbeSuspendInterval = 10 * time.Minute

// errMulticall3NotDeployed is returned on chains where Multicall3 is not deployed
// at the probed address (canonical, or a per-chain override); cached for the process lifetime.
var errMulticall3NotDeployed = errors.New("multicall3 not deployed on this chain")

// multicall3ContractAddress returns Multicall3AddressOverride when set (e.g. Tron's
// non-canonical deployment), otherwise the canonical const.
func (b *EthereumRPC) multicall3ContractAddress() string {
	if b.Multicall3AddressOverride != "" {
		return b.Multicall3AddressOverride
	}
	return multicall3Address
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
	// Instrument like the JSON-RPC batch path — one request per sub-call read, plus the
	// coalescing histogram — but under mode="multicall", so per-token read volume and
	// batch-size percentiles stay continuous when a chain moves batches onto aggregate3.
	// The physical-request counter records the single metered eth_call carrying them all,
	// making the sub-calls-per-request reduction visible.
	b.observeEthCall("multicall", len(calls))
	b.observeEthCallBatch(len(calls))
	b.observeEthCallMulticallRequest()
	resp, err := b.ethCallAtBlock(encoded, b.multicall3ContractAddress(), "", blockNumber, "")
	if err != nil {
		b.observeEthCallError("multicall", "rpc")
		return nil, err
	}
	results, err := decodeAggregate3Result(resp)
	if err != nil {
		return nil, err
	}
	if len(results) != len(calls) {
		return nil, fmt.Errorf("multicall3 returned %d results for %d calls", len(results), len(calls))
	}
	return results, nil
}

// probeMulticall3 reports whether Multicall3 is deployed at the probed address
// (canonical, or a per-chain override). Three outcomes:
//
//   - (true, nil)  — deployed; deterministic, cached for the process lifetime.
//   - (false, nil) — not deployed; deterministic, cached. Also returned while
//     probing is suspended after multicall3MaxProbeFailures consecutive
//     transient failures; probing resumes once the suspension window elapses.
//   - (false, err) — transient probe failure (RPC down, timeout). NOT cached;
//     the next call retries. Returned to callers so they can distinguish
//     "this chain has no Multicall3" from "RPC is having a moment."
//
// Concurrent probers are collapsed via singleflight, so a thundering herd
// at process start performs at most one eth_getCode.
func (b *EthereumRPC) probeMulticall3() (bool, error) {
	// The probe is set exactly once per process to either multicall3Deployed
	// or multicall3NotDeployed and is never cleared back to the zero value,
	// so any other observed state is multicall3Unprobed and falls through to
	// the singleflight below. The Do callback re-checks the state under
	// singleflight, so no correctness depends on the invariant above.
	switch b.multicall3Probe.Load() {
	case multicall3Deployed:
		return true, nil
	case multicall3NotDeployed:
		return false, nil
	}

	// Probing is suspended after repeated transient failures; report unavailable
	// (not an error) until the window elapses, then the next call retries.
	if until := b.multicall3ProbeSuspendedUntil.Load(); until != 0 && time.Now().UnixNano() < until {
		return false, nil
	}

	type probeResult struct {
		deployed bool
		err      error
	}
	v, _, _ := b.multicall3ProbeSF.Do("multicall3", func() (interface{}, error) {
		// Re-check: a peer may have completed before we entered Do.
		if state := b.multicall3Probe.Load(); state != multicall3Unprobed {
			return probeResult{deployed: state == multicall3Deployed}, nil
		}

		ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
		defer cancel()
		// probe the same address aggregate3 will call (override-aware)
		addr := b.multicall3ContractAddress()
		// Probe at "latest": deployment is checked once at the chain tip. A caller requesting a
		// historical block below Multicall3's deployment finds a codeless address there, so
		// aggregate3 returns "0x" and decodes as an error, safely falling back to the batch path.
		var code string
		if err := b.RPC.CallContext(ctx, &code, "eth_getCode", addr, "latest"); err != nil {
			// Suspend probing after repeated transient failures so a provider that
			// restricts eth_getCode does not incur an extra probe on every request,
			// while a temporarily-down node does not disable multicall forever.
			if b.multicall3ProbeFailures.Add(1) >= multicall3MaxProbeFailures {
				glog.Warningf("multicall3 probe at %s failed %d times; suspending probing for %s, using JSON-RPC batch: %v", addr, multicall3MaxProbeFailures, multicall3ProbeSuspendInterval, err)
				b.multicall3ProbeSuspendedUntil.Store(time.Now().Add(multicall3ProbeSuspendInterval).UnixNano())
				b.multicall3ProbeFailures.Store(0)
				return probeResult{}, nil
			}
			glog.Warningf("multicall3 probe at %s failed: %v (will retry on next call)", addr, err)
			return probeResult{err: err}, nil
		}
		// "0x" means no code at the address.
		if len(code) <= 2 {
			glog.Infof("multicall3 not deployed at %s on this chain; multicall enrichments will be disabled", addr)
			b.multicall3Probe.Store(multicall3NotDeployed)
			return probeResult{}, nil
		}
		b.multicall3Probe.Store(multicall3Deployed)
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

	// Per-tuple encoded size: 3 head words (address, bool, bytes-offset) + 1 length word + padded data.
	tupleSize := func(t tuple) int {
		return 32*4 + paddedLen(len(t.payload))
	}

	// Compute offset words first (relative to the start of the heads block).
	n := len(tuples)
	headBytes := n * 32
	offsets := make([]int, n)
	cursor := headBytes
	for i, t := range tuples {
		offsets[i] = cursor
		cursor += tupleSize(t)
	}

	// Total payload size after the selector: 0x20 word + length word + heads + tails.
	totalAfterSelector := 32 + 32 + cursor
	out := make([]byte, 0, 4+totalAfterSelector)

	// Selector.
	sel, err := hexToBytes(multicall3Aggregate3Signature)
	if err != nil {
		return "", err
	}
	out = append(out, sel...)
	// Outer offset: array starts immediately after this word.
	out = append(out, padLeftWord(big.NewInt(0x20))...)
	// Array length.
	out = append(out, padLeftWord(big.NewInt(int64(n)))...)
	// Heads.
	for _, off := range offsets {
		out = append(out, padLeftWord(big.NewInt(int64(off)))...)
	}
	// Tails.
	for _, t := range tuples {
		// address
		word := make([]byte, 32)
		copy(word[12:], t.target)
		out = append(out, word...)
		// bool
		word = make([]byte, 32)
		word[31] = t.bool32
		out = append(out, word...)
		// offset to bytes within tuple = 0x60 (3 head words)
		out = append(out, padLeftWord(big.NewInt(0x60))...)
		// bytes length
		out = append(out, padLeftWord(big.NewInt(int64(len(t.payload))))...)
		// bytes data, padded to 32 bytes
		padded := make([]byte, paddedLen(len(t.payload)))
		copy(padded, t.payload)
		out = append(out, padded...)
	}

	return "0x" + hex.EncodeToString(out), nil
}

// decodeAggregate3Result inverts encodeAggregate3's return encoding for (bool,bytes)[].
// Layout:
//
//	0x20                                <- outer offset to array
//	N                                   <- array length
//	headOff[0..N-1]                     <- offsets to tuples, relative to heads start
//	tail[0..N-1]                        <- per-tuple (bool, bytes-offset, bytesLen, bytesData)
func decodeAggregate3Result(data string) ([]bchain.EthereumMulticallResult, error) {
	raw, err := hexToBytes(data)
	if err != nil {
		return nil, fmt.Errorf("decode hex: %w", err)
	}
	if len(raw) < 64 {
		return nil, fmt.Errorf("multicall3 response too short: %d bytes", len(raw))
	}
	// Top-level offset word; in well-formed responses always 0x20.
	if v := bigUintAt(raw, 0); v.Cmp(big.NewInt(0x20)) != 0 {
		return nil, fmt.Errorf("multicall3 unexpected outer offset: %s", v)
	}
	headsStart := 64
	// Every offset/length word below is bounded by len(raw) before narrowing to int:
	// anything larger cannot address data inside the response (n elements need n*32
	// bytes of heads, so len(raw) is a safe coarse bound for the count too), and a
	// word ≥ 2^63 would wrap negative in int, defeating the later bounds checks.
	length := bigUintAt(raw, 32)
	if !length.IsUint64() || length.Uint64() > uint64(len(raw)) {
		return nil, fmt.Errorf("multicall3 array length out of range")
	}
	n := int(length.Uint64())
	if n == 0 {
		// Degenerate: encoder short-circuits empty input upstream, so a
		// well-formed n==0 response can only arise from a malformed batch
		// or unusual node behavior. nil matches encodeAggregate3's empty
		// case and the caller's nil-means-no-results contract.
		return nil, nil
	}
	if len(raw) < headsStart+n*32 {
		return nil, fmt.Errorf("multicall3 response truncated in heads")
	}

	results := make([]bchain.EthereumMulticallResult, n)
	for i := 0; i < n; i++ {
		offset := bigUintAt(raw, headsStart+i*32)
		if !offset.IsUint64() || offset.Uint64() > uint64(len(raw)) {
			return nil, fmt.Errorf("multicall3 element %d offset out of range", i)
		}
		tupleStart := headsStart + int(offset.Uint64())
		// Tuple shape: bool (32) | bytesOffsetInTuple (32) | bytesLen (32) | bytesData...
		if len(raw) < tupleStart+96 {
			return nil, fmt.Errorf("multicall3 element %d truncated", i)
		}
		successWord := raw[tupleStart : tupleStart+32]
		// success is rightmost byte of the bool word.
		results[i].Success = successWord[31] == 1

		bytesOffset := bigUintAt(raw, tupleStart+32)
		if !bytesOffset.IsUint64() || bytesOffset.Uint64() > uint64(len(raw)) {
			return nil, fmt.Errorf("multicall3 element %d bytes offset out of range", i)
		}
		bytesPos := tupleStart + int(bytesOffset.Uint64())
		if len(raw) < bytesPos+32 {
			return nil, fmt.Errorf("multicall3 element %d truncated at bytes length", i)
		}
		bytesLen := bigUintAt(raw, bytesPos)
		if !bytesLen.IsUint64() || bytesLen.Uint64() > uint64(len(raw)) {
			return nil, fmt.Errorf("multicall3 element %d bytes length out of range", i)
		}
		bl := int(bytesLen.Uint64())
		if len(raw) < bytesPos+32+bl {
			return nil, fmt.Errorf("multicall3 element %d truncated at bytes data", i)
		}
		results[i].Data = "0x" + hex.EncodeToString(raw[bytesPos+32:bytesPos+32+bl])
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

func padLeftWord(v *big.Int) []byte {
	word := make([]byte, 32)
	v.FillBytes(word)
	return word
}

func bigUintAt(buf []byte, offset int) *big.Int {
	return new(big.Int).SetBytes(buf[offset : offset+32])
}

// paddedLen rounds n up to the next 32-byte word boundary.
func paddedLen(n int) int {
	if n == 0 {
		return 0
	}
	return (n + 31) &^ 31
}
