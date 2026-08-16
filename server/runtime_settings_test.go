//go:build unittest
// +build unittest

package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/trezor/blockbook/bchain/coins/eth"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/tests/dbtestdata"
)

func TestParseAllowedRpcCallTo(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    []string
		wantErr bool
	}{
		{name: "empty", value: "", want: nil},
		{name: "single address", value: "0xcdA9FC258358EcaA88845f19Af595e908bb7EfE9", want: []string{"0xcda9fc258358ecaa88845f19af595e908bb7efe9"}},
		{name: "multiple with spaces", value: " 0xABCD , 0xEF01 ", want: []string{"0xabcd", "0xef01"}},
		{name: "empty entries skipped", value: "0xabcd,,0xef01", want: []string{"0xabcd", "0xef01"}},
		{name: "separators only", value: ",", wantErr: true},
		{name: "whitespace entries only", value: " , ,", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAllowedRpcCallTo("FAKE_ALLOWED_RPC_CALL_TO", tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseAllowedRpcCallTo(%q) = nil err, want error", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAllowedRpcCallTo(%q) unexpected error: %v", tt.value, err)
			}
			if tt.want == nil && got != nil {
				t.Fatalf("parseAllowedRpcCallTo(%q) = %v, want nil", tt.value, got)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseAllowedRpcCallTo(%q) len = %d, want %d", tt.value, len(got), len(tt.want))
			}
			for _, a := range tt.want {
				if _, ok := got[a]; !ok {
					t.Fatalf("parseAllowedRpcCallTo(%q) missing %q", tt.value, a)
				}
			}
		})
	}
}

// A whitespace-only environment value must fail startup resolution the same
// way a value with no parseable entries does — treating it as unset would
// silently leave rpcCall unrestricted.
func TestRuntimeSettingWhitespaceOnlyEnvFailsInit(t *testing.T) {
	t.Setenv("FAKE_ALLOWED_RPC_CALL_TO", " \n")
	is := &common.InternalState{CoinShortcut: "FAKE"}
	if err := initRpcCallAllowlists(nil, is); err == nil || !strings.Contains(err.Error(), "only whitespace") {
		t.Fatalf("initRpcCallAllowlists() err = %v, want whitespace configuration error", err)
	}
}

func doRuntimeSettingRequest(t *testing.T, handler http.HandlerFunc, method, key, body string) (int, map[string]string) {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	r := httptest.NewRequest(method, "/admin/runtime-settings/"+key, rdr)
	w := httptest.NewRecorder()
	handler(w, r)
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("%s %s: cannot unmarshal response %q: %v", method, key, w.Body.String(), err)
	}
	return w.Code, resp
}

// failingRuntimeSettingStore fails every store call; it stands in for a
// broken database to exercise the store-before-publish error paths.
type failingRuntimeSettingStore struct{}

func (failingRuntimeSettingStore) GetRuntimeSetting(name string) (string, bool, error) {
	return "", false, errors.New("get failed")
}

func (failingRuntimeSettingStore) StoreRuntimeSetting(name, value string) error {
	return errors.New("store failed")
}

func (failingRuntimeSettingStore) DeleteRuntimeSetting(name string) error {
	return errors.New("delete failed")
}

// The subtests of TestRuntimeSettingsAPI intentionally share the DB and
// snapshot state and depend on running in order; run the whole test, not
// individual subtests.
func TestRuntimeSettingsAPI(t *testing.T) {
	const envTo = "0xcdA9FC258358EcaA88845f19Af595e908bb7EfE9"
	// the surrounding whitespace must never surface: the env value is trimmed
	// wherever it is resolved (startup and DELETE fallback alike)
	t.Setenv("FAKE_ALLOWED_RPC_CALL_TO", " "+envTo+"\n")
	parser := eth.NewEthereumParser(1, true)
	chain, err := dbtestdata.NewFakeBlockChainEthereumType(parser)
	if err != nil {
		t.Fatal(err)
	}
	d, is, path := setupRocksDB(parser, chain, t, false, &common.Config{CoinName: "Fakecoin", CoinShortcut: "FAKE"})
	defer func() {
		if err := d.Close(); err != nil {
			t.Error(err)
		}
		os.RemoveAll(path)
	}()
	if err := initRpcCallAllowlists(d, is); err != nil {
		t.Fatal(err)
	}
	s := &InternalServer{
		htmlTemplates:   htmlTemplates[InternalTemplateData]{debug: true},
		db:              d,
		is:              is,
		chainParser:     parser,
		runtimeSettings: d,
	}
	handler := s.jsonHandler(s.apiRuntimeSetting, 0)

	t.Run("GET unknown key", func(t *testing.T) {
		code, resp := doRuntimeSettingRequest(t, handler, http.MethodGet, "NO_SUCH_SETTING", "")
		if code != http.StatusBadRequest || !strings.Contains(resp["error"], "Unknown runtime setting") {
			t.Fatalf("got %d %v, want 400 unknown runtime setting", code, resp)
		}
	})

	t.Run("GET env-sourced value", func(t *testing.T) {
		code, resp := doRuntimeSettingRequest(t, handler, http.MethodGet, "ALLOWED_RPC_CALL_TO", "")
		if code != http.StatusOK || resp["value"] != envTo || resp["source"] != common.RuntimeSettingSourceEnv {
			t.Fatalf("got %d %v, want 200 value %q source env", code, resp, envTo)
		}
	})

	t.Run("GET unset value", func(t *testing.T) {
		code, resp := doRuntimeSettingRequest(t, handler, http.MethodGet, "ALLOWED_EVM_CALL_METHODS", "")
		if code != http.StatusOK || resp["value"] != "" || resp["source"] != common.RuntimeSettingSourceUnset {
			t.Fatalf("got %d %v, want 200 empty value source unset", code, resp)
		}
	})

	t.Run("GET all settings", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/admin/runtime-settings/", nil)
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("got %d %s, want 200", w.Code, w.Body.String())
		}
		var settings []runtimeSettingResponse
		if err := json.Unmarshal(w.Body.Bytes(), &settings); err != nil {
			t.Fatalf("cannot unmarshal response %q: %v", w.Body.String(), err)
		}
		want := map[string]runtimeSettingResponse{
			runtimeSettingAllowedRpcCallTo:      {Key: runtimeSettingAllowedRpcCallTo, Value: envTo, Source: common.RuntimeSettingSourceEnv},
			runtimeSettingAllowedEvmCallMethods: {Key: runtimeSettingAllowedEvmCallMethods, Value: "", Source: common.RuntimeSettingSourceUnset},
		}
		if len(settings) != len(want) {
			t.Fatalf("got %d settings %v, want %d", len(settings), settings, len(want))
		}
		for _, s := range settings {
			if s != want[s.Key] {
				t.Fatalf("setting %s = %+v, want %+v", s.Key, s, want[s.Key])
			}
		}
	})

	t.Run("empty key accepts only GET", func(t *testing.T) {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
			code, resp := doRuntimeSettingRequest(t, handler, method, "", `{"value":""}`)
			if code != http.StatusBadRequest || !strings.Contains(resp["error"], "Unknown runtime setting") {
				t.Fatalf("%s: got %d %v, want 400 unknown runtime setting", method, code, resp)
			}
		}
	})

	t.Run("key is case-insensitive", func(t *testing.T) {
		code, resp := doRuntimeSettingRequest(t, handler, http.MethodGet, "allowed_rpc_call_to", "")
		if code != http.StatusOK || resp["key"] != runtimeSettingAllowedRpcCallTo {
			t.Fatalf("got %d %v, want 200 key %s", code, resp, runtimeSettingAllowedRpcCallTo)
		}
	})

	t.Run("POST invalid selector is rejected", func(t *testing.T) {
		code, _ := doRuntimeSettingRequest(t, handler, http.MethodPost, "ALLOWED_EVM_CALL_METHODS", `{"value":"0x12"}`)
		if code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", code)
		}
		if _, found, _ := d.GetRuntimeSetting(runtimeSettingAllowedEvmCallMethods); found {
			t.Fatal("rejected POST must not create a DB row")
		}
		if a := is.GetRpcCallAllowlists(); a.Methods != nil || a.MethodsSource != common.RuntimeSettingSourceUnset {
			t.Fatalf("rejected POST must not change the snapshot, got %+v", a)
		}
	})

	t.Run("POST invalid to list is rejected", func(t *testing.T) {
		code, _ := doRuntimeSettingRequest(t, handler, http.MethodPost, "ALLOWED_RPC_CALL_TO", `{"value":","}`)
		if code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", code)
		}
	})

	t.Run("POST bad body is rejected", func(t *testing.T) {
		for _, body := range []string{"not json", `{"other":"field"}`} {
			code, _ := doRuntimeSettingRequest(t, handler, http.MethodPost, "ALLOWED_EVM_CALL_METHODS", body)
			if code != http.StatusBadRequest {
				t.Fatalf("body %q: got %d, want 400", body, code)
			}
		}
	})

	t.Run("POST whitespace-only value is rejected", func(t *testing.T) {
		// only a value of exactly "" is an explicit unconfigure; a
		// whitespace- or separator-only value is a typo that must not
		// silently un-restrict rpcCall
		for _, body := range []string{`{"value":" "}`, `{"value":"\n"}`} {
			code, _ := doRuntimeSettingRequest(t, handler, http.MethodPost, "ALLOWED_EVM_CALL_METHODS", body)
			if code != http.StatusBadRequest {
				t.Fatalf("body %q: got %d, want 400", body, code)
			}
		}
		if _, found, _ := d.GetRuntimeSetting(runtimeSettingAllowedEvmCallMethods); found {
			t.Fatal("rejected POST must not create a DB row")
		}
	})

	t.Run("unsupported method is rejected", func(t *testing.T) {
		code, _ := doRuntimeSettingRequest(t, handler, http.MethodPatch, "ALLOWED_EVM_CALL_METHODS", "")
		if code != http.StatusBadRequest {
			t.Fatalf("got %d, want 400", code)
		}
	})

	t.Run("POST stores override and updates snapshot", func(t *testing.T) {
		code, resp := doRuntimeSettingRequest(t, handler, http.MethodPost, "ALLOWED_EVM_CALL_METHODS", `{"value":"0xdd62ed3e,0x70a08231"}`)
		if code != http.StatusOK || resp["value"] != "0xdd62ed3e,0x70a08231" || resp["source"] != common.RuntimeSettingSourceDB {
			t.Fatalf("got %d %v, want 200 source db", code, resp)
		}
		value, found, err := d.GetRuntimeSetting(runtimeSettingAllowedEvmCallMethods)
		if err != nil || !found || value != "0xdd62ed3e,0x70a08231" {
			t.Fatalf("DB row = %q, %v, %v, want stored value", value, found, err)
		}
		a := is.GetRpcCallAllowlists()
		if _, ok := a.Methods["70a08231"]; !ok || a.MethodsSource != common.RuntimeSettingSourceDB {
			t.Fatalf("snapshot not updated: %+v", a)
		}
	})

	t.Run("POST empty value explicitly unconfigures", func(t *testing.T) {
		code, resp := doRuntimeSettingRequest(t, handler, http.MethodPost, "ALLOWED_RPC_CALL_TO", `{"value":""}`)
		if code != http.StatusOK || resp["value"] != "" || resp["source"] != common.RuntimeSettingSourceDB {
			t.Fatalf("got %d %v, want 200 empty value source db", code, resp)
		}
		a := is.GetRpcCallAllowlists()
		if a.To != nil || a.ToSource != common.RuntimeSettingSourceDB {
			t.Fatalf("snapshot not cleared: %+v", a)
		}
		if value, found, _ := d.GetRuntimeSetting(runtimeSettingAllowedRpcCallTo); !found || value != "" {
			t.Fatalf("DB row = %q, %v, want stored empty value", value, found)
		}
	})

	t.Run("stored empty override beats env after restart", func(t *testing.T) {
		// simulated restart: the stored empty override must keep the
		// dimension unconfigured even though the env var has a value
		restarted := &common.InternalState{CoinShortcut: "FAKE"}
		if err := initRpcCallAllowlists(d, restarted); err != nil {
			t.Fatal(err)
		}
		a := restarted.GetRpcCallAllowlists()
		if a.To != nil || a.ToValue != "" || a.ToSource != common.RuntimeSettingSourceDB {
			t.Fatalf("after restart got To %v value %q source %q, want unconfigured db override", a.To, a.ToValue, a.ToSource)
		}
	})

	t.Run("stored override shadows malformed env without failing init", func(t *testing.T) {
		// drift between a stored override and the environment must not fail a
		// restart — the shadowed env value is never parsed, the override wins
		// and the mismatch is only logged
		t.Setenv("FAKE_ALLOWED_EVM_CALL_METHODS", "not-a-selector")
		restarted := &common.InternalState{CoinShortcut: "FAKE"}
		if err := initRpcCallAllowlists(d, restarted); err != nil {
			t.Fatal(err)
		}
		a := restarted.GetRpcCallAllowlists()
		if a.MethodsSource != common.RuntimeSettingSourceDB || a.MethodsValue != "0xdd62ed3e,0x70a08231" {
			t.Fatalf("got source %q value %q, want the db override", a.MethodsSource, a.MethodsValue)
		}
	})

	t.Run("DELETE reverts to env", func(t *testing.T) {
		code, resp := doRuntimeSettingRequest(t, handler, http.MethodDelete, "ALLOWED_RPC_CALL_TO", "")
		if code != http.StatusOK || resp["value"] != envTo || resp["source"] != common.RuntimeSettingSourceEnv {
			t.Fatalf("got %d %v, want 200 value %q source env", code, resp, envTo)
		}
		if _, found, _ := d.GetRuntimeSetting(runtimeSettingAllowedRpcCallTo); found {
			t.Fatal("DELETE must remove the DB row")
		}
		a := is.GetRpcCallAllowlists()
		if _, ok := a.To[strings.ToLower(envTo)]; !ok || a.ToSource != common.RuntimeSettingSourceEnv {
			t.Fatalf("snapshot not reverted to env: %+v", a)
		}
	})

	t.Run("DELETE with malformed env fallback is rejected", func(t *testing.T) {
		t.Setenv("FAKE_ALLOWED_EVM_CALL_METHODS", "not-a-selector")
		code, resp := doRuntimeSettingRequest(t, handler, http.MethodDelete, "ALLOWED_EVM_CALL_METHODS", "")
		if code != http.StatusBadRequest {
			t.Fatalf("got %d %v, want 400", code, resp)
		}
		if _, found, _ := d.GetRuntimeSetting(runtimeSettingAllowedEvmCallMethods); !found {
			t.Fatal("rejected DELETE must keep the DB row")
		}
		if a := is.GetRpcCallAllowlists(); a.MethodsSource != common.RuntimeSettingSourceDB {
			t.Fatalf("rejected DELETE must not change the snapshot: %+v", a)
		}
	})

	t.Run("DELETE with whitespace-only env fallback is rejected", func(t *testing.T) {
		// a whitespace-only env value is a configuration error, not unset —
		// reverting to it would silently un-restrict rpcCall and the next
		// restart would resolve the same error
		t.Setenv("FAKE_ALLOWED_EVM_CALL_METHODS", " \n")
		code, resp := doRuntimeSettingRequest(t, handler, http.MethodDelete, "ALLOWED_EVM_CALL_METHODS", "")
		if code != http.StatusBadRequest || !strings.Contains(resp["error"], "only whitespace") {
			t.Fatalf("got %d %v, want 400 whitespace error", code, resp)
		}
		if _, found, _ := d.GetRuntimeSetting(runtimeSettingAllowedEvmCallMethods); !found {
			t.Fatal("rejected DELETE must keep the DB row")
		}
	})

	// proves the DB write happens before the snapshot swap: with a failing
	// store the requests take the error branch (glog.Error + 500) and the
	// live allowlists stay untouched
	t.Run("failed store returns 500 and keeps snapshot", func(t *testing.T) {
		s.runtimeSettings = failingRuntimeSettingStore{}
		defer func() { s.runtimeSettings = d }()
		before := is.GetRpcCallAllowlists()
		code, resp := doRuntimeSettingRequest(t, handler, http.MethodPost, "ALLOWED_EVM_CALL_METHODS", `{"value":"0xa9059cbb"}`)
		if code != http.StatusInternalServerError || !strings.Contains(resp["error"], "Cannot store runtime setting") {
			t.Fatalf("got %d %v, want 500 store error", code, resp)
		}
		if is.GetRpcCallAllowlists() != before {
			t.Fatal("failed DB write must not publish a new snapshot")
		}
		code, resp = doRuntimeSettingRequest(t, handler, http.MethodDelete, "ALLOWED_EVM_CALL_METHODS", "")
		if code != http.StatusInternalServerError || !strings.Contains(resp["error"], "Cannot delete runtime setting") {
			t.Fatalf("got %d %v, want 500 delete error", code, resp)
		}
		if is.GetRpcCallAllowlists() != before {
			t.Fatal("failed DB delete must not publish a new snapshot")
		}
	})
}
