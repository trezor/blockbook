# Sync

The sync engine connects blocks from the backend RPC into the local RocksDB index. It is driven by external block notifications (EVM `newHeads` WebSocket, Tron ZeroMQ, BTC ZMQ) and an internal periodic tick. This page documents the loop, the [tip feed](#tip-feed-and-the-stall-watchdog) that drives it, and the knobs that govern how it recovers from transient backend trouble.

## Sync loop

```mermaid
%%{init: {"theme": "base", "themeVariables": {"lineColor": "#6b7280", "primaryTextColor": "#111827"}}}%%
flowchart TD
    trigger["Notifications or periodic tick"]
    debounce["TickAndDebounce"]
    loop["syncIndexLoop<br/>retry once after 2.5 s on error"]
    resync["ResyncIndex / resyncIndex"]
    done["syncNotNeeded<br/>(no work)"]
    fork["fork detected<br/>handleFork + DisconnectBlocks"]
    mode{"connect mode"}
    bulk["BulkConnectBlocks<br/>(large initial range)"]
    parallel["ParallelConnectBlocks"]
    sequential["connectBlocks + getBlockChain"]
    fetch["per-block fetch/retry<br/>(see below)"]
    connected["blocks connected"]
    recover["errResync / errFork<br/>restart resyncIndex"]
    failed["terminal error<br/>returns to syncIndexLoop"]

    trigger --> debounce --> loop --> resync
    resync --> done
    resync --> fork --> resync
    resync --> mode
    mode -- "initial sync" --> bulk
    mode -- "EVM gap" --> parallel
    mode -- "tail" --> sequential
    bulk --> fetch
    parallel --> fetch
    sequential --> fetch
    fetch -- OK --> connected
    fetch -- "chain changed" --> recover --> resync
    fetch -- "terminal error" --> failed --> loop

    classDef normal fill:#e7f0ff,stroke:#4078c0,color:#10243e;
    classDef error fill:#ffecec,stroke:#c03535,color:#3b0a0a;

    class trigger,debounce,loop,resync,done,fork,mode,bulk,parallel,sequential,fetch,connected,recover normal;
    class failed error;
```

The per-block retry loop is shared by `getBlockChain` and `getBlockWorker`. Probe errors are path-specific: `getBlockChain` propagates immediately, while workers retry until three consecutive probe failures.

```mermaid
%%{init: {"theme": "base", "themeVariables": {"actorBkg": "#e7f0ff", "actorBorder": "#4078c0", "actorTextColor": "#10243e", "activationBkgColor": "#e8f7ed", "activationBorderColor": "#2e8b57", "signalColor": "#6b7280", "signalTextColor": "#111827", "labelBoxBkgColor": "#fff6d7", "labelBoxBorderColor": "#b58400", "loopTextColor": "#312300", "noteBkgColor": "#f1ecff", "noteBorderColor": "#7a55c2"}}}%%
sequenceDiagram
    participant Fetch as getBlockChain/getBlockWorker
    participant RPC as backend RPC
    participant Probe as chain-state probe
    participant DB as RocksDB

    Fetch->>RPC: GetBlock(hash, height)
    alt OK
        RPC-->>Fetch: block
        Fetch->>DB: ConnectBlock
    else non-retryable error
        Fetch-->>Fetch: propagate, except worker mid-queue retries
    else retryable error
        Fetch-->>Fetch: onRetryableMiss and increment retries
        opt threshold reached
            Fetch->>Probe: shouldRestartSyncOnMissingBlock
            alt restart=true
                Probe-->>Fetch: errResync
            else restart=false
                Probe-->>Fetch: keep retrying
            else probe error
                Probe-->>Fetch: getBlockChain propagates, worker after 3 failures
            end
        end
        opt MaxStallDuration exceeded
            Fetch-->>Fetch: errResync
        end
        Fetch-->>Fetch: sleep RetryDelay, then retry GetBlock
    end
```

`errResync` and `errFork` cause `resyncIndex` to be re-entered (handling the new chain state); any other error propagates up and `syncIndexLoop` retries once before waiting for the next trigger.

## Tip feed and the stall watchdog

The "Notifications" that wake the loop above are not free-standing — for EVM and Tron they come from a single cached best-header that is advanced **only** by a backend push feed (EVM `newHeads` WebSocket, Tron ZeroMQ). `resyncIndex` reads that cached tip to decide whether work is needed, so a feed that goes quiet freezes the tip and the loop silently concludes `syncNotNeeded`.

The failure mode that motivated this is a load balancer that drops the upstream **without** signalling `sub.Err()`: the error-driven resubscribe never fires, the cached tip freezes, and the index stalls until the ~15-minute periodic tick — with no error logged and no metric moving. The `tipWatchdog` closes that gap by watching feed liveness directly and healing a silent feed.

```mermaid
%%{init: {"theme": "base", "themeVariables": {"lineColor": "#6b7280", "primaryTextColor": "#111827"}}}%%
flowchart TD
    feed["Backend push feed<br/>EVM newHeads WS · Tron ZeroMQ"]
    notifier["feed reader<br/>(newHeads / ZeroMQ)"]
    advance["advance cached tip<br/>EVM: setBestHeader(feed header)<br/>Tron: refreshBestHeaderFromChain"]
    mark["markSubscriptionAlive<br/>(EVM: only on tip advance)"]
    push["PushHandler(NotificationNewBlock)"]
    trigger["chanSyncIndex → syncIndexLoop<br/>(sync loop above)"]
    ts[("lastSubNotifyNs<br/>liveness timestamp")]
    cache[("cached bestHeader<br/>read by resyncIndex → synced?")]

    feed -- notification --> notifier
    notifier --> advance
    advance -. advances .-> cache
    advance -- "tip advanced" --> mark
    mark -. stamps .-> ts
    advance -- "tip advanced" --> push --> trigger

    subgraph wd ["tipWatchdog (EVM + Tron, started once)"]
        tick["tick every<br/>clamp(threshold/3, 5s, 60s)"]
        check{"age ≥ TipStaleThreshold?<br/>clamp(30 × blockTime, 30s, 5min)"}
        ok["set age gauge · feed alive<br/>return"]
        stall["watchdog_stall<br/>feed silent, sub.Err() never fired"]
        wpoll["poll tip directly<br/>refreshBestHeaderFromChain"]
        wadv["watchdog_tip_advanced"]
        wrec["EVM: reconnectRPC<br/>watchdog_reconnect / _failed<br/>Tron: poll-only (ZeroMQ self-heals)"]
    end

    ts -. read by .-> check
    tick --> check
    check -- "no (fresh)" --> ok
    check -- "yes (silent)" --> stall --> wpoll
    wpoll -. advances .-> cache
    wpoll -- "tip advanced" --> wadv --> push
    wpoll --> wrec
    wrec -. "re-arm window<br/>+ restore push" .-> feed

    classDef normal fill:#e7f0ff,stroke:#4078c0,color:#10243e;
    classDef store fill:#e8f7ed,stroke:#2e8b57,color:#0b2c19;
    classDef watch fill:#fff6d7,stroke:#b58400,color:#312300;

    class feed,notifier,mark,advance,push,trigger,tick,check,ok normal;
    class ts,cache store;
    class stall,wpoll,wadv,wrec watch;
```

Key invariants this design relies on:

- **The cached tip is advanced from the feed's own header, not re-derived over the load-balanced call path (EVM).** `newHeads` (WS) is sticky to one upstream, but JSON-RPC calls (`HeaderByNumber`, `GetBlock`) are load-balanced across the pool and can land on a lagging node. Re-querying the tip over that path could read a stale height and silently freeze sync into a false "synced" even while `newHeads` keeps flowing; instead the header delivered by the feed sets the tip directly via `setBestHeader`, which is **monotonic** so a resubscribe onto a slightly-behind node cannot regress the tip and trip a spurious fork. Blocks the call path cannot yet serve surface as ordinary `ErrBlockNotFound` and are absorbed by the [retry budget](#troubleshooting) — visible via `block_not_found_retries` / `sync_yields` — rather than hidden as a frozen tip. (Tron's ZeroMQ notification carries no header, so it still re-queries via `refreshBestHeaderFromChain`.)
- **The liveness timestamp is armed when the subscription is established and refreshed only by a feed-driven tip advance.** `markSubscriptionAlive` (EVM) / `markNotifyAlive` (Tron) is stamped on the push-feed path, never by the watchdog's own fallback poll — so a watchdog that is carrying sync can never mask a dead feed: `age` keeps growing until real push delivery resumes. On EVM it is refreshed **only when the delivered header actually moved the tip forward**, so the watchdog also catches a feed that keeps delivering but is stuck on one height (a load-balancer upstream that stopped advancing), not just one that went fully silent. Crucially, EVM also stamps it **once at subscribe time** (`subscribeEvents`): the watchdog's `lastSubNotifyNs == 0` gate uses it as a proxy for "subscription wired up", so if it were stamped *only* on advance, a subscription that comes up silently behind a load balancer (never advancing) would leave the gate closed and the watchdog disabled forever — the cached tip would freeze into a false "synced" with no error or metric. Seeding at subscribe time means even a born-silent feed ages past the threshold and gets polled/reconnected. The watchdog re-arms the window after a *successful* EVM reconnect (and Tron re-arms after each poll to avoid polling every tick during a legitimate lull).
- **`TipStaleThreshold` is chain-aware.** `clamp(30 × averageBlockTimeMs, 30s, 5min)` replaces the old fixed 15 minutes, which on Polygon's 2 s blocks meant ~450 missed blocks before any reaction. Per-chain values: Polygon/Optimism/Base/Avalanche 60 s, BSC/Tron 90 s, Arbitrum 30 s (floor), Ethereum 5 min (cap). The sample interval is `clamp(threshold/3, 5s, 60s)`.
- **Reader goroutines start once.** `newBlockNotifier`, `tipWatchdog`, and the `NewBlock`/`NewTx` channel readers are launched under a `sync.Once`; `reconnectRPC` only re-creates the `EthSubscribe`-bound subscriptions, so a reconnect no longer leaks a fresh reader set. `getBestHeader` no longer does a lock-held passive reconnect — liveness is owned by the watchdog, off the `bestHeaderLock`, so a reconnect can't block concurrent tip readers.

EVM coverage is inherited by every coin built on `EthereumRPC` (Ethereum, Polygon, BSC, Arbitrum, Optimism, Base, Avalanche); Tron runs the same watchdog poll-only over its ZeroMQ feed. BTC-family coins do not use this cached-tip feed and are unaffected.

## Troubleshooting

The retry policy is exposed per chain under `additional_params.missingBlockRetry` in `configs/coins/*.json`. Each field is optional; missing or `<= 0` values fall back to the built-in defaults below.

| Knob                  | Current default | Where it bites                                                                  | Semantic                                                              |
| --------------------- | --------------- | ------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| `RetryDelay`          | 1 s             | `getBlockWorker` (parallel) directly; `getBlockChain` clamps to ≤ 250 ms regardless | Sleep between successive `GetBlock` attempts for the same missing block |
| `RecheckThreshold`    | 10              | `getBlockWorker` mid-queue                                                      | Retries before calling `shouldRestartSyncOnMissingBlock`              |
| `TipRecheckThreshold` | 3               | both loops, at the tail                                                         | Retries before chain-state probe, when we're near the tip             |
| `MaxStallDuration`    | 60 s            | both loops                                                                      | Wall-clock cap before yielding `errResync`                            |

Example override (JSON keys are camelCase with the `Ms` suffix for durations):

```json
"additional_params": {
    "missingBlockRetry": {
        "retryDelayMs": 1000,
        "recheckThreshold": 10,
        "tipRecheckThreshold": 3,
        "maxStallMs": 60000
    }
}
```

When an override is applied, blockbook logs one `sync: missingBlockRetry override applied: …` line at startup so you can confirm the effective values.

Related Prometheus counters for observing the budget at runtime:

- `blockbook_index_block_not_found_retries` — every transient `ErrBlockNotFound` observed during sync.
- `blockbook_index_sync_yields{reason="deadline"|"probe_failed"}` — wall-clock cap fired vs chain-state probe failed three times.
- `blockbook_index_reorg_events{type="fork"|"resync"|"disconnect"}` — real reorg signals (not stall yields).

For the [tip feed](#tip-feed-and-the-stall-watchdog) (EVM and Tron only):

- `blockbook_backend_subscription_age_seconds` — seconds since the feed last delivered a notification. Healthy: hovers near the chain's block time. A sustained climb to `TipStaleThreshold` (the value `clamp(30 × blockTime, 30s, 5min)` from the watchdog section) means the feed went silent and the watchdog is carrying sync; climbing without bound means the backend is unreachable.
- `blockbook_backend_subscription_events{subscription,event}` — feed lifecycle. `subscription` ∈ `newHeads`, `newPendingTransactions`, `rpc`, `zeromq`; `event` ∈ `error`, `resubscribed`, `resubscribe_failed`, `watchdog_stall`, `watchdog_tip_advanced`, `watchdog_reconnect`, `watchdog_reconnect_failed`. The two to alert on are `watchdog_tip_advanced` (the fallback poll found blocks the feed had dropped — the push feed is broken) and a sustained `subscription_age_seconds` at the threshold.
