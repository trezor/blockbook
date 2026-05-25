import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import createClient from "openapi-fetch";
import { Agent, setGlobalDispatcher } from "undici";
import WebSocket from "ws";

import type { components, paths } from "../.generated/blockbook.js";

type Coin = "ethereum" | "bitcoin";

type StatusResponse = NonNullable<
  paths["/api/status"]["get"]["responses"]["200"]["content"]["application/json"]
>;
type TxResponse = NonNullable<
  paths["/api/v2/tx/{txid}"]["get"]["responses"]["200"]["content"]["application/json"]
>;
type WsRequest = components["schemas"]["WsRequest"];
type WsResponse = components["schemas"]["WsResponse"];
type WsInfoResponse = components["schemas"]["WsInfoRes"];

const supportedCoins = new Set<Coin>(["ethereum", "bitcoin"]);
const defaultCoins: Coin[] = ["ethereum", "bitcoin"];
const searchWindow = 12;

if (process.env.OPENAPI_INSECURE_TLS !== "0") {
  setGlobalDispatcher(new Agent({ connect: { rejectUnauthorized: false } }));
}

const repoRoot =
  process.env.REPO_ROOT ??
  path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../..");

const selectedCoins = resolveSelectedCoins();
if (selectedCoins.length === 0) {
  console.log("OpenAPI smoke: no selected ethereum/bitcoin target, skipping.");
  process.exit(0);
}

for (const coin of selectedCoins) {
  await smokeCoin(coin);
}

function resolveSelectedCoins(): Coin[] {
  const raw = process.env.OPENAPI_COINS?.trim();
  if (!raw) {
    return defaultCoins;
  }

  const seen = new Set<string>();
  const selected: Coin[] = [];
  for (const value of raw.split(",")) {
    const coin = value.trim();
    if (!coin || seen.has(coin) || !supportedCoins.has(coin as Coin)) {
      continue;
    }
    seen.add(coin);
    selected.push(coin as Coin);
  }
  return selected;
}

async function smokeCoin(coin: Coin) {
  const baseUrl = await resolveHTTPBase(coin);
  const wsUrl = resolveWSURL(coin, baseUrl);
  const client = createClient<paths>({ baseUrl });

  const status = await expectData(
    "GET /api/status",
    client.GET("/api/status"),
  );
  assertStatus(status, coin);

  const wsInfo = await wsGetInfo(coin, wsUrl);
  assertWsInfo(wsInfo, coin);

  const { height, block, txid } = await findSampleBlockAndTx(client, status, coin);

  const tx = await expectData(
    "GET /api/v2/tx/{txid}",
    client.GET("/api/v2/tx/{txid}", {
      params: { path: { txid } },
    }),
  );
  assertTx(tx, txid);

  await expectAnyAddressLookup(client, tx, coin, txid);

  await expectData(
    "GET /api/v2/estimatefee/{blocks}",
    client.GET("/api/v2/estimatefee/{blocks}", {
      params: { path: { blocks: 2 } },
    }),
  );

  const ticker = await expectData(
    "GET /api/v2/tickers/",
    client.GET("/api/v2/tickers/", {
      params: { query: { currency: "usd" } },
    }),
  );
  if (!ticker.rates || Object.keys(ticker.rates).length === 0) {
    throw new Error(`${coin}: fiat ticker returned no rates`);
  }

  console.log(
    `OpenAPI smoke ${coin}: ${status.blockbook?.network ?? coin} wsHeight=${wsInfo.bestHeight} height=${height} tx=${txid.slice(
      0,
      18,
    )}... blockTxs=${block.txCount}`,
  );
}

async function wsGetInfo(coin: Coin, wsUrl: string): Promise<WsInfoResponse> {
  const request = {
    id: `openapi-${coin}-getInfo`,
    method: "getInfo",
    params: {},
  } satisfies WsRequest;

  try {
    return await wsCallGetInfo(wsUrl, request);
  } catch (error) {
    if (!wsUrl.startsWith("ws:")) {
      throw error;
    }
    return wsCallGetInfo(`wss:${wsUrl.slice("ws:".length)}`, request);
  }
}

function wsCallGetInfo(wsUrl: string, request: WsRequest): Promise<WsInfoResponse> {
  return new Promise((resolve, reject) => {
    const ws = new WebSocket(wsUrl, {
      handshakeTimeout: 5000,
      rejectUnauthorized: process.env.OPENAPI_INSECURE_TLS === "0",
    });
    const timeout = setTimeout(() => {
      ws.terminate();
      reject(new Error(`websocket getInfo timed out for ${wsUrl}`));
    }, 10000);

    ws.on("open", () => {
      ws.send(JSON.stringify(request));
    });
    ws.on("message", (data) => {
      clearTimeout(timeout);
      ws.close();
      const response = JSON.parse(data.toString()) as WsResponse;
      if (response.id !== request.id) {
        reject(new Error(`websocket response id mismatch: got ${response.id}, want ${request.id}`));
        return;
      }
      if (isWsError(response.data)) {
        reject(new Error(`websocket getInfo returned error: ${response.data.error.message}`));
        return;
      }
      resolve(response.data as WsInfoResponse);
    });
    ws.on("error", (error) => {
      clearTimeout(timeout);
      reject(error);
    });
  });
}

async function findSampleBlockAndTx(
  client: ReturnType<typeof createClient<paths>>,
  status: StatusResponse,
  coin: Coin,
) {
  const bestHeight = status.blockbook?.bestHeight;
  if (!bestHeight || bestHeight < 1) {
    throw new Error(`${coin}: invalid bestHeight in status response`);
  }

  const startHeight = Math.max(1, bestHeight - 2);
  const minHeight = Math.max(1, startHeight - searchWindow);
  for (let height = startHeight; height >= minHeight; height--) {
    const hashResponse = await expectData(
      "GET /api/v2/block-index/{height}",
      client.GET("/api/v2/block-index/{height}", {
        params: { path: { height } },
      }),
    );
    if (!hashResponse.blockHash) {
      continue;
    }

    const block = await expectData(
      "GET /api/v2/block/{blockId}",
      client.GET("/api/v2/block/{blockId}", {
        params: { path: { blockId: String(height) }, query: { page: 1 } },
      }),
    );
    const txid = firstTxidFromBlock(block);
    if (block.hash && block.height === height && txid) {
      return { height, block, txid };
    }
  }

  throw new Error(`${coin}: no sample block with transactions found near ${bestHeight}`);
}

async function expectData<T>(
  label: string,
  request: Promise<{ data?: T; error?: unknown; response: Response }>,
): Promise<T> {
  const result = await request;
  if (result.error) {
    throw new Error(`${label} failed with HTTP ${result.response.status}: ${JSON.stringify(result.error)}`);
  }
  if (result.data === undefined || result.data === null) {
    throw new Error(`${label} returned no data`);
  }
  return result.data;
}

function assertStatus(status: StatusResponse, coin: Coin) {
  if (!status.blockbook?.bestHeight || status.blockbook.bestHeight <= 0) {
    throw new Error(`${coin}: status missing positive blockbook.bestHeight`);
  }
  if (!status.backend || Object.keys(status.backend).length === 0) {
    throw new Error(`${coin}: status missing backend object`);
  }
}

function assertWsInfo(info: WsInfoResponse, coin: Coin) {
  if (!info.bestHeight || info.bestHeight <= 0) {
    throw new Error(`${coin}: websocket getInfo missing positive bestHeight`);
  }
  if (!info.bestHash) {
    throw new Error(`${coin}: websocket getInfo missing bestHash`);
  }
}

function assertTx(tx: TxResponse, txid: string) {
  if (tx.txid !== txid) {
    throw new Error(`transaction txid mismatch: got ${tx.txid}, want ${txid}`);
  }
  if (!Array.isArray(tx.vin) || !Array.isArray(tx.vout)) {
    throw new Error(`transaction ${txid} missing vin/vout arrays`);
  }
}

async function expectAnyAddressLookup(
  client: ReturnType<typeof createClient<paths>>,
  tx: TxResponse,
  coin: Coin,
  txid: string,
) {
  const addresses = addressesFromTx(tx);
  if (addresses.length === 0) {
    throw new Error(`${coin}: sampled tx ${txid} did not expose any address`);
  }

  const errors: string[] = [];
  for (const address of addresses) {
    const result = await client.GET("/api/v2/address/{address}", {
      params: { path: { address }, query: { details: "basic" } },
    });
    if (!result.error && result.data) {
      return;
    }
    errors.push(`${address}: HTTP ${result.response.status} ${JSON.stringify(result.error)}`);
  }

  throw new Error(`${coin}: no sampled tx address could be looked up for ${txid}: ${errors.join("; ")}`);
}

function isWsError(data: WsResponse["data"]): data is components["schemas"]["WsErrorData"] {
  return typeof data === "object" && data !== null && "error" in data;
}

function firstTxidFromBlock(
  block: paths["/api/v2/block/{blockId}"]["get"]["responses"]["200"]["content"]["application/json"],
): string | undefined {
  const fromFullTxs = block.txs?.find((tx) => tx.txid)?.txid;
  if (fromFullTxs) {
    return fromFullTxs;
  }
  return block.tx?.find((txid) => txid.trim() !== "");
}

function addressesFromTx(tx: TxResponse): string[] {
  const addresses: string[] = [];
  const seen = new Set<string>();
  const add = (value: string) => {
    const address = value.trim();
    if (address && !seen.has(address)) {
      seen.add(address);
      addresses.push(address);
    }
  };
  for (const input of tx.vin) {
    input.addresses?.forEach(add);
  }
  for (const output of tx.vout) {
    output.addresses?.forEach(add);
  }
  return addresses;
}

async function resolveHTTPBase(coin: Coin): Promise<string> {
  const cfg = loadCoinConfig(coin);
  const testIdentity = cfg.coin?.test_name || coin;
  const envCandidates = [
    `BB_DEV_API_URL_HTTP_${testIdentity}`,
    `BB_DEV_API_URL_HTTP_${testIdentity.replaceAll("-", "_")}`,
  ];
  let baseUrl = firstNonEmptyEnv(envCandidates);
  if (!baseUrl) {
    const port = cfg.ports?.blockbook_public;
    if (!port) {
      throw new Error(`${coin}: missing ports.blockbook_public and no BB_DEV_API_URL_HTTP override`);
    }
    baseUrl = `http://127.0.0.1:${port}`;
  }

  baseUrl = normalizeHTTPBase(baseUrl);
  try {
    const probe = await fetchText(`${baseUrl}/api/status`, 3000);
    if (
      probe.status === 400 &&
      probe.body.toLowerCase().includes("http request to an https server") &&
      baseUrl.startsWith("http:")
    ) {
      baseUrl = `https:${baseUrl.slice("http:".length)}`;
    }
  } catch (error) {
    if (!baseUrl.startsWith("http:")) {
      throw error;
    }
    const httpsBaseUrl = `https:${baseUrl.slice("http:".length)}`;
    await fetchText(`${httpsBaseUrl}/api/status`, 3000);
    baseUrl = httpsBaseUrl;
  }
  return baseUrl.replace(/\/+$/, "");
}

function resolveWSURL(coin: Coin, httpBase: string): string {
  const cfg = loadCoinConfig(coin);
  const testIdentity = cfg.coin?.test_name || coin;
  const envCandidates = [
    `BB_DEV_API_URL_WS_${testIdentity}`,
    `BB_DEV_API_URL_WS_${testIdentity.replaceAll("-", "_")}`,
  ];
  const explicitURL = firstNonEmptyEnv(envCandidates);
  if (explicitURL) {
    return normalizeWSURL(explicitURL);
  }

  const url = new URL(httpBase);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.pathname = "/websocket";
  url.search = "";
  url.hash = "";
  return url.toString();
}

function loadCoinConfig(coin: Coin) {
  const raw = fs.readFileSync(path.join(repoRoot, "configs", "coins", `${coin}.json`), "utf8");
  return JSON.parse(raw) as {
    coin?: { test_name?: string };
    ports?: { blockbook_public?: number };
  };
}

function firstNonEmptyEnv(keys: string[]) {
  for (const key of keys) {
    const value = process.env[key]?.trim();
    if (value) {
      return value;
    }
  }
  return "";
}

function normalizeHTTPBase(raw: string) {
  const url = new URL(raw);
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new Error(`unsupported HTTP URL scheme in ${raw}`);
  }
  url.search = "";
  url.hash = "";
  return url.toString().replace(/\/+$/, "");
}

function normalizeWSURL(raw: string) {
  const url = new URL(raw);
  if (url.protocol === "http:") {
    url.protocol = "ws:";
  } else if (url.protocol === "https:") {
    url.protocol = "wss:";
  } else if (url.protocol !== "ws:" && url.protocol !== "wss:") {
    throw new Error(`unsupported WebSocket URL scheme in ${raw}`);
  }
  if (!url.pathname || url.pathname === "/") {
    url.pathname = "/websocket";
  }
  url.search = "";
  url.hash = "";
  return url.toString();
}

async function fetchText(url: string, timeoutMs: number) {
  const response = await fetch(url, { signal: AbortSignal.timeout(timeoutMs) });
  return {
    status: response.status,
    body: await response.text(),
  };
}
