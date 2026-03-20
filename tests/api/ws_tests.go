//go:build integration

package api

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func testWsGetInfo(t *testing.T, h *TestHandler) {
	info := h.wsGetInfo(t)
	if info.BestHeight <= 0 {
		t.Fatalf("invalid websocket bestHeight: %d", info.BestHeight)
	}
	assertNonEmptyString(t, info.BestHash, "WsGetInfo.bestHash")
}

func testWsGetBlockHash(t *testing.T, h *TestHandler) {
	info := h.wsGetInfo(t)
	if info.BestHeight <= 0 {
		t.Fatalf("invalid websocket bestHeight: %d", info.BestHeight)
	}

	hashResp := h.wsCall(t, "getBlockHash", map[string]int{"height": info.BestHeight})
	var got wsBlockHashResponse
	if err := json.Unmarshal(hashResp.Data, &got); err != nil {
		t.Fatalf("decode getBlockHash response: %v", err)
	}
	assertNonEmptyString(t, got.Hash, "WsGetBlockHash.hash")

	want, ok := h.getBlockHashForHeight(t, info.BestHeight, true)
	if ok {
		assertEqualString(t, got.Hash, want, "websocket block hash")
	}
}

func testWsGetTransaction(t *testing.T, h *TestHandler) {
	txid := h.sampleTxIDOrSkip(t)

	resp := h.wsCall(t, "getTransaction", map[string]string{"txid": txid})
	var tx txDetailResponse
	if err := json.Unmarshal(resp.Data, &tx); err != nil {
		t.Fatalf("decode websocket getTransaction response: %v", err)
	}
	assertNonEmptyString(t, tx.Txid, "WsGetTransaction.txid")
	assertEqualString(t, tx.Txid, txid, "websocket transaction txid")
}

func testWsGetAccountInfo(t *testing.T, h *TestHandler) {
	address := h.sampleAddressOrSkip(t)
	txid := h.sampleTxIDOrSkip(t)

	resp := h.wsCall(t, "getAccountInfo", map[string]interface{}{
		"descriptor": address,
		"details":    "txids",
		"page":       addressPage,
		"pageSize":   addressPageSize,
	})

	var info addressTxidsResponse
	if err := json.Unmarshal(resp.Data, &info); err != nil {
		t.Fatalf("decode websocket getAccountInfo response: %v", err)
	}
	assertAddressTxidsPayload(t, &info, address, txid, "WsGetAccountInfo", addressPageSize)
}

func testWsGetAccountUtxo(t *testing.T, h *TestHandler) {
	address := h.sampleAddressOrSkip(t)

	resp := h.wsCall(t, "getAccountUtxo", map[string]interface{}{
		"descriptor": address,
	})

	var utxos []utxoResponse
	if err := json.Unmarshal(resp.Data, &utxos); err != nil {
		t.Fatalf("decode websocket getAccountUtxo response: %v", err)
	}
	assertUTXOListNonNegativeConfirmations(t, utxos, "WsGetAccountUtxo")
}

func testWsPing(t *testing.T, h *TestHandler) {
	const reqID = "ping-check-id"
	resp := h.wsCallWithID(t, reqID, "ping", map[string]interface{}{})
	assertEqualString(t, resp.ID, reqID, "websocket ping response id")

	var data map[string]json.RawMessage
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode ping response: %v", err)
	}
	if _, hasError := data["error"]; hasError {
		t.Fatalf("websocket ping returned error payload: %s", string(resp.Data))
	}
}

func (h *TestHandler) wsGetInfo(t *testing.T) *wsInfoResponse {
	t.Helper()
	resp := h.wsCall(t, "getInfo", map[string]interface{}{})
	var info wsInfoResponse
	if err := json.Unmarshal(resp.Data, &info); err != nil {
		t.Fatalf("decode getInfo response: %v", err)
	}
	return &info
}

func (h *TestHandler) wsCall(t *testing.T, method string, params interface{}) *wsResponse {
	h.nextWSReq++
	reqID := fmt.Sprintf("api-%s-%d", method, h.nextWSReq)
	return h.wsCallWithID(t, reqID, method, params)
}

func (h *TestHandler) wsCallWithID(t *testing.T, reqID, method string, params interface{}) *wsResponse {
	t.Helper()

	dialer := websocket.Dialer{
		HandshakeTimeout: wsDialTimeout,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
	}

	conn, _, err := dialer.Dial(h.WSURL, nil)
	if err != nil {
		upgradeURL, ok := upgradeWSBaseToWSS(h.WSURL)
		if ok {
			conn, _, err = dialer.Dial(upgradeURL, nil)
			if err == nil {
				h.WSURL = upgradeURL
			}
		}
	}
	if err != nil {
		t.Fatalf("websocket dial %s: %v", h.WSURL, err)
	}
	defer conn.Close()

	req := wsRequest{ID: reqID, Method: method, Params: params}

	conn.SetWriteDeadline(time.Now().Add(wsMessageTimeout))
	if err := conn.WriteJSON(&req); err != nil {
		t.Fatalf("websocket write %s: %v", method, err)
	}

	for i := 0; i < 5; i++ {
		conn.SetReadDeadline(time.Now().Add(wsMessageTimeout))
		_, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("websocket read %s: %v", method, err)
		}

		var resp wsResponse
		if err := json.Unmarshal(payload, &resp); err != nil {
			t.Fatalf("decode websocket response for %s: %v", method, err)
		}
		if resp.ID != reqID {
			continue
		}
		if msg, hasError := websocketError(resp.Data); hasError {
			t.Fatalf("websocket %s returned error: %s", method, msg)
		}
		return &resp
	}

	t.Fatalf("missing websocket response for %s request id %s", method, reqID)
	return nil
}

func websocketError(data json.RawMessage) (string, bool) {
	var e struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &e); err != nil {
		return "", false
	}
	if e.Error == nil {
		return "", false
	}
	return e.Error.Message, true
}
