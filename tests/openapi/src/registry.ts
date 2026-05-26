import { op, schema, ws } from "./coverage.js";
import { commonTests } from "./tests/common.js";
import { evmOnlyTests } from "./tests/evm.js";
import { utxoOnlyTests } from "./tests/utxo.js";
import { wsEVMTests, wsOnlyTests, wsUTXOTests } from "./tests/websocket.js";

import type { CoverageTarget } from "./coverage.js";
import type { TestContext } from "./context.js";
import type { Capability } from "./types.js";

export type TestFunction = (ctx: TestContext) => Promise<void>;

export type TestDefinition = {
  run: TestFunction;
  capability?: Capability;
  group: string;
  covers: CoverageTarget[];
};

const coverageByTest: Record<string, CoverageTarget[]> = {
  Status: [op("/api/status")],
  GetBlockIndex: [op("/api/v2/block-index/{height}")],
  GetBlockByHeight: [op("/api/v2/block/{blockId}")],
  GetBlock: [op("/api/v2/block/{blockId}")],
  GetTransaction: [op("/api/v2/tx/{txid}")],
  GetTransactionSpecific: [op("/api/v2/tx-specific/{txid}")],
  GetAddress: [op("/api/v2/address/{address}")],
  GetAddressTxids: [op("/api/v2/address/{address}")],
  GetAddressTxs: [op("/api/v2/address/{address}")],
  GetAddressTxsScientificNotation: [op("/api/v2/address/{address}"), op("/api/v2/tx-specific/{txid}")],
  GetCurrentFiatRates: [op("/api/v2/tickers/")],
  GetTickersList: [op("/api/v2/tickers-list/")],
  GetMultiTickers: [op("/api/v2/tickers-list/"), op("/api/v2/tickers/"), op("/api/v2/multi-tickers/")],

  GetUtxo: [op("/api/v2/utxo/{descriptor}")],
  GetUtxoConfirmedFilter: [op("/api/v2/utxo/{descriptor}")],

  GetAddressBasicEVM: [op("/api/v2/address/{address}")],
  GetAddressTokensEVM: [op("/api/v2/address/{address}")],
  GetAddressTokenBalances: [op("/api/v2/address/{address}")],
  GetAddressProtocolsEVM: [op("/api/v2/address/{address}")],
  GetAddressProtocolsOptInEVM: [op("/api/v2/address/{address}")],
  GetContractInfoEVM: [op("/api/v2/contract/{contract}")],
  GetContractInfoOptInEVM: [op("/api/v2/contract/{contract}")],
  GetContractInfoNonVaultEVM: [op("/api/v2/contract/{contract}")],
  Erc4626FeeInvariantEVM: [op("/api/v2/contract/{contract}"), schema("#/components/schemas/ContractInfoResult")],
  GetAddressTxidsPaginationEVM: [op("/api/v2/address/{address}")],
  GetAddressTxsPaginationEVM: [op("/api/v2/address/{address}")],
  GetAddressContractFilterEVM: [op("/api/v2/address/{address}")],
  GetTransactionEVMShape: [op("/api/v2/tx/{txid}")],

  WsGetInfo: [ws("getInfo", "#/components/schemas/WsInfoRes")],
  WsGetBlockHash: [ws("getBlockHash", "#/components/schemas/WsBlockHashRes")],
  WsGetTransaction: [ws("getTransaction", "#/components/schemas/Tx")],
  WsGetAccountInfo: [ws("getAccountInfo", "#/components/schemas/Address")],
  WsGetAccountInfoBasic: [ws("getAccountInfo", "#/components/schemas/Address")],
  WsGetAccountUtxo: [ws("getAccountUtxo", "#/components/schemas/Utxo")],
  WsPing: [ws("ping")],

  WsGetAccountInfoBasicEVM: [ws("getAccountInfo", "#/components/schemas/Address")],
  WsGetAccountInfoEVM: [ws("getAccountInfo", "#/components/schemas/Address")],
  WsGetAccountInfoTxidsConsistencyEVM: [op("/api/v2/address/{address}"), ws("getAccountInfo", "#/components/schemas/Address")],
  WsGetAccountInfoTxsConsistencyEVM: [op("/api/v2/address/{address}"), ws("getAccountInfo", "#/components/schemas/Address")],
  WsGetAccountInfoContractFilterEVM: [ws("getAccountInfo", "#/components/schemas/Address")],
  WsGetAccountInfoProtocolsEVM: [ws("getAccountInfo", "#/components/schemas/Address")],
  WsGetContractInfoEVM: [ws("getContractInfo", "#/components/schemas/ContractInfoResult")],
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
    const covers = coverageByTest[name];
    if (!covers) {
      throw new Error(`missing coverage metadata for api test definition: ${name}`);
    }
    registry[name] = { run, capability, group, covers };
  }
}
