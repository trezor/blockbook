//go:build integration

package connectivity

import (
	"crypto/tls"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/trezor/blockbook/bchain"
	apitests "github.com/trezor/blockbook/tests/api"
)

type blockbookStatusEnvelope struct {
	Blockbook json.RawMessage `json:"blockbook"`
	Backend   json.RawMessage `json:"backend"`
}

type blockbookWSRequest struct {
	ID     string      `json:"id"`
	Method string      `json:"method"`
	Params interface{} `json:"params"`
}

type blockbookWSResponse struct {
	ID   string          `json:"id"`
	Data json.RawMessage `json:"data"`
}

type blockbookWSInfo struct {
	BestHeight int    `json:"bestHeight"`
	BestHash   string `json:"bestHash"`
}

func BlockbookHTTPIntegrationTest(t *testing.T, coin string, _ bchain.BlockChain, _ bchain.Mempool, _ json.RawMessage) {
	t.Helper()

	httpBase, _, err := apitests.ResolveEndpoints(coin)
	if err != nil {
		t.Fatalf("resolve Blockbook endpoints for %s: %v", coin, err)
	}

	client := &http.Client{
		Timeout: connectivityTimeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	status, body, err := blockbookHTTPGet(client, httpBase, "/api/status")
	if err != nil {
		t.Fatalf("GET %s/api/status: %v", httpBase, err)
	}
	if shouldUpgradeToHTTPS(status, body, httpBase) {
		if upgraded, ok := upgradeHTTPBaseToHTTPS(httpBase); ok {
			httpBase = upgraded
			status, body, err = blockbookHTTPGet(client, httpBase, "/api/status")
			if err != nil {
				t.Fatalf("GET %s/api/status: %v", httpBase, err)
			}
		}
	}

	if status != http.StatusOK {
		t.Fatalf("GET %s/api/status returned HTTP %d: %s", httpBase, status, previewBody(body))
	}

	var envelope blockbookStatusEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode %s/api/status: %v", httpBase, err)
	}
	if !hasNonEmptyJSON(envelope.Blockbook) {
		t.Fatalf("status response missing non-empty blockbook object")
	}
	if !hasNonEmptyJSON(envelope.Backend) {
		t.Fatalf("status response missing non-empty backend object")
	}
}

func BlockbookWSIntegrationTest(t *testing.T, coin string, _ bchain.BlockChain, _ bchain.Mempool, _ json.RawMessage) {
	t.Helper()

	_, wsURL, err := apitests.ResolveEndpoints(coin)
	if err != nil {
		t.Fatalf("resolve Blockbook endpoints for %s: %v", coin, err)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: connectivityTimeout,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true},
	}

	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		if upgraded, ok := upgradeWSBaseToWSS(wsURL); ok {
			conn, _, err = dialer.Dial(upgraded, nil)
			if err == nil {
				wsURL = upgraded
			}
		}
	}
	if err != nil {
		t.Fatalf("websocket dial %s: %v", wsURL, err)
	}
	defer conn.Close()

	reqID := "connectivity-getinfo"
	req := blockbookWSRequest{
		ID:     reqID,
		Method: "getInfo",
		Params: map[string]interface{}{},
	}

	conn.SetWriteDeadline(time.Now().Add(connectivityTimeout))
	if err := conn.WriteJSON(&req); err != nil {
		t.Fatalf("websocket write getInfo: %v", err)
	}

	for i := 0; i < 5; i++ {
		conn.SetReadDeadline(time.Now().Add(connectivityTimeout))
		_, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("websocket read getInfo: %v", err)
		}

		var resp blockbookWSResponse
		if err := json.Unmarshal(payload, &resp); err != nil {
			t.Fatalf("decode websocket response: %v", err)
		}
		if resp.ID != reqID {
			continue
		}
		if msg, hasError := blockbookWebsocketError(resp.Data); hasError {
			t.Fatalf("websocket getInfo returned error: %s", msg)
		}

		var info blockbookWSInfo
		if err := json.Unmarshal(resp.Data, &info); err != nil {
			t.Fatalf("decode websocket getInfo payload: %v", err)
		}
		if info.BestHeight < 0 {
			t.Fatalf("invalid websocket bestHeight: %d", info.BestHeight)
		}
		if strings.TrimSpace(info.BestHash) == "" {
			t.Fatalf("empty websocket bestHash")
		}
		return
	}

	t.Fatalf("missing websocket getInfo response for request id %s", reqID)
}

func blockbookHTTPGet(client *http.Client, baseURL, path string) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, resolveHTTPURL(baseURL, path), nil)
	if err != nil {
		return 0, nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, nil, err
	}
	return resp.StatusCode, body, nil
}

func resolveHTTPURL(baseURL, path string) string {
	if strings.HasPrefix(path, "/") {
		return baseURL + path
	}
	return baseURL + "/" + path
}

func shouldUpgradeToHTTPS(status int, body []byte, baseURL string) bool {
	if status != http.StatusBadRequest {
		return false
	}
	if !strings.Contains(strings.ToLower(string(body)), "http request to an https server") {
		return false
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	return parsed.Scheme == "http"
}

func upgradeHTTPBaseToHTTPS(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" {
		return "", false
	}
	u.Scheme = "https"
	return strings.TrimRight(u.String(), "/"), true
}

func upgradeWSBaseToWSS(raw string) (string, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "ws" {
		return "", false
	}
	u.Scheme = "wss"
	return u.String(), true
}

func blockbookWebsocketError(data json.RawMessage) (string, bool) {
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

func hasNonEmptyJSON(raw json.RawMessage) bool {
	v := strings.TrimSpace(string(raw))
	return v != "" && v != "null" && v != "{}"
}

func previewBody(body []byte) string {
	const max = 256
	s := strings.TrimSpace(string(body))
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
