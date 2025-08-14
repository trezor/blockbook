package bchain

import "math/big"

type BcashNFTCapabilityType uint8

const (
	NFTCapabilityNone    BcashNFTCapabilityType = 0
	NFTCapabilityMutable BcashNFTCapabilityType = 1
	NFTCapabilityMinting BcashNFTCapabilityType = 2
)

type BcashNFTCapabilityLabel string

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
	Commitment string                  `json:"commitment" ts_doc:"Commitment of the NFT, hex encoded, maximum 40 bytes"`
}

// BcashToken represents a CashToken in a BitcoinCash transaction
type BcashToken struct {
	Category string         `json:"category" ts_doc:"Identifier of the token, which is a 32-byte hash of its genesis transaction"`
	Amount   big.Int        `json:"amount" ts_doc:"Fungible token amount in base units"`
	Nft      *BcashTokenNft `json:"nft,omitempty" ts_doc:"Optional pointer to a BcashTokenNft object if the token also holds an NFT"`
}

// BcashSpecific contains data specific to BitcoinCash transactions
type BcashSpecific struct {
	TokenVins  []*BcashToken `json:"tokenVins,omitempty" ts_doc:"Array of pointers to BcashToken objects or nil if there are no tokens at the corresponding vin"`
	TokenVouts []*BcashToken `json:"tokenVouts,omitempty" ts_doc:"Array of pointers to BcashToken objects or nil if there are no tokens at the corresponding vout"`
}
