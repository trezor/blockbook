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
)


def matchable_name(coin: str) -> str:
    marker = "_testnet"
    idx = coin.find(marker)
    if idx != -1:
        return coin[:idx] + "=test"
    return coin + "=main"


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
    escaped = [re.escape(name) for name in unique_names]
    e2e_regex = "TestIntegration/(" + "|".join(escaped) + ")/api"

    output_file = os.environ.get("GITHUB_OUTPUT")
    if not output_file:
        fail("GITHUB_OUTPUT is not set")

    with open(output_file, "a", encoding="utf-8") as out:
        out.write(f"runner_matrix={json.dumps(runner_matrix, separators=(',', ':'))}\n")
        out.write(f"e2e_regex={e2e_regex}\n")
        out.write(f"coins_csv={','.join(requested)}\n")
        out.write(f"test_coins_csv={','.join(unique_test_coins)}\n")

    log("Selected coins: " + ", ".join(requested))
    log("E2E regex: " + e2e_regex)


if __name__ == "__main__":
    main()
