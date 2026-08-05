---
id: ADR-DOC-003
lang: en
counterpart: 003-event-sourced-case.zh.md
doc_version: 1.0.0
status: approved
stage: decide
---

<!-- sdd:item id=ADR-003 stage=decide status=approved derives_from=ARC-003 -->
# ADR-003 — Event-sourced case projection rather than mutable records

**Status.** accepted

**Date.** 2026-08-05

## Context

REQ-0003 requires that a completed investigation replay from its log, and REQ-0053
requires the regenerated report to equal the live one. The conventional shape — mutable
case records plus an audit log written alongside — satisfies both only as long as every
writer remembers to log. That discipline decays, and its decay is invisible until an
audit needs the log that was never written.

The console also needs a live feed (REQ-0064), which a mutable model must synthesise by
diffing or by publishing separately — a second place for the same discipline to fail.

## Decision

We will make the append-only event log the sole source of truth. The case is a
projection computed by folding events, and the same fold produces both the live view
and the replayed view. Nothing writes a projection field directly.

## Consequences

### Positive

- Replay equality is structural rather than tested-into-existence: one function, two
  call sites.
- The live event stream is the log, so the console feed needs no separate publication
  path.
- Every state change is attributable, ordered and timed, which the report and the audit
  trail consume directly.
- Concurrency reduces to appending under one writer, which suits ARC-004.

### Negative

- Reading a case means folding its events; for long cases this is repeated work unless
  the projection is cached in memory, which it is.
- Queries across cases are awkward — there is no index beyond the case list. No
  requirement asks for such queries.
- Changing an event's shape requires care, since old events must still fold. Event
  types are versioned by name.

### Neutral

- The log is also the wire format for the stream and the on-disk format for the file
  store, which removes two serialisation surfaces.

## Alternatives considered

| Alternative | Why it was attractive | Why it lost |
|---|---|---|
| Mutable records plus a separate audit log | Familiar; simplest read path | Two sources of truth that can silently diverge. Replay equality becomes a convention rather than a property |
| Mutable records with change-data capture | Replay without changing the write model | Adds machinery to recover a property that event sourcing has by construction |
| Event sourcing with snapshots | Faster reads on long cases | Premature: case length is bounded by ARC-011, so the fold is bounded too |

## Revisit trigger

A case whose event count makes folding measurably slow in the console path, or a
requirement for cross-case querying.
