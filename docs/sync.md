# Sync

The sync engine connects blocks from the backend RPC into the local RocksDB index. It is driven by external block notifications (EVM `newHeads`, BTC ZMQ) and an internal periodic tick. This page documents the loop and the knobs that govern how it recovers from transient backend trouble.

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
