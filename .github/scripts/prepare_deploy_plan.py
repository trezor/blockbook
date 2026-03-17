#!/usr/bin/env python3

import json
import os
import re
from pathlib import Path

from plan_common import fail, load_runner_map, parse_requested_coins


def matchable_name(coin: str) -> str:
    marker = "_testnet"
    idx = coin.find(marker)
    if idx != -1:
        return coin[:idx] + "=test"
    return coin + "=main"


def load_test_coin_name(config_path: Path) -> str:
    try:
        config = json.loads(config_path.read_text(encoding="utf-8"))
    except Exception as exc:
        fail(f"cannot read {config_path}: {exc}")

    coin_cfg = config.get("coin")
    if not isinstance(coin_cfg, dict):
        fail(f"invalid config {config_path}: missing coin section")

    test_name = coin_cfg.get("test_name")
    if test_name is None:
        return config_path.stem
    if not isinstance(test_name, str):
        fail(f"invalid config {config_path}: coin.test_name must be a string")

    test_name = test_name.strip()
    if not test_name:
        fail(f"invalid config {config_path}: coin.test_name must not be empty")
    return test_name
def main() -> None:
    workspace = Path(os.environ.get("GITHUB_WORKSPACE", ".")).resolve()
    vars_map = json.loads(os.environ.get("VARS_JSON", "{}"))
    coins_input = os.environ.get("COINS_INPUT", "")

    runner_map = load_runner_map(vars_map)
    if not runner_map:
        fail("no BB_RUNNER_* variables found")

    requested = parse_requested_coins(coins_input, runner_map)

    configs_dir = workspace / "configs" / "coins"
    tests_path = workspace / "tests" / "tests.json"

    runner_matrix = []
    e2e_names = []
    test_coins = []

    try:
        tests_cfg = json.loads(tests_path.read_text(encoding="utf-8"))
    except Exception as exc:
        fail(f"cannot read {tests_path}: {exc}")

    for coin in requested:
        if coin not in runner_map:
            fail(f"missing BB_RUNNER_{coin}")

        coin_cfg_path = configs_dir / f"{coin}.json"
        if not coin_cfg_path.exists():
            fail(f"unknown coin '{coin}' (missing {coin_cfg_path})")

        lookup_coin = load_test_coin_name(coin_cfg_path)
        test_cfg = tests_cfg.get(lookup_coin)
        if not isinstance(test_cfg, dict) or "connectivity" not in test_cfg:
            fail(
                f"coin '{coin}' maps to test coin '{lookup_coin}' "
                "which has no connectivity tests in tests/tests.json"
            )

        runner_matrix.append({"coin": coin, "runner": runner_map[coin]})
        e2e_names.append(matchable_name(lookup_coin))
        test_coins.append(lookup_coin)

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

    print("Selected coins:", ", ".join(requested))
    print("E2E regex:", e2e_regex)


if __name__ == "__main__":
    main()
