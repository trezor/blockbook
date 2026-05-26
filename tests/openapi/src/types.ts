import type { components } from "../.generated/blockbook.js";
import type { GetResponse } from "./client.js";

export type StatusResponse = GetResponse<"/api/status">;
export type BlockHashResponse = GetResponse<"/api/v2/block-index/{height}">;
export type BlockResponse = GetResponse<"/api/v2/block/{blockId}">;
export type TxResponse = GetResponse<"/api/v2/tx/{txid}">;
export type AddressResponse = components["schemas"]["Address"];
export type UtxoResponse = components["schemas"]["Utxo"];
export type FiatTickerResponse = components["schemas"]["FiatTicker"];
export type AvailableVsCurrenciesResponse = components["schemas"]["AvailableVsCurrencies"];
export type ContractInfoResponse = components["schemas"]["ContractInfoResult"];
export type TokenResponse = components["schemas"]["Token"];
export type WsRequest = components["schemas"]["WsRequest"];
export type WsResponse = components["schemas"]["WsResponse"];
export type WsInfoResponse = components["schemas"]["WsInfoRes"];
export type WsBlockHashResponse = components["schemas"]["WsBlockHashRes"];
export type WsMethod = WsRequest["method"];
export type WsEnvelope = {
  id: string;
  method: WsMethod;
  params: unknown;
};

export type TestConfig = Record<string, {
  api?: string[];
  connectivity?: string[];
}>;

export type CoinConfig = {
  coin?: {
    test_name?: string;
  };
  ports?: {
    blockbook_public?: number;
  };
};

export type Erc4626Fixture = {
  name: string;
  holder: string;
  contract: string;
};

export type ApiTestData = {
  erc4626Fixtures?: Erc4626Fixture[];
  nonVaultContracts?: string[];
};

export type Capability = "utxo" | "evm";

export type BlockSummary = {
  hash: string;
  height: number;
  hasTxField: boolean;
  txIDs: string[];
  pageSize: number;
};

