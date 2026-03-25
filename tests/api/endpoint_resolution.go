//go:build integration

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ResolveEndpoints resolves Blockbook API endpoints for a coin alias using
// exact BB_DEV_API_URL_* overrides first and coin config fallbacks.
func ResolveEndpoints(coin string) (string, string, error) {
	ep, err := resolveAPIEndpoints(coin)
	if err != nil {
		return "", "", err
	}
	return ep.HTTP, ep.WS, nil
}

func resolveAPIEndpoints(coin string) (*apiEndpoints, error) {
	cfg, err := loadCoinConfig(coin)
	if err != nil {
		return nil, err
	}

	testIdentity := strings.TrimSpace(cfg.Coin.TestName)
	if testIdentity == "" {
		testIdentity = coin
	}

	httpURL := ""
	if v, ok := os.LookupEnv("BB_DEV_API_URL_HTTP_" + testIdentity); ok {
		httpURL = strings.TrimSpace(v)
	}
	if httpURL == "" {
		if cfg.Ports.BlockbookPublic == 0 {
			return nil, fmt.Errorf("missing ports.blockbook_public for %s", coin)
		}
		httpURL = fmt.Sprintf("http://127.0.0.1:%d", cfg.Ports.BlockbookPublic)
	}
	httpURL, err = normalizeHTTPBase(httpURL)
	if err != nil {
		return nil, err
	}

	wsURL := ""
	if v, ok := os.LookupEnv("BB_DEV_API_URL_WS_" + testIdentity); ok {
		wsURL = strings.TrimSpace(v)
	}
	if wsURL == "" {
		wsURL, err = deriveWSFromHTTP(httpURL)
	} else {
		wsURL, err = normalizeWSBase(wsURL)
	}
	if err != nil {
		return nil, err
	}

	return &apiEndpoints{HTTP: httpURL, WS: wsURL}, nil
}

func loadCoinConfig(coin string) (*coinConfig, error) {
	path, err := coinConfigPath(coin)
	if err != nil {
		return nil, err
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg coinConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return &cfg, nil
}

func coinConfigPath(coin string) (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("unable to resolve caller path")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	path := filepath.Join(repoRoot, "configs", "coins", coin+".json")
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return path, nil
}

func normalizeHTTPBase(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported HTTP scheme %q in %q", u.Scheme, raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("missing host in %q", raw)
	}
	if u.Path == "" {
		u.Path = "/"
	}
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func normalizeWSBase(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}

	switch u.Scheme {
	case "ws", "wss":
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return "", fmt.Errorf("unsupported WS scheme %q in %q", u.Scheme, raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("missing host in %q", raw)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/websocket"
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func deriveWSFromHTTP(httpBase string) (string, error) {
	u, err := url.Parse(httpBase)
	if err != nil {
		return "", err
	}

	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	default:
		return "", fmt.Errorf("cannot derive WS URL from scheme %q", u.Scheme)
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = "/websocket"
	} else {
		u.Path = strings.TrimRight(u.Path, "/") + "/websocket"
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
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
