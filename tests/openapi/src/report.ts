import fs from "node:fs";
import path from "node:path";

export type TestStatus = "ok" | "skip" | "fail";

export interface TestResult {
  coin: string;
  name: string;
  group: string;
  status: TestStatus;
  durationMs: number;
  message?: string;
}

export interface CoinSummary {
  coin: string;
  ok: number;
  skip: number;
  fail: number;
  results: TestResult[];
}

export function summarize(coin: string, results: TestResult[]): CoinSummary {
  return {
    coin,
    ok: results.filter((r) => r.status === "ok").length,
    skip: results.filter((r) => r.status === "skip").length,
    fail: results.filter((r) => r.status === "fail").length,
    results,
  };
}

// writeReports emits a JUnit XML report when OPENAPI_JUNIT_PATH points at a file path, so CI test
// reporters can enforce results (distinguish skips from passes, annotate PRs) instead of scraping
// stdout. Opt-in; console output is unaffected.
export function writeReports(summaries: CoinSummary[]): string[] {
  const written: string[] = [];
  const junitPath = process.env.OPENAPI_JUNIT_PATH?.trim();
  if (junitPath) {
    writeFile(junitPath, toJUnitXML(summaries));
    written.push(junitPath);
  }
  return written;
}

function writeFile(filePath: string, content: string): void {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
  fs.writeFileSync(filePath, content);
}

export function toJUnitXML(summaries: CoinSummary[]): string {
  const totalTests = summaries.reduce((n, s) => n + s.results.length, 0);
  const totalFail = summaries.reduce((n, s) => n + s.fail, 0);
  const totalSkip = summaries.reduce((n, s) => n + s.skip, 0);
  const totalTime = seconds(summaries.reduce((n, s) => n + s.results.reduce((m, r) => m + r.durationMs, 0), 0));

  const lines: string[] = [];
  lines.push(`<?xml version="1.0" encoding="UTF-8"?>`);
  lines.push(`<testsuites name="blockbook-openapi-e2e" tests="${totalTests}" failures="${totalFail}" skipped="${totalSkip}" time="${totalTime}">`);
  for (const summary of summaries) {
    const suiteTime = seconds(summary.results.reduce((m, r) => m + r.durationMs, 0));
    lines.push(`  <testsuite name="${attr(summary.coin)}" tests="${summary.results.length}" failures="${summary.fail}" skipped="${summary.skip}" time="${suiteTime}">`);
    for (const result of summary.results) {
      const open = `    <testcase name="${attr(result.name)}" classname="${attr(`${summary.coin}.${result.group}`)}" time="${seconds(result.durationMs)}">`;
      if (result.status === "ok") {
        lines.push(`${open.slice(0, -1)}/>`);
      } else if (result.status === "skip") {
        lines.push(open);
        lines.push(`      <skipped message="${attr(result.message ?? "")}"/>`);
        lines.push(`    </testcase>`);
      } else {
        lines.push(open);
        lines.push(`      <failure message="${attr(result.message ?? "")}">${text(result.message ?? "")}</failure>`);
        lines.push(`    </testcase>`);
      }
    }
    lines.push(`  </testsuite>`);
  }
  lines.push(`</testsuites>`);
  return `${lines.join("\n")}\n`;
}

function seconds(ms: number): string {
  return (ms / 1000).toFixed(3);
}

// Escape the five XML entity characters for attribute context (quotes included).
function attr(value: string): string {
  return sanitize(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&apos;");
}

// Escape for element-text context (quotes are legal in text).
function text(value: string): string {
  return sanitize(value).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

// Replace control chars that are illegal in XML 1.0, keeping tab (0x09), newline (0x0a) and
// carriage return (0x0d), so an arbitrary error message can never produce an invalid document.
function sanitize(value: string): string {
  let out = "";
  for (const ch of value) {
    const code = ch.codePointAt(0) ?? 0;
    out += code < 0x20 && code !== 0x09 && code !== 0x0a && code !== 0x0d ? " " : ch;
  }
  return out;
}
