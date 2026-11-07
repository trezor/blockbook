# Metrics & Grafana — single source of truth

Blockbook's prometheus metrics and its Grafana dashboard are generated from a few
source files, so metric names/help and panel queries/descriptions are never hand-synced.

```mermaid
flowchart TD
    M["configs/metrics.yaml<br/>name · type · help · labels · buckets"]
    T["configs/grafana/template.json<br/>viz skeleton + x-panel-key / x-query-key"]
    P["configs/grafana/panels.yaml<br/>per x-panel-key: title · description · queries · width/height"]
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
| `template.json` | dashboard **skeleton** — rows, panel type, `fieldConfig`, `options`, plus a semantic `x-panel-key` per panel and `x-query-key` per target (the join keys). No titles/descriptions/exprs/legends, no `gridPos`, no `datasource`. | yes |
| `panels.yaml` | per-panel **content**, keyed by `x-panel-key` (e.g. `rpc.request_rate`): `title`, `description`, `queries` keyed by `x-query-key` (each with `promql` + `legend`), and optional `width`/`height` (default `8`×`8`; rows fill the row) | yes |
| `grafana.json` | the rendered dashboard you import into Grafana | **no** (git-ignored) |

`render_grafana.py` packs panels into Grafana's 24-column grid from template order and each panel's
`width`/`height` (panels.yaml), so the committed template carries no brittle `x/y` positions; it also
injects the single Prometheus `datasource` onto every panel and target, so the template repeats none.
It joins each `panels.yaml` entry to its template panel by `x-panel-key`, and each `queries:` entry to
a template target by `x-query-key` (Grafana's own `id`/`refId` stay in the template; the x-keys are
stripped from the rendered `grafana.json`). Inside `promql` / `description`, `{{name:<key>}}` /
`{{help:<key>}}` expand from `../metrics.yaml`, so a metric's name lives in one place and a rename
propagates to the Go binary and every panel.

Use stable, descriptive keys: `x-panel-key` should look like `<section>.<subject>[_stat]`
(for example `rpc.request_duration_p95`), and `x-query-key` should name the plotted series
(`requests`, `errors`, `p95`, `total`, `threshold`). Rename titles freely, but keep these keys
stable once other files refer to them.

## Legend convention

The dashboard has a single-select `$coin` dropdown and every panel filters on it, so a legend
never repeats `{{coin}}` -- it says which **replica** a series comes from instead (issue #1717):

| series is | legend shape | example |
|---|---|---|
| per replica (raw series, or aggregated `by (instance, ...)`) | `{{instance}} - <labels> <unit>` | `{{instance}} - {{method}} p95`, `{{instance}} - {{mode}}/s` |
| summed across replicas on purpose | `all replicas - <labels> <unit>` | `all replicas - {{path}}/min` |
| constant reference line (`vector(...)`) | plain text | `stale cutoff (900s = 15 reload periods)` |
| table (`__auto`) or an all-coins panel with no `coin="$coin"` filter | Grafana default / `{{coin}} - {{instance}}` | `general.synchronized` |

Aggregate `by (instance, ...)` when the panel exists to point at a misbehaving replica (errors,
retries, latency, send-path routing, per-replica caches); keep the fleet sum where the panel measures
load or an external dependency and per-replica lines would only multiply. `--check` rejects a legend
that repeats `{{coin}}`, lacks a replica scope, or claims a scope the query does not have
(`{{instance}}` on a query that sums it away, `all replicas` on a per-instance query).

## Render

```bash
python3 contrib/scripts/render_grafana.py          # write configs/grafana/grafana.json
python3 contrib/scripts/render_grafana.py --check  # validate alignment only, no write (CI)
```

`--check` fails on an unknown metric key, an invalid `width`/`height`, a `gridPos` or `datasource`
that leaked into `template.json`, a template ↔ `panels.yaml` `x-panel-key` or `x-query-key` mismatch,
a leftover placeholder, a legend off the convention above, or any per-panel
title/description/expr/legend that leaked into `template.json`.
It also re-checks the **rendered** dashboard for the invariants Grafana enforces only at import time
(unique panel ids, every panel inside the 24-column grid, no overlaps, a datasource on every panel and
target, every `${input}` declared) so a structural defect fails the render here, not silently in Grafana.

> How to add or rename a metric or panel: see the **Metrics** section in `AGENTS.md`.
> The Grafana UI is preview-only — `template.json` + `panels.yaml` are the source.
