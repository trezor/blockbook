package tron

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/decred/base58"
	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

// TronTypeAddressDescriptorLen - the AddressDescriptor of TronType has fixed length
const TronTypeAddressDescriptorLen = 20

// TronAddressLen - length of Tron Base58 address
const TronAddressLen = 34

// TronAmountDecimalPoint defines number of decimal points in Tron amounts
// base unit is 'SUN', 1 TRX = 1,000,000 SUN
const TronAmountDecimalPoint = 6

// TronParser handle
type TronParser struct {
	*eth.EthereumParser
}

// NewTronParser returns a new instance of TronParser
func NewTronParser(b int, addressAliases bool) *TronParser {
	ethParser := eth.NewEthereumParser(b, addressAliases)
	ethParser.AmountDecimalPoint = TronAmountDecimalPoint
	ethParser.FormatAddressFunc = ToTronAddressFromAddress
	ethParser.FromDescToAddressFunc = ToTronAddressFromDesc
	ethParser.EnsSuffix = ".trx"
	return &TronParser{
		EthereumParser: ethParser,
	}
}

// GetAddrDescFromVout returns internal address representation of given transaction output
func (p *TronParser) GetAddrDescFromVout(output *bchain.Vout) (bchain.AddressDescriptor, error) {
	if len(output.ScriptPubKey.Addresses) != 1 {
		return nil, bchain.ErrAddressMissing
	}
	return p.GetAddrDescFromAddress(output.ScriptPubKey.Addresses[0])
}

func has0xPrefix(s string) bool {
	return len(s) >= 2 && s[0] == '0' && (s[1]|32) == 'x'
}

func (p *TronParser) GetAddrDescFromAddress(address string) (bchain.AddressDescriptor, error) {
	if has0xPrefix(address) {
		address = address[2:]
	}

	if len(address) == TronAddressLen {
		decoded := base58.Decode(address)
		if len(decoded) != 25 || decoded[0] != 0x41 {
			return nil, errors.New("invalid Tron base58 address")
		}
		return decoded[1:21], nil
	} else if len(address) != TronTypeAddressDescriptorLen*2 {
		glog.Infof("Invalid Tron address length: got %d chars: %q", len(address), address)
		return nil, bchain.ErrAddressMissing
	}

	return hex.DecodeString(address)
}

// GetAddressesFromAddrDesc checks len and prefix and converts to base58
func (p *TronParser) GetAddressesFromAddrDesc(desc bchain.AddressDescriptor) ([]string, bool, error) {
	if len(desc) != TronTypeAddressDescriptorLen {
		return nil, false, bchain.ErrAddressMissing
	}

	return []string{ToTronAddressFromDesc(desc)}, true, nil
}

func ToTronAddressFromDesc(addrDesc bchain.AddressDescriptor) string {
	withPrefix := append([]byte{0x41}, addrDesc...)

	firstSHA := sha256.Sum256(withPrefix)
	secondSHA := sha256.Sum256(firstSHA[:])
	checksum := secondSHA[:4]

	fullAddress := append(withPrefix, checksum...)

	base58Addr := base58.Encode(fullAddress)

	return base58Addr
}

func ToTronAddressFromAddress(address string) string {
	if has0xPrefix(address) {
		address = address[2:]
	}
	b, err := hex.DecodeString(address)
	if err != nil {
		return address
	}
	return ToTronAddressFromDesc(b)
}

func (p *TronParser) FromTronAddressToHex(addr string) (string, error) {
	desc, err := p.GetAddrDescFromAddress(addr)
	if err != nil {
		return "", fmt.Errorf("failed to convert Tron address %q: %w", addr, err)
	}
	return "0x" + hex.EncodeToString(desc), nil
}

func (p *TronParser) ParseInputData(signatures *[]bchain.FourByteSignature, data string) *bchain.EthereumParsedInputData {
	parsed := p.EthereumParser.ParseInputData(signatures, data)

	if parsed == nil {
		return nil
	}

	for i, param := range parsed.Params {
		if param.Type == "address" || strings.HasPrefix(param.Type, "address[") {
			for j, v := range param.Values {
				parsed.Params[i].Values[j] = ToTronAddressFromAddress(v)
			}
		}
	}

	return parsed
}

func (p *TronParser) EthereumTypeGetTokenTransfersFromTx(tx *bchain.Tx) (bchain.TokenTransfers, error) {
	var transfers bchain.TokenTransfers
	var err error
	transfers, err = p.EthereumParser.EthereumTypeGetTokenTransfersFromTx(tx)

	if err != nil {
		return nil, err
	}

	// Post-process the transfers to convert addresses to Tron format
	for i, transfer := range transfers {
		if transfer.Contract != "" {
			contract := ToTronAddressFromAddress(transfer.Contract)
			transfers[i].Contract = contract
		}

		if transfer.From != "" {
			from := ToTronAddressFromAddress(transfer.From)
			transfers[i].From = from
		}

		if transfer.To != "" {
			to := ToTronAddressFromAddress(transfer.To)
			transfers[i].To = to
		}

	}

	return transfers, nil
}

func (p *TronParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	r, ok := tx.CoinSpecificData.(bchain.EthereumSpecificData)
	if !ok {
		return nil, errors.New("missing CoinSpecificData")
	}
	r.Tx.AccountNonce = SanitizeHexUint64String(r.Tx.AccountNonce)

	var err error

	r.Tx.From, err = p.FromTronAddressToHex(r.Tx.From)
	if err != nil {
		return nil, fmt.Errorf("failed to convert 'from' address: %w", err)
	}

	r.Tx.To, err = p.FromTronAddressToHex(r.Tx.To)
	if err != nil {
		return nil, fmt.Errorf("failed to convert 'to' address: %w", err)
	}

	for i, l := range r.Receipt.Logs {
		addr, err := p.FromTronAddressToHex(l.Address)
		if err != nil {
			return nil, fmt.Errorf("failed to convert log[%d] address: %w", i, err)
		}
		l.Address = addr
	}

	tx.CoinSpecificData = r
	return p.EthereumParser.PackTx(tx, height, blockTime)
}

// SanitizeHexUint64String Java-Tron's JSON-RPC returns "nonce" in format that is unexpected for `hexutil.DecodeUint64` in PackTx
func SanitizeHexUint64String(s string) string {
	if strings.HasPrefix(s, "0x") {
		sanitized := strings.TrimLeft(s[2:], "0")
		if sanitized == "" {
			return "0x0"
		}
		return "0x" + sanitized
	}
	return s
}
