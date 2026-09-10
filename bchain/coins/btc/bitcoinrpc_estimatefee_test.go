package btc

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/trezor/blockbook/bchain"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// newLegacyFeeRPC wires a BitcoinRPC to an in-process backend (no listening socket)
// configured like dogecoind: only the legacy estimatefee is available.
func newLegacyFeeRPC(backend func(body string) string) *BitcoinRPC {
	cfg := &Configuration{SupportsEstimateFee: true}
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(backend(string(body)))),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	})
	return &BitcoinRPC{
		BaseChain:    &bchain.BaseChain{Parser: NewBitcoinParser(GetChainParams("main"), cfg)},
		client:       http.Client{Transport: transport},
		rpcURL:       "http://backend",
		ChainConfig:  cfg,
		RPCMarshaler: JSONMarshalerV1{},
	}
}

// Replies are what dogecoind 1.14.9 returned live on 2026-09-10.
func fakeLegacyEstimateFee(requests *[]string) func(string) string {
	return func(body string) string {
		*requests = append(*requests, body)
		switch body {
		case `{"method":"estimatefee","params":[1]}`:
			return `{"result":-1,"error":null,"id":1}`
		case `{"method":"estimatefee","params":[2]}`:
			return `{"result":0.50427075,"error":null,"id":1}`
		case `{"method":"estimatefee","params":[8]}`:
			return `{"result":0.01002515,"error":null,"id":1}`
		default:
			return `{"result":-1,"error":null,"id":1}`
		}
	}
}

func TestEstimateFeeLegacyRemapsOneBlock(t *testing.T) {
	var requests []string
	b := newLegacyFeeRPC(fakeLegacyEstimateFee(&requests))

	tests := []struct {
		name    string
		call    func() (fee string, err error)
		wantReq string
		want    string
	}{
		{"EstimateFee(1) asks for 2", func() (string, error) { f, e := b.EstimateFee(1); return f.String(), e },
			`{"method":"estimatefee","params":[2]}`, "50427075"},
		{"EstimateSmartFee(1) falls back to estimatefee 2", func() (string, error) { f, e := b.EstimateSmartFee(1, true); return f.String(), e },
			`{"method":"estimatefee","params":[2]}`, "50427075"},
		{"other targets pass through", func() (string, error) { f, e := b.EstimateFee(8); return f.String(), e },
			`{"method":"estimatefee","params":[8]}`, "1002515"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests = requests[:0]
			fee, err := tt.call()
			if err != nil {
				t.Fatal(err)
			}
			if fee != tt.want {
				t.Errorf("fee = %s, want %s", fee, tt.want)
			}
			if len(requests) != 1 || requests[0] != tt.wantReq {
				t.Errorf("requests = %q, want [%s]", requests, tt.wantReq)
			}
		})
	}
}

func TestEstimateFeeLegacyNegativeIsError(t *testing.T) {
	var requests []string
	b := newLegacyFeeRPC(fakeLegacyEstimateFee(&requests))

	if fee, err := b.EstimateFee(1008); err == nil {
		t.Errorf("expected error for result -1, got fee %s", fee.String())
	}
	if _, err := b.LongTermFeeRate(); err == nil {
		t.Error("expected error for result -1 from LongTermFeeRate")
	}
}
