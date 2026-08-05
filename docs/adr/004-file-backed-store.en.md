---
id: ADR-DOC-004
lang: en
counterpart: 004-file-backed-store.zh.md
doc_version: 1.0.0
status: approved
stage: decide
---

<!-- sdd:item id=ADR-004 stage=decide status=approved derives_from=ARC-013 -->
# ADR-004 — A file-backed append-only store rather than a database

**Status.** accepted

**Date.** 2026-08-05

## Context

The original brief proposed SQLite first and PostgreSQL later. ADR-003 has already
reduced persistence to one access pattern: append events to a case, and read a case's
events in order. There is no join, no secondary index, no concurrent writer and no
cross-case query in any requirement.

CON-008 makes an embedded SQL engine a decision requiring justification — the pure-Go
SQLite drivers are large dependencies whose compile time dominates the rest of the
build — and an external PostgreSQL adds a service the demo must bring up and the tests
must either mock or start.

## Decision

We will define a `Store` port exposing case indexing plus event append and range-read,
and implement it twice: an in-memory adapter (the test and default-demo profile) and a
file-backed adapter writing one JSON-lines log per case, with an atomic case index.

## Consequences

### Positive

- Zero dependencies and zero services; `go test ./...` needs no fixture database.
- The on-disk format is the event format, so a case is inspectable with `cat` and
  diffable — useful when reviewing a demo run.
- Append-and-scan is exactly what the file system is good at; per-case files keep write
  contention nonexistent.
- The port is small enough that a database adapter is a contained addition.

### Negative

- No transactional guarantee across a case index update and an event append; the index
  is rebuilt from the directory on startup, which makes it derived rather than
  authoritative.
- No query capability beyond scanning. Reporting across many cases would require
  reading them all.
- Retention is manual: bounded per case by ARC-011, but old case files are not reaped
  automatically.

### Neutral

- Durability is a deployment choice made by profile, not a design commitment.

## Alternatives considered

| Alternative | Why it was attractive | Why it lost |
|---|---|---|
| Embedded SQL engine | Familiar; queryable; the brief proposed it | Contradicts CON-008 for a workload with no relational access pattern; the driver's build cost exceeds the whole service |
| External PostgreSQL | Production-shaped; the brief's phase two | A service the demo must start and the tests must provide. Buys durability guarantees the demo does not need |
| Memory only | Simplest possible | A restart loses the demo run an evaluator just watched, and REQ-0053 replay would have nothing to replay from |

## Revisit trigger

A requirement for cross-case analytics, concurrent writers to one case, or a retention
policy that scanning cannot serve.
