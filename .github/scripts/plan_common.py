import re
import sys


def fail(message: str) -> None:
    print(f"error: {message}", file=sys.stderr)
    raise SystemExit(1)


def load_runner_map(vars_map: dict) -> dict:
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


def parse_requested_coins(raw: str, available: dict) -> list[str]:
    text = raw.strip()
    if not text:
        fail("coins input is empty")

    if text.upper() == "ALL":
        coins = sorted(available.keys())
        if not coins:
            fail("no BB_RUNNER_* variables found")
        return coins

    tokens = [part.strip() for part in re.split(r"[\s,]+", text) if part.strip()]
    if not tokens:
        fail("coins input resolved to an empty list")
    if any(token.upper() == "ALL" for token in tokens):
        fail("ALL must be used alone")

    seen = set()
    result = []
    for coin in tokens:
        coin = coin.lower()
        if coin in seen:
            continue
        seen.add(coin)
        result.append(coin)
    return result
