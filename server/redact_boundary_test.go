//go:build unittest

package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/trezor/blockbook/api"
)

// A send rejected by every alternative provider reaches the REST client as a public APIError whose
// text is the provider error, and that error embeds the provider URL (our own annotations, or the
// http client's Post "<url>": ... on a dial failure). The handler must reduce it to the host before
// it leaves the process, or a key in the provider URL is served to anyone who can POST /api/v2/sendtx.
func TestJSONHandlerRedactsProviderURLFromClientError(t *testing.T) {
	const key = "SECRET_API_KEY_ABCDEF"
	providerErr := `Post "https://relay.example.com/v3/` + key + `": context deadline exceeded`

	tests := []struct {
		name  string
		err   error
		debug bool
		// status the client gets, to pin that redaction did not change the error's visibility
		wantStatus int
	}{
		{"public api error", api.NewAPIError(providerErr, true), false, http.StatusBadRequest},
		{"internal api error", api.NewAPIError(providerErr, false), false, http.StatusInternalServerError},
		{"plain error in debug mode", &nonAPIError{providerErr}, true, http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &htmlTemplates[TemplateData]{debug: tt.debug}
			handler := h.jsonHandler(func(r *http.Request, apiVersion int) (interface{}, error) {
				return nil, tt.err
			}, apiV2)

			w := httptest.NewRecorder()
			handler(w, httptest.NewRequest(http.MethodPost, "/api/v2/sendtx/", nil))

			body := w.Body.String()
			if strings.Contains(body, key) {
				t.Errorf("api key leaked to the client: %s", body)
			}
			if !strings.Contains(body, "relay.example.com") {
				t.Errorf("provider host should survive redaction, got: %s", body)
			}
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatus)
			}
			var decoded struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &decoded); err != nil {
				t.Fatalf("response is not the usual json error: %v, body %s", err, body)
			}
			if !strings.Contains(decoded.Error, "context deadline exceeded") {
				t.Errorf("backend wording must survive for client-side classification, got %q", decoded.Error)
			}
		})
	}
}

// nonAPIError is an error that is not an *api.APIError, taking the handler's debug branch.
type nonAPIError struct {
	text string
}

func (e *nonAPIError) Error() string {
	return e.text
}
