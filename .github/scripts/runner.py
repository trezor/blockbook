import json
import os
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path

PRODUCTION_RUNNER = "production_builder"
PRODUCTION_RUNNER_LABEL = "production-builder"
LOG_PREFIX = "CI/CD Pipeline:"


class ValidationError(ValueError):
    pass


@dataclass(frozen=True)
class CoinContext:
    runner_map: dict[str, str]
    all_coins: list[str]
    dev_buildable_coins: list[str]
    has_deployability: bool
    deployable_coins: list[str]
    deployability_errors: dict[str, str]


@dataclass(frozen=True)
class BuildSelection:
    requested_all: bool
    coins: list[str]
    skipped_prod_only: list[str]


def fail(message: str) -> None:
    print(f"{LOG_PREFIX} error: {message}", file=sys.stderr)
    raise SystemExit(1)


def log(message: str) -> None:
    print(f"{LOG_PREFIX} {message}", flush=True)


def parse_json_object(raw: str, description: str) -> dict:
    try:
        payload = json.loads(raw)
    except json.JSONDecodeError as exc:
        raise ValidationError(f"cannot decode {description}: {exc}") from exc
    if not isinstance(payload, dict):
        raise ValidationError(f"{description} must contain a JSON object")
    return payload


def load_vars_map(repo: str, raw: str | None = None) -> dict:
    text = raw if raw is not None else os.environ.get("VARS_JSON", "")
    text = text.strip() if text else ""
    if text:
        return parse_json_object(text, "VARS_JSON")

    try:
        result = subprocess.run(
            ["gh", "variable", "list", "-R", repo, "--json", "name,value"],
            check=True,
            capture_output=True,
            text=True,
        )
    except FileNotFoundError as exc:
        raise ValidationError("gh CLI not found and VARS_JSON is not set") from exc
    except subprocess.CalledProcessError as exc:
        details = (exc.stderr or exc.stdout or str(exc)).strip()
        raise ValidationError(
            f"failed to list repository variables for {repo}: {details}"
        ) from exc

    try:
        rows = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        raise ValidationError(f"cannot decode gh variable list output: {exc}") from exc

    if not isinstance(rows, list):
        raise ValidationError("gh variable list output must be a JSON array")

    mapping = {}
    for row in rows:
        if not isinstance(row, dict):
            continue
        name = row.get("name")
        if not isinstance(name, str):
            continue
        mapping[name] = row.get("value")
    return mapping


def load_runner_map(vars_map: dict) -> dict[str, str]:
    prefix = "BB_RUNNER_"
    mapping = {}
    for key, value in vars_map.items():
        if not key.startswith(prefix):
            continue
        coin = key[len(prefix):].strip().lower()
        runner = "" if value is None else str(value).strip()
        if coin and runner:
            mapping[coin] = runner
    return mapping


def parse_coin_tokens(raw: str, *, allow_all: bool) -> tuple[bool, list[str]]:
    text = raw.strip()
    if not text:
        raise ValidationError("coins input is empty")

    if text.upper() == "ALL":
        if not allow_all:
            raise ValidationError("ALL is only supported in build mode")
        return True, []

    tokens = [part.strip() for part in re.split(r"[\s,]+", text) if part.strip()]
    if not tokens:
        raise ValidationError("coins input resolved to an empty list")
    if any(token.upper() == "ALL" for token in tokens):
        if not allow_all:
            raise ValidationError("ALL is only supported in build mode")
        raise ValidationError("ALL must be used alone")

    seen = set()
    result = []
    for coin in tokens:
        normalized = coin.lower()
        if normalized in seen:
            continue
        seen.add(normalized)
        result.append(normalized)
    return False, result


def is_production_only_runner(runner: str) -> bool:
    return runner == PRODUCTION_RUNNER


def build_runner_labels(runner: str, build_env: str) -> list[str]:
    if build_env == "prod":
        return ["self-hosted", PRODUCTION_RUNNER_LABEL]
    return ["self-hosted", "bb-dev-selfhosted", runner]


def coin_config_path(workspace: Path, coin: str) -> Path:
    return workspace / "configs" / "coins" / f"{coin}.json"


def require_coin_config(workspace: Path, coin: str) -> Path:
    config_path = coin_config_path(workspace, coin)
    if not config_path.exists():
        raise ValidationError(f"unknown coin '{coin}' (missing {config_path})")
    return config_path


def validate_runner_map_configs(workspace: Path, runner_map: dict[str, str]) -> None:
    missing = []
    for coin in sorted(runner_map):
        config_path = coin_config_path(workspace, coin)
        if not config_path.exists():
            missing.append(f"{coin} ({config_path})")

    if missing:
        raise ValidationError(
            "BB_RUNNER_* entries without matching configs/coins/<coin>.json: "
            + ", ".join(missing)
        )


def load_json_file(path: Path, description: str) -> dict:
    try:
        payload = json.loads(path.read_text(encoding="utf-8"))
    except Exception as exc:
        raise ValidationError(f"cannot read {path}: {exc}") from exc
    if not isinstance(payload, dict):
        raise ValidationError(f"invalid {description} {path}: expected a JSON object")
    return payload


def load_test_coin_name(config_path: Path) -> str:
    config = load_json_file(config_path, "config")

    coin_cfg = config.get("coin")
    if not isinstance(coin_cfg, dict):
        raise ValidationError(f"invalid config {config_path}: missing coin section")

    test_name = coin_cfg.get("test_name")
    if test_name is None:
        return config_path.stem
    if not isinstance(test_name, str):
        raise ValidationError(f"invalid config {config_path}: coin.test_name must be a string")

    test_name = test_name.strip()
    if not test_name:
        raise ValidationError(f"invalid config {config_path}: coin.test_name must not be empty")
    return test_name


def list_all_coins(workspace: Path, runner_map: dict[str, str]) -> list[str]:
    return sorted(runner_map)


def load_tests_config(workspace: Path) -> dict:
    tests_path = workspace / "tests" / "tests.json"
    return load_json_file(tests_path, "tests config")


def deployability_error(
    workspace: Path,
    runner_map: dict[str, str],
    coin: str,
    tests_cfg: dict | None = None,
) -> str | None:
    if coin not in runner_map:
        return f"missing BB_RUNNER_{coin}"

    configured_runner = runner_map[coin]
    if is_production_only_runner(configured_runner):
        return (
            f"coin '{coin}' is not deployable in dev; "
            f"BB_RUNNER_{coin} points to {configured_runner}"
        )

    config_path = coin_config_path(workspace, coin)
    if not config_path.exists():
        return f"unknown coin '{coin}' (missing {config_path})"

    if tests_cfg is None:
        tests_cfg = load_tests_config(workspace)

    lookup_coin = load_test_coin_name(config_path)
    test_cfg = tests_cfg.get(lookup_coin)
    if not isinstance(test_cfg, dict) or "connectivity" not in test_cfg:
        return (
            f"coin '{coin}' maps to test coin '{lookup_coin}' "
            "which has no connectivity tests in tests/tests.json"
        )

    return None


def load_coin_context(
    workspace: Path,
    vars_map: dict,
    *,
    include_deployability: bool = False,
) -> CoinContext:
    runner_map = load_runner_map(vars_map)
    if not runner_map:
        raise ValidationError("no BB_RUNNER_* variables found")

    validate_runner_map_configs(workspace, runner_map)
    all_coins = list_all_coins(workspace, runner_map)
    dev_buildable_coins = [
        coin for coin in all_coins if not is_production_only_runner(runner_map[coin])
    ]

    deployability_errors = {}
    deployable_coins = []
    if include_deployability:
        tests_cfg = load_tests_config(workspace)
        for coin in all_coins:
            error = deployability_error(workspace, runner_map, coin, tests_cfg)
            if error is None:
                deployable_coins.append(coin)
            else:
                deployability_errors[coin] = error

    return CoinContext(
        runner_map=runner_map,
        all_coins=all_coins,
        dev_buildable_coins=dev_buildable_coins,
        has_deployability=include_deployability,
        deployable_coins=deployable_coins,
        deployability_errors=deployability_errors,
    )


def load_coin_context_from_repo(
    workspace: Path,
    repo: str,
    raw_vars_json: str | None = None,
    *,
    include_deployability: bool = False,
) -> CoinContext:
    return load_coin_context(
        workspace,
        load_vars_map(repo, raw_vars_json),
        include_deployability=include_deployability,
    )


def resolve_build_selection(
    context: CoinContext,
    raw: str,
    build_env: str,
) -> BuildSelection:
    if build_env not in {"dev", "prod"}:
        raise ValidationError(f"invalid build env '{build_env}', expected 'dev' or 'prod'")

    requested_all, requested = parse_coin_tokens(raw, allow_all=True)
    selected = context.all_coins if requested_all else requested

    unknown = [coin for coin in selected if coin not in context.all_coins]
    if unknown:
        raise ValidationError(
            f"unknown build coin(s): {', '.join(unknown)}. "
            f"all selectable coins: {','.join(context.all_coins)}"
        )

    if build_env == "prod":
        if not selected:
            raise ValidationError("no coins selected after validation")
        return BuildSelection(requested_all=requested_all, coins=selected, skipped_prod_only=[])

    skipped_prod_only = [
        coin for coin in selected if coin not in context.dev_buildable_coins
    ]
    if skipped_prod_only and not requested_all:
        noun = "coin" if len(skipped_prod_only) == 1 else "coins"
        pronoun = "it" if len(skipped_prod_only) == 1 else "them"
        raise ValidationError(
            f"{noun} not available in build env=dev: {', '.join(skipped_prod_only)}. "
            f"dev-buildable coins: {','.join(context.dev_buildable_coins)}. "
            f"use --env prod to build {pronoun}"
        )

    coins = [
        coin for coin in selected if coin in context.dev_buildable_coins
    ]
    if not coins:
        raise ValidationError("no coins selected after filtering out prod-only coins for env=dev")

    return BuildSelection(
        requested_all=requested_all,
        coins=coins,
        skipped_prod_only=skipped_prod_only,
    )


def resolve_deploy_selection(context: CoinContext, raw: str) -> list[str]:
    if not context.has_deployability:
        raise ValidationError("deploy selection requires deployability context")

    if raw.strip().upper() == "ALL":
        raise ValidationError(
            "deploy does not support ALL; "
            f"deployable coins: {','.join(context.deployable_coins)}"
        )

    requested_all, requested = parse_coin_tokens(raw, allow_all=False)
    if requested_all:
        raise ValidationError(
            "deploy does not support ALL; "
            f"deployable coins: {','.join(context.deployable_coins)}"
        )

    unknown = [coin for coin in requested if coin not in context.all_coins]
    if unknown:
        raise ValidationError(
            f"unknown deploy coin(s): {', '.join(unknown)}. "
            f"all selectable coins: {','.join(context.all_coins)}"
        )

    not_deployable = [coin for coin in requested if coin not in context.deployable_coins]
    if not_deployable:
        reasons = [
            context.deployability_errors.get(coin, f"coin '{coin}' is not deployable")
            for coin in not_deployable
        ]
        raise ValidationError(
            f"coin(s) not deployable: {', '.join(not_deployable)}. "
            f"reasons: {' | '.join(reasons)}. "
            f"deployable coins: {','.join(context.deployable_coins)}"
        )

    return requested
