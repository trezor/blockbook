package tron

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Connection-pool sizing for the Tron HTTP node clients.
//
// Each TronHTTPClient talks to a single node (full node or solidity node), so
// these caps are effectively per-node. During initial sync GetBlock issues two
// concurrent solidity calls per block (getblockbynum + gettransactioninfobyblocknum)
// across a pool of block-sync workers, so the peak in-flight request count to one
// node is well above Go's http.DefaultTransport default of 2 idle connections per
// host. With only 2 idle connections kept, the transport re-dials (TCP + TLS) on
// every demand dip; those fresh dials land on the request critical path and form
// the latency tail. Benchmarking the sync call pattern (contrib/scripts/tron-sync-bench)
// against a real node showed raising this cap removes the accumulated handshake tax
// and cuts sync-path p99 by ~32% with ~14% higher block throughput.
//
// tronMaxIdleConnsPerHost is sized to comfortably exceed the sync fan-out to one
// node; tronMaxIdleConns is the per-transport global cap, kept above the per-host
// cap so it does not bind first.
const (
	tronMaxIdleConnsPerHost = 64
	tronMaxIdleConns        = 100
)

type TronHTTP interface {
	Request(ctx context.Context, path string, reqBody interface{}, respBody interface{}) error
}

type TronHTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewTronHTTPClient(baseURL string, timeout time.Duration) *TronHTTPClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = tronMaxIdleConns
	transport.MaxIdleConnsPerHost = tronMaxIdleConnsPerHost

	return &TronHTTPClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

func (c *TronHTTPClient) Request(ctx context.Context, path string, reqBody interface{}, respBody interface{}) error {
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to encode request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP error calling Tron API %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("Tron API returned status %d at path: %s %s", resp.StatusCode, c.baseURL, path)
	}

	if respBody != nil {
		return json.NewDecoder(resp.Body).Decode(respBody)
	}

	return nil
}
