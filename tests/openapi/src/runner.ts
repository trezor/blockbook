import path from "node:path";

import { Agent, setGlobalDispatcher } from "undici";

import { loadTestsConfig, repoRoot, resolveSelectedCoins } from "./config.js";
import { errorMessage, SkipTest } from "./errors.js";
import { OpenApiContract } from "./openapi.js";
import { testRegistry } from "./registry.js";
import { TestContext } from "./context.js";

if (process.env.OPENAPI_INSECURE_TLS !== "0") {
  setGlobalDispatcher(new Agent({ connect: { rejectUnauthorized: false } }));
}

export async function runOpenApiE2E() {
  const contract = new OpenApiContract(path.join(repoRoot, "openapi.yaml"));
  const testsConfig = loadTestsConfig();

  const selectedCoins = resolveSelectedCoins(testsConfig);
  if (selectedCoins.length === 0) {
    console.log("OpenAPI e2e: no selected API-enabled coins, skipping.");
    return;
  }

  const failures: string[] = [];
  for (const coin of selectedCoins) {
    await runCoin(coin, contract, failures);
  }

  if (failures.length > 0) {
    console.error(`\nOpenAPI e2e failed with ${failures.length} failure(s):`);
    for (const failure of failures) {
      console.error(`- ${failure}`);
    }
    process.exit(1);
  }

  console.log(`\nOpenAPI e2e passed for ${selectedCoins.length} coin(s): ${selectedCoins.join(", ")}`);
}

async function runCoin(coin: string, contract: OpenApiContract, failures: string[]) {
  const testsConfig = loadTestsConfig();
  const apiTests = testsConfig[coin]?.api ?? [];
  if (apiTests.length === 0) {
    console.log(`OpenAPI e2e ${coin}: no api tests configured, skipping.`);
    return;
  }

  const ctx = await TestContext.create(coin, contract);
  console.log(`\nOpenAPI e2e ${coin}: ${apiTests.length} tests`);
  await ctx.getStatus();

  for (const testName of apiTests) {
    const def = testRegistry[testName];
    if (!def) {
      failures.push(`${coin}/${testName}: test is listed in tests/tests.json but not implemented`);
      console.error(`  fail ${testName}: test is listed in tests/tests.json but not implemented`);
      continue;
    }

    const started = Date.now();
    try {
      if (def.capability) {
        await ctx.requireCapability(def.capability, def.group, testName);
      }
      await def.run(ctx);
      console.log(`  ok   ${testName} (${Date.now() - started}ms)`);
    } catch (error) {
      if (error instanceof SkipTest) {
        console.log(`  skip ${testName}: ${error.message}`);
        continue;
      }
      const message = errorMessage(error);
      failures.push(`${coin}/${testName}: ${message}`);
      console.error(`  fail ${testName}: ${message}`);
    }
  }
}
