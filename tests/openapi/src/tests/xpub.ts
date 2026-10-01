import { SkipTest } from "../errors.js";
import { loadAPITestData } from "../fixtures.js";
import { preview } from "../openapi.js";
import {
  assertEqualString,
  assertNonEmptyString,
  assertPageMeta,
  assertPageSizeUpperBound,
  assertStringSlicesEqual,
  encodePathSegment,
  equalFold,
  optionalBigInt,
  positiveNumber,
  txIDsFromTransactions,
} from "../support.js";
import { assertConfirmedUTXOsIncludedByOutpoint, assertUTXOSetsEqualByOutpoint, isUnconfirmedUtxo, utxoOutpointKey } from "./utxo.js";

import type { TestContext } from "../context.js";
import type { AddressResponse, BalanceHistoryResponse, TokenResponse, TxResponse, UtxoResponse, XpubFixture } from "../types.js";

type TestFunction = (ctx: TestContext) => Promise<void>;

// Small page so even the lightest fixture (a handful of txs) still exercises the paging arithmetic.
const xpubPageSize = 5;
// One bucket per year: the balance-history test only needs lifetime totals, not the curve.
const lifetimeGroupBy = 365 * 24 * 3600;
// m/purpose'/coin'/account'/change/index, as emitted by api/xpub.go tokenFromXpubAddress.
const derivationPathPattern = /^m\/\d+'\/\d+'\/\d+'\/\d+\/\d+$/;
const extendedKeyPattern = /[1-9A-HJ-NP-Za-km-z]{100,120}/;

function xpubFixturesOrSkip(ctx: TestContext): XpubFixture[] {
  const fixtures = loadAPITestData(ctx.coin).xpubFixtures ?? [];
  if (fixtures.length === 0) {
    throw new SkipTest(`openapi/fixtures/${ctx.coin}.json has no xpubFixtures entries`);
  }
  return fixtures;
}

function xpubPath(descriptor: string, query: string) {
  return `/api/v2/xpub/${encodePathSegment(descriptor)}?${query}`;
}

function getXpub(ctx: TestContext, descriptor: string, query: string) {
  return ctx.client.getJson("/api/v2/xpub/{xpub}", xpubPath(descriptor, query));
}

function getXpubUtxos(ctx: TestContext, descriptor: string, confirmedOnly: boolean) {
  const suffix = confirmedOnly ? "?confirmed=true" : "";
  return ctx.client.getJson("/api/v2/utxo/{descriptor}", `/api/v2/utxo/${encodePathSegment(descriptor)}${suffix}`);
}

function requiredBigInt(value: unknown, context: string) {
  const parsed = optionalBigInt(value, context);
  if (parsed === undefined) {
    throw new Error(`${context} is missing`);
  }
  return parsed;
}

function nonNegativeInt(value: unknown, context: string): number {
  if (typeof value !== "number" || !Number.isInteger(value) || value < 0) {
    throw new Error(`${context} is not a non-negative integer: ${String(value)}`);
  }
  return value;
}

function sumBigInt(values: bigint[]) {
  return values.reduce((acc, value) => acc + value, 0n);
}

// Shared shape check for every xpub account response: lifetime totals reconcile with the
// balance and the fixture floors hold. usedTokens and addrTxCount are omitted when zero.
function assertXpubAccount(resp: AddressResponse, fixture: XpubFixture, context: string) {
  assertEqualString(resp.address, fixture.descriptor, `${context}.address`);
  const balance = requiredBigInt(resp.balance, `${context}.balance`);
  const received = requiredBigInt(resp.totalReceived, `${context}.totalReceived`);
  const sent = requiredBigInt(resp.totalSent, `${context}.totalSent`);
  if (received - sent !== balance) {
    throw new Error(`${context} totals do not reconcile: totalReceived ${received} - totalSent ${sent} != balance ${balance}`);
  }
  nonNegativeInt(resp.unconfirmedTxs, `${context}.unconfirmedTxs`);
  const usedTokens = nonNegativeInt(resp.usedTokens ?? 0, `${context}.usedTokens`);
  if (usedTokens < fixture.minUsedAddresses) {
    throw new Error(`${context} usedTokens ${usedTokens} is below the fixture floor ${fixture.minUsedAddresses} (${fixture.name}); is the index missing history?`);
  }
  // addrTxCount sums per-address counts, so it can never drop below the exact tx count the fixture pins.
  const addrTxCount = nonNegativeInt(resp.addrTxCount ?? 0, `${context}.addrTxCount`);
  if (addrTxCount < fixture.minTxs) {
    throw new Error(`${context} addrTxCount ${addrTxCount} is below the fixture floor ${fixture.minTxs} (${fixture.name}); is the index missing history?`);
  }
  if (nonNegativeInt(resp.txs, `${context}.txs`) > addrTxCount) {
    throw new Error(`${context} txs ${resp.txs} exceeds addrTxCount ${addrTxCount}`);
  }
  return { balance, received, sent, usedTokens };
}

// A derived-address row must carry a resolvable address and a full derivation path; an address
// that was never used cannot hold funds.
function assertXpubAddressToken(token: TokenResponse, context: string) {
  if (token.type !== "XPUBAddress" || token.standard !== "XPUBAddress") {
    throw new Error(`${context} is not an XPUBAddress row: type=${token.type} standard=${token.standard}`);
  }
  assertNonEmptyString(token.name, `${context}.name`);
  if (typeof token.path !== "string" || !derivationPathPattern.test(token.path)) {
    throw new Error(`${context}.path is not a full derivation path: ${String(token.path)}`);
  }
  const transfers = nonNegativeInt(token.transfers, `${context}.transfers`);
  nonNegativeInt(token.decimals, `${context}.decimals`);
  const balance = optionalBigInt(token.balance, `${context}.balance`) ?? 0n;
  if (transfers === 0 && balance !== 0n) {
    throw new Error(`${context} has balance ${balance} without any transfer`);
  }
  return { transfers, balance };
}

function tokenNames(tokens: TokenResponse[]) {
  return tokens.map((token) => token.name);
}

function assertUniquePaths(tokens: TokenResponse[], context: string) {
  const seen = new Set<string>();
  for (const token of tokens) {
    const path = token.path ?? "";
    if (seen.has(path)) {
      throw new Error(`${context} lists derivation path ${path} twice`);
    }
    seen.add(path);
  }
}

function txAddresses(tx: TxResponse) {
  const addresses: string[] = [];
  for (const input of tx.vin) {
    addresses.push(...(input.addresses ?? []));
  }
  for (const output of tx.vout) {
    addresses.push(...(output.addresses ?? []));
  }
  return addresses;
}

// Every transaction listed for an account must touch at least one of its own addresses.
function assertTxsTouchOwnAddresses(txs: TxResponse[], own: Set<string>, context: string) {
  txs.forEach((tx, index) => {
    if (!txAddresses(tx).some((address) => own.has(address))) {
      throw new Error(`${context}.transactions[${index}] ${tx.txid} touches none of the account's addresses`);
    }
  });
}

function assertDisjointTxids(a: string[], b: string[], context: string) {
  const set = new Set(a.map((txid) => txid.toLowerCase()));
  for (const txid of b) {
    if (set.has(txid.toLowerCase())) {
      throw new Error(`${context} repeats txid ${txid} across pages`);
    }
  }
}

function assertXpubUtxo(utxo: UtxoResponse, own: Set<string>, context: string) {
  assertNonEmptyString(utxo.txid, `${context}.txid`);
  nonNegativeInt(utxo.vout, `${context}.vout`);
  requiredBigInt(utxo.value, `${context}.value`);
  assertNonEmptyString(utxo.address, `${context}.address`);
  if (!own.has(utxo.address)) {
    throw new Error(`${context} address ${utxo.address} is not one of the account's used addresses`);
  }
  if (typeof utxo.path !== "string" || !derivationPathPattern.test(utxo.path)) {
    throw new Error(`${context}.path is not a full derivation path: ${String(utxo.path)}`);
  }
}

async function usedAddressSet(ctx: TestContext, descriptor: string) {
  const resp = await getXpub(ctx, descriptor, "details=tokens&tokens=used");
  return new Set(tokenNames(resp.tokens ?? []));
}

async function testGetXpubBasic(ctx: TestContext) {
  for (const fixture of xpubFixturesOrSkip(ctx)) {
    const resp = await getXpub(ctx, fixture.descriptor, "details=basic");
    assertXpubAccount(resp, fixture, `GetXpubBasic[${fixture.name}]`);
    if (resp.tokens !== undefined || resp.txids !== undefined || resp.transactions !== undefined) {
      throw new Error(`GetXpubBasic[${fixture.name}] details=basic leaked tokens/txids/transactions`);
    }
    // basic skips the per-tx merge and reports the per-address estimate as txs
    if (resp.txs !== (resp.addrTxCount ?? 0)) {
      throw new Error(`GetXpubBasic[${fixture.name}] details=basic txs ${resp.txs} != addrTxCount ${resp.addrTxCount ?? 0}`);
    }
  }
}

// The three tokens filters are views of one derivation: used ⊂ derived, nonzero ⊂ used, the
// nonzero balances sum to the account balance, and usedTokens is the same total under every filter.
async function testGetXpubTokens(ctx: TestContext) {
  for (const fixture of xpubFixturesOrSkip(ctx)) {
    const context = `GetXpubTokens[${fixture.name}]`;
    const used = await getXpub(ctx, fixture.descriptor, "details=tokenBalances&tokens=used");
    const derived = await getXpub(ctx, fixture.descriptor, "details=tokenBalances&tokens=derived");
    const nonzero = await getXpub(ctx, fixture.descriptor, "details=tokenBalances&tokens=nonzero");
    const account = assertXpubAccount(used, fixture, `${context}.used`);
    assertXpubAccount(derived, fixture, `${context}.derived`);
    assertXpubAccount(nonzero, fixture, `${context}.nonzero`);
    if (derived.usedTokens !== used.usedTokens || nonzero.usedTokens !== used.usedTokens) {
      throw new Error(`${context} usedTokens differs across filters: used=${used.usedTokens} derived=${derived.usedTokens} nonzero=${nonzero.usedTokens}`);
    }

    const usedTokens = used.tokens ?? [];
    usedTokens.forEach((token, index) => {
      const { transfers, balance } = assertXpubAddressToken(token, `${context}.used.tokens[${index}]`);
      if (transfers === 0) {
        throw new Error(`${context}.used.tokens[${index}] ${token.name} has no transfers`);
      }
      const received = requiredBigInt(token.totalReceived, `${context}.used.tokens[${index}].totalReceived`);
      const sent = requiredBigInt(token.totalSent, `${context}.used.tokens[${index}].totalSent`);
      if (received - sent !== balance) {
        throw new Error(`${context}.used.tokens[${index}] ${token.name} totals do not reconcile: ${received} - ${sent} != ${balance}`);
      }
    });
    assertUniquePaths(usedTokens, `${context}.used`);
    if (usedTokens.length !== account.usedTokens) {
      throw new Error(`${context} tokens=used returned ${usedTokens.length} rows but usedTokens=${account.usedTokens}`);
    }
    if (sumBigInt(usedTokens.map((token) => optionalBigInt(token.balance, `${context}.used.balance`) ?? 0n)) !== account.balance) {
      throw new Error(`${context} used address balances do not sum to the account balance ${account.balance}`);
    }

    const derivedTokens = derived.tokens ?? [];
    derivedTokens.forEach((token, index) => assertXpubAddressToken(token, `${context}.derived.tokens[${index}]`));
    assertUniquePaths(derivedTokens, `${context}.derived`);
    // derived adds the unused gap addresses after the last used one on every chain
    if (derivedTokens.length <= usedTokens.length) {
      throw new Error(`${context} tokens=derived returned ${derivedTokens.length} rows, expected more than the ${usedTokens.length} used`);
    }
    const derivedNames = new Set(tokenNames(derivedTokens));
    for (const name of tokenNames(usedTokens)) {
      if (!derivedNames.has(name)) {
        throw new Error(`${context} used address ${name} is missing from tokens=derived`);
      }
    }

    const usedNames = new Set(tokenNames(usedTokens));
    const nonzeroTokens = nonzero.tokens ?? [];
    const nonzeroBalances = nonzeroTokens.map((token, index) => {
      const { balance } = assertXpubAddressToken(token, `${context}.nonzero.tokens[${index}]`);
      if (!usedNames.has(token.name)) {
        throw new Error(`${context}.nonzero.tokens[${index}] ${token.name} is not a used address`);
      }
      if (balance <= 0n) {
        throw new Error(`${context}.nonzero.tokens[${index}] ${token.name} has non-positive balance ${balance}`);
      }
      return balance;
    });
    if (sumBigInt(nonzeroBalances) !== account.balance) {
      throw new Error(`${context} nonzero balances sum to ${sumBigInt(nonzeroBalances)}, account balance is ${account.balance}`);
    }
  }
}

// Paging over the merged per-address history: exact page count, no repeats across pages, the
// last page holds the remainder, and details=txs pages identically to details=txids.
async function testGetXpubTxsPagination(ctx: TestContext) {
  let validated = 0;
  for (const fixture of xpubFixturesOrSkip(ctx)) {
    const context = `GetXpubTxsPagination[${fixture.name}]`;
    const page1 = await getXpub(ctx, fixture.descriptor, `details=txids&page=1&pageSize=${xpubPageSize}`);
    assertXpubAccount(page1, fixture, `${context}.page1`);
    if (page1.unconfirmedTxs > 0) {
      // mempool txs are prepended to the paged sequence, which makes the counts below unstable
      continue;
    }
    assertPageMeta(page1.page, page1.itemsOnPage, page1.totalPages, page1.txs, `${context}.page1`);
    if (page1.txs < fixture.minTxs) {
      throw new Error(`${context} txs ${page1.txs} is below the fixture floor ${fixture.minTxs}`);
    }
    const totalPages = Math.max(1, Math.ceil(page1.txs / xpubPageSize));
    if (page1.totalPages !== totalPages) {
      throw new Error(`${context} totalPages ${page1.totalPages ?? 0} != ceil(${page1.txs}/${xpubPageSize}) = ${totalPages}`);
    }
    const txids1 = page1.txids ?? [];
    assertPageSizeUpperBound(txids1.length, page1.itemsOnPage ?? 0, xpubPageSize, `${context}.page1.txids`);
    if (txids1.length !== Math.min(xpubPageSize, page1.txs)) {
      throw new Error(`${context}.page1 returned ${txids1.length} txids, want ${Math.min(xpubPageSize, page1.txs)}`);
    }
    txids1.forEach((txid) => assertNonEmptyString(txid, `${context}.page1.txids`));

    const txsPage1 = await getXpub(ctx, fixture.descriptor, `details=txs&tokens=used&page=1&pageSize=${xpubPageSize}`);
    assertXpubAccount(txsPage1, fixture, `${context}.txs.page1`);
    assertStringSlicesEqual(txIDsFromTransactions(txsPage1.transactions ?? [], `${context}.txs.page1`), txids1, `${context} details=txs vs details=txids`);
    assertTxsTouchOwnAddresses(txsPage1.transactions ?? [], new Set(tokenNames(txsPage1.tokens ?? [])), `${context}.txs.page1`);

    if (totalPages > 1) {
      const last = await getXpub(ctx, fixture.descriptor, `details=txids&page=${totalPages}&pageSize=${xpubPageSize}`);
      if (last.page !== totalPages) {
        throw new Error(`${context}.last page ${last.page ?? 0} != requested ${totalPages}`);
      }
      const txidsLast = last.txids ?? [];
      const remainder = page1.txs - (totalPages - 1) * xpubPageSize;
      if (txidsLast.length !== remainder) {
        throw new Error(`${context}.last returned ${txidsLast.length} txids, want the remainder ${remainder}`);
      }
      assertDisjointTxids(txids1, txidsLast, `${context} page1 vs last`);
    }
    validated++;
  }
  if (validated === 0) {
    throw new SkipTest("GetXpubTxsPagination: every fixture had mempool activity, paging counts unstable");
  }
}

// Confirmed UTXOs are the balance: their values sum to it exactly, each one belongs to a used
// address with its derivation path, and the unconfirmed view is a superset.
async function testGetXpubUtxo(ctx: TestContext) {
  for (const fixture of xpubFixturesOrSkip(ctx)) {
    const context = `GetXpubUtxo[${fixture.name}]`;
    const own = await usedAddressSet(ctx, fixture.descriptor);

    let basic = await getXpub(ctx, fixture.descriptor, "details=basic");
    let confirmed = await getXpubUtxos(ctx, fixture.descriptor, true);
    const utxoSum = (utxos: UtxoResponse[]) => sumBigInt(utxos.map((utxo) => requiredBigInt(utxo.value, `${context}.value`)));
    if (utxoSum(confirmed) !== requiredBigInt(basic.balance, `${context}.balance`)) {
      // a block landing between the two calls moves both; re-read once before judging
      basic = await getXpub(ctx, fixture.descriptor, "details=basic");
      confirmed = await getXpubUtxos(ctx, fixture.descriptor, true);
    }
    const balance = assertXpubAccount(basic, fixture, `${context}.basic`).balance;
    confirmed.forEach((utxo, index) => {
      assertXpubUtxo(utxo, own, `${context}.confirmed[${index}]`);
      if (isUnconfirmedUtxo(utxo)) {
        throw new Error(`${context}.confirmed[${index}] ${utxoOutpointKey(utxo)} is unconfirmed`);
      }
    });
    if (utxoSum(confirmed) !== balance) {
      throw new Error(`${context} confirmed UTXOs sum to ${utxoSum(confirmed)}, balance is ${balance}`);
    }

    const all = await getXpubUtxos(ctx, fixture.descriptor, false);
    all.forEach((utxo, index) => assertXpubUtxo(utxo, own, `${context}.all[${index}]`));
    assertConfirmedUTXOsIncludedByOutpoint(all, confirmed, `${context}.all-vs-confirmed`);
  }
}

// Lifetime received minus sent over the whole history must land exactly on the confirmed balance
// and the buckets must account for every confirmed transaction.
async function testGetXpubBalanceHistory(ctx: TestContext) {
  for (const fixture of xpubFixturesOrSkip(ctx)) {
    const context = `GetXpubBalanceHistory[${fixture.name}]`;
    const account = await getXpub(ctx, fixture.descriptor, "details=txids&page=1&pageSize=1");
    const { balance } = assertXpubAccount(account, fixture, `${context}.account`);

    const result = await ctx.client.getMaybe(
      "/api/v2/balancehistory/{descriptor}",
      `/api/v2/balancehistory/${encodePathSegment(fixture.descriptor)}?groupBy=${lifetimeGroupBy}`,
    );
    if (result.status !== 200 || result.data === undefined) {
      if (result.body.includes("spans more than")) {
        throw new SkipTest(`${context}: history exceeds the instance's balance-history tx cap`);
      }
      throw new Error(`${context} returned HTTP ${result.status}: ${preview(result.body)}`);
    }
    const history: BalanceHistoryResponse[] = result.data;
    let previousTime = 0;
    let txs = 0;
    let net = 0n;
    history.forEach((entry, index) => {
      if (!positiveNumber(entry.time) || entry.time <= previousTime) {
        throw new Error(`${context}[${index}] time ${String(entry.time)} is not increasing`);
      }
      previousTime = entry.time;
      const bucketTxs = nonNegativeInt(entry.txs, `${context}[${index}].txs`);
      if (bucketTxs === 0) {
        throw new Error(`${context}[${index}] is an empty bucket`);
      }
      txs += bucketTxs;
      net += requiredBigInt(entry.received, `${context}[${index}].received`) - requiredBigInt(entry.sent, `${context}[${index}].sent`);
      optionalBigInt(entry.sentToSelf, `${context}[${index}].sentToSelf`);
    });
    if (net !== balance) {
      throw new Error(`${context} lifetime received - sent = ${net}, balance is ${balance}`);
    }
    // txs from details=txids is the exact confirmed count; the history buckets must cover it
    if (txs !== account.txs) {
      throw new Error(`${context} buckets account for ${txs} txs, history has ${account.txs}`);
    }
  }
}

// A malformed xpub is caller input: it must come back as HTTP 400 with the public error
// envelope, never as a 500 "Internal server error".
async function testGetXpubInvalid(ctx: TestContext) {
  const [fixture] = xpubFixturesOrSkip(ctx);
  const key = extendedKeyPattern.exec(fixture.descriptor)?.[0];
  if (!key) {
    throw new Error(`GetXpubInvalid: fixture ${fixture.name} has no extended key to corrupt`);
  }
  const lastChar = key.at(-1) === "1" ? "2" : "1";
  const corrupted = fixture.descriptor.replace(key, key.slice(0, -1) + lastChar);
  for (const bad of ["not-an-xpub", corrupted]) {
    const result = await ctx.client.getMaybe("/api/v2/xpub/{xpub}", xpubPath(bad, "details=basic"));
    if (result.status !== 400) {
      throw new Error(`GetXpubInvalid: ${bad} returned HTTP ${result.status}, want 400: ${preview(result.body)}`);
    }
    ctx.contract.validateSchemaRef("#/components/schemas/ErrorResponse", `GetXpubInvalid ${bad} error body`, result.data);
    assertNonEmptyString((result.data as { error?: unknown }).error, "GetXpubInvalid.error");
  }
}

function sameXpubAccount(a: AddressResponse, b: AddressResponse) {
  return a.balance === b.balance && a.totalReceived === b.totalReceived && a.totalSent === b.totalSent
    && a.txs === b.txs && a.addrTxCount === b.addrTxCount && a.usedTokens === b.usedTokens
    && a.unconfirmedTxs === b.unconfirmedTxs;
}

// HTTP and WebSocket getAccountInfo share the xpub cache, so the same request must yield the same
// totals and the same used-address rows. Re-read once when they differ: a block landing between
// the two calls legitimately changes both.
async function testWsGetAccountInfoXpub(ctx: TestContext) {
  for (const fixture of xpubFixturesOrSkip(ctx)) {
    const context = `WsGetAccountInfoXpub[${fixture.name}]`;
    const fetchBoth = async () => {
      const http = await getXpub(ctx, fixture.descriptor, "details=tokenBalances&tokens=used");
      const ws = await ctx.wsCall<AddressResponse>(
        "getAccountInfo",
        { descriptor: fixture.descriptor, details: "tokenBalances", tokens: "used" },
        "#/components/schemas/Address",
      );
      return { http, ws };
    };
    let { http, ws } = await fetchBoth();
    if (!sameXpubAccount(ws, http)) {
      ({ http, ws } = await fetchBoth());
    }
    assertXpubAccount(ws, fixture, `${context}.ws`);
    assertXpubAccount(http, fixture, `${context}.http`);
    if (!sameXpubAccount(ws, http)) {
      throw new Error(`${context} totals differ: ws=${JSON.stringify({ balance: ws.balance, txs: ws.txs, usedTokens: ws.usedTokens })} http=${JSON.stringify({ balance: http.balance, txs: http.txs, usedTokens: http.usedTokens })}`);
    }
    const rows = (tokens: TokenResponse[]) => tokens.map((token) => `${token.path ?? ""} ${token.name} ${token.balance ?? ""} ${token.transfers}`).sort();
    assertStringSlicesEqual(rows(ws.tokens ?? []), rows(http.tokens ?? []), `${context} used address rows`);
  }
}

// HTTP and WebSocket UTXO views of an xpub must agree outpoint by outpoint, including the
// derived address and path attached to each one.
async function testWsGetAccountUtxoXpub(ctx: TestContext) {
  for (const fixture of xpubFixturesOrSkip(ctx)) {
    const context = `WsGetAccountUtxoXpub[${fixture.name}]`;
    const fetchBoth = async () => {
      const http = await getXpubUtxos(ctx, fixture.descriptor, false);
      const ws = await ctx.wsCall<UtxoResponse[]>("getAccountUtxo", { descriptor: fixture.descriptor });
      return { http, ws };
    };
    let { http, ws } = await fetchBoth();
    if (!Array.isArray(ws)) {
      throw new Error(`${context} ws response is not an array`);
    }
    if (ws.length !== http.length) {
      ({ http, ws } = await fetchBoth());
    }
    ws.forEach((utxo, index) => ctx.contract.validateSchemaRef("#/components/schemas/Utxo", `${context}.ws[${index}]`, utxo));
    assertUTXOSetsEqualByOutpoint(ws, http, `${context} outpoints`);
    const byOutpoint = new Map(http.map((utxo) => [utxoOutpointKey(utxo), utxo]));
    ws.forEach((utxo, index) => {
      const twin = byOutpoint.get(utxoOutpointKey(utxo));
      if (!twin || !equalFold(twin.address, utxo.address) || twin.path !== utxo.path || twin.value !== utxo.value) {
        throw new Error(`${context}.ws[${index}] ${utxoOutpointKey(utxo)} differs from HTTP: ws=${JSON.stringify(utxo)} http=${JSON.stringify(twin)}`);
      }
    });
  }
}

export const xpubTests: Record<string, TestFunction> = {
  GetXpubBasic: testGetXpubBasic,
  GetXpubTokens: testGetXpubTokens,
  GetXpubTxsPagination: testGetXpubTxsPagination,
  GetXpubUtxo: testGetXpubUtxo,
  GetXpubBalanceHistory: testGetXpubBalanceHistory,
  GetXpubInvalid: testGetXpubInvalid,
};

export const wsXpubTests: Record<string, TestFunction> = {
  WsGetAccountInfoXpub: testWsGetAccountInfoXpub,
  WsGetAccountUtxoXpub: testWsGetAccountUtxoXpub,
};
