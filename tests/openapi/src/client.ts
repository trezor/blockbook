import { userAgentHeaders } from "./config.js";
import { OpenApiContract, preview } from "./openapi.js";

import type { paths } from "../.generated/blockbook.js";

export type GetOperationPath = keyof {
  [P in keyof paths as paths[P] extends { get: unknown } ? P : never]: true;
} & string;

type GetOperation<P extends GetOperationPath> = paths[P] extends { get: infer Operation } ? Operation : never;

type ResponseMap<P extends GetOperationPath> = GetOperation<P> extends { responses: infer Responses } ? Responses : never;

type Response200<P extends GetOperationPath> =
  ResponseMap<P> extends { 200: infer Response }
    ? Response
    : ResponseMap<P> extends { "200": infer Response }
      ? Response
      : never;

export type GetResponse<P extends GetOperationPath> =
  Response200<P> extends { content: { "application/json": infer Body } }
    ? NonNullable<Body>
    : never;

export type HttpResult<T = unknown> = {
  status: number;
  data?: T;
  body: string;
};

export class OpenApiFetchClient {
  constructor(
    private baseUrl: string,
    private readonly contract: OpenApiContract,
  ) {}

  getBaseUrl() {
    return this.baseUrl;
  }

  setBaseUrl(baseUrl: string) {
    this.baseUrl = baseUrl.replace(/\/+$/, "");
  }

  async getJson<P extends GetOperationPath>(
    operationPath: P,
    actualPath: string,
    label = `GET ${operationPath}`,
  ): Promise<GetResponse<P>> {
    const result = await this.getMaybe(operationPath, actualPath);
    if (result.status !== 200 || result.data === undefined) {
      throw new Error(`${label} returned HTTP ${result.status}: ${preview(result.body)}`);
    }
    return result.data;
  }

  async getMaybe<P extends GetOperationPath>(operationPath: P, actualPath: string): Promise<HttpResult<GetResponse<P>>> {
    const url = this.resolveUrl(actualPath);
    let lastError: unknown;
    for (let attempt = 1; attempt <= maxAttempts; attempt++) {
      try {
        const response = await fetch(url, {
          method: "GET",
          signal: AbortSignal.timeout(25_000),
          headers: userAgentHeaders(),
        });
        const body = await response.text();
        if (attempt < maxAttempts && isRetryableHTTPStatus(response.status)) {
          await delay(retryDelayMs(attempt));
          continue;
        }
        const data = parseJSON(body);
        if (response.status === 200) {
          this.contract.validateResponse("get", operationPath, response.status, data);
        }
        return {
          status: response.status,
          data: data as GetResponse<P>,
          body,
        };
      } catch (error) {
        lastError = error;
        if (attempt < maxAttempts && isRetryableError(error)) {
          await delay(retryDelayMs(attempt));
          continue;
        }
        throw error;
      }
    }
    return {
      status: 0,
      body: lastError instanceof Error ? lastError.message : String(lastError),
    };
  }

  resolveUrl(path: string) {
    if (/^https?:\/\//.test(path)) {
      return path;
    }
    const suffix = path.startsWith("/") ? path : `/${path}`;
    return `${this.baseUrl.replace(/\/+$/, "")}${suffix}`;
  }
}

// Edge/LB 5xx bursts in front of an instance last seconds, not one round-trip; back off across a
// few attempts so a single blip does not fail a whole coin run at the status preflight.
const maxAttempts = 4;

function retryDelayMs(attempt: number) {
  return 500 * 2 ** (attempt - 1);
}

function parseJSON(body: string): unknown {
  if (body.trim() === "") {
    return undefined;
  }
  return JSON.parse(body) as unknown;
}

function isRetryableHTTPStatus(status: number) {
  return status === 502 || status === 503 || status === 504;
}

function isRetryableError(error: unknown) {
  if (!(error instanceof Error)) {
    return false;
  }
  const message = error.message.toLowerCase();
  return message.includes("fetch failed") || message.includes("terminated") || message.includes("econnreset");
}

function delay(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
