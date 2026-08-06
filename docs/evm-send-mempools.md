# EVM pending-transaction stores

Blockbook keeps EVM pending transactions in **two** stores when a coin is configured with a private
send-tx relay (`*_ALTERNATIVE_SENDTX_URLS`, `*_ALTERNATIVE_SENDTX_ONLY`,
`*_ALTERNATIVE_FETCH_MEMPOOL_TX`). Without a relay only the Blockbook mempool exists and the private
path below is skipped entirely. The broadcast flow and its invariants are described in
[evm-send.md](/docs/evm-send.md); this page is the map of the two stores.

| | Blockbook mempool | MEV / private cache |
|---|---|---|
| Type | `bchain.MempoolEthereumType` (`b.Mempool`) | `eth.AlternativeSendTxProvider.mempoolTxs` |
| Holds | txid + address index (+ token transfers) | full `RpcTransaction` body, sender, nonce, send generation |
| Populated from | `newPendingTransactions` WS feed; the resync snapshot; own sends when `disableMempoolSync` (and only when the send succeeded) | only sends a relay ACKed, in `ALTERNATIVE_SENDTX_ONLY` + `ALTERNATIVE_FETCH_MEMPOOL_TX` mode — from the signed bytes alone |
| Serves | address and xpub txs (`GetAddrDescTransactions`), wallet `NewTx` pushes | tx bodies on `GetTransaction`, the pending-nonce floor |
| Retention | `mempoolTxTimeout` — 10 min when a relay is configured, else `mempoolTxTimeoutHours` | `alternativeMempoolTxTimeout` — 5 min |
| Reconciled | `Resync` every ~60 s; timeout sweep at most every 10 min | `reconcileMempoolTxs` every 1 min, against the relay |

A private transaction is in **both**: the cache holds the body and is the source of truth, the
wrapped mempool holds the index built from it. A public transaction is only ever in the mempool.

Two couplings between the stores are load-bearing:

- `AddTransactionToMempool` builds its address index through `GetTransactionForMempool` →
  `GetTransaction`, which reads the cache first. A private transaction must therefore be cached
  *before* it is added to the wrapped mempool, or it cannot be indexed at all. The body comes from
  the send's own signed bytes, so this holds even when the relay never surfaces the transaction; the
  fetch-back never writes the cache, so it cannot re-index a transaction that has meanwhile mined and
  been cleared by block sync, nor replace a body Blockbook derived with a relay's view of it.
- The cache must expire **before** the wrapped mempool. Every cache exit clears the wrapped mempool
  too, but the mempool's own timeout sweep does not clear the cache; inverted, a private transaction
  loses its address index while still being served as pending. The defaults are ordered correctly
  and `CreateMempool` warns when an explicit configuration inverts them.

## Broadcast, ingest and eviction

```mermaid
%%{init: {"theme": "base", "themeVariables": {"lineColor": "#6b7280", "primaryTextColor": "#111827"}}}%%
flowchart TD
    send["SendRawTransaction(hex, disableAlternativeRPC)"]
    route{"relay configured<br/>and not disabled?"}
    relay["broadcast to every relay URL<br/>eth_sendRawTransaction, concurrently"]
    acc{"any relay URL accepted?"}
    only{"ALTERNATIVE_SENDTX_ONLY?"}
    fail["return relay error<br/>no cache path, no fetch-back"]
    primary["primary backend<br/>eth_sendRawTransaction"]
    reg["registerSuccessfulSend<br/>sender + accepting URL + nonce slot<br/>assign send generation"]
    ackevict["evictReplacedByNonce<br/>retire same from+nonce predecessor<br/>on ACK, generation-ordered"]
    cache["cacheMempoolTransaction<br/>body decoded from the signed bytes<br/>skipped if a newer send holds the slot"]
    handle["probeSentTransaction (background)<br/>fetch-back eth_getTransactionByHash<br/>reports whether the relay surfaces the send;<br/>never writes the cache"]

    ws["eth_subscribe newPendingTransactions<br/>skipped when disableMempoolSync"]
    snap["startup and Resync snapshot<br/>eth_getBlockByNumber pending<br/>only when queryBackendOnMempoolResync"]

    alt[("MEV / private cache<br/>full tx bodies<br/>timeout 5 min")]
    pub[("Blockbook mempool<br/>txids + address index<br/>timeout 10 min with relay")]

    altrec["reconcileMempoolTxs, every 1 min<br/>evicts mined, nonce_superseded, timeout<br/>keeps provider_missing until timeout"]
    pubrec["Mempool Resync, every 60 s<br/>timeout sweep at most every 10 min<br/>plus backend-missing removal"]

    readalt["GetTransaction read path<br/>entry past the cache timeout"]
    blk["GetBlock: tx in a connected block"]
    readmined["GetTransaction: mined or unknown"]

    altrm[("removeMempoolTx<br/>cache delete decides the race<br/>release nonce routing, metered once")]
    bothrm[("removeTransactionFromMempool<br/>clears BOTH stores")]

    send --> route
    route -- "no" --> primary
    route -- "yes" --> relay --> acc
    acc -- "no" --> only
    only -- "yes" --> fail
    only -- "no" --> primary
    acc -- "yes" --> reg --> ackevict --> cache --> handle
    ackevict -. "predecessor" .-> altrm
    cache -- "1. cache the body" --> alt
    cache -- "2. AddTransactionToMempool,<br/>index built by reading the cache,<br/>then push NewTx" --> pub
    handle -. "probe only,<br/>never writes" .-> alt
    primary -. "only when disableMempoolSync" .-> pub
    ws --> pub
    snap --> pub

    alt --> altrec --> altrm
    readalt --> altrm
    altrm -- "delegate, already metered" --> bothrm
    blk -- "metered sync_removed" --> bothrm
    readmined -- "metered sync_removed" --> bothrm
    bothrm -- "delete" --> alt
    bothrm -- "delete" --> pub
    pub --> pubrec -- "wrapped mempool only" --> pub

    classDef step fill:#e7f0ff,stroke:#4078c0,color:#10243e;
    classDef mev fill:#f3e8ff,stroke:#7c3aed,color:#2b1148;
    classDef pubstore fill:#e8f7ed,stroke:#2e8b57,color:#0b2c19;
    classDef sink fill:#fff7e6,stroke:#b8860b,color:#3a2a00;
    classDef error fill:#ffecec,stroke:#c03535,color:#3b0a0a;
    class send,route,relay,acc,only,primary,reg,ackevict,cache,handle,ws,snap,altrec,pubrec,readalt,blk,readmined step;
    class alt mev;
    class pub pubstore;
    class altrm,bothrm sink;
    class fail error;
```

## What the cache reconcile decides, per entry

Evaluated in this order once a minute for every cached transaction. The labels are the `action`
values of `blockbook_eth_alternative_mempool_reconciliation_events_total`.

```mermaid
%%{init: {"theme": "base", "themeVariables": {"lineColor": "#6b7280", "primaryTextColor": "#111827"}}}%%
flowchart TD
    e["cached entry, 1 min tick"]
    f{"age under 1 min?"}
    keepFresh["keep: skipped_fresh"]
    q["relay eth_getTransactionByHash"]
    qerr{"past 5 min timeout?"}
    evTo["evict: timeout"]
    keepErr["keep: provider_error"]
    m{"blockNumber set?"}
    evMined["evict: mined"]
    s{"confirmed nonce above tx nonce?<br/>relay eth_getTransactionCount latest"}
    evSup["evict: nonce_superseded"]
    k{"still surfaced by the relay?"}
    kto{"past 5 min timeout?"}
    evMiss["evict: provider_missing"]
    keepMiss["keep: provider_missing_pending<br/>an empty probe is not authoritative"]
    yto{"past 5 min timeout?"}
    evTo2["evict: timeout"]
    keepK["keep: kept"]

    e --> f
    f -- "yes" --> keepFresh
    f -- "no" --> q
    q -- "error" --> qerr
    qerr -- "yes" --> evTo
    qerr -- "no" --> keepErr
    q -- "answered" --> m
    m -- "yes" --> evMined
    m -- "no" --> s
    s -- "yes" --> evSup
    s -- "no" --> k
    k -- "no" --> kto
    kto -- "yes" --> evMiss
    kto -- "no" --> keepMiss
    k -- "yes" --> yto
    yto -- "yes" --> evTo2
    yto -- "no" --> keepK

    classDef step fill:#e7f0ff,stroke:#4078c0,color:#10243e;
    classDef keep fill:#e8f7ed,stroke:#2e8b57,color:#0b2c19;
    classDef evict fill:#fff7e6,stroke:#b8860b,color:#3a2a00;
    class e,f,q,qerr,m,s,k,kto,yto step;
    class keepFresh,keepErr,keepMiss,keepK keep;
    class evTo,evTo2,evMined,evSup,evMiss evict;
```

Every `evict:` box funnels through `removeMempoolTx` in the first diagram. In practice a mined
private transaction is usually cleared by block sync first, counted as `sync_removed`, before the
next probe reaches the `mined` branch here.

## How the two stores are read back

Only addresses that sent through the relay within the cache retention are routed to it
(`useForNonces`); everything else is served by the primary backend.

```mermaid
%%{init: {"theme": "base", "themeVariables": {"lineColor": "#6b7280", "primaryTextColor": "#111827"}}}%%
flowchart LR
    tx["GetTransaction(txid)"]
    hit{"in MEV cache<br/>and not expired?"}
    body["serve the cached body<br/>relay never asked"]
    rpcget["primary eth_getTransactionByHash<br/>then pruned-index recovery"]

    addr["address / xpub request"]
    idx["Blockbook mempool address index"]

    non["EthereumTypeGetNonces(addr)"]
    gate{"useForNonces(addr)?<br/>private send within 15 min"}
    nrelay["relay eth_getTransactionCount<br/>single accepting URL, batched pending+latest"]
    nprim["primary eth_getTransactionCount"]
    floor["raiseToPendingFloor<br/>advance over the contiguous cached run,<br/>never past a slot nothing fills"]

    est["EthereumTypeEstimateGas"]
    egate{"from set and useForNonces?"}
    erelay["relay eth_estimateGas"]
    eprim["primary eth_estimateGas"]

    tx --> hit
    hit -- "yes" --> body
    hit -- "no" --> rpcget
    addr --> idx -- "per txid" --> tx
    non --> gate
    gate -- "yes" --> nrelay --> floor
    gate -- "no" --> nprim --> floor
    est --> egate
    egate -- "yes" --> erelay
    erelay -. "error or bad result" .-> eprim
    egate -- "no" --> eprim

    classDef step fill:#e7f0ff,stroke:#4078c0,color:#10243e;
    classDef mev fill:#f3e8ff,stroke:#7c3aed,color:#2b1148;
    classDef pubstore fill:#e8f7ed,stroke:#2e8b57,color:#0b2c19;
    class tx,hit,rpcget,addr,non,gate,nprim,est,egate,erelay,eprim,floor step;
    class body,nrelay mev;
    class idx pubstore;
```
