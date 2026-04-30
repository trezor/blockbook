//go:build unittest
// +build unittest

package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/trezor/blockbook/api"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
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

func TestGetIP(t *testing.T) {
	tests := []struct {
		name       string
		headers    map[string]string
		remoteAddr string
		trusted    []netip.Prefix
		want       string
	}{
		{
			name: "cloudflare ipv6 is preferred",
			headers: map[string]string{
				"CF-Connecting-IPv6": "2001:db8::1",
				"CF-Connecting-IP":   "192.0.2.10",
			},
			remoteAddr: "198.51.100.1:12345",
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
			name: "link-local IPv6 peer with zone is trusted and zone is stripped from key",
			headers: map[string]string{
				"X-Real-Ip": "203.0.113.60",
			},
			remoteAddr: "[fe80::1%eth0]:12345",
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

			got := getIP(r, tt.trusted)
			if got != tt.want {
				t.Fatalf("getIP() = %q, want %q", got, tt.want)
			}
		})
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

	withoutResolvedVins := s.getNewTxSubscriptions(vins, tx.Vout, nil, nil)
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

	withResolvedVins := s.getNewTxSubscriptions(vins, tx.Vout, nil, nil)
	if _, ok := withResolvedVins[string(addr3Desc)]; !ok {
		t.Fatal("sender subscription did not match after vin descriptor resolution")
	}
}

func newShutdownTestServer() *WebsocketServer {
	return &WebsocketServer{activeChannels: make(map[*websocketChannel]struct{})}
}

func TestWebsocketShutdownWaitsForInFlightWork(t *testing.T) {
	s := newShutdownTestServer()
	if !s.trackWork() {
		t.Fatal("trackWork() returned false before shutdown")
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
	if !s.trackWork() {
		t.Fatal("trackWork() returned false before shutdown")
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
	if s.trackWork() {
		t.Fatal("trackWork() returned true after shutdown")
	}
	dummy := &websocketChannel{}
	if s.registerChannel(dummy) {
		t.Fatal("registerChannel() returned true after shutdown")
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
