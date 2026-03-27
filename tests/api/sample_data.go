//go:build integration

package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var scientificNotationPattern = regexp.MustCompile(`"value(?:Zat|Sat)?"\s*:\s*-?\d+\.\d+[eE][+-]?\d+`)

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
		blk, ok := h.getBlockByHashForSampling(t, hash, false)
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

	if h.sampleBlockResolved && h.sampleBlockHash != "" {
		if blk, ok := h.getBlockByHash(t, h.sampleBlockHash, false); ok {
			for _, txid := range blk.TxIDs {
				txid = strings.TrimSpace(txid)
				if txid != "" {
					h.sampleTxResolved = true
					h.sampleTxID = txid
					return h.sampleTxID, true
				}
			}
		}
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

	if h.isEVMTxID(txid) {
		h.sampleAddress = firstAddressFromTxPreferVin(tx)
	} else {
		h.sampleAddress = firstAddressFromTx(tx)
	}
	return h.sampleAddress, h.sampleAddress != ""
}

func (h *TestHandler) getSampleAddressWithScientificNotationTx(t *testing.T) (address, txid string, height int, found bool) {
	if h.sampleSciAddrResolved {
		return h.sampleSciAddress, h.sampleSciTxID, h.sampleSciHeight, h.sampleSciAddress != "" && h.sampleSciTxID != ""
	}
	h.sampleSciAddrResolved = true

	status := h.getStatus(t)
	lower := status.BestHeight - sciNotationWindow + 1
	if lower < 1 {
		lower = 1
	}

	for height = status.BestHeight; height >= lower; height-- {
		hash, ok := h.getBlockHashForHeight(t, height, false)
		if !ok || strings.TrimSpace(hash) == "" {
			continue
		}

		txids, ok := h.getBlockTxIDsForProbe(t, hash, sciNotationTxLimit)
		if !ok {
			continue
		}

		for _, txid = range txids {
			txid = strings.TrimSpace(txid)
			if txid == "" || !h.txSpecificHasScientificNotationAmount(t, txid) {
				continue
			}

			tx, ok := h.getTransactionByID(t, txid, false)
			if !ok {
				continue
			}
			if h.isEVMTxID(txid) {
				address = firstAddressFromTxPreferVin(tx)
			} else {
				address = firstAddressFromTx(tx)
			}
			if !isAddressCandidate(address) {
				continue
			}

			h.sampleSciAddress = address
			h.sampleSciTxID = txid
			h.sampleSciHeight = height
			return address, txid, height, true
		}
	}

	return "", "", 0, false
}

func (h *TestHandler) getBlockTxIDsForProbe(t *testing.T, hash string, pageSize int) ([]string, bool) {
	t.Helper()

	path := fmt.Sprintf("/api/v2/block/%s?page=1&pageSize=%d", url.PathEscape(hash), pageSize)
	status, body := h.getHTTP(t, path)
	if status != http.StatusOK {
		return nil, false
	}

	var res blockResponse
	if err := json.Unmarshal(body, &res); err != nil {
		t.Fatalf("decode block response for scientific-notation probe %s: %v", hash, err)
	}
	return extractTxIDs(t, res.Txs), true
}

func (h *TestHandler) txSpecificHasScientificNotationAmount(t *testing.T, txid string) bool {
	t.Helper()

	path := "/api/v2/tx-specific/" + url.PathEscape(txid)
	status, body := h.getHTTP(t, path)
	if status != http.StatusOK {
		return false
	}
	return scientificNotationPattern.Match(body)
}

func (h *TestHandler) getSampleIndexedBlock(t *testing.T) (height int, hash string, found bool) {
	if h.sampleBlockResolved {
		return h.sampleBlockHeight, h.sampleBlockHash, h.sampleBlockHash != ""
	}

	h.sampleBlockResolved = true
	startHeight, startHash, ok := h.getSampleIndexedHeight(t)
	if !ok {
		return 0, "", false
	}

	lower := startHeight - sampleBlockProbeMax + 1
	if lower < 1 {
		lower = 1
	}

	for height = startHeight; height >= lower; height-- {
		hash = startHash
		if height != startHeight {
			hash, ok = h.getBlockHashForHeight(t, height, false)
		}
		if !ok || strings.TrimSpace(hash) == "" {
			continue
		}
		// Some backends can briefly expose block-index without serving the block body yet.
		path := fmt.Sprintf("/api/v2/block/%d?page=1&pageSize=%d", height, blockPageSize)
		statusCode, _ := h.getHTTP(t, path)
		if statusCode != http.StatusOK {
			continue
		}
		h.sampleBlockHeight = height
		h.sampleBlockHash = hash
		return height, hash, true
	}
	return 0, "", false
}

func (h *TestHandler) getSampleIndexedHeight(t *testing.T) (height int, hash string, found bool) {
	if h.sampleIndexResolved {
		return h.sampleIndexHeight, h.sampleIndexHash, h.sampleIndexHash != ""
	}
	// If block-ready sample is already known, reuse it.
	if h.sampleBlockResolved && h.sampleBlockHash != "" {
		return h.sampleBlockHeight, h.sampleBlockHash, true
	}

	status := h.getStatus(t)
	start := status.BestHeight
	if start > 2 {
		start -= 2
	}
	lower := start - txSearchWindow
	if lower < 1 {
		lower = 1
	}

	h.sampleIndexResolved = true
	for height = start; height >= lower; height-- {
		hash, ok := h.getBlockHashForHeight(t, height, false)
		if !ok || strings.TrimSpace(hash) == "" {
			continue
		}
		h.sampleIndexHeight = height
		h.sampleIndexHash = hash
		return height, hash, true
	}
	return 0, "", false
}

func firstAddressFromTx(tx *txDetailResponse) string {
	for i := range tx.Vout {
		for _, addr := range tx.Vout[i].Addresses {
			if isAddressCandidate(addr) {
				return addr
			}
		}
	}
	for i := range tx.Vin {
		for _, addr := range tx.Vin[i].Addresses {
			if isAddressCandidate(addr) {
				return addr
			}
		}
	}
	return ""
}

func firstAddressFromTxPreferVin(tx *txDetailResponse) string {
	for i := range tx.Vin {
		for _, addr := range tx.Vin[i].Addresses {
			if isAddressCandidate(addr) {
				return addr
			}
		}
	}
	for i := range tx.Vout {
		for _, addr := range tx.Vout[i].Addresses {
			if isAddressCandidate(addr) {
				return addr
			}
		}
	}
	return ""
}

func isAddressCandidate(addr string) bool {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return false
	}
	upper := strings.ToUpper(addr)
	if strings.HasPrefix(upper, "OP_RETURN") {
		return false
	}
	return !strings.ContainsAny(addr, " \t\r\n")
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

func (h *TestHandler) getBlockByHashForSampling(t *testing.T, hash string, strict bool) (*blockSummary, bool) {
	if blk, found := h.blockByHash[hash]; found && len(blk.TxIDs) >= sampleBlockPageSize {
		return blk, true
	}

	path := fmt.Sprintf("/api/v2/block/%s?page=1&pageSize=%d", url.PathEscape(hash), sampleBlockPageSize)
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

	type candidate struct {
		txid   string
		weight int
	}
	candidates := make([]candidate, 0, len(txs))
	for i := range txs {
		raw := txs[i]
		var asString string
		if err := json.Unmarshal(raw, &asString); err == nil {
			asString = strings.TrimSpace(asString)
			if asString != "" {
				candidates = append(candidates, candidate{
					txid:   asString,
					weight: len(raw),
				})
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
			// Smaller transaction payloads tend to produce faster /tx lookups.
			// Keep deterministic ordering by using the raw message size as a hint.
			candidates = append(candidates, candidate{
				txid:   txid,
				weight: len(raw),
			})
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].weight < candidates[j].weight
	})
	txids := make([]string, 0, len(candidates))
	for i := range candidates {
		txids = append(txids, candidates[i].txid)
	}
	return txids
}

func hasNonEmptyObject(raw json.RawMessage) bool {
	v := strings.TrimSpace(string(raw))
	return v != "" && v != "null" && v != "{}"
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

func (h *TestHandler) getSampleFiatTicker(t *testing.T) (fiatTickerResponse, bool) {
	if h.sampleFiatResolved {
		return h.sampleFiatTicker, h.sampleFiatAvailable
	}
	h.sampleFiatResolved = true

	path := "/api/v2/tickers?currency=usd"
	status, body := h.getHTTP(t, path)
	if isFiatDataUnavailable(status, body) {
		return fiatTickerResponse{}, false
	}
	if status != http.StatusOK {
		t.Fatalf("GET %s returned HTTP %d: %s", path, status, preview(body))
	}

	var ticker fiatTickerResponse
	if err := json.Unmarshal(body, &ticker); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if ticker.Timestamp <= 0 || len(ticker.Rates) == 0 {
		return fiatTickerResponse{}, false
	}

	h.sampleFiatAvailable = true
	h.sampleFiatTicker = ticker
	return h.sampleFiatTicker, true
}

func (h *TestHandler) sampleFiatTickerOrSkip(t *testing.T) fiatTickerResponse {
	t.Helper()
	ticker, found := h.getSampleFiatTicker(t)
	if !found {
		status := h.getStatus(t)
		if !status.HasFiatRates {
			t.Skipf("Skipping test, endpoint reports hasFiatRates=false")
		}
		t.Skipf("Skipping test, fiat ticker data currently unavailable")
	}
	return ticker
}

func (h *TestHandler) requireCapabilities(t *testing.T, required testCapability, group, test string) bool {
	t.Helper()
	if required == capabilityNone {
		return true
	}

	h.resolveCapabilities(t)
	if required&capabilityUTXO != 0 && !h.supportsUTXO {
		reason := h.utxoProbeMessage
		if reason == "" {
			reason = "unsupported by endpoint"
		}
		t.Skipf("Skipping %s (%s): UTXO capability required (%s)", test, group, reason)
		return false
	}
	if required&capabilityEVM != 0 && !h.supportsEVM {
		reason := h.evmProbeMessage
		if reason == "" {
			reason = "unsupported by endpoint"
		}
		t.Skipf("Skipping %s (%s): EVM capability required (%s)", test, group, reason)
		return false
	}
	return true
}

func (h *TestHandler) resolveCapabilities(t *testing.T) {
	t.Helper()
	if h.capabilitiesResolved {
		return
	}
	h.capabilitiesResolved = true
	h.supportsUTXO, h.utxoProbeMessage = h.probeUTXOSupport(t)
	h.supportsEVM, h.evmProbeMessage = h.probeEVMSupport(t)
}

func (h *TestHandler) probeUTXOSupport(t *testing.T) (bool, string) {
	t.Helper()

	txid, found := h.getSampleTxID(t)
	if !found {
		return false, fmt.Sprintf("no sample transaction in last %d blocks", txSearchWindow)
	}
	if h.isEVMTxID(txid) {
		return false, "detected EVM-style transaction ids"
	}

	address, found := h.getSampleAddress(t)
	if !found {
		return false, "no sample address available for probe"
	}

	path := "/api/v2/utxo/" + url.PathEscape(address) + "?confirmed=true"
	status, body := h.getHTTP(t, path)
	if status != http.StatusOK {
		t.Fatalf("UTXO capability probe %s returned HTTP %d: %s", path, status, preview(body))
	}

	var utxos []utxoResponse
	if err := json.Unmarshal(body, &utxos); err != nil {
		t.Fatalf("decode UTXO capability probe %s: %v", path, err)
	}

	return true, "UTXO endpoint probe succeeded"
}

func (h *TestHandler) probeEVMSupport(t *testing.T) (bool, string) {
	t.Helper()

	txid, found := h.getSampleTxID(t)
	if !found {
		return false, fmt.Sprintf("no sample transaction in last %d blocks", txSearchWindow)
	}
	if !h.isEVMTxID(txid) {
		return false, "detected non-EVM transaction ids"
	}

	address, found := h.getSampleAddress(t)
	if !found {
		return false, "no sample address available for probe"
	}
	path := buildAddressDetailsPath(address, "tokenBalances", addressPage, addressPageSize)
	status, body := h.getHTTP(t, path)
	if status != http.StatusOK {
		t.Fatalf("EVM capability probe %s returned HTTP %d: %s", path, status, preview(body))
	}

	var resp evmAddressTokenBalanceResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode EVM capability probe %s: %v", path, err)
	}
	assertAddressMatches(t, resp.Address, address, "EVM capability probe address")
	return true, "EVM tokenBalances endpoint probe succeeded"
}

func (h *TestHandler) isEVMTxID(txid string) bool {
	txid = strings.TrimSpace(txid)
	if strings.HasPrefix(strings.ToLower(txid), "0x") {
		return true
	}
	return h.Coin == "tron" && isFixedHex(txid, 64)
}

func (h *TestHandler) isEVMAddress(address string) bool {
	return isEVMAddress(address) || h.Coin == "tron" && isTronAddress(address)
}

func isEVMAddress(address string) bool {
	address = strings.TrimSpace(address)
	return strings.HasPrefix(strings.ToLower(address), "0x")
}

func isFixedHex(s string, length int) bool {
	if len(s) != length {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

func isTronAddress(address string) bool {
	if len(address) != 34 || address[0] != 'T' {
		return false
	}
	for i := 0; i < len(address); i++ {
		c := address[i]
		switch {
		case c >= '1' && c <= '9':
		case c >= 'A' && c <= 'H':
		case c == 'J' || c == 'K':
		case c >= 'L' && c <= 'N':
		case c >= 'P' && c <= 'Z':
		case c >= 'a' && c <= 'k':
		case c >= 'm' && c <= 'z':
		default:
			return false
		}
	}
	return true
}

func (h *TestHandler) sampleEVMTxIDOrSkip(t *testing.T) string {
	t.Helper()
	txid := h.sampleTxIDOrSkip(t)
	if !h.isEVMTxID(txid) {
		t.Skipf("Skipping test, sample txid %s does not look EVM-like", txid)
	}
	return txid
}

func (h *TestHandler) sampleEVMAddressOrSkip(t *testing.T) string {
	t.Helper()
	address := h.sampleAddressOrSkip(t)
	if !h.isEVMAddress(address) {
		t.Skipf("Skipping test, sample address %s does not look EVM-like", address)
	}
	return address
}

func (h *TestHandler) getSampleEVMContract(t *testing.T) (string, bool) {
	if h.sampleContractResolved {
		return h.sampleContract, h.sampleContract != ""
	}

	address, found := h.getSampleAddress(t)
	h.sampleContractResolved = true
	if !found || !h.isEVMAddress(address) {
		return "", false
	}

	path := buildAddressDetailsPath(address, "tokenBalances", addressPage, addressPageSize)
	status, body := h.getHTTP(t, path)
	if status != http.StatusOK {
		t.Fatalf("GET %s returned HTTP %d: %s", path, status, preview(body))
	}

	var resp evmAddressTokenBalanceResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode tokenBalances for sample contract: %v", err)
	}
	assertAddressMatches(t, resp.Address, address, "sample EVM contract probe address")

	for i := range resp.Tokens {
		contract := strings.TrimSpace(resp.Tokens[i].Contract)
		if contract != "" {
			h.sampleContract = contract
			break
		}
	}
	return h.sampleContract, h.sampleContract != ""
}

func (h *TestHandler) sampleEVMContractOrSkip(t *testing.T) string {
	t.Helper()
	contract, found := h.getSampleEVMContract(t)
	if !found {
		t.Skipf("Skipping test, no contract found for sampled EVM address %s", h.sampleAddress)
	}
	return contract
}
