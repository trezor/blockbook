#!/usr/bin/env python3
"""Bootstrap / import helper: convert a Grafana dashboard export into the
parameterized template configs/grafana_template.json, using configs/metrics.yaml
as the metric registry.

It rewrites every PromQL `expr` so that Blockbook metric names become
``{{name:<key>}}`` placeholders keyed by the stable logical id from metrics.yaml.
render_grafana.py then expands those placeholders back into configs/grafana.json,
so renaming a metric's `name:` in metrics.yaml updates both Blockbook and the
dashboard from one place.

Panel descriptions are left exactly as authored. To make a panel's description
track a metric's help text, write ``{{help:<key>}}`` into its description in the
template; the renderer expands it. This is opt-in: the same metric drives several
panels with different, hand-written descriptions, so help text is not auto-injected.

Run this only when (re-)importing a dashboard built in the Grafana UI. Day-to-day
edits are made directly in configs/grafana_template.json (the source of truth).

    python3 contrib/scripts/templatize_grafana.py [INPUT.json]
        INPUT.json defaults to grafana-backup/Blockbook_classic.json
"""
import json
import os
import re
import sys

import yaml

REPO = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
INPUT = sys.argv[1] if len(sys.argv) > 1 else os.path.join(REPO, "grafana-backup", "Blockbook_classic.json")
METRICS = os.path.join(REPO, "configs", "metrics.yaml")
OUTPUT = os.path.join(REPO, "configs", "grafana_template.json")

# prometheus-derived suffixes that are NOT part of the metric name itself
HIST_SUFFIXES = ("_bucket", "_count", "_sum")
METRIC_TOKEN = re.compile(r"blockbook_[a-z0-9_]+")


def load_registry():
    cfg = yaml.safe_load(open(METRICS))
    name_to_key = {}
    for key, d in cfg["metrics"].items():
        name_to_key[d["name"]] = key
    return name_to_key


def rewrite_expr(expr, name_to_key, referenced):
    """Replace blockbook_* metric names in expr with {{name:<key>}} placeholders.
    Records the keys referenced into the `referenced` set."""
    def repl(m):
        tok = m.group(0)
        if tok in name_to_key:
            key = name_to_key[tok]
            referenced.add(key)
            return "{{name:%s}}" % key
        for suf in HIST_SUFFIXES:
            base = tok[: -len(suf)]
            if tok.endswith(suf) and base in name_to_key:
                key = name_to_key[base]
                referenced.add(key)
                return "{{name:%s}}%s" % (key, suf)
        # not a Blockbook-managed metric (process_*, node_*, go_*, or unknown)
        return tok

    return METRIC_TOKEN.sub(repl, expr)


def process_panel(panel, name_to_key, stats):
    referenced = set()
    for t in panel.get("targets", []):
        if "expr" in t and t["expr"]:
            new = rewrite_expr(t["expr"], name_to_key, referenced)
            if new != t["expr"]:
                stats["exprs"] += 1
            t["expr"] = new
    if "targets" in panel:
        stats["panels"] += 1
        stats["covered"].update(referenced)
        # Descriptions are author-curated; only flag empty single-metric panels as
        # candidates for an opt-in {{help:<key>}} placeholder. We never auto-write one.
        if len(referenced) == 1 and not panel.get("description"):
            (key,) = tuple(referenced)
            stats["help_candidates"].append((panel.get("title", "?"), key))
    for child in panel.get("panels", []):
        process_panel(child, name_to_key, stats)


def main():
    name_to_key = load_registry()
    dash = json.load(open(INPUT))
    stats = {"panels": 0, "exprs": 0, "covered": set(), "help_candidates": []}
    for panel in dash.get("panels", []):
        process_panel(panel, name_to_key, stats)

    with open(OUTPUT, "w") as f:
        json.dump(dash, f, indent=2, ensure_ascii=False)
        f.write("\n")

    print(f"templatized {INPUT}")
    print(f"  -> {OUTPUT}")
    print(f"  leaf panels:      {stats['panels']}")
    print(f"  exprs rewritten:  {stats['exprs']}")
    print(f"  metrics covered:  {len(stats['covered'])}/{len(name_to_key)}")
    missing = set(name_to_key.values()) - stats["covered"]
    if missing:
        print(f"  NOTE: {len(missing)} metric(s) defined but not shown on any panel: {sorted(missing)}")
    if stats["help_candidates"]:
        print(f"  {len(stats['help_candidates'])} empty single-metric panel(s) could opt into {{{{help:<key>}}}}:")
        for title, key in stats["help_candidates"]:
            print(f"      - {title!r} -> {{{{help:{key}}}}}")


if __name__ == "__main__":
    main()
