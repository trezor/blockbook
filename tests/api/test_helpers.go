//go:build integration

package api

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

const (
	addressPage     = 1
	addressPageSize = 10
)

func buildAddressDetailsPath(address, details string, page, pageSize int) string {
	return fmt.Sprintf("/api/v2/address/%s?details=%s&page=%d&pageSize=%d", url.PathEscape(address), details, page, pageSize)
}

func buildAddressDetailsPathWithTo(address, details string, page, pageSize, toHeight int) string {
	path := buildAddressDetailsPath(address, details, page, pageSize)
	if toHeight > 0 {
		path += "&to=" + strconv.Itoa(toHeight)
	}
	return path
}

func buildAddressDetailsPathWithRange(address, details string, page, pageSize, fromHeight, toHeight int) string {
	path := buildAddressDetailsPath(address, details, page, pageSize)
	if fromHeight > 0 {
		path += "&from=" + strconv.Itoa(fromHeight)
	}
	if toHeight > 0 {
		path += "&to=" + strconv.Itoa(toHeight)
	}
	return path
}

func assertAddressTxidsPayload(t *testing.T, payload *addressTxidsResponse, address, txid, context string, pageSize int) {
	t.Helper()
	assertAddressMatches(t, payload.Address, address, context+".address")
	assertPageMeta(t, payload.Page, payload.ItemsOnPage, payload.TotalPages, payload.Txs, context)
	assertPageSizeUpperBound(t, len(payload.Txids), payload.ItemsOnPage, pageSize, context+".txids")
	assertTxIDListContains(t, payload.Txids, txid, context+".txids")
}

func assertAddressTxsPayload(t *testing.T, payload *addressTxsResponse, address, txid, context string, pageSize int) {
	t.Helper()
	assertAddressMatches(t, payload.Address, address, context+".address")
	assertPageMeta(t, payload.Page, payload.ItemsOnPage, payload.TotalPages, payload.Txs, context)
	assertPageSizeUpperBound(t, len(payload.Transactions), payload.ItemsOnPage, pageSize, context+".transactions")
	assertTransactionsContainTxID(t, payload.Transactions, txid, context+".transactions")
}

func assertPageMeta(t *testing.T, page, itemsOnPage, totalPages, totalItems int, context string) {
	t.Helper()
	if page <= 0 {
		t.Fatalf("%s invalid page: %d", context, page)
	}
	if itemsOnPage < 0 {
		t.Fatalf("%s invalid itemsOnPage: %d", context, itemsOnPage)
	}
	if totalPages < 0 {
		t.Fatalf("%s invalid totalPages: %d", context, totalPages)
	}
	if totalItems < 0 {
		t.Fatalf("%s invalid txs count: %d", context, totalItems)
	}
	if totalPages > 0 && page > totalPages {
		t.Fatalf("%s invalid page %d > totalPages %d", context, page, totalPages)
	}
}

func assertPageMetaAllowUnknownTotal(t *testing.T, page, itemsOnPage, totalPages, totalItems int, context string) {
	t.Helper()
	if page <= 0 {
		t.Fatalf("%s invalid page: %d", context, page)
	}
	if itemsOnPage < 0 {
		t.Fatalf("%s invalid itemsOnPage: %d", context, itemsOnPage)
	}
	if totalPages < -1 {
		t.Fatalf("%s invalid totalPages: %d", context, totalPages)
	}
	if totalItems < 0 {
		t.Fatalf("%s invalid txs count: %d", context, totalItems)
	}
	if totalPages > 0 && page > totalPages {
		t.Fatalf("%s invalid page %d > totalPages %d", context, page, totalPages)
	}
}

func assertPageSizeUpperBound(t *testing.T, payloadLen, itemsOnPage, requestedPageSize int, context string) {
	t.Helper()
	if requestedPageSize <= 0 {
		return
	}
	if itemsOnPage > requestedPageSize {
		t.Fatalf("%s invalid itemsOnPage %d > requested pageSize %d", context, itemsOnPage, requestedPageSize)
	}
	if payloadLen > requestedPageSize {
		t.Fatalf("%s returned %d items, requested pageSize=%d", context, payloadLen, requestedPageSize)
	}
	if itemsOnPage > 0 && payloadLen > itemsOnPage {
		t.Fatalf("%s returned %d items, greater than itemsOnPage=%d", context, payloadLen, itemsOnPage)
	}
}

func assertTxIDListContains(t *testing.T, txids []string, txid, context string) {
	t.Helper()
	if len(txids) == 0 {
		t.Fatalf("%s returned no txids", context)
	}
	for i := range txids {
		assertNonEmptyString(t, txids[i], context)
	}
	if !containsTxID(txids, txid) {
		t.Fatalf("%s does not include sample transaction %s", context, txid)
	}
}

func assertTransactionsContainTxID(t *testing.T, txs []txDetailResponse, txid, context string) {
	t.Helper()
	if len(txs) == 0 {
		t.Fatalf("%s returned no transactions", context)
	}

	txids := make([]string, 0, len(txs))
	for i := range txs {
		assertNonEmptyString(t, txs[i].Txid, context+".txid")
		txids = append(txids, txs[i].Txid)
	}
	if !containsTxID(txids, txid) {
		t.Fatalf("%s does not include sample transaction %s", context, txid)
	}
}

func assertUTXOList(t *testing.T, utxos []utxoResponse, context string) {
	t.Helper()
	for i := range utxos {
		assertNonEmptyString(t, utxos[i].Txid, context+".txid")
		assertNonEmptyString(t, utxos[i].Value, context+".value")
	}
}

func assertUTXOListConfirmed(t *testing.T, utxos []utxoResponse, context string) {
	t.Helper()
	assertUTXOList(t, utxos, context)
	for i := range utxos {
		if isUnconfirmedUtxo(utxos[i]) {
			t.Fatalf("%s returned unconfirmed UTXO: txid=%s vout=%d confirmations=%d height=%d",
				context, utxos[i].Txid, utxos[i].Vout, utxos[i].Confirmations, utxos[i].Height)
		}
	}
}

func assertUTXOListNonNegativeConfirmations(t *testing.T, utxos []utxoResponse, context string) {
	t.Helper()
	assertUTXOList(t, utxos, context)
	for i := range utxos {
		if utxos[i].Confirmations < 0 {
			t.Fatalf("%s has negative confirmations for %s", context, utxos[i].Txid)
		}
	}
}

func assertFiatTickerPayload(t *testing.T, payload *fiatTickerResponse, context string) {
	t.Helper()
	if payload.Timestamp <= 0 {
		t.Fatalf("%s invalid timestamp: %d", context, payload.Timestamp)
	}
	if len(payload.Rates) == 0 {
		t.Fatalf("%s returned no rates", context)
	}
	for currency, rate := range payload.Rates {
		assertNonEmptyString(t, currency, context+".rates.currency")
		if rate == 0 {
			t.Fatalf("%s returned zero rate for currency %s", context, currency)
		}
	}
}

func assertUTXOSetsEqualByOutpoint(t *testing.T, got, want []utxoResponse, context string) {
	t.Helper()
	gotSet := utxoSetByOutpoint(t, got, context+".got")
	wantSet := utxoSetByOutpoint(t, want, context+".want")
	if len(gotSet) != len(wantSet) {
		t.Fatalf("%s outpoint count mismatch: got=%d want=%d", context, len(gotSet), len(wantSet))
	}
	for key := range wantSet {
		if _, ok := gotSet[key]; !ok {
			t.Fatalf("%s missing outpoint in got set: %s", context, key)
		}
	}
}

func assertConfirmedUTXOsIncludedByOutpoint(t *testing.T, mixed, confirmed []utxoResponse, context string) {
	t.Helper()
	confirmedSet := utxoSetByOutpoint(t, confirmed, context+".confirmed")
	for i := range mixed {
		if isUnconfirmedUtxo(mixed[i]) {
			continue
		}
		key := utxoOutpointKey(mixed[i])
		if _, ok := confirmedSet[key]; !ok {
			t.Fatalf("%s missing confirmed outpoint %s in confirmed=true response", context, key)
		}
	}
}

func utxoSetsEqualByOutpoint(a, b []utxoResponse) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for i := range a {
		set[utxoOutpointKey(a[i])] = struct{}{}
	}
	if len(set) != len(a) {
		return false
	}
	for i := range b {
		if _, ok := set[utxoOutpointKey(b[i])]; !ok {
			return false
		}
	}
	return true
}

func utxoSetByOutpoint(t *testing.T, utxos []utxoResponse, context string) map[string]utxoResponse {
	t.Helper()
	set := make(map[string]utxoResponse, len(utxos))
	for i := range utxos {
		key := utxoOutpointKey(utxos[i])
		if _, exists := set[key]; exists {
			t.Fatalf("%s duplicate outpoint: %s", context, key)
		}
		set[key] = utxos[i]
	}
	return set
}

func utxoOutpointKey(utxo utxoResponse) string {
	return strings.ToLower(strings.TrimSpace(utxo.Txid)) + ":" + strconv.Itoa(utxo.Vout)
}

func assertEVMTokenBalancesPayload(t *testing.T, payload *evmAddressTokenBalanceResponse, address, context string) {
	t.Helper()
	assertAddressMatches(t, payload.Address, address, context+".address")
	assertNonEmptyString(t, payload.Balance, context+".balance")
	tokensWithHoldings := 0
	for i := range payload.Tokens {
		tokenContext := fmt.Sprintf("%s.tokens[%d]", context, i)
		if assertEVMTokenHasHoldings(t, payload.Tokens[i], tokenContext) {
			tokensWithHoldings++
		}
	}
	if len(payload.Tokens) > 0 && tokensWithHoldings == 0 {
		t.Fatalf("%s has tokens array but no token includes holdings fields", context)
	}
}

func assertEVMBasicAddressPayload(t *testing.T, payload *evmAddressTokenBalanceResponse, address, context string) {
	t.Helper()
	assertAddressMatches(t, payload.Address, address, context+".address")
	assertNonEmptyString(t, payload.Balance, context+".balance")
	assertNonEmptyString(t, payload.Nonce, context+".nonce")
	if payload.NonTokenTxs < 0 {
		t.Fatalf("%s has negative nonTokenTxs: %d", context, payload.NonTokenTxs)
	}
	if payload.Txs < 0 {
		t.Fatalf("%s has negative txs: %d", context, payload.Txs)
	}
	if payload.NonTokenTxs > payload.Txs {
		t.Fatalf("%s has nonTokenTxs %d greater than txs %d", context, payload.NonTokenTxs, payload.Txs)
	}
}

func assertEVMTokenHasHoldings(t *testing.T, token evmTokenResponse, context string) bool {
	t.Helper()
	assertNonEmptyString(t, token.Type, context+".type")

	hasBalance := strings.TrimSpace(token.Balance) != ""
	hasIDs := len(token.IDs) > 0
	hasMultiTokenValues := len(token.MultiTokenValues) > 0

	if hasIDs {
		for i := range token.IDs {
			assertNonEmptyString(t, token.IDs[i], context+".ids")
		}
	}
	if hasMultiTokenValues {
		for i := range token.MultiTokenValues {
			mv := token.MultiTokenValues[i]
			if strings.TrimSpace(mv.ID) == "" && strings.TrimSpace(mv.Value) == "" {
				t.Fatalf("%s.multiTokenValues entry has both empty id and value", context)
			}
		}
	}
	return hasBalance || hasIDs || hasMultiTokenValues
}

func assertEVMTokenListContractsMatch(t *testing.T, tokens []evmTokenResponse, contract, context string) {
	t.Helper()
	if len(tokens) == 0 {
		t.Fatalf("%s returned no tokens", context)
	}
	for i := range tokens {
		tokenContext := fmt.Sprintf("%s.tokens[%d]", context, i)
		assertNonEmptyString(t, tokens[i].Contract, tokenContext+".contract")
		if !strings.EqualFold(tokens[i].Contract, contract) {
			t.Fatalf("%s contract mismatch: got %s, want %s", tokenContext, tokens[i].Contract, contract)
		}
	}
}

func assertEVMTokenBalancesHaveHoldingsFields(t *testing.T, payload *evmAddressTokenBalanceResponse, address, context string) {
	t.Helper()
	assertAddressMatches(t, payload.Address, address, context+".address")
	assertNonEmptyString(t, payload.Balance, context+".balance")

	for i := range payload.Tokens {
		token := payload.Tokens[i]
		tokenContext := fmt.Sprintf("%s.tokens[%d]", context, i)
		assertNonEmptyString(t, token.Type, tokenContext+".type")

		hasHoldings := false
		balance := strings.TrimSpace(token.Balance)
		if balance != "" {
			hasHoldings = true
		}

		if len(token.IDs) > 0 {
			for j := range token.IDs {
				assertNonEmptyString(t, token.IDs[j], tokenContext+".ids")
			}
			hasHoldings = true
		}

		if len(token.MultiTokenValues) > 0 {
			for j := range token.MultiTokenValues {
				mv := token.MultiTokenValues[j]
				if strings.TrimSpace(mv.ID) == "" && strings.TrimSpace(mv.Value) == "" {
					t.Fatalf("%s.multiTokenValues entry has both empty id and value", tokenContext)
				}
			}
			hasHoldings = true
		}

		if !hasHoldings {
			t.Fatalf("%s has no holdings fields (balance, ids, multiTokenValues)", tokenContext)
		}
	}
}

func txIDsFromTransactions(t *testing.T, txs []txDetailResponse, context string) []string {
	t.Helper()
	txids := make([]string, 0, len(txs))
	for i := range txs {
		txContext := fmt.Sprintf("%s.transactions[%d].txid", context, i)
		assertNonEmptyString(t, txs[i].Txid, txContext)
		txids = append(txids, txs[i].Txid)
	}
	return txids
}

func assertStringSlicesEqual(t *testing.T, got, want []string, context string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s length mismatch: got %d, want %d", context, len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s[%d] mismatch: got %s, want %s", context, i, got[i], want[i])
		}
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
