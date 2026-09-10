"""Dashboard payload for the 12h run: block truth, quotes, headline stats, and the
congestion windows the long run finally captured.

Column-oriented arrays rather than row objects, and the series are bucketed down to
a plottable width - 186k blocks would inline as tens of MB and no chart can show
that many points anyway.
"""
import csv, json, sys, statistics, collections, os

CHAIN_LABEL = {"eth":"Ethereum","bsc":"BNB Chain","pol":"Polygon","arb":"Arbitrum",
               "op":"Optimism","base":"Base","avax":"Avalanche","hype":"HyperEVM",
               "rhc":"Robinhood","etc":"Eth Classic"}
NATIVE = {"eth":"ETH","arb":"ETH","op":"ETH","base":"ETH","rhc":"ETH",
          "pol":"POL","avax":"AVAX","hype":"HYPE","bsc":"BNB"}
USD = {"eth":2480.91,"arb":2480.91,"op":2480.91,"base":2480.91,"rhc":2480.91,
       "pol":0.126156,"avax":7.62,"hype":85.67,"bsc":776.13}
TIERS = ["low","medium","high"]
TIER_LABEL = {"low":"Economy","medium":"Normal","high":"High"}
PKEY = {"served":"served","1inch":"oneinch","onchain":"onchain","onchainNew":"onchainNew"}
# What Suite actually shows as the tier ETA. A provider that ships maxWaitTimeEstimate
# wins over Suite's own fallback (EthereumFeeLevels.ts defaultBlocks {low:4,medium:2,high:1}),
# so the bar is not the same for every source - which is itself worth showing, because a
# lenient self-declared target flatters the provider that declared it.
# WAIT_MS read live from each host's estimateFee on 2026-09-07; only Infura populates it.
WAIT_MS = {"eth":(48000,24000,12000),"pol":(8000,6000,4000),"arb":(1000,750,500),
           "op":(8000,6000,4000),"base":(8000,6000,4000),"avax":(16000,12000,8000),
           "hype":(16000,12000,8000),"rhc":(400,300,200)}
# Suite blockTime = max(0.1, round(blocktime_seconds)) from connect-data coins-eth.json.
SUITE_BLOCKTIME = {"eth":12,"op":2,"avax":2,"pol":2,"hype":1,"rhc":0.1,"base":2,"arb":0.1}
FALLBACK_BLOCKS = {"low":4,"medium":2,"high":1}

MAXPTS = 700          # points per series after bucketing
SPIKE_MULT = 2.5      # base fee multiple over the window median that counts as congestion
r = lambda v, n=9: round(float(v), n)

samples_path, blocks_path, out_path = sys.argv[1], sys.argv[2], sys.argv[3]
srows = list(csv.DictReader(open(samples_path), delimiter="\t"))
brows = list(csv.DictReader(open(blocks_path), delimiter="\t"))

block_base, block_t = {}, {}
raw = collections.defaultdict(list)
for b in brows:
    c, h = b["chain"], int(b["height"])
    block_base[(c, h)] = float(b["base_gwei"])
    block_t[(c, h)] = int(b["time"])
    raw[c].append(b)

# ---- congestion windows, defined by the data rather than the clock ----------
# A block is congested when its base fee runs SPIKE_MULT over the window median;
# adjacent congested blocks are merged into one window so the page can shade it.
spikes, spike_set = {}, {}
for c, rows in raw.items():
    rows.sort(key=lambda x: int(x["height"]))
    med = statistics.median([float(x["base_gwei"]) for x in rows]) or 0
    hot = [int(x["height"]) for x in rows if med > 0 and float(x["base_gwei"]) > SPIKE_MULT * med]
    spike_set[c] = set(hot)
    wins, cur = [], None
    for h in hot:
        if cur and h - cur[1] <= 30:
            cur[1] = h
        else:
            cur = [h, h]; wins.append(cur)
    # Keep only windows worth shading; a couple of stray blocks is noise, not congestion.
    spikes[c] = [{"h0": a, "h1": b, "t0": block_t[(c,a)], "t1": block_t[(c,b)],
                  "peak": r(max(float(x["base_gwei"]) for x in rows if a <= int(x["height"]) <= b), 6),
                  "med": r(med, 6)}
                 for a, b in wins if block_t[(c,b)] - block_t[(c,a)] >= 300]

def bucket(rows, keyf, fields):
    """Collapse to at most MAXPTS points, median within each bucket. Bucketing by
    index keeps the x-axis evenly covered; the spike is far wider than one bucket."""
    n = len(rows)
    if n <= MAXPTS:
        idx = [[i] for i in range(n)]
    else:
        step = n / MAXPTS
        idx = [list(range(int(i*step), max(int(i*step)+1, int((i+1)*step)))) for i in range(MAXPTS)]
    out = {k: [] for k in fields}
    for grp in idx:
        grp = [g for g in grp if g < n]
        if not grp: continue
        for k, f in fields.items():
            vals = [f(rows[i]) for i in grp]
            out[k].append(r(statistics.median(vals), 9))
    return out

blocks = {}
for c, rows in raw.items():
    blocks[c] = bucket(rows, None, {
        "h": lambda x: int(x["height"]), "t": lambda x: int(x["time"]),
        "base": lambda x: float(x["base_gwei"]), "price": lambda x: float(x["clear_p10_gwei"]),
        "tip": lambda x: float(x["mkt_tip_p10_gwei"]), "txs": lambda x: int(x["txs"])})
    blocks[c]["h"] = [int(v) for v in blocks[c]["h"]]
    blocks[c]["t"] = [int(v) for v in blocks[c]["t"]]
    blocks[c]["txs"] = [int(v) for v in blocks[c]["txs"]]

# ---- replay the post-#1768 on-chain algorithm ------------------------------
# The capture recorded the OLD estimator (mean of p20/p70/p90/p99 over 4 blocks), so its
# rows would describe a Blockbook that no longer exists. blocks.tsv carries the per-block
# reward percentiles, which is everything the new estimator reads, so it can be replayed
# exactly over the same blocks at the same quote heights and scored by the same rule.
#
# Mirrors eip1559TierSpec in bchain/coins/eth/ethrpc.go.
NEW_SPEC = [("reward_p20_gwei", max),          # low     - p20, window maximum
            ("reward_p70_gwei", statistics.median),  # medium - p70, window median
            ("reward_p70_gwei", max)]          # high    - p70, window maximum
WINDOW, DEPTH, HEADROOM = 4, 8, 2

bidx = collections.defaultdict(dict)
for b in brows:
    bidx[b["chain"]][int(b["height"])] = b

def replay_onchain(chain, heights):
    """Regenerate the on-chain rows in the same shape the TSV uses, so everything
    downstream (bucketing, stats, congestion split, ETA table) works unchanged."""
    bl = bidx[chain]
    out = []
    for h in sorted(heights):
        if h not in bl:
            continue
        win = [bl[x] for x in range(h - WINDOW + 1, h + 1) if x in bl]
        if not win or any(h + i not in bl for i in range(1, DEPTH + 1)):
            continue
        base = float(bl[h]["base_gwei"])
        tips = [f([float(x[col]) for x in win]) for col, f in NEW_SPEC]
        for i in range(1, len(tips)):           # the monotonic clamp
            tips[i] = max(tips[i], tips[i - 1])
        for ti, tier in enumerate(TIERS):
            tip = tips[ti]
            mx = HEADROOM * base + tip
            inc, waited, paid, mkt = 0, 0, 0.0, 0.0
            for k in range(1, DEPTH + 1):
                nb = bl[h + k]
                if int(nb["txs"]) == 0:
                    continue
                nbase, clear = float(nb["base_gwei"]), float(nb["clear_p10_gwei"])
                if mx < nbase:
                    continue
                eff = min(mx, nbase + tip)
                if eff < clear:
                    continue
                inc, waited = h + k, k
                mkt = max(0.0, clear - nbase)
                paid = eff / clear if clear > 0 else 0.0
                break
            out.append({
                "chain": chain, "height": str(h), "tier": tier, "provider": "onchainNew",
                # Suite's clamps bound on 0% of rows in this capture, so the clamped and
                # unclamped values coincide; keeping both keys lets the rest of the code
                # stay identical for measured and replayed rows.
                "suite_tip_gwei": repr(tip), "suite_maxfee_gwei": repr(mx),
                "maxfee_gwei": repr(mx), "tip_gwei": repr(tip),
                "base_gwei": repr(base),
                # the on-chain path reads the base fee off the block, so it is exact
                "reported_base_gwei": repr(base),
                "included_at": str(inc), "blocks_waited": str(waited),
                "overpay_p10": repr(paid), "mkt_tip_gwei": repr(mkt),
                "clamp_bound": "false",
            })
    return out

qheights = collections.defaultdict(set)
for row in srows:
    if row["provider"] != "onchain":
        qheights[row["chain"]].add(int(row["height"]))
replayed = []
for c in qheights:
    replayed.extend(replay_onchain(c, qheights[c]))
# Both are kept: the measured rows are what Blockbook really served during the capture,
# the replayed ones are what the same blocks would have produced after #1768. Dropping the
# former would trade a measurement for a simulation.
srows = srows + replayed
print(f"replayed {len(replayed)} on-chain rows over the post-#1768 algorithm")

# ---- quotes, keyed chain -> tier -> provider --------------------------------
qraw = collections.defaultdict(lambda: collections.defaultdict(lambda: collections.defaultdict(list)))
for row in srows:
    if row["tier"] not in TIERS: continue
    qraw[row["chain"]][row["tier"]][PKEY[row["provider"]]].append(row)

def qbase(x):
    return block_base.get((x["chain"], int(x["height"])), float(x["base_gwei"]))

q = collections.defaultdict(lambda: collections.defaultdict(dict))
for c in qraw:
    for t in qraw[c]:
        for p, rows in qraw[c][t].items():
            rows.sort(key=lambda x: int(x["height"]))
            d = bucket(rows, None, {
                "h": lambda x: int(x["height"]),
                "tip": lambda x: float(x["suite_tip_gwei"]),
                # what this quote really pays per gas under the EIP-1559 rule, priced
                # against the block's own base rather than the base the source claimed
                "price": lambda x: min(float(x["suite_maxfee_gwei"]), qbase(x) + float(x["suite_tip_gwei"])),
                "maxfee": lambda x: float(x["suite_maxfee_gwei"]),
                "rbase": lambda x: float(x.get("reported_base_gwei") or x["base_gwei"])})
            d["h"] = [int(v) for v in d["h"]]
            q[c][t][p] = d

# ---- headline numbers, computed on every sample, not the bucketed series ----
def statblock(rows):
    inc = [x for x in rows if int(x["included_at"]) > 0]
    med = lambda f, rs=rows: statistics.median([f(x) for x in rs]) if rs else 0
    paid_rows = [x for x in inc if float(x["overpay_p10"]) > 0]
    usd = lambda gwei, c: gwei * 1e-9 * 21000 * USD.get(c, 0)
    c = rows[0]["chain"]
    price = med(lambda x: min(float(x["suite_maxfee_gwei"]), qbase(x) + float(x["suite_tip_gwei"])))
    going = med(lambda x: float(block_base.get((x["chain"], int(x["height"])), 0)), rows)
    clear = statistics.median([float(x["overpay_p10"]) for x in paid_rows]) if paid_rows else 0
    return dict(
        n=len(rows),
        tip=r(med(lambda x: float(x["suite_tip_gwei"])), 6),
        base=r(going, 6),
        price=r(price, 6),
        maxfee=r(med(lambda x: float(x["suite_maxfee_gwei"])), 6),
        mkt=r(med(lambda x: float(x["mkt_tip_gwei"]), inc), 6),
        paid=r(clear, 4),
        usdPaid=r(usd(price, c), 6),
        usdShown=r(usd(med(lambda x: float(x["suite_maxfee_gwei"])), c), 6),
        incl1=r(100 * sum(1 for x in rows if int(x["blocks_waited"]) == 1) / len(rows), 1),
        inclAll=r(100 * len(inc) / len(rows), 1),
        waitMax=max([int(x["blocks_waited"]) for x in inc], default=0),
        rbase=r(med(lambda x: float(x.get("reported_base_gwei") or x["base_gwei"])), 6),
        # Signed median shows systematic bias (early vs late); absolute median shows the
        # typical size of the error. A source wrong by +/-X every block has a signed
        # median of zero and is still wrong, so both are needed.
        baseLag=r(med(lambda x: float(x.get("reported_base_gwei") or x["base_gwei"]) - qbase(x)), 6),
        baseLagAbs=r(med(lambda x: abs(float(x.get("reported_base_gwei") or x["base_gwei"]) - qbase(x))), 6),
    )

stats, congestion = [], []
for c in qraw:
    for t in qraw[c]:
        for p, rows in qraw[c][t].items():
            stats.append(dict(chain=c, tier=t, provider=p, **statblock(rows)))
            hot = [x for x in rows if int(x["height"]) in spike_set[c]]
            calm = [x for x in rows if int(x["height"]) not in spike_set[c]]
            if len(hot) >= 20 and len(calm) >= 20:
                congestion.append(dict(chain=c, tier=t, provider=p,
                                       calm=statblock(calm), spike=statblock(hot)))

# ---- does the displayed ETA hold? ------------------------------------------
import math as _math
speed = []
for c in qraw:
    for t in TIERS:
        for p, rws in qraw[c][t].items():
            bt = SUITE_BLOCKTIME.get(c)
            if bt is None:
                continue
            # NB: not named `blocks` - that is the module-level per-chain block series,
            # and shadowing it here silently emptied the ground-truth line in the payload.
            if p == "served" and c in WAIT_MS:
                target = _math.ceil(WAIT_MS[c][TIERS.index(t)] / 1000 / bt)
                declared = "provider"
            else:
                target = FALLBACK_BLOCKS[t]
                declared = "fallback"
            ok = sum(1 for x in rws if 0 < int(x["blocks_waited"]) <= target)
            speed.append(dict(chain=c, tier=t, provider=p, blocks=target,
                              seconds=r(target * bt, 3), source=declared,
                              pct=r(100 * ok / len(rws), 1), n=len(rws)))

chains = [c for c in CHAIN_LABEL if c in q]
out = dict(chains=chains, chainLabels={c: CHAIN_LABEL[c] for c in chains},
           native={c: NATIVE.get(c, "") for c in chains}, usd={c: USD.get(c, 0) for c in chains},
           tiers=TIERS, tierLabels=TIER_LABEL,
           providers=[p for p in ["served","oneinch","onchain","onchainNew"]
                      if any(s["provider"] == p for s in stats)],
           blocks=blocks, quotes=q, stats=stats, spikes=spikes, congestion=congestion, speed=speed,
           samples=len(srows), blockCount=len(brows),
           hours=r((max(block_t.values()) - min(block_t.values())) / 3600, 1))
json.dump(out, open(out_path, "w"), separators=(",", ":"))
print(f"{len(srows)} samples, {len(brows)} blocks -> {os.path.getsize(out_path)//1024} KB")
for c in chains:
    if spikes[c]:
        w = spikes[c][0]
        print(f"  {c}: {len(spikes[c])} congestion window(s), first peak {w['peak']:.3f} vs median {w['med']:.3f} Gwei")
