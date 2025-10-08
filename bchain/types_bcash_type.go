package bchain

import (
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/trezor/blockbook/common"
)

type BcashNFTCapabilityType uint8

const (
	PREFIX_TOKEN       = 0xef
	HAS_COMMITMENT_LEN = 0x40
	HAS_NFT            = 0x20
	HAS_AMOUNT         = 0x10
)

const (
	NFTCapabilityNone    BcashNFTCapabilityType = 0
	NFTCapabilityMutable BcashNFTCapabilityType = 1
	NFTCapabilityMinting BcashNFTCapabilityType = 2
)

type BcashNFTCapabilityLabel string

// NFTCapabilityLabelToNumber maps a BcashNFTCapabilityLabel to its corresponding BcashNFTCapabilityType number.
func NFTCapabilityLabelToNumber(label BcashNFTCapabilityLabel) BcashNFTCapabilityType {
	switch label {
	case NFTCapabilityLabelNone:
		return NFTCapabilityNone
	case NFTCapabilityLabelMutable:
		return NFTCapabilityMutable
	case NFTCapabilityLabelMinting:
		return NFTCapabilityMinting
	default:
		return NFTCapabilityNone
	}
}

const (
	NFTCapabilityLabelNone    BcashNFTCapabilityLabel = "none"
	NFTCapabilityLabelMutable BcashNFTCapabilityLabel = "mutable"
	NFTCapabilityLabelMinting BcashNFTCapabilityLabel = "minting"
)

// Map from type to label
func ToNFTCapabilityLabel(c BcashNFTCapabilityType) BcashNFTCapabilityLabel {
	switch c {
	case NFTCapabilityNone:
		return NFTCapabilityLabelNone
	case NFTCapabilityMutable:
		return NFTCapabilityLabelMutable
	case NFTCapabilityMinting:
		return NFTCapabilityLabelMinting
	default:
		return NFTCapabilityLabelNone
	}
}

// Map from label to type
func ToNFTCapabilityType(l BcashNFTCapabilityLabel) BcashNFTCapabilityType {
	switch l {
	case NFTCapabilityLabelNone:
		return NFTCapabilityNone
	case NFTCapabilityLabelMutable:
		return NFTCapabilityMutable
	case NFTCapabilityLabelMinting:
		return NFTCapabilityMinting
	default:
		return NFTCapabilityNone
	}
}

type BcashTokenNft struct {
	Capability BcashNFTCapabilityLabel `json:"capability" ts_doc:"Capability of the NFT, which can be 'none', 'mutable', or 'minting'"`
	Commitment []byte                  `json:"commitment" ts_doc:"Commitment of the NFT, hex encoded, maximum 40 bytes"`
}

// BcashToken represents a CashToken in a BitcoinCash transaction
type BcashToken struct {
	Category []byte         `json:"category" ts_doc:"Identifier of the token, which is a 32-byte hash of its genesis transaction"`
	Amount   common.Amount  `json:"amount" ts_doc:"Fungible token amount in base units"`
	Nft      *BcashTokenNft `json:"nft,omitempty" ts_doc:"Optional pointer to a BcashTokenNft object if the token also holds an NFT"`
}

// MarshalJSON implements custom JSON marshalling for BcashToken to encode Category as hex string.
func (t *BcashToken) MarshalJSON() ([]byte, error) {
	type Alias BcashToken
	return json.Marshal(&struct {
		Category string `json:"category"`
		*Alias
	}{
		Category: hex.EncodeToString(t.Category),
		Alias:    (*Alias)(t),
	})
}

// UnmarshalJSON implements custom JSON unmarshalling for BcashToken to decode Category from hex string.
func (t *BcashToken) UnmarshalJSON(data []byte) error {
	type Alias BcashToken
	aux := &struct {
		Category string `json:"category"`
		*Alias
	}{
		Alias: (*Alias)(t),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	cat, err := hex.DecodeString(aux.Category)
	if err != nil {
		return fmt.Errorf("invalid hex in category: %w", err)
	}
	t.Category = cat
	return nil
}

func (nft *BcashTokenNft) MarshalJSON() ([]byte, error) {
	type Alias BcashTokenNft
	return json.Marshal(&struct {
		Commitment string `json:"commitment"`
		*Alias
	}{
		Commitment: hex.EncodeToString(nft.Commitment),
		Alias:      (*Alias)(nft),
	})
}

func (nft *BcashTokenNft) UnmarshalJSON(data []byte) error {
	type Alias BcashTokenNft
	aux := &struct {
		Commitment string `json:"commitment"`
		*Alias
	}{
		Alias: (*Alias)(nft),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if aux.Commitment != "" {
		commitment, err := hex.DecodeString(aux.Commitment)
		if err != nil {
			return fmt.Errorf("invalid hex in commitment: %w", err)
		}
		nft.Commitment = commitment
	} else {
		nft.Commitment = nil
	}
	return nil
}
