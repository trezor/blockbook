import { SkipTest } from "../errors.js";
import { assertUTXOList, encodePathSegment, stringValue } from "../support.js";

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

export const utxoOnlyTests: Record<string, TestFunction> = {
  GetUtxo: testGetUtxo,
  GetUtxoConfirmedFilter: testGetUtxoConfirmedFilter,
};
