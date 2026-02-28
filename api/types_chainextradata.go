package api

import (
	"encoding/json"

	"github.com/trezor/blockbook/bchain"
)

// ChainExtraData wraps normalized chain-specific data with a payload discriminator.
type ChainExtraData struct {
	PayloadType bchain.ChainExtraPayloadType `json:"payloadType" ts_doc:"Payload discriminator, e.g. 'tron'."`
	Payload     json.RawMessage              `json:"payload,omitempty" ts_type:"any" ts_doc:"Chain-specific payload."`
}
