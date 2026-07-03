package server

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"github.com/trezor/blockbook/api"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
)

// Runtime settings are configuration values that can be changed at runtime
// through the internal /admin/runtime-settings interface. Each setting is
// named after the environment variable that provides its startup default
// (prefixed with the network name, e.g. ETH_ALLOWED_RPC_CALL_TO); an override
// written through the admin interface is persisted in the database, survives
// restarts and takes precedence over the environment variable.
const (
	runtimeSettingAllowedRpcCallTo      = "ALLOWED_RPC_CALL_TO"
	runtimeSettingAllowedEvmCallMethods = "ALLOWED_EVM_CALL_METHODS"
)

// runtimeSettingDef describes one runtime setting: how to validate its raw
// value and how to read/write it in the RpcCallAllowlists snapshot. Future
// runtime settings plug in here.
type runtimeSettingDef struct {
	key string
	// parse validates the raw comma-separated value and returns the parsed
	// set; name is used in error messages.
	parse func(name, value string) (map[string]struct{}, error)
	// apply writes the parsed value into a snapshot under construction.
	apply func(a *common.RpcCallAllowlists, parsed map[string]struct{}, value, source string)
	// get reads the raw value and its source from a snapshot.
	get func(a *common.RpcCallAllowlists) (value, source string)
}

var runtimeSettingDefs = []*runtimeSettingDef{
	{
		key:   runtimeSettingAllowedRpcCallTo,
		parse: parseAllowedRpcCallTo,
		apply: func(a *common.RpcCallAllowlists, parsed map[string]struct{}, value, source string) {
			a.To, a.ToValue, a.ToSource = parsed, value, source
		},
		get: func(a *common.RpcCallAllowlists) (string, string) {
			return a.ToValue, a.ToSource
		},
	},
	{
		key:   runtimeSettingAllowedEvmCallMethods,
		parse: parseAllowedEvmCallMethods,
		apply: func(a *common.RpcCallAllowlists, parsed map[string]struct{}, value, source string) {
			a.Methods, a.MethodsValue, a.MethodsSource = parsed, value, source
		},
		get: func(a *common.RpcCallAllowlists) (string, string) {
			return a.MethodsValue, a.MethodsSource
		},
	},
}

func runtimeSettingDefByKey(key string) *runtimeSettingDef {
	for _, def := range runtimeSettingDefs {
		if def.key == key {
			return def
		}
	}
	return nil
}

func runtimeSettingKeys() string {
	keys := make([]string, len(runtimeSettingDefs))
	for i, def := range runtimeSettingDefs {
		keys[i] = def.key
	}
	return strings.Join(keys, ", ")
}

// parseAllowedRpcCallTo parses a comma-separated list of contract addresses
// into a set keyed by the lowercase address string. Entries are trimmed and
// empty entries skipped; a set value that parses to no addresses at all is a
// configuration error — silently ignoring it would leave rpcCall unrestricted,
// and keeping an empty-string entry would allowlist rpcCall requests with an
// empty to field.
func parseAllowedRpcCallTo(name, value string) (map[string]struct{}, error) {
	if value == "" {
		return nil, nil
	}
	addresses := make(map[string]struct{})
	for _, a := range strings.Split(value, ",") {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		addresses[strings.ToLower(a)] = struct{}{}
	}
	if len(addresses) == 0 {
		return nil, errors.Errorf("%s is set but contains no addresses", name)
	}
	return addresses, nil
}

// parseAllowedEvmCallMethods parses a comma-separated list of 4-byte EVM call
// selectors (optional 0x prefix, case-insensitive) into a set keyed by
// lowercase hex without the prefix. Returns nil when the value is unset.
// A malformed selector or a set value that parses to no selectors at all is a
// configuration error so a typo cannot silently disable the intended
// allowlist.
func parseAllowedEvmCallMethods(name, value string) (map[string]struct{}, error) {
	if value == "" {
		return nil, nil
	}
	methods := make(map[string]struct{})
	for _, m := range strings.Split(value, ",") {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		selector := strings.TrimPrefix(strings.ToLower(m), "0x")
		b, err := hex.DecodeString(selector)
		if err != nil || len(b) != 4 {
			return nil, errors.Errorf("invalid EVM call method selector %q in %s, expecting 4 bytes in hex", m, name)
		}
		methods[selector] = struct{}{}
	}
	if len(methods) == 0 {
		return nil, errors.Errorf("%s is set but contains no method selectors", name)
	}
	return methods, nil
}

// runtimeSettingEnvName returns the environment variable that provides the
// startup default of the given runtime setting.
func runtimeSettingEnvName(network, key string) string {
	return strings.ToUpper(network) + "_" + key
}

// runtimeSettingEnvValue returns the trimmed value of the given environment
// variable, so a stray space or newline in an env file does not leak into the
// value (admin POSTs are trimmed the same way). A set but whitespace-only
// value is a configuration error rather than unset: treating it as unset
// would silently un-restrict rpcCall, unlike every other malformed value,
// which fails loudly.
func runtimeSettingEnvValue(envName string) (string, error) {
	raw := os.Getenv(envName)
	value := strings.TrimSpace(raw)
	if raw != "" && value == "" {
		return "", errors.Errorf("%s contains only whitespace", envName)
	}
	return value, nil
}

// resolveRuntimeSetting returns the effective raw value of a runtime setting
// and its source: a stored override wins over the environment variable.
func resolveRuntimeSetting(store runtimeSettingStore, network, key string) (string, string, error) {
	if store != nil {
		value, found, err := store.GetRuntimeSetting(key)
		if err != nil {
			return "", "", err
		}
		if found {
			return value, common.RuntimeSettingSourceDB, nil
		}
	}
	value, err := runtimeSettingEnvValue(runtimeSettingEnvName(network, key))
	if err != nil {
		return "", "", err
	}
	if value != "" {
		return value, common.RuntimeSettingSourceEnv, nil
	}
	return "", common.RuntimeSettingSourceUnset, nil
}

// buildRpcCallAllowlists resolves and parses all runtime settings into a new
// snapshot. An unparsable value — a malformed environment variable or a
// corrupted stored override (writes are validated, so it cannot get there
// through the admin interface) — is an error; falling back could silently
// widen access.
func buildRpcCallAllowlists(d *db.RocksDB, is *common.InternalState) (*common.RpcCallAllowlists, error) {
	// the explicit nil check avoids a typed-nil interface on which the store
	// methods would be called
	var store runtimeSettingStore
	if d != nil {
		store = d
	}
	network := is.GetNetwork()
	a := &common.RpcCallAllowlists{}
	for _, def := range runtimeSettingDefs {
		value, source, err := resolveRuntimeSetting(store, network, def.key)
		if err != nil {
			return nil, err
		}
		var parsed map[string]struct{}
		if value != "" {
			name := def.key
			if source == common.RuntimeSettingSourceEnv {
				name = runtimeSettingEnvName(network, def.key)
			}
			parsed, err = def.parse(name, value)
			if err != nil {
				return nil, err
			}
		}
		def.apply(a, parsed, value, source)
	}
	return a, nil
}

// initRpcCallAllowlists resolves and publishes the rpcCall allowlist snapshot
// if none exists yet. It is called from both NewWebsocketServer and
// NewInternalServer so that each server gets correct allowlists regardless of
// which of them (or both, in any order) a deployment runs; the CAS in
// InitRpcCallAllowlists keeps an already published — and possibly already
// admin-updated — snapshot intact.
func initRpcCallAllowlists(d *db.RocksDB, is *common.InternalState) error {
	if is.GetRpcCallAllowlists() != nil {
		return nil
	}
	a, err := buildRpcCallAllowlists(d, is)
	if err != nil {
		return err
	}
	if !is.InitRpcCallAllowlists(a) {
		return nil
	}
	if a.To != nil {
		glog.Info("Support of rpcCall for these contracts (source ", a.ToSource, "): ", a.ToValue)
	}
	if a.Methods != nil {
		glog.Info("Support of rpcCall for these method selectors (source ", a.MethodsSource, "): ", a.MethodsValue)
	}
	warnShadowedRuntimeSettingEnv(a, is.GetNetwork())
	return nil
}

// warnShadowedRuntimeSettingEnv logs a warning for every stored override that
// shadows a different environment value, so drift between the deployed env
// file and the database (for example a replica that missed an admin update,
// or an env change rolled out while an override exists) is visible at
// startup. It only compares — a shadowed environment value is never parsed or
// validated, so it cannot fail the start.
func warnShadowedRuntimeSettingEnv(a *common.RpcCallAllowlists, network string) {
	for _, def := range runtimeSettingDefs {
		value, source := def.get(a)
		if source != common.RuntimeSettingSourceDB {
			continue
		}
		envName := runtimeSettingEnvName(network, def.key)
		envValue, err := runtimeSettingEnvValue(envName)
		if err != nil {
			glog.Warning("runtime setting ", def.key, ": stored override shadows a malformed environment value: ", err)
			continue
		}
		if envValue != "" && envValue != value {
			glog.Warningf("runtime setting %s: stored override %q shadows a different environment value %q (%s); the override wins until it is removed",
				def.key, value, envValue, envName)
		}
	}
}

// runtimeSettingStore reads and persists runtime setting overrides;
// *db.RocksDB in production, replaceable in tests to exercise storage
// failures.
type runtimeSettingStore interface {
	GetRuntimeSetting(name string) (value string, found bool, err error)
	StoreRuntimeSetting(name, value string) error
	DeleteRuntimeSetting(name string) error
}

// runtimeSettingResponse is the JSON shape returned by the
// /admin/runtime-settings/<KEY> endpoint.
type runtimeSettingResponse struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

// apiRuntimeSetting handles GET/POST/PUT/DELETE of a single runtime setting at
// /admin/runtime-settings/<KEY>; a GET of the bare collection path returns all
// settings, so a management tool can read the whole state in one request.
func (s *InternalServer) apiRuntimeSetting(r *http.Request, apiVersion int) (interface{}, error) {
	key := strings.ToUpper(urlPathSegment(r))
	if key == "" && r.Method == http.MethodGet {
		return s.listRuntimeSettings()
	}
	def := runtimeSettingDefByKey(key)
	if def == nil {
		return nil, api.NewAPIError("Unknown runtime setting, supported: "+runtimeSettingKeys(), true)
	}
	switch r.Method {
	case http.MethodGet:
		return s.getRuntimeSetting(def)
	case http.MethodPost, http.MethodPut:
		return s.updateRuntimeSetting(def, r)
	case http.MethodDelete:
		return s.deleteRuntimeSetting(def, r)
	}
	return nil, api.NewAPIError("Unsupported method "+r.Method, true)
}

// currentRpcCallAllowlists returns the live snapshot; it is published by
// NewInternalServer before the routes are registered, so a nil snapshot means
// a broken test setup rather than a runtime condition.
func (s *InternalServer) currentRpcCallAllowlists() (*common.RpcCallAllowlists, error) {
	a := s.is.GetRpcCallAllowlists()
	if a == nil {
		return nil, errors.New("runtime settings not initialized")
	}
	return a, nil
}

func (s *InternalServer) getRuntimeSetting(def *runtimeSettingDef) (interface{}, error) {
	a, err := s.currentRpcCallAllowlists()
	if err != nil {
		return nil, err
	}
	value, source := def.get(a)
	return &runtimeSettingResponse{Key: def.key, Value: value, Source: source}, nil
}

// listRuntimeSettings returns the effective value and source of every runtime
// setting as a JSON array.
func (s *InternalServer) listRuntimeSettings() (interface{}, error) {
	a, err := s.currentRpcCallAllowlists()
	if err != nil {
		return nil, err
	}
	settings := make([]runtimeSettingResponse, len(runtimeSettingDefs))
	for i, def := range runtimeSettingDefs {
		value, source := def.get(a)
		settings[i] = runtimeSettingResponse{Key: def.key, Value: value, Source: source}
	}
	return settings, nil
}

// updateRuntimeSetting validates the new value, persists it to the database
// and only then publishes it to the live snapshot, so an admin never gets a
// success response for a change that would not survive a restart. A failed
// database write leaves the live allowlists untouched and returns HTTP 500;
// an invalid value returns HTTP 400.
func (s *InternalServer) updateRuntimeSetting(def *runtimeSettingDef, r *http.Request) (interface{}, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, api.NewAPIError("Cannot get request body", true)
	}
	var req struct {
		Value *string `json:"value"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Value == nil {
		return nil, api.NewAPIError(`Body must be a JSON object {"value":"..."}`, true)
	}
	// A value of exactly "" is a valid override: it explicitly unconfigures
	// the dimension (as if the environment variable was not set), which is
	// the only way to un-restrict at runtime when the environment has a
	// value. Anything else — including whitespace- or separator-only values —
	// must parse to at least one entry, so a botched automation input cannot
	// silently disable the allowlist.
	value := strings.TrimSpace(*req.Value)
	var parsed map[string]struct{}
	if *req.Value != "" {
		parsed, err = def.parse(def.key, *req.Value)
		if err != nil {
			return nil, api.NewAPIError(err.Error(), true)
		}
	}
	s.runtimeSettingsMux.Lock()
	defer s.runtimeSettingsMux.Unlock()
	old, err := s.currentRpcCallAllowlists()
	if err != nil {
		return nil, err
	}
	if err := s.runtimeSettings.StoreRuntimeSetting(def.key, value); err != nil {
		glog.Error("admin: storing runtime setting ", def.key, "=", value, " failed: ", err)
		return nil, api.NewAPIError("Cannot store runtime setting "+def.key+": "+err.Error(), false)
	}
	oldValue, oldSource := def.get(old)
	a := *old
	def.apply(&a, parsed, value, common.RuntimeSettingSourceDB)
	s.is.SetRpcCallAllowlists(&a)
	glog.Infof("admin: runtime setting %s changed from %q (%s) to %q (%s), client %s",
		def.key, oldValue, oldSource, value, common.RuntimeSettingSourceDB, r.RemoteAddr)
	return &runtimeSettingResponse{Key: def.key, Value: value, Source: common.RuntimeSettingSourceDB}, nil
}

// deleteRuntimeSetting removes the stored override and reverts the setting to
// its environment default. The environment fallback is validated before the
// row is deleted — deleting with a malformed environment value would leave
// the database and the live state divergent and fail the next restart.
func (s *InternalServer) deleteRuntimeSetting(def *runtimeSettingDef, r *http.Request) (interface{}, error) {
	envName := runtimeSettingEnvName(s.is.GetNetwork(), def.key)
	envValue, err := runtimeSettingEnvValue(envName)
	if err != nil {
		return nil, api.NewAPIError("Cannot remove override, fallback environment value is invalid: "+err.Error()+"; fix the environment or set a value instead", true)
	}
	var parsed map[string]struct{}
	source := common.RuntimeSettingSourceUnset
	if envValue != "" {
		parsed, err = def.parse(envName, envValue)
		if err != nil {
			return nil, api.NewAPIError("Cannot remove override, fallback environment value is invalid: "+err.Error()+"; fix the environment or set a value instead", true)
		}
		source = common.RuntimeSettingSourceEnv
	}
	s.runtimeSettingsMux.Lock()
	defer s.runtimeSettingsMux.Unlock()
	old, err := s.currentRpcCallAllowlists()
	if err != nil {
		return nil, err
	}
	if err := s.runtimeSettings.DeleteRuntimeSetting(def.key); err != nil {
		glog.Error("admin: deleting runtime setting ", def.key, " failed: ", err)
		return nil, api.NewAPIError("Cannot delete runtime setting "+def.key+": "+err.Error(), false)
	}
	oldValue, oldSource := def.get(old)
	a := *old
	def.apply(&a, parsed, envValue, source)
	s.is.SetRpcCallAllowlists(&a)
	glog.Infof("admin: runtime setting %s override removed, reverted from %q (%s) to %q (%s), client %s",
		def.key, oldValue, oldSource, envValue, source, r.RemoteAddr)
	return &runtimeSettingResponse{Key: def.key, Value: envValue, Source: source}, nil
}

// RuntimeSettingView is a single row of the admin runtime-settings page.
type RuntimeSettingView struct {
	Key    string
	Value  string
	Source string
}

func (s *InternalServer) runtimeSettingsPage(w http.ResponseWriter, r *http.Request) (tpl, *InternalTemplateData, error) {
	data := s.newTemplateData(r)
	if a := s.is.GetRpcCallAllowlists(); a != nil {
		for _, def := range runtimeSettingDefs {
			value, source := def.get(a)
			data.RuntimeSettings = append(data.RuntimeSettings, RuntimeSettingView{Key: def.key, Value: value, Source: source})
		}
	}
	return adminRuntimeSettingsTpl, data, nil
}
