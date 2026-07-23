#!/usr/bin/env python3
"""Render the Blockbook Grafana dashboard from its three sources.

    configs/grafana/template.json   structure/viz skeleton (no per-panel content)
    configs/grafana/panels.yaml     per-panel content: title, description, queries
    configs/metrics.yaml            metric registry (name/help per stable key)
        |
        v
    configs/grafana/grafana.json    the artifact you import into Grafana (NOT committed)

template.json carries a semantic `x-panel-key` on each panel and a semantic `x-query-key`
on each target (alongside Grafana's own `id`/`refId`). It holds no `gridPos` and no
`datasource`: panel x/y are packed from panel order, w/h come from panels.yaml (optional
`width`/`height`, defaulting to a full-width row or an 8x8 cell), and the single Prometheus
datasource is injected at render time. panels.yaml is keyed by `x-panel-key`, and its
`queries:` are keyed by `x-query-key`. For each panel this overlays its title/description
and, per query, its `promql`/`legend` (-> the target's expr/legendFormat). Inside those
strings, {{name:<key>}} and {{help:<key>}} are expanded from configs/metrics.yaml, so a
metric's prometheus name or help is single-sourced and a rename propagates everywhere. The
x-keys are stripped from the rendered grafana.json, which stays pure Grafana.

    python3 contrib/scripts/render_grafana.py [--check]

        --check  validate + render in memory and exit non-zero on any problem
                 (unknown metric key, panel/query key mismatch, residual placeholder)
                 WITHOUT writing the file -- for CI / pre-commit.
"""
import json
import os
import re
import sys

import yaml

REPO = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
METRICS = os.path.join(REPO, "configs", "metrics.yaml")
TEMPLATE = os.path.join(REPO, "configs", "grafana", "template.json")
PANELS = os.path.join(REPO, "configs", "grafana", "panels.yaml")
OUTPUT = os.path.join(REPO, "configs", "grafana", "grafana.json")

PLACEHOLDER = re.compile(r"\{\{\s*(name|help):([a-z0-9_]+)\s*\}\}")
GRID_COLUMNS = 24
DEFAULT_PANEL_WIDTH = 8
DEFAULT_PANEL_HEIGHT = 8
# Single datasource for the whole dashboard -- injected at render time so the template
# carries no per-panel/per-target datasource boilerplate.
DATASOURCE = {"type": "prometheus", "uid": "${DS_PROMETHEUS}"}


def fail(msg):
    sys.exit("error: " + msg)


def load_registry():
    with open(METRICS) as f:
        cfg = yaml.safe_load(f)
    return {key: {"name": d["name"], "help": d.get("help", "")} for key, d in cfg["metrics"].items()}


def load_panels():
    # keyed by the semantic x-panel-key in template.json
    with open(PANELS) as f:
        return yaml.safe_load(f)["panels"]


def expand(value, reg, missing):
    def repl(m):
        field, key = m.group(1), m.group(2)
        if key not in reg:
            missing.add(key)
            return m.group(0)
        return reg[key][field]

    return PLACEHOLDER.sub(repl, value)


def iter_panels(panels):
    for p in panels:
        yield p
        yield from iter_panels(p.get("panels", []))


def panel_label(panel):
    return panel.get("x-panel-key") or panel.get("id") or "<unknown>"


def is_positive_int(value):
    return isinstance(value, int) and not isinstance(value, bool) and value > 0


def layout_items_from_panels(panels, content, columns=GRID_COLUMNS):
    """Return side-effect-free layout hints for a Grafana panel list.

    Per-panel `width`/`height` come from the matching panels.yaml entry (looked up by
    x-panel-key); panels omit them and fall back to a full-width row or a DEFAULT_PANEL
    cell. template.json carries no gridPos at all -- x/y are packed during rendering.
    """
    items = []
    problems = []
    for panel in panels:
        label = panel_label(panel)
        ptype = panel.get("type")
        is_row = ptype == "row"

        if "gridPos" in panel:
            problems.append("template panel %s must not carry gridPos; set width/height in panels.yaml "
                            "(x/y are computed by render_grafana.py)" % label)

        entry = content.get(panel.get("x-panel-key")) or {}
        width = entry.get("width", columns if is_row else DEFAULT_PANEL_WIDTH)
        height = entry.get("height", 1 if is_row else DEFAULT_PANEL_HEIGHT)
        if not is_positive_int(width):
            problems.append("panel %s 'width' must be a positive integer in panels.yaml" % label)
        if not is_positive_int(height):
            problems.append("panel %s 'height' must be a positive integer in panels.yaml" % label)
        if is_positive_int(width) and width > columns:
            problems.append("panel %s width=%d exceeds Grafana's %d-column grid" % (label, width, columns))

        items.append({"key": label, "type": ptype, "w": width, "h": height})
    return items, problems


def compute_grid_positions(items, columns=GRID_COLUMNS):
    """Pack layout items left-to-right into Grafana gridPos dictionaries."""
    positions = []
    problems = []
    x = 0
    y = 0
    row_height = 0

    for item in items:
        key = item["key"]
        width = item["w"]
        height = item["h"]
        if not is_positive_int(width) or not is_positive_int(height):
            problems.append("panel %s has invalid layout dimensions" % key)
            continue
        if width > columns:
            problems.append("panel %s width %d exceeds Grafana's %d-column grid" % (key, width, columns))
            continue

        if item["type"] == "row":
            if x:
                y += row_height
                x = 0
                row_height = 0
            positions.append({"h": height, "w": width, "x": 0, "y": y})
            y += height
            continue

        if x and x + width > columns:
            y += row_height
            x = 0
            row_height = 0

        positions.append({"h": height, "w": width, "x": x, "y": y})
        x += width
        row_height = max(row_height, height)
        if x == columns:
            y += row_height
            x = 0
            row_height = 0

    return positions, problems


def apply_computed_grid_positions(panels, content, columns=GRID_COLUMNS):
    """Apply computed gridPos to a panel list after pure validation/packing."""
    items, problems = layout_items_from_panels(panels, content, columns)
    if problems:
        return problems

    positions, problems = compute_grid_positions(items, columns)
    if problems:
        return problems
    if len(positions) != len(panels):
        return ["computed %d grid positions for %d template panels" % (len(positions), len(panels))]

    for panel, grid_pos in zip(panels, positions):
        panel["gridPos"] = grid_pos
        problems.extend(apply_computed_grid_positions(panel.get("panels", []), content, columns))
    return problems


def render():
    reg = load_registry()
    panels = load_panels()
    with open(TEMPLATE) as f:
        dash = json.load(f)

    missing = set()      # unknown metric keys referenced by a placeholder
    problems = []        # structural mismatches
    used_keys = set()
    seen_panel_keys = set()

    problems.extend(apply_computed_grid_positions(dash["panels"], panels))

    for panel in iter_panels(dash["panels"]):
        pid = panel.get("id")
        pkey = panel.get("x-panel-key")
        # purity: the template must carry no blockbook content -- it lives in panels.yaml
        for f in ("title", "description"):
            if f in panel:
                problems.append("template panel %s must not contain %r (it belongs in panels.yaml)" % (pkey or pid, f))
        # datasource is injected at render time -- it must not be pinned in the template
        if "datasource" in panel:
            problems.append("template panel %s must not contain 'datasource' (it is injected at render time)" % (pkey or pid))
        for t in panel.get("targets", []):
            for f in ("expr", "legendFormat", "datasource"):
                if f in t:
                    problems.append("template panel %s target %s must not contain %r (it belongs in panels.yaml or is injected)"
                                    % (pkey or pid, t.get("x-query-key") or t.get("refId"), f))
        if pkey is None:
            problems.append("template panel id %s has no x-panel-key" % pid)
            continue
        if pkey in seen_panel_keys:
            problems.append("template panel key %r is duplicated" % pkey)
            continue
        seen_panel_keys.add(pkey)
        entry = panels.get(pkey)
        if entry is None:
            problems.append("template panel %r (id %s) has no entry in panels.yaml" % (pkey, pid))
            continue
        used_keys.add(pkey)

        if "title" in entry:
            panel["title"] = expand(entry["title"], reg, missing)
        if "description" in entry:
            panel["description"] = expand(entry["description"], reg, missing)

        queries = entry.get("queries", {})
        tmpl_qkeys = set()
        for t in panel.get("targets", []):
            qkey = t.get("x-query-key")
            if qkey is None:
                problems.append("panel %r target refId %r has no x-query-key" % (pkey, t.get("refId")))
                continue
            if qkey in tmpl_qkeys:
                problems.append("panel %r query key %r is duplicated in template targets" % (pkey, qkey))
                continue
            tmpl_qkeys.add(qkey)
            qc = queries.get(qkey)
            if qc is None:
                problems.append("panel %r target %r (refId %s) has no query in panels.yaml"
                                % (pkey, qkey, t.get("refId")))
                continue
            promql = qc.get("promql")
            if promql is None:
                problems.append("panel %r query %r (refId %s) has no 'promql' in panels.yaml"
                                % (pkey, qkey, t.get("refId")))
                continue
            t["expr"] = expand(promql, reg, missing)
            if "legend" in qc:
                t["legendFormat"] = expand(qc["legend"], reg, missing)
        extra = set(queries) - tmpl_qkeys
        if extra:
            problems.append("panel %r panels.yaml has query key(s) %s absent from the template" % (pkey, sorted(extra)))

    orphans = set(panels) - used_keys
    if orphans:
        problems.append("panels.yaml has entries for x-panel-keys not in template.json: %s" % sorted(orphans))
    if missing:
        problems.append("unknown metric key(s) referenced: %s" % sorted(missing))

    # x-panel-key / x-query-key are render-time join keys, not Grafana fields -- strip them so
    # grafana.json is pure Grafana. (After validation, which needs them; before serialization.)
    # The single datasource is injected here onto every non-row panel and every target.
    for panel in iter_panels(dash["panels"]):
        panel.pop("x-panel-key", None)
        if panel.get("type") != "row":
            panel["datasource"] = dict(DATASOURCE)
        for t in panel.get("targets", []):
            t["datasource"] = dict(DATASOURCE)
            t.pop("x-query-key", None)

    # belt-and-suspenders: nothing in our namespace should survive
    residual = PLACEHOLDER.findall(json.dumps(dash))
    if residual:
        problems.append("unresolved {{name|help:...}} placeholder(s) remain: %s" % residual[:5])

    if problems:
        fail("dashboard render failed:\n       - " + "\n       - ".join(problems))

    return dash, reg, panels


def main():
    check = "--check" in sys.argv[1:]
    dash, reg, panels = render()
    text = json.dumps(dash, indent=2, ensure_ascii=False) + "\n"

    if check:
        print("OK: render is consistent (%d panels, %d metrics, no unresolved keys)" % (len(panels), len(reg)))
        return

    with open(OUTPUT, "w") as f:
        f.write(text)
    print("rendered %s" % os.path.relpath(OUTPUT, REPO))
    print("  from template + panels.yaml (%d panels) + metrics.yaml (%d metrics)" % (len(panels), len(reg)))


if __name__ == "__main__":
    main()
