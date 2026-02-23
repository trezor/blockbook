//go:build unittest

package fiat

import (
	"strings"
	"testing"
)

func testCoinGeckoScopedAPIKeyEnvName(prefix string) string {
	return strings.ToUpper(strings.TrimSpace(prefix)) + coingeckoAPIKeyEnvSuffix
}

func TestResolveCoinGeckoAPIKey(t *testing.T) {
	t.Run("prefers network-specific key", func(t *testing.T) {
		t.Setenv(testCoinGeckoScopedAPIKeyEnvName("OP"), "network-key")
		t.Setenv(testCoinGeckoScopedAPIKeyEnvName("ETH"), "shortcut-key")
		t.Setenv(coingeckoAPIKeyEnv, "global-key")

		got := resolveCoinGeckoAPIKey("op", "eth")
		if got != "network-key" {
			t.Fatalf("unexpected api key: got %q, want %q", got, "network-key")
		}
	})

	t.Run("falls back to shortcut key when network is unrecognized", func(t *testing.T) {
		t.Setenv(testCoinGeckoScopedAPIKeyEnvName("ETH"), "shortcut-key")
		t.Setenv(coingeckoAPIKeyEnv, "global-key")

		got := resolveCoinGeckoAPIKey("unrecognized", "eth")
		if got != "shortcut-key" {
			t.Fatalf("unexpected api key: got %q, want %q", got, "shortcut-key")
		}
	})

	t.Run("falls back to global key when prefixed keys are missing", func(t *testing.T) {
		t.Setenv(coingeckoAPIKeyEnv, "global-key")

		got := resolveCoinGeckoAPIKey("unrecognized", "unknown")
		if got != "global-key" {
			t.Fatalf("unexpected api key: got %q, want %q", got, "global-key")
		}
	})
}

func TestValidateCoinGeckoAPIKeyEnv(t *testing.T) {
	t.Run("network key set empty returns error", func(t *testing.T) {
		networkEnvName := testCoinGeckoScopedAPIKeyEnvName("OP")
		t.Setenv(networkEnvName, "")
		err := validateCoinGeckoAPIKeyEnv("op", "eth")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), networkEnvName) {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("shortcut key set empty returns error when network key unset", func(t *testing.T) {
		shortcutEnvName := testCoinGeckoScopedAPIKeyEnvName("ETH")
		t.Setenv(shortcutEnvName, "   ")
		err := validateCoinGeckoAPIKeyEnv("op", "eth")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), shortcutEnvName) {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("global key set empty returns error", func(t *testing.T) {
		t.Setenv(coingeckoAPIKeyEnv, "")
		err := validateCoinGeckoAPIKeyEnv("op", "eth")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), coingeckoAPIKeyEnv) {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("unset keys are allowed", func(t *testing.T) {
		if err := validateCoinGeckoAPIKeyEnv("op", "eth"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("set non-empty keys are allowed", func(t *testing.T) {
		t.Setenv(testCoinGeckoScopedAPIKeyEnvName("OP"), "network-key")
		t.Setenv(testCoinGeckoScopedAPIKeyEnvName("ETH"), "shortcut-key")
		t.Setenv(coingeckoAPIKeyEnv, "global-key")
		if err := validateCoinGeckoAPIKeyEnv("op", "eth"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
