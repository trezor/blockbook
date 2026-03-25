package bchain

// ChainExtraPayloadType identifies the normalized chainExtraData payload shape.
type ChainExtraPayloadType string

const (
	ChainExtraPayloadTypeUnknown ChainExtraPayloadType = ""
	ChainExtraPayloadTypeTron    ChainExtraPayloadType = "tron"
)

// TronVoteExtra describes a single Tron vote entry.
type TronVoteExtra struct {
	Address string `json:"address,omitempty"`
	Count   string `json:"count,omitempty"`
}

// TronChainExtraData contains normalized Tron-specific transaction metadata.
type TronChainExtraData struct {
	ContractType     string          `json:"contractType,omitempty"`
	Operation        string          `json:"operation,omitempty"`
	Resource         string          `json:"resource,omitempty"`
	StakeAmount      string          `json:"stakeAmount,omitempty"`
	UnstakeAmount    string          `json:"unstakeAmount,omitempty"`
	DelegateAmount   string          `json:"delegateAmount,omitempty"`
	DelegateTo       string          `json:"delegateTo,omitempty"`
	AssetIssueID     string          `json:"assetIssueID,omitempty"`
	TotalFee         string          `json:"totalFee,omitempty"`
	FeeLimit         string          `json:"feeLimit,omitempty"`
	EnergyUsage      string          `json:"energyUsage,omitempty"`
	EnergyUsageTotal string          `json:"energyUsageTotal,omitempty"`
	EnergyFee        string          `json:"energyFee,omitempty"`
	BandwidthUsage   string          `json:"bandwidthUsage,omitempty"`
	BandwidthFee     string          `json:"bandwidthFee,omitempty"`
	Result           string          `json:"result,omitempty"`
	Votes            []TronVoteExtra `json:"votes,omitempty"`
}

// TronAccountExtraData contains normalized Tron-specific account resource metadata.
type TronAccountExtraData struct {
	AvailableBandwidth int64 `json:"availableBandwidth"`
	TotalBandwidth     int64 `json:"totalBandwidth"`
	AvailableEnergy    int64 `json:"availableEnergy"`
	TotalEnergy        int64 `json:"totalEnergy"`
}
