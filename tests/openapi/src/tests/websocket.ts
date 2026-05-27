import { addressPage, addressPageSize, evmHistoryPage, evmHistoryPageSize } from "../constants.js";
import {
  assertAddressTxidsPayload,
  assertBasicAccountInfoPayload,
  assertComparableAccountPages,
  assertEqualString,
  assertEVMTokenBalancesHaveHoldingsFields,
  assertEVMBasicAddressPayload,
  assertEVMTokenBalancesPayload,
  assertEVMTokenListContractsMatch,
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

import type { TestContext } from "../context.js";
import type { AddressResponse, ContractInfoResponse, TxResponse, UtxoResponse, WsBlockHashResponse } from "../types.js";

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

export const wsOnlyTests: Record<string, TestFunction> = {
  WsGetInfo: testWsGetInfo,
  WsGetBlockHash: testWsGetBlockHash,
  WsGetTransaction: testWsGetTransaction,
  WsGetAccountInfo: testWsGetAccountInfo,
  WsGetAccountInfoBasic: testWsGetAccountInfoBasic,
  WsPing: testWsPing,
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
