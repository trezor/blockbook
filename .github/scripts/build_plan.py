#!/usr/bin/env python3

import json
import os
from pathlib import Path

from runner import (
    PRODUCTION_RUNNER,
    ValidationError,
    build_runner_labels,
    fail,
    load_coin_context,
    log,
    parse_json_object,
    resolve_build_selection,
)


def main() -> None:
    workspace = Path(os.environ.get("GITHUB_WORKSPACE", ".")).resolve()
    try:
        vars_map = parse_json_object(os.environ.get("VARS_JSON", "{}"), "VARS_JSON")
    except ValidationError as exc:
        fail(str(exc))
    coins_input = os.environ.get("COINS_INPUT", "")
    build_env = os.environ.get("BUILD_ENV", "dev").strip().lower()

    try:
        context = load_coin_context(workspace, vars_map)
        selection = resolve_build_selection(context, coins_input, build_env)
    except ValidationError as exc:
        fail(str(exc))

    grouped_by_runner = {}
    for coin in selection.coins:
        configured_runner = context.runner_map[coin]
        runner = PRODUCTION_RUNNER if build_env == "prod" else configured_runner
        grouped_by_runner.setdefault(runner, []).append(coin)

    runner_matrix = []
    for runner in sorted(grouped_by_runner):
        coins = grouped_by_runner[runner]
        runner_matrix.append(
            {
                "runner": runner,
                "coins": coins,
                "coins_csv": ",".join(coins),
                "labels_json": json.dumps(build_runner_labels(runner, build_env), separators=(",", ":")),
            }
        )

    output_file = os.environ.get("GITHUB_OUTPUT")
    if not output_file:
        fail("GITHUB_OUTPUT is not set")

    with open(output_file, "a", encoding="utf-8") as out:
        out.write(f"runner_matrix={json.dumps(runner_matrix, separators=(',', ':'))}\n")
        out.write(f"coins_csv={','.join(selection.coins)}\n")

    log(f"Build env: {build_env}")
    if selection.skipped_prod_only and selection.requested_all:
        log("Skipped prod-only coins for env=dev: " + ", ".join(selection.skipped_prod_only))
    log("Selected coins: " + ", ".join(selection.coins))
    for item in runner_matrix:
        log(
            f"Runner {item['runner']} labels={item['labels_json']}: "
            + ", ".join(item["coins"])
        )


if __name__ == "__main__":
    main()
