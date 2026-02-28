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
}

type tronTxExtraVote struct {
	Address string `json:"address,omitempty"`
	Count   string `json:"count,omitempty"`
}

type tronTxExtraTemplateData struct {
	bchain.TronChainExtraData
	TotalFeeAmount     *api.Amount `json:"-"`
	EnergyFeeAmount    *api.Amount `json:"-"`
	BandwidthFeeAmount *api.Amount `json:"-"`
}

func (e *tronTxExtraTemplateData) hasData() bool {
	return e.ContractType != "" ||
		e.Operation != "" ||
		e.Resource != "" ||
		e.StakeAmount != "" ||
		e.UnstakeAmount != "" ||
		e.DelegateAmount != "" ||
		e.DelegateTo != "" ||
		e.AssetIssueID != "" ||
		e.TotalFee != "" ||
		e.EnergyUsage != "" ||
		e.EnergyUsageTotal != "" ||
		e.EnergyFee != "" ||
		e.BandwidthUsage != "" ||
		e.BandwidthFee != "" ||
		e.Result != "" ||
		len(e.Votes) > 0
}

func chainExtra(tx *api.Tx) *tronTxExtraTemplateData {
	if tx == nil || tx.ChainExtraData == nil || tx.ChainExtraData.PayloadType != "tron" || len(tx.ChainExtraData.Payload) == 0 {
		return nil
	}
	var extra bchain.TronChainExtraData
	if err := json.Unmarshal(tx.ChainExtraData.Payload, &extra); err != nil {
		return nil
	}
	extra.Operation = strings.TrimSpace(extra.Operation)
	extra.ContractType = strings.TrimSpace(extra.ContractType)
	extra.Resource = strings.TrimSpace(extra.Resource)
	extra.Result = strings.TrimSpace(extra.Result)
	rv := &tronTxExtraTemplateData{
		TronChainExtraData: extra,
		TotalFeeAmount:     parseTronSunAmount(extra.TotalFee),
		EnergyFeeAmount:    parseTronSunAmount(extra.EnergyFee),
		BandwidthFeeAmount: parseTronSunAmount(extra.BandwidthFee),
	}
	if !rv.hasData() {
		return nil
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
