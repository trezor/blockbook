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
	override := tronBalanceHistoryOverride{}
	if len(payload) == 0 {
		return override, false
	}

	var extra bchain.TronChainExtraData
	if err := json.Unmarshal(payload, &extra); err != nil {
		return override, false
	}

	var amountText string
	switch strings.ToLower(strings.TrimSpace(extra.Operation)) {
	case "freeze":
		override.direction = tronBalanceHistoryDirectionOutgoing
		amountText = extra.StakeAmount
	case "unfreeze":
		override.direction = tronBalanceHistoryDirectionIncoming
		amountText = extra.UnstakeAmount
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

func (w *Worker) processTronBalanceHistory(
	addrDesc bchain.AddressDescriptor,
	txid string,
	bchainTx *bchain.Tx,
	selfAddrDesc map[string]struct{},
	ethTxData *bchain.EthereumTxData,
	bh *BalanceHistory,
) error {
	var value big.Int
	if len(bchainTx.Vout) > 0 {
		value = bchainTx.Vout[0].ValueSat
	}

	payload, err := w.chainParser.GetChainExtraData(bchainTx)
	if err != nil {
		// If extra data is unavailable, fall back to generic Ethereum-like accounting.
		return w.processEthereumLikeBalanceHistory(addrDesc, txid, bchainTx, selfAddrDesc, ethTxData, bh)
	}

	override, hasOverride := tronBalanceHistoryOverrideFromExtraData(payload, &value)
	if !hasOverride {
		return w.processEthereumLikeBalanceHistory(addrDesc, txid, bchainTx, selfAddrDesc, ethTxData, bh)
	}

	includeTransferAmount := ethTxData.Status == bchain.TxStatusOK || ethTxData.Status == bchain.TxStatusUnknown
	if includeTransferAmount {
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
			switch override.direction {
			case tronBalanceHistoryDirectionOutgoing:
				(*big.Int)(bh.SentSat).Add((*big.Int)(bh.SentSat), &override.amount)
			case tronBalanceHistoryDirectionIncoming:
				(*big.Int)(bh.ReceivedSat).Add((*big.Int)(bh.ReceivedSat), &override.amount)
			default:
				(*big.Int)(bh.SentSat).Add((*big.Int)(bh.SentSat), &value)
			}
		}
		addEthereumFeesToBalanceHistory(ethTxData, bh)
	}

	return nil
}
