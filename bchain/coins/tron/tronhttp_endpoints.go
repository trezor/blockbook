package tron

import (
	"bytes"
	"context"
	"encoding/json"
	"math/big"
	"strconv"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/bchain"
)

type tronBroadcastHexResponse struct {
	Result  bool   `json:"result"`
	TxID    string `json:"txid"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

type tronGetTransactionListFromPendingResponse struct {
	TxID []string `json:"txId,omitempty"`
}

type tronGetAccountResourceResponse struct {
	FreeNetLimit   int64 `json:"freeNetLimit"`
	FreeNetUsed    int64 `json:"freeNetUsed"`
	NetLimit       int64 `json:"NetLimit"`
	NetUsed        int64 `json:"NetUsed"`
	EnergyLimit    int64 `json:"EnergyLimit"`
	EnergyUsed     int64 `json:"EnergyUsed"`
	TronPowerUsed  int64 `json:"tronPowerUsed"`
	TronPowerLimit int64 `json:"tronPowerLimit"`
}

type tronFrozenV2Entry struct {
	Type   *tronResourceCode `json:"type,omitempty"`
	Amount *int64            `json:"amount,omitempty"`
}

type tronUnfrozenV2Entry struct {
	UnfreezeAmount     *int64 `json:"unfreeze_amount,omitempty"`
	UnfreezeExpireTime *int64 `json:"unfreeze_expire_time,omitempty"`
}

type tronGetAccountResponse struct {
	FrozenV2                             []tronFrozenV2Entry   `json:"frozenV2,omitempty"`
	UnfrozenV2                           []tronUnfrozenV2Entry `json:"unfrozenV2,omitempty"`
	Votes                                []tronTxVote          `json:"votes,omitempty"`
	DelegatedFrozenV2BalanceForEnergy    int64                 `json:"delegated_frozenV2_balance_for_energy"`
	DelegatedFrozenV2BalanceForBandwidth int64                 `json:"delegated_frozenV2_balance_for_bandwidth"`
}

type tronGetRewardResponse struct {
	Reward int64 `json:"reward"`
}

type tronGetBlockResponse struct {
	Transactions []tronGetTransactionByIDResponse `json:"transactions,omitempty"`
}

type tronGetBlockHeaderResponse struct {
	BlockHeader struct {
		RawData struct {
			Number *uint64 `json:"number"`
		} `json:"raw_data"`
	} `json:"block_header"`
}

func (b *TronRPC) getLookupHTTPClient(isSolidified bool) TronHTTP {
	if isSolidified {
		return b.solidityNodeHTTP
	}
	return b.fullNodeHTTP
}

func (b *TronRPC) getTransactionByID(txid string, isSolidified bool) (*tronGetTransactionByIDResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	return b.requestTransactionByID(ctx, txid, isSolidified)
}

func (b *TronRPC) getTransactionInfoByID(txid string, isSolidified bool) (*tronGetTransactionInfoByIDResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	return b.requestTransactionInfoByID(ctx, txid, isSolidified)
}

func (b *TronRPC) GetMempoolTransactions() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	txs, err := b.requestMempoolTransactions(ctx)
	if err != nil {
		return nil, err
	}
	b.reconcileMempoolWithPendingList(txs)
	return txs, nil
}

// GetAddressChainExtraData returns normalized Tron-specific account/address data.
func (b *TronRPC) GetAddressChainExtraData(addrDesc bchain.AddressDescriptor) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	address := ToTronAddressFromDesc(addrDesc)
	type accountResourceResult struct {
		resp *tronGetAccountResourceResponse
		err  error
	}
	type accountResult struct {
		resp *tronGetAccountResponse
		err  error
	}
	type rewardResult struct {
		resp *tronGetRewardResponse
		err  error
	}
	resourceCh := make(chan accountResourceResult, 1)
	accountCh := make(chan accountResult, 1)
	rewardCh := make(chan rewardResult, 1)

	go func() {
		resp, err := b.requestAccountResource(ctx, address)
		resourceCh <- accountResourceResult{resp: resp, err: err}
	}()
	go func() {
		resp, err := b.requestAccount(ctx, address)
		accountCh <- accountResult{resp: resp, err: err}
	}()
	go func() {
		resp, err := b.requestReward(ctx, address)
		rewardCh <- rewardResult{resp: resp, err: err}
	}()

	resourceRes := <-resourceCh
	if resourceRes.err != nil {
		cancel()
		return nil, resourceRes.err
	}
	accountRes := <-accountCh

	var stakingInfo *bchain.TronStakingInfo
	if accountRes.err != nil {
		// Keep resource fields available even when staking/governance endpoints are temporarily unavailable.
		glog.Warningf("Tron /wallet/getaccount failed for %s: %v", address, accountRes.err)
		// No staking data can be built without /wallet/getaccount, do not wait for /wallet/getReward.
		cancel()
	} else if tronIsEmptyAccountResponse(accountRes.resp) {
		// Empty /wallet/getaccount payload means staking/governance data is unavailable.
		glog.Warningf("Tron /wallet/getaccount returned empty payload for %s", address)
		// No staking data can be built from empty account payload, do not wait for /wallet/getReward.
		cancel()
	} else {
		rewardRes := <-rewardCh
		rewardResp := rewardRes.resp
		if rewardRes.err != nil {
			glog.Warningf("Tron /wallet/getReward failed for %s: %v", address, rewardRes.err)
			rewardResp = &tronGetRewardResponse{}
		}
		stakingInfo = tronBuildStakingInfo(accountRes.resp, resourceRes.resp, rewardResp)
	}

	payload, err := json.Marshal(bchain.TronAccountExtraData{
		AvailableStakedBandwidth: tronAvailableResource(resourceRes.resp.NetLimit, resourceRes.resp.NetUsed),
		TotalStakedBandwidth:     resourceRes.resp.NetLimit,
		AvailableFreeBandwidth:   tronAvailableResource(resourceRes.resp.FreeNetLimit, resourceRes.resp.FreeNetUsed),
		TotalFreeBandwidth:       resourceRes.resp.FreeNetLimit,
		AvailableEnergy:          tronAvailableResource(resourceRes.resp.EnergyLimit, resourceRes.resp.EnergyUsed),
		TotalEnergy:              resourceRes.resp.EnergyLimit,
		StakingInfo:              stakingInfo,
	})
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func tronIsEmptyAccountResponse(resp *tronGetAccountResponse) bool {
	if resp == nil {
		return true
	}
	return len(resp.FrozenV2) == 0 &&
		len(resp.UnfrozenV2) == 0 &&
		len(resp.Votes) == 0 &&
		resp.DelegatedFrozenV2BalanceForEnergy == 0 &&
		resp.DelegatedFrozenV2BalanceForBandwidth == 0
}

func tronBuildStakingInfo(accountResp *tronGetAccountResponse, resourceResp *tronGetAccountResourceResponse, rewardResp *tronGetRewardResponse) *bchain.TronStakingInfo {
	if accountResp == nil {
		accountResp = &tronGetAccountResponse{}
	}
	if resourceResp == nil {
		resourceResp = &tronGetAccountResourceResponse{}
	}
	if rewardResp == nil {
		rewardResp = &tronGetRewardResponse{}
	}

	stakedEnergy := new(big.Int)
	stakedBandwidth := new(big.Int)
	for i := range accountResp.FrozenV2 {
		frozen := &accountResp.FrozenV2[i]
		if frozen.Amount == nil || *frozen.Amount <= 0 {
			continue
		}
		amount := big.NewInt(*frozen.Amount)
		if frozen.Type == nil || *frozen.Type == tronResourceBandwidth {
			stakedBandwidth.Add(stakedBandwidth, amount)
		} else if *frozen.Type == tronResourceEnergy {
			stakedEnergy.Add(stakedEnergy, amount)
		}
	}

	stakedBalance := new(big.Int).Add(new(big.Int).Set(stakedBandwidth), stakedEnergy)
	totalVotingPower := new(big.Int).Div(new(big.Int).Set(stakedBalance), big.NewInt(1_000_000))

	unstakingBatches := make([]bchain.TronUnstakingBatch, 0, len(accountResp.UnfrozenV2))
	for i := range accountResp.UnfrozenV2 {
		unfreeze := &accountResp.UnfrozenV2[i]
		if unfreeze.UnfreezeAmount == nil || *unfreeze.UnfreezeAmount <= 0 {
			continue
		}
		expireTime := int64(0)
		if unfreeze.UnfreezeExpireTime != nil && *unfreeze.UnfreezeExpireTime > 0 {
			expireTime = *unfreeze.UnfreezeExpireTime / 1000
		}
		unstakingBatches = append(unstakingBatches, bchain.TronUnstakingBatch{
			Amount:     strconv.FormatInt(*unfreeze.UnfreezeAmount, 10),
			ExpireTime: expireTime,
		})
	}

	votes := make([]bchain.TronVote, 0, len(accountResp.Votes))
	for i := range accountResp.Votes {
		vote := &accountResp.Votes[i]
		address := ToTronAddressFromAddress(vote.VoteAddress)
		if address == "" {
			continue
		}
		voteCount := int64(0)
		if vote.VoteCount != nil && *vote.VoteCount > 0 {
			voteCount = *vote.VoteCount
		}
		votes = append(votes, bchain.TronVote{
			Address:   address,
			VoteCount: strconv.FormatInt(voteCount, 10),
		})
	}

	availableVotingPower := resourceResp.TronPowerLimit
	if availableVotingPower < 0 {
		availableVotingPower = 0
	}
	unclaimedReward := rewardResp.Reward
	if unclaimedReward < 0 {
		unclaimedReward = 0
	}
	delegatedEnergy := accountResp.DelegatedFrozenV2BalanceForEnergy
	if delegatedEnergy < 0 {
		delegatedEnergy = 0
	}
	delegatedBandwidth := accountResp.DelegatedFrozenV2BalanceForBandwidth
	if delegatedBandwidth < 0 {
		delegatedBandwidth = 0
	}

	return &bchain.TronStakingInfo{
		StakedBalance:             stakedBalance.String(),
		StakedBalanceEnergy:       stakedEnergy.String(),
		StakedBalanceBandwidth:    stakedBandwidth.String(),
		UnstakingBatches:          unstakingBatches,
		TotalVotingPower:          totalVotingPower.String(),
		AvailableVotingPower:      strconv.FormatInt(availableVotingPower, 10),
		Votes:                     votes,
		UnclaimedReward:           strconv.FormatInt(unclaimedReward, 10),
		DelegatedBalanceEnergy:    strconv.FormatInt(delegatedEnergy, 10),
		DelegatedBalanceBandwidth: strconv.FormatInt(delegatedBandwidth, 10),
	}
}

func (b *TronRPC) SendRawTransaction(tx string, disableAlternativeRPC bool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	resp, err := b.requestBroadcastHex(ctx, strip0xPrefix(tx))
	if err != nil {
		return "", err
	}
	if !resp.Result {
		if resp.Code != "" || resp.Message != "" {
			return "", errors.Errorf("Tron broadcasthex failed: %s %s", resp.Code, resp.Message)
		}
		return "", errors.New("Tron broadcasthex failed")
	}

	txID := strip0xPrefix(resp.TxID)
	if b.ChainConfig != nil && b.ChainConfig.DisableMempoolSync && b.Mempool != nil {
		b.Mempool.AddTransactionToMempool(txID)
	}
	return txID, nil
}

func (b *TronRPC) requestTransactionByID(ctx context.Context, txid string, isSolidified bool) (*tronGetTransactionByIDResponse, error) {
	http := b.getLookupHTTPClient(isSolidified)
	raw, err := requestRawMessage(
		ctx,
		http,
		tronLookupPath(isSolidified, "/wallet/gettransactionbyid", "/walletsolidity/gettransactionbyid"),
		map[string]string{"value": strip0xPrefix(txid)},
	)
	if err != nil {
		return nil, err
	}
	if tronIsEmptyObject(raw) {
		return nil, errors.Annotatef(bchain.ErrTxNotFound, "txid %v", txid)
	}

	var resp tronGetTransactionByIDResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (b *TronRPC) requestTransactionInfoByID(ctx context.Context, txid string, isSolidified bool) (*tronGetTransactionInfoByIDResponse, error) {
	http := b.getLookupHTTPClient(isSolidified)
	raw, err := requestRawMessage(
		ctx,
		http,
		tronLookupPath(isSolidified, "/wallet/gettransactioninfobyid", "/walletsolidity/gettransactioninfobyid"),
		map[string]string{"value": strip0xPrefix(txid)},
	)
	if err != nil {
		return nil, err
	}
	if tronIsEmptyObject(raw) {
		return nil, errors.Annotatef(bchain.ErrTxNotFound, "txid %v", txid)
	}

	var resp tronGetTransactionInfoByIDResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

func (b *TronRPC) requestMempoolTransactions(ctx context.Context) ([]string, error) {
	var resp tronGetTransactionListFromPendingResponse
	if err := b.fullNodeHTTP.Request(ctx, "/wallet/gettransactionlistfrompending", map[string]any{}, &resp); err != nil {
		return nil, err
	}
	if len(resp.TxID) == 0 {
		return []string{}, nil
	}
	return resp.TxID, nil
}

func (b *TronRPC) requestAccountResource(ctx context.Context, address string) (*tronGetAccountResourceResponse, error) {
	req := map[string]any{
		"address": address,
		"visible": true,
	}
	var resp tronGetAccountResourceResponse
	if err := b.fullNodeHTTP.Request(ctx, "/wallet/getaccountresource", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (b *TronRPC) requestAccount(ctx context.Context, address string) (*tronGetAccountResponse, error) {
	req := map[string]any{
		"address": address,
		"visible": true,
	}
	var resp tronGetAccountResponse
	if err := b.fullNodeHTTP.Request(ctx, "/wallet/getaccount", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (b *TronRPC) requestReward(ctx context.Context, address string) (*tronGetRewardResponse, error) {
	req := map[string]any{
		"address": address,
		"visible": true,
	}
	var resp tronGetRewardResponse
	if err := b.fullNodeHTTP.Request(ctx, "/wallet/getReward", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (b *TronRPC) requestBroadcastHex(ctx context.Context, tx string) (*tronBroadcastHexResponse, error) {
	req := map[string]string{
		"transaction": tx,
	}
	http := b.fullNodeHTTP
	if http == nil {
		http = b.getLookupHTTPClient(false)
	}
	var resp tronBroadcastHexResponse
	if err := http.Request(ctx, "/wallet/broadcasthex", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (b *TronRPC) requestTransactionInfoByBlockNum(ctx context.Context, blockNum uint32, isSolidified bool) ([]tronGetTransactionInfoByIDResponse, error) {
	if isSolidified && b.internalDataProvider != nil {
		return b.internalDataProvider.GetTransactionInfoByBlockNum(ctx, blockNum)
	}
	http := b.getLookupHTTPClient(isSolidified)
	raw, err := requestRawMessage(ctx, http, tronLookupPath(isSolidified, "/wallet/gettransactioninfobyblocknum", "/walletsolidity/gettransactioninfobyblocknum"), map[string]any{
		"num": blockNum,
	})
	if err != nil {
		return nil, err
	}
	if tronIsEmptyResponse(raw) {
		return nil, nil
	}

	var resp []tronGetTransactionInfoByIDResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (b *TronRPC) requestBlockByNum(ctx context.Context, blockNum uint32, isSolidified bool) (*tronGetBlockResponse, error) {
	req := map[string]any{
		"num": blockNum,
	}
	http := b.getLookupHTTPClient(isSolidified)
	var resp tronGetBlockResponse
	if err := http.Request(ctx, tronLookupPath(isSolidified, "/wallet/getblockbynum", "/walletsolidity/getblockbynum"), req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (b *TronRPC) requestBlockByID(ctx context.Context, blockHash string, isSolidified bool) (*tronGetBlockResponse, error) {
	req := map[string]string{
		"value": strip0xPrefix(blockHash),
	}
	http := b.getLookupHTTPClient(isSolidified)
	var resp tronGetBlockResponse
	if err := http.Request(ctx, tronLookupPath(isSolidified, "/wallet/getblockbyid", "/walletsolidity/getblockbyid"), req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (b *TronRPC) requestLatestSolidifiedBlockHeight(ctx context.Context) (uint64, error) {
	http := b.solidityNodeHTTP
	if http == nil {
		http = b.getLookupHTTPClient(true)
	}
	var resp tronGetBlockHeaderResponse
	if err := http.Request(ctx, "/walletsolidity/getblock", map[string]any{"detail": false}, &resp); err != nil {
		return 0, err
	}
	if resp.BlockHeader.RawData.Number == nil {
		return 0, errors.New("Tron /walletsolidity/getblock returned missing block_header.raw_data.number")
	}
	return *resp.BlockHeader.RawData.Number, nil
}

func requestRawMessage(ctx context.Context, http TronHTTP, path string, reqBody interface{}) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := http.Request(ctx, path, reqBody, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func tronLookupPath(isSolidified bool, walletPath, walletSolidityPath string) string {
	if isSolidified {
		return walletSolidityPath
	}
	return walletPath
}

func tronIsEmptyObject(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("{}"))
}

func tronIsEmptyArray(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("[]"))
}

func tronIsEmptyResponse(raw json.RawMessage) bool {
	return tronIsEmptyObject(raw) || tronIsEmptyArray(raw)
}

func tronAvailableResource(limit, used int64) int64 {
	if used >= limit {
		return 0
	}
	return limit - used
}
