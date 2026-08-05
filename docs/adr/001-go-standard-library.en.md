---
id: ADR-DOC-001
lang: en
counterpart: 001-go-standard-library.zh.md
doc_version: 1.0.0
status: approved
stage: decide
---

<!-- sdd:item id=ADR-001 stage=decide status=approved derives_from=ARC-014 -->
# ADR-001 — Go and the standard library as the implementation substrate

**Status.** accepted

**Date.** 2026-08-05

## Context

The original brief proposed Python with FastAPI and an agent-graph framework. The
project owner directed Go instead. That direction is the decisive input, but it is
worth recording what it buys and what it costs, because the rest of the architecture
leans on the consequences.

Three forces are in tension. The system must be *demonstrable* — it has to start
reliably in a room, on a machine nobody has prepared. It must be *deterministic* — an
autonomous development loop cannot verify what it cannot reproduce. And it must be
*readable as a design*: the orchestration is the substance of this work, so the flow
should be visible as code rather than assembled from framework configuration.

A dependency tree is the usual source of failure in all three. Every module is a
build-time network call, a version resolution, and a behaviour the tests do not own.

## Decision

We will implement the entire system in Go using the standard library only. `go.mod`
will carry no `require` directive, and a test asserts this. External systems are
reached through hand-written ports whose default adapters are in-process.

## Consequences

### Positive

- The artifact is a single static binary; the container image is a base layer plus one
  file. Build requires no module proxy.
- Concurrency for parallel evidence collection is a language primitive, and the
  determinism requirement (REQ-0061) is enforceable by sorting contributions rather
  than by coordinating a framework's scheduler.
- The state machine, the round budget, the coverage guard and the approval gate are
  explicit code in one package. A reviewer can read the whole control flow.
- Supply chain surface is zero. `go vet` and the race detector cover the whole system.

### Negative

- No agent framework, no HTTP router, no template engine, no test assertion library.
  Roughly a few hundred lines of infrastructure must be written and tested by hand —
  routing, SSE, a small template layer, fixture loading.
- No off-the-shelf model client. The model adapter (ADR-002) must be written against
  the provider's HTTP API when it is wired.
- JSON handling via `encoding/json` is more verbose than a schema-generating library
  would be, and validation is explicit.

### Neutral

- Configuration is JSON rather than YAML, since the standard library parses one and not
  the other. This is a cosmetic difference for a small configuration surface.

## Alternatives considered

| Alternative | Why it was attractive | Why it lost |
|---|---|---|
| Python + FastAPI + an agent graph framework | Fastest path to a working agent loop; the ecosystem is where this problem is usually solved | Overridden by the project owner. Independently: a runtime plus a dependency set to install, and the orchestration — the actual subject of the work — would be framework configuration rather than legible code |
| Go with a small set of well-chosen libraries (router, YAML, assertions) | Modest convenience for genuinely mundane code | Each one is a build-time network dependency and a version to maintain, bought against code that is small and stable. CON-008 makes this a decision requiring justification, and the justification is thin |
| Rust | Stronger guarantees, comparable deployment story | No advantage for an I/O-bound orchestration service, and a slower path to a working MVP under a deadline |

## Revisit trigger

A requirement that genuinely cannot be met by the standard library — a real database
adapter, a TLS-terminating gateway, or a model client whose protocol is impractical to
implement by hand. At that point the ADR is amended for that dependency specifically,
not repealed.
