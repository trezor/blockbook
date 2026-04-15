#!/usr/bin/env python3

from __future__ import annotations

import argparse
import os
import shutil
import subprocess
import sys
from pathlib import Path

import backend_policy
from coin_rpc import (
    BUILD_ENV_DEV,
    BUILD_ENV_PROD,
    BUILD_ENV_VAR,
    CoinRPCError,
    load_config,
    resolve_build_env as resolve_build_env_common,
)


LOG_PREFIX = "CI/CD Pipeline:"
SCRIPT_NAME = "[build-packages]"
DEFAULT_PACKAGE_ROOT = "/opt/blockbook-builds"
def log(message: str) -> None:
    print(f"{LOG_PREFIX} {SCRIPT_NAME} {message}", file=sys.stderr, flush=True)


def fail(message: str) -> None:
    print(f"{LOG_PREFIX} error: {message}", file=sys.stderr)
    raise SystemExit(1)


def get_optional_package_name(config: dict, section: str, coin: str) -> str | None:
    value = config.get(section, {}).get("package_name", "")
    if value in (None, ""):
        return None
    if not isinstance(value, str) or not value.strip():
        fail(f"coin '{coin}' does not define a valid {section}.package_name")
    return value.strip()


def resolve_build_env() -> str:
    try:
        return resolve_build_env_common()
    except CoinRPCError as exc:
        fail(str(exc))
    return ""


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


def ensure_writable_dir(path: Path) -> None:
    root_dir = path.parent
    if not root_dir.exists():
        fail(f"writable root directory {root_dir} does not exist; pre-create it for the runner user")
    if not root_dir.is_dir():
        fail(f"writable root path {root_dir} is not a directory")
    if root_dir.stat().st_uid != os.getuid():
        fail(
            f"writable root directory {root_dir} must be owned by the runner user "
            f"(uid {os.getuid()})"
        )

    try:
        path.mkdir(parents=True, exist_ok=True)
        return
    except PermissionError:
        fail(f"cannot write to {path}; ensure {root_dir} is writable by the runner user")


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument(
        "--backend-mode",
        choices=(
            backend_policy.BACKEND_MODE_AUTO,
            backend_policy.BACKEND_MODE_ALWAYS,
            backend_policy.BACKEND_MODE_NEVER,
        ),
        default=backend_policy.BACKEND_MODE_AUTO,
    )
    parser.add_argument("coins", nargs="+")
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> None:
    raw_args = list(sys.argv[1:] if argv is None else argv)
    if not raw_args:
        fail(f"usage: {Path(sys.argv[0]).name} <coin-alias> [<coin-alias> ...]")
    parsed = parse_args(raw_args)
    args = parsed.coins

    backend_mode = parsed.backend_mode
    build_env = resolve_build_env()

    package_root = os.environ.get("BB_PACKAGE_ROOT", "").strip() or DEFAULT_PACKAGE_ROOT
    if not os.path.isabs(package_root):
        fail(f"BB_PACKAGE_ROOT must be an absolute path (got '{package_root}')")
    branch_or_tag = resolve_branch_or_tag()
    branch_or_tag_path = branch_or_tag.replace("/", "-")
    branch_root = Path(package_root) / branch_or_tag_path

    log("requested coins: " + " ".join(args))
    log(f"backend_mode={backend_mode}")
    log(f"{BUILD_ENV_VAR}={build_env}")
    if backend_mode == backend_policy.BACKEND_MODE_AUTO:
        log(
            "backend build rule: auto mode builds backend unless the selected "
            "BB_{DEV|PROD}_RPC_URL_HTTP is non-empty and non-local"
        )
    elif backend_mode == backend_policy.BACKEND_MODE_ALWAYS:
        log("backend build rule: always mode builds backend for coins that define a backend package")
    else:
        log(
            "backend build rule: never mode skips backend for coins that also build "
            "blockbook, but still builds backend-only coins"
        )
    log(f"branch_or_tag={branch_or_tag} -> path={branch_or_tag_path}")
    log(f"package_root={package_root}")

    ensure_writable_dir(branch_root)

    coins: list[str] = []
    blockbook_package_names: list[str | None] = []
    backend_package_names: list[str | None] = []
    build_backend_flags: list[bool] = []
    make_targets: list[str] = []

    for coin in args:
        config_path = Path("configs") / "coins" / f"{coin}.json"
        if not config_path.is_file():
            fail(f"missing coin config {config_path}")

        config = load_config(config_path)
        blockbook_package_name = get_optional_package_name(config, "blockbook", coin)
        backend_package_name = get_optional_package_name(config, "backend", coin)
        if blockbook_package_name is None and backend_package_name is None:
            fail(f"coin '{coin}' does not define blockbook.package_name or backend.package_name")
        try:
            decision = backend_policy.compute_backend_decision(
                coin=coin,
                config=config,
                build_env=build_env,
                backend_mode=backend_mode,
            )
        except CoinRPCError as exc:
            fail(str(exc))
        build_backend = decision["should_build_backend"]
        reason = decision["reason"]
        if backend_package_name is None:
            build_backend = False
            reason = "backend-missing"
        elif blockbook_package_name is None:
            build_backend = True
            reason = "blockbook-missing"

        coins.append(coin)
        blockbook_package_names.append(blockbook_package_name)
        backend_package_names.append(backend_package_name)
        build_backend_flags.append(build_backend)

        if blockbook_package_name is not None and backend_package_name is not None:
            target = f"deb-{coin}" if build_backend else f"deb-blockbook-{coin}"
        elif backend_package_name is not None:
            target = f"deb-backend-{coin}"
        else:
            target = f"deb-blockbook-{coin}"
        log(
            f"validated {coin}: alias={decision['coin_alias']}, blockbook={blockbook_package_name or '<none>'}, "
            f"backend={backend_package_name or '<none>'}, target={target}, build_backend={str(build_backend).lower()}, "
            f"reason={reason}, rpc_env={decision['rpc_env']}, rpc_host={decision['rpc_host'] or '<unset>'}"
        )
        make_targets.append(target)

        if blockbook_package_name is not None:
            log(f"removing previous packages matching build/{blockbook_package_name}_*.deb")
            for path in Path("build").glob(f"{blockbook_package_name}_*.deb"):
                path.unlink()
        if build_backend and backend_package_name is not None:
            log(f"removing previous packages matching build/{backend_package_name}_*.deb")
            for path in Path("build").glob(f"{backend_package_name}_*.deb"):
                path.unlink()
        shutil.rmtree(branch_root / coin, ignore_errors=True)

    log("starting build: make PORTABLE=1 " + " ".join(make_targets))
    try:
        subprocess.run(["make", "PORTABLE=1", *make_targets], check=True)
    except subprocess.CalledProcessError as exc:
        raise SystemExit(exc.returncode) from exc
    log("build finished")

    for coin, blockbook_package_name, backend_package_name, build_backend in zip(
        coins, blockbook_package_names, backend_package_names, build_backend_flags
    ):
        blockbook_package_file: Path | None = None
        backend_package_file: Path | None = None
        if blockbook_package_name is not None:
            blockbook_package_file = latest_package(f"{blockbook_package_name}_*.deb")
        if build_backend and backend_package_name is not None:
            backend_package_file = latest_package(f"{backend_package_name}_*.deb")

        target_dir = branch_root / coin
        target_dir.mkdir(parents=True, exist_ok=True)

        if blockbook_package_file is not None:
            staged_blockbook = target_dir / blockbook_package_file.name
            shutil.copy2(blockbook_package_file, staged_blockbook)
            log(f"staged {coin} blockbook to {staged_blockbook}")

        if build_backend and backend_package_file is not None:
            staged_backend = target_dir / backend_package_file.name
            shutil.copy2(backend_package_file, staged_backend)
            log(f"staged {coin} backend to {staged_backend}")

        if blockbook_package_file is not None:
            log(f"built {coin} blockbook via {blockbook_package_file}")
        if backend_package_file is not None:
            log(f"built {coin} backend via {backend_package_file}")
        if blockbook_package_file is not None:
            print(blockbook_package_file)
        elif backend_package_file is not None:
            print(backend_package_file)


if __name__ == "__main__":
    main()
