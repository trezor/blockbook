// Drift detector: every interface in `blockbook-api.ts` (auto-generated from Go
// structs) must be structurally assignable to its hand-written counterpart in
// `openapi.yaml`. Catches the case where Go types change but the OpenAPI YAML
// is not updated to match.
//
// Direction is one-way on purpose: the OpenAPI schema is allowed to be wider
// than the Go shape, because the wire format reflects observed runtime
// behavior across all supported chains while the Go field carries a single
// concrete Go type. Real cases this allows today:
//
//   - `Block.version`        Bb: `string`           OAS: `string | integer`
//                            Ethereum returns numeric block versions; the Go
//                            field is plain string after JSON round-tripping.
//   - `Vout.addresses`       Bb: `string[]`         OAS: `string[] | null`
//                            A nil Go slice marshals to `null` because the
//                            `addresses` JSON tag has no `,omitempty`.
//
// What we *don't* allow is the Go shape declaring something the OpenAPI
// schema cannot accept (extra fields, narrower-but-incompatible types).
//
// This file emits no runtime code. `tsc --noEmit` (run by `npm run typecheck`)
// fails when an assertion below resolves to a string literal rather than
// `true`. The failing literal names the offending schema so the build log
// points straight at the drift.

import type * as Bb from "../../../blockbook-api.js";
import type { components } from "../.generated/blockbook.js";

type Schemas = components["schemas"];

// `[A] extends [B]` (with bracketed tuples) disables union distribution so the
// whole TS shape is checked against the whole OAS shape as a single unit.
//
// Two separate checks:
//   1. Every value in TS must be assignable to the matching OAS value.
//   2. Every key in TS must exist on the OAS shape (catches "Go added a field
//      but YAML doesn't have it yet"). The reverse — OAS has a key TS lacks —
//      is intentionally allowed: the YAML can describe optional response
//      enrichments that aren't surfaced in the typescriptified Go types.
type Compat<Ts, Oas, Name extends string> = [Ts] extends [Oas]
  ? [keyof Ts] extends [keyof Oas]
    ? true
    : `${Name}: blockbook-api.ts has properties not declared in openapi.yaml`
  : `${Name}: blockbook-api.ts shape is not assignable to openapi.yaml schema`;

// Each `const _<Name>: Compat<...> = true` assignment fails to typecheck when
// `Compat<...>` resolves to a string. The literal-typed failure message names
// the offending schema so the build log points straight at the drift.

const _AddressAlias: Compat<Bb.AddressAlias, Schemas["AddressAlias"], "AddressAlias"> = true;

const _MultiTokenValue: Compat<Bb.MultiTokenValue, Schemas["MultiTokenValue"], "MultiTokenValue"> = true;
const _TokenTransfer: Compat<Bb.TokenTransfer, Schemas["TokenTransfer"], "TokenTransfer"> = true;
const _Vin: Compat<Bb.Vin, Schemas["Vin"], "Vin"> = true;
const _Vout: Compat<Bb.Vout, Schemas["Vout"], "Vout"> = true;

const _EthereumInternalTransfer: Compat<Bb.EthereumInternalTransfer, Schemas["EthereumInternalTransfer"], "EthereumInternalTransfer"> = true;
const _EthereumParsedInputParam: Compat<Bb.EthereumParsedInputParam, Schemas["EthereumParsedInputParam"], "EthereumParsedInputParam"> = true;
const _EthereumParsedInputData: Compat<Bb.EthereumParsedInputData, Schemas["EthereumParsedInputData"], "EthereumParsedInputData"> = true;
const _EthereumSpecific: Compat<Bb.EthereumSpecific, Schemas["EthereumSpecific"], "EthereumSpecific"> = true;

const _TxChainExtraData: Compat<Bb.TxChainExtraData, Schemas["TxChainExtraData"], "TxChainExtraData"> = true;
const _AccountChainExtraData: Compat<Bb.AccountChainExtraData, Schemas["AccountChainExtraData"], "AccountChainExtraData"> = true;

const _Tx: Compat<Bb.Tx, Schemas["Tx"], "Tx"> = true;
const _FeeStats: Compat<Bb.FeeStats, Schemas["FeeStats"], "FeeStats"> = true;

const _Erc4626TokenMetadata: Compat<Bb.Erc4626TokenMetadata, Schemas["Erc4626TokenMetadata"], "Erc4626TokenMetadata"> = true;
const _Erc4626Token: Compat<Bb.Erc4626Token, Schemas["Erc4626Token"], "Erc4626Token"> = true;
const _ContractInfoProtocols: Compat<Bb.ContractInfoProtocols, Schemas["ContractInfoProtocols"], "ContractInfoProtocols"> = true;
const _ContractInfoRates: Compat<Bb.ContractInfoRates, Schemas["ContractInfoRates"], "ContractInfoRates"> = true;
const _ContractInfoResult: Compat<Bb.ContractInfoResult, Schemas["ContractInfoResult"], "ContractInfoResult"> = true;

const _Token: Compat<Bb.Token, Schemas["Token"], "Token"> = true;
const _StakingPool: Compat<Bb.StakingPool, Schemas["StakingPool"], "StakingPool"> = true;
const _Address: Compat<Bb.Address, Schemas["Address"], "Address"> = true;

const _Utxo: Compat<Bb.Utxo, Schemas["Utxo"], "Utxo"> = true;
const _BalanceHistory: Compat<Bb.BalanceHistory, Schemas["BalanceHistory"], "BalanceHistory"> = true;
const _Block: Compat<Bb.Block, Schemas["Block"], "Block"> = true;
const _BlockRaw: Compat<Bb.BlockRaw, Schemas["BlockRaw"], "BlockRaw"> = true;

const _BackendInfo: Compat<Bb.BackendInfo, Schemas["BackendInfo"], "BackendInfo"> = true;
const _InternalStateColumn: Compat<Bb.InternalStateColumn, Schemas["InternalStateColumn"], "InternalStateColumn"> = true;
const _BlockbookInfo: Compat<Bb.BlockbookInfo, Schemas["BlockbookInfo"], "BlockbookInfo"> = true;
const _SystemInfo: Compat<Bb.SystemInfo, Schemas["SystemInfo"], "SystemInfo"> = true;

const _FiatTicker: Compat<Bb.FiatTicker, Schemas["FiatTicker"], "FiatTicker"> = true;
const _FiatTickers: Compat<Bb.FiatTickers, Schemas["FiatTickers"], "FiatTickers"> = true;
const _AvailableVsCurrencies: Compat<Bb.AvailableVsCurrencies, Schemas["AvailableVsCurrencies"], "AvailableVsCurrencies"> = true;

// WebSocket envelopes: `params`/`data` are `any` in Go (typescriptify cannot
// see through the interface{} runtime discriminator), but the YAML enumerates
// the polymorphic shape via oneOf. `any extends X` is always true, so this
// only catches drift in `id`/`method`.
const _WsReq: Compat<Bb.WsReq, Schemas["WsRequest"], "WsRequest"> = true;
const _WsRes: Compat<Bb.WsRes, Schemas["WsResponse"], "WsResponse"> = true;

const _WsAccountInfoReq: Compat<Bb.WsAccountInfoReq, Schemas["WsAccountInfoReq"], "WsAccountInfoReq"> = true;
const _WsContractInfoReq: Compat<Bb.WsContractInfoReq, Schemas["WsContractInfoReq"], "WsContractInfoReq"> = true;
const _WsBackendInfo: Compat<Bb.WsBackendInfo, Schemas["WsBackendInfo"], "WsBackendInfo"> = true;
const _WsInfoRes: Compat<Bb.WsInfoRes, Schemas["WsInfoRes"], "WsInfoRes"> = true;
const _WsBlockHashReq: Compat<Bb.WsBlockHashReq, Schemas["WsBlockHashReq"], "WsBlockHashReq"> = true;
const _WsBlockHashRes: Compat<Bb.WsBlockHashRes, Schemas["WsBlockHashRes"], "WsBlockHashRes"> = true;
const _WsBlockReq: Compat<Bb.WsBlockReq, Schemas["WsBlockReq"], "WsBlockReq"> = true;
const _WsBlockFilterReq: Compat<Bb.WsBlockFilterReq, Schemas["WsBlockFilterReq"], "WsBlockFilterReq"> = true;
const _WsBlockFiltersBatchReq: Compat<Bb.WsBlockFiltersBatchReq, Schemas["WsBlockFiltersBatchReq"], "WsBlockFiltersBatchReq"> = true;
const _WsAccountUtxoReq: Compat<Bb.WsAccountUtxoReq, Schemas["WsAccountUtxoReq"], "WsAccountUtxoReq"> = true;
const _WsBalanceHistoryReq: Compat<Bb.WsBalanceHistoryReq, Schemas["WsBalanceHistoryReq"], "WsBalanceHistoryReq"> = true;
const _WsTransactionReq: Compat<Bb.WsTransactionReq, Schemas["WsTransactionReq"], "WsTransactionReq"> = true;
const _WsTransactionSpecificReq: Compat<Bb.WsTransactionSpecificReq, Schemas["WsTransactionSpecificReq"], "WsTransactionSpecificReq"> = true;
const _WsEstimateFeeReq: Compat<Bb.WsEstimateFeeReq, Schemas["WsEstimateFeeReq"], "WsEstimateFeeReq"> = true;
const _Eip1559Fee: Compat<Bb.Eip1559Fee, Schemas["Eip1559Fee"], "Eip1559Fee"> = true;
const _Eip1559Fees: Compat<Bb.Eip1559Fees, Schemas["Eip1559Fees"], "Eip1559Fees"> = true;
const _WsEstimateFeeRes: Compat<Bb.WsEstimateFeeRes, Schemas["WsEstimateFeeRes"], "WsEstimateFeeRes"> = true;
const _WsSendTransactionReq: Compat<Bb.WsSendTransactionReq, Schemas["WsSendTransactionReq"], "WsSendTransactionReq"> = true;
const _WsSubscribeAddressesReq: Compat<Bb.WsSubscribeAddressesReq, Schemas["WsSubscribeAddressesReq"], "WsSubscribeAddressesReq"> = true;
const _WsSubscribeFiatRatesReq: Compat<Bb.WsSubscribeFiatRatesReq, Schemas["WsSubscribeFiatRatesReq"], "WsSubscribeFiatRatesReq"> = true;
const _WsCurrentFiatRatesReq: Compat<Bb.WsCurrentFiatRatesReq, Schemas["WsCurrentFiatRatesReq"], "WsCurrentFiatRatesReq"> = true;
const _WsFiatRatesForTimestampsReq: Compat<Bb.WsFiatRatesForTimestampsReq, Schemas["WsFiatRatesForTimestampsReq"], "WsFiatRatesForTimestampsReq"> = true;
const _WsFiatRatesTickersListReq: Compat<Bb.WsFiatRatesTickersListReq, Schemas["WsFiatRatesTickersListReq"], "WsFiatRatesTickersListReq"> = true;
const _WsMempoolFiltersReq: Compat<Bb.WsMempoolFiltersReq, Schemas["WsMempoolFiltersReq"], "WsMempoolFiltersReq"> = true;
const _WsRpcCallReq: Compat<Bb.WsRpcCallReq, Schemas["WsRpcCallReq"], "WsRpcCallReq"> = true;
const _WsRpcCallRes: Compat<Bb.WsRpcCallRes, Schemas["WsRpcCallRes"], "WsRpcCallRes"> = true;

const _MempoolTxidFilterEntries: Compat<Bb.MempoolTxidFilterEntries, Schemas["MempoolTxidFilterEntries"], "MempoolTxidFilterEntries"> = true;

// Intentionally not compared (no OpenAPI counterpart):
//   APIError                  — errors surface as `ErrorResponse` wrapper, not as a bare Go shape.
//   TronVoteExtra, TronVote,
//   TronUnstakingBatch,
//   TronStakingInfo,
//   TronChainExtraData,
//   TronAccountExtraData      — Tron payload types are reachable only via the discriminated `(Tx|Account)ChainExtraData` union, which is compared at the wrapper level.
//   BlockInfo, Blocks         — `api.Blocks` is currently surfaced inline in path responses; no top-level YAML schema yet.
//   WsLongTermFeeRateRes      — long-term fee rate is documented inline under the WebSocket path; no top-level YAML schema yet.
//
// Suppress "value never read" for assertions: they exist purely for their
// type-side errors. `void` references keep tsc happy without runtime effect.
void [
  _AddressAlias, _MultiTokenValue, _TokenTransfer, _Vin, _Vout,
  _EthereumInternalTransfer, _EthereumParsedInputParam, _EthereumParsedInputData, _EthereumSpecific,
  _TxChainExtraData, _AccountChainExtraData,
  _Tx, _FeeStats,
  _Erc4626TokenMetadata, _Erc4626Token, _ContractInfoProtocols, _ContractInfoRates, _ContractInfoResult,
  _Token, _StakingPool, _Address,
  _Utxo, _BalanceHistory, _Block, _BlockRaw,
  _BackendInfo, _InternalStateColumn, _BlockbookInfo, _SystemInfo,
  _FiatTicker, _FiatTickers, _AvailableVsCurrencies,
  _WsReq, _WsRes,
  _WsAccountInfoReq, _WsContractInfoReq, _WsBackendInfo, _WsInfoRes,
  _WsBlockHashReq, _WsBlockHashRes, _WsBlockReq, _WsBlockFilterReq, _WsBlockFiltersBatchReq,
  _WsAccountUtxoReq, _WsBalanceHistoryReq, _WsTransactionReq, _WsTransactionSpecificReq,
  _WsEstimateFeeReq, _Eip1559Fee, _Eip1559Fees, _WsEstimateFeeRes,
  _WsSendTransactionReq, _WsSubscribeAddressesReq, _WsSubscribeFiatRatesReq,
  _WsCurrentFiatRatesReq, _WsFiatRatesForTimestampsReq, _WsFiatRatesTickersListReq,
  _WsMempoolFiltersReq, _WsRpcCallReq, _WsRpcCallRes,
  _MempoolTxidFilterEntries,
];
