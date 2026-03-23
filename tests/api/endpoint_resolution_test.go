//go:build integration

package api

import "testing"

func TestResolveAPIEndpointsUsesConfigFilenameWhenTestNameMissing(t *testing.T) {
	t.Setenv("BB_DEV_API_URL_HTTP_polygon", "https://blockbook.example.invalid/polygon")
	t.Setenv("BB_DEV_API_URL_WS_polygon", "wss://blockbook.example.invalid/polygon/websocket")

	endpoints, err := resolveAPIEndpoints("polygon")
	if err != nil {
		t.Fatalf("resolveAPIEndpoints() error = %v", err)
	}
	if endpoints.HTTP != "https://blockbook.example.invalid/polygon" {
		t.Fatalf("HTTP endpoint = %q", endpoints.HTTP)
	}
	if endpoints.WS != "wss://blockbook.example.invalid/polygon/websocket" {
		t.Fatalf("WS endpoint = %q", endpoints.WS)
	}
}

func TestResolveAPIEndpointsUsesCoinTestNameWhenPresent(t *testing.T) {
	t.Setenv("BB_DEV_API_URL_HTTP_polygon", "https://blockbook.example.invalid/polygon")
	t.Setenv("BB_DEV_API_URL_WS_polygon", "wss://blockbook.example.invalid/polygon/websocket")

	endpoints, err := resolveAPIEndpoints("polygon_archive")
	if err != nil {
		t.Fatalf("resolveAPIEndpoints() error = %v", err)
	}
	if endpoints.HTTP != "https://blockbook.example.invalid/polygon" {
		t.Fatalf("HTTP endpoint = %q", endpoints.HTTP)
	}
	if endpoints.WS != "wss://blockbook.example.invalid/polygon/websocket" {
		t.Fatalf("WS endpoint = %q", endpoints.WS)
	}
}
