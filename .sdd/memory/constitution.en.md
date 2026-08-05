---
id: CONSTITUTION
lang: en
counterpart: constitution.zh.md
doc_version: 1.0.0
status: approved
stage: constitution
---

# IncidentOps Arena — Development Constitution

> This document is the root of the specification tree. Nothing in this repository
> may contradict it. Every downstream artifact — goals, requirements, architecture,
> designs, tasks, code, tests and shipped artifacts — derives from an article here
> and is traceable back to it via `sddctl`.

## 0. Scope

This constitution governs **Spec-Driven Development (SDD)** for the IncidentOps Arena
project: a multi-agent system for production incident localisation and remediation.
It is machine-checked. `sddctl validate` fails the build when an article is violated
in a way a tool can detect.

---

## Articles

<!-- sdd:item id=CON-001 stage=constitution status=approved -->
### CON-001 — Specification precedes implementation

No production code is written before the detailed design item it implements exists,
is approved and is sealed. Every source file that carries logic declares the design
item it realises with an `// sdd:impl <ID>` anchor. Code without an anchor pointing at
an existing design item is an *orphan* and fails validation.

Exploratory spikes are permitted but live outside `cmd/` and `internal/`, are never
merged, and never become the deliverable.

<!-- sdd:item id=CON-002 stage=constitution status=approved -->
### CON-002 — One directed artifact chain, no shortcuts

Artifacts form a single directed acyclic graph with a fixed stage order:

```
CON → G (goals) → REQ (requirements) → ARC (architecture) ─┬→ HLD → DLD → T (tasks) → code
                                        ADR (decisions) ───┘
                        REQ ──────────────────────────────→ TC (test cases) → tests
```

An item may only declare `derives_from` parents whose stage is a legal parent stage
of its own (defined in `.sdd/config.json`). Skipping a stage — for example a `DLD`
item deriving directly from a `REQ` — is a validation error. This is what makes
change propagation total: there is no path from a goal to a line of code that
bypasses the chain.

<!-- sdd:item id=CON-003 stage=constitution status=approved -->
### CON-003 — Change cascades downward and is never silently absorbed

Every item's normative content is hashed and recorded in `.sdd/sdd.lock.json` by
`sddctl seal`. When an item's content changes, every transitive descendant becomes
**STALE**. A stale tree cannot pass `sddctl validate`, cannot pass CI, and cannot be
released.

Resolving staleness requires a human or agent to actually revisit each descendant and
then re-seal. `sddctl drift` prints the full impact set — including the exact source
files and test files affected — before any work begins. Amending a goal is therefore
cheap to *decide* and honest about what it *costs*.

<!-- sdd:item id=CON-004 stage=constitution status=approved -->
### CON-004 — Documentation is bilingual and structurally identical

Every specification and user-facing document exists as an `<base>.en.md` /
`<base>.zh.md` pair. The two files:

1. carry the same `doc_version`;
2. contain exactly the same ordered set of `sdd:item` identifiers;
3. contain the same ordered heading skeleton.

Identifiers, code, commands, JSON keys and API paths are language-neutral and appear
verbatim in both. Prose is genuinely translated, never machine-echoed. Editing one
language without the other is a validation error — the pair is a single artifact with
two renderings.

<!-- sdd:item id=CON-005 stage=constitution status=approved -->
### CON-005 — Evidence before conclusion

This is both a product rule and a process rule.

*Product*: no agent may assert a root cause that is not bound to stored evidence
identifiers. An unsupported hypothesis is rejected by the orchestrator, not merely
scored low.

*Process*: no design claim ships as "done" without a named checkpoint, a test case
identifier, and a recorded run of that test. "It should work" is not a checkpoint.

<!-- sdd:item id=CON-006 stage=constitution status=approved -->
### CON-006 — Test cases are planned before the code they judge

For every key implementation point, the test case (`TC-xxxx`) is written into the test
plan *before* the corresponding code. Each test function declares `// sdd:verify <TC>`.
A requirement with no reachable test case is reported by `sddctl trace`. The MVP gate
is: **every P0 requirement has at least one passing test.**

<!-- sdd:item id=CON-007 stage=constitution status=approved -->
### CON-007 — Deterministic by default, intelligent by extension

The system must run end-to-end with no network, no API key and no model provider, and
produce identical output for identical input. Model-backed reasoning is an *adapter*
behind a port, never a load-bearing default.

Rationale: an autonomous ("lights-out") development loop cannot verify a
non-deterministic system, and a demo that depends on a live model endpoint fails in
the room. Determinism is what makes both the CI gate and the demo trustworthy.

<!-- sdd:item id=CON-008 stage=constitution status=approved -->
### CON-008 — Dependencies are a liability, not a feature

The core service depends on the Go standard library only. Any third-party module
requires an ADR stating what it buys and what breaks without it. External systems
(Prometheus, container logs, git) are reached through ports with an offline fixture
adapter, so the test suite never requires them.

<!-- sdd:item id=CON-009 stage=constitution status=approved -->
### CON-009 — Automatic reads, approved writes

Read-only tools execute autonomously. Any tool that mutates a target system is
classified by risk, and medium/high-risk actions halt the state machine at an approval
gate until a human decides. There is no arbitrary shell, no free-form SQL, and no
cross-service bulk operation — not as a prompt instruction, but enforced by the policy
engine before execution.

<!-- sdd:item id=CON-010 stage=constitution status=approved -->
### CON-010 — Lights-out execution, explicit escalation

Development proceeds autonomously as far as the specification allows. Human input is
requested only when a decision is genuinely outside the specification's authority:
an irreversible action, a scope change, a credential, or a contradiction between
articles. Everything else — including revising downstream artifacts after an upstream
change — is executed without asking.

When escalation is unavoidable, the loop records the blocking question, completes all
work that does not depend on the answer, and reports what was left undone.

<!-- sdd:item id=CON-011 stage=constitution status=approved -->
### CON-011 — Every state transition is observable and replayable

Every agent invocation, tool call, state transition and policy decision emits an
immutable event carrying: actor, input digest, output digest, duration and outcome.
The final report of any investigation must be reconstructable from the event log
alone. An unobservable step is treated as a defect.

<!-- sdd:item id=CON-012 stage=constitution status=approved -->
### CON-012 — Untrusted data never becomes instruction

Logs, commit messages, tickets, runbooks and any other retrieved content are data.
They enter the system exclusively as evidence payloads and can never alter agent
instructions, tool selection or policy. The executor accepts structured actions only —
never natural language — and the policy engine performs the final check immediately
before execution, not at planning time.

---

## Amendment procedure

1. Edit the article in **both** language files, bumping `doc_version`.
2. Run `sddctl drift` to obtain the full downstream impact set.
3. Update every affected descendant artifact.
4. Run `sddctl validate && go test ./...`.
5. Run `sddctl seal` to record the new baseline.

An amendment that leaves the tree stale is not an amendment; it is a broken build.

## Enforcement summary

| Article | Enforced by |
|---|---|
| CON-001 | `sddctl trace` — orphan code anchors |
| CON-002 | `sddctl validate` — illegal parent stage |
| CON-003 | `sddctl drift` — stale descendants |
| CON-004 | `sddctl lint` — bilingual parity |
| CON-005 | orchestrator guard + `TC-0021`, `TC-0022` |
| CON-006 | `sddctl trace` — untested requirements |
| CON-007 | `go test ./...` offline in CI |
| CON-008 | `go.mod` has no `require` block; CI asserts it |
| CON-009 | policy engine + `TC-0040`..`TC-0043` |
| CON-010 | this loop's checkpoint records in `docs/checkpoints.md` |
| CON-011 | event log assertions in `TC-0050` |
| CON-012 | policy engine final check + `TC-0044` |
