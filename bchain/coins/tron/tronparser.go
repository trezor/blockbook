package tron

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
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

func (p *TronParser) GetAddrDescFromAddress(address string) (bchain.AddressDescriptor, error) {
	address = strip0xPrefix(address)

	if len(address) == TronAddressLen {
		decoded := base58.Decode(address)
		if len(decoded) != 25 || decoded[0] != 0x41 {
			return nil, errors.New("invalid Tron base58 address")
		}
		payload := decoded[:21]
		checksum := decoded[21:]
		first := sha256.Sum256(payload)
		second := sha256.Sum256(first[:])
		if !bytes.Equal(checksum, second[:4]) {
			return nil, errors.New("invalid Tron base58 checksum")
		}
		return payload[1:], nil
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
	var withPrefix []byte

	// check if already prefixed with 0x41
	if len(addrDesc) == 1+TronTypeAddressDescriptorLen && addrDesc[0] == 0x41 {
		withPrefix = addrDesc
	} else {
		withPrefix = append([]byte{0x41}, addrDesc...)
	}

	firstSHA := sha256.Sum256(withPrefix)
	secondSHA := sha256.Sum256(firstSHA[:])
	checksum := secondSHA[:4]

	fullAddress := append(withPrefix, checksum...)

	base58Addr := base58.Encode(fullAddress)

	return base58Addr
}

func ToTronAddressFromAddress(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	if has0xPrefix(address) {
		address = address[2:]
		address = strings.TrimSpace(address)
		if address == "" {
			return ""
		}
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

func (p *TronParser) GetEthereumTxData(tx *bchain.Tx) *bchain.EthereumTxData {
	r := p.EthereumParser.GetEthereumTxData(tx)
	// Tron reuses Ethereum-like data structure, but some fields are not
	// semantically correct for Tron transactions and should not leak into API output.
	r.Nonce = 0
	r.GasLimit = big.NewInt(0)
	r.GasPrice = nil
	r.GasUsed = nil
	return r
}

func (p *TronParser) GetChainExtraData(tx *bchain.Tx) (json.RawMessage, error) {
	csd, _, err := parseTronExtra(tx)
	if err != nil {
		return nil, err
	}
	return csd.ChainExtraData, nil
}

func (p *TronParser) GetChainExtraPayloadType() bchain.ChainExtraPayloadType {
	return bchain.ChainExtraPayloadTypeTron
}

func parseTronExtra(tx *bchain.Tx) (bchain.EthereumSpecificData, *bchain.TronChainExtraData, error) {
	if tx == nil {
		return bchain.EthereumSpecificData{}, nil, errors.New("tx is nil")
	}
	csd, ok := tx.CoinSpecificData.(bchain.EthereumSpecificData)
	if !ok || len(csd.ChainExtraData) == 0 {
		return bchain.EthereumSpecificData{}, nil, errors.New("missing ethereumSpecificData.chainExtraData")
	}
	var extra bchain.TronChainExtraData
	if err := json.Unmarshal(csd.ChainExtraData, &extra); err != nil {
		return bchain.EthereumSpecificData{}, nil, fmt.Errorf("invalid tron chainExtraData: %w", err)
	}
	return csd, &extra, nil
}

func validateTronChainExtraData(chainExtraData json.RawMessage) error {
	if len(chainExtraData) == 0 {
		return nil
	}
	var extra bchain.TronChainExtraData
	if err := json.Unmarshal(chainExtraData, &extra); err != nil {
		return fmt.Errorf("invalid tron chainExtraData: %w", err)
	}
	return nil
}

func (p *TronParser) PackTx(tx *bchain.Tx, height uint32, blockTime int64) ([]byte, error) {
	r, ok := tx.CoinSpecificData.(bchain.EthereumSpecificData)
	if !ok {
		return nil, errors.New("missing CoinSpecificData")
	}
	if err := validateTronChainExtraData(r.ChainExtraData); err != nil {
		return nil, err
	}
	r.Tx.AccountNonce = SanitizeHexUint64String(r.Tx.AccountNonce)

	var err error

	r.Tx.From, err = p.FromTronAddressToHex(r.Tx.From)
	if err != nil {
		return nil, fmt.Errorf("failed to convert 'from' address: %w", err)
	}

	if r.Tx.To != "" {
		r.Tx.To, err = p.FromTronAddressToHex(r.Tx.To)
		if err != nil {
			return nil, fmt.Errorf("failed to convert 'to' address: %w", err)
		}
	}

	if r.Receipt != nil {
		for i, l := range r.Receipt.Logs {
			addr, err := p.FromTronAddressToHex(l.Address)
			if err != nil {
				return nil, fmt.Errorf("failed to convert log[%d] address: %w", i, err)
			}
			l.Address = addr
		}
	}

	tx.CoinSpecificData = r
	return p.EthereumParser.PackTx(tx, height, blockTime)
}

func (p *TronParser) UnpackTx(buf []byte) (*bchain.Tx, uint32, error) {
	tx, height, err := p.EthereumParser.UnpackTx(buf)
	if err != nil {
		return nil, 0, err
	}
	csd, ok := tx.CoinSpecificData.(bchain.EthereumSpecificData)
	if !ok {
		return nil, 0, errors.New("missing CoinSpecificData")
	}
	if err := validateTronChainExtraData(csd.ChainExtraData); err != nil {
		return nil, 0, err
	}
	// Pending (unsolidified) Tron transactions are intentionally not served from
	// persistent tx cache so they can transition to SUCCESS/FAILED on subsequent
	// backend refreshes.
	if csd.Receipt == nil {
		return nil, 0, nil
	}
	if has0xPrefix(tx.Txid) {
		tx.Txid = tx.Txid[2:]
	}
	return tx, height, nil
}

// UnpackTxid unpacks byte array to txid in Tron format (without 0x prefix).
func (p *TronParser) UnpackTxid(buf []byte) (string, error) {
	txid, err := p.EthereumParser.UnpackTxid(buf)
	if err != nil {
		return "", err
	}
	if has0xPrefix(txid) {
		txid = txid[2:]
	}
	return txid, nil
}

// UnpackBlockHash unpacks byte array to block hash in Tron format (without 0x prefix).
func (p *TronParser) UnpackBlockHash(buf []byte) (string, error) {
	hash, err := p.EthereumParser.UnpackBlockHash(buf)
	if err != nil {
		return "", err
	}
	if has0xPrefix(hash) {
		hash = hash[2:]
	}
	return hash, nil
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

func tronNoteHexToInternalType(noteHex string) (bchain.EthereumInternalTransactionType, error) {
	note, err := decodeNoteHex(noteHex)
	if err != nil {
		return bchain.CALL, err
	}

	switch note {
	case "create":
		return bchain.CREATE, nil
	case "suicide":
		return bchain.SELFDESTRUCT, nil
	case "call":
		return bchain.CALL, nil
	default:
		// add others
		return bchain.CALL, nil
	}
}

func decodeNoteHex(hexStr string) (string, error) {
	decoded, err := hex.DecodeString(hexStr)
	if err != nil {
		return "", fmt.Errorf("invalid hex in note: %s", hexStr)
	}
	return string(decoded), nil
}
