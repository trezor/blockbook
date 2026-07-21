# EVM transaction submission

This page documents how Blockbook broadcasts a **Trezor Suite** EVM transaction through the
private send-tx relay and tracks it in its own pending-transaction cache until the chain confirms
or supersedes it. Fee estimation — a separate step of the same send flow — is covered in
[fees.md](/docs/fees.md) and is not repeated here.

The private-relay pieces (the alternative send-tx provider and its pending-tx cache) are active
only when a coin is configured with the `*_ALTERNATIVE_SENDTX_URLS`, `*_ALTERNATIVE_SENDTX_ONLY`,
and `*_ALTERNATIVE_FETCH_MEMPOOL_TX` environment variables. Without them Blockbook sends through
the normal backend RPC and the cache path below is skipped.

## Broadcast and the pending-transaction cache

`SendRawTransaction` broadcasts and — when the relay accepts and the coin runs in
`ALTERNATIVE_SENDTX_ONLY` + `ALTERNATIVE_FETCH_MEMPOOL_TX` mode — keeps its own pending-tx cache,
because the relay exposes no mempool to reconcile against. A background loop and the read path
reconcile that cache against the chain.

```mermaid
%%{init: {"theme": "base", "themeVariables": {"lineColor": "#6b7280", "primaryTextColor": "#111827"}}}%%
flowchart TD
    send["SendRawTransaction(hex, disableAlternativeRPC)"]
    route{"provider configured<br/>and not disabled?"}
    primary["primary eth_sendRawTransaction"]
    relay["broadcast to relay URLs"]
    acc{"any URL accepted?"}
    reject["onlyAlternative: return error<br/>else: fall back to primary"]
    reg["registerSuccessfulSend<br/>(record sender + URL, assign gen)"]
    evict["evictReplacedByNonce<br/>retire same-(from,nonce) predecessor<br/>on ACK (RBF / cancel), gen-ordered"]
    handle["handleMempoolTransaction<br/>fetch-back → cache pending (gen-ordered)<br/>→ AddTransactionToMempool → notify"]

    subgraph rec ["reconcileMempoolTxs (every minute, per cached tx)"]
        mined["mined → evict"]
        super["nonce_superseded<br/>(confirmed nonce > tx nonce) → evict"]
        miss["provider_missing<br/>(relay stopped surfacing) → keep to timeout"]
        to["past cache timeout → evict"]
    end

    readpath["GetTransaction read path<br/>expired entry → evict"]
    remove[("removeMempoolTx<br/>clear cache + wrapped mempool<br/>+ release nonce routing<br/>metered once")]

    send --> route
    route -- "no" --> primary
    route -- "yes" --> relay --> acc
    acc -- "no" --> reject
    acc -- "yes" --> reg --> evict --> handle
    evict -. removes predecessor .-> remove
    handle -. caches new tx .-> readpath
    mined --> remove
    super --> remove
    to --> remove
    readpath --> remove

    classDef normal fill:#e7f0ff,stroke:#4078c0,color:#10243e;
    classDef store fill:#e8f7ed,stroke:#2e8b57,color:#0b2c19;
    classDef error fill:#ffecec,stroke:#c03535,color:#3b0a0a;
    class send,route,relay,acc,reg,evict,handle,mined,super,miss,to,readpath,primary normal;
    class remove store;
    class reject error;
```

Key invariants:

- **A same-`(from, nonce)` predecessor is retired the moment the relay ACKs its replacement**, from
  the raw hex — not by waiting for the relay to surface the replacement. A Blink drop-mode cancel
  is never surfaced and its nonce is never consumed on-chain, so without this the superseded tx
  lingered as "Unconfirmed" until the cache timeout. This is deliberately distinct from an *empty*
  `eth_getTransactionByHash` probe, which is **not** authoritative — a private relay stops
  surfacing a still-mineable tx while it stays broadcast, so `provider_missing` is kept until the
  timeout rather than evicted early. `mined` and `nonce_superseded` are the only deterministic
  early evictions the reconcile loop makes.
- **Send generations order concurrent same-nonce sends.** Each accepted send gets a monotonic
  generation; an older submission's slow fetch-back neither caches itself over, nor evicts, a
  newer replacement that already holds the nonce slot.
- **Every exit funnels through `removeMempoolTx`**, which clears the provider cache *and* the
  wrapped Blockbook mempool (address index), releases the sender's nonce routing once nothing
  private remains pending, and records the lifecycle metric exactly once (gated on the actual
  removal, so concurrent reconcile / read-path / RBF evictions of the same entry don't
  double-count).
- **Removals are not pushed to the wallet.** Blockbook pushes only *added* txs; a wallet learns a
  pending tx is gone on its next account re-fetch (the initiating device also removes it
  optimistically). The cache timeout is the backstop for anything the deterministic evictions miss.

## Wallet-declared `privatePending` hint (nonce + gas routing)

A private relay exposes no mempool, so a transaction pending only there is invisible to the public
backend RPC. Blockbook otherwise *infers* "this sender has a private tx in flight" from the
`recentSenders` map (populated by `registerSuccessfulSend`), which is fragile across restarts and
across load-balanced replicas without request affinity. A wallet already knows this state with
certainty, so it may **declare** it on its read requests via an optional `privatePending` field.
Blockbook then routes deterministically from that declaration instead of guessing.

The field appears in two places, matching the two consumers of the routing machinery:

- **`getAccountInfo` → top-level `privatePending: {nonces, txids}`** drives the pending-**nonce**
  lookup. A declared nonce *is* the answer: Blockbook routes the `eth_getTransactionCount` to the
  relay and reports at least `max(nonces) + 1`, so the wallet can never reuse the nonce of a private
  tx it has in flight — even for a tx this instance never cached (accepted by another replica, or
  lost to a restart). The declared floor only ever *raises* the reported nonce; it never lowers a
  higher provider/primary answer. The nonce list is capped (see `maxPrivatePendingNonces`) and, past
  the cap, collapsed to its single highest entry — only the maximum matters for the floor.
- **`estimateFee` → `specific.privatePending`** is a **routing signal only**. Unlike a nonce, the
  wallet cannot compute gas itself, so Blockbook still simulates the call — the declaration only says
  "estimate against the relay's pending-private state" (e.g. a privately-submitted `approve` a
  following swap's gas depends on). Presence of a non-empty `nonces` array is all that is read; the
  field is stripped before the `eth_estimateGas` call object is forwarded to the relay.

Only `nonces` drives behavior today; `txids` is accepted for forward compatibility (future
pending-tx correlation) and is not yet consumed on any path.

The hint is **additive and backward-compatible**: absent the field, behavior is exactly as before
(the `recentSenders` heuristic remains the fallback, and is still consulted when no hint is
declared), and an older Blockbook simply ignores the unknown field. With no alternative provider
configured the hint is a no-op (there is no private mempool to reconcile against).

**Trust boundary (accepted).** `privatePending` is an *unauthenticated client hint* — Blockbook does
not verify the caller owns the address or that a private tx actually exists. This is safe because the
declaration is per-request only: it is never written into `recentSenders` or the pending-tx cache, so
a hostile client can distort only its **own** request's answer and cannot poison another client's
view or any shared state. Its one outward effect is forcing the read to route to the relay; that is
bounded by the relay's own rate-limit quota and the per-connection pending-requests limit, and — by
design — does **not** re-introduce the #1629 hot-path quota drain, because a normal wallet declares
the field only when it genuinely has a private tx in flight (rare), not on every keystroke.

## Observability

Prometheus counters for the cache lifecycle:

- `blockbook_eth_alternative_mempool_reconciliation_events_total{action}` — cache exits by reason
  (`mined`, `nonce_superseded`, `provider_missing`, `timeout`, `rbf_replaced`, plus the kept
  actions `skipped_fresh`, `provider_missing_pending`, `kept`, `provider_error`).
- `blockbook_eth_alternative_mempool_tx_residence_seconds{action}` — how long an entry lived before
  each eviction reason fired (e.g. `provider_missing` clustering near the timeout rather than at
  ~1–2 min would flag a premature-eviction regression).
- `blockbook_eth_alternative_mempool_cache_size` — current cache depth.
