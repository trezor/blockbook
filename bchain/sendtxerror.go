package bchain

import (
	"strconv"
	"strings"
)

// Reason labels for a transaction broadcast outcome. The caller-side classes describe a
// transaction refused on its own merits, the backend-side ones (rate_limited, unauthorized,
// timeout, unavailable) mean the send never got a verdict and are the ones an operator should
// alert on.
const (
	ReasonOK                     = "ok"
	ReasonAlreadyKnown           = "already_known"
	ReasonNonceTooLow            = "nonce_too_low"
	ReasonNonceTooHigh           = "nonce_too_high"
	ReasonUnderpriced            = "underpriced"
	ReasonReplacementUnderpriced = "replacement_underpriced"
	ReasonFeeCap                 = "fee_cap_exceeded"
	ReasonInsufficientFunds      = "insufficient_funds"
	ReasonGasLimit               = "gas_limit"
	ReasonIntrinsicGas           = "intrinsic_gas_too_low"
	ReasonMissingInputs          = "missing_inputs"
	ReasonConflict               = "conflict"
	ReasonNonFinal               = "non_final"
	ReasonInvalidTransaction     = "invalid_transaction"
	ReasonRateLimited            = "rate_limited"
	ReasonUnauthorized           = "unauthorized"
	ReasonTimeout                = "timeout"
	ReasonUnavailable            = "unavailable"
	ReasonOther                  = "other"
)

// Status labels for a transaction broadcast outcome. success/failure is the domain the rest of the
// metric registry's status labels use.
const (
	SendTxStatusSuccess = "success"
	SendTxStatusFailure = "failure"
)

// sendTxErrorClasses maps a fragment of a backend rejection message to the reason label reported
// for a failed transaction broadcast. The table is deliberately backend-agnostic - EVM nodes,
// private relays and the bitcoind family all reject with free-form strings, and the resulting
// label space is the same either way. Order matters, the first match wins, so more specific
// patterns come before the general ones they contain.
var sendTxErrorClasses = []struct {
	fragment string
	class    string
}{
	// the transaction is already in the mempool or on chain - a retry, not a failure of ours
	{"already known", ReasonAlreadyKnown},
	{"already in block chain", ReasonAlreadyKnown},
	{"txn-already-in-mempool", ReasonAlreadyKnown},
	{"txn-already-known", ReasonAlreadyKnown},
	{"transaction already exists", ReasonAlreadyKnown},
	// nonce and fee rejections - the wallet built the transaction from a stale view
	{"nonce too low", ReasonNonceTooLow},
	{"nonce too high", ReasonNonceTooHigh},
	{"replacement transaction underpriced", ReasonReplacementUnderpriced},
	{"underpriced", ReasonUnderpriced},
	{"min relay fee not met", ReasonUnderpriced},
	{"mempool min fee not met", ReasonUnderpriced},
	{"insufficient fee", ReasonUnderpriced},
	{"fee too low", ReasonUnderpriced},
	{"tx fee", ReasonFeeCap},
	{"insufficient funds", ReasonInsufficientFunds},
	// the transaction can never be mined as built
	{"exceeds block gas limit", ReasonGasLimit},
	{"intrinsic gas too low", ReasonIntrinsicGas},
	{"missingorspent", ReasonMissingInputs},
	{"txn-mempool-conflict", ReasonConflict},
	{"non-final", ReasonNonFinal},
	{"non-bip68-final", ReasonNonFinal},
	{"oversized data", ReasonInvalidTransaction},
	{"invalid sender", ReasonInvalidTransaction},
	{"transaction type not supported", ReasonInvalidTransaction},
	// anchored on the colon geth's rlp errors always carry: a bare "rlp" is short enough to hit a
	// provider URL inside the message, which would report every outage of that deployment as a
	// caller-side rejection
	{"rlp: ", ReasonInvalidTransaction},
	{"invalid transaction", ReasonInvalidTransaction},
	// the backend or provider itself is the problem - these are the classes worth alerting on
	{"unauthorized", ReasonUnauthorized},
	{"forbidden", ReasonUnauthorized},
	{"too many requests", ReasonRateLimited},
	{"rate limit", ReasonRateLimited},
	{"context deadline exceeded", ReasonTimeout},
	{"timeout", ReasonTimeout},
	{"timed out", ReasonTimeout},
	{"connection refused", ReasonUnavailable},
	{"connection reset", ReasonUnavailable},
	{"no such host", ReasonUnavailable},
	{"eof", ReasonUnavailable},
	{"bad gateway", ReasonUnavailable},
	{"service unavailable", ReasonUnavailable},
	{"internal server error", ReasonUnavailable},
}

// httpStatusReason classifies a failure that carries nothing but an HTTP status. geth renders
// rpc.HTTPError as "<code> <text>[: <body>]", so a revoked key or a failing provider arrives with
// no backend wording to match on, and a proxy in front of the provider is free to replace the
// standard reason phrase the table above looks for. Without this such a failure counts as "other",
// which the dashboards read as a gap in the classifier rather than as the broadcast outage it is.
// Codes whose meaning depends on the request (400, 404) are left to "other" on purpose.
func httpStatusReason(msg string) (string, bool) {
	// the status is the start of the message, which keeps a number inside a rejection body from
	// being read as one
	if len(msg) < 3 || (len(msg) > 3 && msg[3] != ' ' && msg[3] != ':') {
		return "", false
	}
	code, err := strconv.Atoi(msg[:3])
	if err != nil || code < 100 || code > 599 {
		return "", false
	}
	switch {
	case code == 401 || code == 402 || code == 403 || code == 407:
		return ReasonUnauthorized, true
	case code == 408 || code == 504 || code == 524: // 524 is Cloudflare's origin timeout
		return ReasonTimeout, true
	case code == 429:
		return ReasonRateLimited, true
	case code >= 500:
		return ReasonUnavailable, true
	}
	return "", false
}

// SendTxStatus is the status label of a broadcast outcome.
func SendTxStatus(err error) string {
	if err != nil {
		return SendTxStatusFailure
	}
	return SendTxStatusSuccess
}

// ClassifySendTxError reduces a backend rejection to a bounded metric label. It exists because a
// bare success/failure ratio cannot tell a wallet-side mistake (underpriced, nonce_too_low) from
// backend trouble (timeout, rate_limited) - only the class distinguishes the two, and they call
// for opposite responses. A message carrying only an HTTP status is classified from the status
// itself, so a dead provider cannot hide in "other". Everything still unmatched falls into
// "other", which is the signal to extend the table rather than to alert.
func ClassifySendTxError(err error) string {
	if err == nil {
		return ReasonOK
	}
	msg := strings.ToLower(err.Error())
	for _, c := range sendTxErrorClasses {
		if strings.Contains(msg, c.fragment) {
			return c.class
		}
	}
	// after the table, so the backend's own wording wins: a 400 whose body says "nonce too low"
	// is a caller-side rejection, not a provider failure
	if class, found := httpStatusReason(msg); found {
		return class
	}
	return ReasonOther
}
