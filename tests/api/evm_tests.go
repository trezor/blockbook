//go:build integration

package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"testing"
)

const (
	evmHistoryPage     = 1
	evmHistoryPageSize = 3
)

func testGetAddressBasicEVM(t *testing.T, h *TestHandler) {
	address := h.sampleEVMAddressOrSkip(t)

	path := buildAddressDetailsPath(address, "basic", addressPage, addressPageSize)
	var resp evmAddressTokenBalanceResponse
	h.mustGetJSON(t, path, &resp)

	assertEVMBasicAddressPayload(t, &resp, address, "GetAddressBasicEVM")
}

func testGetAddressTxidsPaginationEVM(t *testing.T, h *TestHandler) {
	address := h.sampleEVMAddressOrSkip(t)

	var page1 addressTxidsResponse
	h.mustGetJSON(t, buildAddressDetailsPath(address, "txids", evmHistoryPage, evmHistoryPageSize), &page1)

	assertAddressMatches(t, page1.Address, address, "GetAddressTxidsPaginationEVM.page1.address")
	assertPageMeta(t, page1.Page, page1.ItemsOnPage, page1.TotalPages, page1.Txs, "GetAddressTxidsPaginationEVM.page1")
	if len(page1.Txids) == 0 {
		t.Fatalf("GetAddressTxidsPaginationEVM page 1 returned no txids")
	}
	for i := range page1.Txids {
		assertNonEmptyString(t, page1.Txids[i], "GetAddressTxidsPaginationEVM.page1.txids")
	}

	if page1.TotalPages <= 1 || page1.Txs <= evmHistoryPageSize {
		t.Skipf("Skipping pagination check, address %s has %d txs and %d page(s)", address, page1.Txs, page1.TotalPages)
	}

	var page2 addressTxidsResponse
	h.mustGetJSON(t, buildAddressDetailsPath(address, "txids", evmHistoryPage+1, evmHistoryPageSize), &page2)

	assertAddressMatches(t, page2.Address, address, "GetAddressTxidsPaginationEVM.page2.address")
	assertPageMeta(t, page2.Page, page2.ItemsOnPage, page2.TotalPages, page2.Txs, "GetAddressTxidsPaginationEVM.page2")
	if page2.Page != evmHistoryPage+1 {
		t.Fatalf("GetAddressTxidsPaginationEVM page mismatch: got %d, want %d", page2.Page, evmHistoryPage+1)
	}
	if len(page2.Txids) == 0 {
		t.Fatalf("GetAddressTxidsPaginationEVM page 2 returned no txids")
	}
	for i := range page2.Txids {
		assertNonEmptyString(t, page2.Txids[i], "GetAddressTxidsPaginationEVM.page2.txids")
	}
}

func testGetAddressTxsPaginationEVM(t *testing.T, h *TestHandler) {
	address := h.sampleEVMAddressOrSkip(t)

	var page1 addressTxsResponse
	h.mustGetJSON(t, buildAddressDetailsPath(address, "txs", evmHistoryPage, evmHistoryPageSize), &page1)

	assertAddressMatches(t, page1.Address, address, "GetAddressTxsPaginationEVM.page1.address")
	assertPageMeta(t, page1.Page, page1.ItemsOnPage, page1.TotalPages, page1.Txs, "GetAddressTxsPaginationEVM.page1")
	if len(page1.Transactions) == 0 {
		t.Fatalf("GetAddressTxsPaginationEVM page 1 returned no transactions")
	}
	txIDsFromTransactions(t, page1.Transactions, "GetAddressTxsPaginationEVM.page1")

	if page1.TotalPages <= 1 || page1.Txs <= evmHistoryPageSize {
		t.Skipf("Skipping pagination check, address %s has %d txs and %d page(s)", address, page1.Txs, page1.TotalPages)
	}

	var page2 addressTxsResponse
	h.mustGetJSON(t, buildAddressDetailsPath(address, "txs", evmHistoryPage+1, evmHistoryPageSize), &page2)

	assertAddressMatches(t, page2.Address, address, "GetAddressTxsPaginationEVM.page2.address")
	assertPageMeta(t, page2.Page, page2.ItemsOnPage, page2.TotalPages, page2.Txs, "GetAddressTxsPaginationEVM.page2")
	if page2.Page != evmHistoryPage+1 {
		t.Fatalf("GetAddressTxsPaginationEVM page mismatch: got %d, want %d", page2.Page, evmHistoryPage+1)
	}
	if len(page2.Transactions) == 0 {
		t.Fatalf("GetAddressTxsPaginationEVM page 2 returned no transactions")
	}
	page2Txids := txIDsFromTransactions(t, page2.Transactions, "GetAddressTxsPaginationEVM.page2")
	_ = page2Txids
}

func testGetAddressTokensEVM(t *testing.T, h *TestHandler) {
	address := h.sampleEVMAddressOrSkip(t)

	path := buildAddressDetailsPath(address, "tokens", addressPage, addressPageSize)
	var resp evmAddressTokenBalanceResponse
	h.mustGetJSON(t, path, &resp)

	assertEVMBasicAddressPayload(t, &resp, address, "GetAddressTokensEVM")
	for i := range resp.Tokens {
		tokenContext := fmt.Sprintf("GetAddressTokensEVM.tokens[%d]", i)
		assertNonEmptyString(t, resp.Tokens[i].Type, tokenContext+".type")
		assertNonEmptyString(t, resp.Tokens[i].Contract, tokenContext+".contract")
	}
}

func testGetAddressTokenBalances(t *testing.T, h *TestHandler) {
	address := h.sampleEVMAddressOrSkip(t)

	path := buildAddressDetailsPath(address, "tokenBalances", addressPage, addressPageSize)
	var resp evmAddressTokenBalanceResponse
	h.mustGetJSON(t, path, &resp)

	assertEVMTokenBalancesPayload(t, &resp, address, "GetAddressTokenBalances")
}

func testGetAddressContractFilterEVM(t *testing.T, h *TestHandler) {
	address := h.sampleEVMAddressOrSkip(t)
	contract := h.sampleEVMContractOrSkip(t)

	path := buildAddressDetailsPath(address, "tokenBalances", addressPage, addressPageSize) + "&contract=" + url.QueryEscape(contract)
	var resp evmAddressTokenBalanceResponse
	h.mustGetJSON(t, path, &resp)

	assertEVMTokenBalancesPayload(t, &resp, address, "GetAddressContractFilterEVM")
	assertEVMTokenListContractsMatch(t, resp.Tokens, contract, "GetAddressContractFilterEVM")
}

func testGetTransactionEVMShape(t *testing.T, h *TestHandler) {
	txid := h.sampleEVMTxIDOrSkip(t)

	path := "/api/v2/tx/" + url.PathEscape(txid)
	var tx evmTxShapeResponse
	h.mustGetJSON(t, path, &tx)

	assertEqualString(t, tx.Txid, txid, "GetTransactionEVMShape.txid")
	if !isEVMTxID(tx.Txid) {
		t.Fatalf("GetTransactionEVMShape txid is not EVM-like: %s", tx.Txid)
	}
	if len(tx.Vin) != 1 {
		t.Fatalf("GetTransactionEVMShape expected exactly 1 vin entry, got %d", len(tx.Vin))
	}
	if len(tx.Vout) != 1 {
		t.Fatalf("GetTransactionEVMShape expected exactly 1 vout entry, got %d", len(tx.Vout))
	}
	if !hasNonEmptyObject(tx.EthereumSpecific) {
		t.Fatalf("GetTransactionEVMShape missing ethereumSpecific object for %s", txid)
	}
}

func testWsGetAccountInfoBasicEVM(t *testing.T, h *TestHandler) {
	address := h.sampleEVMAddressOrSkip(t)

	resp := h.wsCall(t, "getAccountInfo", map[string]interface{}{
		"descriptor": address,
		"details":    "basic",
		"page":       addressPage,
		"pageSize":   addressPageSize,
	})

	var info evmAddressTokenBalanceResponse
	if err := json.Unmarshal(resp.Data, &info); err != nil {
		t.Fatalf("decode websocket getAccountInfo EVM basic response: %v", err)
	}

	assertEVMBasicAddressPayload(t, &info, address, "WsGetAccountInfoBasicEVM")
}

func testWsGetAccountInfoEVM(t *testing.T, h *TestHandler) {
	address := h.sampleEVMAddressOrSkip(t)

	resp := h.wsCall(t, "getAccountInfo", map[string]interface{}{
		"descriptor": address,
		"details":    "tokenBalances",
		"page":       addressPage,
		"pageSize":   addressPageSize,
	})

	var info evmAddressTokenBalanceResponse
	if err := json.Unmarshal(resp.Data, &info); err != nil {
		t.Fatalf("decode websocket getAccountInfo EVM response: %v", err)
	}

	assertEVMTokenBalancesPayload(t, &info, address, "WsGetAccountInfoEVM")
}

func testWsGetAccountInfoTxidsConsistencyEVM(t *testing.T, h *TestHandler) {
	address := h.sampleEVMAddressOrSkip(t)
	bestHeight := h.getStatus(t).BestHeight

	var httpResp addressTxidsResponse
	h.mustGetJSON(t, buildAddressDetailsPathWithTo(address, "txids", evmHistoryPage, evmHistoryPageSize, bestHeight), &httpResp)
	assertAddressMatches(t, httpResp.Address, address, "WsGetAccountInfoTxidsConsistencyEVM.http.address")
	assertPageMetaAllowUnknownTotal(t, httpResp.Page, httpResp.ItemsOnPage, httpResp.TotalPages, httpResp.Txs, "WsGetAccountInfoTxidsConsistencyEVM.http")

	wsRaw := h.wsCall(t, "getAccountInfo", map[string]interface{}{
		"descriptor": address,
		"details":    "txids",
		"page":       evmHistoryPage,
		"pageSize":   evmHistoryPageSize,
		"to":         bestHeight,
	})
	var wsResp addressTxidsResponse
	if err := json.Unmarshal(wsRaw.Data, &wsResp); err != nil {
		t.Fatalf("decode websocket getAccountInfo txids EVM response: %v", err)
	}
	assertAddressMatches(t, wsResp.Address, address, "WsGetAccountInfoTxidsConsistencyEVM.ws.address")
	assertPageMetaAllowUnknownTotal(t, wsResp.Page, wsResp.ItemsOnPage, wsResp.TotalPages, wsResp.Txs, "WsGetAccountInfoTxidsConsistencyEVM.ws")

	if wsResp.Page != httpResp.Page || wsResp.ItemsOnPage != httpResp.ItemsOnPage {
		t.Fatalf("WsGetAccountInfoTxidsConsistencyEVM page meta mismatch: ws(page=%d items=%d totalPages=%d txs=%d) http(page=%d items=%d totalPages=%d txs=%d)",
			wsResp.Page, wsResp.ItemsOnPage, wsResp.TotalPages, wsResp.Txs,
			httpResp.Page, httpResp.ItemsOnPage, httpResp.TotalPages, httpResp.Txs)
	}
	if wsResp.TotalPages != httpResp.TotalPages {
		t.Fatalf("WsGetAccountInfoTxidsConsistencyEVM totalPages mismatch: ws=%d http=%d", wsResp.TotalPages, httpResp.TotalPages)
	}
	if wsResp.TotalPages >= 0 && wsResp.Txs != httpResp.Txs {
		t.Fatalf("WsGetAccountInfoTxidsConsistencyEVM tx count mismatch: ws=%d http=%d", wsResp.Txs, httpResp.Txs)
	}
	assertStringSlicesEqual(t, wsResp.Txids, httpResp.Txids, "WsGetAccountInfoTxidsConsistencyEVM.txids")
}

func testWsGetAccountInfoTxsConsistencyEVM(t *testing.T, h *TestHandler) {
	address := h.sampleEVMAddressOrSkip(t)
	bestHeight := h.getStatus(t).BestHeight

	var httpResp addressTxsResponse
	h.mustGetJSON(t, buildAddressDetailsPathWithTo(address, "txs", evmHistoryPage, evmHistoryPageSize, bestHeight), &httpResp)
	assertAddressMatches(t, httpResp.Address, address, "WsGetAccountInfoTxsConsistencyEVM.http.address")
	assertPageMetaAllowUnknownTotal(t, httpResp.Page, httpResp.ItemsOnPage, httpResp.TotalPages, httpResp.Txs, "WsGetAccountInfoTxsConsistencyEVM.http")
	httpTxids := txIDsFromTransactions(t, httpResp.Transactions, "WsGetAccountInfoTxsConsistencyEVM.http")

	wsRaw := h.wsCall(t, "getAccountInfo", map[string]interface{}{
		"descriptor": address,
		"details":    "txs",
		"page":       evmHistoryPage,
		"pageSize":   evmHistoryPageSize,
		"to":         bestHeight,
	})
	var wsResp addressTxsResponse
	if err := json.Unmarshal(wsRaw.Data, &wsResp); err != nil {
		t.Fatalf("decode websocket getAccountInfo txs EVM response: %v", err)
	}
	assertAddressMatches(t, wsResp.Address, address, "WsGetAccountInfoTxsConsistencyEVM.ws.address")
	assertPageMetaAllowUnknownTotal(t, wsResp.Page, wsResp.ItemsOnPage, wsResp.TotalPages, wsResp.Txs, "WsGetAccountInfoTxsConsistencyEVM.ws")
	wsTxids := txIDsFromTransactions(t, wsResp.Transactions, "WsGetAccountInfoTxsConsistencyEVM.ws")

	if wsResp.Page != httpResp.Page || wsResp.ItemsOnPage != httpResp.ItemsOnPage {
		t.Fatalf("WsGetAccountInfoTxsConsistencyEVM page meta mismatch: ws(page=%d items=%d totalPages=%d txs=%d) http(page=%d items=%d totalPages=%d txs=%d)",
			wsResp.Page, wsResp.ItemsOnPage, wsResp.TotalPages, wsResp.Txs,
			httpResp.Page, httpResp.ItemsOnPage, httpResp.TotalPages, httpResp.Txs)
	}
	if wsResp.TotalPages != httpResp.TotalPages {
		t.Fatalf("WsGetAccountInfoTxsConsistencyEVM totalPages mismatch: ws=%d http=%d", wsResp.TotalPages, httpResp.TotalPages)
	}
	if wsResp.TotalPages >= 0 && wsResp.Txs != httpResp.Txs {
		t.Fatalf("WsGetAccountInfoTxsConsistencyEVM tx count mismatch: ws=%d http=%d", wsResp.Txs, httpResp.Txs)
	}
	assertStringSlicesEqual(t, wsTxids, httpTxids, "WsGetAccountInfoTxsConsistencyEVM.txids")
}

func testWsGetAccountInfoContractFilterEVM(t *testing.T, h *TestHandler) {
	address := h.sampleEVMAddressOrSkip(t)
	contract := h.sampleEVMContractOrSkip(t)

	resp := h.wsCall(t, "getAccountInfo", map[string]interface{}{
		"descriptor":     address,
		"details":        "tokenBalances",
		"contractFilter": contract,
		"page":           addressPage,
		"pageSize":       addressPageSize,
	})

	var info evmAddressTokenBalanceResponse
	if err := json.Unmarshal(resp.Data, &info); err != nil {
		t.Fatalf("decode websocket getAccountInfo EVM contractFilter response: %v", err)
	}

	assertEVMTokenBalancesPayload(t, &info, address, "WsGetAccountInfoContractFilterEVM")
	assertEVMTokenListContractsMatch(t, info.Tokens, contract, "WsGetAccountInfoContractFilterEVM")
}
