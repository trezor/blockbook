# Blockbook API

The canonical Blockbook API documentation is now the OpenAPI specification:

- [openapi.yaml](../openapi.yaml)

Every Blockbook public server also serves the same specification and a local
Swagger UI:

- `/api-docs/` - read-only Swagger UI
- `/api-docs/openapi.yaml` - OpenAPI specification used by Swagger UI
- `/openapi.yaml` - direct machine-readable OpenAPI specification

For a local Blockbook public server, open the Swagger UI at the matching coin
port, for example:

- `http://localhost:9130/api-docs/` - Bitcoin
- `http://localhost:9116/api-docs/` - Ethereum

The Swagger UI is served from local pinned assets, does not use the external
Swagger validator, and has "Try it out" disabled so the docs page cannot submit
requests such as transaction broadcasts. Use the OpenAPI file with Swagger UI,
Swagger Editor, Redocly, or any OpenAPI client generator to browse REST
endpoints, schemas, examples, and the documented WebSocket request/response
envelope.

For local validation and generated TypeScript smoke tests, use the OpenAPI test
harness:

```sh
contrib/tests/run-openapi-tests.sh
```

The legacy API V1 is kept only for Bitcoin-type compatibility and is not being
extended. New integrations should use API V2.
