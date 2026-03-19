package api

import (
	"encoding/json"

	"github.com/trezor/blockbook/bchain"
)

type chainExtraDataBase struct {
	PayloadType bchain.ChainExtraPayloadType `json:"payloadType" ts_doc:"Payload discriminator, e.g. 'tron'."`
	Payload     json.RawMessage              `json:"payload,omitempty" ts_type:"any" ts_doc:"Chain-specific payload."`
}

// TxChainExtraData wraps normalized chain-specific transaction data with a payload discriminator.
type TxChainExtraData chainExtraDataBase

// AccountChainExtraData wraps normalized chain-specific account/address data with a payload discriminator.
type AccountChainExtraData chainExtraDataBase
