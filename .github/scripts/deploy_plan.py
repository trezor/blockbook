#!/usr/bin/env python3

import json
import os
import re
from pathlib import Path

from runner import (
    ValidationError,
    fail,
    load_coin_context,
    load_test_coin_name,
    log,
    parse_json_object,
    require_coin_config,
    resolve_deploy_selection,
    write_step_summary,
)


def matchable_name(coin: str) -> str:
    marker = "_testnet"
    idx = coin.find(marker)
    if idx != -1:
        # Preserve the network suffix (e.g. "_sepolia", "_nile", "4") so distinct
        # testnets of the same coin get distinct names instead of all collapsing to
        # "<coin>=test". Keeps the mapping injective. Must stay in sync with
        # getMatchableName() in tests/integration.go.
        return coin[:idx] + "=test" + coin[idx + len(marker):]
    return coin + "=main"


def build_connectivity_regex(names) -> str:
    # Anchor each name so e.g. "bitcoin=test" cannot substring-match the
    # "bitcoin=test4" subtest. Safe because matchable_name() is injective, so
    # Go never appends a "#NN" disambiguator that an anchor would exclude.
    if not names:
        # Fail closed: an empty alternation "()" matches the empty string, so
        # `go test -run` would select EVERY connectivity subtest — the opposite
        # of "no coins". Callers must filter to a non-empty set first.
        raise ValueError("build_connectivity_regex requires at least one name")
    escaped = ["^" + re.escape(name) + "$" for name in names]
    return "TestIntegration/(" + "|".join(escaped) + ")/connectivity"


def main() -> None:
    workspace = Path(os.environ.get("GITHUB_WORKSPACE", ".")).resolve()
    try:
        vars_map = parse_json_object(os.environ.get("VARS_JSON", "{}"), "VARS_JSON")
    except ValidationError as exc:
        fail(str(exc))
    coins_input = os.environ.get("COINS_INPUT", "")

    try:
        context = load_coin_context(workspace, vars_map, include_deployability=True)
        requested = resolve_deploy_selection(context, coins_input)
    except ValidationError as exc:
        fail(str(exc))

    runner_matrix = []
    e2e_names = []
    test_coins = []

    try:
        for coin in requested:
            configured_runner = context.runner_map[coin]
            coin_cfg_path = require_coin_config(workspace, coin)
            lookup_coin = load_test_coin_name(coin_cfg_path)
            runner_matrix.append({"coin": coin, "runner": configured_runner})
            e2e_names.append(matchable_name(lookup_coin))
            test_coins.append(lookup_coin)
    except ValidationError as exc:
        fail(str(exc))

    unique_names = sorted(set(e2e_names))
    if not unique_names:
        fail("no coins selected after validation")
    unique_test_coins = sorted(set(test_coins))
    connectivity_regex = build_connectivity_regex(unique_names)

    output_file = os.environ.get("GITHUB_OUTPUT")
    if not output_file:
        fail("GITHUB_OUTPUT is not set")

    with open(output_file, "a", encoding="utf-8") as out:
        out.write(f"runner_matrix={json.dumps(runner_matrix, separators=(',', ':'))}\n")
        out.write(f"connectivity_regex={connectivity_regex}\n")
        out.write(f"coins_csv={','.join(requested)}\n")
        out.write(f"test_coins_csv={','.join(unique_test_coins)}\n")

    log("Selected coins: " + ", ".join(requested))
    log("Connectivity regex: " + connectivity_regex)

    summary_lines = [
        "### Deploy plan",
        "",
        "| Coin | Runner |",
        "| --- | --- |",
    ]
    for item in runner_matrix:
        summary_lines.append(f"| {item['coin']} | {item['runner']} |")
    summary_lines += [
        "",
        "E2E test coins: " + ", ".join(unique_test_coins),
    ]
    write_step_summary("\n".join(summary_lines))


if __name__ == "__main__":
    main()
