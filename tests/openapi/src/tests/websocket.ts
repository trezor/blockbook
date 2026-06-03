import { addressPage, addressPageSize, blockPageSize, evmHistoryPage, evmHistoryPageSize } from "../constants.js";
import { blockFilterConfig, fiatMaxAgeSeconds } from "../config.js";
import { SkipTest } from "../errors.js";
import {
  assertAddressTxidsPayload,
  assertBasicAccountInfoPayload,
  assertBigIntString,
  assertComparableAccountPages,
  assertEqualString,
  assertEVMTokenBalancesHaveHoldingsFields,
  assertEVMBasicAddressPayload,
  assertEVMTokenBalancesPayload,
  assertEVMTokenListContractsMatch,
  assertFiatTickerEquals,
  assertFiatTickerFresh,
  assertFiatTickerPayload,
  assertGolombParams,
  assertNonEmptyString,
  assertPageMetaAllowUnknownTotal,
  assertStringSlicesEqual,
  assertUTXOListNonNegativeConfirmations,
  buildAddressDetailsPathWithTo,
  isObject,
  positiveNumber,
  txIDsFromTransactions,
} from "../support.js";
import { assertContractInfoFixturesFetched, assertErc4626FixturesInAccountInfo } from "./evm.js";
import { getFiatJSONOrSkip } from "./common.js";

import type { TestContext } from "../context.js";
import type { AddressResponse, AvailableVsCurrenciesResponse, BalanceHistoryResponse, BlockResponse, ContractInfoResponse, FiatTickerResponse, FiatTickersResponse, TxResponse, UtxoResponse, WsBlockHashResponse } from "../types.js";

type TestFunction = (ctx: TestContext) => Promise<void>;

async function testWsGetInfo(ctx: TestContext) {
  const info = await ctx.wsGetInfo();
  if (!positiveNumber(info.bestHeight)) {
    throw new Error(`invalid websocket bestHeight: ${String(info.bestHeight)}`);
  }
  assertNonEmptyString(info.bestHash, "WsGetInfo.bestHash");
}

async function testWsGetBlockHash(ctx: TestContext) {
  const info = await ctx.wsGetInfo();
  if (!positiveNumber(info.bestHeight)) {
    throw new Error(`invalid websocket bestHeight: ${String(info.bestHeight)}`);
  }

  const got = await ctx.wsCall<WsBlockHashResponse>(
    "getBlockHash",
    { height: info.bestHeight },
    "#/components/schemas/WsBlockHashRes",
  );
  assertNonEmptyString(got.hash, "WsGetBlockHash.hash");
  const want = await ctx.getBlockHashForHeight(info.bestHeight, false);
  if (want) {
    assertEqualString(got.hash, want, "websocket block hash");
  }
}

async function testWsGetTransaction(ctx: TestContext) {
  const txid = await ctx.sampleTxIDOrSkip();
  const tx = await ctx.wsCall<TxResponse>(
    "getTransaction",
    { txid },
    "#/components/schemas/Tx",
  );
  assertNonEmptyString(tx.txid, "WsGetTransaction.txid");
  assertEqualString(tx.txid, txid, "websocket transaction txid");
}

async function testWsGetAccountInfo(ctx: TestContext) {
  const address = await ctx.sampleAddressOrSkip();
  const txid = await ctx.sampleTxIDOrSkip();
  const info = await ctx.wsCall<AddressResponse>(
    "getAccountInfo",
    { descriptor: address, details: "txids", page: addressPage, pageSize: addressPageSize },
    "#/components/schemas/Address",
  );
  assertAddressTxidsPayload(info, address, txid, "WsGetAccountInfo", addressPageSize);
}

async function testWsGetAccountInfoBasic(ctx: TestContext) {
  const address = await ctx.sampleAddressOrSkip();
  const info = await ctx.wsCall<AddressResponse>(
    "getAccountInfo",
    { descriptor: address, details: "basic", page: addressPage, pageSize: addressPageSize },
    "#/components/schemas/Address",
  );
  assertBasicAccountInfoPayload(info, address, "WsGetAccountInfoBasic");
}

async function testWsGetAccountUtxo(ctx: TestContext) {
  const address = await ctx.sampleAddressOrSkip();
  const utxos = await ctx.wsCall<UtxoResponse[]>(
    "getAccountUtxo",
    { descriptor: address },
  );
  const label = "WS getAccountUtxo response data";
  if (!Array.isArray(utxos)) {
    throw new Error(`${label} is not an array`);
  }
  utxos.forEach((utxo, i) => {
    ctx.contract.validateSchemaRef("#/components/schemas/Utxo", `${label}[${i}]`, utxo);
  });
  assertUTXOListNonNegativeConfirmations(utxos, "WsGetAccountUtxo");
}

async function testWsPing(ctx: TestContext) {
  const response = await ctx.wsCallWithID<Record<string, unknown>>("ping-check-id", "ping", {});
  if (isObject(response) && "error" in response) {
    throw new Error(`websocket ping returned error payload: ${JSON.stringify(response)}`);
  }
}

async function testWsGetAccountInfoBasicEVM(ctx: TestContext) {
  const address = await ctx.sampleEVMAddressOrSkip();
  const info = await ctx.wsCall<AddressResponse>(
    "getAccountInfo",
    { descriptor: address, details: "basic", page: addressPage, pageSize: addressPageSize },
    "#/components/schemas/Address",
  );
  assertEVMBasicAddressPayload(info, address, "WsGetAccountInfoBasicEVM");
}

async function testWsGetAccountInfoEVM(ctx: TestContext) {
  const address = await ctx.sampleEVMAddressOrSkip();
  const info = await ctx.wsCall<AddressResponse>(
    "getAccountInfo",
    { descriptor: address, details: "tokenBalances", page: addressPage, pageSize: addressPageSize },
    "#/components/schemas/Address",
  );
  assertEVMTokenBalancesPayload(info, address, "WsGetAccountInfoEVM");
  assertEVMTokenBalancesHaveHoldingsFields(info, address, "WsGetAccountInfoEVM");
}

async function accountInfoConsistencyEVM(ctx: TestContext, details: "txids" | "txs", testName: string) {
  const address = await ctx.sampleEVMAddressOrSkip();
  const status = await ctx.getStatus();
  const bestHeight = status.bestHeight ?? 0;

  const httpResp = await ctx.client.getJson(
    "/api/v2/address/{address}",
    buildAddressDetailsPathWithTo(address, details, evmHistoryPage, evmHistoryPageSize, bestHeight),
  );
  const wsResp = await ctx.wsCall<AddressResponse>(
    "getAccountInfo",
    { descriptor: address, details, page: evmHistoryPage, pageSize: evmHistoryPageSize, to: bestHeight },
    "#/components/schemas/Address",
  );

  assertPageMetaAllowUnknownTotal(httpResp.page, httpResp.itemsOnPage, httpResp.totalPages, httpResp.txs, `${testName}.http`);
  assertPageMetaAllowUnknownTotal(wsResp.page, wsResp.itemsOnPage, wsResp.totalPages, wsResp.txs, `${testName}.ws`);
  assertComparableAccountPages(wsResp, httpResp, testName);

  const httpTxids = details === "txids"
    ? (httpResp.txids ?? [])
    : txIDsFromTransactions(httpResp.transactions ?? [], `${testName}.http`);
  const wsTxids = details === "txids"
    ? (wsResp.txids ?? [])
    : txIDsFromTransactions(wsResp.transactions ?? [], `${testName}.ws`);
  assertStringSlicesEqual(wsTxids, httpTxids, `${testName}.txids`);
}

const testWsGetAccountInfoTxidsConsistencyEVM = (ctx: TestContext) =>
  accountInfoConsistencyEVM(ctx, "txids", "WsGetAccountInfoTxidsConsistencyEVM");

const testWsGetAccountInfoTxsConsistencyEVM = (ctx: TestContext) =>
  accountInfoConsistencyEVM(ctx, "txs", "WsGetAccountInfoTxsConsistencyEVM");

async function testWsGetAccountInfoContractFilterEVM(ctx: TestContext) {
  const address = await ctx.sampleEVMAddressOrSkip();
  const contract = await ctx.sampleEVMContractOrSkip();
  const info = await ctx.wsCall<AddressResponse>(
    "getAccountInfo",
    { descriptor: address, details: "tokenBalances", contractFilter: contract, page: addressPage, pageSize: addressPageSize },
    "#/components/schemas/Address",
  );
  assertEVMTokenBalancesPayload(info, address, "WsGetAccountInfoContractFilterEVM");
  assertEVMTokenBalancesHaveHoldingsFields(info, address, "WsGetAccountInfoContractFilterEVM");
  assertEVMTokenListContractsMatch(info.tokens ?? [], contract, "WsGetAccountInfoContractFilterEVM");
}

async function testWsGetAccountInfoProtocolsEVM(ctx: TestContext) {
  await assertErc4626FixturesInAccountInfo(ctx, "WsGetAccountInfoProtocolsEVM", async (fixture) => {
    return ctx.wsCall<AddressResponse>(
      "getAccountInfo",
      {
        descriptor: fixture.holder,
        details: "tokenBalances",
        contractFilter: fixture.contract,
        protocols: ["erc4626"],
        page: addressPage,
        pageSize: addressPageSize,
      },
      "#/components/schemas/Address",
    );
  });
}

async function testWsGetContractInfoEVM(ctx: TestContext) {
  await assertContractInfoFixturesFetched(ctx, "WsGetContractInfoEVM", async (fixture) => {
    return ctx.wsCall<ContractInfoResponse>(
      "getContractInfo",
      { contract: fixture.contract, protocols: ["erc4626"] },
      "#/components/schemas/ContractInfoResult",
    );
  });
}

async function testWsGetCurrentFiatRates(ctx: TestContext) {
  // ws getCurrentFiatRates mirrors HTTP /api/v2/tickers/ (current, unfiltered).
  const ws = await ctx.wsCall<FiatTickerResponse>(
    "getCurrentFiatRates",
    {},
    "#/components/schemas/FiatTicker",
  );
  assertFiatTickerPayload(ws, "WsGetCurrentFiatRates");
  assertFiatTickerFresh(ws, "WsGetCurrentFiatRates", fiatMaxAgeSeconds());

  // HTTP-WS parity: both call GetCurrentFiatRates on the same instance, so they expose the
  // same currency set. Values/ts can differ if a refresh lands between the two calls, so
  // compare the stable currency keys rather than the time-sensitive values.
  const http = await ctx.sampleFiatTickerOrSkip();
  assertStringSlicesEqual(
    Object.keys(ws.rates ?? {}).sort(),
    Object.keys(http.rates ?? {}).sort(),
    "WsGetCurrentFiatRates parity currencies",
  );
}

async function testWsGetFiatRatesForTimestamps(ctx: TestContext) {
  const ticker = await ctx.sampleFiatTickerOrSkip();
  const ts = ticker.ts;
  if (!positiveNumber(ts)) {
    throw new SkipTest("fiat sample timestamp unavailable");
  }

  // Pick a currency that exists at ts (currencies available now may not exist historically).
  const list = await getFiatJSONOrSkip(ctx, "/api/v2/tickers-list/", `/api/v2/tickers-list/?timestamp=${ts}`);
  const currency = list.available_currencies?.[0]?.trim().toLowerCase();
  if (!currency) {
    throw new SkipTest(`no available fiat currencies for timestamp ${ts}`);
  }

  const ws = await ctx.wsCall<FiatTickersResponse>(
    "getFiatRatesForTimestamps",
    { timestamps: [ts], currencies: [currency] },
    "#/components/schemas/FiatTickers",
  );
  if (!Array.isArray(ws.tickers) || ws.tickers.length !== 1) {
    throw new Error(`WsGetFiatRatesForTimestamps expected 1 ticker, got ${ws.tickers?.length ?? 0}`);
  }
  assertFiatTickerPayload(ws.tickers[0], "WsGetFiatRatesForTimestamps");

  // HTTP-WS parity vs /api/v2/multi-tickers/ (immutable historical data → exact match).
  const http = await getFiatJSONOrSkip(
    ctx,
    "/api/v2/multi-tickers/",
    `/api/v2/multi-tickers/?timestamp=${ts}&currency=${encodeURIComponent(currency)}`,
  );
  if (http.length !== 1) {
    throw new Error(`WsGetFiatRatesForTimestamps HTTP expected 1 ticker, got ${http.length}`);
  }
  assertFiatTickerEquals(ws.tickers[0], http[0], "WsGetFiatRatesForTimestamps parity");
}

async function testWsGetFiatRatesTickersList(ctx: TestContext) {
  const ticker = await ctx.sampleFiatTickerOrSkip();
  const ts = ticker.ts;
  if (!positiveNumber(ts)) {
    throw new SkipTest("fiat sample timestamp unavailable");
  }

  const ws = await ctx.wsCall<AvailableVsCurrenciesResponse>(
    "getFiatRatesTickersList",
    { timestamp: ts },
    "#/components/schemas/AvailableVsCurrencies",
  );
  if (!positiveNumber(ws.ts)) {
    throw new Error(`WsGetFiatRatesTickersList invalid timestamp: ${String(ws.ts)}`);
  }
  if (!Array.isArray(ws.available_currencies) || ws.available_currencies.length === 0) {
    throw new Error("WsGetFiatRatesTickersList returned no currencies");
  }
  ws.available_currencies.forEach((currency) => assertNonEmptyString(currency, "WsGetFiatRatesTickersList.available_currencies"));

  // HTTP-WS parity vs /api/v2/tickers-list/.
  const http = await getFiatJSONOrSkip(ctx, "/api/v2/tickers-list/", `/api/v2/tickers-list/?timestamp=${ts}`);
  if (ws.ts !== http.ts) {
    throw new Error(`WsGetFiatRatesTickersList ts mismatch: ws=${ws.ts ?? 0} http=${http.ts ?? 0}`);
  }
  assertStringSlicesEqual(
    [...ws.available_currencies].sort(),
    [...(http.available_currencies ?? [])].sort(),
    "WsGetFiatRatesTickersList parity currencies",
  );
}

// Re-raise a ws error as a SkipTest when its message matches a known "unsupported on this
// chain/config" condition (e.g. ExtendedIndex off, balance history for a contract); otherwise
// rethrow so genuine failures still surface. Returns `never` so callers can rely on it throwing.
function skipWsUnsupportedOrRethrow(error: unknown, needles: string[], reason: string): never {
  const message = (error instanceof Error ? error.message : String(error)).toLowerCase();
  if (needles.some((needle) => message.includes(needle))) {
    throw new SkipTest(reason);
  }
  throw error;
}

async function testWsGetBlock(ctx: TestContext) {
  const sample = await ctx.getSampleIndexedBlock();
  if (!sample) {
    const status = await ctx.getStatus();
    throw new Error(`missing indexed block hash in recent height window near ${status.bestHeight ?? 0}`);
  }

  let block: BlockResponse;
  try {
    block = await ctx.wsCall<BlockResponse>(
      "getBlock",
      { id: sample.hash, page: 1, pageSize: blockPageSize },
      "#/components/schemas/Block",
    );
  } catch (error) {
    // ws getBlock is gated on ExtendedIndex flag; skip on instances where it is not enabled.
    skipWsUnsupportedOrRethrow(error, ["not supported"], "ws getBlock requires ExtendedIndex (not enabled)");
  }

  assertEqualString(block.hash, sample.hash, "WsGetBlock hash");
  if (block.height !== sample.height) {
    throw new Error(`WsGetBlock height mismatch: got ${block.height}, want ${sample.height}`);
  }
  if (!Array.isArray(block.txs)) {
    throw new Error("WsGetBlock response missing txs field");
  }
}

async function testWsGetBalanceHistory(ctx: TestContext) {
  const address = await ctx.sampleAddressOrSkip();
  const txid = await ctx.sampleTxIDOrSkip();
  const tx = await ctx.getTransactionByID(txid, false);

  // An unbounded query (from=0,to=0) aggregates the address's ENTIRE history, which can exceed
  // the ws timeout on busy chains (e.g. tron). The sample address comes from this recent tx
  // (within txSearchWindow=12 blocks of the tip), so bound the window to that block onward:
  // the server scans only ~a dozen blocks and the result still contains the sampled tx.
  const blockTime = tx?.blockTime;
  if (!positiveNumber(blockTime)) {
    throw new SkipTest(`sample tx ${txid} has no block time to bound balance history`);
  }
  const from = blockTime - 1;
  const to = blockTime + 24 * 3600; // clamps to best height; window is bounded by recent blocks

  let history: BalanceHistoryResponse[];
  try {
    history = await ctx.wsCall<BalanceHistoryResponse[]>("getBalanceHistory", { descriptor: address, from, to });
  } catch (error) {
    // EVM rejects balance history for contract addresses; skip when the sample is a contract.
    skipWsUnsupportedOrRethrow(error, ["not allowed"], `ws getBalanceHistory not allowed for ${address} (contract)`);
  }

  if (!Array.isArray(history)) {
    throw new Error("WsGetBalanceHistory response is not an array");
  }
  // An empty history is valid (address with no activity in range); validate any entries present.
  history.forEach((entry, i) => {
    ctx.contract.validateSchemaRef("#/components/schemas/BalanceHistory", `WsGetBalanceHistory[${i}]`, entry);
    if (!positiveNumber(entry.time)) {
      throw new Error(`WsGetBalanceHistory[${i}] invalid time: ${String(entry.time)}`);
    }
    if (!Number.isInteger(entry.txs) || entry.txs < 0) {
      throw new Error(`WsGetBalanceHistory[${i}] invalid txs: ${String(entry.txs)}`);
    }
  });
}

async function testWsGetTransactionSpecific(ctx: TestContext) {
  const txid = await ctx.sampleTxIDOrSkip();
  // Response is chain-specific free-form JSON (no schema ref to validate against).
  const specific = await ctx.wsCall<Record<string, unknown>>("getTransactionSpecific", { txid });
  if (!isObject(specific) || Object.keys(specific).length === 0) {
    throw new Error(`WsGetTransactionSpecific empty response for ${txid}`);
  }
  const rawTxid = specific.txid;
  if (typeof rawTxid === "string" && rawTxid.trim() !== "" && rawTxid.toLowerCase() !== txid.toLowerCase()) {
    throw new Error(`WsGetTransactionSpecific txid mismatch: got ${rawTxid}, want ${txid}`);
  }
}

async function testWsLongTermFeeRate(ctx: TestContext) {
  // UTXO-only method; gated to UTXO coins via the ws-utxo capability group.
  // No response schema exists in openapi.yaml, so validate structurally.
  const res = await ctx.wsCall<{ feePerUnit?: string; blocks?: number }>("longTermFeeRate", {});
  assertBigIntString(res.feePerUnit, "WsLongTermFeeRate.feePerUnit");
  if (!Number.isInteger(res.blocks) || (res.blocks ?? -1) < 0) {
    throw new Error(`WsLongTermFeeRate invalid blocks: ${String(res.blocks)}`);
  }
}

// The Golomb-filter responses (getBlockFilter/getBlockFiltersBatch/getMempoolFilters) are
// anonymous server structs with no openapi schema, so validate the shared P/M/zeroedKey header
// structurally via the shared assertGolombParams helper (support.ts), which is also used by the
// HTTP /api/v2/block-filters twin in utxo.ts.
type WsBlockFilterRes = { P?: number; M?: number; zeroedKey?: boolean; blockFilter?: string };
type WsBlockFiltersBatchRes = { P?: number; M?: number; zeroedKey?: boolean; blockFiltersBatch?: string[] };
type WsMempoolFiltersRes = { P?: number; M?: number; zeroedKey?: boolean; entries?: Record<string, string> };

async function testWsGetBlockFilter(ctx: TestContext) {
  const { scriptType, golombP } = blockFilterConfig(ctx.coin);
  if (!scriptType) {
    throw new SkipTest(`${ctx.coin} has no block_filter_scripts configured`);
  }
  const sample = await ctx.getSampleIndexedBlock();
  if (!sample) {
    const status = await ctx.getStatus();
    throw new Error(`missing indexed block near ${status.bestHeight ?? 0}`);
  }

  let res: WsBlockFilterRes;
  try {
    res = await ctx.wsCall<WsBlockFilterRes>("getBlockFilter", { scriptType, blockHash: sample.hash });
  } catch (error) {
    skipWsUnsupportedOrRethrow(error, ["not supported", "unsupported script"], `ws getBlockFilter unsupported on ${ctx.coin}`);
  }
  assertGolombParams(res, golombP, "WsGetBlockFilter");
  if (typeof res.blockFilter !== "string" || res.blockFilter.trim() === "" || !/^[0-9a-f]+$/i.test(res.blockFilter)) {
    throw new Error(`WsGetBlockFilter blockFilter is not a non-empty hex string: ${String(res.blockFilter)}`);
  }
}

async function testWsGetBlockFiltersBatch(ctx: TestContext) {
  const { scriptType, golombP } = blockFilterConfig(ctx.coin);
  if (!scriptType) {
    throw new SkipTest(`${ctx.coin} has no block_filter_scripts configured`);
  }
  const status = await ctx.getStatus();
  const best = status.bestHeight ?? 0;
  const pageSize = 5;
  // The batch returns filters forward from the anchor, so anchor a few blocks behind the tip
  // to guarantee a non-empty result.
  const anchorHeight = Math.max(1, best - (pageSize + 2));
  const anchorHash = await ctx.getBlockHashForHeight(anchorHeight, false);
  if (!anchorHash) {
    throw new SkipTest(`no block hash for height ${anchorHeight}`);
  }

  let res: WsBlockFiltersBatchRes;
  try {
    res = await ctx.wsCall<WsBlockFiltersBatchRes>(
      "getBlockFiltersBatch",
      { scriptType, bestKnownBlockHash: anchorHash, pageSize },
    );
  } catch (error) {
    skipWsUnsupportedOrRethrow(error, ["not supported", "unsupported script"], `ws getBlockFiltersBatch unsupported on ${ctx.coin}`);
  }
  assertGolombParams(res, golombP, "WsGetBlockFiltersBatch");
  const batch = res.blockFiltersBatch;
  if (!Array.isArray(batch) || batch.length === 0 || batch.length > pageSize) {
    throw new Error(`WsGetBlockFiltersBatch expected 1..${pageSize} entries, got ${Array.isArray(batch) ? batch.length : "non-array"}`);
  }
  batch.forEach((entry, i) => {
    // entry format: "<height>:<blockHash>:<filterHex>"
    const parts = entry.split(":");
    if (parts.length !== 3) {
      throw new Error(`WsGetBlockFiltersBatch[${i}] malformed entry: ${entry}`);
    }
    const [heightStr, blockHash, filter] = parts;
    if (!/^[0-9]+$/.test(heightStr)) {
      throw new Error(`WsGetBlockFiltersBatch[${i}] invalid height: ${heightStr}`);
    }
    if (!/^[0-9a-f]+$/i.test(blockHash)) {
      throw new Error(`WsGetBlockFiltersBatch[${i}] invalid blockHash: ${blockHash}`);
    }
    if (!/^[0-9a-f]*$/i.test(filter)) {
      throw new Error(`WsGetBlockFiltersBatch[${i}] invalid filter hex: ${filter}`);
    }
  });
}

async function testWsGetMempoolFilters(ctx: TestContext) {
  const { scriptType, golombP } = blockFilterConfig(ctx.coin);
  if (!scriptType) {
    throw new SkipTest(`${ctx.coin} has no block_filter_scripts configured`);
  }

  let res: WsMempoolFiltersRes;
  try {
    res = await ctx.wsCall<WsMempoolFiltersRes>("getMempoolFilters", { scriptType, fromTimestamp: 0 });
  } catch (error) {
    skipWsUnsupportedOrRethrow(error, ["not supported", "unsupported script"], `ws getMempoolFilters unsupported on ${ctx.coin}`);
  }
  assertGolombParams(res, golombP, "WsGetMempoolFilters");
  const entries = res.entries;
  if (entries === null || typeof entries !== "object" || Array.isArray(entries)) {
    throw new Error(`WsGetMempoolFilters entries is not an object: ${String(entries)}`);
  }
  // entries may be empty (no matching mempool txs at call time); validate any present.
  for (const [txid, filter] of Object.entries(entries)) {
    assertNonEmptyString(txid, "WsGetMempoolFilters.entries.txid");
    if (typeof filter !== "string" || !/^[0-9a-f]+$/i.test(filter)) {
      throw new Error(`WsGetMempoolFilters entry ${txid} is not a hex filter: ${String(filter)}`);
    }
  }
}

export const wsOnlyTests: Record<string, TestFunction> = {
  WsGetInfo: testWsGetInfo,
  WsGetBlockHash: testWsGetBlockHash,
  WsGetTransaction: testWsGetTransaction,
  WsGetAccountInfo: testWsGetAccountInfo,
  WsGetAccountInfoBasic: testWsGetAccountInfoBasic,
  WsPing: testWsPing,
  WsGetCurrentFiatRates: testWsGetCurrentFiatRates,
  WsGetFiatRatesForTimestamps: testWsGetFiatRatesForTimestamps,
  WsGetFiatRatesTickersList: testWsGetFiatRatesTickersList,
  WsGetBlock: testWsGetBlock,
  WsGetBalanceHistory: testWsGetBalanceHistory,
  WsGetTransactionSpecific: testWsGetTransactionSpecific,
};

export const wsUTXOTests: Record<string, TestFunction> = {
  WsGetAccountUtxo: testWsGetAccountUtxo,
  WsLongTermFeeRate: testWsLongTermFeeRate,
  WsGetBlockFilter: testWsGetBlockFilter,
  WsGetBlockFiltersBatch: testWsGetBlockFiltersBatch,
  WsGetMempoolFilters: testWsGetMempoolFilters,
};

export const wsEVMTests: Record<string, TestFunction> = {
  WsGetAccountInfoBasicEVM: testWsGetAccountInfoBasicEVM,
  WsGetAccountInfoEVM: testWsGetAccountInfoEVM,
  WsGetAccountInfoTxidsConsistencyEVM: testWsGetAccountInfoTxidsConsistencyEVM,
  WsGetAccountInfoTxsConsistencyEVM: testWsGetAccountInfoTxsConsistencyEVM,
  WsGetAccountInfoContractFilterEVM: testWsGetAccountInfoContractFilterEVM,
  WsGetAccountInfoProtocolsEVM: testWsGetAccountInfoProtocolsEVM,
  WsGetContractInfoEVM: testWsGetContractInfoEVM,
};
