#!/usr/bin/env python3
"""Render the Grafana dashboard from the template and the metric registry.

Reads configs/grafana_template.json and configs/metrics.yaml and expands the
placeholders into configs/grafana.json (the artifact you import into Grafana):

    {{name:<key>}}   -> the metric's prometheus name   (metrics.yaml: metrics.<key>.name)
    {{help:<key>}}   -> the metric's help text          (metrics.yaml: metrics.<key>.help)

Because the template references metrics by their stable <key>, renaming a metric's
`name:` (or editing its `help:`) in configs/metrics.yaml propagates to the dashboard
here, keeping Blockbook and Grafana in sync from one source.

    python3 contrib/scripts/render_grafana.py [--check]

        --check  render in memory and fail if configs/grafana.json is stale,
                 without writing it (for CI / pre-commit drift detection).
"""
import json
import os
import re
import sys

import yaml

REPO = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
METRICS = os.path.join(REPO, "configs", "metrics.yaml")
TEMPLATE = os.path.join(REPO, "configs", "grafana_template.json")
OUTPUT = os.path.join(REPO, "configs", "grafana.json")

PLACEHOLDER = re.compile(r"\{\{\s*(name|help):([a-z0-9_]+)\s*\}\}")


def load_registry():
    cfg = yaml.safe_load(open(METRICS))
    reg = {}
    for key, d in cfg["metrics"].items():
        reg[key] = {"name": d["name"], "help": d["help"]}
    return reg


def expand(value, reg, missing):
    def repl(m):
        field, key = m.group(1), m.group(2)
        if key not in reg:
            missing.add(key)
            return m.group(0)
        return reg[key][field]

    return PLACEHOLDER.sub(repl, value)


def walk(node, reg, missing):
    if isinstance(node, dict):
        return {k: walk(v, reg, missing) for k, v in node.items()}
    if isinstance(node, list):
        return [walk(v, reg, missing) for v in node]
    if isinstance(node, str):
        return expand(node, reg, missing)
    return node


def main():
    check = "--check" in sys.argv[1:]
    reg = load_registry()
    template = json.load(open(TEMPLATE))

    missing = set()
    rendered = walk(template, reg, missing)
    if missing:
        sys.exit(f"error: template references unknown metric key(s): {sorted(missing)}")

    text = json.dumps(rendered, indent=2, ensure_ascii=False) + "\n"

    if check:
        current = open(OUTPUT).read() if os.path.exists(OUTPUT) else ""
        if current != text:
            sys.exit(
                "error: configs/grafana.json is out of date.\n"
                "       run: python3 contrib/scripts/render_grafana.py"
            )
        print("configs/grafana.json is up to date")
        return

    with open(OUTPUT, "w") as f:
        f.write(text)
    print(f"rendered {OUTPUT} from template + {len(reg)} metric definitions")


if __name__ == "__main__":
    main()
