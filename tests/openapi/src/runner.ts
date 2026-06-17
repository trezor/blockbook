import path from "node:path";

import { Agent, ProxyAgent, setGlobalDispatcher } from "undici";

import { loadTestsConfig, repoRoot, resolveSelectedCoins } from "./config.js";
import { errorMessage, SkipTest } from "./errors.js";
import { OpenApiContract } from "./openapi.js";
import { CoinSummary, summarize, TestResult, writeReports } from "./report.js";
import { testRegistry } from "./registry.js";
import { proxyFromEnv, TestContext } from "./context.js";

// Configure the global fetch() dispatcher. When an egress proxy is set (e.g. a sandboxed or
// corporate network that only allows outbound traffic via HTTP(S)_PROXY), route fetch() through it;
// requestTls disables certificate verification for the proxied target when OPENAPI_INSECURE_TLS is
// on (dev backends use self-signed certs). With no proxy this is a no-op and we keep the previous
// plain insecure-TLS Agent. The ws client does not use undici's dispatcher, so its matching proxy
// wiring lives in context.ts (wsProxyAgent).
const insecureTLS = process.env.OPENAPI_INSECURE_TLS !== "0";
const insecureConnect = insecureTLS ? { rejectUnauthorized: false } : undefined;
const egressProxy = proxyFromEnv();
if (egressProxy) {
  setGlobalDispatcher(new ProxyAgent({ uri: egressProxy, ...(insecureConnect ? { requestTls: insecureConnect } : {}) }));
} else if (insecureTLS) {
  setGlobalDispatcher(new Agent({ connect: insecureConnect }));
}

export async function runOpenApiE2E() {
  const contract = new OpenApiContract(path.join(repoRoot, "openapi.yaml"));
  const testsConfig = loadTestsConfig();

  const selectedCoins = resolveSelectedCoins(testsConfig);
  if (selectedCoins.length === 0) {
    console.log("OpenAPI e2e: no selected API-enabled coins, skipping.");
    return;
  }

  const summaries: CoinSummary[] = [];
  for (const coin of selectedCoins) {
    summaries.push(await runCoin(coin, contract));
  }

  // Per-coin and aggregate summary so a run that is green-but-empty (everything skipped) is
  // visible at a glance, not hidden behind a zero-failure exit.
  console.log("\nOpenAPI e2e summary:");
  for (const summary of summaries) {
    console.log(`  ${summary.coin}: ${summary.ok} ok, ${summary.skip} skip, ${summary.fail} fail`);
  }
  const totals = summaries.reduce(
    (acc, s) => ({ ok: acc.ok + s.ok, skip: acc.skip + s.skip, fail: acc.fail + s.fail }),
    { ok: 0, skip: 0, fail: 0 },
  );
  console.log(`  total: ${totals.ok} ok, ${totals.skip} skip, ${totals.fail} fail`);

  // Machine-readable artifact for CI (opt-in via OPENAPI_JUNIT_PATH). A requested-but-unwritable
  // report is itself a failure, so CI never silently loses results.
  try {
    for (const reportPath of writeReports(summaries)) {
      console.log(`  report written: ${reportPath}`);
    }
  } catch (error) {
    console.error(`OpenAPI e2e: failed to write report: ${errorMessage(error)}`);
    process.exit(1);
  }

  const failures = summaries.flatMap((summary) =>
    summary.results
      .filter((result) => result.status === "fail")
      .map((result) => `${result.coin}/${result.name}: ${result.message ?? "failed"}`),
  );
  if (failures.length > 0) {
    console.error(`\nOpenAPI e2e failed with ${failures.length} failure(s):`);
    for (const failure of failures) {
      console.error(`- ${failure}`);
    }
    process.exit(1);
  }

  console.log(`\nOpenAPI e2e passed for ${summaries.length} coin(s): ${selectedCoins.join(", ")}`);
}

async function runCoin(coin: string, contract: OpenApiContract): Promise<CoinSummary> {
  const testsConfig = loadTestsConfig();
  const apiTests = testsConfig[coin]?.api ?? [];
  const results: TestResult[] = [];
  if (apiTests.length === 0) {
    console.log(`OpenAPI e2e ${coin}: no api tests configured, skipping.`);
    return summarize(coin, results);
  }

  const ctx = await TestContext.create(coin, contract);
  console.log(`\nOpenAPI e2e ${coin}: ${apiTests.length} tests`);

  // Status preflight: if the node is unreachable or not in sync, every sample-derived test would
  // fail or skip downstream for the same root cause. Surface it once as a single coin-level failure
  // instead of N confusing ones, and stop here (the 0-ok guard would fire anyway).
  try {
    await ctx.getStatus();
  } catch (error) {
    const message = errorMessage(error);
    console.error(`  fail (status preflight): ${message}`);
    results.push({ coin, name: "StatusPreflight", group: "preflight", status: "fail", durationMs: 0, message });
    return summarize(coin, results);
  }

  for (const testName of apiTests) {
    const def = testRegistry[testName];
    if (!def) {
      const message = "test is listed in tests/tests.json but not implemented";
      console.error(`  fail ${testName}: ${message}`);
      results.push({ coin, name: testName, group: "unknown", status: "fail", durationMs: 0, message });
      continue;
    }

    const started = Date.now();
    try {
      if (def.capability) {
        await ctx.requireCapability(def.capability, def.group, testName);
      }
      await def.run(ctx);
      const durationMs = Date.now() - started;
      console.log(`  ok   ${testName} (${durationMs}ms)`);
      results.push({ coin, name: testName, group: def.group, status: "ok", durationMs });
    } catch (error) {
      const durationMs = Date.now() - started;
      if (error instanceof SkipTest) {
        console.log(`  skip ${testName}: ${error.message}`);
        results.push({ coin, name: testName, group: def.group, status: "skip", durationMs, message: error.message });
        continue;
      }
      const message = errorMessage(error);
      console.error(`  fail ${testName}: ${message}`);
      results.push({ coin, name: testName, group: def.group, status: "fail", durationMs, message });
    }
  }

  // Silent-green guard: a coin that ran tests but passed none (all skipped, none failed) is not a
  // pass — it usually means the recent-block window had no usable sample or a capability probe
  // turned everything off. Flag it as a failure so CI does not report green on zero coverage.
  const summary = summarize(coin, results);
  if (summary.ok === 0 && summary.fail === 0 && results.length > 0) {
    const message = `coin ${coin} had 0 passing tests (${summary.skip} skipped) — likely not in sync or the recent-block window yielded no usable sample data`;
    console.error(`  fail (coin health): ${message}`);
    results.push({ coin, name: "CoinHealth", group: "health", status: "fail", durationMs: 0, message });
    return summarize(coin, results);
  }

  return summary;
}
