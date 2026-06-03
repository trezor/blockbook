# Metrics & Grafana — single source of truth

Blockbook's prometheus metrics and its Grafana dashboard are generated from a few
source files, so metric names/help and panel queries/descriptions are never hand-synced.

```mermaid
flowchart TD
    M["configs/metrics.yaml<br/>name · type · help · labels · buckets"]
    T["configs/grafana/template.json<br/>panel layout + viz skeleton + x-panel-key / x-query-key"]
    P["configs/grafana/panels.yaml<br/>per x-panel-key: title · description · queries"]
    G["common.GetMetrics<br/>builds + registers collectors at startup"]
    R(["contrib/scripts/render_grafana.py"])
    D["configs/grafana/grafana.json<br/>import into Grafana — generated, git-ignored"]
    M -->|go:embed| G
    M --> R
    T --> R
    P --> R
    R --> D
```

## Files

| file | holds | committed |
|---|---|---|
| `../metrics.yaml` | every metric, keyed by a **stable id**: `name`, `type`, `help`, `labels`, `buckets` | yes |
| `template.json` | dashboard **skeleton** — rows, `gridPos`, panel type, `fieldConfig`, datasource, plus a semantic `x-panel-key` per panel and `x-query-key` per target (the join keys). No titles/descriptions/exprs/legends. | yes |
| `panels.yaml` | per-panel **content**, keyed by `x-panel-key` (e.g. `rpc.request_rate`): `title`, `description`, and `queries` keyed by `x-query-key`, each with `promql` + `legend` | yes |
| `grafana.json` | the rendered dashboard you import into Grafana | **no** (git-ignored) |

`render_grafana.py` joins each `panels.yaml` entry to its template panel by `x-panel-key`, and each
`queries:` entry to a template target by `x-query-key` (Grafana's own `id`/`refId` stay in the
template; the x-keys are stripped from the rendered `grafana.json`). Inside `promql` / `description`,
`{{name:<key>}}` / `{{help:<key>}}` expand from `../metrics.yaml`, so a metric's name lives in one
place and a rename propagates to the Go binary and every panel.

## Render

```bash
python3 contrib/scripts/render_grafana.py          # write configs/grafana/grafana.json
python3 contrib/scripts/render_grafana.py --check  # validate alignment only, no write (CI)
```

`--check` fails on an unknown metric key, a template ↔ `panels.yaml` `x-panel-key` or `x-query-key`
mismatch, a leftover placeholder, or any per-panel title/description/expr/legend that leaked into `template.json`.

> How to add or rename a metric or panel: see the **Metrics** section in `AGENTS.md`.
> The Grafana UI is preview-only — `template.json` + `panels.yaml` are the source.
