# EVM EIP-1559 fee estimation

This document describes how EIP-1559 fees are produced for EVM coins across **Blockbook**
(the indexer) and **Trezor Suite** (the wallet), and which component owns which decision.

## Responsibilities

The split is deliberate: Blockbook supplies facts, the wallet decides policy.

**Blockbook — provides ground-truth inputs:**

- base fee — the next-block projection, taken from the chain
- priority-fee tiers (tips)
- congestion and trend signals (via the configured alternative provider, when supported)
- per-block gas — `baseFee`, `gasUsed`, `gasLimit`

**Trezor Suite — owns the fee policy:**

- chooses the base-fee source
- applies the head-room buffer (currently `2×`)
- composes `maxFeePerGas = 2 × baseFee + tip`
- clamps to per-coin `minFee` / `minPriorityFee` / `maxFee` limits
- displays the fee options and signs the transaction

This is why the wallet computes `maxFeePerGas` itself instead of trusting a provider's pre-padded
value (e.g. Infura's `suggestedMaxFeePerGas`, which was the source of the "fees too high" reports).

---

## Blockbook — pull path (`estimateFee` / `EthereumTypeGetEip1559Fees`)

On-demand estimate. Returns `Eip1559Fees`: a `baseFeePerGas` plus `low/medium/high` (and `instant`
on the on-chain path) tiers, and — on supported provider paths — congestion/trend ranges.

```mermaid
flowchart TD
    A["estimateFee"] --> B{"EIP-1559 supported?"}
    B -->|no| Z["legacy feePerUnit only"]
    B -->|yes| C{"fresh provider fees available?"}
    C -->|yes| D["use cached provider fees"]
    C -->|no| E["estimate on-chain from base fee + tips"]
    D --> R["return Eip1559Fees<br/>baseFee + fee tiers"]
    E --> R
```

How each source is built:

- **On-chain** (coins with no provider, or a stale one — e.g. `ethereum` non-archive, ETH testnets):
  one `eth_feeHistory` call (8 blocks, newest = `latest`, reward percentiles 20/50/99) yields the
  node's projected next-block `baseFeePerGas` (the last array element, computed with the chain's own
  EIP-1559 parameters) and one reward row per mined block. The window ends at `latest` rather than
  `pending` because a pending row is a half-built block sorted by tip, whose percentiles the window
  maximum would carry, and Erigon has no pending block and would shorten the window by one. Each percentile is
  reduced by its **window maximum** — the busiest of the last 8 blocks prices the quote, so a one-block
  tip spike stays in the estimate for ~100 s instead of being averaged away — and the tiers form a
  ladder on the 20th-percentile floor:

  | tier | tip |
  |---|---|
  | low (Economy) | max p20 — the cheapest tip that still bought a fifth of the busiest recent block |
  | medium (Normal) | 1.5 × low |
  | high (High) | max p50, never below medium |
  | instant | max p99, never below high (kept for the API; Suite does not use it) |

  Blockbook builds `maxFeePerGas = eip1559BaseFeeMultiplier(2) × baseFee + tip`. On a month of
  mainnet blocks (Sep–Oct 2026, 216k quotes) this lands Economy within 4 blocks 99.9% of the time,
  Normal within 2 blocks 99.4% and High in the next block 99.7%, paying 1.14× / 1.21× / 2.04× the
  tip that cleared the block. The former 4-block means of p20/p70/p90 landed 98.7% / 99.99% / 99.99%
  for 1.03× / 2.64× / 11.3×, and a plain floor ladder on a 4-block p20 median (1 / 1.5 / 2×) missed
  the High target 15% of the time.
- **Alternative provider** (archive coins, served from an in-memory cache, no node RPC): blockbook
  returns the provider's `maxFeePerGas` **unchanged**. For Infura, this is `suggestedMaxFeePerGas`,
  padded well above the base fee (~2.5× for the high tier); for 1inch, it is the provider's own
  computed `maxFeePerGas`. Blockbook does not rewrite it — the wallet overrides it (see the Suite
  section). The cache is refilled on demand at most once per `periodSeconds` and stays usable for
  `staleSeconds` afterwards; because the provider is billed per request, dev instances cache for far
  longer via the coin config's `additional_params_dev` block, which only a `BB_BUILD_ENV=dev` build
  applies (see [config.md](/docs/config.md)).

---

## Blockbook — push path (`subscribeNewBlock` → `evmData`)

Every connected block is pushed to subscribers with its block-level gas, so a wallet can keep its
fee projection fresh without polling. No extra RPC — the data comes from the block header already
fetched during sync.

```mermaid
sequenceDiagram
    participant N as EVM node
    participant SY as Sync (connectBlocks)
    participant GB as GetBlock + attachBlockGas
    participant WS as WebsocketServer
    participant C as Subscribers

    N->>SY: new block N
    SY->>GB: GetBlock(hash, height)
    GB->>GB: attachBlockGas: read baseFee, gasUsed, gasLimit
    GB-->>SY: Block + EthereumBlockSpecificData (transient, not persisted)
    SY->>WS: onNewBlock(block)
    WS->>WS: newBlockNotification(block)
    WS->>C: WsNewBlock height, hash, evmData
    Note over C: evmData = baseFeePerGas, blockGasUsed, blockGasLimit<br/>null on non-EVM chains and pre-London blocks
```

---

## Trezor Suite — `EthereumFeeLevels` (the fee policy)

Suite computes its own fees from the chain's base fee plus a `2×` buffer, preferring that block-based
estimate and falling back to the backend's tiers when block data is unavailable (so nothing regresses).

```mermaid
flowchart TD
    A["EthereumFeeLevels.load()"] --> B["fetch backend fees + latest block"]
    B --> C{"block-based estimate available?"}
    C -->|yes| D["compute from chain base fee<br/>maxFeePerGas = 2x baseFee + tip"]
    C -->|no| E["fall back to backend fees"]
    D --> F["clamp to coin min/max limits"]
    E --> F
    F --> G["fee levels: low / normal / high"]
```

Details and current coverage:

- The block-based estimate derives the next-block base fee from the latest block (EIP-1559 formula on
  `gasUsed`/`gasLimit`), takes tips from the block's transaction `maxPriorityFeePerGas` percentiles,
  and sets `maxFeePerGas = 2 × nextBaseFee + tip`. The fallback uses the backend's `eip1559` tiers.
- **evm-rpc-backed networks**: `getBlock('latest')` returns block gas → block-based fees apply.
- **blockbook-backed networks**: `getBlock('latest')` is unsupported → block-based estimate is empty →
  Suite falls back to the backend (provider) tiers. Wiring the `subscribeNewBlock` `evmData` push as the
  block-data source for these networks is the remaining step.

---

## End-to-end flow

```mermaid
sequenceDiagram
    actor U as User
    participant SU as Suite send form
    participant CN as connect — EthereumFeeLevels
    participant BB as Blockbook / node

    U->>SU: open send form
    SU->>CN: blockchainEstimateFee
    par estimateFee
        CN->>BB: estimateFee
    and latest block
        CN->>BB: getBlock latest
    end
    BB-->>CN: eip1559 tiers (+ block, if evm-rpc)
    CN->>CN: compute block-based fees (2x buffer), else fall back
    CN->>CN: clamp to coin min/max
    CN-->>SU: FeeLevel[] maxFeePerGas, maxPriorityFeePerGas, baseFee
    SU-->>U: show fee options
    U->>SU: confirm + sign
    SU->>BB: sendTransaction (EIP-1559)
```

---

## The `maxFeePerGas` formula

Both blockbook's on-chain path and Suite's block-based path compute the same value:

```
maxFeePerGas = 2 × baseFee + tip
```

- **base fee** — consensus-determined for the next block (deterministic from the parent header), so it
  is taken from the chain, never from an oracle estimate.
- **tip** (`maxPriorityFeePerGas`) — the only estimated part; from the fee-history ladder above, or
  the provider's per-tier suggestion.
- **`2×` buffer** — the wallet's policy knob. It absorbs roughly 6 consecutive full blocks of base-fee
  growth (the base fee can rise at most 12.5% per block) before a transaction stalls, and EIP-1559
  refunds the unused part. In blockbook it is the `eip1559BaseFeeMultiplier` constant; in Suite it is
  applied in `EthereumFeeLevels`.
