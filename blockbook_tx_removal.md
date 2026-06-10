# Blockbook Transaction Removal Decision Report

Related PR: [#1500 Decrease mempool timeout for MEV-protected coins](https://github.com/trezor/blockbook/pull/1500)

## Background

The original problem was observed with EVM transactions submitted through a private or MEV-protected alternative send
provider, such as Blinklabs. These providers can drop an unmined transaction quickly, for example after roughly 5
minutes when the fee is too low. Blockbook, however, historically kept EVM mempool transactions for much longer
periods, commonly 12 to 48 hours depending on the coin configuration.

That mismatch caused a user-facing stale pending transaction: the private provider no longer had the transaction, but
Blockbook could continue serving it from its local mempool state until the long Blockbook timeout expired.

One possible solution was to add a way to remove a transaction by `txid` from Blockbook. We decided not to implement
that as the primary fix.

## Why We Did Not Add Tx Removal By Txid

Blockbook's mempool is derived state. It reflects what Blockbook learned from the backend node, subscriptions, resyncs,
and the alternative send provider cache. It is not an authoritative source of truth for whether an EVM transaction is
globally valid, dropped, replaced, or still able to be mined.

Adding a removal endpoint would introduce a new write path into this derived state, and that has several problems:

1. A dropped private-provider transaction is not a consensus fact.

   A low-fee transaction may disappear from one private relay and still be valid. It can reappear through another
   provider, be rebroadcast, become visible in the public mempool later, or still be mined if conditions change.
   Deleting it by command would make Blockbook assert stronger knowledge than it actually has.

2. Removal can be immediately undone by normal Blockbook flows.

   Blockbook can add transactions during backend mempool resync, WebSocket or ZMQ notifications, alternative-provider
   fetches, or ordinary transaction lookup paths. If the backend still reports the transaction, a manual deletion from
   Blockbook can be reintroduced later. That makes an endpoint easy to misunderstand operationally.

   This point is weaker for EVM deployments where `disableMempoolSync` is enabled, for example when the backend provider
   does not support reliable pending transaction subscriptions. In that setup Blockbook does not subscribe to
   `newPendingTransactions`, and local pending state is mostly created by transactions submitted through Blockbook
   itself or through the alternative provider cache. The deletion would therefore be less likely to be undone by backend
   mempool sync, but it would still be a manual mutation of local derived state rather than proof that the transaction is
   invalid or permanently dropped.

3. Authorization is unclear and risky.

   A public or weakly protected endpoint that removes arbitrary `txid`s would let callers hide pending transactions from
   API and WebSocket users. A private admin endpoint would still need authentication, authorization, audit logging,
   rate limits, deployment controls, and a clear answer to who is allowed to decide that a transaction should disappear.
   A `txid` alone does not prove ownership or provider authority.

4. It creates consistency problems across instances.

   Blockbook deployments can run multiple instances. A local removal affects only one process unless additional
   propagation is designed. Users could then see different pending state depending on which instance serves the request.

5. It increases race and indexing complexity.

   Ethereum mempool entries are indexed both by transaction id and by address descriptors. Blockbook already has
   internal removal helpers that maintain those indexes, but exposing them externally would add races with concurrent
   add/resync paths and would need careful behavior around mined transactions, replacements, reorgs, and duplicate
   notifications.

6. It is the wrong operational primitive for this incident.

   The stale transaction was not caused by a need for manual intervention. It was caused by a timeout mismatch between
   private-provider retention and Blockbook retention. A deletion endpoint would treat the symptom one transaction at a
   time, while leaving the default behavior wrong for the next transaction.

## Why PR #1500 Was The Better Alternative

PR [#1500](https://github.com/trezor/blockbook/pull/1500) changed the timeout model instead of adding a transaction
removal API.

The important behavior is:

- When `AlternativeSendTxProvider` is enabled, Blockbook uses a short EVM mempool timeout by default: 10 minutes.
- The alternative-provider transaction cache uses a shorter default timeout: 5 minutes.
- Both values can be overridden by coin config with `mempoolTxTimeout` and `alternativeMempoolTxTimeout`.
- When no alternative send provider is enabled, Blockbook keeps the legacy `mempoolTxTimeoutHours` behavior.

This matches the actual failure mode: private relays expire pending transactions quickly, so Blockbook should not keep
private-provider pending state for hour-scale public mempool periods.

The PR avoids the main risks of tx removal:

- No new public or admin mutation endpoint.
- No new authorization model.
- No per-transaction manual intervention.
- No cross-instance command propagation problem.
- Deterministic cleanup based on configuration.
- Legacy behavior stays unchanged for non-MEV, public-backend-only deployments.

For EVM coins using providers such as QuickNode with `disableMempoolSync` enabled, the same alternative still applies:
the stale transaction is local state created from Blockbook's own send path or the alternative-provider cache. A bounded
timeout removes that state consistently without adding a new mutation API.

## Why The Timeouts Are 5 And 10 Minutes

The alternative-provider cache is scoped to transactions submitted through the alternative provider. Its default timeout
is 5 minutes, matching the expected private relay drop behavior.

The Blockbook mempool timeout is 10 minutes when the alternative provider is enabled. That gives a small buffer for
provider delays, local clock differences, backend lag, and normal async processing, while still preventing transactions
from hanging for 12 to 48 hours.

This is intentionally not zero or immediate removal. Immediate deletion would make short provider hiccups visible to
users as disappearing pending transactions. A short bounded timeout is a more stable user experience.

## Remaining Tradeoffs

This approach can still show a dropped private transaction for up to the configured Blockbook timeout, normally 10
minutes. That is an accepted tradeoff because it avoids the much worse hour-scale stale state without introducing a new
state mutation API.

The PR also does not rebroadcast transactions. Rebroadcasting would require retaining raw signed transaction data and
would introduce separate privacy, fee, and replacement risks. If a transaction is underpriced and dropped by the private
provider, the safer wallet-level recovery is usually to replace or resubmit it with an appropriate fee.

## Conclusion

We did not add tx removal by `txid` because it would be a manual mutation of derived mempool state with unclear
authority, weak consistency guarantees, and meaningful abuse potential. The actual problem was stale retention for
MEV-protected/private-provider transactions.

PR [#1500](https://github.com/trezor/blockbook/pull/1500) solves that problem by aligning Blockbook's local retention
with the alternative provider's short retention model, while keeping the public-backend legacy behavior intact.

## Follow-up: Alternative Provider Status Checks

Blink-style providers expose `eth_getTransactionByHash` for transactions they still know from chain, mempool, or their
own private-provider data. A follow-up improvement can use that endpoint for transactions submitted through the
alternative provider and cached locally by Blockbook.

This is safer than a generic tx-removal endpoint because Blockbook is not accepting an external delete command. It is
asking the same provider that accepted the private transaction whether the transaction is still known. If the provider
returns an empty result, Blockbook can remove the local pending entry as stale. If the provider reports the transaction
as mined, Blockbook can also remove the pending entry. Network or provider errors should not remove anything.

This keeps the bounded-timeout behavior from PR #1500 as the fallback, while allowing faster cleanup when the private
provider explicitly no longer knows the transaction.
