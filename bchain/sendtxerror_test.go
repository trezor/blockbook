package bchain

import (
	"errors"
	"testing"
)

// TestClassifySendTxError pins the classes an operator alerts on: caller-side rejections must
// never be reported as backend trouble and vice versa.
func TestClassifySendTxError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil error is success", nil, ReasonOK},
		{"geth already known", errors.New("already known"), ReasonAlreadyKnown},
		{"bitcoind already in chain", errors.New("-27: transaction already in block chain"), ReasonAlreadyKnown},
		{"geth nonce too low", errors.New("nonce too low: next nonce 5, tx nonce 3"), ReasonNonceTooLow},
		{"geth nonce too high", errors.New("nonce too high: next nonce 5, tx nonce 9"), ReasonNonceTooHigh},
		// the replacement class must win over the plain underpriced fragment it contains
		{"replacement underpriced", errors.New("replacement transaction underpriced"), ReasonReplacementUnderpriced},
		{"geth underpriced", errors.New("transaction underpriced"), ReasonUnderpriced},
		{"bitcoind min relay fee", errors.New("-26: min relay fee not met, 100 < 141"), ReasonUnderpriced},
		{"geth insufficient funds", errors.New("insufficient funds for gas * price + value"), ReasonInsufficientFunds},
		{"geth intrinsic gas", errors.New("intrinsic gas too low: have 1000, want 21000"), ReasonIntrinsicGas},
		{"bitcoind missing inputs", errors.New("-25: bad-txns-inputs-missingorspent"), ReasonMissingInputs},
		{"bitcoind mempool conflict", errors.New("-26: txn-mempool-conflict"), ReasonConflict},
		{"rlp decode failure", errors.New("rlp: expected input list for types.LegacyTx"), ReasonInvalidTransaction},
		// the provider URL travels inside the message, so a fragment must not match a path in it
		{"provider timeout with rlp in the URL", errors.New(`Post "https://relay.example.com/rlp-fast/KEY": context deadline exceeded`), ReasonTimeout},
		{"provider rate limit", errors.New("429 Too Many Requests: rate exceeded"), ReasonRateLimited},
		{"rpc timeout", errors.New("context deadline exceeded"), ReasonTimeout},
		{"provider down", errors.New("dial tcp 127.0.0.1:1: connect: connection refused"), ReasonUnavailable},
		// a failure carrying only an HTTP status is an outage, not a classifier gap
		{"revoked provider key", errors.New(`403 Forbidden: {"error":"invalid api key"}`), ReasonUnauthorized},
		{"provider rejects the credentials", errors.New("401 Unauthorized"), ReasonUnauthorized},
		{"provider internal error", errors.New("500 Internal Server Error: upstream failure"), ReasonUnavailable},
		// a proxy is free to replace the standard reason phrase, so the code alone has to carry it
		{"nonstandard status text", errors.New("503 Backend Fetch Failed"), ReasonUnavailable},
		{"cloudflare origin timeout", errors.New("524 origin took too long"), ReasonTimeout},
		// the backend's own wording outranks the transport status
		{"status with a caller-side body", errors.New("400 Bad Request: nonce too low"), ReasonNonceTooLow},
		// request-dependent statuses stay unclassified on purpose
		{"not found stays unclassified", errors.New("404 Not Found"), ReasonOther},
		{"bitcoind error code is not a status", errors.New("-26: bad-txns-nonstandard"), ReasonOther},
		{"unknown message falls through", errors.New("something nobody has classified yet"), ReasonOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifySendTxError(tt.err); got != tt.want {
				t.Errorf("ClassifySendTxError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}
