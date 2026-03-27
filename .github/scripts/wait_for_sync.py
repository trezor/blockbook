#!/usr/bin/env python3

import json
import os
import ssl
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

LOG_PREFIX = "CI/CD Pipeline:"


def fail(message: str) -> None:
    print(f"{LOG_PREFIX} error: {message}", file=sys.stderr)
    raise SystemExit(1)


def log(message: str) -> None:
    print(f"{LOG_PREFIX} {message}", flush=True)


def parse_requested_coins(raw: str) -> list[str]:
    text = raw.strip()
    if not text:
        fail("COINS_INPUT is empty")

    seen = set()
    result = []
    for part in text.split(","):
        coin = part.strip().lower()
        if not coin or coin in seen:
            continue
        seen.add(coin)
        result.append(coin)
    if not result:
        fail("COINS_INPUT resolved to an empty list")
    return result


def normalize_http_base(raw: str) -> str:
    parsed = urllib.parse.urlparse(raw.strip())
    if parsed.scheme not in ("http", "https"):
        fail(f"unsupported HTTP scheme {parsed.scheme!r} in {raw!r}")
    if not parsed.netloc:
        fail(f"missing host in {raw!r}")
    return urllib.parse.urlunparse(
        (parsed.scheme, parsed.netloc, parsed.path or "/", "", "", "")
    ).rstrip("/")


def should_upgrade_to_https(status: int, body: bytes, base_url: str) -> bool:
    if status != 400:
        return False
    if "http request to an https server" not in body.decode("utf-8", "replace").lower():
        return False
    parsed = urllib.parse.urlparse(base_url)
    return parsed.scheme == "http"


def upgrade_http_base_to_https(raw: str) -> str:
    parsed = urllib.parse.urlparse(raw)
    if parsed.scheme != "http":
        return raw
    return urllib.parse.urlunparse(
        ("https", parsed.netloc, parsed.path, "", "", "")
    ).rstrip("/")


def resolve_http_base(coin: str) -> str:
    value = os.environ.get("BB_DEV_API_URL_HTTP_" + coin, "").strip()
    if not value:
        fail(f"missing BB_DEV_API_URL_HTTP_{coin} for selected test coin {coin!r}")
    return normalize_http_base(value)


def preview_body(body: bytes, limit: int = 200) -> str:
    text = body.decode("utf-8", "replace").strip()
    if len(text) <= limit:
        return text
    return text[: limit - 3] + "..."


def fetch_status(base_url: str, request_timeout: int) -> tuple[int, bytes]:
    request = urllib.request.Request(base_url + "/api/status")
    context = ssl._create_unverified_context()
    with urllib.request.urlopen(request, timeout=request_timeout, context=context) as resp:
        return resp.getcode(), resp.read()


def parse_int(value: object) -> int | None:
    if isinstance(value, bool):
        return None
    if isinstance(value, int):
        return value
    return None


def parse_sync_state(body: bytes) -> tuple[bool, str]:
    try:
        payload = json.loads(body)
    except json.JSONDecodeError as exc:
        return False, f"invalid JSON: {exc}"

    blockbook = payload.get("blockbook")
    if not isinstance(blockbook, dict):
        return False, "response missing blockbook object"

    backend = payload.get("backend")
    if backend is not None and not isinstance(backend, dict):
        return False, "response missing backend object"

    in_sync = blockbook.get("inSync")
    initial_sync = blockbook.get("initialSync")
    best_height = parse_int(blockbook.get("bestHeight"))
    backend_blocks = parse_int(backend.get("blocks")) if isinstance(backend, dict) else None

    ready = in_sync is True and initial_sync is not True
    summary = (
        f"inSync={in_sync!r}, initialSync={initial_sync!r}, "
        f"bestHeight={best_height!r}, backendBlocks={backend_blocks!r}"
    )

    if best_height is not None and backend_blocks is not None:
        height_lag = backend_blocks - best_height
        summary += f", heightLag={height_lag!r}"
        if height_lag > 1:
            ready = False

    return ready, summary


def main() -> None:
    coins = parse_requested_coins(os.environ.get("COINS_INPUT", ""))
    timeout_seconds = int(os.environ.get("SYNC_TIMEOUT_SECONDS", "1800"))
    poll_seconds = int(os.environ.get("SYNC_POLL_SECONDS", "10"))
    request_timeout = int(os.environ.get("SYNC_REQUEST_TIMEOUT_SECONDS", "20"))

    pending = {}
    last_seen = {}
    for coin in coins:
        if coin in pending:
            continue
        pending[coin] = resolve_http_base(coin)
        last_seen[coin] = "not checked yet"

    deadline = time.monotonic() + timeout_seconds
    log(
        "Waiting for Blockbook sync: "
        + ", ".join(f"{coin} -> {base}" for coin, base in sorted(pending.items()))
    )

    while pending:
        for coin in sorted(list(pending)):
            base_url = pending[coin]
            try:
                status, body = fetch_status(base_url, request_timeout)
            except urllib.error.HTTPError as exc:
                status = exc.code
                body = exc.read()
            except Exception as exc:
                last_seen[coin] = f"{base_url}/api/status request failed: {exc}"
                continue

            if should_upgrade_to_https(status, body, base_url):
                base_url = upgrade_http_base_to_https(base_url)
                pending[coin] = base_url
                try:
                    status, body = fetch_status(base_url, request_timeout)
                except urllib.error.HTTPError as exc:
                    status = exc.code
                    body = exc.read()
                except Exception as exc:
                    last_seen[coin] = f"{base_url}/api/status request failed: {exc}"
                    continue

            if status != 200:
                last_seen[coin] = (
                    f"{base_url}/api/status returned HTTP {status}: {preview_body(body)}"
                )
                continue

            in_sync, summary = parse_sync_state(body)
            last_seen[coin] = f"{base_url}/api/status returned HTTP 200: {summary}"
            if in_sync:
                log(f"{coin}: synced ({summary})")
                del pending[coin]

        if not pending:
            break

        remaining_seconds = int(max(0, deadline - time.monotonic()))
        if remaining_seconds == 0:
            break

        details = "; ".join(
            f"{coin}: {last_seen[coin]}" for coin in sorted(pending)
        )
        log(f"Still waiting for Blockbook sync ({remaining_seconds}s left): {details}")
        time.sleep(min(poll_seconds, remaining_seconds))

    if pending:
        details = "; ".join(
            f"{coin}: {last_seen[coin]}" for coin in sorted(pending)
        )
        fail(
            f"timed out after {timeout_seconds}s waiting for Blockbook sync. {details}"
        )

    log("All selected Blockbook instances are synced.")


if __name__ == "__main__":
    main()
