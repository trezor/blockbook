import fs from "node:fs";
import path from "node:path";

import { repoRoot } from "./config.js";

const requiredBlockbookApiInterfaces = [
  "Tx",
  "Address",
  "ContractInfoResult",
  "Utxo",
  "Block",
  "SystemInfo",
  "FiatTicker",
  "AvailableVsCurrencies",
  "WsReq",
  "WsRes",
  "WsInfoRes",
  "WsBlockHashRes",
];

const knownWireShapeDrift = [
  "Block.version is string in blockbook-api.ts, while OpenAPI allows string or integer because Ethereum returns numbers.",
  "Vout.addresses is string[] in blockbook-api.ts, while OpenAPI allows null because nil Go slices serialize as null.",
];

export function checkBlockbookAPIExports() {
  const file = path.join(repoRoot, "blockbook-api.ts");
  const source = fs.readFileSync(file, "utf8");
  if (!source.includes("generated from Golang structs")) {
    throw new Error("blockbook-api.ts does not look like the generated Go-struct TypeScript file");
  }

  const exportedInterfaces = new Set(
    [...source.matchAll(/^export interface ([A-Za-z0-9_]+)/gm)].map((match) => match[1]),
  );
  const missing = requiredBlockbookApiInterfaces.filter((name) => !exportedInterfaces.has(name));
  if (missing.length > 0) {
    throw new Error(`blockbook-api.ts is missing expected public API interfaces: ${missing.join(", ")}`);
  }

  console.log(
    `blockbook-api.ts compatibility check passed: ${requiredBlockbookApiInterfaces.length} shared interface exports present`,
  );
  console.log(`blockbook-api.ts known wire-shape differences: ${knownWireShapeDrift.length}`);
}
