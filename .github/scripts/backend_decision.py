#!/usr/bin/env python3

from __future__ import annotations

import os
import shlex
import sys
from pathlib import Path

import backend_policy
from coin_rpc import CoinRPCError, load_config, resolve_build_env


def format_shell(decision: dict, build_env: str) -> str:
    pairs = {
        "BACKEND_SHOULD_BUILD": "1" if decision["should_build_backend"] else "0",
        "BACKEND_REASON": decision["reason"],
        "BACKEND_RPC_ENV": decision["rpc_env"],
        "BACKEND_RPC_HOST": decision["rpc_host"],
        "BACKEND_COIN_ALIAS": decision["coin_alias"],
        "BACKEND_BUILD_ENV": build_env,
    }
    return "\n".join(f"{key}={shlex.quote(str(value))}" for key, value in pairs.items())


def main(argv: list[str] | None = None) -> None:
    args = list(sys.argv[1:] if argv is None else argv)
    if len(args) != 1:
        raise CoinRPCError(f"usage: {Path(sys.argv[0]).name} <coin>")
    coin = args[0]
    config_path = Path("configs") / "coins" / f"{coin}.json"
    if not config_path.is_file():
        raise CoinRPCError(f"missing coin config {config_path}")
    build_env = resolve_build_env()
    decision = backend_policy.compute_backend_decision(
        coin=coin,
        config=load_config(config_path),
        build_env=build_env,
        always_build_backend=False,
    )
    print(format_shell(decision, build_env))


if __name__ == "__main__":
    try:
        main()
    except CoinRPCError as exc:
        print(str(exc), file=sys.stderr)
        raise SystemExit(1)
