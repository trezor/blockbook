import { SkipTest } from "../errors.js";
import { loadAPITestData } from "../fixtures.js";
import { addressPage, addressPageSize, evmHistoryPage, evmHistoryPageSize } from "../constants.js";
import {
  assertAddressMatches,
  assertErc4626Payload,
  assertEVMTokenBalancesHaveHoldingsFields,
  assertEVMBasicAddressPayload,
  assertEVMTokenBalancesPayload,
  assertEVMTokenListContractsMatch,
  assertEqualString,
  assertFeeInvariantGE,
  assertNonEmptyString,
  assertPageMeta,
  assertPageSizeUpperBound,
  assertTokenDecimals,
  buildAddressDetailsPath,
  encodePathSegment,
  equalFold,
  isObject,
  optionalBigInt,
  positiveNumber,
  txIDsFromTransactions,
} from "../support.js";

import type { TestContext } from "../context.js";
import type { AddressResponse, ContractInfoResponse, Erc4626Fixture } from "../types.js";

type TestFunction = (ctx: TestContext) => Promise<void>;

async function testGetAddressBasicEVM(ctx: TestContext) {
  const address = await ctx.sampleEVMAddressOrSkip();
  const resp = await ctx.client.getJson(
    "/api/v2/address/{address}",
    buildAddressDetailsPath(address, "basic", addressPage, addressPageSize),
  );
  assertEVMBasicAddressPayload(resp, address, "GetAddressBasicEVM");
}

async function testGetAddressConfirmedNonceEVM(ctx: TestContext) {
  const address = await ctx.sampleEVMAddressOrSkip();
  const base = buildAddressDetailsPath(address, "basic", addressPage, addressPageSize);

  // Gate: confirmedNonce must never be returned without the opt-in flag. This holds on every
  // backend version (absent because not requested, or absent because the feature predates it).
  const off = await ctx.client.getJson("/api/v2/address/{address}", base);
  assertEVMBasicAddressPayload(off, address, "GetAddressConfirmedNonceEVM.off");
  if (off.confirmedNonce !== undefined) {
    throw new Error(
      `opt-in gate broken: confirmedNonce=${JSON.stringify(off.confirmedNonce)} returned without ?confirmedNonce=true`,
    );
  }

  // Opt in. On a backend that has not deployed this feature yet the field stays absent, so skip
  // rather than fail; once deployed, validate the value and its relation to the pending nonce.
  const on = await ctx.client.getJson("/api/v2/address/{address}", `${base}&confirmedNonce=true`);
  assertEVMBasicAddressPayload(on, address, "GetAddressConfirmedNonceEVM.on");
  if (on.confirmedNonce === undefined) {
    throw new SkipTest(`${ctx.coin} backend did not return confirmedNonce (feature may not be deployed yet)`);
  }
  assertNonEmptyString(on.confirmedNonce, "GetAddressConfirmedNonceEVM.confirmedNonce");
  const confirmed = Number(on.confirmedNonce);
  const pending = Number(on.nonce);
  if (!Number.isInteger(confirmed) || confirmed < 0) {
    throw new Error(`confirmedNonce is not a non-negative integer string: ${JSON.stringify(on.confirmedNonce)}`);
  }
  // pending counts mempool txs too, so it can never be below the confirmed (mined) nonce
  if (Number.isInteger(pending) && confirmed > pending) {
    throw new Error(`confirmedNonce (${confirmed}) must not exceed pending nonce (${pending})`);
  }
}

async function addressPaginationEVM(ctx: TestContext, details: "txids" | "txs", testName: string) {
  const address = await ctx.sampleEVMAddressOrSkip();
  const itemsField = details === "txids" ? "txids" : "transactions";
  const itemsOf = (resp: AddressResponse) =>
    details === "txids" ? (resp.txids ?? []) : (resp.transactions ?? []);

  const fetchPage = (page: number) => ctx.client.getJson(
    "/api/v2/address/{address}",
    buildAddressDetailsPath(address, details, page, evmHistoryPageSize),
  );
  const assertPage = (resp: AddressResponse, label: string) => {
    assertAddressMatches(resp.address, address, `${label}.address`);
    assertPageMeta(resp.page, resp.itemsOnPage, resp.totalPages, resp.txs, label);
    assertPageSizeUpperBound(itemsOf(resp).length, resp.itemsOnPage ?? 0, evmHistoryPageSize, `${label}.${itemsField}`);
    if (itemsOf(resp).length === 0) {
      throw new Error(`${label} returned no ${itemsField}`);
    }
    if (details === "txs") {
      txIDsFromTransactions(resp.transactions ?? [], label);
    }
  };

  const page1 = await fetchPage(evmHistoryPage);
  assertPage(page1, `${testName}.page1`);

  if ((page1.totalPages ?? 0) <= 1 || (page1.txs ?? 0) <= evmHistoryPageSize) {
    throw new SkipTest(`pagination check: address ${address} has ${page1.txs ?? 0} txs and ${page1.totalPages ?? 0} page(s)`);
  }

  const page2 = await fetchPage(evmHistoryPage + 1);
  assertPage(page2, `${testName}.page2`);
  if (page2.page !== evmHistoryPage + 1) {
    throw new Error(`${testName} page mismatch: got ${page2.page ?? 0}, want ${evmHistoryPage + 1}`);
  }
}

const testGetAddressTxidsPaginationEVM = (ctx: TestContext) =>
  addressPaginationEVM(ctx, "txids", "GetAddressTxidsPaginationEVM");

const testGetAddressTxsPaginationEVM = (ctx: TestContext) =>
  addressPaginationEVM(ctx, "txs", "GetAddressTxsPaginationEVM");

async function testGetAddressTokensEVM(ctx: TestContext) {
  const address = await ctx.sampleEVMAddressOrSkip();
  const resp = await ctx.client.getJson(
    "/api/v2/address/{address}",
    buildAddressDetailsPath(address, "tokens", addressPage, addressPageSize),
  );
  assertEVMBasicAddressPayload(resp, address, "GetAddressTokensEVM");
  resp.tokens?.forEach((token, index) => {
    assertNonEmptyString(token.type, `GetAddressTokensEVM.tokens[${index}].type`);
    assertNonEmptyString(token.contract, `GetAddressTokensEVM.tokens[${index}].contract`);
    assertTokenDecimals(token.decimals, `GetAddressTokensEVM.tokens[${index}]`);
  });
}

async function testGetAddressTokenBalances(ctx: TestContext) {
  const address = await ctx.sampleEVMAddressOrSkip();
  const resp = await ctx.client.getJson(
    "/api/v2/address/{address}",
    buildAddressDetailsPath(address, "tokenBalances", addressPage, addressPageSize),
  );
  assertEVMTokenBalancesPayload(resp, address, "GetAddressTokenBalances");
  assertEVMTokenBalancesHaveHoldingsFields(resp, address, "GetAddressTokenBalances");
}

async function testGetAddressProtocolsEVM(ctx: TestContext) {
  await assertErc4626FixturesInAccountInfo(ctx, "GetAddressProtocolsEVM", async (fixture) => {
    const path = `${buildAddressDetailsPath(fixture.holder, "tokenBalances", addressPage, addressPageSize)}&contract=${encodeURIComponent(fixture.contract)}&protocols=erc4626`;
    return ctx.client.getJson("/api/v2/address/{address}", path);
  });
}

async function testGetAddressProtocolsOptInEVM(ctx: TestContext) {
  const testData = loadAPITestData(ctx.coin);
  const fixtures = testData.erc4626Fixtures ?? [];
  if (fixtures.length === 0) {
    throw new SkipTest(`openapi/fixtures/${ctx.coin}.json has no erc4626Fixtures entries`);
  }

  let validatedFixtures = 0;
  for (const fixture of fixtures) {
    const path = `${buildAddressDetailsPath(fixture.holder, "tokenBalances", addressPage, addressPageSize)}&contract=${encodeURIComponent(fixture.contract)}`;
    const resp = await ctx.client.getJson("/api/v2/address/{address}", path);
    assertAddressMatches(resp.address, fixture.holder, "GetAddressProtocolsOptInEVM.address");
    if (!resp.tokens || resp.tokens.length === 0) {
      continue;
    }
    resp.tokens.forEach((token, index) => {
      if (token.protocols && token.protocols.length > 0) {
        throw new Error(`opt-in gate broken: tokens[${index}].protocols=${JSON.stringify(token.protocols)} without ?protocols= request`);
      }
    });
    validatedFixtures++;
  }
  if (validatedFixtures === 0) {
    throw new Error("GetAddressProtocolsOptInEVM did not validate any ERC4626 fixture");
  }
}

async function testGetContractInfoEVM(ctx: TestContext) {
  await assertContractInfoFixturesFetched(ctx, "GetContractInfoEVM", async (fixture) => {
    return ctx.client.getJson(
      "/api/v2/contract/{contract}",
      `/api/v2/contract/${encodePathSegment(fixture.contract)}?protocols=erc4626`,
    );
  });
}

async function testGetContractInfoOptInEVM(ctx: TestContext) {
  const testData = loadAPITestData(ctx.coin);
  const fixtures = testData.erc4626Fixtures ?? [];
  if (fixtures.length === 0) {
    throw new SkipTest(`openapi/fixtures/${ctx.coin}.json has no erc4626Fixtures entries`);
  }

  for (const fixture of fixtures) {
    const resp = await ctx.client.getJson(
      "/api/v2/contract/{contract}",
      `/api/v2/contract/${encodePathSegment(fixture.contract)}`,
    );
    if (!equalFold(resp.contract, fixture.contract)) {
      throw new Error(`contract mismatch: got ${resp.contract ?? ""} want ${fixture.contract}`);
    }
    if (resp.protocols?.erc4626) {
      throw new Error(`opt-in gate broken: vault ${fixture.contract} leaked protocols.erc4626 without ?protocols= request`);
    }
  }
}

async function testGetContractInfoNonVaultEVM(ctx: TestContext) {
  const testData = loadAPITestData(ctx.coin);
  const contracts = testData.nonVaultContracts ?? [];
  if (contracts.length === 0) {
    throw new SkipTest(`openapi/fixtures/${ctx.coin}.json has no nonVaultContracts entries`);
  }

  for (const contract of contracts) {
    const resp = await ctx.client.getJson(
      "/api/v2/contract/{contract}",
      `/api/v2/contract/${encodePathSegment(contract)}?protocols=erc4626`,
    );
    if (!equalFold(resp.contract, contract)) {
      throw new Error(`contract mismatch: got ${resp.contract ?? ""} want ${contract}`);
    }
    if (resp.protocols?.erc4626) {
      throw new Error(`strict-gate regression: non-vault ${contract} returned protocols.erc4626`);
    }
  }
}

async function testErc4626FeeInvariantEVM(ctx: TestContext) {
  const testData = loadAPITestData(ctx.coin);
  const fixtures = testData.erc4626Fixtures ?? [];
  if (fixtures.length === 0) {
    throw new SkipTest(`openapi/fixtures/${ctx.coin}.json has no erc4626Fixtures entries`);
  }

  for (const fixture of fixtures) {
    const resp = await ctx.client.getJson(
      "/api/v2/contract/{contract}",
      `/api/v2/contract/${encodePathSegment(fixture.contract)}?protocols=erc4626`,
    );
    const erc4626 = resp.protocols?.erc4626;
    if (!erc4626) {
      throw new Error(`missing erc4626 payload for ${fixture.contract}`);
    }
    assertFeeInvariantGE(erc4626.convertToAssets1Share, erc4626.previewRedeem1Share, `${fixture.contract}: convertToAssets1Share >= previewRedeem1Share`);
    assertFeeInvariantGE(erc4626.convertToShares1Asset, erc4626.previewDeposit1Asset, `${fixture.contract}: convertToShares1Asset >= previewDeposit1Asset`);
  }
}

async function testGetAddressContractFilterEVM(ctx: TestContext) {
  const address = await ctx.sampleEVMAddressOrSkip();
  const contract = await ctx.sampleEVMContractOrSkip();
  const path = `${buildAddressDetailsPath(address, "tokenBalances", addressPage, addressPageSize)}&contract=${encodeURIComponent(contract)}`;
  const resp = await ctx.client.getJson("/api/v2/address/{address}", path);
  assertEVMTokenBalancesPayload(resp, address, "GetAddressContractFilterEVM");
  assertEVMTokenBalancesHaveHoldingsFields(resp, address, "GetAddressContractFilterEVM");
  assertEVMTokenListContractsMatch(resp.tokens ?? [], contract, "GetAddressContractFilterEVM");
}

async function testGetTransactionEVMShape(ctx: TestContext) {
  const txid = await ctx.sampleEVMTxIDOrSkip();
  const tx = await ctx.client.getJson(
    "/api/v2/tx/{txid}",
    `/api/v2/tx/${encodePathSegment(txid)}`,
  );
  assertEqualString(tx.txid, txid, "GetTransactionEVMShape.txid");
  if (!ctx.isEVMTxID(tx.txid)) {
    throw new Error(`GetTransactionEVMShape txid is not EVM-like: ${tx.txid}`);
  }
  if (tx.vin.length !== 1) {
    throw new Error(`GetTransactionEVMShape expected exactly 1 vin entry, got ${tx.vin.length}`);
  }
  if (tx.vout.length !== 1) {
    throw new Error(`GetTransactionEVMShape expected exactly 1 vout entry, got ${tx.vout.length}`);
  }
  if (!isObject(tx.ethereumSpecific) || Object.keys(tx.ethereumSpecific).length === 0) {
    throw new Error(`GetTransactionEVMShape missing ethereumSpecific object for ${txid}`);
  }
  // when the sampled tx carries token transfers, each must report decimals (trezor/blockbook#1577)
  tx.tokenTransfers?.forEach((transfer, index) =>
    assertTokenDecimals(transfer.decimals, `GetTransactionEVMShape.tokenTransfers[${index}]`),
  );
}

// testGetTransactionFeeConsistencyEVM samples a mined transaction and checks that the
// API's reported fee is derived from the receipt's effectiveGasPrice (the price actually
// paid on L2 rollups) rather than the transaction's bid gasPrice — the fix for #1227.
// It re-derives the fee from the exposed components and compares it to `fees`, mirroring
// api/worker.go getEthereumFeesSat exactly, so a regression back to the bid gasPrice (which
// differs from effectiveGasPrice on L2s) or a dropped l1Fee is caught end-to-end.
async function testGetTransactionFeeConsistencyEVM(ctx: TestContext) {
  const txid = await ctx.sampleEVMTxIDOrSkip();
  const tx = await ctx.client.getJson(
    "/api/v2/tx/{txid}",
    `/api/v2/tx/${encodePathSegment(txid)}`,
  );
  const eth = tx.ethereumSpecific;
  if (!isObject(eth)) {
    throw new SkipTest(`GetTransactionFeeConsistencyEVM: ${txid} has no ethereumSpecific`);
  }
  // gasUsed and l1Fee are exposed as JSON numbers. gasUsed is always well within range,
  // but l1Fee (wei) can exceed JS safe-integer precision during L1 fee spikes; re-deriving
  // the fee from a rounded number would diverge from the server's exact big-int result and
  // flake. Skip such a tx (deterministic: correct code passes every tx we can represent
  // exactly, so this never turns a good server red — it only declines to check).
  if (typeof eth.gasUsed === "number" && !Number.isSafeInteger(eth.gasUsed)) {
    throw new SkipTest(`GetTransactionFeeConsistencyEVM: ${txid} gasUsed=${eth.gasUsed} exceeds safe-integer precision`);
  }
  if (typeof eth.l1Fee === "number" && !Number.isSafeInteger(eth.l1Fee)) {
    throw new SkipTest(`GetTransactionFeeConsistencyEVM: ${txid} l1Fee=${eth.l1Fee} exceeds safe-integer precision`);
  }
  const gasUsed = optionalBigInt(eth.gasUsed, "GetTransactionFeeConsistencyEVM.gasUsed");
  const fees = optionalBigInt(tx.fees, "GetTransactionFeeConsistencyEVM.fees");
  // The fee is only defined for mined transactions (gasUsed known); skip pending or
  // zero-gas system transactions (e.g. the OP-stack L1-attributes deposit tx).
  if (gasUsed === undefined || gasUsed === 0n || fees === undefined) {
    throw new SkipTest(`GetTransactionFeeConsistencyEVM: ${txid} has no mined fee to verify`);
  }
  // Prefer the receipt effectiveGasPrice when present and positive, else the bid gasPrice,
  // then add any separately reported L1 data fee (OP stack). Mirrors getEthereumFeesSat.
  const effectiveGasPrice = optionalBigInt(eth.effectiveGasPrice, "GetTransactionFeeConsistencyEVM.effectiveGasPrice");
  const gasPrice = optionalBigInt(eth.gasPrice, "GetTransactionFeeConsistencyEVM.gasPrice");
  const perGas = effectiveGasPrice !== undefined && effectiveGasPrice > 0n ? effectiveGasPrice : (gasPrice ?? 0n);
  const l1Fee = optionalBigInt(eth.l1Fee, "GetTransactionFeeConsistencyEVM.l1Fee") ?? 0n;
  const expected = perGas * gasUsed + l1Fee;
  if (fees !== expected) {
    throw new Error(
      `GetTransactionFeeConsistencyEVM: fee mismatch for ${txid}: api fees=${fees}, expected ` +
        `(effectiveGasPrice||gasPrice)*gasUsed + l1Fee = ${expected} ` +
        `[effectiveGasPrice=${eth.effectiveGasPrice ?? "none"}, gasPrice=${eth.gasPrice ?? "none"}, ` +
        `gasUsed=${gasUsed}, l1Fee=${l1Fee}]`,
    );
  }
}

export async function assertErc4626FixturesInAccountInfo(
  ctx: TestContext,
  testName: string,
  fetchInfo: (fixture: Erc4626Fixture) => Promise<AddressResponse>,
) {
  const testData = loadAPITestData(ctx.coin);
  const fixtures = testData.erc4626Fixtures ?? [];
  if (fixtures.length === 0) {
    throw new Error(`openapi/fixtures/${ctx.coin}.json has no erc4626Fixtures entries`);
  }

  let validatedFixtures = 0;
  for (const fixture of fixtures) {
    const info = await fetchInfo(fixture);
    assertAddressMatches(info.address, fixture.holder, `${testName}.address`);
    if (!info.tokens || info.tokens.length === 0) {
      continue;
    }
    info.tokens.forEach((token, index) => {
      if (!equalFold(token.contract, fixture.contract)) {
        throw new Error(`${testName}.tokens[${index}] contract mismatch: got ${token.contract ?? ""} want ${fixture.contract}`);
      }
      if (!token.protocols?.includes("erc4626")) {
        throw new Error(`${testName}.tokens[${index}] missing erc4626 in protocols for ${fixture.contract}, got ${JSON.stringify(token.protocols ?? [])}`);
      }
    });
    validatedFixtures++;
  }

  if (validatedFixtures === 0) {
    throw new Error(`${testName} did not validate any ERC4626 fixture`);
  }
}

export async function assertContractInfoFixturesFetched(
  ctx: TestContext,
  testName: string,
  fetchInfo: (fixture: Erc4626Fixture) => Promise<ContractInfoResponse>,
) {
  const testData = loadAPITestData(ctx.coin);
  const fixtures = testData.erc4626Fixtures ?? [];
  if (fixtures.length === 0) {
    throw new Error(`openapi/fixtures/${ctx.coin}.json has no erc4626Fixtures entries`);
  }

  let validatedFixtures = 0;
  for (const fixture of fixtures) {
    const info = await fetchInfo(fixture);
    if (!equalFold(info.contract, fixture.contract)) {
      throw new Error(`${testName}.contract mismatch: got ${info.contract ?? ""} want ${fixture.contract}`);
    }
    assertNonEmptyString(info.standard, `${testName}.standard`);
    if (!positiveNumber(info.blockHeight)) {
      throw new Error(`${testName}.blockHeight is zero`);
    }
    if (!info.protocols?.erc4626) {
      throw new Error(`${testName} missing erc4626 payload for known ERC4626 contract ${fixture.contract}`);
    }
    assertErc4626Payload(`${testName}.protocols.erc4626`, fixture.contract, info.protocols.erc4626);
    validatedFixtures++;
  }

  if (validatedFixtures === 0) {
    throw new Error(`${testName} did not validate any ERC4626 fixture`);
  }
}

export const evmOnlyTests: Record<string, TestFunction> = {
  GetAddressBasicEVM: testGetAddressBasicEVM,
  GetAddressConfirmedNonceEVM: testGetAddressConfirmedNonceEVM,
  GetAddressTokensEVM: testGetAddressTokensEVM,
  GetAddressTokenBalances: testGetAddressTokenBalances,
  GetAddressProtocolsEVM: testGetAddressProtocolsEVM,
  GetAddressProtocolsOptInEVM: testGetAddressProtocolsOptInEVM,
  GetContractInfoEVM: testGetContractInfoEVM,
  GetContractInfoOptInEVM: testGetContractInfoOptInEVM,
  GetContractInfoNonVaultEVM: testGetContractInfoNonVaultEVM,
  Erc4626FeeInvariantEVM: testErc4626FeeInvariantEVM,
  GetAddressTxidsPaginationEVM: testGetAddressTxidsPaginationEVM,
  GetAddressTxsPaginationEVM: testGetAddressTxsPaginationEVM,
  GetAddressContractFilterEVM: testGetAddressContractFilterEVM,
  GetTransactionEVMShape: testGetTransactionEVMShape,
  GetTransactionFeeConsistencyEVM: testGetTransactionFeeConsistencyEVM,
};
