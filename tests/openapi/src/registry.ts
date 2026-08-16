import { commonTests } from "./tests/common.js";
import { evmOnlyTests } from "./tests/evm.js";
import { utxoOnlyTests } from "./tests/utxo.js";
import { wsEVMTests, wsOnlyTests, wsUTXOTests } from "./tests/websocket.js";

import type { TestContext } from "./context.js";
import type { Capability } from "./types.js";

export type TestFunction = (ctx: TestContext) => Promise<void>;

export type TestDefinition = {
  run: TestFunction;
  capability?: Capability;
  group: string;
};

export const testRegistry = buildTestRegistry();

function buildTestRegistry() {
  const registry: Record<string, TestDefinition> = {};
  addTests(registry, "common", undefined, commonTests);
  addTests(registry, "utxo-only", "utxo", utxoOnlyTests);
  addTests(registry, "evm-only", "evm", {
    ...evmOnlyTests,
    ...wsEVMTests,
  });
  addTests(registry, "ws-only", undefined, wsOnlyTests);
  addTests(registry, "ws-utxo", "utxo", wsUTXOTests);
  return registry;
}

function addTests(
  registry: Record<string, TestDefinition>,
  group: string,
  capability: Capability | undefined,
  tests: Record<string, TestFunction>,
) {
  for (const [name, run] of Object.entries(tests)) {
    if (registry[name]) {
      throw new Error(`duplicate api test definition: ${name}`);
    }
    registry[name] = { run, capability, group };
  }
}
