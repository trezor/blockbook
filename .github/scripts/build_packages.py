#!/usr/bin/env python3

from __future__ import annotations

import argparse
import json
import os
import shutil
import subprocess
import sys
from pathlib import Path
from urllib.parse import urlparse


LOG_PREFIX = "CI/CD Pipeline:"
SCRIPT_NAME = "[build-packages]"
DEFAULT_PACKAGE_ROOT = "/opt/blockbook-builds"
BUILD_ENV_VAR = "BB_BUILD_ENV"
BUILD_ENV_DEV = "dev"
BUILD_ENV_PROD = "prod"


def log(message: str) -> None:
    print(f"{LOG_PREFIX} {SCRIPT_NAME} {message}", file=sys.stderr, flush=True)


def fail(message: str) -> None:
    print(f"{LOG_PREFIX} error: {message}", file=sys.stderr)
    raise SystemExit(1)


def load_config(path: Path) -> dict:
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except Exception as exc:
        fail(f"cannot read {path}: {exc}")
    if not isinstance(payload, dict):
        fail(f"invalid config {path}: expected a JSON object")
    return payload


def get_package_name(config: dict, section: str, coin: str) -> str:
    value = config.get(section, {}).get("package_name", "")
    if not isinstance(value, str) or not value.strip():
        fail(f"coin '{coin}' does not define {section}.package_name")
    return value.strip()


def get_coin_alias(config: dict, coin: str) -> str:
    value = config.get("coin", {}).get("alias", coin)
    if not isinstance(value, str) or not value.strip():
        fail(f"coin '{coin}' does not define coin.alias")
    return value.strip().lower()


def resolve_build_env() -> str:
    build_env = os.environ.get(BUILD_ENV_VAR, "").strip().lower()
    if not build_env:
        return BUILD_ENV_DEV
    if build_env in {BUILD_ENV_DEV, BUILD_ENV_PROD}:
        return build_env
    fail(f"invalid {BUILD_ENV_VAR} value '{build_env}', expected 'dev' or 'prod'")
    return ""


def rpc_url_env_name(alias: str, build_env: str) -> str:
    prefix = "BB_DEV_RPC_URL_HTTP_" if build_env == BUILD_ENV_DEV else "BB_PROD_RPC_URL_HTTP_"
    return f"{prefix}{alias}"


def rpc_hostname(url: str) -> str:
    if not url:
        return ""
    try:
        return urlparse(url).hostname or ""
    except ValueError:
        return ""


def should_build_backend(
    *,
    always_build_backend: bool,
    rpc_url: str,
) -> tuple[bool, str]:
    if always_build_backend:
        return True, "always-build-backend"
    if not rpc_url:
        return True, "rpc-url-env-missing-or-empty"
    rpc_host = rpc_hostname(rpc_url)
    if not rpc_host:
        return False, "rpc-host-missing"
    if rpc_host in {"localhost", "127.0.0.1", "::1"}:
        return True, f"rpc-host-is-local-{rpc_host}"
    return False, f"rpc-host-is-remote-{rpc_host}"


def resolve_branch_or_tag() -> str:
    configured = os.environ.get("BRANCH_OR_TAG", "").strip()
    if configured:
        return configured

    try:
        result = subprocess.run(
            ["git", "branch", "--show-current"],
            check=True,
            capture_output=True,
            text=True,
        )
        current_branch = result.stdout.strip()
    except (FileNotFoundError, subprocess.CalledProcessError):
        current_branch = ""
    if current_branch:
        return current_branch

    try:
        result = subprocess.run(
            ["git", "describe", "--tags", "--exact-match"],
            check=True,
            capture_output=True,
            text=True,
        )
        current_tag = result.stdout.strip()
    except (FileNotFoundError, subprocess.CalledProcessError):
        current_tag = ""
    if current_tag:
        return current_tag

    fail("BRANCH_OR_TAG is not set and the current checkout is neither a branch nor an exact tag")


def latest_package(pattern: str) -> Path:
    matches = sorted(Path("build").glob(pattern), key=lambda p: p.stat().st_mtime, reverse=True)
    if not matches:
        fail(f"built package was not found (pattern build/{pattern})")
    return matches[0]


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--always-build-backend", action="store_true")
    parser.add_argument("coins", nargs="+")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> None:
    raw_args = list(sys.argv[1:] if argv is None else argv)
    if not raw_args:
        fail(f"usage: {Path(sys.argv[0]).name} <coin-alias> [<coin-alias> ...]")
    parsed = parse_args(raw_args)
    args = parsed.coins

    always_build_backend = parsed.always_build_backend
    build_env = resolve_build_env()

    package_root = os.environ.get("BB_PACKAGE_ROOT", "").strip() or DEFAULT_PACKAGE_ROOT
    if not os.path.isabs(package_root):
        fail(f"BB_PACKAGE_ROOT must be an absolute path (got '{package_root}')")
    branch_or_tag = resolve_branch_or_tag()
    branch_or_tag_path = branch_or_tag.replace("/", "-")

    log("requested coins: " + " ".join(args))
    log(f"always_build_backend={int(always_build_backend)}")
    log(f"{BUILD_ENV_VAR}={build_env}")
    log("backend build rule: build unless the selected BB_{DEV|PROD}_RPC_URL_HTTP is non-empty and non-local")
    log(f"branch_or_tag={branch_or_tag} -> path={branch_or_tag_path}")
    log(f"package_root={package_root}")

    coins: list[str] = []
    blockbook_package_names: list[str] = []
    backend_package_names: list[str] = []
    build_backend_flags: list[bool] = []
    make_targets: list[str] = []

    for coin in args:
        config_path = Path("configs") / "coins" / f"{coin}.json"
        if not config_path.is_file():
            fail(f"missing coin config {config_path}")

        config = load_config(config_path)
        blockbook_package_name = get_package_name(config, "blockbook", coin)
        backend_package_name = get_package_name(config, "backend", coin)
        coin_alias = get_coin_alias(config, coin)
        rpc_env = rpc_url_env_name(coin_alias, build_env)
        rpc_url = os.environ.get(rpc_env, "").strip()
        build_backend, reason = should_build_backend(
            always_build_backend=always_build_backend,
            rpc_url=rpc_url,
        )
        host = rpc_hostname(rpc_url)

        coins.append(coin)
        blockbook_package_names.append(blockbook_package_name)
        backend_package_names.append(backend_package_name)
        build_backend_flags.append(build_backend)

        if build_backend:
            target = f"deb-{coin}"
        else:
            target = f"deb-blockbook-{coin}"
        log(
            f"validated {coin}: alias={coin_alias}, blockbook={blockbook_package_name}, "
            f"backend={backend_package_name}, target={target}, build_backend={str(build_backend).lower()}, "
            f"reason={reason}, rpc_env={rpc_env}, rpc_host={host or '<unset>'}"
        )
        make_targets.append(target)

        log(f"removing previous packages matching build/{blockbook_package_name}_*.deb")
        for path in Path("build").glob(f"{blockbook_package_name}_*.deb"):
            path.unlink()
        if build_backend:
            log(f"removing previous packages matching build/{backend_package_name}_*.deb")
            for path in Path("build").glob(f"{backend_package_name}_*.deb"):
                path.unlink()
        shutil.rmtree(Path(package_root) / branch_or_tag_path / coin, ignore_errors=True)

    log("starting build: make " + " ".join(make_targets))
    try:
        subprocess.run(["make", *make_targets], check=True)
    except subprocess.CalledProcessError as exc:
        raise SystemExit(exc.returncode) from exc
    log("build finished")

    for coin, blockbook_package_name, backend_package_name, build_backend in zip(
        coins, blockbook_package_names, backend_package_names, build_backend_flags
    ):
        blockbook_package_file = latest_package(f"{blockbook_package_name}_*.deb")
        backend_package_file: Path | None = None
        if build_backend:
            backend_package_file = latest_package(f"{backend_package_name}_*.deb")

        target_dir = Path(package_root) / branch_or_tag_path / coin
        target_dir.mkdir(parents=True, exist_ok=True)

        staged_blockbook = target_dir / blockbook_package_file.name
        shutil.copy2(blockbook_package_file, staged_blockbook)
        log(f"staged {coin} blockbook to {staged_blockbook}")

        if build_backend and backend_package_file is not None:
            staged_backend = target_dir / backend_package_file.name
            shutil.copy2(backend_package_file, staged_backend)
            log(f"staged {coin} backend to {staged_backend}")

        log(f"built {coin} blockbook via {blockbook_package_file}")
        if backend_package_file is not None:
            log(f"built {coin} backend via {backend_package_file}")
        print(blockbook_package_file)


if __name__ == "__main__":
    main()
