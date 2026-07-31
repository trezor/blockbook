#!/usr/bin/env python3
"""Lint backend artifact sources in configs/coins/*.json.

Security policy (see CI/CD security review, finding M2):

  FAIL when a backend artifact download is not pinned to immutable, verified
  content, specifically:
    * a mutable source ref (a `…/raw/<branch>/…` or `…/archive/refs/heads/…`
      path, or any `refs/heads/…`) in `binary_url` *or* in `extract_command`
      (some coins fetch a config file at build time);
    * a `binary_url` with no integrity check at all (empty/absent
      `verification_type` / `verification_source`);
    * a plaintext `http://` `binary_url` that *also* lacks an integrity check.

  WARN (does not fail the build) when a `binary_url` is plaintext `http://`
  but is checksum/signature verified. The content is integrity-protected, so
  the residual risk is availability/trust rather than tampering; some of these
  upstreams do not offer HTTPS. Surfaced so it stays visible.

Immutable references that are allowed: pinned release tags (`…/download/<tag>/…`,
`…/archive/refs/tags/…`) and 40-hex commit SHAs.
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parents[2]
COINS_DIR = REPO_ROOT / "configs" / "coins"

VALID_VERIFICATION_TYPES = {"sha256", "gpg", "gpg-sha256", "docker"}

COMMIT_SHA_RE = re.compile(r"^[0-9a-fA-F]{40}$")
# `raw.githubusercontent.com/<owner>/<repo>/<ref>/…` and
# `github.com/<owner>/<repo>/raw/<ref>/…` — capture the ref segment.
RAW_HOST_RE = re.compile(r"raw\.githubusercontent\.com/[^/\s]+/[^/\s]+/([^/\s]+)/")
RAW_PATH_RE = re.compile(r"github\.com/[^/\s]+/[^/\s]+/raw/([^/\s]+)/")


def _ref_is_mutable(ref: str) -> bool:
    # A full commit SHA is immutable; a branch name (master/develop/…) is not.
    # `refs` is the first segment of `refs/heads|tags/…`, handled separately.
    return ref != "refs" and not COMMIT_SHA_RE.match(ref)


def is_mutable(text: str) -> bool:
    """True if `text` references a mutable git branch (vs a tag or commit SHA).

    Catches both `binary_url`s and build-time fetches embedded in
    `extract_command`. Pinned commit SHAs and `refs/tags/...` are immutable.
    """
    if not text:
        return False
    # Explicit branch refs: `refs/heads/...`, `/archive/refs/heads/...`.
    if re.search(r"refs/heads/", text) or re.search(r"/archive/refs/heads/", text):
        return True
    for pattern in (RAW_HOST_RE, RAW_PATH_RE):
        for match in pattern.finditer(text):
            if _ref_is_mutable(match.group(1)):
                return True
    return False


def has_integrity(backend: dict) -> bool:
    vtype = (backend.get("verification_type") or "").strip()
    vsource = (backend.get("verification_source") or "").strip()
    return vtype in VALID_VERIFICATION_TYPES and vtype != "" and vsource != ""


def _is_go_zero_value(value: object) -> bool:
    """Return whether a JSON value maps to a zero Go struct field value."""
    if value is None:
        return True
    if isinstance(value, bool):
        return not value
    if isinstance(value, (int, float)):
        return value == 0
    if isinstance(value, str):
        return value == ""
    return False


def merge_backend_platform(base: dict, override: dict) -> dict:
    """Match build/tools LoadConfig platform override behavior.

    The Go loader copies non-zero fields from backend.platforms.<GOARCH> onto
    the top-level backend. Empty strings, false, zero, and null do not override
    top-level defaults.
    """
    merged = dict(base)
    for key, value in override.items():
        if not _is_go_zero_value(value):
            merged[key] = value
    return merged


def effective_backends(backend: dict) -> list[tuple[str, dict]]:
    backends = [("backend", backend)]
    platforms = backend.get("platforms")
    if not isinstance(platforms, dict):
        return backends

    for platform, override in sorted(platforms.items()):
        if isinstance(override, dict):
            backends.append(
                (f"backend.platforms.{platform}", merge_backend_platform(backend, override))
            )

    return backends


def _with_context(context: str, message: str) -> str:
    if context == "backend":
        return message
    return f"{context}: {message}"


def lint_backend(context: str, backend: dict) -> tuple[list[str], list[str]]:
    errors: list[str] = []
    warnings: list[str] = []

    binary_url = (backend.get("binary_url") or "").strip()
    extract_command = backend.get("extract_command") or ""

    # A build-time fetch of a config/asset from a mutable branch is just as
    # dangerous as a mutable binary_url.
    if is_mutable(binary_url):
        errors.append(
            _with_context(context, f"binary_url uses a mutable branch ref: {binary_url}")
        )
    if is_mutable(extract_command):
        errors.append(
            _with_context(
                context,
                "extract_command fetches from a mutable branch ref (pin to a commit SHA)",
            )
        )

    if not binary_url:
        return errors, warnings

    integrity = has_integrity(backend)
    if not integrity:
        errors.append(
            _with_context(
                context,
                "binary_url has no integrity check "
                "(set verification_type + verification_source)",
            )
        )

    if binary_url.lower().startswith("http://"):
        if integrity:
            warnings.append(
                _with_context(
                    context,
                    f"binary_url uses plaintext HTTP (checksum-verified): {binary_url}",
                )
            )
        else:
            errors.append(
                _with_context(
                    context,
                    f"binary_url uses plaintext HTTP with no integrity check: {binary_url}",
                )
            )

    return errors, warnings


def lint_file(path: Path) -> tuple[list[str], list[str]]:
    """Return (errors, warnings) for a single coin config."""
    errors: list[str] = []
    warnings: list[str] = []
    try:
        config = json.loads(path.read_text(encoding="utf-8"))
    except (json.JSONDecodeError, OSError) as exc:
        return [f"could not parse: {exc}"], []

    backend = config.get("backend")
    if not isinstance(backend, dict):
        return errors, warnings

    for context, effective_backend in effective_backends(backend):
        backend_errors, backend_warnings = lint_backend(context, effective_backend)
        errors.extend(backend_errors)
        warnings.extend(backend_warnings)

    return errors, warnings


def main() -> int:
    if not COINS_DIR.is_dir():
        print(f"error: {COINS_DIR} not found", file=sys.stderr)
        return 2

    total_errors = 0
    total_warnings = 0
    for path in sorted(COINS_DIR.glob("*.json")):
        errors, warnings = lint_file(path)
        for msg in warnings:
            print(f"WARN  {path.name}: {msg}")
            total_warnings += 1
        for msg in errors:
            print(f"FAIL  {path.name}: {msg}", file=sys.stderr)
            total_errors += 1

    print(
        f"\nbackend artifact lint: {total_errors} error(s), {total_warnings} warning(s)",
        file=sys.stderr,
    )
    return 1 if total_errors else 0


if __name__ == "__main__":
    raise SystemExit(main())
