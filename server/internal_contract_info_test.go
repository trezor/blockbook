//go:build unittest

package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

// newInternalTestServer builds a full internal server (including the api.Worker
// the contract-info GET path needs) on top of the public-server test harness.
// The caller must have set BB_ADMIN_USER/BB_ADMIN_PASSWORD beforehand — the
// credentials are read in NewInternalServer.
func newInternalTestServer(t *testing.T, s *PublicServer) *httptest.Server {
	t.Helper()
	internal, err := NewInternalServer("localhost:12346", "", s.db, s.chain, s.mempool, s.txCache, metrics, s.is, s.fiatRates)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(internal.https.Handler)
	t.Cleanup(ts.Close)
	return ts
}

func adminRequest(t *testing.T, ts *httptest.Server, method, path, body string) (int, string) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, ts.URL+path, rdr)
	if err != nil {
		t.Fatal(err)
	}
	req.SetBasicAuth("admin", "password")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, strings.TrimSpace(string(b))
}

// TestContractInfoAdminAPI exercises the /admin/contract-info/ JSON API
// end-to-end: GET of one contract, bulk POST on the collection path (the
// response must decode as a JSON object — a regression check for the formerly
// double-encoded string body), DELETE with idempotent semantics, and the
// request-validation failure paths.
func TestContractInfoAdminAPI(t *testing.T) {
	t.Setenv("BB_ADMIN_USER", "admin")
	t.Setenv("BB_ADMIN_PASSWORD", "password")
	parser := eth.NewEthereumParser(1, true)
	chain, err := dbtestdata.NewFakeBlockChainEthereumType(parser)
	if err != nil {
		glog.Fatal("fakechain: ", err)
	}
	s, dbpath := setupPublicHTTPServer(parser, chain, t, false)
	defer closeAndDestroyPublicServer(t, s, dbpath)
	ts := newInternalTestServer(t, s)

	address := "0x" + dbtestdata.EthAddr20

	t.Run("GET collection lists stored contracts", func(t *testing.T) {
		code, body := adminRequest(t, ts, http.MethodGet, "/admin/contract-info/", "")
		if code != http.StatusOK {
			t.Fatalf("GET = %d %s, want 200", code, body)
		}
		var resp contractInfoListResponse
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("GET body %q does not decode: %v", body, err)
		}
	})

	t.Run("GET collection with invalid limit", func(t *testing.T) {
		for _, q := range []string{"limit=0", "limit=-1", "limit=abc", "limit=10001"} {
			code, body := adminRequest(t, ts, http.MethodGet, "/admin/contract-info/?"+q, "")
			if code != http.StatusBadRequest {
				t.Fatalf("%s: GET = %d %s, want 400", q, code, body)
			}
		}
	})

	t.Run("GET address", func(t *testing.T) {
		code, body := adminRequest(t, ts, http.MethodGet, "/admin/contract-info/"+address, "")
		if code != http.StatusOK {
			t.Fatalf("GET = %d %s, want 200", code, body)
		}
		var ci bchain.ContractInfo
		if err := json.Unmarshal([]byte(body), &ci); err != nil {
			t.Fatalf("GET body %q does not decode to ContractInfo: %v", body, err)
		}
		if ci.Contract == "" || ci.Name == "" || ci.Standard != bchain.ERC20TokenStandard {
			t.Fatalf("GET returned unexpected contract info: %+v", ci)
		}
	})

	t.Run("POST collection", func(t *testing.T) {
		code, body := adminRequest(t, ts, http.MethodPost, "/admin/contract-info/",
			`[{"contract":"`+address+`","name":"Renamed","symbol":"RNM","standard":"ERC20","decimals":18}]`)
		if code != http.StatusOK {
			t.Fatalf("POST = %d %s, want 200", code, body)
		}
		// The old handler returned a pre-serialized string that jsonHandler
		// encoded again; decoding into a typed object fails on that shape.
		var resp contractInfoUpdateResponse
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("POST body %q does not decode to an object: %v", body, err)
		}
		if resp.Updated != 1 {
			t.Fatalf("POST updated = %d, want 1", resp.Updated)
		}
		stored, err := s.db.GetContractInfoForAddress(address)
		if err != nil {
			t.Fatal(err)
		}
		if stored == nil || stored.Name != "Renamed" {
			t.Fatalf("stored contract = %+v, want name Renamed", stored)
		}
	})

	t.Run("GET collection contains the updated contract", func(t *testing.T) {
		code, body := adminRequest(t, ts, http.MethodGet, "/admin/contract-info/?limit=10", "")
		if code != http.StatusOK {
			t.Fatalf("GET = %d %s, want 200", code, body)
		}
		var resp contractInfoListResponse
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("GET body %q does not decode: %v", body, err)
		}
		found := false
		for _, c := range resp.Contracts {
			if c.Name == "Renamed" {
				found = true
			}
		}
		if !found {
			t.Fatalf("list %+v does not contain the contract stored by POST", resp.Contracts)
		}
	})

	t.Run("POST with address segment", func(t *testing.T) {
		code, body := adminRequest(t, ts, http.MethodPost, "/admin/contract-info/"+address, `[]`)
		if code != http.StatusBadRequest || !strings.Contains(body, "collection") {
			t.Fatalf("POST = %d %s, want 400 pointing to the collection path", code, body)
		}
	})

	t.Run("POST non-array body", func(t *testing.T) {
		code, body := adminRequest(t, ts, http.MethodPost, "/admin/contract-info/", `{"contract":"x"}`)
		if code != http.StatusBadRequest {
			t.Fatalf("POST = %d %s, want 400", code, body)
		}
	})

	t.Run("unsupported method", func(t *testing.T) {
		code, body := adminRequest(t, ts, http.MethodPatch, "/admin/contract-info/"+address, "")
		if code != http.StatusBadRequest || !strings.Contains(body, "Unsupported method") {
			t.Fatalf("PATCH = %d %s, want 400 Unsupported method", code, body)
		}
	})

	t.Run("DELETE", func(t *testing.T) {
		// stored by the POST subtest above
		code, body := adminRequest(t, ts, http.MethodDelete, "/admin/contract-info/"+address, "")
		if code != http.StatusOK {
			t.Fatalf("DELETE = %d %s, want 200", code, body)
		}
		var resp contractInfoDeleteResponse
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("DELETE body %q does not decode: %v", body, err)
		}
		// the purged record is returned so the operator can restore it (incl.
		// the sync-owned fields the backend re-fetch cannot recover) via POST
		if !resp.Deleted || resp.Contract != address || resp.Purged == nil || resp.Purged.Name != "Renamed" {
			t.Fatalf("DELETE = %+v, want deleted:true with the purged record", resp)
		}
		// assert on the DB directly — the GET endpoint would re-fetch from the
		// backend and re-store the row
		stored, err := s.db.GetContractInfoForAddress(address)
		if err != nil {
			t.Fatal(err)
		}
		if stored != nil {
			t.Fatalf("stored contract after DELETE = %+v, want nil", stored)
		}
		// idempotent: deleting a missing row reports deleted:false, not an error
		code, body = adminRequest(t, ts, http.MethodDelete, "/admin/contract-info/"+address, "")
		if code != http.StatusOK {
			t.Fatalf("repeated DELETE = %d %s, want 200", code, body)
		}
		resp = contractInfoDeleteResponse{}
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			t.Fatalf("repeated DELETE body %q does not decode: %v", body, err)
		}
		if resp.Deleted || resp.Purged != nil {
			t.Fatalf("repeated DELETE = %+v, want deleted:false without a purged record", resp)
		}
	})

	t.Run("DELETE invalid address", func(t *testing.T) {
		code, body := adminRequest(t, ts, http.MethodDelete, "/admin/contract-info/not-an-address", "")
		if code != http.StatusBadRequest {
			t.Fatalf("DELETE = %d %s, want 400", code, body)
		}
	})

	t.Run("DELETE missing address", func(t *testing.T) {
		code, body := adminRequest(t, ts, http.MethodDelete, "/admin/contract-info/", "")
		if code != http.StatusBadRequest || !strings.Contains(body, "Missing contract address") {
			t.Fatalf("DELETE = %d %s, want 400 Missing contract address", code, body)
		}
	})
}

// TestInternalServerRouteGatingNonEVM verifies that the chain-generic
// runtime-settings admin routes are registered on a non-EVM chain while the
// EVM-specific contract-info routes stay gated (404 via the authenticated
// catch-all).
func TestInternalServerRouteGatingNonEVM(t *testing.T) {
	t.Setenv("BB_ADMIN_USER", "admin")
	t.Setenv("BB_ADMIN_PASSWORD", "password")
	parser, chain := setupChain(t)
	s, dbpath := setupPublicHTTPServer(parser, chain, t, false)
	defer closeAndDestroyPublicServer(t, s, dbpath)
	ts := newInternalTestServer(t, s)

	code, body := adminRequest(t, ts, http.MethodGet, "/admin/runtime-settings/ALLOWED_RPC_CALL_TO", "")
	if code != http.StatusOK {
		t.Fatalf("GET runtime setting = %d %s, want 200 on a non-EVM chain", code, body)
	}
	var resp runtimeSettingResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("runtime setting body %q does not decode: %v", body, err)
	}
	if resp.Key != "ALLOWED_RPC_CALL_TO" || resp.Source != "unset" {
		t.Fatalf("runtime setting = %+v, want unset ALLOWED_RPC_CALL_TO", resp)
	}

	code, _ = adminRequest(t, ts, http.MethodGet, "/admin/runtime-settings", "")
	if code != http.StatusOK {
		t.Fatalf("GET runtime-settings page = %d, want 200 on a non-EVM chain", code)
	}

	code, _ = adminRequest(t, ts, http.MethodGet, "/admin/contract-info/0x20cd153de35d469ba46127a0c8f18626b59a256a", "")
	if code != http.StatusNotFound {
		t.Fatalf("GET contract-info = %d, want 404 on a non-EVM chain", code)
	}
}
