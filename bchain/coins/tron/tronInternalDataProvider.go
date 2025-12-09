package tron

import (
	"context"
	"math/big"
	"time"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

type TronInternalDataProvider struct {
	http    TronHTTP
	timeout time.Duration
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

func NewTronInternalDataProvider(http TronHTTP, timeout time.Duration) *TronInternalDataProvider {
	return &TronInternalDataProvider{
		http:    http,
		timeout: timeout,
	}
}

func (p *TronInternalDataProvider) GetInternalDataForBlock(
	blockHash string,
	blockHeight uint32,
	transactions []bchain.RpcTransaction,
) ([]bchain.EthereumInternalData, []bchain.ContractInfo, error) {
	data := make([]bchain.EthereumInternalData, len(transactions))
	contracts := make([]bchain.ContractInfo, 0)

	if !eth.ProcessInternalTransactions {
		return data, contracts, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	var infos []tronTxInfo
	req := map[string]any{
		"num": blockHeight,
	}

	if err := p.http.Request(ctx, "/wallet/gettransactioninfobyblocknum", req, &infos); err != nil {
		glog.Errorf("GetInternalDataForBlock: error calling gettransactioninfobyblocknum: %v", err)
		return nil, nil, err
	}

	return buildInternalDataFromTronInfos(infos, transactions, blockHeight)
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
		id := normalizeTxID(infos[i].ID)
		infoByID[id] = &infos[i]
	}

	for i := range transactions {
		tx := &transactions[i]
		key := normalizeTxID(tx.Hash)

		info, ok := infoByID[key]
		if !ok {
			continue
		}

		d := &data[i]

		topType, createdContract, err := detectTopType(info.InternalTransactions)
		if err != nil {
			return data, contracts, err
		}

		if topType == bchain.CALL && info.ContractAddress != "" {
			topType = bchain.CREATE
			createdContract = ToTronAddressFromAddress(info.ContractAddress)
		}

		d.Type = topType

		if createdContract != "" {
			d.Contract = createdContract
			contracts = append(contracts, bchain.ContractInfo{
				Contract:       createdContract,
				CreatedInBlock: blockHeight,
				Standard:       bchain.UnhandledTokenStandard,
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
func detectTopType(internalTxs []tronInternalTransaction) (
	bchain.EthereumInternalTransactionType,
	string,
	error,
) {
	topType := bchain.CALL
	var createdContract string

	for _, itx := range internalTxs {
		t, err := tronNoteHexToInternalType(itx.Note)
		if err != nil {
			return bchain.CALL, "", err
		}

		switch t {
		case bchain.CALL:
			continue
		case bchain.CREATE:
			return bchain.CREATE,
				ToTronAddressFromAddress(itx.TransferToAddress),
				nil

		case bchain.SELFDESTRUCT:
			topType = bchain.SELFDESTRUCT

		default:
			panic("unhandled eth internal transaction type")
		}
	}

	return topType, createdContract, nil
}
