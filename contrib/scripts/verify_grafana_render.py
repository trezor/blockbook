#!/usr/bin/env python3
"""One-time migration check: prove the template+renderer reproduce the original
dashboard exactly, and that every Blockbook metric name in the template is a
placeholder (so metric names are fully single-sourced in metrics.yaml).

    python3 contrib/scripts/verify_grafana_render.py <original_dashboard.json>
"""
import json
import re
import sys

orig = json.load(open(sys.argv[1]))
rendered = json.load(open("configs/grafana.json"))
template = json.load(open("configs/grafana_template.json"))

problems = []

leaked = []


def walk_exprs(ps):
    for p in ps:
        for t in p.get("targets", []):
            e = t.get("expr", "") or ""
            for tok in re.findall(r"blockbook_[a-z0-9_]+", e):
                leaked.append((p.get("title"), tok))
        walk_exprs(p.get("panels", []))


walk_exprs(template.get("panels", []))
if leaked:
    problems.append(f"A. {len(leaked)} literal blockbook_ token(s) NOT placeholdered: {leaked[:5]}")
else:
    print("A. OK  - template exprs contain no literal blockbook_ names (fully single-sourced)")


def diff(a, b, path=""):
    if type(a) is not type(b):
        return [f"{path}: type {type(a).__name__} vs {type(b).__name__}"]
    if isinstance(a, dict):
        out = []
        for k in set(a) | set(b):
            if k not in a:
                out.append(f"{path}.{k}: missing in rendered")
            elif k not in b:
                out.append(f"{path}.{k}: missing in original")
            else:
                out += diff(a[k], b[k], f"{path}.{k}")
        return out
    if isinstance(a, list):
        if len(a) != len(b):
            return [f"{path}: len {len(a)} vs {len(b)}"]
        out = []
        for i, (x, y) in enumerate(zip(a, b)):
            out += diff(x, y, f"{path}[{i}]")
        return out
    if a != b:
        return [f"{path}: {a!r} != {b!r}"]
    return []


if rendered == orig:
    print("B. OK  - rendered grafana.json is semantically identical to the original dashboard")
else:
    d = diff(rendered, orig)
    problems.append(f"B. rendered != original, {len(d)} diffs:\n   " + "\n   ".join(d[:20]))

if problems:
    print("\nPROBLEMS:")
    for p in problems:
        print(" -", p)
    sys.exit(1)
print("\nALL CHECKS PASSED")
