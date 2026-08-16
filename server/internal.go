package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/trezor/blockbook/api"
	"github.com/trezor/blockbook/bchain"
	"github.com/trezor/blockbook/common"
	"github.com/trezor/blockbook/db"
	"github.com/trezor/blockbook/fiat"
)

// InternalServer is handle to internal http server
type InternalServer struct {
	htmlTemplates[InternalTemplateData]
	https       *http.Server
	certFiles   string
	db          *db.RocksDB
	txCache     *db.TxCache
	chain       bchain.BlockChain
	chainParser bchain.BlockChainParser
	mempool     bchain.Mempool
	is          *common.InternalState
	api         *api.Worker
	// Admin HTTP Basic-auth credentials for the /admin endpoints, derived from
	// BB_ADMIN_USER/BB_ADMIN_PASSWORD. adminAuthEnabled is false when either is unset,
	// keeping the admin surface fail-closed (see requireAdminAuth). Only SHA-256
	// digests are retained, for constant-time comparison.
	adminAuthEnabled bool
	adminUserHash    [32]byte
	adminPassHash    [32]byte
	// runtimeSettingsMux serializes runtime-setting writes (validate → store
	// to DB → publish snapshot) so concurrent admin requests cannot publish a
	// snapshot whose value lost the database write. runtimeSettings is the
	// persistence backend of those writes (the RocksDB in production).
	runtimeSettingsMux sync.Mutex
	runtimeSettings    runtimeSettingStore
}

// NewInternalServer creates new internal http interface to blockbook and returns its handle
func NewInternalServer(binding, certFiles string, db *db.RocksDB, chain bchain.BlockChain, mempool bchain.Mempool, txCache *db.TxCache, metrics *common.Metrics, is *common.InternalState, fiatRates *fiat.FiatRates) (*InternalServer, error) {
	api, err := api.NewWorker(db, chain, mempool, txCache, metrics, is, fiatRates)
	if err != nil {
		return nil, err
	}

	addr, path := splitBinding(binding)
	serveMux := http.NewServeMux()
	https := &http.Server{
		Addr:    addr,
		Handler: serveMux,
	}
	s := &InternalServer{
		htmlTemplates: htmlTemplates[InternalTemplateData]{
			debug: true,
		},
		https:           https,
		certFiles:       certFiles,
		db:              db,
		txCache:         txCache,
		chain:           chain,
		chainParser:     chain.GetChainParser(),
		mempool:         mempool,
		is:              is,
		api:             api,
		runtimeSettings: db,
	}
	s.htmlTemplates.newTemplateData = s.newTemplateData
	s.htmlTemplates.newTemplateDataWithError = s.newTemplateDataWithError
	s.htmlTemplates.parseTemplates = s.parseTemplates
	s.templates = s.parseTemplates()

	// The internal server binds all interfaces by default (configs/coins/*:
	// internal_binding_template is ":<port>"), so the /admin endpoints are gated by
	// HTTP Basic auth. Basic auth (rather than a bearer token) lets the admin HTML
	// pages and forms be used directly from a browser via its native login prompt.
	// Credentials come from the process environment like the other runtime secrets
	// (see docs/env.md, blockbook.env).
	s.configureAdminAuth(os.Getenv("BB_ADMIN_USER"), os.Getenv("BB_ADMIN_PASSWORD"))
	if s.adminAuthEnabled {
		glog.Info("internal server: /admin authentication enabled (HTTP Basic auth)")
	} else {
		glog.Warning("internal server: BB_ADMIN_USER/BB_ADMIN_PASSWORD not both set; /admin endpoints are disabled (HTTP 503). Set them in blockbook.env to enable them.")
	}

	serveMux.Handle(path+"favicon.ico", http.FileServer(http.Dir("./static/")))
	serveMux.Handle(path+"static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static/"))))
	serveMux.HandleFunc(path+"metrics", promhttp.Handler().ServeHTTP)
	serveMux.HandleFunc(path, s.index)
	// Gate the whole /admin surface behind auth. The trailing-slash catch-all keeps
	// unregistered /admin/* subpaths authenticated (so they cannot fall through to the
	// public index handler); a bare "/admin/" is redirected to the canonical "/admin".
	adminPath := path + "admin"
	serveMux.HandleFunc(adminPath, s.requireAdminAuth(s.htmlTemplateHandler(s.adminIndex)))
	serveMux.HandleFunc(adminPath+"/", s.requireAdminAuth(s.adminSubtreeHandler(adminPath)))
	serveMux.HandleFunc(adminPath+"/ws-limit-exceeding-ips", s.requireAdminAuth(s.htmlTemplateHandler(s.wsLimitExceedingIPs)))
	// Runtime settings are chain-generic (initRpcCallAllowlists already runs
	// unconditionally in NewWebsocketServer); the currently defined settings only
	// affect EVM rpcCall and are simply unset on other chains. The init must stay
	// before the route registration — currentRpcCallAllowlists relies on it.
	if err := initRpcCallAllowlists(db, is); err != nil {
		return nil, err
	}
	serveMux.HandleFunc(adminPath+"/runtime-settings", s.requireAdminAuth(s.htmlTemplateHandler(s.runtimeSettingsPage)))
	serveMux.HandleFunc(adminPath+"/runtime-settings/", s.requireAdminAuth(s.jsonHandler(s.apiRuntimeSetting, 0)))
	if s.chainParser.GetChainType() == bchain.ChainEthereumType {
		serveMux.HandleFunc(adminPath+"/internal-data-errors", s.requireAdminAuth(s.htmlTemplateHandler(s.internalDataErrors)))
		serveMux.HandleFunc(adminPath+"/contract-info", s.requireAdminAuth(s.htmlTemplateHandler(s.contractInfoPage)))
		serveMux.HandleFunc(adminPath+"/contract-info/", s.requireAdminAuth(s.jsonHandler(s.apiContractInfo, 0)))
	}
	return s, nil
}

// configureAdminAuth derives the /admin Basic-auth credentials from the given raw
// values (the BB_ADMIN_USER/BB_ADMIN_PASSWORD environment variables). Surrounding
// whitespace is stripped so a stray space or newline in blockbook.env does not lock
// the operator out. If either value is empty the admin surface stays disabled
// (fail-closed). Only the SHA-256 digests are kept, for constant-time comparison.
func (s *InternalServer) configureAdminAuth(rawUser, rawPass string) {
	user := strings.TrimSpace(rawUser)
	pass := strings.TrimSpace(rawPass)
	s.adminAuthEnabled = user != "" && pass != ""
	s.adminUserHash = sha256.Sum256([]byte(user))
	s.adminPassHash = sha256.Sum256([]byte(pass))
}

// adminSubtreeHandler backs the /admin/ trailing-slash catch-all. A bare "/admin/"
// (the trailing-slash form of the index) is redirected to the canonical adminPath
// ("/admin"); any deeper unregistered /admin/* path is a 404. It is registered behind
// requireAdminAuth, so unknown subpaths stay gated rather than reaching the index.
func (s *InternalServer) adminSubtreeHandler(adminPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == adminPath+"/" {
			http.Redirect(w, r, adminPath, http.StatusFound)
			return
		}
		http.NotFound(w, r)
	}
}

// urlPathSegment returns the last segment of the request path with surrounding
// whitespace trimmed, or "" when the path ends in "/" or has no sub-segment
// below a registered subtree (a root-level path like "/admin" yields ""). Callers
// apply their own normalization on top (runtime-setting keys are uppercased,
// contract addresses are left to the chain parser, the authority on case and
// checksum handling).
func urlPathSegment(r *http.Request) string {
	var seg string
	if i := strings.LastIndexByte(r.URL.Path, '/'); i > 0 {
		seg = r.URL.Path[i+1:]
	}
	return strings.TrimSpace(seg)
}

// requireAdminAuth wraps an internal-server handler so it is reachable only with
// valid HTTP Basic credentials. The admin surface is fail-closed: when the
// credentials are not configured the endpoints return 503 rather than serving
// unauthenticated, because the internal server binds all interfaces by default.
func (s *InternalServer) requireAdminAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.adminAuthEnabled {
			http.Error(w, "admin interface disabled", http.StatusServiceUnavailable)
			return
		}
		if !s.validBasicAuth(r) {
			// Prompt browsers for credentials; charset advertises UTF-8 passwords.
			w.Header().Set("WWW-Authenticate", `Basic realm="blockbook-admin", charset="UTF-8"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// validBasicAuth reports whether the request carries the configured admin Basic
// credentials. Both fields are compared via SHA-256 digests so the comparison is
// constant-time and independent of the submitted lengths; both comparisons are
// evaluated before being combined so a username mismatch is not distinguishable by
// timing from a password mismatch.
func (s *InternalServer) validBasicAuth(r *http.Request) bool {
	user, pass, ok := r.BasicAuth()
	if !ok {
		return false
	}
	gotUser := sha256.Sum256([]byte(user))
	gotPass := sha256.Sum256([]byte(pass))
	userOK := subtle.ConstantTimeCompare(gotUser[:], s.adminUserHash[:]) == 1
	passOK := subtle.ConstantTimeCompare(gotPass[:], s.adminPassHash[:]) == 1
	return userOK && passOK
}

// Run starts the server
func (s *InternalServer) Run() error {
	if s.certFiles == "" {
		glog.Info("internal server: starting to listen on http://", s.https.Addr)
		return s.https.ListenAndServe()
	}
	glog.Info("internal server: starting to listen on https://", s.https.Addr)
	return s.https.ListenAndServeTLS(fmt.Sprint(s.certFiles, ".crt"), fmt.Sprint(s.certFiles, ".key"))
}

// Close closes the server
func (s *InternalServer) Close() error {
	glog.Infof("internal server: closing")
	return s.https.Close()
}

// Shutdown shuts down the server
func (s *InternalServer) Shutdown(ctx context.Context) error {
	glog.Infof("internal server: shutdown")
	return s.https.Shutdown(ctx)
}

func (s *InternalServer) index(w http.ResponseWriter, r *http.Request) {
	si, err := s.api.GetSystemInfo(true)
	if err != nil {
		glog.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	buf, err := json.MarshalIndent(si, "", "    ")
	if err != nil {
		glog.Error(err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Write(buf)
}

const (
	adminIndexTpl = iota + errorInternalTpl + 1
	adminInternalErrorsTpl
	adminLimitExceedingIPSTpl
	adminContractInfoTpl
	adminRuntimeSettingsTpl

	internalTplCount
)

// WsLimitExceedingIP is used to transfer data to the templates
type WsLimitExceedingIP struct {
	IP    string
	Count int
}

// WsBlockedIPView is a single row of the websocket IP blocklist rendered on the
// admin page (times pre-formatted so the template needs no time helpers).
type WsBlockedIPView struct {
	Key       string
	Breaches  int
	Rejected  int
	BlockedAt string
	Until     string
	Remaining string
}

// InternalTemplateData is used to transfer data to the templates
type InternalTemplateData struct {
	CoinName               string
	CoinShortcut           string
	CoinLabel              string
	ChainType              bchain.ChainType
	Error                  *api.APIError
	InternalDataErrors     []db.BlockInternalDataError
	RefetchingInternalData bool
	WsGetAccountInfoLimit  int
	WsLimitExceedingIPs    []WsLimitExceedingIP
	WsBlockedIPs           []WsBlockedIPView
	RuntimeSettings        []RuntimeSettingView
}

func (s *InternalServer) newTemplateData(r *http.Request) *InternalTemplateData {
	t := &InternalTemplateData{
		CoinName:     s.is.Coin,
		CoinShortcut: s.is.CoinShortcut,
		CoinLabel:    s.is.CoinLabel,
		ChainType:    s.chainParser.GetChainType(),
	}
	return t
}

func (s *InternalServer) newTemplateDataWithError(error *api.APIError, r *http.Request) *InternalTemplateData {
	td := s.newTemplateData(r)
	td.Error = error
	return td
}

func (s *InternalServer) parseTemplates() []*template.Template {
	templateFuncMap := template.FuncMap{
		"formatUint32": formatUint32,
	}
	createTemplate := func(filenames ...string) *template.Template {
		if len(filenames) == 0 {
			panic("Missing templates")
		}
		return template.Must(template.New(filepath.Base(filenames[0])).Funcs(templateFuncMap).ParseFiles(filenames...))
	}
	t := make([]*template.Template, internalTplCount)
	t[errorTpl] = createTemplate("./static/internal_templates/error.html", "./static/internal_templates/base.html")
	t[errorInternalTpl] = createTemplate("./static/internal_templates/error.html", "./static/internal_templates/base.html")
	t[adminIndexTpl] = createTemplate("./static/internal_templates/index.html", "./static/internal_templates/base.html")
	t[adminInternalErrorsTpl] = createTemplate("./static/internal_templates/block_internal_data_errors.html", "./static/internal_templates/base.html")
	t[adminLimitExceedingIPSTpl] = createTemplate("./static/internal_templates/ws_limit_exceeding_ips.html", "./static/internal_templates/base.html")
	t[adminContractInfoTpl] = createTemplate("./static/internal_templates/contract_info.html", "./static/internal_templates/base.html")
	t[adminRuntimeSettingsTpl] = createTemplate("./static/internal_templates/runtime_settings.html", "./static/internal_templates/base.html")
	return t
}

func (s *InternalServer) adminIndex(w http.ResponseWriter, r *http.Request) (tpl, *InternalTemplateData, error) {
	data := s.newTemplateData(r)
	return adminIndexTpl, data, nil
}

func (s *InternalServer) internalDataErrors(w http.ResponseWriter, r *http.Request) (tpl, *InternalTemplateData, error) {
	if r.Method == http.MethodPost {
		err := s.api.RefetchInternalData()
		if err != nil {
			return errorTpl, nil, err
		}
	}
	data := s.newTemplateData(r)
	internalErrors, err := s.db.GetBlockInternalDataErrorsEthereumType()
	if err != nil {
		return errorTpl, nil, err
	}
	data.InternalDataErrors = internalErrors
	data.RefetchingInternalData = s.api.IsRefetchingInternalData()
	return adminInternalErrorsTpl, data, nil
}

func (s *InternalServer) wsLimitExceedingIPs(w http.ResponseWriter, r *http.Request) (tpl, *InternalTemplateData, error) {
	if r.Method == http.MethodPost {
		// The page has two reset buttons; reset=blocked clears the temporary IP
		// blocklist, anything else (including the legacy button with no field)
		// clears the getAccountInfo limit-exceeding counters.
		if r.FormValue("reset") == "blocked" {
			s.is.ResetWsBlockedIPs()
		} else {
			s.is.ResetWsLimitExceedingIPs()
		}
	}
	data := s.newTemplateData(r)
	// snapshot under the InternalState mutex; ranging over the live map races
	// with AddWsLimitExceedingIP
	exceeding := s.is.WsLimitExceedingIPsSnapshot()
	ips := make([]WsLimitExceedingIP, 0, len(exceeding))
	for k, v := range exceeding {
		ips = append(ips, WsLimitExceedingIP{k, v})
	}
	sort.Slice(ips, func(i, j int) bool {
		return ips[i].Count > ips[j].Count
	})
	data.WsLimitExceedingIPs = ips
	data.WsGetAccountInfoLimit = s.is.WsGetAccountInfoLimit

	now := time.Now()
	for _, b := range s.is.WsBlockedIPsSnapshot(now) {
		data.WsBlockedIPs = append(data.WsBlockedIPs, WsBlockedIPView{
			Key:       b.Key,
			Breaches:  b.Breaches,
			Rejected:  b.Rejected,
			BlockedAt: b.BlockedAt.UTC().Format(time.RFC3339),
			Until:     b.Until.UTC().Format(time.RFC3339),
			Remaining: b.Until.Sub(now).Round(time.Second).String(),
		})
	}
	return adminLimitExceedingIPSTpl, data, nil
}

func (s *InternalServer) contractInfoPage(w http.ResponseWriter, r *http.Request) (tpl, *InternalTemplateData, error) {
	data := s.newTemplateData(r)
	return adminContractInfoTpl, data, nil
}

// contractInfoUpdateResponse is the JSON shape returned by POST /admin/contract-info/.
type contractInfoUpdateResponse struct {
	Updated int `json:"updated"`
}

// contractInfoListResponse is the JSON shape returned by GET /admin/contract-info/.
// Next, when present, is the from parameter of the next page.
type contractInfoListResponse struct {
	Contracts []bchain.ContractInfo `json:"contracts"`
	Next      string                `json:"next,omitempty"`
}

const (
	contractInfoListDefaultLimit = 1000
	contractInfoListMaxLimit     = 10000
)

// contractInfoDeleteResponse is the JSON shape returned by
// DELETE /admin/contract-info/<address>. Purged carries the removed record so
// the operator can restore it verbatim with a POST — the row includes the
// sync-owned createdInBlock/destructedInBlock fields, which the backend
// re-fetch on the next read cannot recover.
type contractInfoDeleteResponse struct {
	Contract string               `json:"contract"`
	Deleted  bool                 `json:"deleted"`
	Purged   *bchain.ContractInfo `json:"purged,omitempty"`
}

// apiContractInfo handles GET/POST/PUT/DELETE of cached contract metadata at
// /admin/contract-info/<address> (POST/PUT write the collection path
// /admin/contract-info/ with a JSON array body; a GET of the collection path
// lists the stored records page by page).
func (s *InternalServer) apiContractInfo(r *http.Request, apiVersion int) (interface{}, error) {
	address := urlPathSegment(r)
	switch r.Method {
	case http.MethodGet:
		if address == "" {
			return s.listContractInfos(r)
		}
		return s.getContractInfo(address)
	case http.MethodPost, http.MethodPut:
		// The bulk write addresses each contract in its body; reject a POST to
		// an address path instead of silently ignoring the address segment.
		if address != "" {
			return nil, api.NewAPIError("POST updates the collection; use /admin/contract-info/ with a JSON array body", true)
		}
		return s.updateContracts(r)
	case http.MethodDelete:
		return s.deleteContractInfo(address, r)
	}
	return nil, api.NewAPIError("Unsupported method "+r.Method, true)
}

// listContractInfos returns one page of the stored contract records. Unlike
// the runtime-settings list, the collection is unbounded (sync stores a row
// per contract creation), so the page size is limited and the response
// carries a next cursor.
func (s *InternalServer) listContractInfos(r *http.Request) (interface{}, error) {
	limit := contractInfoListDefaultLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > contractInfoListMaxLimit {
			return nil, api.NewAPIError("Invalid limit, expecting a number in 1.."+strconv.Itoa(contractInfoListMaxLimit), true)
		}
		limit = n
	}
	contracts, next, err := s.db.ListContractInfos(r.URL.Query().Get("from"), limit)
	if err != nil {
		return nil, api.NewAPIError(err.Error(), true)
	}
	return &contractInfoListResponse{Contracts: contracts, Next: next}, nil
}

func (s *InternalServer) getContractInfo(address string) (interface{}, error) {
	contractInfo, valid, err := s.api.GetContractInfo(address, bchain.UnknownTokenStandard)
	if err != nil {
		return nil, api.NewAPIError(err.Error(), true)
	}
	if !valid {
		return nil, api.NewAPIError("Not a contract", true)
	}
	return contractInfo, nil
}

func (s *InternalServer) updateContracts(r *http.Request) (interface{}, error) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, api.NewAPIError("Cannot get request body", true)
	}
	var contractInfos []bchain.ContractInfo
	err = json.Unmarshal(data, &contractInfos)
	if err != nil {
		return nil, api.NewAPIError("Cannot unmarshal body to array of ContractInfo objects: "+err.Error(), true)
	}
	for i := range contractInfos {
		c := &contractInfos[i]
		err := s.db.StoreContractInfo(c)
		if err != nil {
			return nil, api.NewAPIError("Error updating contract "+c.Contract+" "+err.Error(), true)
		}

	}
	return &contractInfoUpdateResponse{Updated: len(contractInfos)}, nil
}

// deleteContractInfo purges the stored metadata of one contract so the next
// read re-fetches it from the backend node. The whole row is discarded — the
// backend re-fetch restores only name/symbol/decimals, not the sync-owned
// createdInBlock/destructedInBlock, so the purged record is logged and
// returned for a POST restore. Deleting is idempotent: a missing row reports
// deleted=false rather than an error, matching the runtime-settings DELETE
// semantics.
func (s *InternalServer) deleteContractInfo(address string, r *http.Request) (interface{}, error) {
	if address == "" {
		return nil, api.NewAPIError("Missing contract address", true)
	}
	purged, err := s.db.DeleteContractInfoForAddress(address)
	if err != nil {
		return nil, api.NewAPIError(err.Error(), true)
	}
	if purged != nil {
		glog.Infof("admin: contract info %s purged (%+v), client %s", address, *purged, r.RemoteAddr)
	}
	return &contractInfoDeleteResponse{Contract: address, Deleted: purged != nil, Purged: purged}, nil
}
