import { SkipTest } from "../errors.js";
import {
  assertConfirmedUTXOsIncludedByOutpoint,
  assertUTXOList,
  assertUTXOListConfirmed,
  assertUTXOSetsEqualByOutpoint,
  encodePathSegment,
  utxoSetsEqualByOutpoint,
} from "../support.js";

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

export const utxoOnlyTests: Record<string, TestFunction> = {
  GetUtxo: testGetUtxo,
  GetUtxoConfirmedFilter: testGetUtxoConfirmedFilter,
};
