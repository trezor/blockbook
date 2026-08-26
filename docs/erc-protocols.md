# ERC protocol enrichments

Blockbook can enrich EVM contract responses with **protocol-level facts** that go beyond
the ERC-20/721/1155 metadata the indexer stores natively — for example "this contract is
an ERC-4626 vault, and its underlying asset is DAI". Protocol detection is lazy and
API-driven: it never slows down sync, and it is opt-in per request via the `protocols`
parameter on `getContractInfo` and `getAccountInfo`.

ERC-4626 is the first protocol implemented and doubles as the reference implementation.
The pieces below it — storage, reorg safety, batched probing, caching — are protocol-agnostic
and are meant to be reused as-is by the next protocol.

## The shared foundation

**Storage.** Protocol records live in their own `ercProtocols` column family, keyed by
`(protocolId, contract)` with an opaque, protocol-defined payload — see
[RocksDB](/docs/rocksdb.md) for the exact key layout. Being a separate CF from `contracts`
means API-time protocol writes can never clobber sync-owned contract metadata, and purging a
contract's metadata leaves its protocol records intact.

**Reorg safety.** This is the part you get for free, and the part that is easiest to get
wrong. Every record is anchored to the `persistHeight` at which it was observed. The writer
samples the reorg generation and block hash *before* the probe and refuses the write under
`connectBlockMux` if either shifted, so a reorg racing an in-flight multicall cannot persist a
stale fact. A secondary `byHeight` index lets `DisconnectBlockRangeEthereumType` revert exactly
the affected records with one range scan. Cost of a false refusal is one re-probe.

**Probing.** Detection uses Multicall3 `aggregate3` with `AllowFailure`, so a whole page of
token candidates is classified in one `eth_call`, and contracts that don't implement the
protocol simply come back unsuccessful instead of erroring. Chains without a Multicall3
deployment degrade to no enrichment rather than failing.

## Querying for protocol information 

Detection is a two-call probe — `asset()` and `totalAssets()`, both mandated by EIP-4626 — run
against ERC-20-shaped contracts. On success the underlying asset address is persisted as the
payload (it is immutable per spec, so the record is written once and reused), and the response is
enriched with vault metadata plus live conversion rates (`convertToAssets`, `previewRedeem`,
and their asset-side counterparts). Partial failures degrade gracefully: confirmed fields are
returned alongside an `error` string rather than dropping the whole enrichment.

The split is worth internalising, because a new protocol will want the same shape:

* **`getAccountInfo`** returns cheap, already-known identifiers — `token.protocols: ["erc4626"]`,
  sourced from indexed records and one batched probe for unknown candidates.
* **`getContractInfo`** returns the rich, freshly-fetched payload under `protocols.erc4626`.

## Adding a new protocol, e.g. ERC-2612

ERC-2612 (`permit` — gasless approvals) is a good next candidate: detectable by view calls
(`DOMAIN_SEPARATOR()`, `nonces(address)`), relevant to any ERC-20 holder, and a natural
`getAccountInfo` flag. The path:

1. **Reserve a protocol ID.** Take the next free byte in `db/rocksdb_protocols.go`
   (`ErcProtocolErc2612 byte = 0x02`) and add it to the list `disconnectErcProtocols` iterates
   — that list is what makes records reorg-revertible, so it is the one step not to skip.
2. **Define the payload.** Whatever the protocol needs, packed with the existing `packString` /
   `packVaruint` helpers. Empty is fine when presence alone is the fact. Consider leading the
   payload with a version byte if you expect the format to evolve.
3. **Add typed accessors** in `db/rocksdb_contracts.go` mirroring
   `SetContractInfoErc4626Vault` / `GetContractInfoErc4626Vault`, and merge the record into
   `GetContractInfo` if the read path needs it.
4. **Write the probe** in a new `api/erc2612.go`, modelled on `api/erc4626.go`: declare the
   method selectors, build the multicall, decode results. Extend the encode/decode helpers if
   your calls need argument or return types beyond no-arg / `uint256` / `address`.
5. **Register the name** in `knownErcProtocols` (`api/contract.go`) so `protocols=erc2612` is
   accepted, and hook the enrichment into `enrichTokenProtocols` and `GetContractInfoData`.
6. **Extend the response types** — a `Erc2612Token` struct in `api/types.go` and a field on
   `ContractInfoProtocols`. Typed clients get a precise shape rather than a loose map.
7. **Update the API surface by hand**: `openapi.yaml` and `blockbook-api.ts` are edited
   together, plus the protocol ID table in [RocksDB](/docs/rocksdb.md).
8. **Test** against `db/rocksdb_protocols_test.go` (storage and disconnect) and
   `api/erc4626_test.go` (probe and enrichment) as templates.

One policy decision is yours to make. ERC-4626 stores an immutable fact, so the writer refuses
to overwrite a differing payload and warns instead. A protocol whose facts can legitimately
change wants the opposite — overwrite the record and re-anchor `persistHeight` so the byHeight
index still tracks it. That is a small branch in `SetErcProtocol`, keyed on the protocol ID;
everything below it stays as-is.
