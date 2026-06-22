//go:build unittest

package server

import (
	"net/http"
	"net/http/httptest"
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
	mux.HandleFunc("/admin/", s.requireAdminAuth(http.NotFound))

	// All of these must be gated by admin auth (401 without credentials), never
	// served by the unauthenticated index handler.
	for _, path := range []string{"/admin", "/admin/", "/admin/unknown", "/admin/contract-info"} {
		t.Run(path, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s: status = %d, want 401 (gated by admin auth)", path, w.Code)
			}
			if w.Body.String() == "INDEX" {
				t.Errorf("%s: leaked the unauthenticated index handler", path)
			}
		})
	}
}
