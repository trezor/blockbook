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
		return nil, nil, err
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
		// has no recipient (java-tron's JSON-RPC leaves `to` null solely for
		// CreateSmartContract) AND the node reported the deployed address in the
		// transaction info. contract_address alone is not a creation signal:
		// java-tron fills it for ordinary TriggerSmartContract calls as well.
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

			t, err := tronNoteHexToInternalType(itx.Note)
			if err != nil {
				return data, contracts, err
			}

			// a rejected internal transaction did not execute - it moved no
			// value, created no contract and destroyed none; it only flags
			// the transaction error below
			if itx.Rejected {
				continue
			}

			from := ToTronAddressFromAddress(itx.CallerAddress)
			to := ToTronAddressFromAddress(itx.TransferToAddress)

			// registry events are emitted in note order (parity with eth
			// processCallTrace), so that an ephemeral contract's creation is
			// stored before its destruction merges into it
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
					// Tron internal transactions describe only nested frames,
					// so unlike eth the top-level SELFDESTRUCT type is
					// inferred from the first destroyed contract
					if d.Type == bchain.CALL {
						d.Type = bchain.SELFDESTRUCT
						d.Contract = from
					}
				}
			}

			for _, cv := range itx.CallValueInfo {
				// skip TRC-10
				if cv.CallValue <= 0 || cv.TokenID != "" {
					continue
				}

				val := *big.NewInt(cv.CallValue)
				d.Transfers = append(d.Transfers, bchain.EthereumInternalTransfer{
					Type:  t,
					From:  from,
					To:    to,
					Value: val,
				})
			}
		}

		if info.Receipt.Result != "" && info.Receipt.Result != "SUCCESS" {
			d.Error = info.Receipt.Result
		}

		for _, itx := range info.InternalTransactions {
			if itx.Rejected {
				if d.Error == "" {
					d.Error = "Internal transaction rejected"
				} else {
					d.Error += "; internal transaction rejected"
				}
				break
			}
		}
	}

	return data, contracts, nil
}
