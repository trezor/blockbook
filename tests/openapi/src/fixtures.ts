import fs from "node:fs";
import path from "node:path";

import { repoRoot } from "./config.js";

import type { ApiTestData } from "./types.js";

export function loadAPITestData(coin: string) {
  const file = path.join(repoRoot, "tests", "openapi", "fixtures", `${coin}.json`);
  return JSON.parse(fs.readFileSync(file, "utf8")) as ApiTestData;
}
