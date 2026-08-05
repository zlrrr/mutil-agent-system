---
id: CHARTER-001
lang: en
counterpart: charter.zh.md
doc_version: 1.0.0
status: approved
stage: charter
---

# IncidentOps Arena — Goal Charter

## 1. Problem

When a production service degrades, the person on call is doing evidence work under
time pressure: reading alerts, correlating metrics, grepping logs, checking what
shipped in the last hour, and deciding whether the service they are staring at is the
cause or a victim. The expensive failures are not the ones where nobody looked — they
are the ones where someone looked, found a plausible story, and stopped.

A single language model reproduces that failure mode faithfully. Given a log full of
`connection timeout`, it will confidently narrate a database outage, because the first
coherent story is the one it tells. Nothing in a single-agent loop is structurally
obliged to ask *what else would produce this evidence?*

Two things follow. First, evidence gathering should be decomposed by source, so no
single context window decides which evidence exists. Second, somebody has to be paid
to disagree — and that role needs actual authority, not a politely worded caveat.

## 2. Outcome statement

An operator receives an alert and, without leaving one screen, sees: which agent
gathered which evidence, the ranked root-cause hypotheses with per-term score
breakdowns, the challenges an adversarial reviewer raised, the additional evidence
those challenges forced, why the runner-up explanations were eliminated, a
risk-classified remediation plan that stopped at an approval gate, and — after they
approve — verification that the metrics actually recovered. Every claim on that screen
carries an evidence identifier that resolves to the raw query behind it.

## 3. Goals

<!-- sdd:item id=G-001 stage=charter status=approved derives_from=CON-005,CON-011 priority=P0 -->
### G-001 — Root causes are defensible, not merely produced

**Statement.** Every hypothesis the system reports is bound to stored evidence from
named sources, carries a decomposed confidence score, and is rejected outright by the
orchestrator if it is not.

**Measure.** Zero hypotheses reach a report without at least two distinct evidence
kinds; every score renders as a per-term breakdown that sums to the reported value.

**Priority.** P0

**Non-goal boundary.** This goal does not require the top hypothesis to be correct on
every input; it requires that an incorrect one be visibly unsupported.

<!-- sdd:item id=G-002 stage=charter status=approved derives_from=CON-005 priority=P0 -->
### G-002 — The adversarial reviewer has real authority

**Statement.** A dedicated critic role can force additional evidence collection,
demote or reject the leading hypothesis, and block progression to remediation. Its
objections are structured records, not prose appended to a report.

**Measure.** In the reference scenario the critic changes the outcome: it forces at
least one additional collection round, and the ranking after critique differs from the
ranking before it.

**Priority.** P0

**Non-goal boundary.** The critic does not propose root causes of its own; it
challenges, demands and vetoes.

<!-- sdd:item id=G-003 stage=charter status=approved derives_from=CON-009,CON-012 priority=P0 -->
### G-003 — Remediation is risk-classified and gated

**Statement.** Every proposed action carries a risk class, a rollback, a precondition
and a verification step. Read-only tools run autonomously; anything that mutates a
target system above the low-risk class halts the state machine until a human decides.

**Measure.** No medium- or high-risk action can execute without a recorded approval;
attempts to bypass the gate are refused by the policy engine and logged, and the
refusal is enforced immediately before execution rather than at planning time.

**Priority.** P0

<!-- sdd:item id=G-004 stage=charter status=approved derives_from=CON-011 priority=P0 -->
### G-004 — Recovery is verified and the investigation replays

**Statement.** After an approved action executes, the system re-queries the signals
that triggered the incident and records whether they recovered. The complete
investigation — every agent call, tool call, state transition and policy decision —
reconstructs from the event log alone.

**Measure.** The final report is regenerated from the event log and matches the report
produced live, field for field.

**Priority.** P0

<!-- sdd:item id=G-005 stage=charter status=approved derives_from=CON-005,CON-007 priority=P1 -->
### G-005 — The multi-agent flow demonstrably beats a single-agent baseline

**Statement.** The same fault scenarios run in three modes — single agent, multi-agent
without critic, and the full flow — over the same evidence, and the comparison is
computed rather than asserted.

**Measure.** An evaluation command emits top-1 accuracy, top-3 coverage, evidence-kind
completeness and critic-corrected-hypothesis counts per mode, over at least three
reproducible fault cases.

**Priority.** P1

**Non-goal boundary.** The claim is about this benchmark, not about production SRE work
in general; the report states the sample size next to every number.

<!-- sdd:item id=G-006 stage=charter status=approved derives_from=CON-007 priority=P0 -->
### G-006 — One complete business scenario runs end to end, reproducibly

**Statement.** The order-API connection-pool exhaustion scenario runs from alert
ingestion to signed-off report, deterministically, with no network access and no model
provider.

**Measure.** A single command reproduces the scenario; repeated runs produce identical
hypothesis rankings, identical evidence identifiers and identical report content.

**Priority.** P0

<!-- sdd:item id=G-007 stage=charter status=approved derives_from=CON-008 priority=P1 -->
### G-007 — Delivered as a container image with a bilingual manual

**Statement.** The deliverable is a container image plus a compose stack that brings up
the API, the console and the demo environment, accompanied by a user manual in English
and Chinese covering installation, the demo walkthrough, the API, configuration and
troubleshooting.

**Measure.** A reader who has never seen the repository can bring the demo up and drive
the reference scenario using the manual alone.

**Priority.** P1

<!-- sdd:item id=G-008 stage=charter status=approved derives_from=CON-006,CON-010 priority=P0 -->
### G-008 — The MVP passes every planned test, offline

**Statement.** Milestone M1 is reached when every planned test case executes and passes
with no network, no credentials and no external service, and the governance tree is
sealed with no drift.

**Measure.** `go test ./...` and `sddctl validate` both exit zero in a network-isolated
environment.

**Priority.** P0

## 4. Non-goals

| # | Excluded | Why | Revisit when |
|---|---|---|---|
| N1 | Autonomous repair of real production systems | The approval gate is the product, not an obstacle to remove | Never for this system; a different risk model would be required |
| N2 | Cloud and SaaS connectors (Datadog, PagerDuty, Kubernetes, cloud APIs) | Each adds credentials and a live dependency that breaks a demo | A deployment target is chosen and its credentials are managed |
| N3 | Vector database and knowledge graph | The knowledge base is small enough for exact search; a vector store would add a service without changing an outcome | The runbook corpus exceeds a few hundred documents |
| N4 | Arbitrary shell or free-form SQL for agents | Unbounded blast radius, and unnecessary — every real action is expressible as a typed tool call | Never |
| N5 | A general-purpose SRE assistant for all operational work | Breadth would dilute the adversarial mechanism that is the point | The adversarial flow is proven on the narrow scope |
| N6 | Multi-tenant authentication and authorisation | Single-operator demo scope; adding it now would obscure the flow | The system is deployed beyond a single trusted operator |

## 5. Milestones

| ID | Milestone | Goals covered | Exit criteria |
|---|---|---|---|
| M1 | Working MVP passing all basic tests | G-001, G-002, G-003, G-004, G-006, G-008 | Reference scenario runs offline end to end; every planned test passes; `sddctl validate` clean |
| M2 | Container delivery and manual | G-007 | Image builds; compose stack starts API, console and demo services; bilingual manual complete |
| M3 | Evaluation and baseline comparison | G-005 | Three fault cases run in three modes; comparison table generated from real runs |
| M4 | Live signal adapters | G-001 | Prometheus and container-log adapters exercised against the compose stack, fixtures still passing |
| M5 | Extended fault library and model-backed reasoning | G-002, G-005 | Additional cases including a misleading-log case; model adapter behind the reasoner port |

## 6. Scope corrections to the original project brief

The original brief (`project.md`) is the source of the problem statement and the agent
role decomposition, both of which are carried forward. Three decisions in it are
amended here, under the amendment authority the brief itself grants.

| # | Brief said | This charter says | Why |
|---|---|---|---|
| A1 | Python + FastAPI backend, LangGraph orchestration | Go, standard library only | Directed by the project owner. It also serves CON-007 and CON-008: a single static binary, no dependency tree to resolve at build time, and an orchestrator whose state machine is explicit code rather than framework configuration |
| A2 | Next.js + TypeScript console | Server-rendered console embedded in the binary | The console's job is to make the agent timeline and evidence chain visible. A build toolchain and a second runtime would add delivery risk without adding visibility |
| A3 | Live Prometheus, Docker logs and git as first-class sources | The same sources behind ports, with a deterministic fixture adapter as the default | CON-007. The demo and the test suite must not depend on a scrape target being healthy; live adapters remain, selected by configuration |

Two decisions in the brief are explicitly retained: the agent role decomposition of
section 8, and the evidence-weighted scoring model of section 12.1. Both are carried
into the requirements without change of intent.

## 7. Open questions requiring human authority

| # | Question | Blocked work | Default assumed if unanswered |
|---|---|---|---|
| Q1 | Which model provider, if any, backs the optional reasoning adapter | Nothing in M1..M4 | No provider is wired; the port ships with the deterministic adapter and a documented interface |
| Q2 | Is the demo ever to be pointed at a real environment | Live actuator adapters | Actuators mutate only the demo compose stack; the real-system adapter is not implemented |
| Q3 | Target audience for the manual: operator or evaluator | Manual emphasis | Written for an evaluator who must also operate it — installation first, then the walkthrough |
