import { preview } from "./openapi.js";

import type { components } from "../.generated/blockbook.js";
import type { AddressResponse, BlockResponse, BlockSummary, ContractInfoResponse, FiatTickerResponse, TokenResponse, TxResponse, UtxoResponse, WsResponse } from "./types.js";

export function summarizeBlock(block: BlockResponse, pageSize: number): BlockSummary {
  return {
    hash: stringValue(block.hash).trim(),
    height: numberValue(block.height),
    hasTxField: Array.isArray(block.txs),
    txIDs: extractTxIDs(block),
    pageSize,
  };
}

export function extractTxIDs(block: BlockResponse) {
  const txs = block.txs;
  if (Array.isArray(txs)) {
    return txs.map((tx) => stringValue(tx.txid).trim()).filter(Boolean);
  }
  return block.tx?.map((txid) => txid.trim()).filter(Boolean) ?? [];
}

export function firstAddressFromTx(tx: TxResponse) {
  for (const output of tx.vout) {
    for (const address of output.addresses ?? []) {
      if (isAddressCandidate(address)) {
        return address;
      }
    }
  }
  for (const input of tx.vin) {
    for (const address of input.addresses ?? []) {
      if (isAddressCandidate(address)) {
        return address;
      }
    }
  }
  return "";
}

export function firstAddressFromTxPreferVin(tx: TxResponse) {
  for (const input of tx.vin) {
    for (const address of input.addresses ?? []) {
      if (isAddressCandidate(address)) {
        return address;
      }
    }
  }
  for (const output of tx.vout) {
    for (const address of output.addresses ?? []) {
      if (isAddressCandidate(address)) {
        return address;
      }
    }
  }
  return "";
}

export function isAddressCandidate(address: string) {
  const trimmed = address.trim();
  if (!trimmed) {
    return false;
  }
  if (trimmed.toUpperCase().startsWith("OP_RETURN")) {
    return false;
  }
  return !/[ \t\r\n]/.test(trimmed);
}

export function assertAddressTxidsPayload(payload: AddressResponse, address: string, txid: string, context: string, pageSize: number) {
  assertAddressMatches(payload.address, address, `${context}.address`);
  assertPageMeta(payload.page, payload.itemsOnPage, payload.totalPages, payload.txs, context);
  assertPageSizeUpperBound(payload.txids?.length ?? 0, payload.itemsOnPage ?? 0, pageSize, `${context}.txids`);
  assertTxIDListContains(payload.txids ?? [], txid, `${context}.txids`);
}

export function assertAddressTxsPayload(payload: AddressResponse, address: string, txid: string, context: string, pageSize: number) {
  assertAddressMatches(payload.address, address, `${context}.address`);
  assertPageMeta(payload.page, payload.itemsOnPage, payload.totalPages, payload.txs, context);
  assertPageSizeUpperBound(payload.transactions?.length ?? 0, payload.itemsOnPage ?? 0, pageSize, `${context}.transactions`);
  assertTransactionsContainTxID(payload.transactions ?? [], txid, `${context}.transactions`);
}

export function assertBasicAccountInfoPayload(payload: AddressResponse, address: string, context: string) {
  assertAddressMatches(payload.address, address, `${context}.address`);
  if (!Number.isInteger(payload.unconfirmedTxs) || payload.unconfirmedTxs < 0) {
    throw new Error(`${context} invalid unconfirmedTxs: ${String(payload.unconfirmedTxs)}`);
  }
  if ("unconfirmedBalance" in payload) {
    throw new Error(`${context} includes unconfirmedBalance for details=basic`);
  }
}

export function assertEVMBasicAddressPayload(payload: AddressResponse, address: string, context: string) {
  assertAddressMatches(payload.address, address, `${context}.address`);
  assertNonEmptyString(payload.balance, `${context}.balance`);
  assertNonEmptyString(payload.nonce, `${context}.nonce`);
  if ((payload.nonTokenTxs ?? 0) < 0) {
    throw new Error(`${context} has negative nonTokenTxs: ${payload.nonTokenTxs ?? 0}`);
  }
  if ((payload.txs ?? 0) < 0) {
    throw new Error(`${context} has negative txs: ${payload.txs ?? 0}`);
  }
  if ((payload.nonTokenTxs ?? 0) > (payload.txs ?? 0)) {
    throw new Error(`${context} has nonTokenTxs ${payload.nonTokenTxs ?? 0} greater than txs ${payload.txs ?? 0}`);
  }
}

export function assertEVMTokenBalancesPayload(payload: AddressResponse, address: string, context: string) {
  assertAddressMatches(payload.address, address, `${context}.address`);
  assertNonEmptyString(payload.balance, `${context}.balance`);
  let tokensWithHoldings = 0;
  for (const [index, token] of (payload.tokens ?? []).entries()) {
    if (assertEVMTokenHasHoldings(token, `${context}.tokens[${index}]`)) {
      tokensWithHoldings++;
    }
  }
  if ((payload.tokens?.length ?? 0) > 0 && tokensWithHoldings === 0) {
    throw new Error(`${context} has tokens array but no token includes holdings fields`);
  }
}

export function assertEVMTokenHasHoldings(token: TokenResponse, context: string) {
  assertNonEmptyString(token.type, `${context}.type`);
  const hasBalance = stringValue(token.balance).trim() !== "";
  const hasIDs = (token.ids?.length ?? 0) > 0;
  const hasMultiTokenValues = (token.multiTokenValues?.length ?? 0) > 0;
  token.ids?.forEach((id) => assertNonEmptyString(id, `${context}.ids`));
  token.multiTokenValues?.forEach((value) => {
    if (!stringValue(value.id).trim() && !stringValue(value.value).trim()) {
      throw new Error(`${context}.multiTokenValues entry has both empty id and value`);
    }
  });
  return hasBalance || hasIDs || hasMultiTokenValues;
}

export function assertEVMTokenBalancesHaveHoldingsFields(payload: AddressResponse, address: string, context: string) {
  assertAddressMatches(payload.address, address, `${context}.address`);
  assertNonEmptyString(payload.balance, `${context}.balance`);
  for (const [index, token] of (payload.tokens ?? []).entries()) {
    if (!assertEVMTokenHasHoldings(token, `${context}.tokens[${index}]`)) {
      throw new Error(`${context}.tokens[${index}] has no holdings fields (balance, ids, multiTokenValues)`);
    }
  }
}

export function assertEVMTokenListContractsMatch(tokens: TokenResponse[], contract: string, context: string) {
  if (tokens.length === 0) {
    throw new Error(`${context} returned no tokens`);
  }
  tokens.forEach((token, index) => {
    assertNonEmptyString(token.contract, `${context}.tokens[${index}].contract`);
    if (!equalFold(token.contract, contract)) {
      throw new Error(`${context}.tokens[${index}] contract mismatch: got ${token.contract ?? ""}, want ${contract}`);
    }
  });
}

export function assertErc4626Payload(context: string, shareContract: string, payload: NonNullable<ContractInfoResponse["protocols"]>["erc4626"]) {
  if (!payload) {
    throw new Error(`${context} missing payload`);
  }
  if (!payload.asset) {
    throw new Error(`${context} missing asset metadata`);
  }
  assertNonEmptyString(payload.asset.contract, `${context}.asset.contract`);
  if (!isEVMAddress(payload.asset.contract)) {
    throw new Error(`${context}.asset.contract is not EVM-like: ${payload.asset.contract}`);
  }
  if ((payload.asset.decimals ?? 0) < 0) {
    throw new Error(`${context}.asset.decimals is negative: ${payload.asset.decimals ?? 0}`);
  }

  if (!payload.share) {
    throw new Error(`${context} missing share metadata`);
  }
  assertNonEmptyString(payload.share.contract, `${context}.share.contract`);
  if (!equalFold(payload.share.contract, shareContract)) {
    throw new Error(`${context}.share.contract mismatch: got ${payload.share.contract}, want ${shareContract}`);
  }
  if ((payload.share.decimals ?? 0) < 0) {
    throw new Error(`${context}.share.decimals is negative: ${payload.share.decimals ?? 0}`);
  }

  assertBigIntString(payload.totalAssets, `${context}.totalAssets`);
  assertOptionalBigIntString(payload.convertToAssets1Share, `${context}.convertToAssets1Share`);
  assertOptionalBigIntString(payload.convertToShares1Asset, `${context}.convertToShares1Asset`);
  assertOptionalBigIntString(payload.previewDeposit1Asset, `${context}.previewDeposit1Asset`);
  assertOptionalBigIntString(payload.previewRedeem1Share, `${context}.previewRedeem1Share`);
}

export function assertFiatTickerPayload(payload: FiatTickerResponse, context: string) {
  if (!positiveNumber(payload.ts)) {
    throw new Error(`${context} invalid timestamp: ${String(payload.ts)}`);
  }
  if (!payload.rates || Object.keys(payload.rates).length === 0) {
    throw new Error(`${context} returned no rates`);
  }
  for (const [currency, rate] of Object.entries(payload.rates)) {
    assertNonEmptyString(currency, `${context}.rates.currency`);
    if (rate === 0) {
      throw new Error(`${context} returned zero rate for currency ${currency}`);
    }
  }
}

export function assertPageMeta(page: unknown, itemsOnPage: unknown, totalPages: unknown, totalItems: unknown, context: string) {
  const p = numberValue(page);
  const items = numberValue(itemsOnPage);
  const pages = numberValue(totalPages);
  const total = numberValue(totalItems);
  if (p <= 0) {
    throw new Error(`${context} invalid page: ${p}`);
  }
  if (items < 0) {
    throw new Error(`${context} invalid itemsOnPage: ${items}`);
  }
  if (pages < 0) {
    throw new Error(`${context} invalid totalPages: ${pages}`);
  }
  if (total < 0) {
    throw new Error(`${context} invalid txs count: ${total}`);
  }
  if (pages > 0 && p > pages) {
    throw new Error(`${context} invalid page ${p} > totalPages ${pages}`);
  }
}

export function assertPageMetaAllowUnknownTotal(page: unknown, itemsOnPage: unknown, totalPages: unknown, totalItems: unknown, context: string) {
  const p = numberValue(page);
  const items = numberValue(itemsOnPage);
  const pages = numberValue(totalPages);
  const total = numberValue(totalItems);
  if (p <= 0) {
    throw new Error(`${context} invalid page: ${p}`);
  }
  if (items < 0) {
    throw new Error(`${context} invalid itemsOnPage: ${items}`);
  }
  if (pages < -1) {
    throw new Error(`${context} invalid totalPages: ${pages}`);
  }
  if (total < 0) {
    throw new Error(`${context} invalid txs count: ${total}`);
  }
  if (pages > 0 && p > pages) {
    throw new Error(`${context} invalid page ${p} > totalPages ${pages}`);
  }
}

export function assertPageSizeUpperBound(payloadLen: number, itemsOnPage: number, requestedPageSize: number, context: string) {
  if (itemsOnPage > requestedPageSize) {
    throw new Error(`${context} invalid itemsOnPage ${itemsOnPage} > requested pageSize ${requestedPageSize}`);
  }
  if (payloadLen > requestedPageSize) {
    throw new Error(`${context} returned ${payloadLen} items, requested pageSize=${requestedPageSize}`);
  }
  if (itemsOnPage > 0 && payloadLen > itemsOnPage) {
    throw new Error(`${context} returned ${payloadLen} items, greater than itemsOnPage=${itemsOnPage}`);
  }
}

export function assertTxIDListContains(txids: string[], txid: string, context: string) {
  if (txids.length === 0) {
    throw new Error(`${context} returned no txids`);
  }
  txids.forEach((value) => assertNonEmptyString(value, context));
  if (!containsTxID(txids, txid)) {
    throw new Error(`${context} does not include sample transaction ${txid}`);
  }
}

export function assertTransactionsContainTxID(txs: TxResponse[], txid: string, context: string) {
  if (txs.length === 0) {
    throw new Error(`${context} returned no transactions`);
  }
  const txids = txIDsFromTransactions(txs, context);
  if (!containsTxID(txids, txid)) {
    throw new Error(`${context} does not include sample transaction ${txid}`);
  }
}

export function assertUTXOList(utxos: UtxoResponse[], context: string) {
  utxos.forEach((utxo) => {
    assertNonEmptyString(utxo.txid, `${context}.txid`);
    assertNonEmptyString(utxo.value, `${context}.value`);
  });
}

export function assertUTXOListNonNegativeConfirmations(utxos: UtxoResponse[], context: string) {
  assertUTXOList(utxos, context);
  utxos.forEach((utxo) => {
    if ((utxo.confirmations ?? 0) < 0) {
      throw new Error(`${context} has negative confirmations for ${utxo.txid}`);
    }
  });
}

export function txIDsFromTransactions(txs: TxResponse[], context: string) {
  return txs.map((tx, index) => {
    assertNonEmptyString(tx.txid, `${context}.transactions[${index}].txid`);
    return tx.txid;
  });
}

export function assertStringSlicesEqual(got: string[], want: string[], context: string) {
  if (got.length !== want.length) {
    throw new Error(`${context} length mismatch: got ${got.length}, want ${want.length}`);
  }
  got.forEach((value, index) => {
    if (value !== want[index]) {
      throw new Error(`${context}[${index}] mismatch: got ${value}, want ${want[index]}`);
    }
  });
}

export function assertComparableAccountPages(wsResp: AddressResponse, httpResp: AddressResponse, context: string) {
  if (wsResp.page !== httpResp.page || wsResp.itemsOnPage !== httpResp.itemsOnPage) {
    throw new Error(`${context} page meta mismatch: ws(page=${wsResp.page ?? 0} items=${wsResp.itemsOnPage ?? 0} totalPages=${wsResp.totalPages ?? 0} txs=${wsResp.txs ?? 0}) http(page=${httpResp.page ?? 0} items=${httpResp.itemsOnPage ?? 0} totalPages=${httpResp.totalPages ?? 0} txs=${httpResp.txs ?? 0})`);
  }
  if (wsResp.totalPages !== httpResp.totalPages) {
    throw new Error(`${context} totalPages mismatch: ws=${wsResp.totalPages ?? 0} http=${httpResp.totalPages ?? 0}`);
  }
  if ((wsResp.totalPages ?? 0) >= 0 && wsResp.txs !== httpResp.txs) {
    throw new Error(`${context} tx count mismatch: ws=${wsResp.txs ?? 0} http=${httpResp.txs ?? 0}`);
  }
}

export function assertFeeInvariantGE(lhs: unknown, rhs: unknown, context: string) {
  const a = optionalBigInt(lhs, `${context}.lhs`);
  const b = optionalBigInt(rhs, `${context}.rhs`);
  if (a === undefined || b === undefined) {
    return;
  }
  if (a < b) {
    throw new Error(`${context} violated: ${String(lhs).trim()} < ${String(rhs).trim()}`);
  }
}

export function assertBigIntString(value: unknown, context: string) {
  const parsed = optionalBigInt(value, context);
  if (parsed === undefined) {
    throw new Error(`${context} is empty`);
  }
}

export function assertOptionalBigIntString(value: unknown, context: string) {
  optionalBigInt(value, context);
}

export function optionalBigInt(value: unknown, context: string) {
  const text = stringValue(value).trim();
  if (!text) {
    return undefined;
  }
  if (!/^[0-9]+$/.test(text)) {
    throw new Error(`${context} is not a valid non-negative decimal integer: ${text}`);
  }
  return BigInt(text);
}

export function assertNonEmptyList<T>(items: T[] | undefined, message: string): asserts items is T[] {
  if (!items || items.length === 0) {
    throw new Error(message);
  }
}

export function assertNonEmptyString(value: unknown, field: string): asserts value is string {
  if (typeof value !== "string" || value.trim() === "") {
    throw new Error(`empty value for ${field}`);
  }
}

export function assertEqualString(got: unknown, want: string | undefined, field: string) {
  if (got !== want) {
    throw new Error(`${field} mismatch: got ${String(got)}, want ${String(want)}`);
  }
}

export function assertAddressMatches(got: unknown, want: string, field: string) {
  assertNonEmptyString(got, field);
  if (!equalFold(got, want)) {
    throw new Error(`${field} mismatch: got ${got}, want ${want}`);
  }
}

export function containsTxID(txids: string[], txid: string) {
  return txids.some((value) => equalFold(value.trim(), txid));
}

export function equalFold(a: unknown, b: unknown) {
  return typeof a === "string" && typeof b === "string" && a.toLowerCase() === b.toLowerCase();
}

export function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function isWsError(data: WsResponse["data"]): data is components["schemas"]["WsErrorData"] {
  return isObject(data) && "error" in data;
}

export function positiveNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value > 0;
}

export function numberValue(value: unknown) {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

export function stringValue(value: unknown) {
  return typeof value === "string" ? value : "";
}

export function isEVMAddress(address: string) {
  return address.trim().toLowerCase().startsWith("0x");
}

export function isFixedHex(value: string, length: number) {
  return value.length === length && /^[0-9a-fA-F]+$/.test(value);
}

export function isTronAddress(address: string) {
  return address.length === 34 && address[0] === "T" && /^[1-9A-HJ-NP-Za-km-z]+$/.test(address);
}

export function buildAddressDetailsPath(address: string, details: string, page: number, pageSize: number) {
  return `/api/v2/address/${encodePathSegment(address)}?details=${encodeURIComponent(details)}&page=${page}&pageSize=${pageSize}`;
}

export function buildAddressDetailsPathWithTo(address: string, details: string, page: number, pageSize: number, toHeight: number) {
  const base = buildAddressDetailsPath(address, details, page, pageSize);
  return toHeight > 0 ? `${base}&to=${toHeight}` : base;
}

export function buildAddressDetailsPathWithRange(address: string, details: string, page: number, pageSize: number, fromHeight: number, toHeight: number) {
  let base = buildAddressDetailsPath(address, details, page, pageSize);
  if (fromHeight > 0) {
    base += `&from=${fromHeight}`;
  }
  if (toHeight > 0) {
    base += `&to=${toHeight}`;
  }
  return base;
}

export function encodePathSegment(value: string | number) {
  return encodeURIComponent(String(value));
}

export function isFiatDataUnavailable(status: number, body: string) {
  if (status !== 400 && status !== 500) {
    return false;
  }
  const msg = preview(body).toLowerCase();
  return msg.includes("no tickers found") || msg.includes("error finding ticker");
}

export function upgradeWSBaseToWSS(raw: string) {
  const url = new URL(raw);
  if (url.protocol !== "ws:") {
    return "";
  }
  url.protocol = "wss:";
  return url.toString();
}

export class Lazy<T> {
  private promise: Promise<T> | undefined;
  constructor(private readonly compute: () => Promise<T>) {}
  get(): Promise<T> {
    this.promise ??= this.compute();
    return this.promise;
  }
}
