package bchain

// ChainExtraPayloadType identifies the normalized chainExtraData payload shape.
type ChainExtraPayloadType string

const (
	ChainExtraPayloadTypeUnknown ChainExtraPayloadType = ""
	ChainExtraPayloadTypeTron    ChainExtraPayloadType = "tron"
)

// ChainExtraPayload binds a payloadType to the structs serialized into the chainExtraData payload,
// so generated client types can narrow the payload on payloadType.
type ChainExtraPayload struct {
	Type    ChainExtraPayloadType
	Tx      interface{}
	Account interface{}
}

// ChainExtraPayloads lists every payloadType a parser can return from GetChainExtraPayloadType;
// a parser that emits a payload must have an entry here (see the parser's registry test).
var ChainExtraPayloads = []ChainExtraPayload{
	{Type: ChainExtraPayloadTypeTron, Tx: TronChainExtraData{}, Account: TronAccountExtraData{}},
}

// ChainExtraPayloadByType returns the registry entry for the payloadType, if any.
func ChainExtraPayloadByType(t ChainExtraPayloadType) (ChainExtraPayload, bool) {
	for _, p := range ChainExtraPayloads {
		if p.Type == t {
			return p, true
		}
	}
	return ChainExtraPayload{}, false
}

// ChainExtraPayloadWrapper is implemented by the API structs whose Payload carries one of the
// registered structs; the method picks that wrapper's struct out of a registry entry so the
// generated client types can be derived without naming the wrappers anywhere else.
type ChainExtraPayloadWrapper interface {
	ChainExtraPayload(ChainExtraPayload) interface{}
}
