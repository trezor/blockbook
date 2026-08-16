package eth

import "testing"

// TestInitAlternativeFeeProviderFailFast verifies that when a coin config
// explicitly selects an EVM alternative fee provider whose required API-key env
// var is missing, initialization fails fast instead of silently reverting to
// default fee estimation. An unset provider stays a no-op.
func TestInitAlternativeFeeProviderFailFast(t *testing.T) {
	const infuraParams = `{"url":"https://gas.api.infura.io/v3/${api_key}/networks/1/suggestedGasFees","periodSeconds":60}`
	const oneInchParams = `{"url":"https://api.1inch.dev/gas-price/v1.5/1","periodSeconds":60}`

	tests := []struct {
		name        string
		feeProvider string
		params      string
		wantErr     bool
	}{
		{name: "infura without INFURA_API_KEY fails fast", feeProvider: "infura", params: infuraParams, wantErr: true},
		{name: "1inch without ONE_INCH_API_KEY fails fast", feeProvider: "1inch", params: oneInchParams, wantErr: true},
		{name: "no provider configured is a no-op", feeProvider: "", params: "", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Ensure the required env vars are unset for this test.
			t.Setenv("INFURA_API_KEY", "")
			t.Setenv("ONE_INCH_API_KEY", "")

			b := &EthereumRPC{ChainConfig: &Configuration{
				AlternativeEstimateFee:       tt.feeProvider,
				AlternativeEstimateFeeParams: tt.params,
			}}

			err := b.initAlternativeFeeProvider()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected initialization to fail fast, got nil error")
				}
				if b.alternativeFeeProvider != nil {
					t.Fatal("alternativeFeeProvider should be nil after a failed init")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
