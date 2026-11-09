#!/usr/bin/env python3
"""Write the synthetic Claude Code logs of the golden end-to-end test.

    python3 -I scripts/gen-golden.py [--out testdata/golden]

The logs imitate what Claude Code writes under <CLAUDE_CONFIG_DIR>/projects:
<encoded-folder>/<session>.jsonl for main sessions and
<encoded-folder>/<session>/subagents/agent-*.jsonl for sub-agents. Every
name, folder and text is made up; no real log is copied. The output is
deterministic: a fixed seed, fixed dates, no clock, so running it twice
gives the same bytes.

testdata/golden/README.md lists what the logs cover. The expected figures
are not written here: scripts/recount.py works them out from these logs.
"""

import argparse
import json
import os
import random
import shutil
import uuid
from datetime import datetime, timedelta, timezone

SEED = 20261109
VERSION = "2.1.318"
NS = uuid.UUID("6f1c2d3e-4b5a-4c6d-8e7f-0a1b2c3d4e5f")  # for name-based ids

rng = random.Random(SEED)
_counter = [0]


def next_id(kind):
    """A deterministic id: a UUID made from a counter."""
    _counter[0] += 1
    return str(uuid.uuid5(NS, f"{kind}-{_counter[0]}"))


def msg_id():
    return "msg_01" + next_id("msg").replace("-", "")[:22]


def iso(t):
    return t.strftime("%Y-%m-%dT%H:%M:%S.") + f"{t.microsecond // 1000:03d}Z"


def at(s):
    """A UTC time from 'YYYY-MM-DD HH:MM[:SS]'."""
    fmt = "%Y-%m-%d %H:%M:%S" if s.count(":") == 2 else "%Y-%m-%d %H:%M"
    return datetime.strptime(s, fmt).replace(tzinfo=timezone.utc)


WORDS = ["alpha", "beta", "gamma", "delta", "parse", "build", "check", "order", "queue", "cache", "table", "row", "column", "value", "field", "index", "token", "buffer", "stream", "render", "layout", "report", "widget", "handler", "config", "option", "flag", "module", "package", "import", "export", "return", "error", "result"]


def filler(n):
    """About n bytes of made-up words."""
    out = []
    size = 0
    while size < n:
        w = rng.choice(WORDS)
        out.append(w)
        size += len(w) + 1
    return " ".join(out)[:n]


def encode(folder):
    """Claude Code's folder name for a working folder: / and . become -."""
    return folder.replace("/", "-").replace(".", "-")


def usage(
    read=0,
    w5=0,
    w1=0,
    unc=3,
    out=200,
    geo=None,
    speed=None,
    split=True,
    iterations=None,
):
    u = {
        "input_tokens": unc,
        "cache_creation_input_tokens": w5 + w1,
        "cache_read_input_tokens": read,
    }
    if split:
        u["cache_creation"] = {
            "ephemeral_5m_input_tokens": w5,
            "ephemeral_1h_input_tokens": w1,
        }
    u["output_tokens"] = out
    u["server_tool_use"] = {"web_search_requests": 0, "web_fetch_requests": 0}
    u["service_tier"] = "standard"
    if geo:
        u["inference_geo"] = geo
    if speed:
        u["speed"] = speed
    if iterations is not None:
        u["iterations"] = iterations
    return u


class Log:
    """One log file being written."""

    def __init__(self, root, folder, session, cwd, agent=None, agent_type=None):
        self.session, self.cwd, self.agent = session, cwd, agent
        d = os.path.join(root, "projects", encode(folder))
        if agent:
            d = os.path.join(d, session, "subagents")
            self.path = os.path.join(d, f"agent-{agent}.jsonl")
        else:
            self.path = os.path.join(d, f"{session}.jsonl")
        self.meta = None
        if agent_type:
            self.meta = (
                os.path.join(d, f"agent-{agent}.meta.json"),
                {"agentType": agent_type, "spawnDepth": 1},
            )
        self.lines = []
        self.parent = None
        self.ctx = 0  # tokens the next request re-sends from the cache
        self.life = "1h"  # cache lifetime this log writes with

    def line(self, typ, t, body, cwd=None, extra=None):
        d = {
            "parentUuid": self.parent,
            "isSidechain": bool(self.agent),
            "userType": "external",
            "cwd": cwd or self.cwd,
            "sessionId": self.session,
            "version": VERSION,
            "gitBranch": "main",
        }
        if self.agent:
            d["agentId"] = self.agent
        d["type"] = typ
        d.update(body)
        if extra:
            d.update(extra)
        d["uuid"] = next_id("line")
        d["timestamp"] = iso(t)
        self.parent = d["uuid"]
        self.lines.append(d)
        return d

    def prompt(self, t, text=None, cwd=None):
        return self.line(
            "user",
            t,
            {
                "message": {"role": "user", "content": text or filler(120)},
                "origin": {"kind": "human"},
            },
            cwd=cwd,
        )

    def snapshot(self, t, system_bytes, tools):
        tl = [
            {
                "name": n,
                "description": filler(db),
                "schema": {
                    "type": "object",
                    "properties": {
                        "arg": {"type": "string", "description": filler(sb)}
                    },
                },
            }
            for n, db, sb in tools
        ]
        return self.line(
            "attachment",
            t,
            {
                "attachment": {
                    "type": "prompt_snapshot",
                    "systemPrompt": [filler(system_bytes)],
                    "tools": tl,
                }
            },
        )

    def reply(self, t, model, u, cwd=None, partial=None, tool=None, extra=None):
        """An assistant reply. partial: output tokens on an earlier streamed
        line of the same message (logged first, with the same id); tool: a
        Bash command run by the reply, answered by a tool result."""
        mid = msg_id()
        blocks = [{"type": "text", "text": filler(80)}]
        lines = []
        if partial is not None:
            p = json.loads(json.dumps(u))
            p["output_tokens"] = partial
            lines.append(
                ([{"type": "thinking", "thinking": "", "signature": filler(40)}], p, t)
            )
        tid = None
        if tool:
            tid = "toolu_01" + next_id("tool").replace("-", "")[:22]
            lines.append((blocks, u, t + timedelta(seconds=1)))
            lines.append(
                (
                    [
                        {
                            "type": "tool_use",
                            "id": tid,
                            "name": "Bash",
                            "input": {"command": tool},
                        }
                    ],
                    u,
                    t + timedelta(seconds=2),
                )
            )
        else:
            lines.append((blocks, u, t + timedelta(seconds=1)))
        for content, uu, tt in lines:
            self.line(
                "assistant",
                tt,
                {
                    "message": {
                        "id": mid,
                        "type": "message",
                        "role": "assistant",
                        "model": model,
                        "content": content,
                        "stop_reason": "tool_use" if tid else "end_turn",
                        "stop_sequence": None,
                        "usage": uu,
                    },
                    "requestId": "req_01" + next_id("req").replace("-", "")[:22],
                },
                cwd=cwd,
                extra=extra,
            )
        if tid:
            self.line(
                "user",
                t + timedelta(seconds=5),
                {
                    "message": {
                        "role": "user",
                        "content": [
                            {
                                "type": "tool_result",
                                "tool_use_id": tid,
                                "is_error": False,
                                "content": filler(600),
                            }
                        ],
                    },
                    "toolUseResult": {"stdout": "", "stderr": "", "interrupted": False},
                },
                cwd=cwd,
            )
        return mid

    def grow(self, t, model, new, out=300, cwd=None, **kw):
        """A turn that reads the whole earlier context from the cache and
        writes `new` tokens with this log's cache lifetime."""
        w5, w1 = (new, 0) if self.life == "5m" else (0, new)
        unc = rng.randint(1, 9)
        u = usage(
            read=self.ctx,
            w5=w5,
            w1=w1,
            unc=unc,
            out=out,
            geo=kw.pop("geo", None),
            speed=kw.pop("speed", None),
        )
        self.ctx += new + unc + out
        return self.reply(t, model, u, cwd=cwd, **kw)

    def compact(self, t, trigger, pre, post, ms, after_ctx):
        self.line(
            "system",
            t,
            {
                "subtype": "compact_boundary",
                "content": "Conversation compacted",
                "isMeta": False,
                "level": "info",
                "compactMetadata": {
                    "trigger": trigger,
                    "preTokens": pre,
                    "postTokens": post,
                    "durationMs": ms,
                },
            },
        )
        self.line(
            "user",
            t + timedelta(seconds=1),
            {
                "message": {"role": "user", "content": filler(400)},
                "isCompactSummary": True,
                "isVisibleInTranscriptOnly": True,
            },
        )
        self.ctx = after_ctx

    def synthetic(self, t):
        self.line(
            "assistant",
            t,
            {
                "message": {
                    "id": msg_id(),
                    "type": "message",
                    "role": "assistant",
                    "model": "<synthetic>",
                    "content": [{"type": "text", "text": "No response requested."}],
                    "stop_reason": "stop_sequence",
                    "stop_sequence": "",
                    "usage": usage(unc=0, out=0),
                }
            },
        )

    def slept(self, t):
        self.line(
            "assistant",
            t,
            {
                "message": {
                    "id": msg_id(),
                    "type": "message",
                    "role": "assistant",
                    "model": "<synthetic>",
                    "content": [
                        {
                            "type": "text",
                            "text": "API Error: Your computer went to sleep mid-response.",
                        }
                    ],
                    "stop_reason": "stop_sequence",
                    "stop_sequence": "",
                    "usage": usage(unc=0, out=0),
                },
                "isApiErrorMessage": True,
                "error": "unknown",
            },
        )

    def cost_state(self, t, start, total, model_usage):
        d = {
            "type": "cost-state",
            "sessionId": self.session,
            "timestamp": iso(t),
            "startTime": int(start.timestamp() * 1000),
            "totalCostUSD": total,
            "totalAPIDuration": 60_000,
            "totalAPIDurationWithoutRetries": 60_000,
            "totalToolDuration": 9_000,
            "totalDuration": 600_000,
            "totalLinesAdded": 12,
            "totalLinesRemoved": 3,
            "modelUsage": {},
        }
        for model, (inp, out, rd, wr, cost) in model_usage.items():
            d["modelUsage"][model] = {
                "inputTokens": inp,
                "outputTokens": out,
                "cacheReadInputTokens": rd,
                "cacheCreationInputTokens": wr,
                "costUSD": cost,
                "webSearchRequests": 0,
            }
        self.lines.append(d)

    def write(self):
        os.makedirs(os.path.dirname(self.path), exist_ok=True)
        with open(self.path, "w", newline="\n") as f:
            f.writelines(json.dumps(d, separators=(",", ":"), ensure_ascii=False) + "\n" for d in self.lines)
        if self.meta:
            with open(self.meta[0], "w", newline="\n") as f:
                f.write(json.dumps(self.meta[1], separators=(",", ":")) + "\n")


TOOLS = [
    ("Bash", 1500, 400),
    ("Read", 900, 300),
    ("Edit", 900, 300),
    ("Write", 600, 200),
    ("Grep", 1100, 300),
    ("Glob", 500, 150),
    ("Agent", 1800, 500),
    ("WebFetch", 700, 200),
]

ALPHA, BETA, ZETA = "/home/dev/alpha", "/home/dev/beta", "/home/dev/zeta"


def alpha_1(root):
    """Opus 5.5 on the hour's cache, past 200k, a fast turn, streamed lines,
    a subfolder, a long pause, an automatic compaction; one sub-agent."""
    s = Log(root, ALPHA, "alpha-1", ALPHA)
    t = at("2026-10-12 09:00")
    s.prompt(t)
    s.snapshot(t + timedelta(seconds=1), 15_000, TOOLS)
    t += timedelta(seconds=4)
    s.grow(t, "claude-opus-5-5", 14_000, out=420, tool="ls -la")
    for i, new in enumerate([21_000, 18_500, 24_000, 19_000, 22_500, 17_000, 23_000]):
        t += timedelta(minutes=2, seconds=rng.randint(0, 50))
        cwd = ALPHA + "/web" if 2 <= i <= 4 else None
        if i == 3:
            s.grow(
                t, "claude-opus-5-5", new, out=900, cwd=cwd, partial=12
            )  # streamed: partial line first
        elif i == 5:
            s.grow(
                t, "claude-opus-5-5", new, out=650, speed="fast"
            )  # fast mode: double rates
        else:
            s.grow(
                t,
                "claude-opus-5-5",
                new,
                out=rng.randint(200, 800),
                cwd=cwd,
                tool="sed -n '1,80p' web/app.js" if cwd else None,
            )
    t += timedelta(minutes=70)  # a long pause: past the hour's cache, so the context is written again
    s.prompt(t - timedelta(seconds=30))
    s.reply(t, "claude-opus-5-5", usage(read=0, w1=s.ctx + 2_000, unc=4, out=500))
    s.ctx += 2_504
    for new in (25_000, 21_000, 18_000):
        t += timedelta(minutes=3)
        s.grow(t, "claude-opus-5-5", new, out=rng.randint(300, 700), partial=8)
    t += timedelta(minutes=1)
    s.compact(t, "auto", s.ctx, 21_400, 31_000, after_ctx=0)
    t += timedelta(seconds=40)
    s.reply(t, "claude-opus-5-5", usage(read=0, w1=34_000, unc=5, out=600))
    s.ctx = 34_605
    t += timedelta(minutes=2)
    last = t
    s.grow(t, "claude-opus-5-5", 3_000, out=250)
    s.cost_state(
        last + timedelta(seconds=20),
        at("2026-10-12 08:59"),
        4.29,
        {
            "claude-opus-5-5": (120, 9_000, 2_000_000, 360_000, 4.2879),
            "claude-haiku-4-5-20251001": (1_800, 60, 0, 0, 0.0021),
        },
    )
    s.write()

    a = Log(
        root,
        ALPHA,
        "alpha-1",
        ALPHA + "/web",
        agent="a1explore01",
        agent_type="Explore",
    )
    a.life = "5m"
    t = at("2026-10-12 09:20")
    a.prompt(t, cwd=ALPHA + "/web")
    for new in (9_000, 6_000, 7_500, 5_000):
        t += timedelta(seconds=rng.randint(20, 90))
        a.grow(
            t, "claude-opus-5-5", new, out=rng.randint(150, 500), tool="grep -rn TODO ."
        )
    a.write()


def alpha_2_and_3(root):
    """Fable 5.1 with one US-only turn and a turn over 200k that rebuilt the
    cache after a long pause; a record written part way (out of date); a
    manual compaction. alpha-3 resumes it from another folder (zeta): the
    lines up to the compaction and the reply after it are copied into its
    log, in alpha's folder, then it goes on a week later on Sonnet 5, once
    from the folder above. Its project is zeta, where it started, not alpha,
    where the copied lines ran: the turn above zeta counts as zeta."""
    s = Log(root, ALPHA, "alpha-2", ALPHA)
    t = at("2026-10-13 14:00")
    s.prompt(t)
    s.snapshot(t + timedelta(seconds=1), 15_000, TOOLS)
    t += timedelta(seconds=5)
    s.grow(t, "claude-fable-5-1", 16_000, out=500)
    for i, new in enumerate([30_000, 28_000, 35_000, 32_000, 31_000, 30_000]):
        t += timedelta(minutes=3, seconds=rng.randint(0, 50))
        s.grow(
            t,
            "claude-fable-5-1",
            new,
            out=rng.randint(300, 900),
            geo="us" if i == 2 else None,
        )
    s.cost_state(
        t + timedelta(seconds=10),
        at("2026-10-13 13:59"),
        7.80,
        {"claude-fable-5-1": (60, 4_800, 700_000, 210_000, 7.80)},
    )
    t += timedelta(minutes=75)
    s.prompt(t - timedelta(seconds=20))
    # The cache expired: only the start of the request is read, the rest written again.
    s.reply(t, "claude-fable-5-1", usage(read=10_000, w1=214_000, unc=3, out=700))
    s.ctx = 224_703
    t += timedelta(minutes=4)
    s.prompt(t - timedelta(seconds=10), text="/compact")
    s.compact(t, "manual", s.ctx, 19_800, 26_000, after_ctx=0)
    t += timedelta(seconds=30)
    s.prompt(t - timedelta(seconds=5))
    s.reply(t, "claude-fable-5-1", usage(read=0, w1=30_500, unc=4, out=400))
    s.ctx = 30_904
    copied = len(s.lines)
    t += timedelta(minutes=12)
    s.grow(t, "claude-fable-5-1", 4_000, out=300)
    s.write()

    r = Log(root, ZETA, "alpha-3", ZETA)
    for d in s.lines[:copied]:
        if d.get("type") == "cost-state":
            continue  # a run's cost record stays in its own log
        c = dict(d)
        c["sessionId"] = "alpha-3"
        r.lines.append(c)
    r.parent = r.lines[-1]["uuid"]
    r.ctx = 0
    t = at("2026-10-20 10:00")
    start = t - timedelta(seconds=10)
    r.prompt(t)
    t += timedelta(seconds=6)
    r.reply(t, "claude-sonnet-5", usage(read=0, w1=33_000, unc=6, out=450))
    r.ctx = 33_456
    for new, cwd in ((6_000, None), (8_000, "/home/dev"), (5_500, None)):
        t += timedelta(minutes=2)
        r.grow(t, "claude-sonnet-5", new, out=rng.randint(200, 600), cwd=cwd)
    r.cost_state(
        t + timedelta(seconds=15),
        start,
        0.27,
        {"claude-sonnet-5": (30, 1_900, 110_000, 52_000, 0.27)},
    )
    r.write()


def beta_1(root):
    """Sonnet 5.5 on the 5-minute cache: a made-up zero reply, a computer
    that slept, a reply priced from its steps, work in an unrelated folder
    and its subfolder; two sub-agents, one on Haiku 5.5 (long-prompt rates,
    past 200k, a compaction, an unrelated folder of its own) and one on a
    Bedrock Haiku 4.5 id with US-only inference."""
    s = Log(root, BETA, "beta-1", BETA)
    s.life = "5m"
    t = at("2026-10-21 10:00")
    s.prompt(t)
    s.snapshot(t + timedelta(seconds=1), 9_000, TOOLS[:5])
    t += timedelta(seconds=4)
    s.grow(t, "claude-sonnet-5-5", 11_000, out=380)
    t += timedelta(minutes=1)
    s.grow(t, "claude-sonnet-5-5", 7_000, out=250, tool="go test ./...")
    t += timedelta(seconds=30)
    s.synthetic(t)
    t += timedelta(minutes=2)
    s.prompt(t)
    t += timedelta(seconds=5)
    # The top level is zero; the steps carry the usage.
    steps = [
        usage(read=s.ctx, w5=4_000, unc=2, out=120),
        usage(read=s.ctx + 4_000, w5=1_500, unc=1, out=90),
    ]
    for st in steps:
        del st["server_tool_use"], st["service_tier"]
    s.reply(t, "claude-sonnet-5-5", usage(read=0, unc=0, out=0, iterations=steps))
    s.ctx += 4_000 + 1_500 + 3 + 210
    t += timedelta(minutes=2)
    s.slept(t)
    for cwd in ("/home/dev/gamma", "/home/dev/gamma", "/home/dev/gamma/tools", BETA):
        t += timedelta(minutes=2)
        s.grow(
            t,
            "claude-sonnet-5-5",
            rng.randint(3_000, 9_000),
            out=rng.randint(150, 500),
            cwd=cwd,
        )
    t += timedelta(minutes=50)  # waited for the sub-agents: the 5-minute cache expired
    s.reply(t, "claude-sonnet-5-5", usage(w5=s.ctx + 2_000, unc=6, out=300))
    s.cost_state(
        t + timedelta(seconds=30),
        at("2026-10-21 09:59"),
        0.49,
        {
            "claude-sonnet-5-5": (40, 3_900, 600_000, 90_000, 0.31),
            "claude-haiku-5-5": (60, 2_100, 700_000, 330_000, 0.135),
            "us.anthropic.claude-haiku-4-5-20251001-v1:0": (
                25,
                1_200,
                30_000,
                25_000,
                0.045,
            ),
        },
    )
    s.write()

    a = Log(
        root,
        BETA,
        "beta-1",
        "/home/dev/delta",
        agent="b1research01",
        agent_type="general-purpose",
    )
    a.life = "5m"
    t = at("2026-10-21 10:30")
    a.prompt(t)
    t += timedelta(seconds=5)
    a.grow(t, "claude-haiku-5-5", 40_000, out=400)
    t += timedelta(minutes=1)
    a.grow(t, "claude-haiku-5-5", 110_000, out=600)  # past 100k: long-prompt rates
    t += timedelta(minutes=1)
    a.reply(
        t, "claude-haiku-5-5", usage(read=150_603, w5=60_000, unc=7, out=800)
    )  # past 200k
    a.ctx = 211_410
    t += timedelta(minutes=1)
    a.compact(t, "auto", 211_410, 18_000, 22_000, after_ctx=0)
    t += timedelta(seconds=25)
    a.reply(t, "claude-haiku-5-5", usage(read=0, w5=29_000, unc=4, out=350))
    a.ctx = 29_354
    t += timedelta(minutes=1)
    a.grow(t, "claude-haiku-5-5", 4_000, out=200, cwd="/home/dev/delta/sub")
    a.write()

    p = Log(root, BETA, "beta-1", BETA, agent="b1plan01", agent_type="Plan")
    p.life = "5m"
    model = "us.anthropic.claude-haiku-4-5-20251001-v1:0"
    t = at("2026-10-21 10:40")
    p.prompt(t)
    t += timedelta(seconds=5)
    p.grow(t, model, 12_000, out=300, geo="us")
    t += timedelta(seconds=40)
    # An older record: the write total with no split by lifetime.
    p.reply(
        t, model, usage(read=p.ctx, w5=6_000, unc=5, out=280, geo="us", split=False)
    )
    p.ctx += 6_285
    t += timedelta(seconds=40)
    p.grow(t, model, 7_000, out=420, geo="us")
    p.write()


def beta_2(root):
    """Opus 4.8 in a worktree under .claude/worktrees, across the Sunday to
    Monday week boundary (UTC), with one reply from a model the catalogue
    does not hold; its record is left out as unpriced."""
    wt = BETA + "/.claude/worktrees/feat-x"
    s = Log(root, wt, "beta-2", wt)
    t = at("2026-10-25 23:30")
    s.prompt(t)
    t += timedelta(seconds=5)
    s.grow(t, "claude-opus-4-8", 15_000, out=500)
    for new in (8_000, 9_500):
        t += timedelta(minutes=9)
        s.grow(t, "claude-opus-4-8", new, out=rng.randint(200, 600))
    t += timedelta(minutes=9)  # 23:57 -> next turns on Monday
    s.reply(t, "claude-nova-1", usage(read=0, w5=2_000, unc=40, out=120))
    for new in (7_000, 6_000):
        t += timedelta(minutes=8)
        s.grow(t, "claude-opus-4-8", new, out=rng.randint(200, 600))
    s.cost_state(
        t + timedelta(seconds=20),
        at("2026-10-25 23:29"),
        0.60,
        {"claude-opus-4-8": (25, 2_000, 120_000, 47_000, 0.60)},
    )
    s.write()


def zeta_orphan(root):
    """A sub-agent whose main session's log is gone."""
    a = Log(root, "/home/dev/zeta", "zeta-1", "/home/dev/zeta", agent="z1orphan01")
    a.life = "5m"
    t = at("2026-10-27 16:00")
    a.prompt(t)
    for new in (10_000, 4_000, 6_000):
        t += timedelta(seconds=rng.randint(30, 120))
        a.grow(t, "claude-sonnet-5-5", new, out=rng.randint(150, 400))
    a.write()


def epsilon_0(root):
    """A log with no reply that counts: a prompt and a made-up reply."""
    s = Log(root, "/home/dev/epsilon", "epsilon-0", "/home/dev/epsilon")
    t = at("2026-10-28 08:00")
    s.prompt(t)
    s.synthetic(t + timedelta(seconds=2))
    s.write()


def work_alpha_1(root):
    """A second folder named alpha; Opus 5 in fast mode; Sonnet 5.5 in fast
    mode, which has no fast price (unpriced); no cost record; a manual
    compaction that no reply followed."""
    w = "/srv/work/alpha"
    s = Log(root, w, "work-alpha-1", w)
    t = at("2026-11-02 08:00")
    s.prompt(t)
    t += timedelta(seconds=5)
    s.grow(t, "claude-opus-5", 20_000, out=600, speed="fast")
    t += timedelta(minutes=2)
    s.grow(t, "claude-opus-5", 9_000, out=300)
    t += timedelta(minutes=2)
    s.grow(t, "claude-sonnet-5-5", 4_000, out=200, speed="fast")
    t += timedelta(minutes=2)
    s.grow(t, "claude-fable-5", 5_000, out=350)
    t += timedelta(minutes=1)
    s.prompt(t - timedelta(seconds=5), text="/compact")
    s.compact(t, "manual", s.ctx, 9_000, 15_000, after_ctx=0)
    s.write()


def epsilon_1(root):
    """Haiku 5.5 as the main model: long-prompt rates and a turn past 200k;
    a git worktree elsewhere (its own project); two runs, each with its own
    running cost record."""
    e = "/home/dev/epsilon"
    s = Log(root, e, "epsilon-1", e)
    s.life = "5m"
    t = at("2026-11-04 13:00")
    run1 = t - timedelta(minutes=1)
    s.prompt(t)
    s.snapshot(t + timedelta(seconds=1), 5_000, TOOLS[:3])
    t += timedelta(seconds=5)
    s.grow(t, "claude-haiku-5-5", 60_000, out=500)
    s.cost_state(
        t + timedelta(seconds=5),
        run1,
        0.05,
        {"claude-haiku-5-5": (5, 500, 0, 60_000, 0.05)},
    )
    t += timedelta(minutes=2)
    s.grow(t, "claude-haiku-5-5", 90_000, out=700)  # 150k: long-prompt rates
    t += timedelta(minutes=2)
    s.grow(
        t, "claude-haiku-5-5", 3_000, out=200, cwd="/home/dev/epsilon-worktrees/feat"
    )
    s.cost_state(
        t + timedelta(seconds=5),
        run1,
        0.11,
        {"claude-haiku-5-5": (15, 1_400, 210_000, 153_000, 0.11)},
    )
    t = at("2026-11-04 13:40")
    run2 = t - timedelta(seconds=30)
    s.prompt(t)
    t += timedelta(seconds=5)
    s.grow(t, "claude-haiku-5-5", 87_000, out=900)  # past 200k
    s.cost_state(
        t + timedelta(seconds=5),
        run2,
        0.04,
        {"claude-haiku-5-5": (8, 900, 154_000, 87_000, 0.04)},
    )
    s.write()


def epsilon_2(root):
    """A session whose only cost record is of a run that began 45 minutes
    after its first turn: the record does not cover it."""
    e = "/home/dev/epsilon"
    s = Log(root, e, "epsilon-2", e)
    t = at("2026-11-05 10:00")
    s.prompt(t)
    t += timedelta(seconds=5)
    s.grow(t, "claude-opus-5-5", 18_000, out=400)
    t += timedelta(minutes=45)
    start = t - timedelta(seconds=20)
    s.prompt(t)
    s.grow(t + timedelta(seconds=5), "claude-opus-5-5", 3_000, out=300)
    t += timedelta(minutes=3)
    s.grow(t, "claude-opus-5-5", 2_500, out=250)
    s.cost_state(
        t + timedelta(seconds=10),
        start,
        0.22,
        {"claude-opus-5-5": (10, 550, 42_000, 5_500, 0.22)},
    )
    s.write()


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    here = os.path.dirname(os.path.abspath(__file__))
    ap.add_argument(
        "--out",
        default=os.path.join(here, "..", "testdata", "golden"),
        help="the CLAUDE_CONFIG_DIR to write (its projects folder is replaced)",
    )
    root = os.path.normpath(ap.parse_args().out)
    shutil.rmtree(os.path.join(root, "projects"), ignore_errors=True)
    for build in (
        alpha_1,
        alpha_2_and_3,
        beta_1,
        beta_2,
        zeta_orphan,
        epsilon_0,
        work_alpha_1,
        epsilon_1,
        epsilon_2,
    ):
        build(root)
    print("wrote", os.path.join(root, "projects"))


if __name__ == "__main__":
    main()
