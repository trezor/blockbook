import fs from "node:fs";
import { createRequire } from "node:module";

import type { ValidateFunction } from "ajv";
import YAML from "yaml";

const require = createRequire(import.meta.url);

type AjvInstance = {
  addFormat: (name: string, format: unknown) => void;
  compile: (schema: unknown) => ValidateFunction;
  errorsText: (errors: ValidateFunction["errors"], options?: { dataVar?: string; separator?: string }) => string;
};

type AjvConstructor = new (options: Record<string, unknown>) => AjvInstance;

const ajvModule = require("ajv/dist/2020") as { default?: AjvConstructor } | AjvConstructor;
const Ajv2020 = ("default" in ajvModule && ajvModule.default ? ajvModule.default : ajvModule) as AjvConstructor;
const addFormatsModule = require("ajv-formats") as { default?: (ajv: AjvInstance) => void } | ((ajv: AjvInstance) => void);
const addFormats = ("default" in addFormatsModule && addFormatsModule.default ? addFormatsModule.default : addFormatsModule) as (ajv: AjvInstance) => void;

type JsonObject = Record<string, unknown>;
type HttpMethod = "get" | "post";

type OpenApiOperation = {
  responses?: Record<string, {
    content?: Record<string, {
      schema?: unknown;
    }>;
  }>;
};

type OpenApiDocument = {
  paths?: Record<string, Partial<Record<HttpMethod, OpenApiOperation>>>;
  components?: {
    schemas?: Record<string, unknown>;
  };
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
  }

  validateResponse(method: HttpMethod, operationPath: string, status: number, data: unknown) {
    const schema = this.responseSchema(method, operationPath, status);
    if (schema === undefined) {
      throw new Error(`${method.toUpperCase()} ${operationPath} has no JSON schema for HTTP ${status}`);
    }
    this.validateSchema(schema, `${method.toUpperCase()} ${operationPath} HTTP ${status}`, data);
  }

  validateSchemaRef(ref: string, label: string, data: unknown) {
    this.validateSchema({ $ref: ref }, label, data);
  }

  hasResponseSchema(method: HttpMethod, operationPath: string, status = 200) {
    return this.responseSchema(method, operationPath, status) !== undefined;
  }

  hasSchemaRef(ref: string) {
    try {
      this.resolvePointer(ref);
      return true;
    } catch {
      return false;
    }
  }

  validateSchema(schema: unknown, label: string, data: unknown) {
    const validator = this.compile(schema, label);
    if (validator(data)) {
      return;
    }
    const details = this.ajv.errorsText(validator.errors, {
      dataVar: label,
      separator: "\n",
    });
    throw new Error(`${label} failed OpenAPI schema validation:\n${details}`);
  }

  private responseSchema(method: HttpMethod, operationPath: string, status: number) {
    const operation = this.document.paths?.[operationPath]?.[method];
    const response = operation?.responses?.[String(status)] ?? operation?.responses?.default;
    return response?.content?.["application/json"]?.schema;
  }

  private compile(schema: unknown, label: string) {
    const key = `${label}:${JSON.stringify(schema)}`;
    const cached = this.validators.get(key);
    if (cached) {
      return cached;
    }

    const dereferenced = this.dereference(schema, new Set());
    const validator = this.ajv.compile(dereferenced);
    this.validators.set(key, validator);
    return validator;
  }

  private dereference(value: unknown, seenRefs: Set<string>): unknown {
    if (Array.isArray(value)) {
      return value.map((item) => this.dereference(item, seenRefs));
    }
    if (!isObject(value)) {
      return value;
    }

    const ref = typeof value.$ref === "string" ? value.$ref : "";
    if (ref) {
      const resolved = seenRefs.has(ref)
        ? {}
        : this.dereference(this.resolvePointer(ref), new Set([...seenRefs, ref]));
      const siblings = Object.fromEntries(Object.entries(value).filter(([key]) => key !== "$ref"));
      if (Object.keys(siblings).length === 0) {
        return resolved;
      }
      return {
        allOf: [
          resolved,
          this.dereference(siblings, seenRefs),
        ],
      };
    }

    return Object.fromEntries(
      Object.entries(value).map(([key, nested]) => [key, this.dereference(nested, seenRefs)]),
    );
  }

  private resolvePointer(ref: string) {
    if (!ref.startsWith("#/")) {
      throw new Error(`unsupported non-local OpenAPI $ref: ${ref}`);
    }
    let cursor: unknown = this.document;
    for (const rawPart of ref.slice(2).split("/")) {
      const part = rawPart.replaceAll("~1", "/").replaceAll("~0", "~");
      if (!isObject(cursor) && !Array.isArray(cursor)) {
        throw new Error(`invalid OpenAPI $ref ${ref}`);
      }
      cursor = (cursor as Record<string, unknown>)[part];
    }
    if (cursor === undefined) {
      throw new Error(`OpenAPI $ref not found: ${ref}`);
    }
    return cursor;
  }
}

export function preview(body: string | Uint8Array, limit = 600) {
  const text = typeof body === "string" ? body : Buffer.from(body).toString("utf8");
  if (text.length <= limit) {
    return text;
  }
  return `${text.slice(0, limit)}...`;
}

function isObject(value: unknown): value is JsonObject {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
