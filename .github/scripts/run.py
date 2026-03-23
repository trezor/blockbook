#!/usr/bin/env python3

from __future__ import annotations

import argparse
import json
import os
import shlex
import subprocess
import sys
from pathlib import Path

from runner import (
    ValidationError,
    load_coin_context_from_repo,
    resolve_build_selection,
    resolve_deploy_selection,
)


SCRIPT_PATH = Path(__file__).resolve()
SCRIPT_NAME = SCRIPT_PATH.name
REPO_ROOT = SCRIPT_PATH.parents[2]
DEFAULT_REPO = "trezor/blockbook"


class Formatter(argparse.RawTextHelpFormatter):
    pass


def die(message: str) -> None:
    print(f"error: {message}", file=sys.stderr)
    raise SystemExit(1)


def current_branch() -> str:
    try:
        result = subprocess.run(
            ["git", "branch", "--show-current"],
            cwd=REPO_ROOT,
            check=True,
            capture_output=True,
            text=True,
        )
    except (FileNotFoundError, subprocess.CalledProcessError):
        return ""
    return result.stdout.strip()


def workflow_ref_default() -> str:
    return current_branch()


def workflow_ref_display() -> str:
    return workflow_ref_default() or "<current-git-branch>"


def load_context(repo: str):
    try:
        return load_coin_context_from_repo(
            REPO_ROOT,
            repo,
            os.environ.get("VARS_JSON"),
            include_deployability=False,
        )
    except ValidationError as exc:
        die(str(exc))


def load_deploy_context(repo: str):
    try:
        return load_coin_context_from_repo(
            REPO_ROOT,
            repo,
            os.environ.get("VARS_JSON"),
            include_deployability=True,
        )
    except ValidationError as exc:
        die(str(exc))


def build_command(
    repo: str,
    workflow_ref: str,
    branch_or_tag: str,
    build_env: str,
    coins: str,
    always_build_backend: bool,
) -> list[str]:
    cmd = [
        "gh",
        "workflow",
        "run",
        "deploy.yml",
        "-R",
        repo,
        "--ref",
        workflow_ref,
        "-f",
        "mode=build",
        "-f",
        f"env={build_env}",
        "-f",
        f"coins={coins}",
    ]
    if always_build_backend:
        cmd += ["-f", "always_build_backend=true"]
    if branch_or_tag:
        cmd += ["-f", f"branch_or_tag={branch_or_tag}"]
    return cmd


def deploy_command(
    repo: str,
    workflow_ref: str,
    branch_or_tag: str,
    coins: str,
) -> list[str]:
    cmd = [
        "gh",
        "workflow",
        "run",
        "deploy.yml",
        "-R",
        repo,
        "--ref",
        workflow_ref,
        "-f",
        "mode=deploy",
        "-f",
        "env=dev",
        "-f",
        f"coins={coins}",
    ]
    if branch_or_tag:
        cmd += ["-f", f"branch_or_tag={branch_or_tag}"]
    return cmd


def print_or_run(cmd: list[str], execute: bool) -> None:
    if execute:
        subprocess.run(cmd, check=True)
        return
    print(shlex.join(cmd))


def add_common_workflow_args(parser: argparse.ArgumentParser) -> None:
    parser.add_argument(
        "--repo",
        default=DEFAULT_REPO,
        help=f"GitHub repository (default: {DEFAULT_REPO})",
    )
    parser.add_argument(
        "--workflow-ref",
        default=workflow_ref_default(),
        help="Branch/tag/commit that contains deploy.yml (default: current git branch)",
    )
    parser.add_argument(
        "--branch-or-tag",
        default=workflow_ref_default(),
        help="Branch or tag to run the workflow on (default: current git branch)",
    )
    parser.add_argument(
        "--run",
        action="store_true",
        help="Execute the generated gh command instead of printing it",
    )


def handle_help(args: argparse.Namespace) -> None:
    parser = args.parser_map[args.topic] if args.topic else args.parser
    parser.print_help()


def handle_list(args: argparse.Namespace) -> None:
    context = load_context(args.repo)
    coins = context.dev_buildable_coins if args.env == "dev" else context.all_coins

    if args.format == "csv":
        print(",".join(coins))
        return
    for coin in coins:
        print(coin)


def handle_build(args: argparse.Namespace) -> None:
    workflow_ref = args.workflow_ref or current_branch()
    if not workflow_ref:
        die("could not determine current git branch; pass --workflow-ref")

    context = load_context(args.repo)
    try:
        selection = resolve_build_selection(context, args.coins, args.env)
    except ValidationError as exc:
        die(str(exc))

    print_or_run(
        build_command(
            args.repo,
            workflow_ref,
            args.branch_or_tag,
            args.env,
            "ALL" if selection.requested_all else ",".join(selection.coins),
            args.always_build_backend,
        ),
        args.run,
    )


def handle_deploy(args: argparse.Namespace) -> None:
    workflow_ref = args.workflow_ref or current_branch()
    if not workflow_ref:
        die("could not determine current git branch; pass --workflow-ref")

    context = load_deploy_context(args.repo)
    try:
        coins = resolve_deploy_selection(context, args.coins)
    except ValidationError as exc:
        die(str(exc))

    print_or_run(
        deploy_command(args.repo, workflow_ref, args.branch_or_tag, ",".join(coins)),
        args.run,
    )


def latest_run_id(repo: str) -> str:
    try:
        result = subprocess.run(
            [
                "gh",
                "run",
                "list",
                "-R",
                repo,
                "--workflow",
                "deploy.yml",
                "--limit",
                "1",
                "--json",
                "databaseId",
                "--jq",
                ".[0].databaseId",
            ],
            check=True,
            capture_output=True,
            text=True,
        )
    except FileNotFoundError:
        die("gh CLI not found")
    except subprocess.CalledProcessError as exc:
        details = (exc.stderr or exc.stdout or str(exc)).strip()
        die(f"failed to fetch latest Build / Deploy run: {details}")
    return result.stdout.strip()


def run_metadata(repo: str, run_id: str) -> dict:
    try:
        result = subprocess.run(
            [
                "gh",
                "run",
                "view",
                "-R",
                repo,
                run_id,
                "--json",
                "status,conclusion",
            ],
            check=True,
            capture_output=True,
            text=True,
        )
    except FileNotFoundError:
        die("gh CLI not found")
    except subprocess.CalledProcessError as exc:
        details = (exc.stderr or exc.stdout or str(exc)).strip()
        die(f"failed to fetch Build / Deploy run metadata: {details}")
    try:
        payload = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        die(f"failed to decode Build / Deploy run metadata: {exc}")
    if not isinstance(payload, dict):
        die("Build / Deploy run metadata must be a JSON object")
    return payload


def show_run_logs(repo: str, run_id: str) -> None:
    subprocess.run(["gh", "run", "view", "-R", repo, run_id, "--log"], check=True)


def handle_watch(args: argparse.Namespace) -> None:
    run_id = args.run_id or latest_run_id(args.repo)
    if not run_id or run_id == "null":
        die("no Build / Deploy workflow runs found")
    metadata = run_metadata(args.repo, run_id)
    status = str(metadata.get("status") or "").strip().lower()
    if status == "completed":
        show_run_logs(args.repo, run_id)
        return
    subprocess.run(["gh", "run", "watch", "-R", args.repo, run_id], check=True)


def create_parser() -> tuple[argparse.ArgumentParser, dict[str, argparse.ArgumentParser]]:
    workflow_ref = workflow_ref_display()
    parser = argparse.ArgumentParser(
        prog=SCRIPT_NAME,
        formatter_class=Formatter,
        description="Helper for the Build / Deploy GitHub workflow.",
        epilog=(
            "Defaults:\n"
            f"  --repo: {DEFAULT_REPO}\n"
            f"  --workflow-ref: {workflow_ref}\n"
            f"  --branch-or-tag: {workflow_ref}\n"
            "  --env: dev\n\n"
            "Use '<command> --help' for command-specific options."
        ),
    )

    subparsers = parser.add_subparsers(dest="command")
    parser_map: dict[str, argparse.ArgumentParser] = {}

    help_parser = subparsers.add_parser(
        "help",
        formatter_class=Formatter,
        help="Show top-level or subcommand help.",
        description="Show top-level help or help for a specific subcommand.",
    )
    help_parser.add_argument("topic", nargs="?", choices=["list", "build", "deploy", "watch"])
    help_parser.set_defaults(func=handle_help)
    parser_map["help"] = help_parser

    list_parser = subparsers.add_parser(
        "list",
        formatter_class=Formatter,
        help="List coins available for dev or prod builds.",
        description="List available coins for a build environment.",
    )
    list_parser.add_argument(
        "--repo",
        default=DEFAULT_REPO,
        help=f"GitHub repository (default: {DEFAULT_REPO})",
    )
    list_parser.add_argument(
        "--env",
        choices=("dev", "prod"),
        default="dev",
        help="Build environment to list coins for (default: dev)",
    )
    list_parser.add_argument(
        "--format",
        choices=("csv", "lines"),
        default="lines",
        help="Output format (default: lines)",
    )
    list_parser.set_defaults(func=handle_list)
    parser_map["list"] = list_parser

    build_parser = subparsers.add_parser(
        "build",
        formatter_class=Formatter,
        help="Print or run the Build / Deploy workflow in build mode.",
        description=(
            "Build Debian packages only.\n"
            "- env=dev uses BB_RUNNER_* mapping and ALL skips prod-only coins\n"
            "- env=prod builds selected coins on the production-builder runner"
        ),
    )
    add_common_workflow_args(build_parser)
    build_parser.add_argument(
        "--coins",
        required=True,
        help="Required. Coin list, e.g. bitcoin,bsc_archive or ALL",
    )
    build_parser.add_argument(
        "--env",
        choices=("dev", "prod"),
        default="dev",
        help="Build environment (default: dev)",
    )
    build_parser.add_argument(
        "--always-build-backend",
        action="store_true",
        help=(
            "Build backend packages for every selected coin. "
            "If omitted, backend builds are derived from "
            "BB_BUILD_ENV plus BB_{DEV|PROD}_RPC_URL_HTTP_<coin_alias>; "
            "backend is skipped only for present non-local values"
        ),
    )
    build_parser.set_defaults(func=handle_build)
    parser_map["build"] = build_parser

    deploy_parser = subparsers.add_parser(
        "deploy",
        formatter_class=Formatter,
        help="Print or run the Build / Deploy workflow in deploy mode.",
        description=(
            "Build, install, restart, wait for sync, then run e2e tests.\n"
            "- env is fixed to dev\n"
            "- ALL is not accepted\n"
            "- coins mapped to production_builder are rejected"
        ),
    )
    add_common_workflow_args(deploy_parser)
    deploy_parser.add_argument(
        "--coins",
        required=True,
        help="Required. Coin list, e.g. bitcoin,bsc_archive",
    )
    deploy_parser.set_defaults(func=handle_deploy)
    parser_map["deploy"] = deploy_parser

    watch_parser = subparsers.add_parser(
        "watch",
        formatter_class=Formatter,
        help="Watch the latest Build / Deploy workflow run or a specific run ID.",
        description="Watch the latest Build / Deploy workflow run or a specific run ID.",
    )
    watch_parser.add_argument("run_id", nargs="?", help="Optional workflow run ID to watch")
    watch_parser.add_argument(
        "--repo",
        default=DEFAULT_REPO,
        help=f"GitHub repository (default: {DEFAULT_REPO})",
    )
    watch_parser.set_defaults(func=handle_watch)
    parser_map["watch"] = watch_parser

    return parser, parser_map


def main(argv: list[str] | None = None) -> None:
    parser, parser_map = create_parser()
    args = parser.parse_args(sys.argv[1:] if argv is None else argv)
    if not getattr(args, "command", None):
        parser.print_help()
        return
    args.parser = parser
    args.parser_map = parser_map
    args.func(args)


if __name__ == "__main__":
    main()
