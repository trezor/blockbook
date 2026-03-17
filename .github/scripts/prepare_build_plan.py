#!/usr/bin/env python3

import json
import os
from pathlib import Path

from plan_common import fail, load_runner_map, parse_requested_coins


def main() -> None:
    workspace = Path(os.environ.get("GITHUB_WORKSPACE", ".")).resolve()
    vars_map = json.loads(os.environ.get("VARS_JSON", "{}"))
    coins_input = os.environ.get("COINS_INPUT", "")

    runner_map = load_runner_map(vars_map)
    if not runner_map:
        fail("no BB_RUNNER_* variables found")

    requested = parse_requested_coins(coins_input, runner_map)
    configs_dir = workspace / "configs" / "coins"

    grouped_by_runner = {}
    for coin in requested:
        if coin not in runner_map:
            fail(f"missing BB_RUNNER_{coin}")

        coin_cfg_path = configs_dir / f"{coin}.json"
        if not coin_cfg_path.exists():
            fail(f"unknown coin '{coin}' (missing {coin_cfg_path})")

        runner = runner_map[coin]
        grouped_by_runner.setdefault(runner, []).append(coin)

    runner_matrix = []
    for runner in sorted(grouped_by_runner):
        coins = grouped_by_runner[runner]
        runner_matrix.append(
            {
                "runner": runner,
                "coins": coins,
                "coins_csv": ",".join(coins),
            }
        )

    output_file = os.environ.get("GITHUB_OUTPUT")
    if not output_file:
        fail("GITHUB_OUTPUT is not set")

    with open(output_file, "a", encoding="utf-8") as out:
        out.write(f"runner_matrix={json.dumps(runner_matrix, separators=(',', ':'))}\n")
        out.write(f"coins_csv={','.join(requested)}\n")

    print("Selected coins:", ", ".join(requested))
    for item in runner_matrix:
        print(f"Runner {item['runner']}: {', '.join(item['coins'])}")


if __name__ == "__main__":
    main()
