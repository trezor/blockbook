#!/usr/bin/env python3
"""Refresh Ethereum historical fiat truth fixtures from CoinGecko."""

import argparse
import datetime as dt
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request


TOKENS = [
    {
        "coinId": "usd-coin",
        "symbol": "USDC",
        "contract": "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48",
        "relativeTolerance": 0.001,
    },
    {
        "coinId": "tether",
        "symbol": "USDT",
        "contract": "0xdac17f958d2ee523a2206206994597c13d831ec7",
        "relativeTolerance": 0.001,
    },
    {
        "coinId": "wrapped-bitcoin",
        "symbol": "WBTC",
        "contract": "0x2260fac5e5542a773aa44fbcfedf7c193bc2c599",
        "relativeTolerance": 0.05,
    },
    {
        "coinId": "aave",
        "symbol": "AAVE",
        "contract": "0x7fc66500c84a76ad7e9c93437bfc5ac33e2ddae9",
        "relativeTolerance": 0.05,
    },
    {
        "coinId": "uniswap",
        "symbol": "UNI",
        "contract": "0x1f9840a85d5af5bf1d1762f925bdaddc4201f984",
        "relativeTolerance": 0.05,
    },
]

DEFAULT_OUTPUT = "tests/api/testdata/ethereum_fiat_truth.json"
DEFAULT_POINTS = 20
FREE_BASE_URL = "https://api.coingecko.com/api/v3"
PRO_BASE_URL = "https://pro-api.coingecko.com/api/v3"


def parse_args():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--date",
        action="append",
        dest="dates",
        help="UTC date to fetch in YYYY-MM-DD format. Can be repeated. Overrides --points.",
    )
    parser.add_argument("--points", type=int, default=DEFAULT_POINTS, help="Number of spread daily points per token.")
    parser.add_argument("--output", default=DEFAULT_OUTPUT, help="Fixture path to write.")
    parser.add_argument(
        "--days",
        default="365",
        help="CoinGecko market_chart days parameter. Use max with a paid plan for older dates.",
    )
    parser.add_argument(
        "--plan",
        choices=("free", "pro"),
        default=os.getenv("COINGECKO_PLAN", "free"),
        help="CoinGecko API plan. Defaults to COINGECKO_PLAN or free.",
    )
    return parser.parse_args()


def utc_timestamp(date_text):
    try:
        parsed = dt.datetime.strptime(date_text, "%Y-%m-%d")
    except ValueError as exc:
        raise SystemExit(f"invalid --date {date_text!r}: expected YYYY-MM-DD") from exc
    return int(parsed.replace(tzinfo=dt.timezone.utc).timestamp())


def coingecko_base_url(plan):
    if plan == "pro":
        return PRO_BASE_URL
    return FREE_BASE_URL


def api_headers(plan):
    headers = {"User-Agent": "blockbook-fiat-truth-refresh/1.0"}
    api_key = os.getenv("COINGECKO_API_KEY", "").strip()
    if api_key:
        if plan == "pro":
            headers["x-cg-pro-api-key"] = api_key
        else:
            headers["x-cg-demo-api-key"] = api_key
    return headers


def fetch_json(url, headers):
    req = urllib.request.Request(url, headers=headers)
    last_error = None
    for attempt in range(4):
        try:
            with urllib.request.urlopen(req, timeout=30) as response:
                return json.load(response)
        except urllib.error.URLError as exc:
            last_error = exc
            if attempt == 3:
                break
            time.sleep(1.0 + attempt)
    raise last_error


def market_chart_url(base_url, coin_id, currency, days):
    query = urllib.parse.urlencode(
        {
            "vs_currency": currency,
            "days": days,
            "interval": "daily",
        }
    )
    return f"{base_url}/coins/{coin_id}/market_chart?{query}"


def price_by_timestamp(prices):
    result = {}
    for ts_ms, price in prices:
        if ts_ms % 1000 != 0:
            continue
        result[int(ts_ms) // 1000] = price
    return result


def select_spread_points(prices, count):
    points = []
    today_midnight = dt.datetime.now(dt.timezone.utc).replace(hour=0, minute=0, second=0, microsecond=0)
    today_midnight_ts = int(today_midnight.timestamp())
    for ts_ms, price in prices:
        if ts_ms % 1000 != 0 or price <= 0:
            continue
        ts = int(ts_ms) // 1000
        if ts % 86400 != 0 or ts >= today_midnight_ts:
            continue
        points.append([ts, price])
    if count <= 0:
        raise SystemExit("--points must be positive")
    if len(points) < count:
        raise SystemExit(f"CoinGecko returned only {len(points)} daily points, cannot select {count}")
    if count == 1:
        return [points[-1]]

    selected = []
    last_index = len(points) - 1
    for i in range(count):
        index = round(i * last_index / (count - 1))
        selected.append(points[index])
    return selected


def build_fixture(args):
    timestamps = []
    if args.dates:
        timestamps = [(date, utc_timestamp(date)) for date in args.dates]
    fetched_at = dt.datetime.now(dt.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
    base_url = coingecko_base_url(args.plan)
    headers = api_headers(args.plan)
    cases = []

    for token in TOKENS:
        url = market_chart_url(base_url, token["coinId"], "usd", args.days)
        data = fetch_json(url, headers)
        prices = data.get("prices", [])
        expected_rates = []
        if timestamps:
            prices_by_timestamp = price_by_timestamp(prices)
            for date_text, timestamp in timestamps:
                price = prices_by_timestamp.get(timestamp)
                if price is None:
                    raise SystemExit(
                        f"{token['symbol']} has no daily price for {date_text} in {url}; "
                        "use a wider --days value, a paid plan, or choose a newer fixture date"
                    )
                expected_rates.append([timestamp, price])
        else:
            expected_rates = select_spread_points(prices, args.points)

        cases.append(
            {
                "name": token["symbol"],
                "coinId": token["coinId"],
                "symbol": token["symbol"],
                "contract": token["contract"],
                "currency": "usd",
                "expectedRates": expected_rates,
                "relativeTolerance": token["relativeTolerance"],
                "source": url,
                "fetchedAt": fetched_at,
            }
        )
        time.sleep(2.0)

    return {
        "source": "coingecko",
        "currency": "usd",
        "fetchedAt": fetched_at,
        "cases": cases,
    }


def main():
    args = parse_args()
    fixture = build_fixture(args)
    with open(args.output, "w", encoding="utf-8") as f:
        json.dump(fixture, f, indent=2)
        f.write("\n")


if __name__ == "__main__":
    try:
        main()
    except urllib.error.HTTPError as exc:
        sys.exit(f"CoinGecko request failed: HTTP {exc.code}: {exc.read().decode('utf-8', 'replace')}")
