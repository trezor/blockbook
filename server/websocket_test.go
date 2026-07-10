//go:build unittest
// +build unittest

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/trezor/blockbook/api"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

func TestCheckOriginAllowAll(t *testing.T) {
	s := &WebsocketServer{}
	tests := []struct {
		name   string
		origin string
		want   bool
	}{
		{
			name: "no origin",
			want: true,
		},
		{
			name:   "valid origin",
			origin: "https://example.com",
			want:   true,
		},
		{
			name:   "invalid origin",
			origin: "://bad",
			want:   true,
		},
		{
			name:   "null origin",
			origin: "null",
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{Header: make(http.Header)}
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			got := s.checkOrigin(r)
			if got != tt.want {
				t.Fatalf("checkOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckOriginAllowlist(t *testing.T) {
	allowedOrigins := make(map[string]struct{})
	for _, origin := range []string{"https://example.com", "http://localhost:3000"} {
		normalizedOrigin, ok := normalizeOrigin(origin)
		if !ok {
			t.Fatalf("normalizeOrigin(%q) failed", origin)
		}
		allowedOrigins[normalizedOrigin] = struct{}{}
	}
	s := &WebsocketServer{allowedOrigins: allowedOrigins}

	tests := []struct {
		name   string
		origin string
		want   bool
	}{
		{
			name: "no origin",
			want: true,
		},
		{
			name:   "allowed origin",
			origin: "https://example.com",
			want:   true,
		},
		{
			name:   "allowed origin different case",
			origin: "HTTP://LOCALHOST:3000",
			want:   true,
		},
		{
			name:   "disallowed origin",
			origin: "https://evil.com",
			want:   false,
		},
		{
			name:   "invalid origin",
			origin: "://bad",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{Header: make(http.Header)}
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			got := s.checkOrigin(r)
			if got != tt.want {
				t.Fatalf("checkOrigin() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseAllowedOrigins(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want []string
	}{
		{
			name: "empty",
			env:  "",
			want: nil,
		},
		{
			name: "valid entries",
			env:  "https://example.com,http://localhost:3000",
			want: []string{"https://example.com", "http://localhost:3000"},
		},
		{
			name: "trims and normalizes",
			env:  " HTTPS://Example.com:9130 , http://LOCALHOST:3000 ",
			want: []string{"https://example.com:9130", "http://localhost:3000"},
		},
		{
			name: "invalid filtered",
			env:  "https://example.com,://bad,",
			want: []string{"https://example.com"},
		},
		{
			name: "all invalid",
			env:  "://bad,not-a-url",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseAllowedOrigins("FAKE_WS_ALLOWED_ORIGINS", tt.env)
			if len(got) != len(tt.want) {
				t.Fatalf("parseAllowedOrigins() len = %d, want %d", len(got), len(tt.want))
			}
			for _, origin := range tt.want {
				if _, ok := got[origin]; !ok {
					t.Fatalf("parseAllowedOrigins() missing %q", origin)
				}
			}
		})
	}
}

func TestParseAllowedEvmCallMethods(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		want    []string
		wantErr bool
	}{
		{name: "empty", env: "", want: nil},
		{name: "empty entries only", env: " , ,", wantErr: true},
		{name: "single selector", env: "0xdd62ed3e", want: []string{"dd62ed3e"}},
		{name: "without prefix", env: "dd62ed3e", want: []string{"dd62ed3e"}},
		{name: "uppercase", env: "0XDD62ED3E", want: []string{"dd62ed3e"}},
		{name: "multiple with spaces", env: " 0xdd62ed3e , 0x70a08231 ", want: []string{"dd62ed3e", "70a08231"}},
		{name: "empty entries skipped", env: "0xdd62ed3e,,0x70a08231", want: []string{"dd62ed3e", "70a08231"}},
		{name: "too short", env: "0xdd62ed", wantErr: true},
		{name: "too long", env: "0xdd62ed3e00", wantErr: true},
		{name: "odd length", env: "0xdd62ed3", wantErr: true},
		{name: "non hex", env: "0xdd62ed3g", wantErr: true},
		{name: "bare prefix", env: "0x", wantErr: true},
		{name: "one invalid among valid", env: "0xdd62ed3e,invalid", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAllowedEvmCallMethods("FAKE_ALLOWED_EVM_CALL_METHODS", tt.env)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseAllowedEvmCallMethods(%q) = nil err, want error", tt.env)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAllowedEvmCallMethods(%q) unexpected error: %v", tt.env, err)
			}
			if tt.want == nil && got != nil {
				t.Fatalf("parseAllowedEvmCallMethods(%q) = %v, want nil", tt.env, got)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseAllowedEvmCallMethods(%q) len = %d, want %d", tt.env, len(got), len(tt.want))
			}
			for _, selector := range tt.want {
				if _, ok := got[selector]; !ok {
					t.Fatalf("parseAllowedEvmCallMethods(%q) missing %q", tt.env, selector)
				}
			}
		})
	}
}

func TestRpcCallAllowed(t *testing.T) {
	// allowance(owner, spender) calldata with the 0xdd62ed3e selector
	const allowanceData = "0xdd62ed3e0000000000000000000000009ea3721b5bf3b64b4418c38b603154d2d597fae3000000000000000000000000e4db1c5a1b709ce4d2ada6985d9d506e58f73829"
	const allowedTo = "0xcdA9FC258358EcaA88845f19Af595e908bb7EfE9"
	const otherTo = "0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599"
	allowedToSet := map[string]struct{}{strings.ToLower(allowedTo): {}}
	allowanceSelectorSet := map[string]struct{}{"dd62ed3e": {}}

	tests := []struct {
		name    string
		to      map[string]struct{}
		methods map[string]struct{}
		req     WsRpcCallReq
		want    bool
	}{
		{
			name: "no allowlists configured allows all",
			req:  WsRpcCallReq{To: otherTo, Data: "0x12345678"},
			want: true,
		},
		{
			name: "allowed address passes",
			to:   allowedToSet,
			req:  WsRpcCallReq{To: allowedTo, Data: "0x12345678"},
			want: true,
		},
		{
			name: "allowed address is case-insensitive",
			to:   allowedToSet,
			req:  WsRpcCallReq{To: strings.ToUpper(allowedTo), Data: "0x12345678"},
			want: true,
		},
		{
			name: "address not allowed without methods list",
			to:   allowedToSet,
			req:  WsRpcCallReq{To: otherTo, Data: allowanceData},
			want: false,
		},
		{
			name:    "allowed address passes regardless of selector",
			to:      allowedToSet,
			methods: allowanceSelectorSet,
			req:     WsRpcCallReq{To: allowedTo, Data: "0x12345678"},
			want:    true,
		},
		{
			name:    "allowed selector passes to any address",
			to:      allowedToSet,
			methods: allowanceSelectorSet,
			req:     WsRpcCallReq{To: otherTo, Data: allowanceData},
			want:    true,
		},
		{
			name:    "selector alone is enough for exact 4 byte calldata",
			methods: allowanceSelectorSet,
			req:     WsRpcCallReq{To: otherTo, Data: "0xdd62ed3e"},
			want:    true,
		},
		{
			name:    "uppercase hex calldata matches",
			methods: allowanceSelectorSet,
			req:     WsRpcCallReq{To: otherTo, Data: "0XDD62ED3E"},
			want:    true,
		},
		{
			name:    "only methods list set rejects other selectors",
			methods: allowanceSelectorSet,
			req:     WsRpcCallReq{To: otherTo, Data: "0x70a08231000000000000000000000000e4db1c5a1b709ce4d2ada6985d9d506e58f73829"},
			want:    false,
		},
		{
			name:    "not allowed address nor selector",
			to:      allowedToSet,
			methods: allowanceSelectorSet,
			req:     WsRpcCallReq{To: otherTo, Data: "0x12345678"},
			want:    false,
		},
		{
			name:    "missing 0x prefix fails closed",
			methods: allowanceSelectorSet,
			req:     WsRpcCallReq{To: otherTo, Data: allowanceData[2:]},
			want:    false,
		},
		{
			name:    "odd length calldata fails closed",
			methods: allowanceSelectorSet,
			req:     WsRpcCallReq{To: otherTo, Data: allowanceData + "0"},
			want:    false,
		},
		{
			name:    "non hex tail after valid selector fails closed",
			methods: allowanceSelectorSet,
			req:     WsRpcCallReq{To: otherTo, Data: "0xdd62ed3exx"},
			want:    false,
		},
		{
			name:    "calldata shorter than selector fails closed",
			methods: allowanceSelectorSet,
			req:     WsRpcCallReq{To: otherTo, Data: "0xdd62ed"},
			want:    false,
		},
		{
			name:    "empty calldata fails closed",
			methods: allowanceSelectorSet,
			req:     WsRpcCallReq{To: otherTo, Data: ""},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			is := &common.InternalState{}
			is.SetRpcCallAllowlists(&common.RpcCallAllowlists{To: tt.to, Methods: tt.methods})
			s := &WebsocketServer{is: is}
			if got := s.rpcCallAllowed(&tt.req); got != tt.want {
				t.Fatalf("rpcCallAllowed(to=%q, data=%q) = %v, want %v", tt.req.To, tt.req.Data, got, tt.want)
			}
		})
	}

	t.Run("nil snapshot allows all", func(t *testing.T) {
		s := &WebsocketServer{is: &common.InternalState{}}
		if !s.rpcCallAllowed(&WsRpcCallReq{To: otherTo, Data: "0x12345678"}) {
			t.Fatal("rpcCallAllowed with uninitialized snapshot = false, want true")
		}
	})
}

func TestParseTrustedProxies(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		want      []string
		wantErr   bool
		errSubstr string
	}{
		{name: "empty value yields nil", value: "", want: nil},
		{name: "whitespace only yields nil", value: "  , ,  ", want: nil},
		{name: "single ipv4 cidr", value: "203.0.113.0/24", want: []string{"203.0.113.0/24"}},
		{name: "multiple cidrs with spaces", value: " 203.0.113.0/24 , 2001:db8::/32 ", want: []string{"203.0.113.0/24", "2001:db8::/32"}},
		{name: "single host as /32 is fine", value: "10.0.0.5/32", want: []string{"10.0.0.5/32"}},
		{name: "rejects 0.0.0.0/0", value: "0.0.0.0/0", wantErr: true, errSubstr: "too broad"},
		{name: "rejects ::/0", value: "::/0", wantErr: true, errSubstr: "too broad"},
		{name: "rejects ipv4 broader than /8", value: "10.0.0.0/4", wantErr: true, errSubstr: "too broad"},
		{name: "rejects ipv6 broader than /16", value: "2000::/8", wantErr: true, errSubstr: "too broad"},
		{name: "rejects broad ipv4-mapped cidr", value: "::ffff:0.0.0.0/0", wantErr: true, errSubstr: "IPv4-mapped"},
		{name: "rejects specific ipv4-mapped cidr", value: "::ffff:192.0.2.0/120", wantErr: true, errSubstr: "IPv4-mapped"},
		{name: "rejects malformed cidr", value: "not-a-cidr", wantErr: true, errSubstr: "invalid CIDR"},
		{name: "rejects bare ip without prefix", value: "10.0.0.5", wantErr: true, errSubstr: "invalid CIDR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseTrustedProxies("TEST_ENV", tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseTrustedProxies(%q) = nil err, want error containing %q", tt.value, tt.errSubstr)
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Fatalf("parseTrustedProxies(%q) err = %q, want substring %q", tt.value, err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTrustedProxies(%q) unexpected error: %v", tt.value, err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseTrustedProxies(%q) = %v, want %v", tt.value, got, tt.want)
			}
			for i, p := range got {
				if p.String() != tt.want[i] {
					t.Errorf("parseTrustedProxies(%q)[%d] = %q, want %q", tt.value, i, p.String(), tt.want[i])
				}
			}
		})
	}
}

func TestParseCloudflareProxies(t *testing.T) {
	const envName = "TEST_WS_CLOUDFLARE_IPS"
	// unset and the builtin spellings resolve to the embedded edge list
	for _, v := range []string{"", "builtin", "Default"} {
		got, err := parseCloudflareProxies(envName, v)
		if err != nil || len(got) == 0 {
			t.Fatalf("parseCloudflareProxies(%q) = %d prefixes, err %v; want the embedded list, nil", v, len(got), err)
		}
		// spot-check long-standing Cloudflare ranges from cloudflare_ips.txt
		if !prefixesContain(got, "104.16.0.0/13") || !prefixesContain(got, "2606:4700::/32") {
			t.Fatalf("parseCloudflareProxies(%q) is missing known Cloudflare ranges: %v", v, got)
		}
	}
	// a value starting with @ loads the list from a file: one CIDR per line,
	// commas also accepted, blank lines and #-comments ignored
	cidrFile := filepath.Join(t.TempDir(), "cf.txt")
	if err := os.WriteFile(cidrFile, []byte("# test ranges\n203.0.113.0/24\n\n2400:cb00::/32, 198.51.100.0/24 # trailing comment\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	fromFile, err := parseCloudflareProxies(envName, "@"+cidrFile)
	if err != nil || len(fromFile) != 3 || !prefixesContain(fromFile, "203.0.113.0/24") || !prefixesContain(fromFile, "198.51.100.0/24") {
		t.Fatalf("file list = %v, err %v; want the three prefixes from the file", fromFile, err)
	}
	// a missing file and a file with no CIDRs must fail startup rather than
	// silently disabling verification
	if _, err := parseCloudflareProxies(envName, "@"+filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Fatal("missing CIDR file: expected error, got nil")
	}
	if err := os.WriteFile(cidrFile, []byte("# only comments\n\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := parseCloudflareProxies(envName, "@"+cidrFile); err == nil {
		t.Fatal("CIDR file without CIDRs: expected error, got nil")
	}
	// the off spellings disable verification
	for _, v := range []string{"off", "none", "false", "0", "disabled", " OFF "} {
		got, err := parseCloudflareProxies(envName, v)
		if err != nil || got != nil {
			t.Fatalf("parseCloudflareProxies(%q) = %v, err %v; want nil, nil", v, got, err)
		}
	}
	// a custom list replaces the built-in ranges
	custom, err := parseCloudflareProxies(envName, "203.0.113.0/24, 2400:cb00::/32")
	if err != nil || len(custom) != 2 || custom[0] != netip.MustParsePrefix("203.0.113.0/24") {
		t.Fatalf("custom list = %v, err %v; want the two configured prefixes", custom, err)
	}
	// invalid CIDRs, IPv4-mapped notation, and a list with no CIDRs at all must
	// fail rather than silently disabling verification
	for _, v := range []string{"not-a-cidr", "::ffff:192.0.2.0/120", ", ,"} {
		if _, err := parseCloudflareProxies(envName, v); err == nil {
			t.Fatalf("parseCloudflareProxies(%q) expected error, got nil", v)
		}
	}
}

func TestResolveClientIPLegacyAndTrustedProxy(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		trusted    []netip.Prefix
		cloudflare []netip.Prefix
		pseudoIPv6 bool
		want       string
	}{
		{
			name: "cloudflare ip is preferred over spoofable ipv6 header by default",
			headers: map[string]string{
				"CF-Connecting-IPv6": "2001:db8::1",
				"CF-Connecting-IP":   "192.0.2.10",
			},
			remoteAddr: "198.51.100.1:12345",
			want:       "192.0.2.10",
		},
		{
			name: "cloudflare ipv6 preferred only with pseudo-ipv4 opt-in",
			headers: map[string]string{
				"CF-Connecting-IPv6": "2001:db8::1",
				"CF-Connecting-IP":   "192.0.2.10",
			},
			remoteAddr: "198.51.100.1:12345",
			pseudoIPv6: true,
			want:       "2001:db8::1",
		},
		{
			name: "cloudflare ip is canonicalized",
			headers: map[string]string{
				"CF-Connecting-IP": " 192.0.2.10 ",
			},
			remoteAddr: "198.51.100.1:12345",
			want:       "192.0.2.10",
		},
		{
			name: "invalid cloudflare ip falls back to remote address",
			headers: map[string]string{
				"CF-Connecting-IP": "not-an-ip",
				"X-Real-Ip":        "203.0.113.10",
			},
			remoteAddr: "198.51.100.1:12345",
			want:       "198.51.100.1",
		},
		{
			name:       "remote ipv6 address strips port",
			remoteAddr: "[2001:db8::2]:443",
			want:       "2001:db8::2",
		},
		{
			name:       "remote address without port is accepted",
			remoteAddr: "198.51.100.2",
			want:       "198.51.100.2",
		},
		{
			name: "x-real-ip honored when remote is loopback",
			headers: map[string]string{
				"X-Real-Ip": "203.0.113.10",
			},
			remoteAddr: "127.0.0.1:54321",
			want:       "203.0.113.10",
		},
		{
			name: "x-real-ip honored when remote is private network",
			headers: map[string]string{
				"X-Real-Ip": "203.0.113.11",
			},
			remoteAddr: "10.0.0.5:54321",
			want:       "203.0.113.11",
		},
		{
			name: "x-real-ip ignored when remote is public",
			headers: map[string]string{
				"X-Real-Ip": "203.0.113.12",
			},
			remoteAddr: "198.51.100.3:54321",
			want:       "198.51.100.3",
		},
		{
			name: "invalid x-real-ip from trusted proxy falls back to remote",
			headers: map[string]string{
				"X-Real-Ip": "not-an-ip",
			},
			remoteAddr: "127.0.0.1:54321",
			want:       "127.0.0.1",
		},
		{
			name: "x-real-ip honored when remote matches configured public CIDR",
			headers: map[string]string{
				"X-Real-Ip": "203.0.113.50",
			},
			remoteAddr: "198.51.100.5:54321",
			trusted:    []netip.Prefix{netip.MustParsePrefix("198.51.100.0/24")},
			want:       "203.0.113.50",
		},
		{
			name: "custom trusted proxy ignores spoofed cloudflare header",
			headers: map[string]string{
				"CF-Connecting-IP": "192.0.2.99",
				"X-Real-Ip":        "203.0.113.52",
			},
			remoteAddr: "198.51.100.5:54321",
			trusted:    []netip.Prefix{netip.MustParsePrefix("198.51.100.0/24")},
			want:       "203.0.113.52",
		},
		{
			name: "custom trusted proxy ignores cloudflare header without x-real-ip",
			headers: map[string]string{
				"CF-Connecting-IP": "192.0.2.100",
			},
			remoteAddr: "198.51.100.5:54321",
			trusted:    []netip.Prefix{netip.MustParsePrefix("198.51.100.0/24")},
			want:       "198.51.100.5",
		},
		{
			name: "x-real-ip ignored for public remote outside configured CIDRs",
			headers: map[string]string{
				"X-Real-Ip": "203.0.113.51",
			},
			remoteAddr: "198.51.100.6:54321",
			trusted:    []netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")},
			want:       "198.51.100.6",
		},
		{
			name: "link-local IPv6 peer is NOT implicitly trusted; X-Real-Ip ignored",
			headers: map[string]string{
				"X-Real-Ip": "203.0.113.60",
			},
			remoteAddr: "[fe80::1%eth0]:12345",
			want:       "fe80::1", // header ignored, falls back to the (zone-stripped) peer
		},
		{
			name: "link-local IPv6 peer trusted only when listed explicitly; zone stripped for matching",
			headers: map[string]string{
				"X-Real-Ip": "203.0.113.60",
			},
			remoteAddr: "[fe80::1%eth0]:12345",
			trusted:    []netip.Prefix{netip.MustParsePrefix("fe80::1/128")},
			want:       "203.0.113.60",
		},
		{
			name:       "link-local IPv6 zone identifier is stripped from returned address",
			remoteAddr: "[fe80::1%eth0]:12345",
			want:       "fe80::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{
				Header:     make(http.Header),
				RemoteAddr: tt.remoteAddr,
			}
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}

			got, _, _ := resolveClientIP(r, tt.trusted, tt.cloudflare, tt.pseudoIPv6)
			if got != tt.want {
				t.Fatalf("resolveClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

// mustParsePrefixes is a test helper that parses CIDR strings into prefixes.
func mustParsePrefixes(t *testing.T, cidrs ...string) []netip.Prefix {
	t.Helper()
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}

func TestResolveClientIPCloudflareVerification(t *testing.T) {
	cf := mustParsePrefixes(t, "203.0.113.0/24", "2400:cb00::/32")
	trusted := mustParsePrefixes(t, "198.51.100.0/24")
	tests := []struct {
		name          string
		headers       map[string]string
		remoteAddr    string
		trusted       []netip.Prefix
		cloudflare    []netip.Prefix
		pseudoIPv6    bool
		want          string
		wantBlockSafe bool
	}{
		{
			name:          "verified cloudflare peer: CF header trusted and block-safe",
			headers:       map[string]string{"CF-Connecting-IP": "192.0.2.10"},
			remoteAddr:    "203.0.113.5:443", // inside configured CF range
			cloudflare:    cf,
			want:          "192.0.2.10",
			wantBlockSafe: true,
		},
		{
			name: "spoofed CF-Connecting-IPv6 ignored by default; CF-Connecting-IP used",
			headers: map[string]string{
				"CF-Connecting-IPv6": "2001:db8:dead::1",
				"CF-Connecting-IP":   "192.0.2.10",
			},
			remoteAddr:    "203.0.113.5:443",
			cloudflare:    cf,
			want:          "192.0.2.10",
			wantBlockSafe: true,
		},
		{
			name: "pseudo-ipv4 opt-in: CF-Connecting-IPv6 preferred over synthetic CF-Connecting-IP",
			headers: map[string]string{
				"CF-Connecting-IPv6": "2001:db8:beef::1",
				"CF-Connecting-IP":   "192.0.2.10",
			},
			remoteAddr:    "203.0.113.5:443",
			cloudflare:    cf,
			pseudoIPv6:    true,
			want:          "2001:db8:beef::1",
			wantBlockSafe: true,
		},
		{
			name:          "verified peer, only spoofed CF-Connecting-IPv6, default: header ignored, peer not block-safe",
			headers:       map[string]string{"CF-Connecting-IPv6": "2001:db8:dead::1"},
			remoteAddr:    "203.0.113.5:443",
			cloudflare:    cf,
			want:          "203.0.113.5",
			wantBlockSafe: false, // an untrusted CF header was present: do not block the peer
		},
		{
			name: "pseudo-ipv4 opt-in falls back to CF-Connecting-IP when IPv6 header absent",
			headers: map[string]string{
				"CF-Connecting-IP": "192.0.2.10",
			},
			remoteAddr:    "203.0.113.5:443",
			cloudflare:    cf,
			pseudoIPv6:    true,
			want:          "192.0.2.10",
			wantBlockSafe: true,
		},
		{
			name:          "unverified public peer: CF header ignored, peer not block-safe (spoof guard)",
			headers:       map[string]string{"CF-Connecting-IP": "192.0.2.10"},
			remoteAddr:    "198.51.100.7:443", // NOT a CF range
			cloudflare:    cf,
			want:          "198.51.100.7",
			wantBlockSafe: false, // an untrusted CF header was present: do not block the peer
		},
		{
			name:          "loopback proxy fronting cloudflare: CF header trusted and block-safe",
			headers:       map[string]string{"CF-Connecting-IP": "192.0.2.20"},
			remoteAddr:    "127.0.0.1:5000",
			cloudflare:    cf,
			want:          "192.0.2.20",
			wantBlockSafe: true,
		},
		{
			name:          "verification disabled: CF header trusted for rate limit but NOT block-safe",
			headers:       map[string]string{"CF-Connecting-IP": "192.0.2.30"},
			remoteAddr:    "198.51.100.7:443",
			cloudflare:    nil,
			want:          "192.0.2.30",
			wantBlockSafe: false, // spoofable without peer verification
		},
		{
			name:          "native IPv6 in CF-Connecting-IP without IPv6 header",
			headers:       map[string]string{"CF-Connecting-IP": "2001:db8::99"},
			remoteAddr:    "203.0.113.5:443",
			cloudflare:    cf,
			want:          "2001:db8::99",
			wantBlockSafe: true,
		},
		{
			name: "malformed CF-Connecting-IPv6 falls through to CF-Connecting-IP",
			headers: map[string]string{
				"CF-Connecting-IPv6": "not-an-ip",
				"CF-Connecting-IP":   "192.0.2.40",
			},
			remoteAddr:    "203.0.113.5:443",
			cloudflare:    cf,
			want:          "192.0.2.40",
			wantBlockSafe: true,
		},
		{
			name:          "X-Real-Ip from trusted proxy is block-safe",
			headers:       map[string]string{"X-Real-Ip": "192.0.2.50"},
			remoteAddr:    "198.51.100.7:443", // inside configured trusted-proxy range
			trusted:       trusted,
			want:          "192.0.2.50",
			wantBlockSafe: true,
		},
		{
			name:          "direct public peer with no forwarding header is block-safe",
			remoteAddr:    "192.0.2.60:443",
			cloudflare:    cf,
			want:          "192.0.2.60",
			wantBlockSafe: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{Header: make(http.Header), RemoteAddr: tt.remoteAddr}
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			got, blockSafe, _ := resolveClientIP(r, tt.trusted, tt.cloudflare, tt.pseudoIPv6)
			if got != tt.want || blockSafe != tt.wantBlockSafe {
				t.Fatalf("resolveClientIP() = %q, %v, want %q, %v", got, blockSafe, tt.want, tt.wantBlockSafe)
			}
		})
	}
}

func TestResolveClientIPFromHeader(t *testing.T) {
	cf := mustParsePrefixes(t, "203.0.113.0/24")
	tests := []struct {
		name           string
		headers        map[string]string
		remoteAddr     string
		wantFromHeader bool
	}{
		{
			name:           "CF header honored from verified peer",
			headers:        map[string]string{"CF-Connecting-IP": "192.0.2.10"},
			remoteAddr:     "203.0.113.5:443",
			wantFromHeader: true,
		},
		{
			name:           "X-Real-Ip honored from loopback proxy",
			headers:        map[string]string{"X-Real-Ip": "192.0.2.11"},
			remoteAddr:     "127.0.0.1:5000",
			wantFromHeader: true,
		},
		{
			name:           "untrusted CF header falls back to bare peer",
			headers:        map[string]string{"CF-Connecting-IP": "192.0.2.10"},
			remoteAddr:     "198.51.100.7:443",
			wantFromHeader: false,
		},
		{
			name:           "bare loopback peer without headers",
			remoteAddr:     "127.0.0.1:5000",
			wantFromHeader: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &http.Request{Header: make(http.Header), RemoteAddr: tt.remoteAddr}
			for k, v := range tt.headers {
				r.Header.Set(k, v)
			}
			if _, _, fromHeader := resolveClientIP(r, nil, cf, false); fromHeader != tt.wantFromHeader {
				t.Fatalf("resolveClientIP() fromHeader = %v, want %v", fromHeader, tt.wantFromHeader)
			}
		})
	}
}

func TestRateLimitKey(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"192.0.2.10", "192.0.2.10"},
		{"::ffff:192.0.2.10", "192.0.2.10"}, // IPv4-mapped IPv6 unmaps to the IPv4 key
		{"2001:db8:1:2:3:4:5:6", "2001:db8:1:2::/64"},
		{"2001:db8:1:2::ffff", "2001:db8:1:2::/64"},
		{"2001:db8:1:3::1", "2001:db8:1:3::/64"},
		{"not-an-ip", "not-an-ip"},
	}
	for _, tt := range tests {
		if got := rateLimitKey(tt.in); got != tt.want {
			t.Fatalf("rateLimitKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	// addresses within the same /64 must share a key, distinct /64s must not
	if rateLimitKey("2001:db8:1:2::1") != rateLimitKey("2001:db8:1:2::2") {
		t.Fatal("addresses in the same /64 should share a rate-limit key")
	}
	if rateLimitKey("2001:db8:1:2::1") == rateLimitKey("2001:db8:1:3::1") {
		t.Fatal("addresses in different /64s should not share a rate-limit key")
	}
}

func TestBlockKey(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"192.0.2.10", "192.0.2.10"},                     // IPv4 verbatim (== rateLimitKey)
		{"::ffff:192.0.2.10", "192.0.2.10"},              // IPv4-mapped IPv6 unmaps to the IPv4 key
		{"2001:db8:1:2:3:4:5:6", "2001:db8:1:2:3:4:5:6"}, // IPv6 kept at the full /128
		{"2001:db8:1:2::ffff", "2001:db8:1:2::ffff"},
		{"[2001:db8:1:2::1%eth0]", "[2001:db8:1:2::1%eth0]"}, // brackets/zone are not a bare addr; verbatim
		{"2001:db8:1:2::1%eth0", "2001:db8:1:2::1"},          // zone stripped from a bare addr
		{"not-an-ip", "not-an-ip"},
	}
	for _, tt := range tests {
		if got := blockKey(tt.in); got != tt.want {
			t.Fatalf("blockKey(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	// The whole point of decoupling: two addresses in one /64 share a rate-limit
	// key (so the limiter aggregates) but get distinct block keys (so a block on
	// one does not take out the other).
	a, b := "2001:db8:1:2::1", "2001:db8:1:2::2"
	if rateLimitKey(a) != rateLimitKey(b) {
		t.Fatal("same /64 should share a rate-limit key")
	}
	if blockKey(a) == blockKey(b) {
		t.Fatal("distinct /128s in the same /64 must get distinct block keys")
	}
	// IPv4 block key must equal its rate-limit key (IPv4 behavior unchanged).
	if blockKey("192.0.2.10") != rateLimitKey("192.0.2.10") {
		t.Fatal("IPv4 block key should equal its rate-limit key")
	}
}

func TestIsBlockableKey(t *testing.T) {
	cf := mustParsePrefixes(t, "203.0.113.0/24")
	trusted := mustParsePrefixes(t, "198.51.100.0/24")
	tests := []struct {
		ip   string
		want bool
	}{
		{"192.0.2.10", true},          // ordinary public address
		{"2001:db8:1:2::5", true},     // ordinary public IPv6 address
		{"127.0.0.1", false},          // loopback
		{"10.1.2.3", false},           // RFC1918
		{"192.168.1.1", false},        // RFC1918
		{"169.254.0.1", false},        // link-local
		{"203.0.113.9", false},        // inside Cloudflare range
		{"198.51.100.9", false},       // inside trusted-proxy range
		{"::ffff:10.1.2.3", false},    // IPv4-mapped private unmaps before the checks
		{"::ffff:203.0.113.9", false}, // IPv4-mapped form of a Cloudflare-range address
		{"not-an-ip", false},          // unparseable
	}
	for _, tt := range tests {
		if got := isBlockableKey(tt.ip, trusted, cf); got != tt.want {
			t.Fatalf("isBlockableKey(%q) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestConnMessageRate(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	m := newConnMessageRate(10 * time.Minute)

	// 100 messages in the same instant accumulate.
	var last int
	for i := 0; i < 100; i++ {
		last = m.observe(base)
	}
	if last != 100 {
		t.Fatalf("after 100 messages at one instant, count = %d, want 100", last)
	}

	// Messages spread across the window keep accumulating (sliding, not reset).
	m2 := newConnMessageRate(10 * time.Minute)
	count := 0
	for i := 0; i < 60; i++ {
		count = m2.observe(base.Add(time.Duration(i) * 5 * time.Second)) // 0..295s
	}
	if count != 60 {
		t.Fatalf("60 messages within the window, count = %d, want 60", count)
	}

	// Once the full window has elapsed since the first message, old buckets drop
	// out of the trailing window.
	after := m2.observe(base.Add(10*time.Minute + time.Second))
	if after >= 60 {
		t.Fatalf("after the window elapsed the count should drop, got %d", after)
	}

	// A gap longer than the whole window resets the counter.
	reset := m2.observe(base.Add(time.Hour))
	if reset != 1 {
		t.Fatalf("after a gap longer than the window, count = %d, want 1", reset)
	}
}

func TestConnMessageRateClockSkew(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	m := newConnMessageRate(10 * time.Minute)
	m.observe(base.Add(time.Minute))
	// A backwards clock jump must not panic or rewrite history; it folds into the
	// current bucket.
	if got := m.observe(base); got != 2 {
		t.Fatalf("backwards clock observe = %d, want 2", got)
	}
}

func TestWebsocketConnectionLimiterConnectionAttempts(t *testing.T) {
	limiter := newWebsocketConnectionLimiter()
	now := time.Unix(1700000000, 0)
	ip := "192.0.2.10"

	for i := 0; i < maxWebsocketConnectionAttemptsPerIP; i++ {
		ok, reason := limiter.accept(ip, now)
		if !ok {
			t.Fatalf("accept(%d) rejected with %q", i, reason)
		}
		limiter.release(ip, now)
	}

	ok, reason := limiter.accept(ip, now)
	if ok || reason != "connection_attempt_limit" {
		t.Fatalf("accept() = %v, %q, want false, connection_attempt_limit", ok, reason)
	}

	ok, reason = limiter.accept(ip, now.Add(websocketConnectionAttemptWindow+time.Second))
	if !ok {
		t.Fatalf("accept() after window rejected with %q", reason)
	}
}

func TestWebsocketConnectionLimiterActiveConnections(t *testing.T) {
	limiter := newWebsocketConnectionLimiter()
	now := time.Unix(1700000000, 0)
	ip := "192.0.2.20"

	for i := 0; i < maxWebsocketConnectionsPerIP; i++ {
		if i > 0 && i%maxWebsocketConnectionAttemptsPerIP == 0 {
			now = now.Add(websocketConnectionAttemptWindow + time.Second)
		}
		ok, reason := limiter.accept(ip, now)
		if !ok {
			t.Fatalf("accept(%d) rejected with %q", i, reason)
		}
	}

	ok, reason := limiter.accept(ip, now)
	if ok || reason != "connection_limit" {
		t.Fatalf("accept() = %v, %q, want false, connection_limit", ok, reason)
	}

	limiter.release(ip, now)
	ok, reason = limiter.accept(ip, now.Add(websocketConnectionAttemptWindow+time.Second))
	if !ok {
		t.Fatalf("accept() after release rejected with %q", reason)
	}
}

func TestWebsocketConnectionLimiterSweepEvictsIdleEntries(t *testing.T) {
	limiter := newWebsocketConnectionLimiter()
	now := time.Unix(1700000000, 0)
	idle := "192.0.2.40"
	active := "192.0.2.41"

	if ok, reason := limiter.accept(idle, now); !ok {
		t.Fatalf("accept(idle) rejected with %q", reason)
	}
	limiter.release(idle, now)
	if ok, reason := limiter.accept(active, now); !ok {
		t.Fatalf("accept(active) rejected with %q", reason)
	}

	// sweep() is what the periodic-cleanup goroutine calls; verify it evicts
	// TTL-expired idle entries while keeping entries with active connections.
	limiter.sweep(now.Add(websocketConnectionLimiterTTL + time.Second))

	limiter.mux.Lock()
	_, idleStillTracked := limiter.clients[idle]
	_, activeStillTracked := limiter.clients[active]
	limiter.mux.Unlock()
	if idleStillTracked {
		t.Fatal("idle TTL-expired entry was not evicted by sweep")
	}
	if !activeStillTracked {
		t.Fatal("entry with active connection was evicted by sweep")
	}
}

func TestWebsocketConnectionLimiterStats(t *testing.T) {
	limiter := newWebsocketConnectionLimiter()
	now := time.Unix(1700000000, 0)
	a := "192.0.2.50"
	b := "192.0.2.51"

	if unique, max := limiter.stats(); unique != 0 || max != 0 {
		t.Fatalf("stats() on empty limiter = %d, %d, want 0, 0", unique, max)
	}

	for i := 0; i < 3; i++ {
		if ok, reason := limiter.accept(a, now); !ok {
			t.Fatalf("accept(a, %d) rejected with %q", i, reason)
		}
	}
	if ok, reason := limiter.accept(b, now); !ok {
		t.Fatalf("accept(b) rejected with %q", reason)
	}

	// two distinct IPs hold connections; the busiest holds three
	if unique, max := limiter.stats(); unique != 2 || max != 3 {
		t.Fatalf("stats() = %d, %d, want 2, 3", unique, max)
	}

	// releasing every connection from a leaves an idle (active==0) entry that
	// is still tracked for the TTL window; stats must not count it as a cluster
	for i := 0; i < 3; i++ {
		limiter.release(a, now)
	}
	limiter.mux.Lock()
	_, aStillTracked := limiter.clients[a]
	limiter.mux.Unlock()
	if !aStillTracked {
		t.Fatal("released entry should remain tracked within the TTL window")
	}
	if unique, max := limiter.stats(); unique != 1 || max != 1 {
		t.Fatalf("stats() after releasing idle IP = %d, %d, want 1, 1", unique, max)
	}
}

func TestWebsocketConnectionLimiterCleanup(t *testing.T) {
	limiter := newWebsocketConnectionLimiter()
	now := time.Unix(1700000000, 0)
	ip := "192.0.2.30"

	ok, reason := limiter.accept(ip, now)
	if !ok {
		t.Fatalf("accept() rejected with %q", reason)
	}
	limiter.release(ip, now)

	_, _ = limiter.accept("192.0.2.31", now.Add(websocketConnectionLimiterTTL+websocketConnectionLimiterCleanupInterval+time.Second))
	if _, ok := limiter.clients[ip]; ok {
		t.Fatal("idle client limit entry was not cleaned up")
	}
}

func TestEstimateFeeRejectsTooManyBlocks(t *testing.T) {
	blocks := make([]int, maxWebsocketEstimateFeeBlocks+1)
	params, err := json.Marshal(WsEstimateFeeReq{Blocks: blocks})
	if err != nil {
		t.Fatal(err)
	}

	s := &WebsocketServer{}
	_, err = s.estimateFee(params)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*api.APIError)
	if !ok {
		t.Fatalf("expected *api.APIError, got %T", err)
	}
	if !apiErr.Public {
		t.Fatal("expected public api error")
	}
	if !strings.Contains(apiErr.Error(), "blocks max 32") {
		t.Fatalf("unexpected error message %q", apiErr.Error())
	}
}

func TestUnmarshalAddressesRejectsTooManyAddresses(t *testing.T) {
	addresses := make([]string, maxWebsocketSubscribeAddresses+1)
	params, err := json.Marshal(WsSubscribeAddressesReq{Addresses: addresses})
	if err != nil {
		t.Fatal(err)
	}

	s := &WebsocketServer{}
	_, _, err = s.unmarshalAddresses(params)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*api.APIError)
	if !ok {
		t.Fatalf("expected *api.APIError, got %T", err)
	}
	if !apiErr.Public {
		t.Fatal("expected public api error")
	}
	if !strings.Contains(apiErr.Error(), "addresses max 1000") {
		t.Fatalf("unexpected error message %q", apiErr.Error())
	}
}

func TestUnmarshalAddressesRejectsTooManyNewBlockTxAddresses(t *testing.T) {
	addresses := make([]string, maxWebsocketSubscribeAddressesWithNewBlockTxs+1)
	params, err := json.Marshal(WsSubscribeAddressesReq{
		Addresses:   addresses,
		NewBlockTxs: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	s := &WebsocketServer{}
	_, _, err = s.unmarshalAddresses(params)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*api.APIError)
	if !ok {
		t.Fatalf("expected *api.APIError, got %T", err)
	}
	if !apiErr.Public {
		t.Fatal("expected public api error")
	}
	if !strings.Contains(apiErr.Error(), "addresses max 100") {
		t.Fatalf("unexpected error message %q", apiErr.Error())
	}
}

func TestUnmarshalAddressesDeduplicatesDescriptors(t *testing.T) {
	parser, _ := setupChain(t)
	params, err := json.Marshal(WsSubscribeAddressesReq{
		Addresses: []string{dbtestdata.Addr1, dbtestdata.Addr1},
	})
	if err != nil {
		t.Fatal(err)
	}

	s := &WebsocketServer{chainParser: parser}
	addresses, newBlockTxs, err := s.unmarshalAddresses(params)
	if err != nil {
		t.Fatal(err)
	}
	if newBlockTxs {
		t.Fatal("newBlockTxs = true, want false")
	}
	if len(addresses) != 1 {
		t.Fatalf("len(addresses) = %d, want 1", len(addresses))
	}
}

func TestSetConfirmedBlockTxMetadataSetsConfirmedFields(t *testing.T) {
	tx := bchain.Tx{
		Confirmations: 0,
		Blocktime:     0,
		Time:          0,
	}

	setConfirmedBlockTxMetadata(&tx, 123456)

	if tx.Confirmations != 1 {
		t.Fatalf("Confirmations = %d, want 1", tx.Confirmations)
	}
	if tx.Blocktime != 123456 {
		t.Fatalf("Blocktime = %d, want 123456", tx.Blocktime)
	}
	if tx.Time != 123456 {
		t.Fatalf("Time = %d, want 123456", tx.Time)
	}
}

func TestUnmarshalAddressesReturnsPublicAPIError(t *testing.T) {
	s := &WebsocketServer{
		chainParser: eth.NewEthereumParser(0, false),
	}

	_, _, err := s.unmarshalAddresses([]byte(`{"addresses":[""]}`))
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*api.APIError)
	if !ok {
		t.Fatalf("expected *api.APIError, got %T", err)
	}
	if !apiErr.Public {
		t.Fatal("expected public api error")
	}
	if !strings.Contains(apiErr.Error(), "Address missing") {
		t.Fatalf("unexpected error message %q", apiErr.Error())
	}
}

func TestSetConfirmedBlockTxMetadataLeavesConfirmedTxUnchanged(t *testing.T) {
	tx := bchain.Tx{
		Confirmations: 3,
		Blocktime:     100,
		Time:          200,
	}

	setConfirmedBlockTxMetadata(&tx, 123456)

	if tx.Confirmations != 3 {
		t.Fatalf("Confirmations = %d, want 3", tx.Confirmations)
	}
	if tx.Blocktime != 100 {
		t.Fatalf("Blocktime = %d, want 100", tx.Blocktime)
	}
	if tx.Time != 200 {
		t.Fatalf("Time = %d, want 200", tx.Time)
	}
}

func TestGetEthereumInternalTransfersMissingData(t *testing.T) {
	tx := bchain.Tx{}

	transfers := getEthereumInternalTransfers(&tx)

	if len(transfers) != 0 {
		t.Fatalf("len(transfers) = %d, want 0", len(transfers))
	}
}

func TestGetEthereumInternalTransfersReturnsTransfers(t *testing.T) {
	expected := []bchain.EthereumInternalTransfer{
		{From: "0x111", To: "0x222"},
	}
	tx := bchain.Tx{
		CoinSpecificData: bchain.EthereumSpecificData{
			InternalData: &bchain.EthereumInternalData{
				Transfers: expected,
			},
		},
	}

	transfers := getEthereumInternalTransfers(&tx)

	if len(transfers) != len(expected) {
		t.Fatalf("len(transfers) = %d, want %d", len(transfers), len(expected))
	}
	if transfers[0].From != expected[0].From || transfers[0].To != expected[0].To {
		t.Fatalf("transfers[0] = %+v, want %+v", transfers[0], expected[0])
	}
}

func TestSetEthereumReceiptIfAvailableKeepsTxWhenReceiptFails(t *testing.T) {
	tx := bchain.Tx{
		Txid: "0xabc",
		CoinSpecificData: bchain.EthereumSpecificData{
			Tx: &bchain.RpcTransaction{Hash: "0xabc"},
		},
	}

	setEthereumReceiptIfAvailable(&tx, func(string) (*bchain.RpcReceipt, error) {
		return nil, errors.New("rpc failure")
	})

	csd, ok := tx.CoinSpecificData.(bchain.EthereumSpecificData)
	if !ok {
		t.Fatal("CoinSpecificData has unexpected type")
	}
	if csd.Receipt != nil {
		t.Fatalf("Receipt = %+v, want nil", csd.Receipt)
	}
}

func TestSetEthereumReceiptIfAvailableSetsReceipt(t *testing.T) {
	tx := bchain.Tx{
		Txid: "0xdef",
		CoinSpecificData: bchain.EthereumSpecificData{
			Tx: &bchain.RpcTransaction{Hash: "0xdef"},
		},
	}
	wantReceipt := &bchain.RpcReceipt{GasUsed: "0x5208"}

	setEthereumReceiptIfAvailable(&tx, func(string) (*bchain.RpcReceipt, error) {
		return wantReceipt, nil
	})

	csd, ok := tx.CoinSpecificData.(bchain.EthereumSpecificData)
	if !ok {
		t.Fatal("CoinSpecificData has unexpected type")
	}
	if csd.Receipt != wantReceipt {
		t.Fatalf("Receipt = %+v, want %+v", csd.Receipt, wantReceipt)
	}
}

func TestSendOnNewTxAddrFiltersNewBlockTxSubscriptions(t *testing.T) {
	parser, _ := setupChain(t)
	s := &WebsocketServer{
		chainParser:          parser,
		addressSubscriptions: make(map[string]map[*websocketChannel]*addressDetails),
	}
	addrDesc, err := parser.GetAddrDescFromAddress(dbtestdata.Addr1)
	if err != nil {
		t.Fatal(err)
	}
	stringAddrDesc := string(addrDesc)
	onlyMempool := &websocketChannel{out: make(chan *WsRes, 1), alive: true}
	withNewBlockTxs := &websocketChannel{out: make(chan *WsRes, 1), alive: true}
	s.addressSubscriptions[stringAddrDesc] = map[*websocketChannel]*addressDetails{
		onlyMempool: {
			requestID:          "mempool-only",
			publishNewBlockTxs: false,
		},
		withNewBlockTxs: {
			requestID:          "with-new-block-txs",
			publishNewBlockTxs: true,
		},
	}

	s.sendOnNewTxAddr(stringAddrDesc, &api.Tx{Txid: "new-block-tx"}, true)

	if len(onlyMempool.out) != 0 {
		t.Fatalf("mempool-only subscriber received %d messages, want 0", len(onlyMempool.out))
	}
	if len(withNewBlockTxs.out) != 1 {
		t.Fatalf("newBlockTxs subscriber received %d messages, want 1", len(withNewBlockTxs.out))
	}
}

func TestPopulateBitcoinVinAddrDescsEnablesSenderOnlyMatching(t *testing.T) {
	parser, _ := setupChain(t)
	block := dbtestdata.GetTestBitcoinTypeBlock2(parser)
	tx := block.Txs[0] // spends Addr3/Addr2 and pays Addr6/Addr7

	vins := make([]bchain.MempoolVin, len(tx.Vin))
	for i := range tx.Vin {
		vins[i] = bchain.MempoolVin{Vin: tx.Vin[i]}
	}
	addr3Desc, err := parser.GetAddrDescFromAddress(dbtestdata.Addr3)
	if err != nil {
		t.Fatal(err)
	}
	addr2Desc, err := parser.GetAddrDescFromAddress(dbtestdata.Addr2)
	if err != nil {
		t.Fatal(err)
	}
	dummy := &websocketChannel{}
	s := &WebsocketServer{
		chainParser: parser,
		addressSubscriptions: map[string]map[*websocketChannel]*addressDetails{
			string(addr3Desc): {dummy: {requestID: "sender", publishNewBlockTxs: true}},
		},
	}

	withoutResolvedVins := s.getNewTxSubscriptions(vins, tx.Vout, nil, nil, true)
	if _, ok := withoutResolvedVins[string(addr3Desc)]; ok {
		t.Fatal("sender subscription unexpectedly matched before vin descriptor resolution")
	}

	populateBitcoinVinAddrDescs(vins, func(txid string, vout uint32) (bchain.AddressDescriptor, error) {
		switch {
		case txid == dbtestdata.TxidB1T2 && vout == 0:
			return addr3Desc, nil
		case txid == dbtestdata.TxidB1T1 && vout == 1:
			return addr2Desc, nil
		default:
			return nil, errors.New("not found")
		}
	})

	withResolvedVins := s.getNewTxSubscriptions(vins, tx.Vout, nil, nil, true)
	if _, ok := withResolvedVins[string(addr3Desc)]; !ok {
		t.Fatal("sender subscription did not match after vin descriptor resolution")
	}
}

func TestGetNewTxSubscriptionsFiltersMempoolOnlyForNewBlockTxs(t *testing.T) {
	parser, _ := setupChain(t)
	addrDesc, err := parser.GetAddrDescFromAddress(dbtestdata.Addr1)
	if err != nil {
		t.Fatal(err)
	}
	stringAddrDesc := string(addrDesc)
	dummy := &websocketChannel{}
	s := &WebsocketServer{
		addressSubscriptions: map[string]map[*websocketChannel]*addressDetails{
			stringAddrDesc: {dummy: {requestID: "mempool-only", publishNewBlockTxs: false}},
		},
	}
	vins := []bchain.MempoolVin{{AddrDesc: addrDesc}}

	mempoolSubscribed := s.getNewTxSubscriptions(vins, nil, nil, nil, false)
	if _, ok := mempoolSubscribed[stringAddrDesc]; !ok {
		t.Fatal("mempool notification did not match mempool-only subscriber")
	}
	newBlockSubscribed := s.getNewTxSubscriptions(vins, nil, nil, nil, true)
	if _, ok := newBlockSubscribed[stringAddrDesc]; ok {
		t.Fatal("newBlockTxs matching included mempool-only subscriber")
	}

	s.addressSubscriptions[stringAddrDesc][dummy].publishNewBlockTxs = true
	newBlockSubscribed = s.getNewTxSubscriptions(vins, nil, nil, nil, true)
	if _, ok := newBlockSubscribed[stringAddrDesc]; !ok {
		t.Fatal("newBlockTxs matching did not include newBlockTxs subscriber")
	}
}

func newShutdownTestServer() *WebsocketServer {
	return &WebsocketServer{activeChannels: make(map[*websocketChannel]struct{})}
}

func TestWebsocketShutdownWaitsForInFlightWork(t *testing.T) {
	s := newShutdownTestServer()
	if ok, reason := s.trackWork(); !ok {
		t.Fatalf("trackWork() returned false before shutdown, reason %q", reason)
	}

	finished := make(chan struct{})
	go func() {
		// Simulate a DB-touching goroutine that takes some time.
		time.Sleep(50 * time.Millisecond)
		s.workDone()
		close(finished)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() = %v, want nil", err)
	}
	elapsed := time.Since(start)
	if elapsed < 50*time.Millisecond {
		t.Fatalf("Shutdown returned in %v, expected to wait for in-flight work (~50ms)", elapsed)
	}
	select {
	case <-finished:
	default:
		t.Fatal("Shutdown returned before tracked goroutine finished")
	}
}

func TestWebsocketShutdownTimesOutOnStuckWork(t *testing.T) {
	s := newShutdownTestServer()
	if ok, reason := s.trackWork(); !ok {
		t.Fatalf("trackWork() returned false before shutdown, reason %q", reason)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	finished := make(chan error)
	go func() {
		finished <- s.Shutdown(ctx)
	}()

	time.Sleep(60 * time.Millisecond)
	select {
	case err := <-finished:
		t.Fatalf("Shutdown returned before tracked work finished: %v", err)
	default:
	}
	s.workDone()
	if err := <-finished; err == nil {
		t.Fatal("Shutdown() = nil, want context deadline error")
	}
	if elapsed := time.Since(start); elapsed < 60*time.Millisecond {
		t.Fatalf("Shutdown returned in %v, expected to wait for tracked work after timeout", elapsed)
	}
}

func TestWebsocketShutdownRefusesNewWork(t *testing.T) {
	s := newShutdownTestServer()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() = %v, want nil", err)
	}
	if ok, reason := s.trackWork(); ok || reason != "server_shutdown" {
		t.Fatalf("trackWork() = %v, %q after shutdown, want false, server_shutdown", ok, reason)
	}
	dummy := &websocketChannel{}
	if s.registerChannel(dummy) {
		t.Fatal("registerChannel() returned true after shutdown")
	}
}

func TestWebsocketTrackWorkAppliesGlobalLimit(t *testing.T) {
	s := newShutdownTestServer()
	s.activeRequests = maxWebsocketActiveRequests
	if ok, reason := s.trackWork(); ok || reason != "work_limit" {
		t.Fatalf("trackWork() = %v, %q at global limit, want false, work_limit", ok, reason)
	}

	s.activeRequests = 0
	if ok, reason := s.trackWork(); !ok || reason != "" {
		t.Fatalf("trackWork() = %v, %q below global limit, want true, empty reason", ok, reason)
	}
	s.workDone()
	if s.activeRequests != 0 {
		t.Fatalf("activeRequests = %d after workDone, want 0", s.activeRequests)
	}
}

func TestWebsocketMempoolFilterResponseSlots(t *testing.T) {
	c := &websocketChannel{
		mempoolFiltersSlots: make(chan struct{}, maxWebsocketMempoolFiltersResponses),
	}
	for i := 0; i < maxWebsocketMempoolFiltersResponses; i++ {
		if !c.acquireMempoolFiltersSlot() {
			t.Fatalf("acquireMempoolFiltersSlot() = false at slot %d", i)
		}
	}
	if c.acquireMempoolFiltersSlot() {
		t.Fatal("acquireMempoolFiltersSlot() = true at limit")
	}
	c.releaseMempoolFiltersSlot()
	if !c.acquireMempoolFiltersSlot() {
		t.Fatal("acquireMempoolFiltersSlot() = false after release")
	}
}

func TestWebsocketCloseOutReleasesQueuedMempoolFilterResponses(t *testing.T) {
	c := &websocketChannel{
		out:                 make(chan *WsRes, outChannelSize),
		mempoolFiltersSlots: make(chan struct{}, maxWebsocketMempoolFiltersResponses),
		alive:               true,
	}
	if !c.acquireMempoolFiltersSlot() {
		t.Fatal("acquireMempoolFiltersSlot() = false")
	}
	c.DataOut(&WsRes{ID: "mempool", Data: struct{}{}, release: c.releaseMempoolFiltersSlot})
	if got := len(c.mempoolFiltersSlots); got != 1 {
		t.Fatalf("held mempool filter slots = %d before CloseOut, want 1", got)
	}
	if closed, reason := c.CloseOut("test"); !closed || reason != "test" {
		t.Fatalf("CloseOut() = %v, %q, want true, test", closed, reason)
	}
	if got := len(c.mempoolFiltersSlots); got != 0 {
		t.Fatalf("held mempool filter slots = %d after CloseOut, want 0", got)
	}
}

// TestWebsocketOutputLoopReleasesMempoolFilterSlotAfterWrite exercises the
// primary slot-release path for a live connection: outputLoop's c.finalize(m)
// after a successful WriteJSON. Neither TestWebsocketMempoolFilterResponseSlots
// (raw semaphore) nor TestWebsocketCloseOutReleasesQueuedMempoolFilterResponses
// (drain-on-close) touch this line, so without this test a removal of the
// finalize call would leak a slot on every successful response and go undetected.
func TestWebsocketOutputLoopReleasesMempoolFilterSlotAfterWrite(t *testing.T) {
	// Stand up a real websocket connection pair so outputLoop's WriteJSON and
	// finalize run against a live *websocket.Conn.
	upgrader := websocket.Upgrader{}
	serverConnCh := make(chan *websocket.Conn, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		serverConnCh <- conn
	}))
	defer ts.Close()

	clientConn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(ts.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer clientConn.Close()
	serverConn := <-serverConnCh
	defer serverConn.Close()

	c := &websocketChannel{
		conn:                serverConn,
		out:                 make(chan *WsRes, outChannelSize),
		mempoolFiltersSlots: make(chan struct{}, maxWebsocketMempoolFiltersResponses),
		alive:               true,
	}
	if !c.acquireMempoolFiltersSlot() {
		t.Fatal("acquireMempoolFiltersSlot() = false")
	}
	c.out <- &WsRes{ID: "mempool", Data: struct{}{}, release: c.releaseMempoolFiltersSlot}
	close(c.out) // outputLoop returns once the single queued response is written

	s := &WebsocketServer{}
	done := make(chan struct{})
	go func() {
		s.outputLoop(c)
		close(done)
	}()

	if _, _, err := clientConn.ReadMessage(); err != nil {
		t.Fatalf("client read: %v", err)
	}
	<-done

	if got := len(c.mempoolFiltersSlots); got != 0 {
		t.Fatalf("held mempool filter slots = %d after successful write, want 0", got)
	}
}

func TestWebsocketShutdownIsIdempotent(t *testing.T) {
	s := newShutdownTestServer()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("first Shutdown() = %v, want nil", err)
	}
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown() = %v, want nil", err)
	}
}
