import { blockFilterConfig } from "../config.js";
import { SkipTest } from "../errors.js";
import { assertGolombParams, assertUTXOList, encodePathSegment, stringValue } from "../support.js";

import type { TestContext } from "../context.js";
import type { UtxoResponse } from "../types.js";

type TestFunction = (ctx: TestContext) => Promise<void>;

async function testGetUtxo(ctx: TestContext) {
  const address = await ctx.sampleAddressOrSkip();
  const utxos = await ctx.client.getJson(
    "/api/v2/utxo/{descriptor}",
    `/api/v2/utxo/${encodePathSegment(address)}?confirmed=true`,
  );
  assertUTXOList(utxos, "GetUtxo");
}

async function testGetUtxoConfirmedFilter(ctx: TestContext) {
  const address = await ctx.sampleAddressOrSkip();
  const confirmed = await ctx.client.getJson(
    "/api/v2/utxo/{descriptor}",
    `/api/v2/utxo/${encodePathSegment(address)}?confirmed=true`,
  );
  let all = await ctx.client.getJson(
    "/api/v2/utxo/{descriptor}",
    `/api/v2/utxo/${encodePathSegment(address)}`,
  );
  let explicitFalse = await ctx.client.getJson(
    "/api/v2/utxo/{descriptor}",
    `/api/v2/utxo/${encodePathSegment(address)}?confirmed=false`,
  );

  if (all.length === 0 && explicitFalse.length === 0 && confirmed.length === 0) {
    throw new SkipTest(`address ${address} currently has no UTXOs`);
  }

  assertUTXOListConfirmed(confirmed, "GetUtxoConfirmedFilter");
  assertUTXOList(all, "GetUtxoConfirmedFilter.all");
  assertUTXOList(explicitFalse, "GetUtxoConfirmedFilter.confirmed=false");

  if (!utxoSetsEqualByOutpoint(all, explicitFalse)) {
    all = await ctx.client.getJson("/api/v2/utxo/{descriptor}", `/api/v2/utxo/${encodePathSegment(address)}`);
    explicitFalse = await ctx.client.getJson("/api/v2/utxo/{descriptor}", `/api/v2/utxo/${encodePathSegment(address)}?confirmed=false`);
    assertUTXOSetsEqualByOutpoint(all, explicitFalse, "GetUtxoConfirmedFilter.default-vs-confirmed=false");
  }

  assertConfirmedUTXOsIncludedByOutpoint(explicitFalse, confirmed, "GetUtxoConfirmedFilter.confirmed-false-vs-true");
}

function assertUTXOListConfirmed(utxos: UtxoResponse[], context: string) {
  assertUTXOList(utxos, context);
  utxos.forEach((utxo) => {
    if (isUnconfirmedUtxo(utxo)) {
      throw new Error(`${context} returned unconfirmed UTXO: txid=${utxo.txid} vout=${utxo.vout} confirmations=${utxo.confirmations} height=${utxo.height ?? 0}`);
    }
  });
}

function assertUTXOSetsEqualByOutpoint(got: UtxoResponse[], want: UtxoResponse[], context: string) {
  const gotSet = utxoSetByOutpoint(got, `${context}.got`);
  const wantSet = utxoSetByOutpoint(want, `${context}.want`);
  if (gotSet.size !== wantSet.size) {
    throw new Error(`${context} outpoint count mismatch: got=${gotSet.size} want=${wantSet.size}`);
  }
  for (const key of wantSet.keys()) {
    if (!gotSet.has(key)) {
      throw new Error(`${context} missing outpoint in got set: ${key}`);
    }
  }
}

function assertConfirmedUTXOsIncludedByOutpoint(mixed: UtxoResponse[], confirmed: UtxoResponse[], context: string) {
  const confirmedSet = utxoSetByOutpoint(confirmed, `${context}.confirmed`);
  for (const utxo of mixed) {
    if (isUnconfirmedUtxo(utxo)) {
      continue;
    }
    const key = utxoOutpointKey(utxo);
    if (!confirmedSet.has(key)) {
      throw new Error(`${context} missing confirmed outpoint ${key} in confirmed=true response`);
    }
  }
}

function utxoSetsEqualByOutpoint(a: UtxoResponse[], b: UtxoResponse[]) {
  if (a.length !== b.length) {
    return false;
  }
  const set = new Set(a.map(utxoOutpointKey));
  if (set.size !== a.length) {
    return false;
  }
  return b.every((utxo) => set.has(utxoOutpointKey(utxo)));
}

function utxoSetByOutpoint(utxos: UtxoResponse[], context: string) {
  const set = new Map<string, UtxoResponse>();
  for (const utxo of utxos) {
    const key = utxoOutpointKey(utxo);
    if (set.has(key)) {
      throw new Error(`${context} duplicate outpoint: ${key}`);
    }
    set.set(key, utxo);
  }
  return set;
}

function utxoOutpointKey(utxo: UtxoResponse) {
  return `${stringValue(utxo.txid).trim().toLowerCase()}:${String(utxo.vout ?? 0)}`;
}

function isUnconfirmedUtxo(utxo: UtxoResponse) {
  return (utxo.confirmations ?? 0) <= 0 || (utxo.height ?? 0) <= 0;
}

// HTTP twin of the ws getBlockFilter* tests: /api/v2/block-filters returns the same Golomb
// P/M/zeroedKey header plus a {height: {blockHash, filter}} map. Gated on the coin's configured
// block_filter_scripts (skips when filters are not enabled) and on the utxo capability.
async function testGetBlockFilters(ctx: TestContext) {
  const { scriptType, golombP } = blockFilterConfig(ctx.coin);
  if (!scriptType) {
    throw new SkipTest(`${ctx.coin} has no block_filter_scripts configured`);
  }

  // lastN=5 anchors the window to the last 5 blocks from the tip, so the server scans only a
  // handful of blocks and the result is guaranteed non-empty.
  const lastN = 5;
  const res = await ctx.client.getJson(
    "/api/v2/block-filters/",
    `/api/v2/block-filters/?scriptType=${encodeURIComponent(scriptType)}&lastN=${lastN}`,
  );
  assertGolombParams(res, golombP, "GetBlockFilters");

  const filters = res.blockFilters ?? {};
  const heights = Object.keys(filters);
  if (heights.length === 0 || heights.length > lastN) {
    throw new Error(`GetBlockFilters expected 1..${lastN} entries, got ${heights.length}`);
  }
  for (const [height, entry] of Object.entries(filters)) {
    if (!/^[0-9]+$/.test(height)) {
      throw new Error(`GetBlockFilters invalid height key: ${height}`);
    }
    if (typeof entry.blockHash !== "string" || !/^[0-9a-f]+$/i.test(entry.blockHash)) {
      throw new Error(`GetBlockFilters[${height}] invalid blockHash: ${String(entry.blockHash)}`);
    }
    // filter hex can be empty for a block with no matching scripts; require hex chars when present.
    if (typeof entry.filter !== "string" || !/^[0-9a-f]*$/i.test(entry.filter)) {
      throw new Error(`GetBlockFilters[${height}] invalid filter hex: ${String(entry.filter)}`);
    }
  }
}

// Negative twin: an unknown scriptType must be rejected (public.go apiBlockFilters returns a 400
// API error "Invalid scriptType ..."), never silently served.
async function testGetBlockFiltersInvalidScriptType(ctx: TestContext) {
  const { scriptType } = blockFilterConfig(ctx.coin);
  if (!scriptType) {
    throw new SkipTest(`${ctx.coin} has no block_filter_scripts configured`);
  }
  const result = await ctx.client.getMaybe(
    "/api/v2/block-filters/",
    `/api/v2/block-filters/?scriptType=bogus_${scriptType}&lastN=1`,
  );
  if (result.status === 200) {
    throw new Error("GetBlockFiltersInvalidScriptType: server accepted an invalid scriptType (expected non-200)");
  }
}

export const utxoOnlyTests: Record<string, TestFunction> = {
  GetUtxo: testGetUtxo,
  GetUtxoConfirmedFilter: testGetUtxoConfirmedFilter,
  GetBlockFilters: testGetBlockFilters,
  GetBlockFiltersInvalidScriptType: testGetBlockFiltersInvalidScriptType,
};
