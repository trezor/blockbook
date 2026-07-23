package eth

import (
	"bytes"
	"encoding/hex"
	"math/big"
	"runtime/debug"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
)

func parseSimpleNumericProperty(data string) *big.Int {
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
	return nil
}

func parseSimpleStringProperty(data string) string {
	if has0xPrefix(data) {
		data = data[2:]
	}
	if len(data) > 128 {
		n := parseSimpleNumericProperty(data[64:128])
		if n != nil {
			l := n.Int64()
			if l > 0 && int(l) <= ((len(data)-128)>>1) {
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
		i := min(bytes.Index(b, []byte{0}), 32)
		if i > 0 {
			b = b[:i]
		}
		if utf8.Valid(b) {
			return string(b)
		}
	}
	return ""
}

func decamel(s string) string {
	var b bytes.Buffer
	splittable := false
	for i, v := range s {
		if i == 0 {
			b.WriteRune(unicode.ToUpper(v))
		} else {
			if splittable && unicode.IsUpper(v) {
				b.WriteByte(' ')
			}
			b.WriteRune(v)
			// special handling of ETH to be able to convert "addETHToContract" to "Add ETH To Contract"
			splittable = unicode.IsLower(v) || unicode.IsNumber(v) || (i >= 2 && s[i-2:i+1] == "ETH")
		}
	}
	return b.String()
}

func GetSignatureFromData(data string) uint32 {
	if has0xPrefix(data) {
		data = data[2:]
	}
	if len(data) < 8 {
		return 0
	}
	sig, err := strconv.ParseUint(data[:8], 16, 32)
	if err != nil {
		return 0
	}
	return uint32(sig)
}

const ErrorTy byte = 255

func processParam(data string, index int, dataOffset int, t *abi.Type, processed []bool) ([]string, int, bool) {
	var retval []string
	d := index << 6
	if d+64 > len(data) {
		return nil, 0, false
	}
	block := data[d : d+64]
	switch t.T {
	// static types
	case abi.IntTy, abi.UintTy, abi.BoolTy:
		var n big.Int
		_, ok := n.SetString(block, 16)
		if !ok {
			return nil, 0, false
		}
		if t.T == abi.BoolTy {
			if n.Int64() != 0 {
				retval = []string{"true"}
			} else {
				retval = []string{"false"}
			}
		} else {
			retval = []string{n.String()}
		}
		processed[index] = true
		index++
	case abi.AddressTy:
		b, err := hex.DecodeString(block[24:])
		if err != nil {
			return nil, 0, false
		}
		retval = []string{EIP55Address(b)}
		processed[index] = true
		index++
	case abi.FixedBytesTy:
		retval = []string{"0x" + block[:t.Size<<1]}
		processed[index] = true
		index++
	case abi.ArrayTy:
		for i := 0; i < t.Size; i++ {
			var r []string
			var ok bool
			r, index, ok = processParam(data, index, dataOffset, t.Elem, processed)
			if !ok {
				return nil, 0, false
			}
			retval = append(retval, r...)
		}
	// dynamic types
	case abi.StringTy, abi.BytesTy, abi.SliceTy:
		// get offset of dynamic type
		offset, err := strconv.ParseInt(block, 16, 64)
		if err != nil {
			return nil, 0, false
		}
		processed[index] = true
		index++
		offset <<= 1
		d = int(offset) + dataOffset
		dynIndex := d >> 6
		if d+64 > len(data) || d < 0 || d+64 < d {
			return nil, 0, false
		}
		// get element count of dynamic type
		c, err := strconv.ParseInt(data[d:d+64], 16, 64)
		if err != nil {
			return nil, 0, false
		}
		count := int(c)
		processed[dynIndex] = true
		dynIndex++
		if t.T == abi.StringTy || t.T == abi.BytesTy {
			d += 64
			// count<<1 can overflow and wrap below d (same class as the
			// getEnsRecord bug); the lower bound must be the slice start d, not 0,
			// so that d <= de <= len(data) and data[d:de] below can never panic.
			// count==0 gives de==d, which is still accepted.
			de := d + (count << 1)
			if de > len(data) || de < d {
				return nil, 0, false
			}
			if count == 0 {
				retval = []string{""}
			} else {
				block = data[d:de]
				if t.T == abi.StringTy {
					b, err := hex.DecodeString(block)
					if err != nil {
						return nil, 0, false
					}
					retval = []string{string(b)}
				} else {
					retval = []string{"0x" + block}
				}
				count = ((count - 1) >> 5) + 1
				for i := 0; i < count; i++ {
					// A dynamic field whose declared length runs past the data
					// (malformed or non-32-byte-aligned input) would index beyond
					// processed, which is sized len(data)/64; treat it as a
					// non-matching signature instead of panicking.
					if dynIndex >= len(processed) {
						return nil, 0, false
					}
					processed[dynIndex] = true
					dynIndex++
				}
			}
		} else {
			newOffset := dataOffset + dynIndex<<6
			for i := 0; i < count; i++ {
				var r []string
				var ok bool
				r, dynIndex, ok = processParam(data, dynIndex, newOffset, t.Elem, processed)
				if !ok {
					return nil, 0, false
				}
				retval = append(retval, r...)
			}
		}
	// types not processed
	case abi.HashTy, abi.FixedPointTy, abi.FunctionTy, abi.TupleTy:
		fallthrough
	default:
		return nil, 0, false
	}
	return retval, index, true
}

func tryParseParams(data string, params []string, parsedParams []abi.Type) []bchain.EthereumParsedInputParam {
	processed := make([]bool, len(data)/64)
	parsed := make([]bchain.EthereumParsedInputParam, len(params))
	index := 0
	var values []string
	var ok bool
	for i := range params {
		t := &parsedParams[i]
		values, index, ok = processParam(data, index, 0, t, processed)
		if !ok {
			return nil
		}
		parsed[i] = bchain.EthereumParsedInputParam{Type: params[i], Values: values}
	}
	// all data must be processed, otherwise wrong signature
	for _, p := range processed {
		if !p {
			return nil
		}
	}
	return parsed
}

// ParseInputData tries to parse transaction input data from known FourByteSignatures
// as there may be multiple signatures for the same four bytes, it tries to match the input to the known parameters
// it does not parse tuples for now
func (p *EthereumParser) ParseInputData(signatures *[]bchain.FourByteSignature, data string) *bchain.EthereumParsedInputData {
	if len(data) <= 2 { // data is empty or 0x
		return &bchain.EthereumParsedInputData{Name: "Transfer"}
	}
	if len(data) < 10 {
		return nil
	}
	parsed := bchain.EthereumParsedInputData{
		MethodId: data[:10],
	}
	defer func() {
		if r := recover(); r != nil {
			glog.Error("ParseInputData recovered from panic: ", r, ", ", data, ",signatures ", signatures)
			debug.PrintStack()
		}
	}()
	if signatures != nil {
		data = data[10:]
		for i := range *signatures {
			s := &(*signatures)[i]
			// if not yet done, set DecamelName and Function and parse parameter types from string to abi.Type
			// the signatures are stored in cache
			if s.DecamelName == "" {
				s.DecamelName = decamel(s.Name)
				s.Function = s.Name + "(" + strings.Join(s.Parameters, ", ") + ")"
				s.ParsedParameters = make([]abi.Type, len(s.Parameters))
				for j := range s.Parameters {
					var t abi.Type
					if len(s.Parameters[j]) > 0 && s.Parameters[j][0] == '(' {
						// Tuple type is not supported for now
						t = abi.Type{T: abi.TupleTy}
					} else {
						var err error
						t, err = abi.NewType(s.Parameters[j], "", nil)
						if err != nil {
							t = abi.Type{T: ErrorTy}
						}
					}
					s.ParsedParameters[j] = t
				}
			}
			parsedParams := tryParseParams(data, s.Parameters, s.ParsedParameters)
			if parsedParams != nil {
				parsed.Name = s.DecamelName
				parsed.Function = s.Function
				parsed.Params = parsedParams
				break
			}
		}
	}
	return &parsed
}

// maxEnsAliasNameLen caps the decoded ENS name length (in bytes) accepted as an
// address alias, bounding storage and display abuse from an oversized log.
const maxEnsAliasNameLen = 256

// validEnsAliasName rejects names unsafe to store or render as an address alias:
// empty, over the length cap, not valid UTF-8, or containing control characters
// (NUL, newline, ANSI escapes) or bidirectional overrides (e.g. U+202E) that could
// spoof or corrupt the displayed alias. Bidi_Control is used rather than the whole
// Cf category so legitimate emoji names using the zero-width joiner still pass.
func validEnsAliasName(name string) bool {
	if len(name) == 0 || len(name) > maxEnsAliasNameLen {
		return false
	}
	if !utf8.ValidString(name) {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) {
			return false
		}
	}
	return true
}

// getEnsRecord processes a transaction log entry and tries to parse an ENS
// address alias (a NameRegistered event) from it. Both the original 5-argument
// event (old ETHRegistrarController) and the newer 6-argument event that split
// cost into baseCost + premium are recognized. registrars is the set of contract
// addresses (lower-cased, 0x-prefixed) trusted to emit these events: a log whose
// emitter (l.Address) is not in the set is rejected, so an arbitrary contract
// cannot forge an ENS alias for any address. An empty set therefore trusts no
// one and records nothing; the special entry "*" accepts any emitter (legacy
// behavior, for chains with a different name service).
func getEnsRecord(l *rpcLogWithTxHash, registrars map[string]struct{}) *bchain.AddressAliasRecord {
	if len(l.Topics) != 3 {
		return nil
	}
	if l.Topics[0] != nameRegisteredEventSignature && l.Topics[0] != nameRegisteredWithPremiumEventSignature {
		return nil
	}
	if _, acceptAny := registrars[ensRegistrarWildcard]; !acceptAny {
		emitter := strings.ToLower(l.Address)
		if !strings.HasPrefix(emitter, "0x") {
			emitter = "0x" + emitter
		}
		if _, ok := registrars[emitter]; !ok {
			return nil
		}
	}
	address, err := addressFromPaddedHex(l.Topics[2])
	if err != nil {
		return nil
	}
	name, ok := parseEnsNameFromLogData(l.Data)
	if !ok || !validEnsAliasName(name) {
		return nil
	}
	return &bchain.AddressAliasRecord{Address: address, Name: name}
}

// readHexWordAt reads one 32-byte EVM word starting at hexStart (an index into
// the 0x-prefixed hex data string) as an int64, reporting false if the word is
// out of range or does not fit in an int64.
func readHexWordAt(data string, hexStart int) (int64, bool) {
	if hexStart < 2 || hexStart+evmWordHex > len(data) {
		return 0, false
	}
	v, err := strconv.ParseInt(data[hexStart:hexStart+evmWordHex], 16, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseEnsNameFromLogData extracts the ABI-encoded `string name` (the first
// non-indexed parameter of every NameRegistered variant) from a log's data. The
// leading word holds the byte offset to the name's length word; the name bytes
// follow. Reading via the offset - rather than a hardcoded position - keeps this
// layout-agnostic, so the extra baseCost/premium word of the newer event is
// handled without special-casing. Every index derives from attacker-controlled
// input, so all bounds are checked and no slice expression can panic.
func parseEnsNameFromLogData(data string) (string, bool) {
	offset, ok := readHexWordAt(data, 2) // leading word: byte offset to the name length word
	if !ok || offset < 0 || offset > int64(len(data)) {
		return "", false
	}
	lenStart := 2 + int(offset)*2
	length, ok := readHexWordAt(data, lenStart)
	// Cap the length before using it as a slice bound: it both rejects absurd,
	// overflow-prone values and enforces the alias length limit early.
	if !ok || length < 0 || length > maxEnsAliasNameLen {
		return "", false
	}
	nameStart := lenStart + evmWordHex
	nameEnd := nameStart + int(length)*2
	if nameEnd > len(data) {
		return "", false
	}
	b, err := hex.DecodeString(data[nameStart:nameEnd])
	if err != nil {
		return "", false
	}
	return string(b), true
}
