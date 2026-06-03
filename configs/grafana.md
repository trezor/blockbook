# Metrics & Grafana — single source of truth

All of Blockbook's prometheus metrics are declared once, in
[`configs/metrics.yaml`](metrics.yaml). Both the Go binary and the Grafana
dashboard are derived from that file, so a metric's name, help, labels or
buckets never have to be kept in sync by hand.

```
                       configs/metrics.yaml
                       (name, type, help, labels, buckets)
                          │                       │
        go:embed          │                       │  render_grafana.py
   (configs.MetricsYAML)  │                       │
                          ▼                       ▼
   common/GetMetrics  ── builds & registers   configs/grafana_template.json
   the prometheus collectors                  (PromQL exprs use {{name:<key>}})
   into the typed Metrics struct                      │
                                                      ▼
                                              configs/grafana.json
                                              (import this into Grafana)
```

Each metric is keyed by a **stable logical id** (e.g. `rpc_latency`). The Go
struct field binds to it via a `metric:"rpc_latency"` tag, and the dashboard
template references it as `{{name:rpc_latency}}`. The prometheus `name:` can
therefore be renamed in one place and both sides follow — the key never changes.

## Common tasks

**Add a metric**

1. Add an entry under `metrics:` in `configs/metrics.yaml` (pick a new stable key).
2. Add a field to the `Metrics` struct in `common/metrics.go` with a matching
   `metric:"<key>"` tag and the corresponding collector type.
3. Use it at the call sites. `go test -tags unittest ./common/...` checks the
   struct and the YAML stay in 1:1 correspondence.
4. Add a panel to `configs/grafana_template.json` referencing `{{name:<key>}}`,
   then re-render (below).

**Rename a metric**: change only its `name:` in `configs/metrics.yaml`, then
re-render. The stable key, the struct tag and the template stay untouched.

**Edit the dashboard**: edit `configs/grafana_template.json` (the source), then
re-render. The Grafana UI is for preview only — do not treat an export as source.

## Commands

```bash
# render configs/grafana.json from the template + metrics.yaml
python3 contrib/scripts/render_grafana.py

# CI / pre-commit: fail if configs/grafana.json is stale
python3 contrib/scripts/render_grafana.py --check
```

A panel's `description` can be made to track a metric's help text by writing
`{{help:<key>}}` into it; the renderer expands it. This is opt-in — most panels
keep their own hand-written descriptions.

## Re-importing a dashboard built in the Grafana UI

If you must prototype in the Grafana UI, export the JSON and convert it back into
the template, then re-render:

```bash
python3 contrib/scripts/templatize_grafana.py path/to/export.json
python3 contrib/scripts/render_grafana.py
# optional: confirm the round-trip reproduced your export exactly
python3 contrib/scripts/verify_grafana_render.py path/to/export.json
```
