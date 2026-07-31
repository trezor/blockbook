#!/usr/bin/env python3
"""ENS address-alias regression evidence for Blockbook (Ethereum mainnet).

Queries a running Blockbook's public API and asserts the *correct* (fixed) state
for a curated set of on-chain cases. Run it BEFORE deploying the ENS-registrar
fix (and running -rebuildensaliases) to capture evidence the current build is
wrong, and AFTER to prove it is fixed.

  - "missing"  : a real .eth registered via the current 7-arg ETHRegistrar
                 controller (0x59e16fcc…), which the old 5-arg-only parser never
                 recorded. Correct state: owner's alias == the name.
  - "spoofed"  : an alias written by an untrusted look-alike emitter
                 (0xeeb9b6bf…). Correct state: no such alias (purged).
  - "legit"    : a real name via a trusted controller. Correct state: unchanged
                 (proves the fix does not over-purge).

Exit code 0 => all correct (fixed). Non-zero => at least one case wrong, and the
failing rows ARE the evidence.

Usage:
  ./check_ens_aliases.py [--url https://eth.trezor.io] [--verbose]

No third-party deps (stdlib only). The API path used is
  /api/v2/address/<addr>?details=txs&pageSize=1
whose `addressAliases[<addr>]` carries the queried address's own alias.
"""
import argparse
import json
import sys
import time
import urllib.request

# Cases captured live against eth.trezor.io (gitCommit 2ede49f) on 2026-07-30.
# kind: "missing" | "spoofed" | "legit"
CASES = [
    # Modern 7-arg controller registrations the old parser dropped.
    {"kind": "missing", "addr": "0xd55b370335459e5e9dbd9d1c0a4fc5fa341a57b3", "name": "theprism.eth"},
    {"kind": "missing", "addr": "0x9d56610c9fdabd06a19486ced80cccca00a2866a", "name": "rell.eth"},
    {"kind": "missing", "addr": "0x29b65c53e48aba1c3692c66544b74ac60235c418", "name": "yasir.eth"},
    {"kind": "missing", "addr": "0x174976a7bae6bf9abb80a7bd6896a78fbd17386d", "name": "cormaya.eth"},
    {"kind": "missing", "addr": "0x6bae7f8c2d243cc08ae6ada73a4d728d5595f97b", "name": "samsrep.eth"},
    {"kind": "missing", "addr": "0x172b3047ad0b5e88718f70258dd0ec1a423d075e", "name": "jesuschristisking.eth"},
    # Spoofed alias from the untrusted emitter 0xeeb9b6bf5fb68fb726005f7ba549c2f4b32f2dad.
    {"kind": "spoofed", "addr": "0x0031A2D5EA5B69bcb8e191f5F0aE7919e9c9f0dB", "name": "gizmoimaginarykitten.eth"},
    # A well-known legit name via a trusted controller; must survive the rebuild.
    {"kind": "legit", "addr": "0x287740B694CdA65642a3A1CD186C81287E93D802", "name": "liquidnetwork.eth"},
]


def _get_json(url, timeout=30, retries=6):
    last = None
    for attempt in range(retries):
        try:
            with urllib.request.urlopen(url, timeout=timeout) as r:
                return json.load(r)
        except Exception as e:  # transient network hiccups: retry with backoff
            last = e
            time.sleep(1.0 * (attempt + 1))
    raise last


def alias_of(base, addr, timeout=30):
    d = _get_json(f"{base}/api/v2/address/{addr}?details=txs&pageSize=1", timeout)
    aliases = d.get("addressAliases", {}) or {}
    for k, v in aliases.items():
        if k.lower() == addr.lower():
            return v.get("Alias", ""), v.get("Type", "")
    return "", ""


def backend_info(base, timeout=30):
    try:
        with urllib.request.urlopen(f"{base}/api/v2/api", timeout=timeout) as r:
            bb = json.load(r).get("blockbook", {})
        return f"{bb.get('coin')} v{bb.get('version')} @ {bb.get('gitCommit')} (bestHeight {bb.get('bestHeight')})"
    except Exception as e:
        return f"<could not read /api/v2/api: {e}>"


def evaluate(kind, name, got_alias):
    """Return (ok, expected_str). ok == True means the FIXED/correct state."""
    if kind == "spoofed":
        return (got_alias.lower() != name.lower(), f"absent (not {name})")
    return (got_alias == name, name)  # missing + legit both expect the name


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", default="https://eth.trezor.io", help="Blockbook base URL")
    ap.add_argument("--verbose", action="store_true")
    args = ap.parse_args()
    base = args.url.rstrip("/")

    print(f"Blockbook: {backend_info(base)}")
    print(f"{'KIND':8} {'ADDRESS':44} {'EXPECTED':26} {'GOT':26} STATUS")
    print("-" * 118)
    failures = 0
    for c in CASES:
        try:
            got, gtype = alias_of(base, c["addr"])
        except Exception as e:
            print(f"{c['kind']:8} {c['addr']:44} {'<request error>':26} {str(e)[:24]:26} ERROR")
            failures += 1
            continue
        ok, expected = evaluate(c["kind"], c["name"], got)
        status = "OK" if ok else "WRONG"
        if not ok:
            failures += 1
        shown = got if got else "<none>"
        print(f"{c['kind']:8} {c['addr']:44} {expected:26} {shown:26} {status}")

    print("-" * 118)
    total = len(CASES)
    if failures:
        print(f"RESULT: {failures}/{total} case(s) WRONG — this build's ENS aliases are incorrect.")
        print("        (Run after deploying the fix + `-rebuildensaliases` to confirm all OK.)")
        sys.exit(1)
    print(f"RESULT: all {total} case(s) correct — ENS aliases are fixed.")


if __name__ == "__main__":
    main()
