package tron

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

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
	// Drain the body before closing so net/http can return the connection to the
	// idle pool for keep-alive reuse. Without this, the >=300 early return (which
	// reads nothing) and any bytes the JSON decoder leaves unconsumed would force
	// the connection closed, defeating the MaxIdleConns* pooling configured above.
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("Tron API returned status %d at path: %s %s", resp.StatusCode, c.baseURL, path)
	}

	if respBody != nil {
		return json.NewDecoder(resp.Body).Decode(respBody)
	}

	return nil
}
