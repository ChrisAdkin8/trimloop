# Value case

| | |
|---|---|
| Status | Hypothesis — not yet measured |
| Last updated | 2026-11-05 |
| Validated by | M0 (`trimloop analyse`) on real sessions — see [§8](#8-validation-plan) |

Why Trimloop should save money and time, how much, and where the value runs
out. All figures are theoretical until M0 produces measured numbers.

## 1. Why the waste exists

Coding agents re-send their whole working context — system prompt, tool
definitions, memory files, conversation history, file reads, tool output — on
every turn. Two consequences drive the economics:

1. **Input dominates.** A typical turn sends tens of thousands of input tokens
   and receives a few hundred to a few thousand output tokens.
2. **Prompt caching decides the bill.** Providers serve an unchanged request
   prefix from cache at a steep discount:

   | Provider | Cache read | Cache write |
   |---|---|---|
   | Anthropic | ≈ 0.1 × input price | ≈ 1.25 × input price (5-minute TTL); higher for the 1-hour TTL |
   | OpenAI | ≈ 10–50 % of input price, depending on model | Not billed |

   Prices change; the authoritative figures are the price catalogue
   ([data-model.md](data-model.md#price-catalogue)).

So an agent that breaks its cache, or carries dead weight in its context, pays
for the same tokens again and again. The same work can cost several times
more than it needs to.

## 2. The levers, ranked by expected value

| Rank | Lever | Patterns | Why this rank |
|---|---|---|---|
| 1 | **Fix broken caching** | P01 cache-prefix churn, P02 cache TTL expiry | Applies to almost every turn; moving the cache-hit rate from ~70 % to ~95 % roughly halves the cost of a turn (§3) |
| 2 | **Trim bloated or stale context** | P04 oversized tool output, P07 bloated standing context, P08 stale context | Shrinks the base that every later turn pays for; research reports 40–60 % input-token reductions (§4) |
| 3 | **Stop loops and wasted runs** | P05 tool-call loops, P11 post-failure burn | Rarer, but each occurrence wastes a whole run's tokens and the developer's time |
| 4 | **Use cheaper models for easy steps** | P13 model over-provisioning | Large per-turn saving, but needs outcome data and carries the most quality risk |
| 5 | **Reuse repeated work** | P15 duplicated exploration, P16 repeated workflow, P17 repeated instructions, P18 hot documentation | Saves developer time as well as tokens, via skills, summaries and documentation retrieval ([ADR 0007](adr/0007-recommend-skills-and-doc-retrieval.md)). Savings show up as fewer turns per task, which takes longer to measure, so it ranks last |

Other patterns (P03, P06, P09, P10, P12, P14) add to these but are
expected to be smaller individually. The ranking is a starting hypothesis; M0
will re-rank by measured waste.

## 3. Worked example: one turn

Assumptions, priced relative to the normal input price (shape of Anthropic's
pricing):

| Token class | Relative price |
|---|---|
| Input (uncached) | 1 |
| Cache read | 0.1 |
| Cache write | 1.25 |
| Output | 5 |

One turn: 60,000 tokens of context, 1,000 tokens of output. Uncached context
is assumed to be written to cache (the usual case for an agent that sets
cache breakpoints).

| Scenario | Calculation | Relative cost | Saving vs A |
|---|---|---|---|
| **A.** 70 % of context read from cache | 42,000 × 0.1 + 18,000 × 1.25 + 1,000 × 5 | 31,700 | — |
| **B.** Caching fixed: 95 % read from cache | 57,000 × 0.1 + 3,000 × 1.25 + 1,000 × 5 | 14,450 | 54 % |
| **C.** B, plus context trimmed by 30 % (42,000 tokens) | 39,900 × 0.1 + 2,100 × 1.25 + 1,000 × 5 | 11,615 | 63 % |

Observations:

- The cache-hit rate is the dominant variable: in scenario A, cache writes are
  71 % of the turn's cost.
- Once caching is fixed, output becomes a much larger share of the bill (35 %
  in B, 43 % in C), which is why context trimming has a smaller marginal effect
  than fixing caching.
- On providers without a write premium (OpenAI), the A → B gap is smaller;
  context trimming matters relatively more.

## 4. Evidence so far

| Source | Reported result | Weight |
|---|---|---|
| Developer write-up of a local reverse proxy for Claude Code | Cache-hit rate 38 % → 70 % after reordering CLAUDE.md | Single anecdote |
| `claude-code-cache-fix` proxy | Cache-hit rate 82 % → 95 % on first warm turn | Self-reported A/B, one harness version |
| AgentDiet (trajectory reduction research) | 39.9–59.7 % input-token reduction by removing useless, redundant and expired context | Peer research, specific agents |
| "Context rot" research (e.g. Chroma, 2025) | Model performance degrades as context grows with irrelevant material | Supports quality benefit, not a cost figure |

None of these measure Trimloop. They show the levers exist and are large
enough to be worth building for.

## 5. Forms of value

| Form | Who feels it | How it shows up |
|---|---|---|
| **Lower spend** | Budget holder | Avoidable waste of perhaps 20–60 % of agent spend for poorly tuned teams (5–15 % for well-tuned ones). The net saving is smaller: §6's example nets 14 % after fixable and adoption shares |
| **Speed** | Developer | Cached prefixes reduce time to first token; smaller context and fewer loops finish tasks sooner. Developer time costs more than tokens |
| **Headroom under limits** | Developers on flat-fee plans; API users near rate limits | Waste hits usage caps and rate limits sooner. Efficiency means more work before being throttled, not a smaller bill |
| **Answer quality** | Developer | Less stale context and fewer loops may improve results. Plausible, not proven — hence outcome signals ([design.md §6.3](design.md#63-outcome-signals)) |
| **Control and proof** | Platform lead, FinOps | Attribution by developer, repo and harness; recommendations ranked by saving; before/after measurement of fixes applied |

## 6. Value model

```text
annual value = annual agent spend
             × waste share          (fraction of spend that is avoidable)
             × fixable share        (fraction of waste Trimloop can fix)
             × adoption share       (fraction of fixes actually applied)
```

Illustrative team:

| Input | Value |
|---|---|
| Developers | 50 |
| Agent spend per developer per month | $300 |
| Annual agent spend | $180,000 |
| Waste share | 40 % |
| Fixable share | 50 % |
| Adoption share | 70 % |
| **Annual saving** | **$25,200** |

This excludes developer time saved and headroom gained. Each input is a
guess until measured; the model's purpose is to show which inputs to measure.

**Pricing implication.** At this size, charging 10–20 % of measured savings
($2,500–$5,000 a year) is an easy purchase for the budget holder, and the
before/after measurement in the recommendation engine is what makes
"measured savings" credible.

## 7. Where the value runs out

| Limit | Effect | Response |
|---|---|---|
| Well-tuned teams | Little waste to remove | Lead with control, proof and speed rather than savings |
| Vendors improve their own caching and analytics | Easy single-harness wins shrink over time | Durable value is cross-harness, team-wide, and measured — vendors will not do this across competitors |
| Flat-fee plans | Savings are not cash unless the team moves to a cheaper plan | Sell headroom and speed for these users |
| Behaviour-changing fixes (L3) | Risk to quality | Canary rollout and outcome signals; L3 is opt-in |
| Unmeasured | Every figure above is a hypothesis | §8 |

## 8. Validation plan

1. Run `trimloop analyse` (M0) on the founder's own Claude Code sessions.
   Record actual cache-hit rate, token mix by class, and estimated waste per
   pattern.
2. Repeat on 5–10 design-partner developers; add a second harness once the
   Codex reader ships (M1).
3. Replace the hypotheses in §2, §3 and §6 with measured values: re-rank the
   levers, publish the measured waste-share range.
4. For one or two applied L1 fixes (e.g. CLAUDE.md reordering), measure the
   before/after cache-hit rate and cost on the same developers and repos.
5. Update this document's status from *Hypothesis* to *Measured*, with dates
   and sample sizes.

## Sources

- [DEV: a local reverse proxy to see what Claude Code sends](https://dev.to/houleixx/i-built-a-local-reverse-proxy-to-see-what-claude-code-actually-sends-to-anthropic-5foo)
- [claude-code-cache-fix](https://github.com/cnighswonger/claude-code-cache-fix)
- [AgentDiet: improving the efficiency of LLM agent systems through trajectory reduction](https://arxiv.org/html/2509.23586v2)
