# ADR 0005: Go core, thin Python plugin for LiteLLM

- Status: Proposed — amended by [ADR 0006](0006-gateway-plugin-forwards-to-sidecar.md) (sidecar does fingerprinting by default)
- Date: 2026-11-05

## Context

Many teams already route model traffic through an LLM gateway. A gateway
plugin gives Trimloop full request and response bodies, and the ability to
apply in-path transforms, with one install per team instead of a proxy on
every developer machine.

Each gateway dictates its plugin language:

| Gateway | Plugin mechanism |
|---|---|
| LiteLLM | Python (`CustomLogger` callbacks; `async_pre_call_hook` can modify requests) |
| Portkey | TypeScript (before/after request hooks, guardrail-oriented) |
| Envoy-based gateways | External processing (gRPC service, any language) or proxy-wasm |
| Kong | Lua natively; Go/JS/Python plugin servers; proxy-wasm |

LiteLLM is the most widely self-hosted gateway and is the first target. The
core (ADR 0002) is Go.

Rust was considered because one Rust core could ship as a Python extension
(PyO3) and a proxy-wasm filter. Rejected: Envoy and Kong already accept a Go
service directly, so the cross-gateway gain is limited to LiteLLM and Portkey,
and it does not outweigh Go's faster iteration on detectors.

## Decision

- The LiteLLM plugin is written in Python and kept thin. It:
  - captures requests and responses,
  - redacts secrets and fingerprints segments,
  - forwards events to the Trimloop core (team collector),
  - applies transforms the core has decided on (L2/L3), identified by
    transform version.
- It contains no detectors and no recommendation logic.
- Envoy and Kong integrations use Go via external processing and Kong's Go
  plugin server respectively.
- Portkey (TypeScript) is deferred until a customer needs it.

## Consequences

- Fingerprinting and redaction exist in two languages. The rules are specified
  once in `docs/data-model.md` and both implementations are tested against a
  shared set of test vectors; a mismatch fails CI.
- Transform logic that must run inside LiteLLM is limited to applying a
  declarative transform spec produced by the core, so behaviour cannot drift.
- A plugin failure must fail open: requests pass through untouched.
- LiteLLM hook signatures change between versions; the plugin pins a supported
  range and is tested against each.
