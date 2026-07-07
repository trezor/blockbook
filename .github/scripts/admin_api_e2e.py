#!/usr/bin/env python3
"""E2E tests of the blockbook admin API (/admin/runtime-settings/,
/admin/contract-info/) against a deployed dev instance.

The tests are idempotent and leave the instance's database and its live
rpcCall policy exactly as they were found:

* runtime settings are exercised with a zero-policy-impact roundtrip — the
  POSTed value is always the *current effective value* (or the explicit-empty
  override when unset), so the parsed allowlist is identical at every instant,
  and the final DELETE restores the original source;
* the contract-info roundtrip uses a reserved never-real address and deletes
  its record at the end (and before starting, which heals a crashed run).

Configuration (environment):
  BB_ADMIN_E2E_COIN   coin this test targets (default base_archive)
  BB_ADMIN_E2E_URL    explicit internal-server base URL; when unset, the host
                      is derived from the BB_DEV_API_URL_HTTP_<coin> repo
                      variable and the port from the coin config's
                      ports.blockbook_internal, so a host move is a one-place
                      repo-variable change
  DEPLOYED_COINS      comma-separated deploy config names of this deploy run;
                      when set and the target coin is absent, the test skips
                      itself
  BB_ENV              content of blockbook.env; BB_ADMIN_USER and
                      BB_ADMIN_PASSWORD are extracted from it (never printed)
  BB_ADMIN_USER/BB_ADMIN_PASSWORD  direct credentials, override BB_ENV
  SYNC_CA_FILE/SYNC_TLS_INSECURE   TLS verification, as in wait_for_sync.py
"""

import base64
import json
import os
import ssl
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

from runner import LOG_PREFIX, ValidationError, fail, load_test_coin_name, log
from wait_for_sync import build_ssl_context, resolve_http_base

REPO_ROOT = Path(__file__).resolve().parents[2]

SETTING_KEYS = ("ALLOWED_RPC_CALL_TO", "ALLOWED_EVM_CALL_METHODS")
# Invalid values per setting; each must be rejected with 400 and change nothing.
INVALID_SETTING_VALUES = {
    "ALLOWED_RPC_CALL_TO": ",",
    "ALLOWED_EVM_CALL_METHODS": "0x12",
}
# Reserved, never-real address the contract roundtrip may create and delete.
TEST_CONTRACT_ADDRESS = "0xE2Ee00000000000000000000000000000000e2eE"


def strip_matching_quotes(value: str) -> str:
    """Remove one pair of surrounding quotes, the way systemd's
    EnvironmentFile does before the value reaches the server process."""
    if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
        return value[1:-1]
    return value


def parse_env_credentials(blob: str) -> tuple[str, str]:
    """Extract BB_ADMIN_USER/BB_ADMIN_PASSWORD from KEY=value lines."""
    values = {}
    for line in blob.splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, _, value = line.partition("=")
        key = key.strip()
        if key in ("BB_ADMIN_USER", "BB_ADMIN_PASSWORD"):
            values[key] = strip_matching_quotes(value.strip())
    return values.get("BB_ADMIN_USER", ""), values.get("BB_ADMIN_PASSWORD", "")


def coin_deployed(deployed_csv: str, coin: str) -> bool:
    """Whether coin is in the comma-separated deploy list; an empty list means
    the caller did not restrict the run (local use) and the test proceeds."""
    csv = deployed_csv.strip()
    if not csv:
        return True
    coins = {part.strip().lower() for part in csv.split(",") if part.strip()}
    return coin.strip().lower() in coins


def resolve_admin_url(coin: str) -> str:
    """Resolve the internal-server base URL: an explicit BB_ADMIN_E2E_URL
    wins; otherwise the host follows the same repo variable every other
    post-deploy check uses — keyed by the coin's *test name* (tests.json
    convention, e.g. BB_DEV_API_URL_HTTP_base for base_archive), not the
    deploy alias — with the internal port from the coin config. The internal
    server always serves HTTPS (the packaged self-signed certificate)."""
    explicit = os.environ.get("BB_ADMIN_E2E_URL", "").strip()
    if explicit:
        return explicit.rstrip("/")
    config_path = REPO_ROOT / "configs" / "coins" / (coin + ".json")
    try:
        test_coin = load_test_coin_name(config_path)
    except ValidationError as exc:
        fail(str(exc))
    host = urllib.parse.urlparse(resolve_http_base(test_coin)).hostname
    try:
        with open(config_path, encoding="utf-8") as f:
            port = json.load(f)["ports"]["blockbook_internal"]
    except (OSError, ValueError, KeyError) as exc:
        fail(f"cannot read ports.blockbook_internal from {config_path}: {exc}")
    return f"https://{host}:{port}"


def skip(message: str, annotation: str) -> None:
    """Exit 0 with a visible workflow annotation, so a skip can never be
    mistaken for a pass in the run summary."""
    print(f"::{annotation}::admin API e2e tests skipped: {message}")
    log(f"{message}; skipping admin API e2e tests")
    raise SystemExit(0)


def plan_settings_roundtrip(value: str, source: str) -> list[tuple[str, str | None, str, str]]:
    """Return the (method, body value, expected value, expected source) steps
    that exercise POST/GET/DELETE for a setting currently at (value, source)
    while keeping the parsed policy identical at every instant and ending in
    the starting state. A db-sourced setting is only rewritten in place: its
    env fallback is unknown, so a DELETE could change policy."""
    if source == "db":
        return [("POST", value, value, "db")]
    if source == "env":
        return [("POST", value, value, "db"), ("DELETE", None, value, "env")]
    if source == "unset":
        # the explicit-empty override behaves exactly like unset
        return [("POST", "", "", "db"), ("DELETE", None, "", "unset")]
    raise ValueError(f"unknown setting source {source!r}")


class AdminClient:
    # one retry absorbs a transient connection error without masking a real
    # outage; every request in this test is idempotent, so retrying is safe
    RETRIES = 1
    RETRY_DELAY_SECONDS = 3

    def __init__(self, base_url: str, user: str, password: str, timeout: int, context: ssl.SSLContext):
        self.base_url = base_url.rstrip("/")
        self.timeout = timeout
        self.context = context
        token = base64.b64encode(f"{user}:{password}".encode()).decode()
        self.auth_header = "Basic " + token

    def request(self, method: str, path: str, body: object = None, auth: bool = True) -> tuple[int, object]:
        data = None
        headers = {}
        if body is not None:
            data = json.dumps(body).encode()
            headers["Content-Type"] = "application/json"
        req = urllib.request.Request(self.base_url + path, data=data, method=method, headers=headers)
        if auth:
            req.add_header("Authorization", self.auth_header)
        for attempt in range(self.RETRIES + 1):
            try:
                with urllib.request.urlopen(req, timeout=self.timeout, context=self.context) as resp:
                    return resp.getcode(), self._parse(resp.read())
            except urllib.error.HTTPError as exc:
                return exc.code, self._parse(exc.read())
            except OSError as exc:  # URLError, TLS errors, timeouts
                if attempt < self.RETRIES:
                    log(f"{method} {path} failed ({exc}), retrying in {self.RETRY_DELAY_SECONDS}s")
                    time.sleep(self.RETRY_DELAY_SECONDS)
                    continue
                fail(f"{method} {self.base_url + path} failed after {attempt + 1} attempts: {exc}")

    @staticmethod
    def _parse(raw: bytes) -> object:
        try:
            return json.loads(raw)
        except (json.JSONDecodeError, UnicodeDecodeError):
            return {"raw": raw.decode("utf-8", "replace")}


def check(condition: bool, what: str, actual: object) -> None:
    if not condition:
        fail(f"{what}: got {actual!r}")


def get_setting(client: AdminClient, key: str) -> dict:
    status, resp = client.request("GET", f"/admin/runtime-settings/{key}")
    check(status == 200 and isinstance(resp, dict), f"GET {key} expected 200 object", (status, resp))
    return resp


def check_auth_gating(client: AdminClient) -> None:
    status, resp = client.request("GET", "/admin/runtime-settings/", auth=False)
    if status == 503:
        fail("admin interface is disabled on the instance (503) — BB_ADMIN_USER/BB_ADMIN_PASSWORD not configured there")
    check(status == 401, "unauthenticated GET expected 401", (status, resp))
    log("PASS auth: admin API rejects unauthenticated requests with 401")


def check_settings_list(client: AdminClient) -> dict[str, dict]:
    status, resp = client.request("GET", "/admin/runtime-settings/")
    check(status == 200 and isinstance(resp, list), "GET runtime-settings list expected 200 array", (status, resp))
    settings = {entry.get("key"): entry for entry in resp if isinstance(entry, dict)}
    for key in SETTING_KEYS:
        check(key in settings, f"runtime-settings list must contain {key}", sorted(settings))
        entry = settings[key]
        check(entry.get("source") in ("db", "env", "unset"), f"{key} has a valid source", entry)
    log("PASS settings: list returns all settings " + json.dumps([settings[k] for k in SETTING_KEYS]))
    return settings


def check_settings_negative(client: AdminClient, before: dict[str, dict]) -> None:
    status, resp = client.request("GET", "/admin/runtime-settings/NO_SUCH_SETTING")
    check(status == 400, "GET unknown setting expected 400", (status, resp))
    for key, invalid in INVALID_SETTING_VALUES.items():
        status, resp = client.request("POST", f"/admin/runtime-settings/{key}", {"value": invalid})
        check(status == 400, f"POST invalid value to {key} expected 400", (status, resp))
        after = get_setting(client, key)
        check(after == before[key], f"rejected POST must not change {key}", (before[key], after))
    log("PASS settings: unknown key and invalid values are rejected without effect")


def settings_roundtrip(client: AdminClient, key: str, current: dict) -> None:
    value, source = current.get("value", ""), current.get("source", "")
    steps = plan_settings_roundtrip(value, source)
    # If the run dies between POST and DELETE, still try to restore the
    # original source. The flag is raised before the POST is attempted: a
    # spurious restore DELETE of a missing override is a harmless no-op that
    # reverts to the env value, i.e. the state the failed POST left in place.
    restore_delete = False
    try:
        for method, body_value, want_value, want_source in steps:
            if method == "POST" and source != "db":
                restore_delete = True
            body = {"value": body_value} if method == "POST" else None
            status, resp = client.request(method, f"/admin/runtime-settings/{key}", body)
            want = {"key": key, "value": want_value, "source": want_source}
            check(status == 200 and resp == want, f"{method} {key} expected {want}", (status, resp))
            after = get_setting(client, key)
            check(after == want, f"GET {key} after {method} expected {want}", after)
            if method == "DELETE":
                restore_delete = False
    finally:
        if restore_delete:
            try:
                status, resp = client.request("DELETE", f"/admin/runtime-settings/{key}")
                log(f"restore: DELETE {key} -> {status} {json.dumps(resp)}")
            except Exception as exc:  # do not mask the original failure
                print(f"{LOG_PREFIX} restore DELETE {key} failed: {exc}", file=sys.stderr)
    log(f"PASS settings: {key} roundtrip from source={source!r} left state unchanged")


def contract_roundtrip(client: AdminClient) -> None:
    address = TEST_CONTRACT_ADDRESS
    path = f"/admin/contract-info/{address}"

    # pre-clean: heals a previously crashed run; deleted:false is the norm
    status, resp = client.request("DELETE", path)
    check(status == 200 and isinstance(resp, dict), "pre-clean DELETE expected 200", (status, resp))
    if resp.get("deleted"):
        log(f"pre-clean removed a leftover test record: {json.dumps(resp)}")

    # list pagination
    status, resp = client.request("GET", "/admin/contract-info/?limit=1")
    check(
        status == 200 and isinstance(resp, dict) and isinstance(resp.get("contracts"), list),
        "GET contract list expected 200 with contracts array",
        (status, resp),
    )
    check(len(resp["contracts"]) <= 1, "contract list must honor limit=1", len(resp["contracts"]))
    if resp.get("next"):
        status, page2 = client.request("GET", f"/admin/contract-info/?limit=1&from={resp['next']}")
        check(
            status == 200 and isinstance(page2, dict) and isinstance(page2.get("contracts"), list),
            "GET contract list next page expected 200",
            (status, page2),
        )
    status, resp = client.request("GET", "/admin/contract-info/?limit=0")
    check(status == 400, "GET contract list with limit=0 expected 400", (status, resp))

    # negative writes
    status, resp = client.request("POST", path, [])
    check(status == 400, "POST to an address path expected 400", (status, resp))
    status, resp = client.request("PATCH", "/admin/contract-info/")
    check(status == 400, "PATCH expected 400", (status, resp))

    # create -> read -> list from cursor -> delete -> delete again
    record = {
        "contract": address,
        "name": "TestToken-e2e",
        "symbol": "TSTE2E",
        "decimals": 18,
        "standard": "ERC20",
        "type": "ERC20",
    }
    # raised before the POST is attempted: the server may commit even when the
    # response is lost, and a spurious cleanup DELETE of a missing row is a
    # harmless no-op (same reasoning as the settings restore flag)
    created = False
    try:
        created = True
        status, resp = client.request("POST", "/admin/contract-info/", [record])
        check(status == 200 and resp == {"updated": 1}, "POST test contract expected {'updated': 1}", (status, resp))

        status, resp = client.request("GET", path)
        check(status == 200 and isinstance(resp, dict), "GET test contract expected 200", (status, resp))
        got = {k: resp.get(k) for k in ("name", "symbol", "decimals", "standard")}
        want = {k: record[k] for k in ("name", "symbol", "decimals", "standard")}
        check(got == want, f"GET test contract expected {want}", got)
        check(
            str(resp.get("contract", "")).lower() == address.lower(),
            "GET test contract address echo",
            resp.get("contract"),
        )

        status, resp = client.request("GET", f"/admin/contract-info/?limit=1&from={address}")
        check(
            status == 200
            and isinstance(resp, dict)
            and len(resp.get("contracts", [])) == 1
            and str(resp["contracts"][0].get("contract", "")).lower() == address.lower(),
            "list from=<test address> must start with the test record",
            (status, resp),
        )

        status, resp = client.request("DELETE", path)
        check(
            status == 200
            and isinstance(resp, dict)
            and resp.get("deleted") is True
            and resp.get("purged", {}).get("name") == record["name"],
            "DELETE must report deleted:true with the purged record",
            (status, resp),
        )
        created = False
        status, resp = client.request("DELETE", path)
        check(
            status == 200 and isinstance(resp, dict) and resp.get("deleted") is False and "purged" not in resp,
            "repeated DELETE must report deleted:false",
            (status, resp),
        )
    finally:
        if created:  # a failure after the POST must not leave the record behind
            try:
                status, resp = client.request("DELETE", path)
                log(f"cleanup: DELETE test contract -> {status} {json.dumps(resp)}")
            except Exception as exc:  # do not mask the original failure
                print(f"{LOG_PREFIX} cleanup DELETE {address} failed: {exc}", file=sys.stderr)
    log("PASS contracts: list/POST/GET/DELETE roundtrip left no record behind")


def main() -> None:
    coin = os.environ.get("BB_ADMIN_E2E_COIN", "base_archive")
    if not coin_deployed(os.environ.get("DEPLOYED_COINS", ""), coin):
        skip(f"{coin} is not part of this deploy run", "notice")

    user = os.environ.get("BB_ADMIN_USER", "").strip()
    password = os.environ.get("BB_ADMIN_PASSWORD", "").strip()
    if not (user and password):
        blob = os.environ.get("BB_ENV", "")
        if not blob.strip():
            # a warning, not a notice: in CI the secret is expected to exist,
            # so this skip usually means a lost/renamed BB_RUNTIME_ENV
            skip("BB_RUNTIME_ENV secret not set", "warning")
        user, password = parse_env_credentials(blob)
        if not (user and password):
            fail("BB_ENV does not contain BB_ADMIN_USER and BB_ADMIN_PASSWORD")

    base_url = resolve_admin_url(coin)
    ssl_context, tls_mode = build_ssl_context()
    log(f"TLS certificate verification: {tls_mode}")
    timeout = int(os.environ.get("ADMIN_E2E_REQUEST_TIMEOUT_SECONDS", "20"))
    client = AdminClient(base_url, user, password, timeout, ssl_context)
    log(f"Running admin API e2e tests against {base_url} ({coin})")

    check_auth_gating(client)
    settings = check_settings_list(client)
    check_settings_negative(client, settings)
    for key in SETTING_KEYS:
        settings_roundtrip(client, key, settings[key])
    contract_roundtrip(client)
    log("All admin API e2e tests passed; instance state is unchanged.")


if __name__ == "__main__":
    main()
