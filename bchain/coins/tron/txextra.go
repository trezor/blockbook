package tron

import (
	"encoding/json"
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

func tronFirstContract(txByID *tronGetTransactionByIDResponse) *tronTxContract {
	if txByID == nil || len(txByID.RawData.Contract) == 0 {
		return nil
	}
	return &txByID.RawData.Contract[0]
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
					if count := tronInt64PtrToString(vote.VoteCount); count != "" {
						extra.Votes = append(extra.Votes, bchain.TronVoteExtra{
							Address: ToTronAddressFromAddress(vote.VoteAddress),
							Count:   count,
						})
					}
				}
			}
		case "FreezeBalanceContract", "FreezeBalanceV2Contract":
			extra.StakeAmount = tronInt64PtrToString(v.FrozenBalance)
		case "UnfreezeBalanceContract", "UnfreezeBalanceV2Contract", "WithdrawExpireUnfreezeContract":
			extra.UnstakeAmount = tronInt64PtrToString(v.UnfreezeBalance)
		case "DelegateResourceContract", "UnDelegateResourceContract":
			extra.DelegateAmount = tronInt64PtrToString(v.Balance)
			extra.DelegateTo = ToTronAddressFromAddress(v.ReceiverAddress)
		}
	}

	extra.AssetIssueID = strings.TrimSpace(txInfo.AssetIssueID)
	extra.TotalFee = tronInt64PtrToString(txInfo.Fee)
	extra.EnergyUsage = tronInt64PtrToString(txInfo.Receipt.EnergyUsage)
	extra.EnergyUsageTotal = tronInt64PtrToString(txInfo.Receipt.EnergyUsageTotal)
	extra.EnergyFee = tronInt64PtrToString(txInfo.Receipt.EnergyFee)
	extra.BandwidthUsage = tronInt64PtrToString(txInfo.Receipt.NetUsage)
	extra.BandwidthFee = tronInt64PtrToString(txInfo.Receipt.NetFee)
	if extra.BandwidthUsage == "" {
		extra.BandwidthUsage = "0"
	}
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
		tx.From = ToTronAddressFromAddress(v.OwnerAddress)
		switch c.Type {
		case "TransferContract", "TransferAssetContract":
			tx.To = strings.TrimSpace(v.ToAddress)
			tx.Value = tronInt64PtrToHexQuantity(v.Amount)
		case "TriggerSmartContract":
			tx.To = strings.TrimSpace(v.ContractAddress)
			tx.Value = tronInt64PtrToHexQuantity(v.CallValue)
			if data := normalizeHexString(v.Data); data != "" {
				tx.Payload = data
			}
		case "FreezeBalanceContract", "FreezeBalanceV2Contract":
			tx.To = tronFirstAddress(v.ReceiverAddress, v.OwnerAddress)
			tx.Value = tronInt64PtrToHexQuantity(v.FrozenBalance)
		case "UnfreezeBalanceContract", "WithdrawExpireUnfreezeContract":
			tx.To = tronFirstAddress(v.ReceiverAddress, v.OwnerAddress)
		case "UnfreezeBalanceV2Contract":
			tx.To = tronFirstAddress(v.ReceiverAddress, v.OwnerAddress)
			tx.Value = tronInt64PtrToHexQuantity(v.UnfreezeBalance)
		case "DelegateResourceContract", "UnDelegateResourceContract":
			tx.To = tronFirstAddress(v.ReceiverAddress, v.ContractAddress, v.ToAddress)
			tx.Value = tronInt64PtrToHexQuantity(v.Balance)
		default:
			tx.To = tronFirstAddress(v.ToAddress, v.ContractAddress, v.ReceiverAddress)
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
	if txInfo.BlockNumber != nil && *txInfo.BlockNumber >= 0 {
		blockNumber = uint64(*txInfo.BlockNumber)
		hasBlockNumber = true
	}
	if txInfo.BlockTimeStamp != nil && *txInfo.BlockTimeStamp >= 0 {
		blockTime = *txInfo.BlockTimeStamp / 1000
	}

	return blockTime, blockNumber, hasBlockNumber
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
