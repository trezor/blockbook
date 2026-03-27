//go:build integration

package api

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"strings"
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
	assertPageSizeUpperBound(t, len(page1.Txids), page1.ItemsOnPage, evmHistoryPageSize, "GetAddressTxidsPaginationEVM.page1.txids")
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
	assertPageSizeUpperBound(t, len(page2.Txids), page2.ItemsOnPage, evmHistoryPageSize, "GetAddressTxidsPaginationEVM.page2.txids")
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
	assertPageSizeUpperBound(t, len(page1.Transactions), page1.ItemsOnPage, evmHistoryPageSize, "GetAddressTxsPaginationEVM.page1.transactions")
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
	assertPageSizeUpperBound(t, len(page2.Transactions), page2.ItemsOnPage, evmHistoryPageSize, "GetAddressTxsPaginationEVM.page2.transactions")
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
	assertEVMTokenBalancesHaveHoldingsFields(t, &resp, address, "GetAddressTokenBalances")
}

func testGetAddressIncludeErc4626EVM(t *testing.T, h *TestHandler) {
	assertErc4626FixturesInAccountInfo(t, h, "GetAddressIncludeErc4626EVM", func(t *testing.T, fixture erc4626Fixture) evmAddressTokenBalanceResponse {
		path := buildAddressDetailsPath(fixture.Holder, "tokenBalances", addressPage, addressPageSize) +
			"&contract=" + url.QueryEscape(fixture.Contract) +
			"&includeErc4626=true"

		var resp evmAddressTokenBalanceResponse
		h.mustGetJSON(t, path, &resp)
		return resp
	})
}

func testGetAddressContractFilterEVM(t *testing.T, h *TestHandler) {
	address := h.sampleEVMAddressOrSkip(t)
	contract := h.sampleEVMContractOrSkip(t)

	path := buildAddressDetailsPath(address, "tokenBalances", addressPage, addressPageSize) + "&contract=" + url.QueryEscape(contract)
	var resp evmAddressTokenBalanceResponse
	h.mustGetJSON(t, path, &resp)

	assertEVMTokenBalancesPayload(t, &resp, address, "GetAddressContractFilterEVM")
	assertEVMTokenBalancesHaveHoldingsFields(t, &resp, address, "GetAddressContractFilterEVM")
	assertEVMTokenListContractsMatch(t, resp.Tokens, contract, "GetAddressContractFilterEVM")
}

func testGetTransactionEVMShape(t *testing.T, h *TestHandler) {
	txid := h.sampleEVMTxIDOrSkip(t)

	path := "/api/v2/tx/" + url.PathEscape(txid)
	var tx evmTxShapeResponse
	h.mustGetJSON(t, path, &tx)

	assertEqualString(t, tx.Txid, txid, "GetTransactionEVMShape.txid")
	if !h.isEVMTxID(tx.Txid) {
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
	assertEVMTokenBalancesHaveHoldingsFields(t, &info, address, "WsGetAccountInfoEVM")
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
	assertEVMTokenBalancesHaveHoldingsFields(t, &info, address, "WsGetAccountInfoContractFilterEVM")
	assertEVMTokenListContractsMatch(t, info.Tokens, contract, "WsGetAccountInfoContractFilterEVM")
}

func testWsGetAccountInfoIncludeErc4626EVM(t *testing.T, h *TestHandler) {
	assertErc4626FixturesInAccountInfo(t, h, "WsGetAccountInfoIncludeErc4626EVM", func(t *testing.T, fixture erc4626Fixture) evmAddressTokenBalanceResponse {
		resp := h.wsCall(t, "getAccountInfo", map[string]interface{}{
			"descriptor":     fixture.Holder,
			"details":        "tokenBalances",
			"contractFilter": fixture.Contract,
			"includeErc4626": true,
			"page":           addressPage,
			"pageSize":       addressPageSize,
		})

		var info evmAddressTokenBalanceResponse
		if err := json.Unmarshal(resp.Data, &info); err != nil {
			t.Fatalf("decode websocket getAccountInfo includeErc4626 response: %v", err)
		}
		return info
	})
}

func assertErc4626FixturesInAccountInfo(t *testing.T, h *TestHandler, testName string, fetch func(t *testing.T, fixture erc4626Fixture) evmAddressTokenBalanceResponse) {
	testData, err := loadAPITestData(h.Coin)
	if err != nil {
		t.Fatalf("load api test data for %s: %v", h.Coin, err)
	}
	if len(testData.ERC4626Fixtures) == 0 {
		t.Fatalf("api/testdata/%s.json has no erc4626Fixtures entries", h.Coin)
	}

	validatedFixtures := 0

	for _, fixture := range testData.ERC4626Fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			info := fetch(t, fixture)

			assertAddressMatches(t, info.Address, fixture.Holder, testName+".address")
			if len(info.Tokens) == 0 {
				t.Skipf("fixture %s returned no tokens for contract %s", fixture.Name, fixture.Contract)
			}

			for i := range info.Tokens {
				token := info.Tokens[i]
				context := fmt.Sprintf("%s.tokens[%d]", testName, i)
				if !strings.EqualFold(token.Contract, fixture.Contract) {
					t.Fatalf("%s contract mismatch: got %s want %s", context, token.Contract, fixture.Contract)
				}
				if token.Erc4626 == nil {
					t.Fatalf("%s missing erc4626 payload for known ERC4626 contract %s", context, fixture.Contract)
				}
				assertErc4626Payload(t, context+".erc4626", fixture.Contract, token.Erc4626)
			}

			validatedFixtures++
		})
	}

	if validatedFixtures == 0 {
		t.Fatalf("%s did not validate any ERC4626 fixture", testName)
	}
}

func assertErc4626Payload(t *testing.T, context, shareContract string, payload *evmErc4626Response) {
	t.Helper()
	if payload == nil {
		t.Fatalf("%s missing payload", context)
	}
	if payload.Asset == nil {
		t.Fatalf("%s missing asset metadata", context)
	}
	assertNonEmptyString(t, payload.Asset.Contract, context+".asset.contract")
	if !isEVMAddress(payload.Asset.Contract) {
		t.Fatalf("%s.asset.contract is not EVM-like: %s", context, payload.Asset.Contract)
	}
	if payload.Asset.Decimals < 0 {
		t.Fatalf("%s.asset.decimals is negative: %d", context, payload.Asset.Decimals)
	}

	if payload.Share == nil {
		t.Fatalf("%s missing share metadata", context)
	}
	assertNonEmptyString(t, payload.Share.Contract, context+".share.contract")
	if !strings.EqualFold(payload.Share.Contract, shareContract) {
		t.Fatalf("%s.share.contract mismatch: got %s want %s", context, payload.Share.Contract, shareContract)
	}
	if payload.Share.Decimals < 0 {
		t.Fatalf("%s.share.decimals is negative: %d", context, payload.Share.Decimals)
	}

	assertBigIntString(t, payload.TotalAssets, context+".totalAssets")
	assertOptionalBigIntString(t, payload.ConvertToAssets1Share, context+".convertToAssets1Share")
	assertOptionalBigIntString(t, payload.ConvertToShares1Asset, context+".convertToShares1Asset")
	assertOptionalBigIntString(t, payload.PreviewDeposit1Asset, context+".previewDeposit1Asset")
	assertOptionalBigIntString(t, payload.PreviewRedeem1Share, context+".previewRedeem1Share")
	if strings.TrimSpace(payload.Error) != "" {
		assertNonEmptyString(t, payload.Error, context+".error")
	}
}

func assertBigIntString(t *testing.T, value, context string) {
	t.Helper()
	value = strings.TrimSpace(value)
	if value == "" {
		t.Fatalf("%s is empty", context)
	}
	assertOptionalBigIntString(t, value, context)
}

func assertOptionalBigIntString(t *testing.T, value, context string) {
	t.Helper()
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	n, ok := new(big.Int).SetString(value, 10)
	if !ok {
		t.Fatalf("%s is not a valid decimal integer: %s", context, value)
	}
	if n.Sign() < 0 {
		t.Fatalf("%s is negative: %s", context, value)
	}
}
