package bchain

import "strings"

// Reason labels for a transaction broadcast outcome. The caller-side classes describe a
// transaction refused on its own merits, the backend-side ones (rate_limited, timeout,
// unavailable) mean the send never got a verdict and are the ones an operator should alert on.
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
	{"rlp", ReasonInvalidTransaction},
	{"invalid transaction", ReasonInvalidTransaction},
	// the backend or provider itself is the problem - these are the classes worth alerting on
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
// for opposite responses. Unmatched messages fall into "other", which is the signal to extend
// the table rather than to alert.
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
	return ReasonOther
}
