package api

import (
	"bytes"
	"encoding/json"
	"math/big"
	"strings"

	"github.com/trezor/blockbook/bchain"
)

type tronBalanceHistoryDirection int

const (
	tronBalanceHistoryDirectionNone tronBalanceHistoryDirection = iota
	tronBalanceHistoryDirectionOutgoing
	tronBalanceHistoryDirectionIncoming
)

type tronBalanceHistoryOverride struct {
	direction tronBalanceHistoryDirection
	amount    big.Int
}

func parseBase10BigInt(value string) (*big.Int, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}
	a, ok := new(big.Int).SetString(value, 10)
	return a, ok
}

func tronBalanceHistoryOverrideFromExtraData(payload json.RawMessage, fallbackValue *big.Int) (tronBalanceHistoryOverride, bool) {
	if len(payload) == 0 {
		return tronBalanceHistoryOverride{}, false
	}
	var extra bchain.TronChainExtraData
	if err := json.Unmarshal(payload, &extra); err != nil {
		return tronBalanceHistoryOverride{}, false
	}
	return tronBalanceHistoryOverrideFromExtraDataParsed(&extra, fallbackValue)
}

func tronBalanceHistoryOverrideFromExtraDataParsed(extra *bchain.TronChainExtraData, fallbackValue *big.Int) (tronBalanceHistoryOverride, bool) {
	override := tronBalanceHistoryOverride{}
	if extra == nil {
		return override, false
	}

	var amountText string
	switch extra.Operation {
	case "freeze":
		override.direction = tronBalanceHistoryDirectionOutgoing
		amountText = extra.StakeAmount
	case "withdraw":
		override.direction = tronBalanceHistoryDirectionIncoming
		amountText = extra.UnstakeAmount
	case "VoteRewardAmount":
		override.direction = tronBalanceHistoryDirectionIncoming
		amountText = extra.ClaimedVoteReward
	case "unfreeze":
		// Unfreeze starts unlock period but funds are not yet spendable.
		// Do not account principal movement in balance history at this stage.
		override.direction = tronBalanceHistoryDirectionNone
		override.amount.SetInt64(0)
		return override, true
	default:
		return override, false
	}

	if a, ok := parseBase10BigInt(amountText); ok {
		override.amount.Set(a)
	} else if fallbackValue != nil {
		override.amount.Set(fallbackValue)
	} else {
		override.amount.SetInt64(0)
	}

	return override, true
}

func tronBalanceHistoryFeeFromExtraDataParsed(extra *bchain.TronChainExtraData) big.Int {
	var fee big.Int
	if extra == nil {
		return fee
	}
	if a, ok := parseBase10BigInt(extra.TotalFee); ok {
		fee.Set(a)
	}
	return fee
}

func (w *Worker) processTronBalanceHistory(
	addrDesc bchain.AddressDescriptor,
	txid string,
	bchainTx *bchain.Tx,
	selfAddrDesc map[string]struct{},
	ethTxData *bchain.EthereumTxData,
	bh *BalanceHistory,
) error {
	// Value is kept as fallback amount source when chainExtra amount is absent.
	var value big.Int
	if len(bchainTx.Vout) > 0 {
		value = bchainTx.Vout[0].ValueSat
	}

	// Tron balance history is operation-driven (freeze/unfreeze/withdraw),
	// not purely based on Ethereum-like Vout semantics
	var extra *bchain.TronChainExtraData
	payload, err := w.chainParser.GetChainExtraData(bchainTx)
	if err == nil {
		var parsed bchain.TronChainExtraData
		if unmarshalErr := json.Unmarshal(payload, &parsed); unmarshalErr == nil {
			extra = &parsed
		}
	}
	feeSat := tronBalanceHistoryFeeFromExtraDataParsed(extra)

	override, hasOverride := tronBalanceHistoryOverrideFromExtraDataParsed(extra, &value)

	includeTransferAmount := ethTxData.Status == bchain.TxStatusOK || ethTxData.Status == bchain.TxStatusUnknown
	countSentToSelf := false
	if includeTransferAmount {
		// For non-overridden Tron operations, keep generic Ethereum-like
		// principal movement semantics.
		if !hasOverride {
			countSentToSelf, err = w.processPrimaryVoutForBalanceHistory(addrDesc, bchainTx, selfAddrDesc, bh)
			if err != nil {
				return err
			}
		}

		// Internal transfers remain shared accounting for call-style transactions.
		if err := w.processInternalTransactionsForBalanceHistory(addrDesc, txid, bh); err != nil {
			return err
		}
	}

	for i := range bchainTx.Vin {
		bchainVin := &bchainTx.Vin[i]
		if len(bchainVin.Addresses) == 0 {
			continue
		}

		txAddrDesc, err := w.chainParser.GetAddrDescFromAddress(bchainVin.Addresses[0])
		if err != nil {
			return err
		}
		if !bytes.Equal(addrDesc, txAddrDesc) {
			continue
		}

		if includeTransferAmount {
			if hasOverride {
				switch override.direction {
				case tronBalanceHistoryDirectionOutgoing:
					(*big.Int)(bh.SentSat).Add((*big.Int)(bh.SentSat), &override.amount)
				case tronBalanceHistoryDirectionIncoming:
					(*big.Int)(bh.ReceivedSat).Add((*big.Int)(bh.ReceivedSat), &override.amount)
				case tronBalanceHistoryDirectionNone:
					// Explicitly no principal movement for this operation.
				}
			} else {
				(*big.Int)(bh.SentSat).Add((*big.Int)(bh.SentSat), &value)
				if countSentToSelf {
					if _, found := selfAddrDesc[string(txAddrDesc)]; found {
						(*big.Int)(bh.SentToSelfSat).Add((*big.Int)(bh.SentToSelfSat), &value)
					}
				}
			}
		}
		// Fees always reduce spendable balance for sender-side matches.
		(*big.Int)(bh.SentSat).Add((*big.Int)(bh.SentSat), &feeSat)
	}

	return nil
}
