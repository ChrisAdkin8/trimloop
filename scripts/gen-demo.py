#!/usr/bin/env python3
"""Write the made-up Claude Code logs that 'trimloop demo' reports on.

    python3 -I scripts/gen-demo.py [--out internal/demo/data]

The logs are five weeks of work by one invented developer on four made-up
projects, laid out as Claude Code keeps them under CLAUDE_CONFIG_DIR:
projects/<encoded-folder>/<session>.jsonl for main sessions and
projects/<encoded-folder>/<session>/subagents/agent-*.jsonl, with a
.meta.json each, for sub-agents. Every name, folder, title and text is made
up; no real log is copied. The output is deterministic: a fixed seed, fixed
dates, no clock, so running it twice gives the same bytes.

Unlike the golden logs (scripts/gen-golden.py), these hold no edge cases:
they are meant to read like an ordinary person's month, with the habits
trimloop's findings are about. Token counts are in the usage fields, so the
text stays short; tool results are sized in bytes, as the tool-output
finding measures them. The whole folder is embedded in the binary, so keep
it small (internal/demo).
"""

import argparse
import json
import os
import random
import shutil
import uuid
from datetime import datetime, timedelta, timezone

SEED = 20261005
VERSION = "2.1.320"
NS = uuid.UUID("3b8e5f0a-6c1d-4e2f-9a7b-5d4c3b2a1908")  # for name-based ids
HOME = "/home/dev"

rng = random.Random(SEED)
_counter = [0]


def next_id(kind):
    """A deterministic id: a UUID made from a counter."""
    _counter[0] += 1
    return str(uuid.uuid5(NS, f"{kind}-{_counter[0]}"))


def short_id(kind, n=22):
    return next_id(kind).replace("-", "")[:n]


def iso(t):
    return t.strftime("%Y-%m-%dT%H:%M:%S.") + f"{t.microsecond // 1000:03d}Z"


def at(s):
    """A UTC time from 'YYYY-MM-DD HH:MM'."""
    return datetime.strptime(s, "%Y-%m-%d %H:%M").replace(tzinfo=timezone.utc)


WORDS = (
    "cart order basket price total line item tax rate rule queue worker job "
    "retry batch flag option config parse token buffer render layout panel "
    "table column index cache store fetch route handler client server field "
    "value check guard error result report export import module package"
).split()


def filler(n):
    """About n bytes of made-up words, in lines of about 70."""
    out, line, size = [], [], 0
    while size < n:
        w = rng.choice(WORDS)
        line.append(w)
        size += len(w) + 1
        if sum(len(x) + 1 for x in line) > 70:
            out.append(" ".join(line))
            line = []
    out.append(" ".join(line))
    return "\n".join(out)[:n]


def encode(folder):
    """Claude Code's folder name for a working folder: / and . become -."""
    return folder.replace("/", "-").replace(".", "-")


# What a reply says before its tool call; the words do not matter, the
# usage does.
SAYS = [
    "Let me look at that.",
    "I'll check the tests first.",
    "Reading the handler now.",
    "That change is in. Running the tests.",
    "Found it; fixing the guard.",
    "Searching for the other callers.",
]

# Messages the developer typed; short, since only their timing counts.
ASKS = [
    "Can you take a look at the failing test?",
    "Looks good. Now the export path.",
    "Try the other approach.",
    "Add a test for the empty case.",
    "Why is this slower than before?",
    "Carry on.",
    "OK, let's tidy this up and commit.",
]

TOOLS = [
    ("Bash", 700, 190),
    ("Read", 450, 150),
    ("Edit", 450, 160),
    ("Write", 250, 100),
    ("Grep", 500, 150),
    ("Glob", 220, 80),
    ("Agent", 850, 240),
    ("WebFetch", 320, 100),
    ("mcp__tracker__search_issues", 350, 130),
    ("mcp__tracker__get_issue", 260, 90),
]


def usage(read=0, w5=0, w1=0, unc=3, out=200, thinking=None):
    u = {
        "input_tokens": unc,
        "cache_creation_input_tokens": w5 + w1,
        "cache_read_input_tokens": read,
        "cache_creation": {
            "ephemeral_5m_input_tokens": w5,
            "ephemeral_1h_input_tokens": w1,
        },
        "output_tokens": out,
    }
    if thinking is not None:
        u["output_tokens_details"] = {"thinking_tokens": thinking}
    return u


class Log:
    """One log file being written."""

    def __init__(self, folder, session, agent=None, agent_type=None):
        self.folder, self.session, self.agent = folder, session, agent
        d = os.path.join("projects", encode(folder))
        if agent:
            d = os.path.join(d, session, "subagents")
            self.path = os.path.join(d, f"agent-{agent}.jsonl")
            self.meta = (
                os.path.join(d, f"agent-{agent}.meta.json"),
                {"agentType": agent_type, "spawnDepth": 1},
            )
        else:
            self.path = os.path.join(d, f"{session}.jsonl")
            self.meta = None
        self.lines = []
        self.parent = None
        self.ctx = 0  # tokens the next request re-sends from the cache
        self.life = "5m" if agent else "1h"

    def line(self, typ, t, body):
        d = {
            "parentUuid": self.parent,
            "isSidechain": bool(self.agent),
            "cwd": self.folder,
            "sessionId": self.session,
            "version": VERSION,
        }
        if self.agent:
            d["agentId"] = self.agent
        d["type"] = typ
        d.update(body)
        d["uuid"] = next_id("line")
        d["timestamp"] = iso(t)
        self.parent = d["uuid"]
        self.lines.append(d)
        return d

    def title(self, text):
        self.lines.append({"type": "ai-title", "sessionId": self.session, "aiTitle": text})

    def prompt(self, t, text=None):
        self.line(
            "user",
            t,
            {
                "message": {"role": "user", "content": text or rng.choice(ASKS)},
                "origin": {"kind": "human"},
            },
        )

    def snapshot(self, t, system_bytes, tools):
        tl = [
            {
                "name": n,
                "description": filler(db),
                "schema": {
                    "type": "object",
                    "properties": {"arg": {"type": "string", "description": filler(sb)}},
                },
            }
            for n, db, sb in tools
        ]
        self.line(
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

    def memory(self, t, size):
        """The project's CLAUDE.md, which the harness adds to the start."""
        text = filler(size)
        self.line(
            "attachment",
            t,
            {
                "attachment": {
                    "type": "instructions",
                    "files": [
                        {
                            "path": self.folder + "/CLAUDE.md",
                            "type": "Project",
                            "content": text,
                        }
                    ],
                },
                "rendered": [{"content": text}],
            },
        )

    def reply(self, t, model, u, effort, call=None):
        """An assistant reply, with a tool call when call is (name, input)."""
        content = [{"type": "text", "text": rng.choice(SAYS)}]
        tid = None
        if call:
            tid = "toolu_01" + short_id("tool")
            content.append({"type": "tool_use", "id": tid, "name": call[0], "input": call[1]})
        self.line(
            "assistant",
            t,
            {
                "message": {
                    "id": "msg_01" + short_id("msg"),
                    "type": "message",
                    "role": "assistant",
                    "model": model,
                    "content": content,
                    "stop_reason": "tool_use" if tid else "end_turn",
                    "usage": u,
                },
                "effort": effort,
            },
        )
        return tid

    def result(self, t, tid, text):
        self.line(
            "user",
            t,
            {
                "message": {
                    "role": "user",
                    "content": [
                        {
                            "type": "tool_result",
                            "tool_use_id": tid,
                            "is_error": False,
                            "content": text,
                        }
                    ],
                },
            },
        )

    def turn(self, t, model, new, out, effort, thinking, call=None, result=None, rebuild=False):
        """A turn that reads the earlier context from the cache and writes
        `new` tokens; after a long break (rebuild) the cache has expired, so
        it writes the whole context again. A tool call's result follows a
        few seconds later."""
        unc = rng.randint(2, 9)
        read, write = self.ctx, new
        if rebuild:
            read, write = 0, self.ctx + new
        w5, w1 = (write, 0) if self.life == "5m" else (0, write)
        u = usage(read=read, w5=w5, w1=w1, unc=unc, out=out, thinking=thinking)
        self.ctx += new + unc + out
        tid = self.reply(t, model, u, effort, call)
        if tid:
            self.result(t + timedelta(seconds=rng.randint(2, 20)), tid, result)

    def compact(self, t, trigger, summary, ms):
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
                    "preTokens": self.ctx,
                    "postTokens": summary,
                    "durationMs": ms,
                },
            },
        )
        self.line(
            "user",
            t + timedelta(seconds=1),
            {
                "message": {"role": "user", "content": filler(300)},
                "isCompactSummary": True,
                "isVisibleInTranscriptOnly": True,
            },
        )

    def write(self, root):
        path = os.path.join(root, self.path)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "w", newline="\n") as f:
            f.writelines(
                json.dumps(d, separators=(",", ":"), ensure_ascii=False) + "\n"
                for d in self.lines
            )
        if self.meta:
            with open(os.path.join(root, self.meta[0]), "w", newline="\n") as f:
                f.write(json.dumps(self.meta[1], separators=(",", ":")) + "\n")


# Files each project's tool calls name.
FILES = {
    "oak": ["web/cart.ts", "web/checkout.ts", "api/orders.go", "api/prices.go", "api/tax.go"],
    "larch": ["cmd/larch/main.go", "internal/queue/worker.go", "internal/queue/retry.go"],
    "birch": ["src/layout.rs", "src/render.rs", "src/table.rs", "tests/render.rs"],
    "rowan": ["deploy/stack.tf", "deploy/variables.tf", "scripts/release.sh"],
}
TESTS = {
    "oak": "go test ./api/... && npm test --prefix web",
    "larch": "go test ./...",
    "birch": "cargo test",
    "rowan": "terraform plan -no-color",
}


def tool_call(project, folder, big):
    """A tool call and the size of its result, in bytes. big: one of the
    long outputs (a full test run, a whole file) that fill the context."""
    files = FILES[project]
    if big:
        if rng.random() < 0.6:
            return ("Bash", {"command": TESTS[project]}), rng.randint(5_000, 8_000)
        return ("Read", {"file_path": folder + "/" + rng.choice(files)}), rng.randint(5_000, 7_000)
    r = rng.random()
    if r < 0.30:
        f = folder + "/" + rng.choice(files)
        return ("Read", {"file_path": f, "offset": 1, "limit": 80}), rng.randint(300, 900)
    if r < 0.50:
        f = folder + "/" + rng.choice(files)
        return ("Edit", {"file_path": f, "old_string": "a", "new_string": "b"}), rng.randint(50, 90)
    if r < 0.65:
        return ("Grep", {"pattern": rng.choice(WORDS), "path": folder}), rng.randint(150, 600)
    if r < 0.72:
        issue = f"{project.upper()}-{rng.randint(10, 400)}"
        return ("mcp__tracker__get_issue", {"id": issue}), rng.randint(300, 800)
    cmd = rng.choice(["git status --short", "git diff --stat", "make lint"])
    return ("Bash", {"command": cmd}), rng.randint(80, 400)


def sub_agent(root, folder, session, t, agent_type, turns, model):
    """A sub-agent run started at t, reading and searching the project on
    the 5-minute cache; returns when it finished."""
    project = os.path.basename(folder)
    a = Log(folder, session, agent="a" + short_id("agent", 16), agent_type=agent_type)
    a.prompt(t, text="Find where the " + rng.choice(WORDS) + " is worked out and summarise it.")
    for i in range(turns):
        t += timedelta(seconds=rng.randint(15, 50))
        new = 24_000 if i == 0 else rng.randint(10_000, 24_000)
        out = rng.randint(400, 1_300)
        call, size = tool_call(project, folder, big=False)
        if call[0] == "Edit":  # helpers that search and read do not edit
            call = ("Grep", {"pattern": rng.choice(WORDS), "path": folder})
        a.turn(t, model, new, out, "high", out // 2, call=call, result=filler(size))
    t += timedelta(seconds=rng.randint(15, 40))
    a.turn(t, model, rng.randint(2_000, 4_000), rng.randint(800, 1_400), "high", 400)
    a.write(root)
    return t


def session(root, spec):
    """One main session from its spec (see SESSIONS). It grows by `grow`
    tokens a turn while it reads its way in, then by `late` (3,000 to
    8,000 unless given) once past 150,000 tokens, as later turns are mostly
    small edits and checks."""
    project = spec["project"]
    folder = f"{HOME}/{project}"
    sid = next_id("session")
    s = Log(folder, sid)
    s.title(spec["title"])
    t = at(spec["start"])
    s.prompt(t)
    s.snapshot(t + timedelta(seconds=1), 2_500, TOOLS)
    s.memory(t + timedelta(seconds=1), 800)
    model = spec["model"]
    breaks = spec.get("breaks", {})
    agents = spec.get("agents", {})
    compact = spec.get("compact", {})
    xhigh = spec.get("xhigh", 0.0)
    t += timedelta(seconds=6)
    for i in range(spec["turns"]):
        lo, hi = spec["grow"] if s.ctx < 150_000 else spec.get("late", (3_000, 8_000))
        new = 24_000 if i == 0 else rng.randint(lo, hi)
        rebuild = False
        if i in breaks:
            t += timedelta(minutes=breaks[i])
            s.prompt(t - timedelta(seconds=40))
            rebuild = True
        elif i in compact:
            trigger, summary = compact[i]
            if trigger == "manual":
                s.prompt(t - timedelta(seconds=30), text="/compact")
            s.compact(t - timedelta(seconds=20), trigger, summary, rng.randint(40_000, 95_000))
            s.ctx = 0
            new = summary + 14_000  # the summary, the fixed start and re-attached files
        elif i > 0 and rng.random() < 0.22:
            t += timedelta(minutes=rng.randint(1, 6))
            s.prompt(t - timedelta(seconds=10))
        effort = "xhigh" if rng.random() < xhigh else "high"
        out = rng.randint(1_000, 3_200) if effort == "high" else rng.randint(3_000, 6_000)
        thinking = int(out * (0.45 if effort == "high" else 0.75))
        if i in agents:
            agent_type, n = agents[i]
            call = ("Agent", {"subagent_type": agent_type, "description": "Look around " + project})
            tid = s.reply(t, model, usage(read=s.ctx, w1=new, unc=4, out=out, thinking=thinking), effort, call)
            s.ctx += new + 4 + out
            done = sub_agent(root, folder, sid, t + timedelta(seconds=3), agent_type, n, spec.get("agent_model", model))
            s.result(done + timedelta(seconds=2), tid, filler(rng.randint(600, 1_200)))
            t = done + timedelta(seconds=8)
            continue
        if i == spec["turns"] - 1:
            s.turn(t, model, new, out, effort, thinking, rebuild=rebuild)
            break
        call, size = tool_call(project, folder, rng.random() < spec.get("big", 0.05))
        s.turn(t, model, new, out, effort, thinking, call=call, result=filler(size), rebuild=rebuild)
        t += timedelta(seconds=rng.randint(25, 80))
    s.write(root)


# The developer's five weeks, Monday 5 October to Friday 6 November 2026
# (UTC). They began on Opus 5 and moved to Opus 5.5 at the end of the first week, use
# Fable 5.1 for design work, keep long sessions going without compacting,
# carry on after lunch in the same session, and let sub-agents run on the
# main model. The first and last replies fall between 10:00 and 11:00 UTC,
# so the report's dates are the same in every time zone from UTC-10 to
# UTC+13.
#
# turns: main-session turns; breaks: {turn: minutes away before it};
# agents: {turn: (agent type, turns)}; compact: {turn: (trigger, summary
# tokens)}; grow, late: see session; xhigh: the share of turns at xhigh effort; big: the share of
# tool calls with a long output.
SESSIONS = [
    # Week 1
    dict(start="2026-10-05 10:02", project="oak", model="claude-opus-5", turns=27, grow=(16_000, 33_000),
         breaks={20: 85}, agents={4: ("Explore", 10)}, title="Fix rounding in the cart total"),
    dict(start="2026-10-06 09:10", project="oak", model="claude-opus-5", turns=14, grow=(10_000, 26_000),
         agents={3: ("general-purpose", 9)}, title="Add tax rules for the new regions"),
    dict(start="2026-10-07 13:30", project="larch", model="claude-opus-5", turns=24, grow=(16_000, 33_000),
         breaks={19: 70}, title="Retry failed jobs with backoff"),
    dict(start="2026-10-08 09:00", project="birch", model="claude-fable-5-1", turns=11, grow=(12_000, 26_000),
         xhigh=0.6, title="Design the table layout engine"),
    dict(start="2026-10-09 14:00", project="oak", model="claude-opus-5-5", turns=11, grow=(9_000, 20_000),
         title="Checkout page loading state"),
    # Week 2
    dict(start="2026-10-12 09:20", project="larch", model="claude-opus-5-5", turns=30, grow=(20_000, 38_000),
         breaks={20: 95}, agents={4: ("Explore", 12)}, title="Worker pool drains too slowly"),
    dict(start="2026-10-13 10:00", project="oak", model="claude-opus-5-5", turns=18, grow=(12_000, 30_000),
         agents={2: ("Explore", 10), 10: ("general-purpose", 10)}, title="Order export to CSV"),
    dict(start="2026-10-14 09:40", project="rowan", model="claude-opus-5-5", turns=9, grow=(6_000, 16_000),
         title="Split the release script into steps"),
    dict(start="2026-10-15 09:05", project="birch", model="claude-opus-5-5", turns=23, grow=(15_000, 32_000),
         xhigh=0.3, breaks={18: 110}, title="Text wrapping in narrow columns"),
    dict(start="2026-10-16 13:15", project="oak", model="claude-opus-5-5", turns=14, grow=(16_000, 30_000),
         compact={11: ("manual", 21_000)}, title="Discount codes that stack"),
    # Week 3
    dict(start="2026-10-19 09:30", project="oak", model="claude-opus-5-5", turns=32, grow=(21_000, 42_000),
         late=(12_000, 20_000),
         breaks={20: 80, 29: 75}, agents={5: ("Explore", 12)}, big=0.1, title="Price cache goes stale after deploys"),
    dict(start="2026-10-20 10:10", project="larch", model="claude-opus-5-5", turns=14, grow=(10_000, 26_000),
         agents={4: ("general-purpose", 10)}, title="Dead-letter queue for poison jobs"),
    dict(start="2026-10-21 09:00", project="birch", model="claude-opus-5-5", turns=23, grow=(15_000, 32_000),
         xhigh=0.3, breaks={17: 75}, title="Renderer benchmarks and a faster path"),
    dict(start="2026-10-22 14:20", project="rowan", model="claude-opus-5-5", turns=8, grow=(6_000, 15_000),
         title="Tag releases from CI"),
    dict(start="2026-10-23 09:15", project="oak", model="claude-opus-5-5", turns=20, grow=(20_000, 33_000),
         compact={14: ("auto", 26_000)}, agents={3: ("Explore", 10)}, title="Refunds for partly shipped orders"),
    # Week 4
    dict(start="2026-10-26 09:00", project="larch", model="claude-opus-5-5", turns=34, grow=(21_000, 40_000),
         late=(12_000, 20_000),
         breaks={22: 90}, agents={4: ("Explore", 12), 27: ("general-purpose", 10)}, big=0.1,
         title="Move the queue to the new broker"),
    dict(start="2026-10-27 10:30", project="oak", model="claude-opus-5-5", turns=13, grow=(10_000, 22_000),
         title="Flaky checkout test"),
    dict(start="2026-10-28 09:10", project="birch", model="claude-fable-5-1", turns=11, grow=(12_000, 26_000),
         xhigh=0.6, agents={4: ("Explore", 10)}, title="Plan the plugin interface"),
    dict(start="2026-10-29 13:00", project="rowan", model="claude-opus-5-5", turns=11, grow=(9_000, 20_000),
         breaks={7: 65}, title="Staging stack drifts from production"),
    dict(start="2026-10-30 09:20", project="oak", model="claude-opus-5-5", turns=16, grow=(12_000, 27_000),
         xhigh=0.25, title="Slow order search"),
    # Week 5
    dict(start="2026-11-02 09:00", project="oak", model="claude-opus-5-5", turns=31, grow=(21_000, 40_000),
         late=(12_000, 20_000),
         breaks={20: 100}, agents={4: ("Explore", 12)}, big=0.1, title="Gift cards at checkout"),
    dict(start="2026-11-03 10:00", project="larch", model="claude-opus-5-5", turns=16, grow=(16_000, 30_000),
         compact={13: ("manual", 19_000)}, agents={4: ("general-purpose", 10)}, title="Metrics for job latency"),
    dict(start="2026-11-04 09:30", project="birch", model="claude-opus-5-5", turns=22, grow=(15_000, 32_000),
         xhigh=0.3, breaks={16: 80}, title="Export tables to HTML"),
    dict(start="2026-11-05 13:40", project="oak", model="claude-opus-5-5", turns=13, grow=(9_000, 21_000),
         title="Address form validation"),
    dict(start="2026-11-06 10:00", project="rowan", model="claude-opus-5-5", turns=9, grow=(6_000, 16_000),
         title="Rotate the deploy keys"),
]


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    here = os.path.dirname(os.path.abspath(__file__))
    ap.add_argument(
        "--out",
        default=os.path.join(here, "..", "internal", "demo", "data"),
        help="the CLAUDE_CONFIG_DIR to write (its projects folder is replaced)",
    )
    root = os.path.normpath(ap.parse_args().out)
    shutil.rmtree(os.path.join(root, "projects"), ignore_errors=True)
    for spec in SESSIONS:
        session(root, spec)
    print("wrote", os.path.join(root, "projects"))


if __name__ == "__main__":
    main()
