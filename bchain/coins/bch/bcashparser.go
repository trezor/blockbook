package bch

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/martinboehm/bchutil"
	"github.com/martinboehm/btcd/wire"
	"github.com/martinboehm/btcutil"
	"github.com/martinboehm/btcutil/chaincfg"
	"github.com/martinboehm/btcutil/txscript"
	"github.com/schancel/cashaddr-converter/address"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/btc"
)

// AddressFormat type is used to specify different formats of address
type AddressFormat = uint8

const (
	// Legacy AddressFormat is the same as Bitcoin
	Legacy AddressFormat = iota
	// CashAddr AddressFormat is new Bitcoin Cash standard
	CashAddr
)

const (
	// MainNetPrefix is CashAddr prefix for mainnet
	MainNetPrefix = "bitcoincash:"
	// TestNetPrefix is CashAddr prefix for testnet
	TestNetPrefix = "bchtest:"
	// RegTestPrefix is CashAddr prefix for regtest
	RegTestPrefix = "bchreg:"
)

var (
	// MainNetParams are parser parameters for mainnet
	MainNetParams chaincfg.Params
	// TestNetParams are parser parameters for testnet
	TestNetParams chaincfg.Params
	// RegtestParams are parser parameters for regtest
	RegtestParams chaincfg.Params
)

func init() {
	MainNetParams = chaincfg.MainNetParams
	MainNetParams.Net = bchutil.MainnetMagic

	TestNetParams = chaincfg.TestNet3Params
	TestNetParams.Net = bchutil.TestnetMagic

	RegtestParams = chaincfg.RegressionNetParams
	RegtestParams.Net = bchutil.Regtestmagic
}

// BCashParser handle
type BCashParser struct {
	*btc.BitcoinLikeParser
	AddressFormat AddressFormat
}

// NewBCashParser returns new BCashParser instance
func NewBCashParser(params *chaincfg.Params, c *btc.Configuration) (*BCashParser, error) {
	var format AddressFormat
	switch c.AddressFormat {
	case "":
		fallthrough
	case "cashaddr":
		format = CashAddr
	case "legacy":
		format = Legacy
	default:
		return nil, fmt.Errorf("Unknown address format: %s", c.AddressFormat)
	}
	p := &BCashParser{
		BitcoinLikeParser: btc.NewBitcoinLikeParser(params, c),
		AddressFormat:     format,
	}
	p.OutputScriptToAddressesFunc = p.outputScriptToAddresses
	return p, nil
}

// GetChainParams contains network parameters for the main Bitcoin Cash network,
// the regression test Bitcoin Cash network, the test Bitcoin Cash network and
// the simulation test Bitcoin Cash network, in this order
func GetChainParams(chain string) *chaincfg.Params {
	if !chaincfg.IsRegistered(&MainNetParams) {
		err := chaincfg.Register(&MainNetParams)
		if err == nil {
			err = chaincfg.Register(&TestNetParams)
		}
		if err == nil {
			err = chaincfg.Register(&RegtestParams)
		}
		if err != nil {
			panic(err)
		}
	}
	switch chain {
	case "test":
		return &TestNetParams
	case "regtest":
		return &RegtestParams
	default:
		return &MainNetParams
	}
}

// GetAddrDescFromAddress returns internal address representation of given address
func (p *BCashParser) GetAddrDescFromAddress(address string) (bchain.AddressDescriptor, error) {
	return p.addressToOutputScript(address)
}

// GetScriptFromAddrDesc returns the locking script information without token information
func (p *BCashParser) GetScriptFromAddrDesc(addrDesc bchain.AddressDescriptor) ([]byte, error) {
	_, pkScriptStart, err := p.ParseTokenData(addrDesc)
	if err != nil {
		return nil, err
	}

	return addrDesc[pkScriptStart:], nil
}

// addressToOutputScript converts bitcoin address to ScriptPubKey
func (p *BCashParser) addressToOutputScript(address string) ([]byte, error) {
	if isCashAddr(address) {
		da, err := bchutil.DecodeAddress(address, p.Params)
		if err != nil {
			return nil, err
		}
		script, err := bchutil.PayToAddrScript(da)
		if err != nil {
			return nil, err
		}
		return script, nil
	}
	da, err := btcutil.DecodeAddress(address, p.Params)
	if err != nil {
		return nil, err
	}
	script, err := txscript.PayToAddrScript(da)
	if err != nil {
		return nil, err
	}
	return script, nil
}

func isCashAddr(addr string) bool {
	n := len(addr)
	switch {
	case n > len(MainNetPrefix) && addr[0:len(MainNetPrefix)] == MainNetPrefix:
		return true
	case n > len(TestNetPrefix) && addr[0:len(TestNetPrefix)] == TestNetPrefix:
		return true
	case n > len(RegTestPrefix) && addr[0:len(RegTestPrefix)] == RegTestPrefix:
		return true
	}

	return false
}

// outputScriptToAddresses converts ScriptPubKey to bitcoin addresses
func (p *BCashParser) outputScriptToAddresses(script []byte) ([]string, bool, error) {
	var err error
	script, err = p.GetScriptFromAddrDesc(script)
	if err != nil {
		return nil, false, err
	}

	// convert possible P2PK script to P2PK, which bchutil can process
	script, err = txscript.ConvertP2PKtoP2PKH(p.Params.Base58CksumHasher, script)
	if err != nil {
		return nil, false, err
	}
	a, err := bchutil.ExtractPkScriptAddrs(script, p.Params)
	if err != nil {
		// do not return unknown script type error as error
		if err.Error() == "unknown script type" {
			// try OP_RETURN script
			or := p.TryParseOPReturn(script)
			if or != "" {
				return []string{or}, false, nil
			}
			return []string{}, false, nil
		}
		return nil, false, err
	}
	// EncodeAddress returns CashAddr address
	addr := a.EncodeAddress()
	if p.AddressFormat == Legacy {
		da, err := address.NewFromString(addr)
		if err != nil {
			return nil, false, err
		}
		ca, err := da.Legacy()
		if err != nil {
			return nil, false, err
		}
		addr, err = ca.Encode()
		if err != nil {
			return nil, false, err
		}
	}
	return []string{addr}, len(addr) > 0, nil
}

// https://github.com/bitjson/cashtokens/blob/1d3745e04b2c454f7a194d9fab368df72e8adc69/readme.md#token-encoding
// https://github.com/bitauth/libauth/blob/60aec239cc2d57ae21d0069c5bbafb346abc9b66/src/lib/message/transaction-encoding.ts#L223
func (p *BCashParser) ParseTokenData(script []byte) (*bchain.BcashToken, int, error) {
	if len(script) == 0 {
		return nil, 0, nil
	}

	br := bytes.NewReader(script)

	// Check for prefix 0xef
	b, err := br.ReadByte()
	if err != nil {
		return nil, 0, err
	}
	if b != 0xef {
		return nil, 0, nil // Not a token prefix
	}

	// Check minimum length
	if br.Size() < 34 {
		return nil, 0, fmt.Errorf("Invalid token prefix: insufficient length. The minimum possible length is 34. Missing bytes: %d", 34-br.Size())
	}

	token := &bchain.BcashToken{}

	// Read tokenId (32 bytes, reversed)
	categoryBin := make([]byte, 32)
	br.Read(categoryBin[:])
	for i, j := 0, len(categoryBin)-1; i < j; i, j = i+1, j-1 {
		categoryBin[i], categoryBin[j] = categoryBin[j], categoryBin[i]
	}
	token.Category = hex.EncodeToString(categoryBin)

	// Read bitfield
	bitfield, err := br.ReadByte()
	if err != nil {
		return nil, 0, err
	}
	if bitfield == 0 {
		return nil, 0, fmt.Errorf("Invalid token prefix: must encode at least one token. Bitfield: 0b%08b", bitfield)
	}

	prefixStructure := bitfield & 0xf0
	reserved := prefixStructure & 0x80
	if reserved != 0 {
		return nil, 0, fmt.Errorf("Invalid token prefix: reserved bit is set. Bitfield: 0b%08b", bitfield)
	}
	hasCommitmentLength := prefixStructure & 0x40
	hasNFT := prefixStructure & 0x20
	hasAmount := prefixStructure & 0x10

	NFTCapability := bchain.BcashNFTCapabilityType(bitfield & 0x0f)

	var commitmentLength uint64 = 0
	if hasNFT != 0 {
		token.Nft = &bchain.BcashTokenNft{}

		if hasCommitmentLength != 0 {
			commitmentLength, err = wire.ReadVarInt(br, 0)
			if err != nil {
				return nil, 0, fmt.Errorf("Invalid token prefix: invalid non-fungible token commitment. Error reading CompactSize-prefixed bin: invalid CompactSize. Error reading CompactSize.")
			}
			if commitmentLength == 0 {
				return nil, 0, fmt.Errorf("Invalid token prefix: if encoded, commitment length must be greater than 0.")
			}
			if br.Len() < int(commitmentLength) {
				return nil, 0, fmt.Errorf("Invalid token prefix: invalid non-fungible token commitment. Error reading CompactSize-prefixed bin: insufficient bytes. Required bytes: %d, remaining bytes: %d", commitmentLength, br.Len())
			}
		}
		if NFTCapability > 2 {
			return nil, 0, fmt.Errorf("Invalid token prefix: capability must be none (0), mutable (1), or minting (2). Capability value: %d", NFTCapability)
		}
		token.Nft.Capability = bchain.ToNFTCapabilityLabel(NFTCapability)

		if hasCommitmentLength != 0 {
			commitmentBin := make([]byte, commitmentLength)
			_, err = br.Read(commitmentBin[:])
			if err != nil {
				return nil, 0, fmt.Errorf("Invalid token prefix: invalid non-fungible token commitment.")
			}
			token.Nft.Commitment = hex.EncodeToString(commitmentBin)
		} else {
			token.Nft.Commitment = ""
		}
	} else {
		if hasCommitmentLength != 0 {
			return nil, 0, fmt.Errorf("Invalid token prefix: commitment requires an NFT. Bitfield: 0b%08b", bitfield)
		}
		if NFTCapability > 0 {
			return nil, 0, fmt.Errorf("Invalid token prefix: capability requires an NFT. Bitfield: 0b%04b", bitfield)
		}
	}

	if hasAmount != 0 {
		ftAmount, err := wire.ReadVarInt(br, 0)
		if err != nil {
			return nil, 0, fmt.Errorf("Invalid token prefix: invalid fungible token amount encoding. Error reading CompactSize.")
		}
		if ftAmount == 0 {
			return nil, 0, fmt.Errorf("Invalid token prefix: if encoded, fungible token amount must be greater than 0.")
		}

		if ftAmount > 9223372036854775807 {
			return nil, 0, fmt.Errorf("Invalid token prefix: exceeds maximum fungible token amount of 9223372036854775807. Encoded amount: %d", ftAmount)
		}
		token.Amount = *big.NewInt(int64(ftAmount))
	} else {
		token.Amount = *big.NewInt(0)
	}

	return token, int(br.Size()) - br.Len(), nil
}

// BcashTypePostProcessApiTx is unsupported
func (b *BCashParser) BcashTypeParseTokenData(addrDesc bchain.AddressDescriptor) (*bchain.BcashToken, int, error) {
	return b.ParseTokenData(addrDesc)
}
