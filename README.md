# IncidentOps Arena

*[中文版](README.zh-CN.md)*

A multi-agent system for production incident localisation and remediation, in which one
agent is paid to disagree with the others.

The expensive incidents are not the ones nobody looked at. They are the ones where
someone looked, found a plausible story, and stopped. A single reasoner reproduces that
failure faithfully: given a log full of `connection timeout`, it will confidently narrate
a database outage, because the first coherent story is the one it tells.

So this system splits evidence gathering by source, and gives one role structural
authority to object — it can demand specific additional evidence, which sends the
investigation back for another round, and it can refuse to let a conclusion proceed.

## What that looks like

The reference scenario, run offline in about a second:

```
Round 1 — five collectors, default queries
  1. A traffic increase exceeded capacity ......................... 0.49   ← wrong
  2. The database connection pool was exhausted ................... 0.40
  3. The database became unavailable .............................. 0.35

  The critic: the top two are 0.09 apart, inside the 0.15 margin. The pool
  explanation needs a saturation metric nobody queried and a change nobody
  looked back far enough to find. Nothing rules out the rivals.
  → four demands issued, investigation returns to collection

Round 2 — demands answered
  1. The pool size was reduced 20 → 2, exhausting the pool ......... 0.94   ← correct
  2. The database became unavailable .............................. 0.37
  3. A traffic increase exceeded capacity ......................... 0.29   (counter-evidence)

  → set_config order-api DB_POOL_SIZE 20, medium risk, HALTED for approval
  → after approval: 5xx 0.129 → 0.003, latency 1623ms → 190ms, recovered
```

The critic did not annotate the outcome. It changed it.

## Quick start

```bash
make demo          # the reference scenario end to end, offline, ~1s
make demo-gate     # the same, stopping at the approval gate
make eval          # compare single-agent, no-critic and full flows
make serve         # API and console on http://localhost:8080
make up            # the full demo stack with a live target and Prometheus
```

With Docker, from a published release (`linux/amd64`):

```bash
docker pull ghcr.io/zlrrr/mutil-agent-system:0.1.0
docker run --rm -p 8080:8080 ghcr.io/zlrrr/mutil-agent-system:0.1.0
```

Or build it yourself:

```bash
docker build -f deploy/docker/Dockerfile -t incidentops-arena:0.1.0-mvp .
docker run --rm -p 8080:8080 incidentops-arena:0.1.0-mvp
```

Then open <http://localhost:8080>, pick a fault case and press **Open investigation**.

Each release also attaches the image as a loadable tarball and checksums every asset, so
it stays usable without registry access — see [the manual](docs/manual/user-manual.en.md)
for that path.

## Architecture

Three planes with a one-way dependency rule: the Control Plane depends on the Reasoning
Plane, which depends on the Signal Plane's *ports* — never on their adapters. A test
enforces this by parsing imports.

```
Control    Orchestrator · Policy · Event log · Case projection · API/SSE/Console
              │ decides what happens next; the only writer; the only actuator caller
Reasoning  Agent roles · Reasoner port (rule | model) · Scoring · Critique rules
              │ decides what is true; never touches a store or an adapter
Signal     MetricSource · LogSource · ChangeSource · TopologySource ·
           KnowledgeSource · Actuator — each with an offline fixture adapter
```

Four decisions carry most of the weight:

**Agents return values, not effects.** Every agent is a pure function of a case
snapshot returning `[]Contribution` — the closed set `AddEvidence`,
`ProposeHypothesis`, `RaiseCritique`, `DemandEvidence`, `ProposeAction`,
`RecordVerification`, `RecordExecution`. The orchestrator is the single writer. Three
properties follow for free: agents are table-testable, parallel collection is safe
without locks, and the event log writes itself.

**The case is a fold over an append-only log.** The live view and a replayed view are
the same function, so replay equality is structural rather than tested into existence.

**Reasoning sits behind a port whose default is deterministic.** A rule engine over a
declarative fault signature catalog — not a stub. The whole flow therefore runs offline,
reproducibly, with no model provider, which is what makes both the test suite and the
demo trustworthy. A model-backed adapter implements the same interface.

**Policy is a chokepoint, not advice.** Shell, SQL, deletion and cross-service bulk
operations are refused as categories before any allowlist is consulted. Policy is
evaluated again in the instant before execution, against the action exactly as it then
stands.

Full detail: [architecture](specs/001-incidentops-arena/architecture.en.md) ·
[high level design](specs/001-incidentops-arena/hld.en.md) ·
[detailed design](specs/001-incidentops-arena/dld.en.md) ·
[decisions](docs/adr/)

## Agent roles

| Role | May emit | Responsibility |
|---|---|---|
| `metrics` | evidence | Anomaly onset, saturation, co-movement — timing, never causation |
| `logs` | evidence | Clustered by normalised template, bounded, with representative samples |
| `change` | evidence | Configuration and deployment history; never rolls anything back |
| `topology` | evidence | Candidate origin versus victim, blast radius |
| `knowledge` | evidence | Runbook retrieval, treated strictly as data |
| `analysis` | hypotheses | Ranked explanations with a mechanism and a decomposed score |
| `critic` | critiques, demands | Challenge, demand, veto — and never propose an answer of its own |
| `remediation` | actions | A complete proposal: risk, rollback, precondition, verification |
| `executor` | executions | The only holder of an actuator reference |
| `verification` | evidence, results | Re-query the triggering signals after the action |

The role capability table is enforced by the orchestrator, not by convention: a
collector that proposes a hypothesis has its contribution rejected and the violation
recorded.

## The claim, computed

```bash
make eval
```

| Mode | Samples | Top-1 accuracy | Top-3 coverage | Evidence kinds | Rounds |
|---|---|---|---|---|---|
| single | 3 | 0% | 100% | 5.0 | 1.0 |
| multi_no_critic | 3 | 0% | 100% | 5.0 | 1.0 |
| multi_with_critic | 3 | 100% | 100% | 6.0 | 2.0 |

Two caveats, printed alongside the numbers rather than buried: the sample is three fault
cases, and the single-agent baseline is given the *same tools and the same default
queries* as the full flow. It models "one context window, one look", not a weaker
toolset — which is the fairest comparison available, and the only one that makes the
result attributable to the flow.

## Specification-driven development

Every artifact here is linked in one machine-checked chain:

```
constitution → goals → requirements → architecture → HLD → DLD → tasks → code
                                          ↘ ADRs      requirements → test cases → tests
```

`sddctl` enforces it. Change a goal and every downstream artifact becomes visibly stale
until revisited — including the exact source files:

```bash
make sdd-impact ID=G-002    # what changing this goal would oblige you to revisit
make sdd-validate           # bilingual parity, traceability and drift
make sdd-matrix             # the requirement-to-source matrix
```

All documentation exists in English and Chinese, and the pairing is enforced: the two
renderings must carry the same version, the same ordered item identifiers and the same
heading skeleton. Editing one language alone fails the build.

Read more: [SDD workflow](docs/sdd-workflow.en.md) ·
[constitution](.sdd/memory/constitution.en.md) ·
[traceability matrix](docs/traceability-matrix.md)

## Repository layout

```
.sdd/                      constitution, templates, governance configuration, lock file
specs/000-sdd-framework/   the governance tool's own specification chain
specs/001-incidentops-arena/
                           charter, requirements, architecture, HLD, DLD, tasks, test plan
docs/                      user manual, ADRs, workflow, checkpoints, generated reports
cmd/                       arena, evalctl, faultctl, sddctl, demo-order-api
internal/domain/           entities, contribution algebra, events, case projection
internal/signal/           six ports, bounds, anomaly detection, clustering, fixtures
internal/catalog/          embedded fault signatures, runbooks and fault cases
internal/reasoner/         scoring, rule reasoner, the six critique rules
internal/agent/            collectors, analysis, critic, remediation, verification, baseline
internal/policy/           allowlists, categorical denials, the executor
internal/orchestrator/     state machine, contribution application, guards, engine
internal/httpapi/          REST, SSE, embedded console
deploy/                    Dockerfile, compose stack, Prometheus configuration
```

## Requirements and status

Go 1.22 or newer. No third-party dependencies — `go.mod` has no `require` block, and CI
asserts it.

Milestone M1 is complete: the reference scenario runs end to end offline, every planned
test passes under the race detector, and the governance tree validates with no drift.

```bash
make check     # vet, the full suite, and the governance gate
```

## Documentation

| Document | English | 中文 |
|---|---|---|
| User manual | [en](docs/manual/user-manual.en.md) | [zh](docs/manual/user-manual.zh.md) |
| SDD workflow | [en](docs/sdd-workflow.en.md) | [zh](docs/sdd-workflow.zh.md) |
| Constitution | [en](.sdd/memory/constitution.en.md) | [zh](.sdd/memory/constitution.zh.md) |
| Goal charter | [en](specs/001-incidentops-arena/charter.en.md) | [zh](specs/001-incidentops-arena/charter.zh.md) |
| Requirements | [en](specs/001-incidentops-arena/spec.en.md) | [zh](specs/001-incidentops-arena/spec.zh.md) |
| Architecture | [en](specs/001-incidentops-arena/architecture.en.md) | [zh](specs/001-incidentops-arena/architecture.zh.md) |
| High level design | [en](specs/001-incidentops-arena/hld.en.md) | [zh](specs/001-incidentops-arena/hld.zh.md) |
| Detailed design | [en](specs/001-incidentops-arena/dld.en.md) | [zh](specs/001-incidentops-arena/dld.zh.md) |
| Test plan | [en](specs/001-incidentops-arena/test-plan.en.md) | [zh](specs/001-incidentops-arena/test-plan.zh.md) |
| Task breakdown | [en](specs/001-incidentops-arena/tasks.en.md) | [zh](specs/001-incidentops-arena/tasks.zh.md) |

## Licence

See [LICENSE](LICENSE).
