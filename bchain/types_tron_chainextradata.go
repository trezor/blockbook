package bchain

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

// TronUnstakingBatch describes one pending Tron unstaking batch (Stake 2.0).
type TronUnstakingBatch struct {
	Amount     string `json:"amount"`
	ExpireTime int64  `json:"expireTime"`
}

// TronVote describes one current vote allocation to a Tron Super Representative.
type TronVote struct {
	Address   string `json:"address"`
	VoteCount string `json:"voteCount"`
}

// TronStakingInfo contains normalized Tron staking and governance account metadata.
type TronStakingInfo struct {
	StakedBalance             string               `json:"stakedBalance"`
	StakedBalanceEnergy       string               `json:"stakedBalanceEnergy"`
	StakedBalanceBandwidth    string               `json:"stakedBalanceBandwidth"`
	UnstakingBatches          []TronUnstakingBatch `json:"unstakingBatches"`
	TotalVotingPower          string               `json:"totalVotingPower"`
	AvailableVotingPower      string               `json:"availableVotingPower"`
	Votes                     []TronVote           `json:"votes"`
	UnclaimedReward           string               `json:"unclaimedReward"`
	DelegatedBalanceEnergy    string               `json:"delegatedBalanceEnergy"`
	DelegatedBalanceBandwidth string               `json:"delegatedBalanceBandwidth"`
}

// TronAccountExtraData contains normalized Tron-specific account resource metadata.
type TronAccountExtraData struct {
	AvailableStakedBandwidth int64            `json:"availableStakedBandwidth"`
	TotalStakedBandwidth     int64            `json:"totalStakedBandwidth"`
	AvailableFreeBandwidth   int64            `json:"availableFreeBandwidth"`
	TotalFreeBandwidth       int64            `json:"totalFreeBandwidth"`
	AvailableEnergy          int64            `json:"availableEnergy"`
	TotalEnergy              int64            `json:"totalEnergy"`
	StakingInfo              *TronStakingInfo `json:"stakingInfo,omitempty"`
}
