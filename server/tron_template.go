package server

import (
	"encoding/json"
	"strings"

	"github.com/trezor/blockbook/api"
)

func init() {
	registerTemplateFunc("chainExtra", chainExtra)
}

type tronTxExtraVote struct {
	Address string `json:"address,omitempty"`
	Count   string `json:"count,omitempty"`
}

type tronTxExtraTemplateData struct {
	ContractType     string            `json:"contractType,omitempty"`
	Operation        string            `json:"operation,omitempty"`
	Resource         string            `json:"resource,omitempty"`
	StakeAmount      string            `json:"stakeAmount,omitempty"`
	UnstakeAmount    string            `json:"unstakeAmount,omitempty"`
	DelegateAmount   string            `json:"delegateAmount,omitempty"`
	DelegateTo       string            `json:"delegateTo,omitempty"`
	AssetIssueID     string            `json:"assetIssueID,omitempty"`
	TotalFee         string            `json:"totalFee,omitempty"`
	EnergyUsage      string            `json:"energyUsage,omitempty"`
	EnergyUsageTotal string            `json:"energyUsageTotal,omitempty"`
	EnergyFee        string            `json:"energyFee,omitempty"`
	BandwidthUsage   string            `json:"bandwidthUsage,omitempty"`
	BandwidthFee     string            `json:"bandwidthFee,omitempty"`
	Result           string            `json:"result,omitempty"`
	Votes            []tronTxExtraVote `json:"votes,omitempty"`
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
	if tx == nil || len(tx.ChainExtraData) == 0 {
		return nil
	}
	var extra tronTxExtraTemplateData
	if err := json.Unmarshal(tx.ChainExtraData, &extra); err != nil {
		return nil
	}
	extra.Operation = strings.TrimSpace(extra.Operation)
	extra.ContractType = strings.TrimSpace(extra.ContractType)
	extra.Resource = strings.TrimSpace(extra.Resource)
	extra.Result = strings.TrimSpace(extra.Result)
	if !extra.hasData() {
		return nil
	}
	return &extra
}
