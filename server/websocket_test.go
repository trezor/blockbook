//go:build unittest
// +build unittest

package server

import (
	"errors"
	"net/http"
	"strings"
	"testing"

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
