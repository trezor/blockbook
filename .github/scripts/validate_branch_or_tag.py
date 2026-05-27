#!/usr/bin/env python3

from __future__ import annotations

import argparse
import subprocess
import sys


LOG_PREFIX = "CI/CD Pipeline:"
SCRIPT_NAME = "[validate-branch-or-tag]"


def log(message: str) -> None:
    print(f"{LOG_PREFIX} {SCRIPT_NAME} {message}", file=sys.stderr, flush=True)


def fail(message: str) -> None:
    print(f"{LOG_PREFIX} error: {message}", file=sys.stderr)
    raise SystemExit(1)


def ref_exists(repo: str, ref: str, kind: str) -> bool:
    remote = f"https://github.com/{repo}.git"
    try:
        subprocess.run(
            ["git", "ls-remote", "--exit-code", f"--{kind}", remote, ref],
            check=True,
            capture_output=True,
            text=True,
        )
    except FileNotFoundError as exc:
        fail("git is required for branch/tag validation")
        raise AssertionError("unreachable") from exc
    except subprocess.CalledProcessError:
        return False
    return True


def validate_branch_or_tag(repo: str, ref: str) -> str:
    candidate = ref.strip()
    if not candidate:
        fail("branch_or_tag resolved to an empty value")

    if ref_exists(repo, candidate, "heads"):
        log(f"validated branch '{candidate}' in {repo}")
        return "branch"
    if ref_exists(repo, candidate, "tags"):
        log(f"validated tag '{candidate}' in {repo}")
        return "tag"

    fail(f"branch_or_tag '{candidate}' does not exist as a branch or tag in {repo}")
    raise AssertionError("unreachable")


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Validate that a branch_or_tag exists in a GitHub repository.")
    parser.add_argument("--repo", required=True)
    parser.add_argument("--ref", required=True)
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> None:
    args = parse_args(argv)
    validate_branch_or_tag(args.repo, args.ref)


if __name__ == "__main__":
    main()
