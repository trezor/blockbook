package bcmr

// https://github.com/bitjson/chip-bcmr/blob/master/bcmr-v2.schema.ts

// URIs is a mapping of identifiers to URIs associated with an entity.
type URIs map[string]string

// Extensions is a mapping of extension identifiers to extension definitions.
type Extensions map[string]any

// Tag allows registries to classify and group identities by characteristics.
type Tag struct {
	Name        string      `json:"name"`
	Description *string     `json:"description,omitempty"`
	Uris        *URIs       `json:"uris,omitempty"`
	Extensions  *Extensions `json:"extensions,omitempty"`
}

// NftType defines one type of NFT within a token category.
type NftType struct {
	Name        string      `json:"name"`
	Description *string     `json:"description,omitempty"`
	Fields      *[]string   `json:"fields,omitempty"`
	Uris        *URIs       `json:"uris,omitempty"`
	Extensions  *Extensions `json:"extensions,omitempty"`
}

// NftCategoryFieldEncoding defines the encoding for an NFT field.
type NftCategoryFieldEncoding struct {
	Type      string  `json:"type"`
	Aggregate *string `json:"aggregate,omitempty"`
	Decimals  *int    `json:"decimals,omitempty"`
	Unit      *string `json:"unit,omitempty"`
}

// NftCategoryFieldValue defines a field that can be encoded in NFTs.
type NftCategoryFieldValue struct {
	Name        *string                  `json:"name,omitempty"`
	Description *string                  `json:"description,omitempty"`
	Encoding    NftCategoryFieldEncoding `json:"encoding"`
	Uris        *URIs                    `json:"uris,omitempty"`
	Extensions  *Extensions              `json:"extensions,omitempty"`
}

// NftCategoryField is a mapping of field identifiers to field definitions.
type NftCategoryField map[string]NftCategoryFieldValue

// SequentialNftCollection is for collections of sequential NFTs.
type SequentialNftCollection struct {
	Types map[string]NftType `json:"types"`
}

// ParsableNftCollection is for collections of parsable NFTs.
type ParsableNftCollection struct {
	SequentialNftCollection
	Bytecode string `json:"bytecode,omitempty"`
}

// NftCategory specifies the non-fungible token information for a token category.
type NftCategory struct {
	Description *string                `json:"description,omitempty"`
	Fields      *NftCategoryField      `json:"fields,omitempty"`
	Parse       *ParsableNftCollection `json:"parse"` // SequentialNftCollection or ParsableNftCollection
}

// TokenCategory specifies information about an identity's token category.
type TokenCategory struct {
	Category string       `json:"category"`
	Symbol   string       `json:"symbol"`
	Decimals *int         `json:"decimals,omitempty"`
	Nfts     *NftCategory `json:"nfts,omitempty"`
}

// IdentitySnapshot is a snapshot of the metadata for a particular identity.
type IdentitySnapshot struct {
	Name        string         `json:"name"`
	Description *string        `json:"description,omitempty"`
	Tags        *[]string      `json:"tags,omitempty"`
	Migrated    *string        `json:"migrated,omitempty"`
	Token       *TokenCategory `json:"token,omitempty"`
	Status      *string        `json:"status,omitempty"`
	SplitId     *string        `json:"splitId,omitempty"`
	Uris        *URIs          `json:"uris,omitempty"`
	Extensions  *Extensions    `json:"extensions,omitempty"`
}

// ChainSnapshot is a snapshot of the metadata for a particular chain/network.
type ChainSnapshot struct {
	Name        string      `json:"name"`
	Description *string     `json:"description,omitempty"`
	Tags        *[]string   `json:"tags,omitempty"`
	Status      *string     `json:"status,omitempty"`
	SplitId     *string     `json:"splitId,omitempty"`
	Uris        *URIs       `json:"uris,omitempty"`
	Extensions  *Extensions `json:"extensions,omitempty"`
	Token       struct {
		Symbol   string `json:"symbol"`
		Decimals *int   `json:"decimals,omitempty"`
	} `json:"token"`
}

// RegistryTimestampKeyedValues is a field keyed by timestamps.
type RegistryTimestampKeyedValues[T any] map[string]T

// ChainHistory is a block height-keyed map of ChainSnapshots.
type ChainHistory = RegistryTimestampKeyedValues[ChainSnapshot]

// IdentityHistory is a timestamp-keyed map of IdentitySnapshots.
type IdentityHistory = RegistryTimestampKeyedValues[IdentitySnapshot]

// OffChainRegistryIdentity is an identity representing a metadata registry not published on-chain.
type OffChainRegistryIdentity struct {
	Name        string      `json:"name"`
	Description *string     `json:"description,omitempty"`
	Uris        *URIs       `json:"uris,omitempty"`
	Tags        *[]string   `json:"tags,omitempty"`
	Extensions  *Extensions `json:"extensions,omitempty"`
}

// Registry is a Bitcoin Cash Metadata Registry.
type Registry struct {
	Schema  *string `json:"$schema,omitempty"`
	Version struct {
		Major int `json:"major"`
		Minor int `json:"minor"`
		Patch int `json:"patch"`
	} `json:"version"`
	LatestRevision   string                      `json:"latestRevision"`
	RegistryIdentity any                         `json:"registryIdentity"` // OffChainRegistryIdentity or string
	Identities       *map[string]IdentityHistory `json:"identities,omitempty"`
	Tags             *map[string]Tag             `json:"tags,omitempty"`
	DefaultChain     *string                     `json:"defaultChain,omitempty"`
	Chains           *map[string]ChainHistory    `json:"chains,omitempty"`
	License          *string                     `json:"license,omitempty"`
	Locales          *map[string]struct {
		Chains     *map[string]ChainHistory    `json:"chains,omitempty"`
		Extensions *Extensions                 `json:"extensions,omitempty"`
		Identities *map[string]IdentityHistory `json:"identities,omitempty"`
		Tags       *map[string]Tag             `json:"tags,omitempty"`
	} `json:"locales,omitempty"`
	Extensions *Extensions `json:"extensions,omitempty"`
}
