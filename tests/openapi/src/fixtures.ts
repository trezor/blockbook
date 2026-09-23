import fs from "node:fs";
import path from "node:path";

import { repoRoot } from "./config.js";

import type { ApiTestData } from "./types.js";

export function loadAPITestData(coin: string): ApiTestData {
  const file = path.join(repoRoot, "tests", "openapi", "fixtures", `${coin}.json`);
  // no fixture file means no fixtures: fixture-driven tests then skip instead of failing on ENOENT
  if (!fs.existsSync(file)) {
    return {};
  }
  return JSON.parse(fs.readFileSync(file, "utf8")) as ApiTestData;
}
