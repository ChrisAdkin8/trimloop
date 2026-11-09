#!/usr/bin/env python3
"""Recount trimloop's headline figures from Claude Code logs, independently.

    python3 -I scripts/recount.py <CLAUDE_CONFIG_DIR> [--json out.json]
        [--compare trimloop.json] [--tz UTC] [--tolerance 0.005]

This is a second implementation of docs/measurements.md, written from the
documents and the log format, not from the Go code. It exists so that
trimloop's figures can be checked against something that did not share its
mistakes: testdata/golden/expected.json is its output on the golden logs,
and cmd/trimloop/golden_test.go compares trimloop with it.

Keep it independent: when a figure differs, decide from the documents (the
measurements page, the ADRs it cites, docs/data-model.md) which side is
right, and fix only the side that is wrong. Do not copy Go logic here.

--compare reads `trimloop --no-history --format json` output and prints each
figure beside this script's, exiting 1 if any differs by more than
--tolerance dollars (counts must match exactly). Run trimloop with the same
time zone as --tz (TZ=UTC), since weeks are calendar weeks in local time.

Standard library only. The price table below copies pkg/price/price.go's
catalogue (version RATES_VERSION): update it whenever the catalogue changes.
"""

import argparse
import glob
import json
import math
import os
import sys
from collections import defaultdict
from datetime import datetime, timedelta, timezone

# ---------------------------------------------------------------------------
# Prices: US dollars per million tokens, as (input, 5m write, 1h write, cache
# read, output). Copied from pkg/price/price.go; keep in step with it.

RATES_VERSION = "anthropic-list-2026-11-09"

RATES = {
    "claude-opus-5-5": (4, 5, 8, 0.20, 20),
    "claude-opus-5": (5, 6.25, 10, 0.50, 25),
    "claude-fable-5-1": (10, 12.50, 20, 0.25, 50),
    "claude-fable-5": (10, 12.50, 20, 1, 50),
    "claude-opus-4-8": (5, 6.25, 10, 0.50, 25),
    "claude-opus-4-7": (5, 6.25, 10, 0.50, 25),
    "claude-opus-4-6": (5, 6.25, 10, 0.50, 25),
    "claude-sonnet-5-5": (2, 2.50, 4, 0.10, 10),
    "claude-sonnet-5": (2, 2.50, 4, 0.20, 10),
    "claude-sonnet-4-6": (3, 3.75, 6, 0.30, 15),
    "claude-haiku-5-5": (0.10, 0.125, 0.20, 0.01, 0.50),
    "claude-opus-4-5": (5, 6.25, 10, 0.50, 25),
    "claude-sonnet-4-5": (3, 3.75, 6, 0.30, 15),
    "claude-haiku-4-5-20251001": (1, 1.25, 2, 0.10, 5),
    "claude-haiku-4-5": (1, 1.25, 2, 0.10, 5),
}
# Long-prompt rates: (above this many input tokens, rates).
LONG = {"claude-haiku-5-5": (100_000, (0.50, 0.625, 1, 0.05, 2.50))}
FAST = {
    "claude-opus-5-5": 2,
    "claude-opus-5": 2,
    "claude-opus-4-8": 2,
}  # others: no fast price
NO_US_SURCHARGE = {
    "claude-opus-4-5",
    "claude-sonnet-4-5",
    "claude-haiku-4-5-20251001",
    "claude-haiku-4-5",
}
US_SURCHARGE = 1.1

CLASSES = ("input_uncached", "cache_read", "cache_write_5m", "cache_write_1h", "output")
THRESHOLD = 200_000  # P19
STALE = timedelta(minutes=10)  # reconciliation: ADR 0033, 0035
BYTES_PER_TOKEN = 4
SYNTHETIC = "<synthetic>"


def resolve(model):
    """The catalogue id a logged model id names, or None (measurements.md §1):
    as written; else without a region and 'anthropic.' prefix and a Bedrock
    '-v1:0' version; else also without a '-YYYYMMDD' or '@YYYYMMDD' date."""
    if not model:
        return None
    if model in RATES:
        return model
    m = model
    for region in ("us.", "eu.", "apac.", "global."):
        if m.startswith(region):
            m = m[len(region) :]
            break
    m = m.removeprefix("anthropic.")
    head, sep, tail = m.rpartition("-v")
    if sep and ":" in tail and all(p.isdigit() and p for p in tail.split(":", 1)):
        m = head
    if m in RATES:
        return m
    if len(m) > 9 and m[-9] in "-@" and m[-8:].isdigit():
        m = m[:-9]
    return m if m in RATES else None


def price(model, tok, fast, us):
    """Cost by class, and whether the turn could be priced."""
    if sum(tok) == 0:
        return [0.0] * 5, True  # nothing sent: free whatever the model
    key = resolve(model)
    if key is None:
        return [0.0] * 5, False
    r = RATES[key]
    if key in LONG and sum(tok[:4]) > LONG[key][0]:
        r = LONG[key][1]
    mult = 1.0
    if fast:
        if key not in FAST:
            return [0.0] * 5, False
        mult *= FAST[key]
    if us and key not in NO_US_SURCHARGE:
        mult *= US_SURCHARGE
    # tok is (uncached, read, 5m, 1h, output); r is (input, 5m, 1h, read, output)
    rate = (r[0], r[3], r[1], r[2], r[4])
    return [tok[i] * rate[i] * mult / 1e6 for i in range(5)], True


# ---------------------------------------------------------------------------
# Reading


def norm_usage(u):
    """docs/data-model.md, Anthropic Messages row: (uncached, read, 5m, 1h,
    output). The split by lifetime when given; else the write total is 5m."""
    cc = u.get("cache_creation")
    if isinstance(cc, dict):
        w5 = cc.get("ephemeral_5m_input_tokens") or 0
        w1 = cc.get("ephemeral_1h_input_tokens") or 0
    else:
        w5, w1 = u.get("cache_creation_input_tokens") or 0, 0
    return [
        u.get("input_tokens") or 0,
        u.get("cache_read_input_tokens") or 0,
        w5,
        w1,
        u.get("output_tokens") or 0,
    ]


def reply_tokens(u):
    """A reply's tokens; a reply whose top-level counts are all zero is
    counted from its steps (iterations), ADR 0035."""
    top = [
        u.get(k) or 0
        for k in (
            "input_tokens",
            "cache_read_input_tokens",
            "cache_creation_input_tokens",
            "output_tokens",
        )
    ]
    steps = u.get("iterations") or []
    if sum(top) == 0 and steps:
        tok = [0] * 5
        for s in steps:
            tok = [a + b for a, b in zip(tok, norm_usage(s))]
        return tok
    return norm_usage(u)


def when(s):
    try:
        return datetime.fromisoformat(s.replace("Z", "+00:00"))
    except (AttributeError, ValueError):
        return None


def snapshot_tokens(att):
    """The fixed start a prompt snapshot records (system prompt and tool
    definitions), at 4 bytes per token, rounded up."""
    snap = att.get("snapshot") if isinstance(att.get("snapshot"), dict) else att
    n = sum(
        len(p.encode()) for p in snap.get("systemPrompt") or [] if isinstance(p, str)
    )
    for t in snap.get("tools") or []:
        n += len((t.get("name") or "").encode()) + len(
            (t.get("description") or "").encode()
        )
        if t.get("schema") is not None:
            n += len(
                json.dumps(
                    t["schema"], separators=(",", ":"), ensure_ascii=False
                ).encode()
            )
    return math.ceil(n / BYTES_PER_TOKEN)


class Log:
    def __init__(self, path, projects):
        rel = os.path.relpath(path, projects).split(os.sep)
        self.path = path
        self.sub = len(rel) == 4  # <folder>/<session>/subagents/agent-*.jsonl
        stem = rel[-1][: -len(".jsonl")]
        self.conv = rel[1] if self.sub else stem
        self.id = f"{rel[1]}/{stem}" if self.sub else stem
        self.replies = {}  # message id -> reply, in first-seen order
        self.compactions = []  # (line uuid, trigger, time)
        self.records = []  # (startTime, totalCostUSD, unknown, date of the last reply before it)
        self.first_cwd = None
        self.times = []
        self.bad = 0
        self.read()

    def read(self):
        fixed = 0  # tokens in the latest prompt snapshot
        last_reply = None
        with open(self.path, encoding="utf-8", errors="replace") as f:
            for n, raw in enumerate(f):
                raw = raw.strip()
                if not raw:
                    continue
                try:
                    o = json.loads(raw)
                except ValueError:
                    self.bad += 1
                    continue
                if not isinstance(o, dict):
                    self.bad += 1
                    continue
                t = when(o.get("timestamp"))
                if t:
                    self.times.append(t)
                if self.first_cwd is None and o.get("cwd"):
                    self.first_cwd = o["cwd"]
                kind = o.get("type")
                if (
                    kind == "attachment"
                    and (o.get("attachment") or {}).get("type") == "prompt_snapshot"
                ):
                    fixed = snapshot_tokens(o["attachment"])
                elif kind == "system" and o.get("subtype") == "compact_boundary":
                    self.compactions.append(
                        (
                            o.get("uuid") or f"{self.path}:{n}",
                            (o.get("compactMetadata") or {}).get("trigger"),
                            t,
                        )
                    )
                elif kind == "cost-state" and o.get("totalCostUSD") is not None:
                    self.records.append(
                        (
                            o.get("startTime"),
                            float(o["totalCostUSD"]),
                            bool(o.get("hasUnknownModelCost")),
                            last_reply,
                        )
                    )
                elif kind == "assistant":
                    m = o.get("message") or {}
                    u = m.get("usage")
                    if (
                        not isinstance(u, dict)
                        or m.get("model") == SYNTHETIC
                        or o.get("isApiErrorMessage")
                    ):
                        continue  # not a request to the model: a made-up or error reply
                    mid = (
                        m.get("id")
                        or f"line:{o.get('uuid') or self.path + ':' + str(n)}"
                    )
                    tok = reply_tokens(u)
                    prev = self.replies.get(mid)
                    if prev is None:
                        self.replies[mid] = dict(
                            id=mid,
                            model=m.get("model"),
                            tok=tok,
                            time=t,
                            cwd=o.get("cwd"),
                            fast=u.get("speed") == "fast",
                            us=u.get("inference_geo") == "us",
                            fixed=fixed,
                            log=self,
                        )
                    elif sum(tok) > sum(prev["tok"]):
                        prev["tok"] = tok  # the fullest line of a streamed reply counts
                    if t and (last_reply is None or t > last_reply):
                        last_reply = t

    def span(self):
        return (min(self.times), max(self.times)) if self.times else (None, None)


def discover(cfg):
    projects = os.path.join(cfg, "projects")
    paths = glob.glob(os.path.join(projects, "*", "*.jsonl"))
    paths += glob.glob(os.path.join(projects, "*", "*", "subagents", "*.jsonl"))
    return projects, sorted(paths)


# ---------------------------------------------------------------------------
# Projects (measurements.md §1, ADR 0053)

WORKTREES = "/.claude/worktrees/"


def project_folder(cwd):
    if not cwd:
        return None
    i = cwd.find(WORKTREES)
    if i > 0:
        cwd = cwd[:i]
    return os.path.normpath(cwd)


def inside(path, folder):
    return path == folder or path.startswith(folder.rstrip("/") + "/")


def short_names(folders):
    """Each folder's name, with as many parents as tell same-named folders
    apart; a folder with no parent left keeps the name it has."""
    parts = {f: [p for p in f.split("/") if p] for f in folders}
    names = {}
    depth = 1
    left = set(folders)
    while left:
        by = defaultdict(list)
        for f in left:
            by["/".join(parts[f][-depth:])].append(f)
        for name, fs in by.items():
            for f in fs:
                if len(fs) == 1 or len(parts[f]) <= depth:
                    names[f] = name
                    left.discard(f)
        depth += 1
    return names


# ---------------------------------------------------------------------------


def monday(t, tz):
    d = t.astimezone(tz).date()
    return (d - timedelta(days=d.weekday())).isoformat()


def recount(cfg, tz):
    projects, paths = discover(cfg)
    logs = [Log(p, projects) for p in paths]

    # Each reply once, in the oldest log that has it (ADR 0026): the log
    # whose first line is earliest; a resumed log copies the original's
    # lines with their times, so on a tie the one that ended first.
    def age(log):
        a, b = log.span()
        far = datetime.max.replace(tzinfo=timezone.utc)
        return (a or far, b or far, log.path)

    seen = set()
    turns = []
    for log in sorted(logs, key=age):
        for r in log.replies.values():
            if r["id"] in seen:
                continue
            seen.add(r["id"])
            r["cost"], r["priced"] = price(r["model"], r["tok"], r["fast"], r["us"])
            r["total"] = sum(r["cost"])
            turns.append(r)
    with_turns = {r["log"].id: r["log"] for r in turns}

    R = {"tz": getattr(tz, "key", None) or str(tz), "prices": RATES_VERSION}
    R["read"] = {
        "logs": len(logs),
        "bad_lines": sum(log.bad for log in logs),
        "sessions": len(with_turns),
        "turns": len(turns),
        "unpriced": sum(1 for r in turns if not r["priced"]),
    }
    tok = [sum(r["tok"][i] for r in turns) for i in range(5)]
    cost = [sum(r["cost"][i] for r in turns) for i in range(5)]
    R["tokens"] = dict(zip(CLASSES, tok))
    R["cost"] = dict(zip(CLASSES, cost))
    R["total"] = sum(cost)

    # By model, grouped by the catalogue model an id names (an unknown id as
    # written). by_model has every turn; main_by_model is P13's table of
    # main-session turns, which prices turns again on another model, so
    # it holds priced turns only.
    by_model = defaultdict(lambda: {"turns": 0, "cost": 0.0})
    main_models = defaultdict(lambda: {"turns": 0, "cost": 0.0})
    sub_models = defaultdict(lambda: {"turns": 0, "cost": 0.0})
    for r in turns:
        key = resolve(r["model"]) or r["model"]
        groups = [by_model]
        if r["log"].sub:
            groups.append(sub_models)
        elif r["priced"]:
            groups.append(main_models)
        for group in groups:
            group[key]["turns"] += 1
            group[key]["cost"] += r["total"]
    R["by_model"] = dict(sorted(by_model.items()))
    R["main_by_model"] = dict(sorted(main_models.items()))
    R["sub_agents"] = {
        "runs": sum(1 for log in with_turns.values() if log.sub),
        "turns": sum(1 for r in turns if r["log"].sub),
        "cost": sum(r["total"] for r in turns if r["log"].sub),
        "by_model": dict(sorted(sub_models.items())),
    }
    per_session = defaultdict(lambda: {"turns": 0, "cost": 0.0})
    for r in turns:
        per_session[r["log"].id]["turns"] += 1
        per_session[r["log"].id]["cost"] += r["total"]
    R["sessions"] = dict(sorted(per_session.items()))

    weeks = defaultdict(lambda: {"spend": 0.0, "turns": 0, "compactions": 0})
    for r in turns:
        if r["time"]:
            w = weeks[monday(r["time"], tz)]
            w["spend"] += r["total"]
            w["turns"] += 1

    # Period: whole 24-hour spans from the first line to the last, plus one.
    first = min(t for log in logs for t in log.times[:1])
    last = max(t for log in logs for t in log.times[-1:])
    days = int((last - first).total_seconds() // 86400) + 1
    R["period"] = {
        "days": days,
        "per_week": R["total"] / (days / 7) if days >= 7 else 0.0,
    }

    # Compactions: each marker once by line id, wherever copied.
    comp = {}
    for log in sorted(logs, key=age):
        for line_id, trigger, t in log.compactions:
            comp.setdefault(line_id, (trigger, t))
    R["compactions"] = {
        "total": len(comp),
        "manual": sum(1 for tr, _ in comp.values() if tr == "manual"),
    }
    for _, t in comp.values():
        if t:  # in the week it happened
            weeks[monday(t, tz)]["compactions"] += 1

    # P19: turns over 200k. Cost: their input cost. Saving: the tokens above
    # 200k as removed history, placed just after the fixed start and priced
    # by where they sit in the request: read, written, or uncached.
    p19 = {"turns": 0, "sessions": set(), "cost": 0.0, "saving": 0.0}
    for r in turns:
        u, rd, w5, w1, _ = r["tok"]
        ctx = u + rd + w5 + w1
        if ctx <= THRESHOLD or not r["priced"]:
            continue
        n = ctx - THRESHOLD
        lo = min(r["fixed"], ctx - n)
        c = r["cost"]

        def overlap(a, b):
            return max(0, min(b, lo + n) - max(a, lo))

        s = 0.0
        if rd:
            s += overlap(0, rd) * c[1] / rd
        if w5 + w1:
            s += overlap(rd, rd + w5 + w1) * (c[2] + c[3]) / (w5 + w1)
        if u:
            s += overlap(rd + w5 + w1, ctx) * c[0] / u
        p19["turns"] += 1
        p19["sessions"].add(r["log"].id)
        p19["cost"] += sum(c[:4])
        p19["saving"] += s
    p19["sessions"] = len(p19["sessions"])
    R["p19"] = p19

    # Spend by project: the conversation's folder (where its main session
    # started) when a turn ran there, below it or above it; else the folder
    # the turn ran in.
    # A main session starts where its first reply counted in its own log
    # ran: a resumed or forked log begins with lines copied from an older
    # one, in the older session's folder, and those count there.
    home = {}
    for r in sorted(turns, key=lambda r: (r["time"] is None, r["time"] or 0)):
        log = r["log"]
        if not log.sub and r["cwd"] and log.conv not in home:
            home[log.conv] = project_folder(r["cwd"])
    for log in logs:
        if not log.sub and log.first_cwd and log.conv not in home:
            home[log.conv] = project_folder(log.first_cwd)
    folders = {}
    for r in turns:
        log = r["log"]
        here = project_folder(r["cwd"])
        base = home.get(log.conv) or project_folder(
            log.first_cwd
        )  # no main log: the sub-agent's own
        if here is None or base is None:
            folders[id(r)] = here or base
        elif inside(here, base) or inside(base, here):
            folders[id(r)] = base
        else:
            folders[id(r)] = here
    names = short_names({f for f in folders.values() if f})
    projects_out = defaultdict(
        lambda: {"cost": 0.0, "turns": 0, "conversations": set(), "sub_agent_cost": 0.0}
    )
    for r in turns:
        f = folders[id(r)]
        p = projects_out[names[f] if f else "(unknown)"]
        p["cost"] += r["total"]
        p["turns"] += 1
        p["conversations"].add(r["log"].conv)
        if r["log"].sub:
            p["sub_agent_cost"] += r["total"]
    for p in projects_out.values():
        p["conversations"] = len(p["conversations"])
    R["projects"] = dict(sorted(projects_out.items()))

    # Reconciliation (ADR 0033, 0035): each main session's records against
    # its conversation's logged cost. A run's records are running totals:
    # its latest counts, runs are added. A record is dated by the last reply
    # logged before it. Compared only when covered: no turn more than 10
    # minutes after the latest record, none more than 10 minutes before the
    # earliest run began.
    conv = defaultdict(
        lambda: {"cost": 0.0, "first": None, "last": None, "unpriced": False}
    )
    for r in turns:
        c = conv[r["log"].conv]
        c["cost"] += r["total"]
        c["unpriced"] |= not r["priced"]
        if r["time"]:
            c["first"] = min(c["first"] or r["time"], r["time"])
            c["last"] = max(c["last"] or r["time"], r["time"])
    rec = {k: 0 for k in ("sessions", "behind", "unrecorded", "unpriced")}
    rec.update({k: 0.0 for k in ("harness", "ours", "behind_cost", "unrecorded_cost")})
    main_ids = set()
    for log in logs:
        if log.sub or log.id not in with_turns:
            continue
        main_ids.add(log.conv)
        c = conv[log.conv]
        if not log.records:
            rec["unrecorded"] += 1
            rec["unrecorded_cost"] += c["cost"]
            continue
        runs = {}
        for start, total, unknown, dated in log.records:
            runs[start] = (total, unknown, dated)  # the latest record of each run
        if c["unpriced"] or any(u for _, u, _ in runs.values()):
            rec["unpriced"] += 1
            continue
        dated = max((d for _, _, d in runs.values() if d), default=None)
        starts = [
            datetime.fromtimestamp(s / 1000, timezone.utc)
            for s in runs
            if isinstance(s, (int, float))
        ]
        behind = (dated and c["last"] and c["last"] - dated > STALE) or (
            starts and c["first"] and min(starts) - c["first"] > STALE
        )
        if behind:
            rec["behind"] += 1
            rec["behind_cost"] += c["cost"]
            continue
        rec["sessions"] += 1
        rec["harness"] += sum(total for total, _, _ in runs.values())
        rec["ours"] += c["cost"]
    for cid, c in sorted(conv.items()):
        if cid not in main_ids:  # sub-agents whose main session's log is gone
            rec["unrecorded"] += 1
            rec["unrecorded_cost"] += c["cost"]
    rec["gap"] = rec["harness"] - rec["ours"]
    R["reconciliation"] = rec

    R["weeks"] = dict(sorted(weeks.items()))
    return R


# ---------------------------------------------------------------------------
# Comparison with trimloop's JSON


def theirs(T):
    """The figures this script works out, as trimloop's JSON gives them."""
    out = {}
    rd = T.get("read", {})
    for k in ("logs", "bad_lines", "sessions", "turns", "unpriced"):
        out[f"read.{k}"] = rd.get(k)
    for k in CLASSES:
        out[f"cost.{k}"] = T.get("cost", {}).get(k)
    out["total"] = T.get("total")
    for m in T.get("p13_main_models", {}).get("by_model") or []:
        out[f"main_by_model.{m['model']}.turns"] = m.get("turns")
        out[f"main_by_model.{m['model']}.cost"] = m.get("cost")
    sa = T.get("p13_sub_agents", {})
    for k in ("runs", "turns", "cost"):
        out[f"sub_agents.{k}"] = sa.get(k)
    for w in T.get("weeks") or []:
        out[f"weeks.{w['week']}.spend"] = w.get("spend")
        out[f"weeks.{w['week']}.turns"] = w.get("turns")
        out[f"weeks.{w['week']}.compactions"] = w.get("compactions")
    p = T.get("period", {})
    out["period.days"] = p.get("days")
    out["period.per_week"] = p.get("per_week")
    g = T.get("p19_growth", {})
    out["compactions.total"] = g.get("compactions")
    out["compactions.manual"] = g.get("manual")
    c = T.get("p19_context", {})
    for k in ("turns", "sessions", "cost", "saving"):
        out[f"p19.{k}"] = c.get(k)
    rc = T.get("reconciliation", {})
    for k in (
        "sessions",
        "harness",
        "ours",
        "gap",
        "behind",
        "behind_cost",
        "unrecorded",
        "unrecorded_cost",
        "unpriced",
    ):
        out[f"reconciliation.{k}"] = rc.get(k)
    for s in T.get("attribution", {}).get("by_project") or []:
        for k in ("cost", "turns", "conversations", "sub_agent_cost"):
            out[f"projects.{s['name']}.{k}"] = s.get(k)
    return out


def ours(R):
    out = {}
    for k, v in R["read"].items():
        out[f"read.{k}"] = v
    for k, v in R["cost"].items():
        out[f"cost.{k}"] = v
    out["total"] = R["total"]
    for m, v in R["main_by_model"].items():
        out[f"main_by_model.{m}.turns"] = v["turns"]
        out[f"main_by_model.{m}.cost"] = v["cost"]
    for k in ("runs", "turns", "cost"):
        out[f"sub_agents.{k}"] = R["sub_agents"][k]
    for w, v in R["weeks"].items():
        out[f"weeks.{w}.spend"] = v["spend"]
        out[f"weeks.{w}.turns"] = v["turns"]
        out[f"weeks.{w}.compactions"] = v["compactions"]
    out["period.days"] = R["period"]["days"]
    out["period.per_week"] = R["period"]["per_week"]
    out["compactions.total"] = R["compactions"]["total"]
    out["compactions.manual"] = R["compactions"]["manual"]
    for k, v in R["p19"].items():
        out[f"p19.{k}"] = v
    for k, v in R["reconciliation"].items():
        out[f"reconciliation.{k}"] = v
    for name, v in R["projects"].items():
        for k in ("cost", "turns", "conversations", "sub_agent_cost"):
            out[f"projects.{name}.{k}"] = v[k]
    return out


def compare(R, T, tol):
    a, b = ours(R), theirs(T)
    # trimloop lists its 10 largest projects only.
    shown = len(T.get("attribution", {}).get("by_project") or [])
    bad = 0
    width = max(len(k) for k in set(a) | set(b))
    print(f"{'figure':<{width}}  {'recount':>14}  {'trimloop':>14}")
    for k in sorted(set(a) | set(b)):
        x, y = a.get(k), b.get(k)
        if y is None and k.startswith("projects.") and shown >= 10:
            continue
        if isinstance(x, float) or isinstance(y, float):
            ok = x is not None and y is not None and abs(x - y) <= tol
            fx = "-" if x is None else f"{x:.4f}"
            fy = "-" if y is None else f"{y:.4f}"
        else:
            ok = x == y
            fx, fy = str(x if x is not None else "-"), str(y if y is not None else "-")
        bad += not ok
        print(f"{k:<{width}}  {fx:>14}  {fy:>14}{'' if ok else '   <-- differs'}")
    print(
        f"\n{bad} difference(s) above ${tol} (counts exact). Not in trimloop's JSON, so not compared: "
        "tokens, by_model (all turns), sub_agents.by_model, sessions."
    )
    return bad


def rounded(v):
    """v with dollar figures rounded to 9 places, for a stable, readable file."""
    if isinstance(v, float):
        return round(v, 9)
    if isinstance(v, dict):
        return {k: rounded(x) for k, x in v.items()}
    return v


def main():
    ap = argparse.ArgumentParser(
        description="Recount trimloop's figures from Claude Code logs, independently."
    )
    ap.add_argument(
        "config_dir", help="the CLAUDE_CONFIG_DIR whose projects folder holds the logs"
    )
    ap.add_argument("--json", metavar="FILE", help="write the figures to FILE as JSON")
    ap.add_argument(
        "--compare",
        metavar="FILE",
        help="trimloop --format json output to compare with",
    )
    ap.add_argument(
        "--tz",
        default="UTC",
        help="time zone for calendar weeks (IANA name; default UTC)",
    )
    ap.add_argument(
        "--tolerance",
        type=float,
        default=0.005,
        help="dollars a figure may differ by (0.005)",
    )
    a = ap.parse_args()
    if a.tz == "UTC":
        tz = timezone.utc
    else:
        from zoneinfo import ZoneInfo

        tz = ZoneInfo(a.tz)
    R = recount(a.config_dir, tz)
    if a.json:
        with open(a.json, "w", newline="\n") as f:
            json.dump(rounded(R), f, indent=1, sort_keys=True)
            f.write("\n")
    if a.compare:
        with open(a.compare) as f:
            sys.exit(1 if compare(R, json.load(f), a.tolerance) else 0)
    if not a.json:
        json.dump(rounded(R), sys.stdout, indent=1, sort_keys=True)
        print()


if __name__ == "__main__":
    main()
