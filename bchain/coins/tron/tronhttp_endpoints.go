package tron

import (
	"context"
	"encoding/json"

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
	FreeNetLimit int64 `json:"freeNetLimit"`
	FreeNetUsed  int64 `json:"freeNetUsed"`
	NetLimit     int64 `json:"NetLimit"`
	NetUsed      int64 `json:"NetUsed"`
	EnergyLimit  int64 `json:"EnergyLimit"`
	EnergyUsed   int64 `json:"EnergyUsed"`
}

type tronGetBlockResponse struct {
	Transactions []tronGetTransactionByIDResponse `json:"transactions,omitempty"`
}

func (b *TronRPC) getTransactionByID(txid string, isMempool bool) (*tronGetTransactionByIDResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	return requestTransactionByID(ctx, b.http, txid, isMempool)
}

func (b *TronRPC) getTransactionInfoByID(txid string, isMempool bool) (*tronGetTransactionInfoByIDResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	return requestTransactionInfoByID(ctx, b.http, txid, isMempool)
}

func (b *TronRPC) GetMempoolTransactions() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	return requestMempoolTransactions(ctx, b.http)
}

// GetAddressChainExtraData returns normalized Tron-specific account/address data.
func (b *TronRPC) GetAddressChainExtraData(addrDesc bchain.AddressDescriptor) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	resp, err := requestAccountResource(ctx, b.http, ToTronAddressFromDesc(addrDesc))
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(bchain.TronAccountExtraData{
		AvailableBandwidth: tronAvailableResource(resp.FreeNetLimit, resp.FreeNetUsed) + tronAvailableResource(resp.NetLimit, resp.NetUsed),
		TotalBandwidth:     resp.FreeNetLimit + resp.NetLimit,
		AvailableEnergy:    tronAvailableResource(resp.EnergyLimit, resp.EnergyUsed),
		TotalEnergy:        resp.EnergyLimit,
	})
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func (b *TronRPC) SendRawTransaction(tx string, disableAlternativeRPC bool) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), b.Timeout)
	defer cancel()

	resp, err := requestBroadcastHex(ctx, b.http, strip0xPrefix(tx))
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

func requestTransactionByID(ctx context.Context, http TronHTTP, txid string, isMempool bool) (*tronGetTransactionByIDResponse, error) {
	raw, err := requestRawMessage(
		ctx,
		http,
		tronLookupPath(isMempool, "/wallet/gettransactionbyid", "/walletsolidity/gettransactionbyid"),
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

func requestTransactionInfoByID(ctx context.Context, http TronHTTP, txid string, isMempool bool) (*tronGetTransactionInfoByIDResponse, error) {
	raw, err := requestRawMessage(
		ctx,
		http,
		tronLookupPath(isMempool, "/wallet/gettransactioninfobyid", "/walletsolidity/gettransactioninfobyid"),
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

func requestMempoolTransactions(ctx context.Context, http TronHTTP) ([]string, error) {
	var resp tronGetTransactionListFromPendingResponse
	if err := http.Request(ctx, "/wallet/gettransactionlistfrompending", map[string]any{}, &resp); err != nil {
		return nil, err
	}
	if len(resp.TxID) == 0 {
		return []string{}, nil
	}
	return resp.TxID, nil
}

func requestAccountResource(ctx context.Context, http TronHTTP, address string) (*tronGetAccountResourceResponse, error) {
	req := map[string]any{
		"address": address,
		"visible": true,
	}
	var resp tronGetAccountResourceResponse
	if err := http.Request(ctx, "/wallet/getaccountresource", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func requestBroadcastHex(ctx context.Context, http TronHTTP, tx string) (*tronBroadcastHexResponse, error) {
	req := map[string]string{
		"transaction": tx,
	}
	var resp tronBroadcastHexResponse
	if err := http.Request(ctx, "/wallet/broadcasthex", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func requestTransactionInfoByBlockNum(ctx context.Context, http TronHTTP, blockNum uint32) ([]tronGetTransactionInfoByIDResponse, error) {
	raw, err := requestRawMessage(ctx, http, "/wallet/gettransactioninfobyblocknum", map[string]any{
		"num": blockNum,
	})
	if err != nil {
		return nil, err
	}
	if tronIsEmptyObject(raw) {
		return nil, nil
	}

	var resp []tronGetTransactionInfoByIDResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	return resp, nil
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

func requestRawMessage(ctx context.Context, http TronHTTP, path string, reqBody interface{}) (json.RawMessage, error) {
	var raw json.RawMessage
	if err := http.Request(ctx, path, reqBody, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func tronLookupPath(isMempool bool, walletPath, walletSolidityPath string) string {
	if isMempool {
		return walletPath
	}
	return walletSolidityPath
}

func tronIsEmptyObject(raw json.RawMessage) bool {
	return string(raw) == "{}"
}

func tronAvailableResource(limit, used int64) int64 {
	if used >= limit {
		return 0
	}
	return limit - used
}
