---
id: ADR-DOC-006
lang: en
counterpart: 006-declarative-catalog.zh.md
doc_version: 1.0.0
status: approved
stage: decide
---

<!-- sdd:item id=ADR-006 stage=decide status=approved derives_from=ARC-006 -->
# ADR-006 — Declarative fault signature and case catalog rather than hard-coded scenarios

**Status.** accepted

**Date.** 2026-08-05

## Context

Two things must grow over the life of this system: the set of fault signatures the
reasoner can recognise, and the set of scenarios the evaluation runs against. REQ-0082
requires adding a case without touching agent code.

If either lives in Go source, growth means recompiling and, worse, means the person
adding a scenario must understand the agent internals. It also makes overfitting
invisible: a signature written inside the function that uses it is indistinguishable
from a special case.

There is a subtler risk. If the fixture data for a scenario and the signature that
recognises it are written together in code, a test can pass because both encode the
same assumption. Keeping them as separate data files, loaded by the same generic
machinery, makes that coupling visible in review.

## Decision

We will express both as embedded JSON data:

- **Fault signatures** declare required evidence patterns, the mechanism narrative
  template, the discriminating evidence that would refute them, and per-term scoring
  hints.
- **Fault cases** declare the alert, the fixture signal set (series, log lines,
  changes, topology), the expected root cause and the expected remediation.

Both are embedded in the binary and loaded by generic code. Adding either is a data
change.

## Consequences

### Positive

- A new scenario is a reviewable data diff; the evaluation picks it up with no code
  change.
- Signature coverage is auditable — one can read the catalog and see what the system
  can and cannot recognise, which is exactly what a reviewer should be able to check.
- The same fixture data drives the unit tests, the end-to-end scenario and the
  evaluation, so they cannot drift apart.
- Overfitting is measurable: signatures and cases are separate files, so a signature
  that only ever matches one case is visible.

### Negative

- A declarative pattern language is less expressive than code. Signatures that need
  genuinely novel matching logic require extending the matcher, which is a code change.
- Two schemas to document and validate, with their own error messages.
- JSON is a poor medium for long mechanism narratives; they are terse and templated.

### Neutral

- The catalog is the natural place a model adapter would later *propose* additions to,
  which keeps the extension path open.

## Alternatives considered

| Alternative | Why it was attractive | Why it lost |
|---|---|---|
| Signatures as Go functions | Full expressiveness; no schema | Recompilation to extend, and a signature becomes indistinguishable from a special case in review |
| Scenarios as test fixtures only | Simplest for testing | The evaluation and the demo would need their own copies, which drift |
| External data files loaded from disk | Editable without rebuild | Contradicts ARC-014's single self-contained artifact; a missing file becomes a runtime failure mode |

## Revisit trigger

A signature that cannot be expressed in the pattern language without contorting it, or
a catalog large enough that embedding is no longer appropriate.
