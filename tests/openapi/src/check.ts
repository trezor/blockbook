import path from "node:path";

import { loadTestsConfig, repoRoot } from "./config.js";
import { checkBlockbookAPIExports } from "./blockbook-api-compat.js";
import { validateCoverageMetadata } from "./coverage.js";
import { OpenApiContract } from "./openapi.js";
import { testRegistry } from "./registry.js";

const contract = new OpenApiContract(path.join(repoRoot, "openapi.yaml"));
const testsConfig = loadTestsConfig();
validateCoverageMetadata(contract, testsConfig, testRegistry);
checkBlockbookAPIExports();

console.log(`OpenAPI e2e metadata check passed: ${Object.keys(testRegistry).length} registered API tests`);
