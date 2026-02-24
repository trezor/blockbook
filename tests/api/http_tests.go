//go:build integration

package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func testStatus(t *testing.T, h *TestHandler) {
	_ = h.getStatus(t)
}

func testGetBlockIndex(t *testing.T, h *TestHandler) {
	status := h.getStatus(t)
	if _, ok := h.getBlockHashForHeight(t, status.BestHeight, true); !ok {
		t.Fatalf("missing block hash for best height %d", status.BestHeight)
	}
}

func testGetBlock(t *testing.T, h *TestHandler) {
	status := h.getStatus(t)
	bestHash, ok := h.getBlockHashForHeight(t, status.BestHeight, true)
	if !ok {
		t.Fatalf("missing block hash for best height %d", status.BestHeight)
	}

	blk, ok := h.getBlockByHash(t, bestHash, true)
	if !ok {
		t.Fatalf("missing block for hash %s", bestHash)
	}
	assertEqualString(t, blk.Hash, bestHash, "block hash")
	if blk.Height != status.BestHeight {
		t.Fatalf("block height mismatch: got %d, want %d", blk.Height, status.BestHeight)
	}
	if !blk.HasTxField {
		t.Fatalf("block response missing txs field")
	}
}

func testGetBlockByHeight(t *testing.T, h *TestHandler) {
	status := h.getStatus(t)
	height := status.BestHeight
	if height > 2 {
		height = height - 2
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

	path := "/api/v2/address/" + url.PathEscape(address) + "?details=txids&page=1&pageSize=10"
	var addr addressTxidsResponse
	h.mustGetJSON(t, path, &addr)

	assertAddressMatches(t, addr.Address, address, "GetAddressTxids.address")
	if len(addr.Txids) == 0 {
		t.Fatalf("GetAddressTxids returned no txids for %s", address)
	}
	for i := range addr.Txids {
		assertNonEmptyString(t, addr.Txids[i], "GetAddressTxids.txids")
	}
	if !containsTxID(addr.Txids, txid) {
		t.Fatalf("GetAddressTxids does not include sample transaction %s for %s", txid, address)
	}
}

func testGetAddressTxs(t *testing.T, h *TestHandler) {
	address := h.sampleAddressOrSkip(t)
	txid := h.sampleTxIDOrSkip(t)

	path := "/api/v2/address/" + url.PathEscape(address) + "?details=txs&page=1&pageSize=10"
	var addr addressTxsResponse
	h.mustGetJSON(t, path, &addr)

	assertAddressMatches(t, addr.Address, address, "GetAddressTxs.address")
	if len(addr.Transactions) == 0 {
		t.Fatalf("GetAddressTxs returned no transactions for %s", address)
	}

	txIDs := make([]string, 0, len(addr.Transactions))
	for i := range addr.Transactions {
		assertNonEmptyString(t, addr.Transactions[i].Txid, "GetAddressTxs.transactions.txid")
		txIDs = append(txIDs, addr.Transactions[i].Txid)
	}
	if !containsTxID(txIDs, txid) {
		t.Fatalf("GetAddressTxs does not include sample transaction %s for %s", txid, address)
	}
}

func testGetUtxo(t *testing.T, h *TestHandler) {
	address := h.sampleAddressOrSkip(t)

	var utxos []utxoResponse
	h.mustGetJSON(t, "/api/v2/utxo/"+url.PathEscape(address)+"?confirmed=true", &utxos)
	for i := range utxos {
		assertNonEmptyString(t, utxos[i].Txid, "GetUtxo entry txid")
		assertNonEmptyString(t, utxos[i].Value, "GetUtxo entry value")
	}
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

	for i := range confirmed {
		assertNonEmptyString(t, confirmed[i].Txid, "GetUtxoConfirmedFilter.txid")
		assertNonEmptyString(t, confirmed[i].Value, "GetUtxoConfirmedFilter.value")
		if isUnconfirmedUtxo(confirmed[i]) {
			t.Fatalf("GetUtxoConfirmedFilter returned unconfirmed UTXO: txid=%s vout=%d confirmations=%d height=%d",
				confirmed[i].Txid, confirmed[i].Vout, confirmed[i].Confirmations, confirmed[i].Height)
		}
	}

	for i := range all {
		assertNonEmptyString(t, all[i].Txid, "GetUtxoConfirmedFilter.all.txid")
		assertNonEmptyString(t, all[i].Value, "GetUtxoConfirmedFilter.all.value")
	}
}

func (h *TestHandler) sampleTxIDOrSkip(t *testing.T) string {
	t.Helper()
	txid, found := h.getSampleTxID(t)
	if !found {
		t.Skipf("Skipping test, no transaction found in last %d blocks from height %d", txSearchWindow, h.getStatus(t).BestHeight)
	}
	return txid
}

func (h *TestHandler) sampleAddressOrSkip(t *testing.T) string {
	t.Helper()
	address, found := h.getSampleAddress(t)
	if !found {
		t.Skipf("Skipping test, no address found from recent transaction window at height %d", h.getStatus(t).BestHeight)
	}
	return address
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

	req, err := http.NewRequest(http.MethodGet, h.resolveHTTPURL(baseURL, path), nil)
	if err != nil {
		t.Fatalf("build GET %s: %v", path, err)
	}

	resp, err := h.HTTP.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response %s: %v", path, err)
	}
	return resp.StatusCode, body
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

func containsTxID(txids []string, txid string) bool {
	for i := range txids {
		if strings.EqualFold(strings.TrimSpace(txids[i]), txid) {
			return true
		}
	}
	return false
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
