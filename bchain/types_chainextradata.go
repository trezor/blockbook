package bchain

// ChainExtraPayloadType identifies the normalized chainExtraData payload shape.
type ChainExtraPayloadType string

const (
	ChainExtraPayloadTypeUnknown ChainExtraPayloadType = ""
	ChainExtraPayloadTypeTron    ChainExtraPayloadType = "tron"
)
