#!/usr/bin/env python3

import argparse
import os
from pathlib import Path

from runner import ValidationError, fail, load_coin_context_from_repo


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="List selectable or dev-buildable coins from BB_RUNNER_* repository variables."
    )
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument(
        "--all",
        action="store_true",
        help="print all selectable coins (runner-mapped coins with existing configs)",
    )
    mode.add_argument(
        "--dev",
        action="store_true",
        help="print dev-buildable coins (selectable coins not mapped to production_builder)",
    )
    parser.add_argument(
        "--repo",
        default="trezor/blockbook",
        help="repository to query when VARS_JSON is not set (default: trezor/blockbook)",
    )
    parser.add_argument(
        "--format",
        choices=("csv", "lines"),
        default="csv",
        help="output format (default: csv)",
    )
    return parser.parse_args()


def main() -> None:
    args = parse_args()
    workspace = Path(os.environ.get("GITHUB_WORKSPACE", ".")).resolve()

    try:
        context = load_coin_context_from_repo(
            workspace,
            args.repo,
            os.environ.get("VARS_JSON"),
            include_deployability=False,
        )
    except ValidationError as exc:
        fail(str(exc))

    coins = context.all_coins if args.all else context.dev_buildable_coins
    if args.format == "lines":
        for coin in coins:
            print(coin)
    else:
        print(",".join(coins))


if __name__ == "__main__":
    main()
