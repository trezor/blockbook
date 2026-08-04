package tron

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"time"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
)

type TronInternalDataProvider struct {
	solidityNodeHTTP TronHTTP
	timeout          time.Duration
}

type tronCallValueInfo struct {
	CallValue int64  `json:"callValue"`
	TokenID   string `json:"tokenId,omitempty"`
}

type tronInternalTransaction struct {
	Hash              string              `json:"hash"`
	CallerAddress     string              `json:"caller_address"`
	TransferToAddress string              `json:"transferTo_address"`
	Note              string              `json:"note"`     // "call", "create", "suicide", ...
	Rejected          bool                `json:"rejected"` // true = fail
	CallValueInfo     []tronCallValueInfo `json:"callValueInfo"`
}

type tronReceipt struct {
	Result string `json:"result"` // "SUCCESS", "REVERT", ...
}

type tronTxInfo struct {
	ID                   string                    `json:"id"`
	BlockNumber          int64                     `json:"blockNumber"`
	ContractAddress      string                    `json:"contract_address"`
	InternalTransactions []tronInternalTransaction `json:"internal_transactions"`
	Receipt              tronReceipt               `json:"receipt"`
}

func NewTronInternalDataProvider(solidityNodeHTTP TronHTTP, timeout time.Duration) *TronInternalDataProvider {
	return &TronInternalDataProvider{
		solidityNodeHTTP: solidityNodeHTTP,
		timeout:          timeout,
	}
}

// GetInternalDataForBlock is not used in production: TronRPC.GetBlock shadows
// EthereumRPC.GetBlock and calls buildInternalDataFromTronInfos directly with
// the tx infos it already fetched for fees/receipts.
func (p *TronInternalDataProvider) GetInternalDataForBlock(
	blockHash string,
	blockHeight uint32,
	transactions []bchain.RpcTransaction,
) ([]bchain.EthereumInternalData, []bchain.ContractInfo, error) {
	data := make([]bchain.EthereumInternalData, len(transactions))
	contracts := make([]bchain.ContractInfo, 0)

	if !bchain.ProcessInternalTransactions {
		return data, contracts, nil
	}
	if len(transactions) == 0 {
		return data, contracts, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	responses, err := p.GetTransactionInfoByBlockNum(ctx, blockHeight)
	if err != nil {
		glog.Errorf("GetInternalDataForBlock: error calling gettransactioninfobyblocknum: %v", err)
		// the caller indexes into data even on error (eth contract), so the
		// slice must keep the size of transactions
		return data, contracts, err
	}
	infos := tronTxInfosFromResponses(responses)

	return buildInternalDataFromTronInfos(infos, transactions, blockHeight)
}

func (p *TronInternalDataProvider) GetTransactionInfoByBlockNum(ctx context.Context, blockNum uint32) ([]tronGetTransactionInfoByIDResponse, error) {
	return p.requestTransactionInfoByBlockNumWithHTTP(ctx, p.solidityNodeHTTP, blockNum)
}

func (p *TronInternalDataProvider) requestTransactionInfoByBlockNumWithHTTP(ctx context.Context, http TronHTTP, blockNum uint32) ([]tronGetTransactionInfoByIDResponse, error) {
	if http == nil {
		return nil, errors.New("Tron internal data provider missing solidity http client")
	}
	var raw json.RawMessage
	if err := http.Request(ctx, "/walletsolidity/gettransactioninfobyblocknum", map[string]any{
		"num": blockNum,
	}, &raw); err != nil {
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

func tronTxInfosFromResponses(responses []tronGetTransactionInfoByIDResponse) []tronTxInfo {
	if len(responses) == 0 {
		return nil
	}
	infos := make([]tronTxInfo, len(responses))
	for i := range responses {
		r := &responses[i]
		info := &infos[i]
		info.ID = r.ID
		info.ContractAddress = r.ContractAddr
		info.InternalTransactions = r.InternalTransactions
		if r.BlockNumber != nil {
			info.BlockNumber = *r.BlockNumber
		}
		info.Receipt.Result = r.Receipt.Result
	}
	return infos
}

// internal transaction format described at https://developers.tron.network/docs/tron-protocol-transaction#internal-transactions
func buildInternalDataFromTronInfos(
	infos []tronTxInfo,
	transactions []bchain.RpcTransaction,
	blockHeight uint32,
) ([]bchain.EthereumInternalData, []bchain.ContractInfo, error) {

	data := make([]bchain.EthereumInternalData, len(transactions))
	contracts := make([]bchain.ContractInfo, 0)

	// make sure the tx order is correct
	infoByID := make(map[string]*tronTxInfo, len(infos))
	for i := range infos {
		id := infos[i].ID
		infoByID[id] = &infos[i]
	}

	for i := range transactions {
		tx := &transactions[i]
		key := strip0xPrefix(tx.Hash)

		info, ok := infoByID[key]
		if !ok {
			continue
		}

		d := &data[i]

		// A transaction deploys a contract only when its eth-style representation
		// has no recipient AND the node reported the deployed address. Neither
		// signal alone discriminates: java-tron fills contract_address for
		// ordinary TriggerSmartContract calls too, and `to` is null also for
		// native non-VM operations (FreezeBalance, WithdrawBalance, ...), which
		// have an empty contract_address.
		deployedContract := ""
		if tx.To == "" && info.ContractAddress != "" {
			deployedContract = ToTronAddressFromAddress(info.ContractAddress)
			d.Type = bchain.CREATE
			d.Contract = deployedContract
			contracts = append(contracts, bchain.ContractInfo{
				Contract:       deployedContract,
				CreatedInBlock: blockHeight,
				Standard:       bchain.UnhandledTokenStandard,
			})
		}

		for _, itx := range info.InternalTransactions {

			note, err := decodeNoteHex(itx.Note)
			if err != nil {
				return data, contracts, err
			}

			// a rejected internal transaction did not execute - it moved no
			// value, created no contract and destroyed none
			if itx.Rejected {
				continue
			}

			t, handled := tronNoteToInternalType(note)
			if !handled {
				// featured frames carry the staked/delegated amount in
				// callValue although no TRX moves - never book them
				glog.V(1).Infof("Tron: skipping internal transaction note %q in tx %s", note, info.ID)
				continue
			}

			from := ToTronAddressFromAddress(itx.CallerAddress)
			to := ToTronAddressFromAddress(itx.TransferToAddress)

			// registry events are emitted in execution order (parity with eth
			// processCallTrace); storeContractInfo merges an ephemeral
			// contract's destruction into its same-block creation
			switch t {
			case bchain.CREATE:
				// nested create frames register the child contract but do not
				// change the top-level type - factory calls remain CALLs
				if to != "" && to != deployedContract {
					contracts = append(contracts, bchain.ContractInfo{
						Contract:       to,
						CreatedInBlock: blockHeight,
						Standard:       bchain.UnhandledTokenStandard,
					})
				}
			case bchain.SELFDESTRUCT:
				if from != "" {
					contracts = append(contracts, bchain.ContractInfo{
						Contract:          from,
						DestructedInBlock: blockHeight,
					})
				}
			}

			// java-tron puts at most one TRX entry in callValueInfo (extras
			// are TRC-10), so like eth this emits one transfer per frame
			transferEmitted := false
			for _, cv := range itx.CallValueInfo {
				// skip TRC-10
				if cv.CallValue <= 0 || cv.TokenID != "" {
					continue
				}
				transferEmitted = true

				val := *big.NewInt(cv.CallValue)
				d.Transfers = append(d.Transfers, bchain.EthereumInternalTransfer{
					Type:  t,
					From:  from,
					To:    to,
					Value: val,
				})
			}

			// eth processCallTrace parity: create and suicide frames emit a
			// transfer even when no TRX moved, so that the created/destroyed
			// contract's own address history contains this transaction - plain
			// zero-value calls stay skipped, and the root deployment is already
			// indexed through d.Contract
			if !transferEmitted && ((t == bchain.CREATE && to != "" && to != deployedContract) || (t == bchain.SELFDESTRUCT && from != "")) {
				d.Transfers = append(d.Transfers, bchain.EthereumInternalTransfer{
					Type: t,
					From: from,
					To:   to,
				})
			}
		}

		if info.Receipt.Result != "" && info.Receipt.Result != "SUCCESS" {
			d.Error = info.Receipt.Result
		}
	}

	return data, contracts, nil
}
