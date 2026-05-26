import WebSocket from "ws";

import { OpenApiFetchClient } from "./client.js";
import { OpenApiContract, preview } from "./openapi.js";
import { resolveHTTPBase, resolveWSURL } from "./config.js";
import { SkipTest } from "./errors.js";
import { addressPage, addressPageSize, blockPageSize, sampleBlockPageSize, sampleBlockProbeMax, sciNotationTxLimit, sciNotationWindow, scientificNotationPattern, txSearchWindow, wsDialTimeoutMs, wsMessageTimeoutMs } from "./constants.js";
import {
  assertAddressMatches,
  buildAddressDetailsPath,
  encodePathSegment,
  extractTxIDs,
  firstAddressFromTx,
  firstAddressFromTxPreferVin,
  isAddressCandidate,
  isEVMAddress as isEVMAddressValue,
  isFiatDataUnavailable,
  isFixedHex,
  isObject,
  isTronAddress,
  isWsError,
  Lazy,
  positiveNumber,
  stringValue,
  summarizeBlock,
  upgradeWSBaseToWSS,
} from "./support.js";

import type { Capability, AddressResponse, BlockHashResponse, BlockResponse, BlockSummary, FiatTickerResponse, StatusResponse, TxResponse, UtxoResponse, WsEnvelope, WsInfoResponse, WsMethod, WsResponse } from "./types.js";

export class TestContext {
  readonly client: OpenApiFetchClient;

  private status?: NonNullable<StatusResponse["blockbook"]>;
  private nextWSReq = 0;
  private blockHashByHeight = new Map<number, string>();
  private blockByHash = new Map<string, BlockSummary>();
  private txByID = new Map<string, TxResponse>();

  private sampleTxResolved = false;
  private sampleTxID = "";
  private sampleAddrResolved = false;
  private sampleAddress = "";
  private sampleIndexResolved = false;
  private sampleIndexHeight = 0;
  private sampleIndexHash = "";
  private sampleBlockResolved = false;
  private sampleBlockHeight = 0;
  private sampleBlockHash = "";
  private sampleContractResolved = false;
  private sampleContract = "";
  private sampleFiatResolved = false;
  private sampleFiatAvailable = false;
  private sampleFiatTicker?: FiatTickerResponse;

  private readonly capabilities = new Lazy(() => this.probeCapabilities());
  private readonly scientificNotationCase = new Lazy(() => this.findScientificNotationCase());

  private constructor(
    readonly coin: string,
    readonly contract: OpenApiContract,
    private wsURL: string,
    client: OpenApiFetchClient,
  ) {
    this.client = client;
  }

  static async create(coin: string, contract: OpenApiContract) {
    const httpBase = await resolveHTTPBase(coin);
    const wsURL = resolveWSURL(coin, httpBase);
    return new TestContext(coin, contract, wsURL, new OpenApiFetchClient(httpBase, contract));
  }

  async getStatus() {
    if (this.status) {
      return this.status;
    }

    const envelope = await this.client.getJson("/api/status", "/api/status");
    if (!isObject(envelope.blockbook) || Object.keys(envelope.blockbook).length === 0) {
      throw new Error("status response missing non-empty blockbook object");
    }
    if (!isObject(envelope.backend) || Object.keys(envelope.backend).length === 0) {
      throw new Error("status response missing non-empty backend object");
    }
    if (!positiveNumber(envelope.blockbook.bestHeight)) {
      throw new Error(`invalid status bestHeight: ${String(envelope.blockbook.bestHeight)}`);
    }

    this.status = envelope.blockbook;
    return this.status;
  }

  async requireCapability(required: Capability, group: string, testName: string) {
    const caps = await this.capabilities.get();
    const probe = required === "utxo" ? caps.utxo : caps.evm;
    if (!probe.supported) {
      throw new SkipTest(`Skipping ${testName} (${group}): ${required.toUpperCase()} capability required (${probe.message})`);
    }
  }

  async sampleScientificNotationCaseOrSkip() {
    const found = await this.scientificNotationCase.get();
    if (!found) {
      throw new SkipTest(`no tx-specific scientific-notation amounts found in last ${sciNotationWindow} blocks`);
    }
    return found;
  }

  async getSampleIndexedHeight() {
    if (this.sampleIndexResolved) {
      return this.sampleIndexHash ? { height: this.sampleIndexHeight, hash: this.sampleIndexHash } : undefined;
    }
    if (this.sampleBlockResolved && this.sampleBlockHash) {
      return { height: this.sampleBlockHeight, hash: this.sampleBlockHash };
    }

    const status = await this.getStatus();
    let start = status.bestHeight ?? 0;
    if (start > 2) {
      start -= 2;
    }
    const lower = Math.max(1, start - txSearchWindow);
    this.sampleIndexResolved = true;

    for (let height = start; height >= lower; height--) {
      const hash = await this.getBlockHashForHeight(height, false);
      if (hash) {
        this.sampleIndexHeight = height;
        this.sampleIndexHash = hash;
        return { height, hash };
      }
    }
    return undefined;
  }

  async getSampleIndexedBlock() {
    if (this.sampleBlockResolved) {
      return this.sampleBlockHash ? { height: this.sampleBlockHeight, hash: this.sampleBlockHash } : undefined;
    }

    this.sampleBlockResolved = true;
    const sample = await this.getSampleIndexedHeight();
    if (!sample) {
      return undefined;
    }

    const lower = Math.max(1, sample.height - sampleBlockProbeMax + 1);
    for (let height = sample.height; height >= lower; height--) {
      const hash = height === sample.height ? sample.hash : await this.getBlockHashForHeight(height, false);
      if (!hash) {
        continue;
      }
      const block = await this.getBlockByHashForSampling(hash, false);
      if (!block || !block.hasTxField) {
        continue;
      }
      this.sampleBlockHeight = height;
      this.sampleBlockHash = hash;
      return { height, hash };
    }
    return undefined;
  }

  async getSampleTxID() {
    if (this.sampleTxResolved) {
      return this.sampleTxID || undefined;
    }

    if (this.sampleBlockResolved && this.sampleBlockHash) {
      const block = await this.getBlockByHash(this.sampleBlockHash, false);
      const txid = block?.txIDs.find((value) => value.trim() !== "");
      if (txid) {
        this.sampleTxResolved = true;
        this.sampleTxID = txid;
        return txid;
      }
    }

    const status = await this.getStatus();
    const found = await this.findTransactionNearHeight(status.bestHeight ?? 0, txSearchWindow);
    this.sampleTxResolved = true;
    if (!found) {
      return undefined;
    }
    this.sampleTxID = found.txid;
    return found.txid;
  }

  async sampleTxIDOrSkip() {
    const txid = await this.getSampleTxID();
    if (!txid) {
      const status = await this.getStatus();
      throw new SkipTest(`no transaction found in last ${txSearchWindow} blocks from height ${status.bestHeight ?? 0}`);
    }
    return txid;
  }

  async getSampleAddress() {
    if (this.sampleAddrResolved) {
      return this.sampleAddress || undefined;
    }

    this.sampleAddrResolved = true;
    const txid = await this.getSampleTxID();
    if (!txid) {
      return undefined;
    }
    const tx = await this.getTransactionByID(txid, false);
    if (!tx) {
      return undefined;
    }

    this.sampleAddress = this.isEVMTxID(txid)
      ? firstAddressFromTxPreferVin(tx)
      : firstAddressFromTx(tx);
    return this.sampleAddress || undefined;
  }

  async sampleAddressOrSkip() {
    const address = await this.getSampleAddress();
    if (!address) {
      const status = await this.getStatus();
      throw new SkipTest(`no address found from recent transaction window at height ${status.bestHeight ?? 0}`);
    }
    return address;
  }

  async getBlockHashForHeight(height: number, strict: boolean) {
    const cached = this.blockHashByHeight.get(height);
    if (cached) {
      return cached;
    }

    const path = `/api/v2/block-index/${height}`;
    const result = await this.client.getMaybe("/api/v2/block-index/{height}", path);
    if (result.status !== 200 || result.data === undefined) {
      if (strict) {
        throw new Error(`GET ${path} returned HTTP ${result.status}: ${preview(result.body)}`);
      }
      return undefined;
    }

    const hash = stringValue(result.data.blockHash).trim();
    if (!hash) {
      if (strict) {
        throw new Error(`empty blockHash for height ${height}`);
      }
      return undefined;
    }

    this.blockHashByHeight.set(height, hash);
    return hash;
  }

  async getBlockByHash(hash: string, strict: boolean) {
    return this.getBlockSummary(hash, strict, blockPageSize);
  }

  async getBlockByHashForSampling(hash: string, strict: boolean) {
    const cached = this.blockByHash.get(hash);
    if (cached && cached.pageSize >= sampleBlockPageSize) {
      return cached;
    }
    return this.getBlockSummary(hash, strict, sampleBlockPageSize);
  }

  async getTransactionByID(txid: string, strict: boolean) {
    const cached = this.txByID.get(txid);
    if (cached) {
      return cached;
    }

    const path = `/api/v2/tx/${encodePathSegment(txid)}`;
    const result = await this.client.getMaybe("/api/v2/tx/{txid}", path);
    if (result.status !== 200 || result.data === undefined) {
      if (strict) {
        throw new Error(`GET ${path} returned HTTP ${result.status}: ${preview(result.body)}`);
      }
      return undefined;
    }
    if (!result.data.txid) {
      if (strict) {
        throw new Error(`empty txid in transaction response for ${txid}`);
      }
      return undefined;
    }
    if (result.data.txid !== txid) {
      if (strict) {
        throw new Error(`transaction mismatch: got ${result.data.txid}, want ${txid}`);
      }
      return undefined;
    }

    this.txByID.set(txid, result.data);
    return result.data;
  }

  async sampleFiatTickerOrSkip() {
    if (this.sampleFiatResolved) {
      if (this.sampleFiatAvailable && this.sampleFiatTicker) {
        return this.sampleFiatTicker;
      }
      throw new SkipTest("fiat ticker data currently unavailable");
    }

    this.sampleFiatResolved = true;
    const path = "/api/v2/tickers/?currency=usd";
    const result = await this.client.getMaybe("/api/v2/tickers/", path);
    if (isFiatDataUnavailable(result.status, result.body)) {
      throw new SkipTest("fiat ticker data currently unavailable");
    }
    if (result.status !== 200 || result.data === undefined) {
      throw new Error(`GET ${path} returned HTTP ${result.status}: ${preview(result.body)}`);
    }
    if (!positiveNumber(result.data.ts) || !result.data.rates || Object.keys(result.data.rates).length === 0) {
      throw new SkipTest("fiat ticker data currently unavailable");
    }

    this.sampleFiatAvailable = true;
    this.sampleFiatTicker = result.data;
    return result.data;
  }

  async sampleEVMTxIDOrSkip() {
    const txid = await this.sampleTxIDOrSkip();
    if (!this.isEVMTxID(txid)) {
      throw new SkipTest(`sample txid ${txid} does not look EVM-like`);
    }
    return txid;
  }

  async sampleEVMAddressOrSkip() {
    const address = await this.sampleAddressOrSkip();
    if (!this.isEVMAddress(address)) {
      throw new SkipTest(`sample address ${address} does not look EVM-like`);
    }
    return address;
  }

  async sampleEVMContractOrSkip() {
    if (this.sampleContractResolved) {
      if (this.sampleContract) {
        return this.sampleContract;
      }
      throw new SkipTest(`no contract found for sampled EVM address ${this.sampleAddress}`);
    }

    this.sampleContractResolved = true;
    const address = await this.getSampleAddress();
    if (!address || !this.isEVMAddress(address)) {
      throw new SkipTest("no EVM sample address available for contract probe");
    }

    const resp = await this.client.getJson(
      "/api/v2/address/{address}",
      buildAddressDetailsPath(address, "tokenBalances", addressPage, addressPageSize),
    );
    assertAddressMatches(resp.address, address, "sample EVM contract probe address");

    this.sampleContract = resp.tokens?.map((token) => stringValue(token.contract).trim()).find(Boolean) ?? "";
    if (!this.sampleContract) {
      throw new SkipTest(`no contract found for sampled EVM address ${address}`);
    }
    return this.sampleContract;
  }

  async wsGetInfo() {
    return this.wsCall<WsInfoResponse>("getInfo", {}, "#/components/schemas/WsInfoRes");
  }

  async wsCall<T>(method: WsMethod, params: unknown, dataSchemaRef?: string) {
    this.nextWSReq++;
    return this.wsCallWithID<T>(`openapi-${this.coin}-${method}-${this.nextWSReq}`, method, params, dataSchemaRef);
  }

  async wsCallWithID<T>(id: string, method: WsMethod, params: unknown, dataSchemaRef?: string) {
    const request: WsEnvelope = { id, method, params };
    try {
      return await this.wsCallOnce<T>(this.wsURL, request, dataSchemaRef);
    } catch (error) {
      const upgraded = upgradeWSBaseToWSS(this.wsURL);
      if (!upgraded) {
        throw error;
      }
      const result = await this.wsCallOnce<T>(upgraded, request, dataSchemaRef);
      this.wsURL = upgraded;
      return result;
    }
  }

  isEVMTxID(txid: string) {
    const trimmed = txid.trim();
    return trimmed.toLowerCase().startsWith("0x") || (this.coin === "tron" && isFixedHex(trimmed, 64));
  }

  isEVMAddress(address: string) {
    return isEVMAddressValue(address) || (this.coin === "tron" && isTronAddress(address));
  }

  private async probeCapabilities() {
    return {
      utxo: await this.probeUTXOSupport(),
      evm: await this.probeEVMSupport(),
    };
  }

  private async probeUTXOSupport(): Promise<{ supported: boolean; message: string }> {
    const txid = await this.getSampleTxID();
    if (!txid) {
      return { supported: false, message: `no sample transaction in last ${txSearchWindow} blocks` };
    }
    if (this.isEVMTxID(txid)) {
      return { supported: false, message: "detected EVM-style transaction ids" };
    }

    const address = await this.getSampleAddress();
    if (!address) {
      return { supported: false, message: "no sample address available for probe" };
    }

    const path = `/api/v2/utxo/${encodePathSegment(address)}?confirmed=true`;
    const result = await this.client.getMaybe("/api/v2/utxo/{descriptor}", path);
    if (result.status !== 200) {
      throw new Error(`UTXO capability probe ${path} returned HTTP ${result.status}: ${preview(result.body)}`);
    }
    return { supported: true, message: "UTXO endpoint probe succeeded" };
  }

  private async probeEVMSupport(): Promise<{ supported: boolean; message: string }> {
    const txid = await this.getSampleTxID();
    if (!txid) {
      return { supported: false, message: `no sample transaction in last ${txSearchWindow} blocks` };
    }
    if (!this.isEVMTxID(txid)) {
      return { supported: false, message: "detected non-EVM transaction ids" };
    }

    const address = await this.getSampleAddress();
    if (!address) {
      return { supported: false, message: "no sample address available for probe" };
    }
    const path = buildAddressDetailsPath(address, "tokenBalances", addressPage, addressPageSize);
    const result = await this.client.getMaybe("/api/v2/address/{address}", path);
    if (result.status !== 200 || result.data === undefined) {
      throw new Error(`EVM capability probe ${path} returned HTTP ${result.status}: ${preview(result.body)}`);
    }
    assertAddressMatches(result.data.address, address, "EVM capability probe address");
    return { supported: true, message: "EVM tokenBalances endpoint probe succeeded" };
  }

  private async findScientificNotationCase() {
    const status = await this.getStatus();
    const lower = Math.max(1, (status.bestHeight ?? 0) - sciNotationWindow + 1);
    for (let height = status.bestHeight ?? 0; height >= lower; height--) {
      const hash = await this.getBlockHashForHeight(height, false);
      if (!hash) {
        continue;
      }
      const txids = await this.blockTxIDsForProbe(hash, sciNotationTxLimit);
      for (const txid of txids) {
        if (!txid || !(await this.txSpecificHasScientificNotation(txid))) {
          continue;
        }
        const tx = await this.getTransactionByID(txid, false);
        if (!tx) {
          continue;
        }
        const address = this.isEVMTxID(txid) ? firstAddressFromTxPreferVin(tx) : firstAddressFromTx(tx);
        if (!isAddressCandidate(address)) {
          continue;
        }
        return { address, txid, height };
      }
    }
    return undefined;
  }

  private async blockTxIDsForProbe(hash: string, pageSize: number) {
    const result = await this.client.getMaybe(
      "/api/v2/block/{blockId}",
      `/api/v2/block/${encodePathSegment(hash)}?page=1&pageSize=${pageSize}`,
    );
    if (result.status !== 200 || result.data === undefined) {
      return [];
    }
    return extractTxIDs(result.data);
  }

  private async txSpecificHasScientificNotation(txid: string) {
    const result = await this.client.getMaybe(
      "/api/v2/tx-specific/{txid}",
      `/api/v2/tx-specific/${encodePathSegment(txid)}`,
    );
    return result.status === 200 && scientificNotationPattern.test(result.body);
  }

  private async findTransactionNearHeight(fromHeight: number, window: number) {
    const lower = Math.max(0, fromHeight - window);
    for (let height = fromHeight; height >= lower; height--) {
      const hash = await this.getBlockHashForHeight(height, false);
      if (!hash) {
        continue;
      }
      const block = await this.getBlockByHashForSampling(hash, false);
      const txid = block?.txIDs.find((value) => value.trim() !== "");
      if (txid) {
        return { txid, height, hash };
      }
    }
    return undefined;
  }

  private async getBlockSummary(hash: string, strict: boolean, pageSize: number) {
    const cached = this.blockByHash.get(hash);
    if (cached && cached.pageSize >= pageSize) {
      return cached;
    }

    const path = `/api/v2/block/${encodePathSegment(hash)}?page=1&pageSize=${pageSize}`;
    const result = await this.client.getMaybe("/api/v2/block/{blockId}", path);
    if (result.status !== 200 || result.data === undefined) {
      if (strict) {
        throw new Error(`GET ${path} returned HTTP ${result.status}: ${preview(result.body)}`);
      }
      return undefined;
    }

    const summary = summarizeBlock(result.data, pageSize);
    if (!summary.hash) {
      if (strict) {
        throw new Error(`empty hash in block response for ${hash}`);
      }
      return undefined;
    }
    this.blockByHash.set(hash, summary);
    return summary;
  }

  private wsCallOnce<T>(wsURL: string, request: WsEnvelope, dataSchemaRef?: string) {
    return new Promise<T>((resolve, reject) => {
      const ws = new WebSocket(wsURL, {
        handshakeTimeout: wsDialTimeoutMs,
        rejectUnauthorized: process.env.OPENAPI_INSECURE_TLS === "0",
      });
      const timeout = setTimeout(() => {
        ws.terminate();
        reject(new Error(`websocket ${request.method} timed out for ${wsURL}`));
      }, wsMessageTimeoutMs);

      ws.on("open", () => {
        ws.send(JSON.stringify(request));
      });
      ws.on("message", (data) => {
        const response = JSON.parse(data.toString()) as WsResponse;
        if (response.id !== request.id) {
          return;
        }
        clearTimeout(timeout);
        ws.close();
        if (isWsError(response.data)) {
          reject(new Error(`websocket ${request.method} returned error: ${response.data.error.message}`));
          return;
        }
        if (dataSchemaRef) {
          this.contract.validateSchemaRef(dataSchemaRef, `WS ${request.method} response data`, response.data);
        }
        resolve(response.data as T);
      });
      ws.on("error", (error) => {
        clearTimeout(timeout);
        reject(error);
      });
    });
  }
}
