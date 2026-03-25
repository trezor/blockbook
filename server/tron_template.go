package server

import (
	"encoding/json"
	"math/big"
	"strings"

	"github.com/trezor/blockbook/api"
	"github.com/trezor/blockbook/bchain"
)

func init() {
	registerTemplateFunc("chainExtra", chainExtra)
	registerTemplateFunc("accountChainExtra", accountChainExtra)
}

type tronTxExtraTemplateData struct {
	bchain.TronChainExtraData
	TotalFeeAmount      *api.Amount `json:"-"`
	EnergyFeeAmount     *api.Amount `json:"-"`
	BandwidthFeeAmount  *api.Amount `json:"-"`
	DelegateAmountValue *api.Amount `json:"-"`
	StakeAmountValue    *api.Amount `json:"-"`
	UnstakeAmountValue  *api.Amount `json:"-"`
}

type tronAccountExtraTemplateData struct {
	bchain.TronAccountExtraData
}

func chainExtra(tx *api.Tx) *tronTxExtraTemplateData {
	if tx == nil || tx.ChainExtraData == nil {
		return nil
	}
	var extra bchain.TronChainExtraData
	if err := json.Unmarshal(tx.ChainExtraData.Payload, &extra); err != nil {
		return nil
	}

	rv := &tronTxExtraTemplateData{
		TronChainExtraData:  extra,
		TotalFeeAmount:      parseTronSunAmount(extra.TotalFee),
		EnergyFeeAmount:     parseTronSunAmount(extra.EnergyFee),
		BandwidthFeeAmount:  parseTronSunAmount(extra.BandwidthFee),
		DelegateAmountValue: parseTronSunAmount(extra.DelegateAmount),
		StakeAmountValue:    parseTronSunAmount(extra.StakeAmount),
		UnstakeAmountValue:  parseTronSunAmount(extra.UnstakeAmount),
	}
	return rv
}

func accountChainExtra(addr *api.Address) *tronAccountExtraTemplateData {
	if addr == nil || addr.ChainExtraData == nil {
		return nil
	}
	var extra bchain.TronAccountExtraData
	if err := json.Unmarshal(addr.ChainExtraData.Payload, &extra); err != nil {
		return nil
	}
	rv := &tronAccountExtraTemplateData{
		TronAccountExtraData: extra,
	}
	return rv
}

func parseTronSunAmount(amount string) *api.Amount {
	amount = strings.TrimSpace(amount)
	if amount == "" {
		return nil
	}
	bi, ok := new(big.Int).SetString(amount, 10)
	if !ok {
		return nil
	}
	return (*api.Amount)(bi)
}
