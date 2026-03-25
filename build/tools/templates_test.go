package build

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

func TestResolveBuildEnvDefaultsToDev(t *testing.T) {
	t.Setenv(buildEnvVar, "")

	got, err := resolveBuildEnv()
	if err != nil {
		t.Fatalf("resolveBuildEnv() error = %v", err)
	}
	if got != buildEnvDev {
		t.Fatalf("resolveBuildEnv() = %q, want %q", got, buildEnvDev)
	}
}

func TestResolveBuildEnvUsesExplicitProd(t *testing.T) {
	t.Setenv(buildEnvVar, buildEnvProd)

	got, err := resolveBuildEnv()
	if err != nil {
		t.Fatalf("resolveBuildEnv() error = %v", err)
	}
	if got != buildEnvProd {
		t.Fatalf("resolveBuildEnv() = %q, want %q", got, buildEnvProd)
	}
}

func TestResolveBuildEnvRejectsInvalidValue(t *testing.T) {
	t.Setenv(buildEnvVar, "staging")

	if _, err := resolveBuildEnv(); err == nil {
		t.Fatal("expected invalid BB_BUILD_ENV to fail")
	}
}

func TestLookupEnvWithArchiveFallback_PrefersExactAlias(t *testing.T) {
	const prefix = "TEST_LOOKUP_PREFIX_"
	t.Setenv(prefix+"base", "https://base")
	t.Setenv(prefix+"base_archive", "https://base-archive")

	got, ok := lookupEnvWithArchiveFallback(prefix, "base")
	if !ok {
		t.Fatal("expected exact alias lookup to succeed")
	}
	if got != "https://base" {
		t.Fatalf("expected exact alias to win, got %q", got)
	}
}

func TestLookupEnvWithArchiveFallback_UsesArchiveSuffixFallback(t *testing.T) {
	const prefix = "TEST_LOOKUP_PREFIX_"
	t.Setenv(prefix+"base_archive", "https://base-archive")

	got, ok := lookupEnvWithArchiveFallback(prefix, "base")
	if !ok {
		t.Fatal("expected suffix archive fallback to succeed")
	}
	if got != "https://base-archive" {
		t.Fatalf("unexpected suffix fallback value %q", got)
	}
}

func TestLookupEnvWithArchiveFallback_UsesArchiveInfixFallback(t *testing.T) {
	const prefix = "TEST_LOOKUP_PREFIX_"
	t.Setenv(prefix+"polygon_archive_bor", "https://polygon-archive")

	got, ok := lookupEnvWithArchiveFallback(prefix, "polygon_bor")
	if !ok {
		t.Fatal("expected infix archive fallback to succeed")
	}
	if got != "https://polygon-archive" {
		t.Fatalf("unexpected infix fallback value %q", got)
	}
}

func TestLookupEnvWithArchiveFallback_DoesNotDoubleArchive(t *testing.T) {
	const prefix = "TEST_LOOKUP_PREFIX_"
	t.Setenv(prefix+"polygon_archive_archive_bor", "https://invalid")
	t.Setenv(prefix+"polygon_archive_bor_archive", "https://invalid")

	if _, ok := lookupEnvWithArchiveFallback(prefix, "polygon_archive_bor"); ok {
		t.Fatal("unexpected lookup success for duplicate archive alias variants")
	}
}

func TestRPCURLUsesLoopback(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "localhost", raw: "http://localhost:8030", want: true},
		{name: "loopback-ipv4", raw: "http://127.0.0.1:8030", want: true},
		{name: "loopback-ipv6", raw: "http://[::1]:8030", want: true},
		{name: "remote", raw: "https://backend5.sldev.cz:8030", want: false},
		{name: "invalid", raw: "not-a-url", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rpcURLUsesLoopback(tt.raw); got != tt.want {
				t.Fatalf("rpcURLUsesLoopback(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestLoadConfigSetsWantsBackendServiceFromEffectiveRPCURL(t *testing.T) {
	configsDir := filepath.Clean(filepath.Join("..", "..", "configs"))

	t.Run("default-loopback-template", func(t *testing.T) {
		withTemporarilyUnsetEnv(t,
			buildEnvVar,
			devRPCURLHTTPPrefix+"bitcoin",
			devRPCURLHTTPPrefix+"bitcoin_archive",
			prodRPCURLHTTPPrefix+"bitcoin",
			prodRPCURLHTTPPrefix+"bitcoin_archive",
		)

		config, err := LoadConfig(configsDir, "bitcoin")
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if !config.Env.WantsBackendService {
			t.Fatal("expected WantsBackendService for default localhost RPC")
		}
	})

	t.Run("remote-dev-override", func(t *testing.T) {
		t.Setenv(buildEnvVar, buildEnvDev)
		t.Setenv(devRPCURLHTTPPrefix+"bitcoin", "http://backend5.sldev.cz:8030")

		config, err := LoadConfig(configsDir, "bitcoin")
		if err != nil {
			t.Fatalf("LoadConfig() error = %v", err)
		}
		if config.Env.WantsBackendService {
			t.Fatal("did not expect WantsBackendService for remote RPC override")
		}
	})
}

func TestBlockbookServiceTemplateGatesWantsLine(t *testing.T) {
	config := &Config{}
	config.Coin.Name = "Bitcoin"
	config.Coin.Alias = "bitcoin"
	config.Backend.PackageName = "backend-bitcoin"
	config.Blockbook.SystemUser = "blockbook"
	config.Blockbook.ExplorerURL = "https://example.invalid"
	config.Env.BlockbookInstallPath = "/opt/coins/blockbook"
	config.Env.BlockbookDataPath = "/var/lib/blockbook"
	config.Blockbook.InternalBindingTemplate = "127.0.0.1:9130"
	config.Blockbook.PublicBindingTemplate = "127.0.0.1:9130"

	renderService := func(t *testing.T, wants bool) string {
		t.Helper()
		config.Env.WantsBackendService = wants

		templ := config.ParseTemplate()
		templ = template.Must(templ.ParseFiles(filepath.Join("..", "templates", "blockbook", "debian", "service")))

		var out bytes.Buffer
		if err := templ.ExecuteTemplate(&out, "main", config); err != nil {
			t.Fatalf("ExecuteTemplate() error = %v", err)
		}
		return out.String()
	}

	if rendered := renderService(t, true); !strings.Contains(rendered, "Wants=backend-bitcoin.service") {
		t.Fatalf("expected Wants line in rendered service:\n%s", rendered)
	}
	if rendered := renderService(t, false); strings.Contains(rendered, "Wants=backend-bitcoin.service") {
		t.Fatalf("did not expect Wants line in rendered service:\n%s", rendered)
	}
}

func withTemporarilyUnsetEnv(t *testing.T, keys ...string) {
	t.Helper()

	restore := make(map[string]*string, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			valueCopy := value
			restore[key] = &valueCopy
		} else {
			restore[key] = nil
		}
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("Unsetenv(%q) error = %v", key, err)
		}
	}

	t.Cleanup(func() {
		for key, value := range restore {
			if value == nil {
				_ = os.Unsetenv(key)
				continue
			}
			_ = os.Setenv(key, *value)
		}
	})
}
