//go:build unittest

package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// okHandler is a sentinel "next" handler; reaching it means auth passed.
func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func newAdminServer(user, pass string) *InternalServer {
	s := &InternalServer{}
	s.configureAdminAuth(user, pass)
	return s
}

func TestRequireAdminAuth(t *testing.T) {
	const user, pass = "admin", "s3cr3t-pass"
	tests := []struct {
		name       string
		cfgUser    string
		cfgPass    string
		setAuth    bool
		user       string
		pass       string
		wantStatus int
	}{
		{"no creds configured -> fail-closed 503", "", "", true, user, pass, http.StatusServiceUnavailable},
		{"only user configured -> 503", user, "", true, user, pass, http.StatusServiceUnavailable},
		{"only pass configured -> 503", "", pass, true, user, pass, http.StatusServiceUnavailable},
		{"configured, no Authorization header -> 401", user, pass, false, "", "", http.StatusUnauthorized},
		{"configured, wrong user -> 401", user, pass, true, "nope", pass, http.StatusUnauthorized},
		{"configured, wrong pass -> 401", user, pass, true, user, "nope", http.StatusUnauthorized},
		{"configured, valid creds -> 200", user, pass, true, user, pass, http.StatusOK},
		// Regression for the asymmetric-trim lockout: surrounding whitespace in the
		// configured values is stripped, so the operator logs in with the clean value.
		{"configured creds whitespace-padded, clean login -> 200", "  " + user + "\n", " " + pass + " ", true, user, pass, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newAdminServer(tt.cfgUser, tt.cfgPass)
			h := s.requireAdminAuth(okHandler)
			r := httptest.NewRequest(http.MethodGet, "/admin", nil)
			if tt.setAuth {
				r.SetBasicAuth(tt.user, tt.pass)
			}
			w := httptest.NewRecorder()
			h(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			// A 401 must advertise the scheme so browsers show their login prompt.
			if tt.wantStatus == http.StatusUnauthorized {
				if got := w.Header().Get("WWW-Authenticate"); got == "" {
					t.Errorf("401 response missing WWW-Authenticate header")
				}
			}
		})
	}
}

// TestRequireAdminAuthCSRF covers the same-origin gate on state-changing methods:
// reject cross-origin browser mutations, allow same-origin and header-less clients.
func TestRequireAdminAuthCSRF(t *testing.T) {
	const user, pass = "admin", "s3cr3t-pass"
	const host = "blockbook-internal:9130"
	tests := []struct {
		name       string
		method     string
		tls        bool
		host       string
		origin     string
		referer    string
		wantStatus int
	}{
		// Safe methods are never subject to the origin check.
		{"GET cross-origin -> allowed", http.MethodGet, false, host, "http://evil.example", "", http.StatusOK},
		{"HEAD cross-origin -> allowed", http.MethodHead, false, host, "http://evil.example", "", http.StatusOK},
		// State-changing methods: same-origin passes, cross-origin is forbidden.
		{"POST same-origin -> allowed", http.MethodPost, false, host, "http://" + host, "", http.StatusOK},
		{"POST cross-origin -> forbidden", http.MethodPost, false, host, "http://evil.example", "", http.StatusForbidden},
		{"PUT cross-origin -> forbidden", http.MethodPut, false, host, "https://evil.example", "", http.StatusForbidden},
		{"DELETE cross-origin -> forbidden", http.MethodDelete, false, host, "http://evil.example", "", http.StatusForbidden},
		// Host comparison is case-insensitive.
		{"POST same-origin host case-insensitive -> allowed", http.MethodPost, false, host, "http://" + strings.ToUpper(host), "", http.StatusOK},
		// Opaque origin (sandboxed iframe / data: document) never matches.
		{"POST null origin -> forbidden", http.MethodPost, false, host, "null", "", http.StatusForbidden},
		// Origin is authoritative; a matching Origin passes even with a foreign Referer.
		{"POST origin match beats foreign referer -> allowed", http.MethodPost, false, host, "http://" + host, "http://evil.example/x", http.StatusOK},
		// Referer is the fallback only when Origin is absent.
		{"POST referer same-origin, no origin -> allowed", http.MethodPost, false, host, "", "http://" + host + "/admin", http.StatusOK},
		{"POST referer cross-origin, no origin -> forbidden", http.MethodPost, false, host, "", "http://evil.example/x", http.StatusForbidden},
		// Non-browser client: neither header present -> allowed (curl/automation).
		{"POST no origin/referer -> allowed", http.MethodPost, false, host, "", "", http.StatusOK},
		// A garbage Origin does not parse to our host -> forbidden.
		{"POST unparsable origin -> forbidden", http.MethodPost, false, host, "://::::", "", http.StatusForbidden},
		// TLS server: a plaintext (downgraded) same-host Origin is an on-path MITM vector.
		{"POST http origin to TLS server -> forbidden", http.MethodPost, true, host, "http://" + host, "", http.StatusForbidden},
		{"POST https origin to TLS server -> allowed", http.MethodPost, true, host, "https://" + host, "", http.StatusOK},
		// Plaintext server behind a TLS-terminating proxy: an https Origin still passes.
		{"POST https origin to plaintext server -> allowed", http.MethodPost, false, host, "https://" + host, "", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newAdminServer(user, pass)
			if tt.tls {
				s.certFiles = "testcert"
			}
			h := s.requireAdminAuth(okHandler)
			r := httptest.NewRequest(tt.method, "/admin/runtime-settings/ALLOWED_RPC_CALL_TO", nil)
			r.Host = tt.host
			r.SetBasicAuth(user, pass)
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			if tt.referer != "" {
				r.Header.Set("Referer", tt.referer)
			}
			w := httptest.NewRecorder()
			h(w, r)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func Test_urlPathSegment(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"/admin/contract-info/0xAbC123", "0xAbC123"}, // case preserved
		{"/admin/runtime-settings/allowed_rpc_call_to", "allowed_rpc_call_to"},
		{"/admin/contract-info/ 0xabc \t", "0xabc"}, // whitespace trimmed
		{"/admin/contract-info/", ""},               // collection path
		{"/admin", ""},                              // root-level path has no sub-segment
		{"/", ""},
		{"", ""},
	}
	for _, tt := range tests {
		r := httptest.NewRequest(http.MethodGet, "/x", nil)
		r.URL.Path = tt.path // bypass NewRequest path validation to test raw shapes
		if got := urlPathSegment(r); got != tt.want {
			t.Errorf("urlPathSegment(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

// TestAdminSubtreeIsGated guards against the ServeMux fall-through where
// trailing-slash or unknown /admin paths would otherwise reach the unauthenticated
// index handler. It mirrors the route registration in NewInternalServer.
func TestAdminSubtreeIsGated(t *testing.T) {
	s := newAdminServer("admin", "pass")
	mux := http.NewServeMux()
	// Unauthenticated index, as registered for "/".
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("INDEX"))
	})
	mux.HandleFunc("/admin", s.requireAdminAuth(okHandler))
	mux.HandleFunc("/admin/", s.requireAdminAuth(s.adminSubtreeHandler("/admin")))

	// Without credentials, every /admin path is gated (401) and never reaches the
	// unauthenticated index handler.
	for _, p := range []string{"/admin", "/admin/", "/admin/unknown", "/admin/contract-info", "/admin/runtime-settings/ALLOWED_RPC_CALL_TO"} {
		t.Run("no-auth "+p, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, p, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s: status = %d, want 401 (gated by admin auth)", p, w.Code)
			}
			if w.Body.String() == "INDEX" {
				t.Errorf("%s: leaked the unauthenticated index handler", p)
			}
		})
	}

	// With valid credentials, a bare /admin/ canonicalizes to /admin and an unknown
	// subpath is a gated 404 -- never the index.
	authed := []struct {
		path     string
		wantCode int
		wantLoc  string
	}{
		{"/admin/", http.StatusFound, "/admin"},
		{"/admin/unknown", http.StatusNotFound, ""},
	}
	for _, tc := range authed {
		t.Run("auth "+tc.path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.path, nil)
			r.SetBasicAuth("admin", "pass")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != tc.wantCode {
				t.Errorf("%s: status = %d, want %d", tc.path, w.Code, tc.wantCode)
			}
			if tc.wantLoc != "" && w.Header().Get("Location") != tc.wantLoc {
				t.Errorf("%s: Location = %q, want %q", tc.path, w.Header().Get("Location"), tc.wantLoc)
			}
			if w.Body.String() == "INDEX" {
				t.Errorf("%s: leaked the unauthenticated index handler", tc.path)
			}
		})
	}
}
