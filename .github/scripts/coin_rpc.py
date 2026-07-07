#!/usr/bin/env python3

from __future__ import annotations

import json
import os
from pathlib import Path
from urllib.parse import urlparse

BUILD_ENV_VAR = "BB_BUILD_ENV"
BUILD_ENV_DEV = "dev"
BUILD_ENV_PROD = "prod"


class CoinRPCError(ValueError):
    pass


def load_config(path: Path) -> dict:
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except Exception as exc:
        raise CoinRPCError(f"cannot read {path}: {exc}") from exc
    if not isinstance(payload, dict):
        raise CoinRPCError(f"invalid config {path}: expected a JSON object")
    return payload


def get_coin_alias(config: dict, coin: str) -> str:
    value = config.get("coin", {}).get("alias", coin)
    if not isinstance(value, str) or not value.strip():
        raise CoinRPCError(f"coin '{coin}' does not define coin.alias")
    return value.strip().lower()


def resolve_build_env(raw: str | None = None) -> str:
    build_env = raw or os.environ.get(BUILD_ENV_VAR, "")
    build_env = build_env.strip().lower()
    if not build_env:
        return BUILD_ENV_DEV
    if build_env in {BUILD_ENV_DEV, BUILD_ENV_PROD}:
        return build_env
    raise CoinRPCError(
        f"invalid {BUILD_ENV_VAR} value '{build_env}', expected 'dev' or 'prod'"
    )


def rpc_url_env_name(alias: str, build_env: str) -> str:
    prefix = "BB_DEV_RPC_URL_HTTP_" if build_env == BUILD_ENV_DEV else "BB_PROD_RPC_URL_HTTP_"
    return f"{prefix}{alias.replace('-', '_')}"


def rpc_hostname(url: str) -> str:
    if not url:
        return ""
    try:
        return urlparse(url).hostname or ""
    except ValueError:
        return ""
