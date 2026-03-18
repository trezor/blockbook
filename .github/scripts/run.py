#!/usr/bin/env python3

from __future__ import annotations

import argparse
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


def default_workflow_ref() -> str:
    return current_branch() or "<workflow-branch>"


def print_help() -> None:
    workflow_ref = default_workflow_ref()
    print(
        f"""Usage:
  {SCRIPT_NAME} help
  {SCRIPT_NAME} list [--env <dev|prod>] [--repo <owner/repo>] [--format <csv|lines>]
  {SCRIPT_NAME} build --coins <csv> [--env <dev|prod>] [--workflow-ref <ref>] [--checkout-ref <ref>] [--repo <owner/repo>] [--run]
  {SCRIPT_NAME} deploy --coins <csv> [--workflow-ref <ref>] [--checkout-ref <ref>] [--repo <owner/repo>] [--run]
  {SCRIPT_NAME} watch [<run-id>] [--repo <owner/repo>]

Commands:
  help      Show this help.
  list      List coins available for dev or prod builds.
  build     Print or run the Build / Deploy workflow in build mode.
  deploy    Print or run the Build / Deploy workflow in deploy mode.
  watch     Watch the latest Build / Deploy workflow run or a specific run ID.

Defaults:
  --repo : {DEFAULT_REPO}
  --workflow-ref: {workflow_ref}
  --checkout-ref: {workflow_ref}
  --env: dev

Operations:
  list: Prints available coins for a build environment.
    env=dev  -> coins buildable on dev runners
    env=prod -> all configured runner-mapped coins

  build: Builds Debian packages only.
    env=dev  -> uses BB_RUNNER_* mapping, ALL skips prod-only coins
    env=prod -> builds selected coins on production_builder

  deploy: Builds, installs, restarts, waits for sync, then runs e2e tests.
    env is fixed to dev.
    ALL is not accepted.
    Coins mapped to production_builder are rejected.

  watch: Watches the latest Build / Deploy run by default.
    You may also pass a specific run ID.

Shared options for build/deploy:
  --repo <owner/repo>          GitHub repository.
                               Default: {DEFAULT_REPO}
  --workflow-ref <ref>         Branch/tag/commit that contains deploy.yml.
                               Default: current git branch.
  --checkout-ref <ref>         Branch/tag/commit to run the workflow on.
                               Default: current git branch.
  --coins <csv>                Required. Coin list, e.g. bitcoin,bsc_archive or ALL (only for build).
  --run                        Execute the generated gh command instead of printing it.

Build options:
  --env <dev|prod>             Build environment (not accepted for deploy).
                               Default: dev.

List options:
  --env <dev|prod>             Which build environment to list coins for.
                               Default: dev.
  --format <csv|lines>         Output format.
                               Default: lines.

Examples:
  {SCRIPT_NAME} list --env dev
  {SCRIPT_NAME} list --env prod --format csv
  {SCRIPT_NAME} build --env dev --coins ALL
  {SCRIPT_NAME} build --env prod --coins bitcoin,bsc_archive
  {SCRIPT_NAME} build --env prod --coins bitcoin,bsc_archive --workflow-ref my-branch
  {SCRIPT_NAME} deploy --coins bitcoin,bsc_archive
  {SCRIPT_NAME} deploy --coins bitcoin --checkout-ref master --run
  {SCRIPT_NAME} watch
  {SCRIPT_NAME} watch 123456789"""
    )


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
    checkout_ref: str,
    build_env: str,
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
        "mode=build",
        "-f",
        f"env={build_env}",
        "-f",
        f"coins={coins}",
    ]
    if checkout_ref:
        cmd += ["-f", f"checkout_ref={checkout_ref}"]
    return cmd


def deploy_command(
    repo: str,
    workflow_ref: str,
    checkout_ref: str,
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
    if checkout_ref:
        cmd += ["-f", f"checkout_ref={checkout_ref}"]
    return cmd


def print_or_run(cmd: list[str], execute: bool) -> None:
    if execute:
        subprocess.run(cmd, check=True)
        return
    print(shlex.join(cmd))


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--repo", default=DEFAULT_REPO)
    parser.add_argument("--workflow-ref", default=current_branch())
    parser.add_argument("--checkout-ref", default=current_branch())
    parser.add_argument("--coins", required=True)
    parser.add_argument("--env", choices=("dev", "prod"), default="dev")
    parser.add_argument("--run", action="store_true")
    return parser


def deploy_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--repo", default=DEFAULT_REPO)
    parser.add_argument("--workflow-ref", default=current_branch())
    parser.add_argument("--checkout-ref", default=current_branch())
    parser.add_argument("--coins", required=True)
    parser.add_argument("--run", action="store_true")
    return parser


def watch_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("run_id", nargs="?")
    parser.add_argument("--repo", default=DEFAULT_REPO)
    return parser


def list_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--repo", default=DEFAULT_REPO)
    parser.add_argument("--env", choices=("dev", "prod"), default="dev")
    parser.add_argument("--format", choices=("csv", "lines"), default="lines")
    return parser


def command_build(argv: list[str]) -> None:
    if any(arg in {"-h", "--help"} for arg in argv):
        print_help()
        return
    args = build_parser().parse_args(argv)
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
            args.checkout_ref,
            args.env,
            "ALL" if selection.requested_all else ",".join(selection.coins),
        ),
        args.run,
    )


def command_deploy(argv: list[str]) -> None:
    if any(arg in {"-h", "--help"} for arg in argv):
        print_help()
        return
    args = deploy_parser().parse_args(argv)
    workflow_ref = args.workflow_ref or current_branch()
    if not workflow_ref:
        die("could not determine current git branch; pass --workflow-ref")

    context = load_deploy_context(args.repo)
    try:
        coins = resolve_deploy_selection(context, args.coins)
    except ValidationError as exc:
        die(str(exc))

    print_or_run(
        deploy_command(args.repo, workflow_ref, args.checkout_ref, ",".join(coins)),
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


def command_watch(argv: list[str]) -> None:
    if any(arg in {"-h", "--help"} for arg in argv):
        print_help()
        return
    args = watch_parser().parse_args(argv)
    run_id = args.run_id or latest_run_id(args.repo)
    if not run_id or run_id == "null":
        die("no Build / Deploy workflow runs found")
    subprocess.run(["gh", "run", "watch", "-R", args.repo, run_id], check=True)


def command_list(argv: list[str]) -> None:
    if any(arg in {"-h", "--help"} for arg in argv):
        print_help()
        return
    args = list_parser().parse_args(argv)
    context = load_context(args.repo)
    coins = context.dev_buildable_coins if args.env == "dev" else context.all_coins

    if args.format == "csv":
        print(",".join(coins))
        return
    for coin in coins:
        print(coin)


def main(argv: list[str] | None = None) -> None:
    args = list(sys.argv[1:] if argv is None else argv)
    command = args.pop(0) if args else "help"

    if command in {"help", "-h", "--help"}:
        print_help()
        return
    if command == "list":
        command_list(args)
        return
    if command == "build":
        command_build(args)
        return
    if command == "deploy":
        command_deploy(args)
        return
    if command == "watch":
        command_watch(args)
        return
    die(f"unknown command: {command}")


if __name__ == "__main__":
    main()
