package build

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateAdditionalParamsOverlayRejectsUndeclaredSetting(t *testing.T) {
	base := map[string]json.RawMessage{
		"alternative_estimate_fee_params": json.RawMessage(`"{\"periodSeconds\": 10}"`),
	}
	overlay := map[string]json.RawMessage{
		"alternative_estimate_fee_paarms": json.RawMessage(`{"periodSeconds": 300}`),
	}

	err := validateAdditionalParamsOverlay(base, overlay)
	if err == nil {
		t.Fatal("expected a setting missing from additional_params to be rejected")
	}
	if !strings.Contains(err.Error(), "alternative_estimate_fee_paarms") {
		t.Fatalf("expected the offending setting to be named, got %v", err)
	}

	if err := validateAdditionalParamsOverlay(base, nil); err != nil {
		t.Fatalf("validateAdditionalParamsOverlay(no overlay) error = %v", err)
	}
}

func TestMergeAdditionalParams(t *testing.T) {
	const feeParams = `"{\"url\": \"https://gas.api.infura.io/v3/${api_key}/networks/1/suggestedGasFees\", \"periodSeconds\": 10, \"staleSeconds\": 600}"`

	tests := []struct {
		name    string
		base    map[string]json.RawMessage
		overlay map[string]json.RawMessage
		want    map[string]string
	}{
		{
			name:    "no overlay keeps base",
			base:    map[string]json.RawMessage{"eip1559Fees": json.RawMessage(`true`)},
			overlay: nil,
			want:    map[string]string{"eip1559Fees": `true`},
		},
		{
			name:    "scalar is replaced",
			base:    map[string]json.RawMessage{"disableMempoolSync": json.RawMessage(`false`)},
			overlay: map[string]json.RawMessage{"disableMempoolSync": json.RawMessage(`true`)},
			want:    map[string]string{"disableMempoolSync": `true`},
		},
		{
			name:    "string encoded params merge field by field and stay a string",
			base:    map[string]json.RawMessage{"alternative_estimate_fee_params": json.RawMessage(feeParams)},
			overlay: map[string]json.RawMessage{"alternative_estimate_fee_params": json.RawMessage(`{"periodSeconds": 300}`)},
			want: map[string]string{
				"alternative_estimate_fee_params": `"{\"periodSeconds\":300,\"staleSeconds\":600,\"url\":\"https://gas.api.infura.io/v3/${api_key}/networks/1/suggestedGasFees\"}"`,
			},
		},
		{
			name:    "string encoded overlay is accepted too",
			base:    map[string]json.RawMessage{"fiat_rates_params": json.RawMessage(`"{\"coin\": \"ethereum\", \"periodSeconds\": 900}"`)},
			overlay: map[string]json.RawMessage{"fiat_rates_params": json.RawMessage(`"{\"periodSeconds\": 3600}"`)},
			want: map[string]string{
				"fiat_rates_params": `"{\"coin\":\"ethereum\",\"periodSeconds\":3600}"`,
			},
		},
		{
			name:    "overlay may add a field the base params omit",
			base:    map[string]json.RawMessage{"alternative_estimate_fee_params": json.RawMessage(`"{\"periodSeconds\": 10}"`)},
			overlay: map[string]json.RawMessage{"alternative_estimate_fee_params": json.RawMessage(`{"staleSeconds": 1800}`)},
			want: map[string]string{
				"alternative_estimate_fee_params": `"{\"periodSeconds\":10,\"staleSeconds\":1800}"`,
			},
		},
		{
			name:    "plain objects merge recursively",
			base:    map[string]json.RawMessage{"missingBlockRetry": json.RawMessage(`{"attempts": 3, "backoff": {"initialMs": 500, "maxMs": 5000}}`)},
			overlay: map[string]json.RawMessage{"missingBlockRetry": json.RawMessage(`{"backoff": {"maxMs": 60000}}`)},
			want: map[string]string{
				"missingBlockRetry": `{"attempts":3,"backoff":{"initialMs":500,"maxMs":60000}}`,
			},
		},
		{
			name:    "settings outside the overlay are untouched",
			base:    map[string]json.RawMessage{"eip1559Fees": json.RawMessage(`true`), "mempoolTxTimeoutHours": json.RawMessage(`48`)},
			overlay: map[string]json.RawMessage{"mempoolTxTimeoutHours": json.RawMessage(`2`)},
			want:    map[string]string{"eip1559Fees": `true`, "mempoolTxTimeoutHours": `2`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merged, err := mergeAdditionalParams(tt.base, tt.overlay)
			if err != nil {
				t.Fatalf("mergeAdditionalParams() error = %v", err)
			}
			if len(merged) != len(tt.want) {
				t.Fatalf("merged has %d settings, want %d: %v", len(merged), len(tt.want), merged)
			}
			for name, want := range tt.want {
				if got := string(merged[name]); got != want {
					t.Fatalf("merged[%q] = %s, want %s", name, got, want)
				}
			}
		})
	}
}

func TestMergeAdditionalParamsDoesNotMutateBase(t *testing.T) {
	const original = `"{\"url\": \"https://example.invalid\", \"periodSeconds\": 10}"`
	base := map[string]json.RawMessage{"alternative_estimate_fee_params": json.RawMessage(original)}
	overlay := map[string]json.RawMessage{"alternative_estimate_fee_params": json.RawMessage(`{"periodSeconds": 300}`)}

	if _, err := mergeAdditionalParams(base, overlay); err != nil {
		t.Fatalf("mergeAdditionalParams() error = %v", err)
	}
	if got := string(base["alternative_estimate_fee_params"]); got != original {
		t.Fatalf("base was mutated: %s", got)
	}
}

func TestDecodeJSONObjectReportsForm(t *testing.T) {
	tests := []struct {
		name          string
		raw           string
		wantOK        bool
		wantWasString bool
	}{
		{name: "object", raw: `{"a": 1}`, wantOK: true},
		{name: "string encoded object", raw: `"{\"a\": 1}"`, wantOK: true, wantWasString: true},
		{name: "plain string", raw: `"infura"`},
		{name: "number", raw: `42`},
		{name: "array", raw: `[1, 2]`},
		{name: "empty", raw: ``},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, wasString, ok := decodeJSONObject(json.RawMessage(tt.raw))
			if ok != tt.wantOK {
				t.Fatalf("decodeJSONObject(%s) ok = %v, want %v", tt.raw, ok, tt.wantOK)
			}
			if wasString != tt.wantWasString {
				t.Fatalf("decodeJSONObject(%s) wasString = %v, want %v", tt.raw, wasString, tt.wantWasString)
			}
		})
	}
}
