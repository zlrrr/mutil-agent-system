---
id: ADR-DOC-005
lang: en
counterpart: 005-embedded-console.zh.md
doc_version: 1.0.0
status: approved
stage: decide
---

<!-- sdd:item id=ADR-005 stage=decide status=approved derives_from=ARC-014 -->
# ADR-005 — A server-rendered embedded console rather than a front-end application

**Status.** accepted

**Date.** 2026-08-05

## Context

The brief proposed Next.js and TypeScript. The console's job, per REQ-0065, is narrow
and specific: make the agent timeline, the evidence chain, the score breakdowns, the
critiques and the approval gate visible on one screen, updating live.

That is a read-mostly view with one interactive control (approve or reject) and a live
feed. It is not an application.

A separate front end costs a second runtime in the image, a package install at build
time, a build step in CI, and a version skew surface between two deployables — all
against a view whose entire purpose is legibility.

## Decision

We will serve the console from the same binary using `html/template` with assets
embedded via `embed`, and stream updates over server-sent events. No JavaScript
framework, no build step, no external asset path.

## Consequences

### Positive

- One artifact, one port, one version. `docker run` and open a browser.
- No build-time network access, which keeps REQ-0092 satisfiable.
- The view renders from the same case projection the API serves, so the console and the
  API cannot disagree.
- Server-sent events are a few dozen lines over the standard library and reconnect with
  a sequence cursor natively (ARC-012).

### Negative

- Rich interaction — filtering, graph zoom, drag-and-drop timelines — would be
  laborious. None is required.
- Styling and layout are hand-written CSS.
- A reviewer expecting a polished single-page application may read the console as less
  finished than the system behind it.

### Neutral

- The API is complete and documented independently of the console, so a separate front
  end remains possible later without server changes.

## Alternatives considered

| Alternative | Why it was attractive | Why it lost |
|---|---|---|
| Next.js single-page console | Richer interaction; the brief proposed it | Second toolchain and runtime for a read-mostly view; build-time network access conflicts with REQ-0092 |
| Server-rendered plus a small vanilla script for live updates | Middle ground | This is what was chosen — the "small script" is the SSE subscriber, a few dozen lines with no framework |
| Terminal UI only | No web surface at all | Requirement REQ-0065 asks for a screen an evaluator can watch; a terminal is a poorer fit for evidence panels |

## Revisit trigger

An interaction requirement that server rendering cannot serve — live graph
manipulation, or an operator workflow with substantial client-side state.
