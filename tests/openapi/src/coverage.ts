import fs from "node:fs";
import path from "node:path";

import { repoRoot } from "./config.js";

import type { OpenApiContract } from "./openapi.js";
import type { CoverageSink, TestConfig } from "./types.js";
import type { TestDefinition } from "./registry.js";

export type OperationCoverageTarget = {
  kind: "operation";
  method: "get" | "post";
  path: string;
  status?: number;
};

export type SchemaCoverageTarget = {
  kind: "schema";
  ref: string;
};

export type WebSocketCoverageTarget = {
  kind: "websocket";
  method: string;
  schemaRef?: string;
};

export type CoverageTarget = OperationCoverageTarget | SchemaCoverageTarget | WebSocketCoverageTarget;

export class CoverageRecorder implements CoverageSink {
  private readonly intendedTests = new Map<string, CoverageTarget[]>();
  private readonly observedOperations = new Set<string>();
  private readonly observedSchemaRefs = new Set<string>();
  private readonly observedWebSocketMethods = new Set<string>();

  recordIntendedTest(name: string, covers: CoverageTarget[]) {
    this.intendedTests.set(name, covers);
  }

  recordOperation(method: "get" | "post", operationPath: string, status: number) {
    this.observedOperations.add(operationKey(method, operationPath, status));
  }

  recordSchemaRef(ref: string) {
    this.observedSchemaRefs.add(ref);
  }

  recordWebSocketMethod(method: string) {
    this.observedWebSocketMethods.add(method);
  }

  summary() {
    return {
      intendedTests: this.intendedTests.size,
      observedOperations: this.observedOperations.size,
      observedSchemas: this.observedSchemaRefs.size,
      observedWebSocketMethods: this.observedWebSocketMethods.size,
    };
  }

  printSummary() {
    const summary = this.summary();
    console.log(
      `OpenAPI coverage: ${summary.intendedTests} planned test(s), ` +
      `${summary.observedOperations} observed REST operation(s), ` +
      `${summary.observedSchemas} observed schema ref(s), ` +
      `${summary.observedWebSocketMethods} observed websocket method(s)`,
    );
  }

  writeJSON() {
    const outputPath = process.env.OPENAPI_COVERAGE_JSON?.trim() ||
      path.join(repoRoot, "tests", "openapi", ".generated", "e2e-coverage.json");
    fs.mkdirSync(path.dirname(outputPath), { recursive: true });
    fs.writeFileSync(outputPath, `${JSON.stringify({
      summary: this.summary(),
      intendedTests: Object.fromEntries(this.intendedTests),
      observedOperations: [...this.observedOperations].sort(),
      observedSchemaRefs: [...this.observedSchemaRefs].sort(),
      observedWebSocketMethods: [...this.observedWebSocketMethods].sort(),
    }, null, 2)}\n`);
    console.log(`OpenAPI coverage report: ${outputPath}`);
  }
}

export function validateCoverageMetadata(
  contract: OpenApiContract,
  testsConfig: TestConfig,
  registry: Record<string, TestDefinition>,
) {
  const configured = new Set<string>();
  for (const cfg of Object.values(testsConfig)) {
    for (const name of cfg.api ?? []) {
      configured.add(name);
    }
  }

  const implemented = new Set(Object.keys(registry));
  const errors: string[] = [];
  for (const name of configured) {
    if (!implemented.has(name)) {
      errors.push(`tests/tests.json references API test ${name}, but TypeScript registry does not implement it`);
    }
  }
  for (const name of implemented) {
    if (!configured.has(name)) {
      errors.push(`TypeScript registry implements API test ${name}, but tests/tests.json never selects it`);
    }
  }

  for (const [name, def] of Object.entries(registry)) {
    if (def.covers.length === 0) {
      errors.push(`${name} has no coverage metadata`);
      continue;
    }
    for (const target of def.covers) {
      if (target.kind === "operation") {
        const status = target.status ?? 200;
        if (!contract.hasResponseSchema(target.method, target.path, status)) {
          errors.push(`${name} covers missing OpenAPI response: ${target.method.toUpperCase()} ${target.path} HTTP ${status}`);
        }
      } else if (target.kind === "schema") {
        if (!contract.hasSchemaRef(target.ref)) {
          errors.push(`${name} covers missing OpenAPI schema: ${target.ref}`);
        }
      } else if (target.schemaRef && !contract.hasSchemaRef(target.schemaRef)) {
        errors.push(`${name} covers missing websocket schema: ${target.schemaRef}`);
      }
    }
  }

  if (errors.length > 0) {
    throw new Error(`OpenAPI e2e coverage metadata failed:\n${errors.map((error) => `- ${error}`).join("\n")}`);
  }
}

export function op(path: string, method: "get" | "post" = "get", status = 200): CoverageTarget {
  return { kind: "operation", method, path, status };
}

export function schema(ref: string): CoverageTarget {
  return { kind: "schema", ref };
}

export function ws(method: string, schemaRef?: string): CoverageTarget {
  return { kind: "websocket", method, schemaRef };
}

function operationKey(method: "get" | "post", operationPath: string, status: number) {
  return `${method.toUpperCase()} ${operationPath} HTTP ${status}`;
}
