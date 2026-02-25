//go:build integration

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func testStatus(t *testing.T, h *TestHandler) {
	_ = h.getStatus(t)
}

func testGetBlockIndex(t *testing.T, h *TestHandler) {
	height, _, ok := h.getSampleIndexedHeight(t)
	if !ok {
		t.Fatalf("missing indexed block hash in recent height window near %d", h.getStatus(t).BestHeight)
	}
	if _, ok := h.getBlockHashForHeight(t, height, true); !ok {
		t.Fatalf("missing block hash for sampled height %d", height)
	}
}

func testGetBlock(t *testing.T, h *TestHandler) {
	height, bestHash, ok := h.getSampleIndexedBlock(t)
	if !ok {
		t.Fatalf("missing indexed block hash in recent height window near %d", h.getStatus(t).BestHeight)
	}

	blk, ok := h.getBlockByHash(t, bestHash, true)
	if !ok {
		t.Fatalf("missing block for hash %s", bestHash)
	}
	assertEqualString(t, blk.Hash, bestHash, "block hash")
	if blk.Height != height {
		t.Fatalf("block height mismatch: got %d, want %d", blk.Height, height)
	}
	if !blk.HasTxField {
		t.Fatalf("block response missing txs field")
	}
}

func testGetBlockByHeight(t *testing.T, h *TestHandler) {
	height, _, ok := h.getSampleIndexedBlock(t)
	if !ok {
		t.Fatalf("missing indexed block hash in recent height window near %d", h.getStatus(t).BestHeight)
	}

	path := fmt.Sprintf("/api/v2/block/%d?page=1&pageSize=%d", height, blockPageSize)
	var blk blockResponse
	h.mustGetJSON(t, path, &blk)

	assertNonEmptyString(t, blk.Hash, "GetBlockByHeight.hash")
	if blk.Height != height {
		t.Fatalf("GetBlockByHeight mismatch: got height %d, want %d", blk.Height, height)
	}
	if blk.Txs == nil {
		t.Fatalf("GetBlockByHeight response missing txs field")
	}

	// Reuse this block response in subsequent tests to avoid an extra full block fetch.
	h.blockHashByHeight[height] = blk.Hash
	h.blockByHash[blk.Hash] = &blockSummary{
		Hash:       strings.TrimSpace(blk.Hash),
		Height:     blk.Height,
		HasTxField: blk.Txs != nil,
		TxIDs:      extractTxIDs(t, blk.Txs),
	}

	hashByIndex, ok := h.getBlockHashForHeight(t, height, true)
	if !ok {
		t.Fatalf("missing block hash for height %d", height)
	}
	assertEqualString(t, blk.Hash, hashByIndex, "GetBlockByHeight block hash")
}

func testGetTransaction(t *testing.T, h *TestHandler) {
	txid := h.sampleTxIDOrSkip(t)
	tx, ok := h.getTransactionByID(t, txid, true)
	if !ok {
		t.Fatalf("missing transaction %s", txid)
	}
	assertEqualString(t, tx.Txid, txid, "transaction txid")
}

func testGetTransactionSpecific(t *testing.T, h *TestHandler) {
	txid := h.sampleTxIDOrSkip(t)

	var specific map[string]json.RawMessage
	h.mustGetJSON(t, "/api/v2/tx-specific/"+url.PathEscape(txid), &specific)
	if len(specific) == 0 {
		t.Fatalf("empty tx-specific response for %s", txid)
	}

	if rawTxID, ok := specific["txid"]; ok {
		var gotTxID string
		if err := json.Unmarshal(rawTxID, &gotTxID); err != nil {
			t.Fatalf("decode tx-specific txid for %s: %v", txid, err)
		}
		if strings.TrimSpace(gotTxID) != "" && !strings.EqualFold(gotTxID, txid) {
			t.Fatalf("tx-specific txid mismatch: got %s, want %s", gotTxID, txid)
		}
	}
}

func testGetAddress(t *testing.T, h *TestHandler) {
	address := h.sampleAddressOrSkip(t)

	var addr addressResponse
	h.mustGetJSON(t, "/api/v2/address/"+url.PathEscape(address)+"?details=basic", &addr)
	assertNonEmptyString(t, addr.Address, "GetAddress.address")
	if !strings.EqualFold(addr.Address, address) {
		t.Fatalf("address mismatch: got %s, want %s", addr.Address, address)
	}
}

func testGetAddressTxids(t *testing.T, h *TestHandler) {
	address := h.sampleAddressOrSkip(t)
	txid := h.sampleTxIDOrSkip(t)

	path := buildAddressDetailsPath(address, "txids", addressPage, addressPageSize)
	var addr addressTxidsResponse
	h.mustGetJSON(t, path, &addr)

	assertAddressTxidsPayload(t, &addr, address, txid, "GetAddressTxids")
}

func testGetAddressTxs(t *testing.T, h *TestHandler) {
	address := h.sampleAddressOrSkip(t)
	txid := h.sampleTxIDOrSkip(t)

	path := buildAddressDetailsPath(address, "txs", addressPage, addressPageSize)
	var addr addressTxsResponse
	h.mustGetJSON(t, path, &addr)

	assertAddressTxsPayload(t, &addr, address, txid, "GetAddressTxs")
}

func testGetUtxo(t *testing.T, h *TestHandler) {
	address := h.sampleAddressOrSkip(t)

	var utxos []utxoResponse
	h.mustGetJSON(t, "/api/v2/utxo/"+url.PathEscape(address)+"?confirmed=true", &utxos)
	assertUTXOList(t, utxos, "GetUtxo")
}

func testGetUtxoConfirmedFilter(t *testing.T, h *TestHandler) {
	address := h.sampleAddressOrSkip(t)

	var all []utxoResponse
	h.mustGetJSON(t, "/api/v2/utxo/"+url.PathEscape(address), &all)

	var confirmed []utxoResponse
	h.mustGetJSON(t, "/api/v2/utxo/"+url.PathEscape(address)+"?confirmed=true", &confirmed)

	if len(all) == 0 && len(confirmed) == 0 {
		t.Skipf("Skipping test, address %s currently has no UTXOs", address)
	}

	assertUTXOListConfirmed(t, confirmed, "GetUtxoConfirmedFilter")
	assertUTXOList(t, all, "GetUtxoConfirmedFilter.all")
}

func (h *TestHandler) mustGetJSON(t *testing.T, path string, out interface{}) {
	t.Helper()

	status, body := h.getHTTP(t, path)
	if status != http.StatusOK {
		t.Fatalf("GET %s returned HTTP %d: %s", path, status, preview(body))
	}
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func (h *TestHandler) getHTTP(t *testing.T, path string) (int, []byte) {
	t.Helper()

	status, body := h.getHTTPWithBase(t, h.HTTPBase, path)
	if shouldUpgradeToHTTPS(status, body, h.HTTPBase) {
		upgradeBase, ok := upgradeHTTPBaseToHTTPS(h.HTTPBase)
		if ok {
			h.HTTPBase = upgradeBase
			status, body = h.getHTTPWithBase(t, h.HTTPBase, path)
		}
	}
	return status, body
}

func (h *TestHandler) getHTTPWithBase(t *testing.T, baseURL, path string) (int, []byte) {
	t.Helper()

	const maxAttempts = 2
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequest(http.MethodGet, h.resolveHTTPURL(baseURL, path), nil)
		if err != nil {
			t.Fatalf("build GET %s: %v", path, err)
		}

		resp, err := h.HTTP.Do(req)
		if err != nil {
			if attempt < maxAttempts && shouldRetryHTTPError(err) {
				time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
				continue
			}
			return 0, []byte(err.Error())
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			if attempt < maxAttempts && shouldRetryHTTPError(err) {
				time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
				continue
			}
			return 0, []byte(err.Error())
		}
		if attempt < maxAttempts && isRetryableHTTPStatus(resp.StatusCode) {
			time.Sleep(time.Duration(attempt) * 300 * time.Millisecond)
			continue
		}
		return resp.StatusCode, body
	}

	return 0, []byte("exhausted retry attempts")
}

func (h *TestHandler) resolveHTTPURL(baseURL, path string) string {
	if strings.HasPrefix(path, "/") {
		return baseURL + path
	}
	return baseURL + "/" + path
}

func assertNonEmptyString(t *testing.T, value, field string) {
	t.Helper()
	if strings.TrimSpace(value) == "" {
		t.Fatalf("empty value for %s", field)
	}
}

func assertEqualString(t *testing.T, got, want, field string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s mismatch: got %s, want %s", field, got, want)
	}
}

func assertAddressMatches(t *testing.T, got, want, field string) {
	t.Helper()
	assertNonEmptyString(t, got, field)
	if !strings.EqualFold(got, want) {
		t.Fatalf("%s mismatch: got %s, want %s", field, got, want)
	}
}

func isUnconfirmedUtxo(utxo utxoResponse) bool {
	return utxo.Confirmations <= 0 || utxo.Height <= 0
}

func preview(body []byte) string {
	const max = 256
	s := strings.TrimSpace(string(body))
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func shouldRetryHTTPError(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") || strings.Contains(msg, "temporary")
}

func isRetryableHTTPStatus(status int) bool {
	switch status {
	case http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}
