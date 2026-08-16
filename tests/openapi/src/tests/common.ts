import { preview } from "../openapi.js";
import { SkipTest } from "../errors.js";
import { addressPage, addressPageSize, blockPageSize } from "../constants.js";
import type { GetOperationPath, GetResponse } from "../client.js";
import {
  assertAddressTxidsPayload,
  assertAddressTxsPayload,
  assertBasicAccountInfoPayload,
  assertEqualString,
  assertFiatTickerFresh,
  assertFiatTickerPayload,
  assertNonEmptyString,
  buildAddressDetailsPath,
  buildAddressDetailsPathWithRange,
  encodePathSegment,
  isFiatDataUnavailable,
  isObject,
  positiveNumber,
} from "../support.js";
import { fiatMaxAgeSeconds } from "../config.js";

import type { TestContext } from "../context.js";

type TestFunction = (ctx: TestContext) => Promise<void>;

async function testStatus(ctx: TestContext) {
  await ctx.getStatus();
}

async function testGetBlockIndex(ctx: TestContext) {
  const sample = await ctx.getSampleIndexedHeight();
  if (!sample) {
    const status = await ctx.getStatus();
    throw new Error(`missing indexed block hash in recent height window near ${status.bestHeight ?? 0}`);
  }
  const hash = await ctx.getBlockHashForHeight(sample.height, true);
  assertNonEmptyString(hash, "GetBlockIndex.blockHash");
}

async function testGetBlock(ctx: TestContext) {
  const sample = await ctx.getSampleIndexedBlock();
  if (!sample) {
    const status = await ctx.getStatus();
    throw new Error(`missing indexed block hash in recent height window near ${status.bestHeight ?? 0}`);
  }
  const block = await ctx.getBlockByHash(sample.hash, true);
  if (!block) {
    throw new Error(`missing block for hash ${sample.hash}`);
  }
  assertEqualString(block.hash, sample.hash, "block hash");
  if (block.height !== sample.height) {
    throw new Error(`block height mismatch: got ${block.height}, want ${sample.height}`);
  }
  if (!block.hasTxField) {
    throw new Error("block response missing txs field");
  }
}

async function testGetBlockByHeight(ctx: TestContext) {
  const sample = await ctx.getSampleIndexedBlock();
  if (!sample) {
    const status = await ctx.getStatus();
    throw new Error(`missing indexed block hash in recent height window near ${status.bestHeight ?? 0}`);
  }

  const path = `/api/v2/block/${sample.height}?page=1&pageSize=${blockPageSize}`;
  const block = await ctx.client.getJson("/api/v2/block/{blockId}", path);
  assertNonEmptyString(block.hash, "GetBlockByHeight.hash");
  if (block.height !== sample.height) {
    throw new Error(`GetBlockByHeight mismatch: got height ${block.height}, want ${sample.height}`);
  }
  if (!Array.isArray(block.txs)) {
    throw new Error("GetBlockByHeight response missing txs field");
  }

  const hashByIndex = await ctx.getBlockHashForHeight(sample.height, true);
  assertEqualString(block.hash, hashByIndex, "GetBlockByHeight block hash");
}

async function testGetTransaction(ctx: TestContext) {
  const txid = await ctx.sampleTxIDOrSkip();
  const tx = await ctx.getTransactionByID(txid, true);
  if (!tx) {
    throw new Error(`missing transaction ${txid}`);
  }
  assertEqualString(tx.txid, txid, "transaction txid");
}

async function testGetTransactionSpecific(ctx: TestContext) {
  const txid = await ctx.sampleTxIDOrSkip();
  const specific = await ctx.client.getJson(
    "/api/v2/tx-specific/{txid}",
    `/api/v2/tx-specific/${encodePathSegment(txid)}`,
  );
  if (!isObject(specific) || Object.keys(specific).length === 0) {
    throw new Error(`empty tx-specific response for ${txid}`);
  }
  const rawTxid = specific.txid;
  if (typeof rawTxid === "string" && rawTxid.trim() !== "" && rawTxid.toLowerCase() !== txid.toLowerCase()) {
    throw new Error(`tx-specific txid mismatch: got ${rawTxid}, want ${txid}`);
  }
}

async function testGetAddress(ctx: TestContext) {
  const address = await ctx.sampleAddressOrSkip();
  const addr = await ctx.client.getJson(
    "/api/v2/address/{address}",
    `/api/v2/address/${encodePathSegment(address)}?details=basic`,
  );
  assertBasicAccountInfoPayload(addr, address, "GetAddress");
}

async function testGetCurrentFiatRates(ctx: TestContext) {
  // sampleFiatTickerOrSkip fetches the unfiltered /api/v2/tickers/, so this verifies the
  // full currency map (e.g. cny, eur, usd): non-empty, every rate > 0, and timestamp fresh.
  const ticker = await ctx.sampleFiatTickerOrSkip();
  assertFiatTickerPayload(ticker, "GetCurrentFiatRates");
  assertFiatTickerFresh(ticker, "GetCurrentFiatRates", fiatMaxAgeSeconds());
}

async function testGetTickersList(ctx: TestContext) {
  const ticker = await ctx.sampleFiatTickerOrSkip();
  const list = await getFiatJSONOrSkip(
    ctx,
    "/api/v2/tickers-list/",
    `/api/v2/tickers-list/?timestamp=${ticker.ts}`,
  );
  if (!positiveNumber(list.ts)) {
    throw new Error(`GetTickersList invalid timestamp: ${String(list.ts)}`);
  }
  if (!Array.isArray(list.available_currencies) || list.available_currencies.length === 0) {
    throw new Error("GetTickersList returned no currencies");
  }
  list.available_currencies.forEach((currency) => assertNonEmptyString(currency, "GetTickersList.available_currencies"));
}

async function testGetMultiTickers(ctx: TestContext) {
  const ticker = await ctx.sampleFiatTickerOrSkip();
  const list = await getFiatJSONOrSkip(
    ctx,
    "/api/v2/tickers-list/",
    `/api/v2/tickers-list/?timestamp=${ticker.ts}`,
  );
  const currency = list.available_currencies?.[0]?.trim().toLowerCase();
  if (!currency) {
    throw new SkipTest(`no available fiat currencies for timestamp ${ticker.ts ?? 0}`);
  }

  const single = await getFiatJSONOrSkip(
    ctx,
    "/api/v2/tickers/",
    `/api/v2/tickers/?timestamp=${ticker.ts}&currency=${encodeURIComponent(currency)}`,
  );
  assertFiatTickerPayload(single, "GetMultiTickers.single");

  const multi = await getFiatJSONOrSkip(
    ctx,
    "/api/v2/multi-tickers/",
    `/api/v2/multi-tickers/?timestamp=${ticker.ts}&currency=${encodeURIComponent(currency)}`,
  );
  if (multi.length !== 1) {
    throw new Error(`GetMultiTickers expected exactly 1 entry, got ${multi.length}`);
  }
  assertFiatTickerPayload(multi[0], "GetMultiTickers.multi[0]");
  if (multi[0].ts !== single.ts) {
    throw new Error(`GetMultiTickers timestamp mismatch: single=${single.ts ?? 0} multi=${multi[0].ts ?? 0}`);
  }
  if (single.rates?.[currency] !== multi[0].rates?.[currency]) {
    throw new Error(`GetMultiTickers rate mismatch for ${currency}: single=${single.rates?.[currency]} multi=${multi[0].rates?.[currency]}`);
  }
}

async function testGetAddressTxids(ctx: TestContext) {
  const { address, txid } = await ctx.sampleAddressTxOrSkip();
  const addr = await ctx.client.getJson(
    "/api/v2/address/{address}",
    buildAddressDetailsPath(address, "txids", addressPage, addressPageSize),
  );
  assertAddressTxidsPayload(addr, address, txid, "GetAddressTxids", addressPageSize);
}

async function testGetAddressTxs(ctx: TestContext) {
  const { address, txid } = await ctx.sampleAddressTxOrSkip();
  const addr = await ctx.client.getJson(
    "/api/v2/address/{address}",
    buildAddressDetailsPath(address, "txs", addressPage, addressPageSize),
  );
  assertAddressTxsPayload(addr, address, txid, "GetAddressTxs", addressPageSize);
}

async function testGetAddressTxsScientificNotation(ctx: TestContext) {
  const found = await ctx.sampleScientificNotationCaseOrSkip();
  const addr = await ctx.client.getJson(
    "/api/v2/address/{address}",
    buildAddressDetailsPathWithRange(found.address, "txs", addressPage, 1000, found.height, found.height),
  );
  assertAddressTxsPayload(addr, found.address, found.txid, "GetAddressTxsScientificNotation", 1000);
}

// HTTP twin of the ws getBalanceHistory test: GET /api/v2/balancehistory/{descriptor}. Bounds the
// window to the sample tx's block time (± a day) so the server scans only a small range instead of
// the address's entire history (which can exceed timeouts on busy chains).
async function testGetBalanceHistory(ctx: TestContext) {
  const address = await ctx.sampleAddressOrSkip();
  const txid = await ctx.sampleTxIDOrSkip();
  const tx = await ctx.getTransactionByID(txid, false);

  const blockTime = tx?.blockTime;
  if (!positiveNumber(blockTime)) {
    throw new SkipTest(`sample tx ${txid} has no block time to bound balance history`);
  }
  const from = blockTime - 1;
  const to = blockTime + 24 * 3600; // clamps to best height; window is bounded by recent blocks

  const result = await ctx.client.getMaybe(
    "/api/v2/balancehistory/{descriptor}",
    `/api/v2/balancehistory/${encodePathSegment(address)}?from=${from}&to=${to}`,
  );
  if (result.status !== 200 || result.data === undefined) {
    // EVM rejects balance history for contract addresses ("...for a contract not allowed"); skip
    // those, mirroring the ws getBalanceHistory test.
    if (result.body.toLowerCase().includes("not allowed")) {
      throw new SkipTest(`balance history not allowed for ${address} (contract)`);
    }
    throw new Error(`GET /api/v2/balancehistory returned HTTP ${result.status}: ${preview(result.body)}`);
  }

  const history = result.data;
  if (!Array.isArray(history)) {
    throw new Error("GetBalanceHistory response is not an array");
  }
  // An empty history is valid (address with no activity in range); the 200 body is already
  // schema-validated against BalanceHistory[] by the client, so just assert the entry invariants.
  history.forEach((entry, i) => {
    if (!positiveNumber(entry.time)) {
      throw new Error(`GetBalanceHistory[${i}] invalid time: ${String(entry.time)}`);
    }
    if (!Number.isInteger(entry.txs) || entry.txs < 0) {
      throw new Error(`GetBalanceHistory[${i}] invalid txs: ${String(entry.txs)}`);
    }
  });
}

export async function getFiatJSONOrSkip<P extends GetOperationPath>(
  ctx: TestContext,
  operationPath: P,
  actualPath: string,
): Promise<GetResponse<P>> {
  const result = await ctx.client.getMaybe(operationPath, actualPath);
  if (result.status === 200 && result.data !== undefined) {
    return result.data;
  }
  if (isFiatDataUnavailable(result.status, result.body)) {
    throw new SkipTest(`fiat data unavailable for ${actualPath} (HTTP ${result.status}: ${preview(result.body)})`);
  }
  throw new Error(`GET ${actualPath} returned HTTP ${result.status}: ${preview(result.body)}`);
}

export const commonTests: Record<string, TestFunction> = {
  Status: testStatus,
  GetBlockIndex: testGetBlockIndex,
  GetBlockByHeight: testGetBlockByHeight,
  GetBlock: testGetBlock,
  GetTransaction: testGetTransaction,
  GetTransactionSpecific: testGetTransactionSpecific,
  GetAddress: testGetAddress,
  GetAddressTxids: testGetAddressTxids,
  GetAddressTxs: testGetAddressTxs,
  GetAddressTxsScientificNotation: testGetAddressTxsScientificNotation,
  GetBalanceHistory: testGetBalanceHistory,
  GetCurrentFiatRates: testGetCurrentFiatRates,
  GetTickersList: testGetTickersList,
  GetMultiTickers: testGetMultiTickers,
};
