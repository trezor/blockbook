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

		topType, contractAddr, err := detectTopType(info.InternalTransactions)
		if err != nil {
			return data, contracts, err
		}

		if topType == bchain.CALL && info.ContractAddress != "" {
			topType = bchain.CREATE
			contractAddr = ToTronAddressFromAddress(info.ContractAddress)
		}

		d.Type = topType
		if contractAddr != "" {
			d.Contract = contractAddr
		}

		if topType == bchain.CREATE && contractAddr != "" {
			contracts = append(contracts, bchain.ContractInfo{
				Contract:       contractAddr,
				CreatedInBlock: blockHeight,
				Standard:       bchain.UnhandledTokenStandard,
			})
		} else if topType == bchain.SELFDESTRUCT {
			contracts = append(contracts, bchain.ContractInfo{
				Contract:          contractAddr,
				DestructedInBlock: blockHeight,
			})
		}

		for _, itx := range info.InternalTransactions {

			t, err := tronNoteHexToInternalType(itx.Note)
			if err != nil {
				return data, contracts, err
			}

			from := ToTronAddressFromAddress(itx.CallerAddress)
			to := ToTronAddressFromAddress(itx.TransferToAddress)

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

// we need to figure out the root type of the transaction
// CREATE > SELFDESTRUCT > CALL
func detectTopType(internalTxs []tronInternalTransaction) (
	bchain.EthereumInternalTransactionType,
	string,
	error,
) {
	var createdContract string
	var destructedContract string

	for _, itx := range internalTxs {
		t, err := tronNoteHexToInternalType(itx.Note)
		if err != nil {
			return bchain.CALL, "", err
		}

		switch t {
		case bchain.CALL:
			continue
		case bchain.CREATE:
			if createdContract == "" {
				createdContract = ToTronAddressFromAddress(itx.TransferToAddress)
			}
		case bchain.SELFDESTRUCT:
			if destructedContract == "" {
				destructedContract = ToTronAddressFromAddress(itx.CallerAddress)
			}
		default:
			glog.Warningf("Unknown Tron internal transaction type %v", t)
		}
	}

	if createdContract != "" {
		return bchain.CREATE, createdContract, nil
	}
	if destructedContract != "" {
		return bchain.SELFDESTRUCT, destructedContract, nil
	}
	return bchain.CALL, "", nil
}
