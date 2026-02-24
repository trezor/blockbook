//go:build integration

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func (h *TestHandler) getStatus(t *testing.T) *statusBlockbook {
	if h.status != nil {
		return h.status
	}

	var envelope statusEnvelope
	h.mustGetJSON(t, "/api/status", &envelope)
	if !hasNonEmptyObject(envelope.Blockbook) {
		t.Fatalf("status response missing non-empty blockbook object")
	}
	if !hasNonEmptyObject(envelope.Backend) {
		t.Fatalf("status response missing non-empty backend object")
	}

	var bb statusBlockbook
	if err := json.Unmarshal(envelope.Blockbook, &bb); err != nil {
		t.Fatalf("decode status blockbook object: %v", err)
	}
	if bb.BestHeight <= 0 {
		t.Fatalf("invalid status bestHeight: %d", bb.BestHeight)
	}

	h.status = &bb
	return h.status
}

func (h *TestHandler) findTransactionNearHeight(t *testing.T, fromHeight, window int) (txid string, height int, hash string, found bool) {
	lower := fromHeight - window
	if lower < 0 {
		lower = 0
	}

	for height = fromHeight; height >= lower; height-- {
		hash, ok := h.getBlockHashForHeight(t, height, false)
		if !ok {
			continue
		}
		blk, ok := h.getBlockByHash(t, hash, false)
		if !ok {
			continue
		}
		if len(blk.TxIDs) == 0 {
			continue
		}
		txid = strings.TrimSpace(blk.TxIDs[0])
		if txid == "" {
			continue
		}
		return txid, height, hash, true
	}

	return "", 0, "", false
}

func (h *TestHandler) getSampleTxID(t *testing.T) (string, bool) {
	if h.sampleTxResolved {
		return h.sampleTxID, h.sampleTxID != ""
	}

	status := h.getStatus(t)
	txid, _, _, found := h.findTransactionNearHeight(t, status.BestHeight, txSearchWindow)
	h.sampleTxResolved = true
	if !found {
		return "", false
	}
	h.sampleTxID = txid
	return h.sampleTxID, true
}

func (h *TestHandler) getSampleAddress(t *testing.T) (string, bool) {
	if h.sampleAddrResolved {
		return h.sampleAddress, h.sampleAddress != ""
	}

	txid, found := h.getSampleTxID(t)
	h.sampleAddrResolved = true
	if !found {
		return "", false
	}

	tx, ok := h.getTransactionByID(t, txid, false)
	if !ok {
		return "", false
	}

	h.sampleAddress = firstAddressFromTx(tx)
	return h.sampleAddress, h.sampleAddress != ""
}

func firstAddressFromTx(tx *txDetailResponse) string {
	for i := range tx.Vout {
		for _, addr := range tx.Vout[i].Addresses {
			if strings.TrimSpace(addr) != "" {
				return addr
			}
		}
	}
	for i := range tx.Vin {
		for _, addr := range tx.Vin[i].Addresses {
			if strings.TrimSpace(addr) != "" {
				return addr
			}
		}
	}
	return ""
}

func (h *TestHandler) getTransactionByID(t *testing.T, txid string, strict bool) (*txDetailResponse, bool) {
	if tx, found := h.txByID[txid]; found {
		return tx, true
	}

	path := "/api/v2/tx/" + url.PathEscape(txid)
	status, body := h.getHTTP(t, path)
	if status != http.StatusOK {
		if strict {
			t.Fatalf("GET %s returned HTTP %d: %s", path, status, preview(body))
		}
		return nil, false
	}

	var tx txDetailResponse
	if err := json.Unmarshal(body, &tx); err != nil {
		t.Fatalf("decode transaction response for %s: %v", txid, err)
	}

	if tx.Txid == "" {
		if strict {
			t.Fatalf("empty txid in transaction response for %s", txid)
		}
		return nil, false
	}
	if tx.Txid != txid {
		if strict {
			t.Fatalf("transaction mismatch: got %s, want %s", tx.Txid, txid)
		}
		return nil, false
	}

	h.txByID[txid] = &tx
	return &tx, true
}

func (h *TestHandler) getBlockHashForHeight(t *testing.T, height int, strict bool) (string, bool) {
	if hash, found := h.blockHashByHeight[height]; found {
		return hash, true
	}

	path := fmt.Sprintf("/api/v2/block-index/%d", height)
	status, body := h.getHTTP(t, path)
	if status != http.StatusOK {
		if strict {
			t.Fatalf("GET %s returned HTTP %d: %s", path, status, preview(body))
		}
		return "", false
	}

	var res blockIndexResponse
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode block-index response at height %d: %v", height, err)
	}
	res.BlockHash = strings.TrimSpace(res.BlockHash)
	if res.BlockHash == "" {
		if strict {
			t.Fatalf("empty blockHash for height %d", height)
		}
		return "", false
	}

	h.blockHashByHeight[height] = res.BlockHash
	return res.BlockHash, true
}

func (h *TestHandler) getBlockByHash(t *testing.T, hash string, strict bool) (*blockSummary, bool) {
	if blk, found := h.blockByHash[hash]; found {
		return blk, true
	}

	path := fmt.Sprintf("/api/v2/block/%s?page=1&pageSize=%d", url.PathEscape(hash), blockPageSize)
	status, body := h.getHTTP(t, path)
	if status != http.StatusOK {
		if strict {
			t.Fatalf("GET %s returned HTTP %d: %s", path, status, preview(body))
		}
		return nil, false
	}

	var res blockResponse
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode block response for %s: %v", hash, err)
	}

	blk := &blockSummary{
		Hash:       strings.TrimSpace(res.Hash),
		Height:     res.Height,
		HasTxField: res.Txs != nil,
		TxIDs:      extractTxIDs(t, res.Txs),
	}
	if blk.Hash == "" {
		if strict {
			t.Fatalf("empty hash in block response for %s", hash)
		}
		return nil, false
	}

	h.blockByHash[hash] = blk
	return blk, true
}

func extractTxIDs(t *testing.T, txs []json.RawMessage) []string {
	t.Helper()
	if txs == nil {
		return nil
	}

	txids := make([]string, 0, len(txs))
	for i := range txs {
		raw := txs[i]
		var asString string
		if err := json.Unmarshal(raw, &asString); err == nil {
			asString = strings.TrimSpace(asString)
			if asString != "" {
				txids = append(txids, asString)
			}
			continue
		}

		var asObject struct {
			Txid string `json:"txid"`
			Hash string `json:"hash"`
		}
		if err := json.Unmarshal(raw, &asObject); err != nil {
			t.Fatalf("unexpected tx format at index %d: %v", i, err)
		}
		txid := strings.TrimSpace(asObject.Txid)
		if txid == "" {
			txid = strings.TrimSpace(asObject.Hash)
		}
		if txid != "" {
			txids = append(txids, txid)
		}
	}

	return txids
}

func hasNonEmptyObject(raw json.RawMessage) bool {
	v := strings.TrimSpace(string(raw))
	return v != "" && v != "null" && v != "{}"
}
