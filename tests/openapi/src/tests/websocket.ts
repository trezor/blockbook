import { addressPage, addressPageSize, evmHistoryPage, evmHistoryPageSize } from "../constants.js";
import { fiatMaxAgeSeconds } from "../config.js";
import { SkipTest } from "../errors.js";
import {
  assertAddressTxidsPayload,
  assertBasicAccountInfoPayload,
  assertComparableAccountPages,
  assertEqualString,
  assertEVMTokenBalancesHaveHoldingsFields,
  assertEVMBasicAddressPayload,
  assertEVMTokenBalancesPayload,
  assertEVMTokenListContractsMatch,
  assertFiatTickerEquals,
  assertFiatTickerFresh,
  assertFiatTickerPayload,
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
import type { AddressResponse, AvailableVsCurrenciesResponse, ContractInfoResponse, FiatTickerResponse, FiatTickersResponse, TxResponse, UtxoResponse, WsBlockHashResponse } from "../types.js";

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
};

export const wsUTXOTests: Record<string, TestFunction> = {
  WsGetAccountUtxo: testWsGetAccountUtxo,
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
