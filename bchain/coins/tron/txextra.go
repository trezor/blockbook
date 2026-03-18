package tron

import (
	"context"
	"encoding/json"
	"math/big"
	"strconv"
	"strings"

	"github.com/trezor/blockbook/bchain"
)

type tronGetTransactionInfoByIDResponse struct {
	ID                   string                    `json:"id,omitempty"`
	Fee                  *int64                    `json:"fee,omitempty"`
	BlockNumber          *int64                    `json:"blockNumber,omitempty"`
	BlockTimeStamp       *int64                    `json:"blockTimeStamp,omitempty"`
	ContractResult       []string                  `json:"contractResult,omitempty"`
	ContractAddr         string                    `json:"contract_address,omitempty"`
	Result               string                    `json:"result,omitempty"` // omitted on success, FAILED on error
	ResMessage           string                    `json:"resMessage,omitempty"`
	AssetIssueID         string                    `json:"assetIssueID,omitempty"`
	WithdrawAmount       *int64                    `json:"withdraw_amount,omitempty"`
	UnfreezeAmount       *int64                    `json:"unfreeze_amount,omitempty"`
	InternalTransactions []tronInternalTransaction `json:"internal_transactions,omitempty"`
	WithdrawExpireAmount *int64                    `json:"withdraw_expire_amount,omitempty"`
	Receipt              struct {
		Result             string `json:"result"`
		EnergyUsage        *int64 `json:"energy_usage,omitempty"`
		EnergyUsageTotal   *int64 `json:"energy_usage_total,omitempty"`
		EnergyFee          *int64 `json:"energy_fee,omitempty"`
		OriginEnergyUsage  *int64 `json:"origin_energy_usage,omitempty"`
		NetUsage           *int64 `json:"net_usage,omitempty"`
		NetFee             *int64 `json:"net_fee,omitempty"`
		EnergyPenaltyTotal *int64 `json:"energy_penalty_total,omitempty"`
	} `json:"receipt"`
	Log []*bchain.RpcLog `json:"log,omitempty"`
}

func tronOperationFromContractType(contractType string) string {
	switch contractType {
	case "VoteWitnessContract":
		return "vote"
	case "FreezeBalanceContract", "FreezeBalanceV2Contract":
		return "freeze"
	case "UnfreezeBalanceContract", "UnfreezeBalanceV2Contract", "WithdrawExpireUnfreezeContract":
		return "unfreeze"
	case "DelegateResourceContract":
		return "delegate"
	case "UnDelegateResourceContract":
		return "undelegate"
	case "TransferContract":
		return "transfer"
	case "TransferAssetContract":
		return "trc10Transfer"
	case "TriggerSmartContract":
		return "contractCall"
	default:
		return ""
	}
}

func tronNumberToString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(t)
	case float64:
		return strconv.FormatInt(int64(t), 10)
	case float32:
		return strconv.FormatInt(int64(t), 10)
	case int:
		return strconv.FormatInt(int64(t), 10)
	case int8:
		return strconv.FormatInt(int64(t), 10)
	case int16:
		return strconv.FormatInt(int64(t), 10)
	case int32:
		return strconv.FormatInt(int64(t), 10)
	case int64:
		return strconv.FormatInt(t, 10)
	case uint:
		return strconv.FormatUint(uint64(t), 10)
	case uint8:
		return strconv.FormatUint(uint64(t), 10)
	case uint16:
		return strconv.FormatUint(uint64(t), 10)
	case uint32:
		return strconv.FormatUint(uint64(t), 10)
	case uint64:
		return strconv.FormatUint(t, 10)
	case json.Number:
		return t.String()
	default:
		return ""
	}
}

func tronDecimalToHexQuantity(v interface{}) string {
	s := tronNumberToString(v)
	if s == "" {
		return ""
	}
	n, ok := new(big.Int).SetString(strings.TrimSpace(s), 0)
	if !ok {
		n, ok = new(big.Int).SetString(strings.TrimSpace(s), 10)
	}
	if !ok {
		return ""
	}
	if n.Sign() < 0 {
		return "0x0"
	}
	return "0x" + n.Text(16)
}

func tronResourceToString(v interface{}) string {
	s := strings.ToUpper(tronNumberToString(v))
	switch s {
	case "ENERGY", "1":
		return "energy"
	case "BANDWIDTH", "0":
		return "bandwidth"
	default:
		return ""
	}
}

func tronInt64PtrToString(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}

func tronInt64PtrToHexQuantity(v *int64) string {
	if v == nil {
		return ""
	}
	n := big.NewInt(*v)
	if n.Sign() < 0 {
		return ""
	}
	return "0x" + n.Text(16)
}

func tronInt64PtrToUint64(v *int64) (uint64, bool) {
	if v == nil || *v < 0 {
		return 0, false
	}
	return uint64(*v), true
}

func tronUint64(v interface{}) (uint64, bool) {
	s := strings.TrimSpace(tronNumberToString(v))
	if s == "" {
		return 0, false
	}
	n, ok := new(big.Int).SetString(s, 0)
	if !ok || n.Sign() < 0 || !n.IsUint64() {
		return 0, false
	}
	return n.Uint64(), true
}

func tronFirstContract(txByID *tronGetTransactionByIDResponse) *tronTxContract {
	if txByID == nil || len(txByID.RawData.Contract) == 0 {
		return nil
	}
	return &txByID.RawData.Contract[0]
}

func tronFirstAddress(values ...string) string {
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return ""
}

func tronAddressToBase58(address string) string {
	address = strings.TrimSpace(address)
	if address == "" {
		return ""
	}
	return ToTronAddressFromAddress(address)
}

func tronFirstHexQuantity(values ...interface{}) string {
	for _, v := range values {
		if s := tronDecimalToHexQuantity(v); s != "" {
			return s
		}
	}
	return ""
}

func tronNormalizeLogs(logs []*bchain.RpcLog) []*bchain.RpcLog {
	for _, l := range logs {
		if l == nil {
			continue
		}
		l.Address = normalizeHexString(l.Address)
		l.Data = normalizeHexString(l.Data)
		for i, t := range l.Topics {
			l.Topics[i] = normalizeHexString(t)
		}
	}
	return logs
}

func tronBuildExtraData(txByID *tronGetTransactionByIDResponse, txInfo *tronGetTransactionInfoByIDResponse) bchain.TronChainExtraData {
	extra := bchain.TronChainExtraData{}
	extra.FeeLimit = tronInt64PtrToString(txByID.RawData.FeeLimit)

	if c := tronFirstContract(txByID); c != nil {
		extra.ContractType = c.Type
		extra.Operation = tronOperationFromContractType(c.Type)
		v := c.Parameter.Value
		extra.Resource = tronResourceToString(v.Resource)
		switch c.Type {
		case "VoteWitnessContract":
			if len(v.Votes) > 0 {
				extra.Votes = make([]bchain.TronVoteExtra, 0, len(v.Votes))
				for _, vote := range v.Votes {
					if count := tronNumberToString(vote.VoteCount); count != "" {
						extra.Votes = append(extra.Votes, bchain.TronVoteExtra{
							Address: tronAddressToBase58(vote.VoteAddress),
							Count:   count,
						})
					}
				}
			}
		case "FreezeBalanceContract", "FreezeBalanceV2Contract":
			extra.StakeAmount = tronNumberToString(v.FrozenBalance)
			if extra.StakeAmount == "" {
				extra.StakeAmount = tronNumberToString(v.Amount)
			}
		case "UnfreezeBalanceContract", "UnfreezeBalanceV2Contract", "WithdrawExpireUnfreezeContract":
			extra.UnstakeAmount = tronNumberToString(v.UnfreezeBalance)
			if extra.UnstakeAmount == "" {
				extra.UnstakeAmount = tronNumberToString(v.Balance)
			}
			if extra.UnstakeAmount == "" {
				extra.UnstakeAmount = tronNumberToString(v.Amount)
			}
		case "DelegateResourceContract", "UnDelegateResourceContract":
			extra.DelegateAmount = tronNumberToString(v.Balance)
			if extra.DelegateAmount == "" {
				extra.DelegateAmount = tronNumberToString(v.Amount)
			}
			extra.DelegateTo = tronAddressToBase58(tronFirstAddress(v.ReceiverAddress, v.ContractAddress, v.ToAddress))
		}
	}

	extra.AssetIssueID = strings.TrimSpace(txInfo.AssetIssueID)
	extra.TotalFee = tronInt64PtrToString(txInfo.Fee)
	extra.EnergyUsage = tronInt64PtrToString(txInfo.Receipt.EnergyUsage)
	extra.EnergyUsageTotal = tronInt64PtrToString(txInfo.Receipt.EnergyUsageTotal)
	extra.EnergyFee = tronInt64PtrToString(txInfo.Receipt.EnergyFee)
	extra.BandwidthUsage = tronInt64PtrToString(txInfo.Receipt.NetUsage)
	extra.BandwidthFee = tronInt64PtrToString(txInfo.Receipt.NetFee)
	extra.Result = strings.TrimSpace(txInfo.Receipt.Result)
	if extra.Result == "" {
		extra.Result = strings.TrimSpace(txInfo.Result)
	}
	if extra.UnstakeAmount == "" {
		extra.UnstakeAmount = tronInt64PtrToString(txInfo.UnfreezeAmount)
	}

	return extra
}

func tronBuildRpcReceipt(txInfo *tronGetTransactionInfoByIDResponse) *bchain.RpcReceipt {
	receipt := &bchain.RpcReceipt{}
	if strings.TrimSpace(txInfo.Result) == "" {
		receipt.Status = "0x1" // success
	} else {
		receipt.Status = "0x0" // failed
	}

	if gasUsed := tronInt64PtrToHexQuantity(txInfo.Receipt.EnergyUsageTotal); gasUsed != "" {
		receipt.GasUsed = gasUsed
	}
	if txInfo.ContractAddr != "" {
		receipt.ContractAddress = normalizeHexString(txInfo.ContractAddr)
	}
	logs := txInfo.Log
	if len(logs) > 0 {
		receipt.Logs = tronNormalizeLogs(logs)
	}

	if receipt.Status == "" && receipt.GasUsed == "" && len(receipt.Logs) == 0 && receipt.ContractAddress == "" {
		return nil
	}
	return receipt
}

func tronBuildRpcTransaction(txByID *tronGetTransactionByIDResponse, txInfo *tronGetTransactionInfoByIDResponse) *bchain.RpcTransaction {
	tx := &bchain.RpcTransaction{
		AccountNonce:     "0x0",
		GasPrice:         "0x0",
		GasLimit:         "0x0",
		Value:            "0x0",
		Payload:          "0x",
		Hash:             normalizeHexString(txByID.TxID),
		TransactionIndex: "0x0",
	}
	if gasLimit := tronInt64PtrToHexQuantity(txByID.RawData.FeeLimit); gasLimit != "" {
		tx.GasLimit = gasLimit
	}
	if c := tronFirstContract(txByID); c != nil {
		v := c.Parameter.Value
		tx.From = strings.TrimSpace(v.OwnerAddress)
		switch c.Type {
		case "TransferContract", "TransferAssetContract":
			tx.To = strings.TrimSpace(v.ToAddress)
			tx.Value = tronFirstHexQuantity(v.Amount)
		case "TriggerSmartContract":
			tx.To = strings.TrimSpace(v.ContractAddress)
			tx.Value = tronFirstHexQuantity(v.CallValue)
			if data := normalizeHexString(v.Data); data != "" {
				tx.Payload = data
			}
		case "FreezeBalanceContract", "FreezeBalanceV2Contract":
			tx.To = tronFirstAddress(v.ReceiverAddress, v.OwnerAddress)
			tx.Value = tronFirstHexQuantity(v.FrozenBalance, v.Amount)
		case "UnfreezeBalanceContract", "UnfreezeBalanceV2Contract", "WithdrawExpireUnfreezeContract":
			tx.To = tronFirstAddress(v.ReceiverAddress, v.OwnerAddress)
			tx.Value = tronFirstHexQuantity(v.UnfreezeBalance, v.Balance, v.Amount)
		case "DelegateResourceContract", "UnDelegateResourceContract":
			tx.To = tronFirstAddress(v.ReceiverAddress, v.ContractAddress, v.ToAddress)
			tx.Value = tronFirstHexQuantity(v.Balance, v.Amount)
		default:
			tx.To = tronFirstAddress(v.ToAddress, v.ContractAddress, v.ReceiverAddress)
			tx.Value = tronFirstHexQuantity(v.Amount, v.CallValue, v.FrozenBalance, v.UnfreezeBalance, v.Balance)
			if tx.Payload == "0x" {
				if data := normalizeHexString(v.Data); data != "" {
					tx.Payload = data
				}
			}
		}
	}

	if bn := tronInt64PtrToHexQuantity(txInfo.BlockNumber); bn != "" {
		tx.BlockNumber = bn
	}

	if tx.Value == "" {
		tx.Value = "0x0"
	}
	return tx
}

func tronBuildEthereumSpecificData(txByID *tronGetTransactionByIDResponse, txInfo *tronGetTransactionInfoByIDResponse) bchain.EthereumSpecificData {
	csd := bchain.EthereumSpecificData{
		Tx:      tronBuildRpcTransaction(txByID, txInfo),
		Receipt: tronBuildRpcReceipt(txInfo),
	}
	extra := tronBuildExtraData(txByID, txInfo)
	if m, err := json.Marshal(extra); err == nil {
		csd.ChainExtraData = m
	}
	return csd
}

func tronTxMeta(txInfo *tronGetTransactionInfoByIDResponse) (int64, uint64, bool) {
	var (
		blockTime      int64
		blockNumber    uint64
		hasBlockNumber bool
	)
	if n, ok := tronInt64PtrToUint64(txInfo.BlockNumber); ok {
		blockNumber = n
		hasBlockNumber = true
	}
	if ts, ok := tronInt64PtrToUint64(txInfo.BlockTimeStamp); ok {
		blockTime = int64(ts / 1000)
	}

	return blockTime, blockNumber, hasBlockNumber
}

func requestTransactionInfoByBlockNum(ctx context.Context, http TronHTTP, blockNum uint32) ([]tronGetTransactionInfoByIDResponse, error) {
	req := map[string]any{
		"num": blockNum,
	}
	var raw json.RawMessage
	if err := http.Request(ctx, "/wallet/gettransactioninfobyblocknum", req, &raw); err != nil {
		return nil, err
	}

	if string(raw) == "{}" {
		return nil, nil
	}

	var resp []tronGetTransactionInfoByIDResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

type tronGetBlockResponse struct {
	Transactions []tronGetTransactionByIDResponse `json:"transactions,omitempty"`
}

func requestBlockByNum(ctx context.Context, http TronHTTP, blockNum uint32) (*tronGetBlockResponse, error) {
	req := map[string]any{
		"num": blockNum,
	}
	var resp tronGetBlockResponse
	if err := http.Request(ctx, "/wallet/getblockbynum", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func requestBlockByID(ctx context.Context, http TronHTTP, blockHash string) (*tronGetBlockResponse, error) {
	req := map[string]string{
		"value": strip0xPrefix(blockHash),
	}
	var resp tronGetBlockResponse
	if err := http.Request(ctx, "/wallet/getblockbyid", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func mapTransactionInfoByID(infos []tronGetTransactionInfoByIDResponse) map[string]*tronGetTransactionInfoByIDResponse {
	if len(infos) == 0 {
		return nil
	}
	r := make(map[string]*tronGetTransactionInfoByIDResponse, len(infos))
	for i := range infos {
		txInfo := &infos[i]
		id := txInfo.ID
		if id == "" {
			continue
		}
		r[id] = txInfo
	}
	return r
}

func mapTransactionByID(txs []tronGetTransactionByIDResponse) map[string]*tronGetTransactionByIDResponse {
	if len(txs) == 0 {
		return nil
	}
	r := make(map[string]*tronGetTransactionByIDResponse, len(txs))
	for i := range txs {
		txByID := &txs[i]
		id := txByID.TxID
		if id == "" {
			continue
		}
		r[id] = txByID
	}
	return r
}
