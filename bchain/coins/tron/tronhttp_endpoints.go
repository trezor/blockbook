package tron

import (
	"bytes"
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

	resp, err := b.requestAccountResource(ctx, ToTronAddressFromDesc(addrDesc))
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
