#!/usr/bin/env python3

from __future__ import annotations

import os
from typing import Mapping

from coin_rpc import get_coin_alias, rpc_hostname, rpc_url_env_name

BACKEND_MODE_AUTO = "auto"
BACKEND_MODE_ALWAYS = "always"
BACKEND_MODE_NEVER = "never"


def should_build_backend(
    *,
    backend_mode: str,
    rpc_url: str,
) -> tuple[bool, str]:
    if backend_mode == BACKEND_MODE_NEVER:
        return False, "backend-mode-never"
    if backend_mode == BACKEND_MODE_ALWAYS:
        return True, "backend-mode-always"
    if not rpc_url:
        return True, "rpc-url-env-missing-or-empty"
    rpc_host = rpc_hostname(rpc_url)
    if not rpc_host:
        return False, "rpc-host-missing"
    if rpc_host in {"localhost", "127.0.0.1", "::1"}:
        return True, f"rpc-host-is-local-{rpc_host}"
    return False, f"rpc-host-is-remote-{rpc_host}"


def compute_backend_decision(
    *,
    coin: str,
    config: dict,
    build_env: str,
    backend_mode: str,
    env: Mapping[str, str] | None = None,
) -> dict:
    if env is None:
        env = os.environ
    coin_alias = get_coin_alias(config, coin)
    rpc_env = rpc_url_env_name(coin_alias, build_env)
    rpc_url = env.get(rpc_env, "").strip()
    should_build, reason = should_build_backend(
        backend_mode=backend_mode,
        rpc_url=rpc_url,
    )
    return {
        "coin_alias": coin_alias,
        "rpc_env": rpc_env,
        "rpc_host": rpc_hostname(rpc_url),
        "should_build_backend": should_build,
        "reason": reason,
    }
