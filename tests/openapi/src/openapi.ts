import fs from "node:fs";
import { createRequire } from "node:module";

import type { ValidateFunction } from "ajv";
import YAML from "yaml";

const require = createRequire(import.meta.url);

type AjvInstance = {
  addFormat: (name: string, format: unknown) => void;
  addSchema: (schema: unknown, key?: string) => void;
  compile: (schema: unknown) => ValidateFunction;
  errorsText: (errors: ValidateFunction["errors"], options?: { dataVar?: string; separator?: string }) => string;
};

type AjvConstructor = new (options: Record<string, unknown>) => AjvInstance;

const ajvModule = require("ajv/dist/2020") as { default?: AjvConstructor } | AjvConstructor;
const Ajv2020 = ("default" in ajvModule && ajvModule.default ? ajvModule.default : ajvModule) as AjvConstructor;
const addFormatsModule = require("ajv-formats") as { default?: (ajv: AjvInstance) => void } | ((ajv: AjvInstance) => void);
const addFormats = ("default" in addFormatsModule && addFormatsModule.default ? addFormatsModule.default : addFormatsModule) as (ajv: AjvInstance) => void;

const documentId = "openapi://blockbook";

type HttpMethod = "get" | "post";

type OpenApiDocument = {
  paths?: Record<string, Partial<Record<HttpMethod, {
    responses?: Record<string, {
      content?: Record<string, { schema?: unknown }>;
    }>;
  }>>>;
  components?: { schemas?: Record<string, unknown> };
};

export class OpenApiContract {
  private readonly document: OpenApiDocument;
  private readonly ajv: AjvInstance;
  private readonly validators = new Map<string, ValidateFunction>();

  constructor(openApiPath: string) {
    this.document = YAML.parse(fs.readFileSync(openApiPath, "utf8")) as OpenApiDocument;
    this.ajv = new Ajv2020({
      allErrors: true,
      strict: false,
      validateFormats: true,
    });
    addFormats(this.ajv);
    this.ajv.addFormat("int64", true);
    this.ajv.addSchema({ ...this.document, $id: documentId });
  }

  validateResponse(method: HttpMethod, operationPath: string, status: number, data: unknown) {
    if (!this.findResponseSchema(method, operationPath, status)) {
      throw new Error(`${method.toUpperCase()} ${operationPath} has no JSON schema for HTTP ${status}`);
    }
    const pointer = responsePointer(method, operationPath, status);
    this.run(this.validatorFor(pointer), `${method.toUpperCase()} ${operationPath} HTTP ${status}`, data);
  }

  validateSchemaRef(ref: string, label: string, data: unknown) {
    this.run(this.validatorFor(absolutePointer(ref)), label, data);
  }

  private run(validator: ValidateFunction, label: string, data: unknown) {
    if (validator(data)) {
      return;
    }
    const details = this.ajv.errorsText(validator.errors, { dataVar: label, separator: "\n" });
    throw new Error(`${label} failed OpenAPI schema validation:\n${details}`);
  }

  private validatorFor(absoluteRef: string) {
    const cached = this.validators.get(absoluteRef);
    if (cached) {
      return cached;
    }
    const validator = this.ajv.compile({ $ref: absoluteRef });
    this.validators.set(absoluteRef, validator);
    return validator;
  }

  private findResponseSchema(method: HttpMethod, operationPath: string, status: number) {
    const operation = this.document.paths?.[operationPath]?.[method];
    const response = operation?.responses?.[String(status)] ?? operation?.responses?.default;
    return response?.content?.["application/json"]?.schema;
  }
}

export function preview(body: string | Uint8Array, limit = 600) {
  const text = typeof body === "string" ? body : Buffer.from(body).toString("utf8");
  if (text.length <= limit) {
    return text;
  }
  return `${text.slice(0, limit)}...`;
}

function responsePointer(method: HttpMethod, operationPath: string, status: number) {
  return `${documentId}#/paths/${encodePointer(operationPath)}/${method}/responses/${status}/content/${encodePointer("application/json")}/schema`;
}

function absolutePointer(ref: string) {
  if (ref.startsWith("#")) {
    return `${documentId}${ref}`;
  }
  return ref;
}

function encodePointer(segment: string) {
  return segment.replace(/~/g, "~0").replace(/\//g, "~1");
}
