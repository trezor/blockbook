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
		if d+64 > len(data) || d < 0 {
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
			de := d + (count << 1)
			if de > len(data) || de < 0 {
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
func ParseInputData(signatures *[]bchain.FourByteSignature, data string) *bchain.EthereumParsedInputData {
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

// getEnsRecord processes transaction log entry and tries to parse ENS record from it
func getEnsRecord(l *rpcLogWithTxHash) *bchain.AddressAliasRecord {
	if len(l.Topics) == 3 && l.Topics[0] == nameRegisteredEventSignature && len(l.Data) >= 322 {
		address, err := addressFromPaddedHex(l.Topics[2])
		if err != nil {
			return nil
		}
		c, err := strconv.ParseInt(l.Data[194:194+64], 16, 64)
		if err != nil {
			return nil
		}
		de := 194 + 64 + (int(c) << 1)
		if de > len(l.Data) || de < 0 {
			return nil
		}
		b, err := hex.DecodeString(l.Data[194+64 : de])
		if err != nil {
			return nil
		}
		return &bchain.AddressAliasRecord{Address: address, Name: string(b)}
	}
	return nil
}
