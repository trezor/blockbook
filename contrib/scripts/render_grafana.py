#!/usr/bin/env python3
"""Render the Blockbook Grafana dashboard from its three sources.

    configs/grafana/template.json   structure/viz skeleton (no per-panel content)
    configs/grafana/panels.yaml     per-panel content: title, description, queries
    configs/metrics.yaml            metric registry (name/help per stable key)
        |
        v
    configs/grafana/grafana.json    the artifact you import into Grafana (NOT committed)

template.json carries a semantic `x-panel-key` on each panel and a semantic `x-query-key`
on each target (alongside Grafana's own `id`/`refId`). panels.yaml is keyed by `x-panel-key`,
and its `queries:` are keyed by `x-query-key`. For each panel this overlays its title/description
and, per query, its `promql`/`legend` (-> the target's expr/legendFormat). Inside those strings,
{{name:<key>}} and {{help:<key>}} are expanded from configs/metrics.yaml, so a metric's prometheus
name or help is single-sourced and a rename propagates everywhere. The x-keys are stripped from the
rendered grafana.json, which stays pure Grafana.

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


def fail(msg):
    sys.exit("error: " + msg)


def load_registry():
    cfg = yaml.safe_load(open(METRICS))
    return {key: {"name": d["name"], "help": d.get("help", "")} for key, d in cfg["metrics"].items()}


def load_panels():
    # keyed by the semantic x-panel-key in template.json
    return yaml.safe_load(open(PANELS))["panels"]


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


def render():
    reg = load_registry()
    panels = load_panels()
    dash = json.load(open(TEMPLATE))

    missing = set()      # unknown metric keys referenced by a placeholder
    problems = []        # structural mismatches
    used_keys = set()

    for panel in iter_panels(dash["panels"]):
        pid = panel.get("id")
        pkey = panel.get("x-panel-key")
        # purity: the template must carry no blockbook content -- it lives in panels.yaml
        for f in ("title", "description"):
            if f in panel:
                problems.append("template panel %s must not contain %r (it belongs in panels.yaml)" % (pkey or pid, f))
        for t in panel.get("targets", []):
            for f in ("expr", "legendFormat"):
                if f in t:
                    problems.append("template panel %s target %s must not contain %r (it belongs in panels.yaml)"
                                    % (pkey or pid, t.get("x-query-key") or t.get("refId"), f))
        if pkey is None:
            problems.append("template panel id %s has no x-panel-key" % pid)
            continue
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
    for panel in iter_panels(dash["panels"]):
        panel.pop("x-panel-key", None)
        for t in panel.get("targets", []):
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
