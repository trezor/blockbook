import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import type { CoinConfig, TestConfig } from "./types.js";

export const repoRoot =
  process.env.REPO_ROOT ??
  path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../..");

export function loadTestsConfig() {
  return JSON.parse(fs.readFileSync(path.join(repoRoot, "tests", "tests.json"), "utf8")) as TestConfig;
}

// Maximum age (seconds) a current fiat ticker may have before it is considered stale.
// Coins refresh rates every 60s (BTC) … 900s (ETH); 2h gives margin against a transient
// CoinGecko skip while still flagging a feed stalled for hours.
export const DEFAULT_FIAT_MAX_AGE_SECONDS = 7200; // 2h

export function fiatMaxAgeSeconds() {
  const raw = process.env.BB_FIAT_MAX_AGE_SECONDS?.trim();
  if (raw) {
    const parsed = Number(raw);
    if (Number.isFinite(parsed) && parsed > 0) {
      return parsed;
    }
  }
  return DEFAULT_FIAT_MAX_AGE_SECONDS;
}

// allowOutOfSync lets dev runs proceed against a node still catching up to its backend. Off by
// default so CI fails loudly on a stale tip (which makes every recent-block sample unreliable).
// Set OPENAPI_ALLOW_OUT_OF_SYNC=1 to downgrade the inSync assertion in TestContext.getStatus.
export function allowOutOfSync() {
  const raw = process.env.OPENAPI_ALLOW_OUT_OF_SYNC?.trim().toLowerCase();
  return raw === "1" || raw === "true" || raw === "yes";
}

export function resolveSelectedCoins(config: TestConfig) {
  const raw = process.env.OPENAPI_COINS?.trim();
  const requested = raw
    ? raw.split(",").map((value) => value.trim()).filter(Boolean)
    : Object.entries(config).filter(([, value]) => value.api && value.api.length > 0).map(([coin]) => coin);

  const selected: string[] = [];
  const seen = new Set<string>();
  for (const coinOrConfig of requested) {
    const coin = resolveTestCoinName(coinOrConfig, config);
    if (seen.has(coin)) {
      continue;
    }
    seen.add(coin);
    if (!config[coin]?.api || config[coin].api.length === 0) {
      console.log(`OpenAPI e2e: ${coinOrConfig} maps to ${coin}, which has no api tests in tests/tests.json; skipping.`);
      continue;
    }
    selected.push(coin);
  }
  return selected;
}

export function resolveTestCoinName(coinOrConfig: string, config: TestConfig) {
  if (config[coinOrConfig]) {
    return coinOrConfig;
  }
  const configPath = path.join(repoRoot, "configs", "coins", `${coinOrConfig}.json`);
  if (!fs.existsSync(configPath)) {
    throw new Error(`unknown coin '${coinOrConfig}' (missing ${configPath})`);
  }
  const configData = JSON.parse(fs.readFileSync(configPath, "utf8")) as CoinConfig;
  return configData.coin?.test_name?.trim() || coinOrConfig;
}

export async function resolveHTTPBase(coin: string) {
  const cfg = loadCoinConfig(coin);
  const testIdentity = cfg.coin?.test_name?.trim() || coin;
  const candidates = [
    `BB_DEV_API_URL_HTTP_${testIdentity}`,
    `BB_DEV_API_URL_HTTP_${testIdentity.replaceAll("-", "_")}`,
  ];

  let baseUrl = firstNonEmptyEnv(candidates);
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

export function resolveWSURL(coin: string, httpBase: string) {
  const cfg = loadCoinConfig(coin);
  const testIdentity = cfg.coin?.test_name?.trim() || coin;
  const candidates = [
    `BB_DEV_API_URL_WS_${testIdentity}`,
    `BB_DEV_API_URL_WS_${testIdentity.replaceAll("-", "_")}`,
  ];
  const explicitURL = firstNonEmptyEnv(candidates);
  if (explicitURL) {
    return normalizeWSURL(explicitURL);
  }

  const url = new URL(httpBase);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.pathname = !url.pathname || url.pathname === "/"
    ? "/websocket"
    : `${url.pathname.replace(/\/+$/, "")}/websocket`;
  url.search = "";
  url.hash = "";
  return url.toString();
}

export function loadCoinConfig(coin: string) {
  const raw = fs.readFileSync(path.join(repoRoot, "configs", "coins", `${coin}.json`), "utf8");
  return JSON.parse(raw) as CoinConfig;
}

// Golomb block-filter settings for a coin, read from its config (the source of truth for the
// instance's BlockFilterScripts/BlockGolombFilterP). scriptType is "" when block filters are
// not configured, which callers use to skip the ws filter tests.
export function blockFilterConfig(coin: string) {
  const params = loadCoinConfig(coin).blockbook?.block_chain?.additional_params ?? {};
  return {
    scriptType: (params.block_filter_scripts ?? "").trim(),
    golombP: params.block_golomb_filter_p ?? 0,
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
