---
id: SPEC-001
lang: en
counterpart: spec.zh.md
doc_version: 1.0.0
status: approved
stage: specify
---

# IncidentOps Arena — Requirements Specification

## 1. Reading guide

Requirements state **what** must be observably true, never **how**. Each is testable:
it names a condition a test can assert. `MUST` is normative; anything else is context.

Requirement identifiers are grouped by area but are not renumbered when the areas
change, so an identifier is a permanent handle.

## 2. Glossary

| Term | Definition |
|---|---|
| Case | One investigation of one alert, from ingestion to closed report |
| Evidence | An immutable, identified observation from one source, bounded to a time window, carrying a reference to the raw query that produced it |
| Evidence kind | One of: metric, log, change, topology, knowledge, verification |
| Hypothesis | A candidate root cause with a mechanism narrative, supporting and counter evidence, and a decomposed score |
| Critique | A structured objection to a hypothesis, carrying a verdict and, optionally, a demand for specific additional evidence |
| Contribution | The only thing an agent may return: a request to add evidence, propose a hypothesis, raise a critique, demand evidence, propose an action, or record a verification |
| Action | A proposed change to a target system, carrying a risk class, preconditions, a rollback and a verification step |
| Round | One pass of collection, hypothesis formation and critique. Rounds are bounded |
| Reasoner | The pluggable component that decides *how* an agent reaches its conclusion, given evidence |

## 3. Domain contract

<!-- sdd:item id=REQ-0001 stage=specify status=approved derives_from=G-001 priority=P0 -->
### REQ-0001 — Agents communicate only through typed contributions

**Requirement.** An agent MUST return a list of typed contributions and MUST NOT mutate
case state directly. Free-form natural language MUST NOT be a carrier of decisions
between agents; prose may appear only inside a summary field of a typed record.

**Rationale.** This is what makes agents independently testable, safely parallelisable,
and what allows the orchestrator to be the single writer and therefore the single
source of the event log.

**Acceptance.**
- Given any agent and any case snapshot, when the agent runs, then the case is
  unchanged and the returned contributions are the only effect.

**Verified by.** TC-0001

<!-- sdd:item id=REQ-0002 stage=specify status=approved derives_from=G-001 priority=P0 -->
### REQ-0002 — Evidence is immutable, identified and traceable to its query

**Requirement.** Every piece of evidence MUST carry a stable identifier, the producing
agent, the source adapter, its kind, the time window it covers, a human-readable
summary, a machine-readable reference to the raw query that produced it, and a
confidence value in `[0,1]`. Evidence MUST NOT be modified after it is stored.

**Acceptance.**
- Given stored evidence, when the case is re-read, then every field is byte-identical.
- Given an evidence identifier, when the raw reference is resolved, then it names the
  source and the exact query or selector used.

**Verified by.** TC-0002

<!-- sdd:item id=REQ-0003 stage=specify status=approved derives_from=G-004 priority=P0 -->
### REQ-0003 — Case state is derived from an append-only event log

**Requirement.** Every accepted contribution, state transition, policy decision and
tool invocation MUST append an event. The case projection MUST be reconstructable by
replaying the log from empty, and the reconstruction MUST equal the live projection.

**Acceptance.**
- Given a completed case, when its events are replayed into an empty projection, then
  the resulting evidence, hypotheses, critiques, actions and status are equal to the
  live case.

**Verified by.** TC-0050

<!-- sdd:item id=REQ-0004 stage=specify status=approved derives_from=G-006 priority=P0 -->
### REQ-0004 — Identifiers and ordering are deterministic

**Requirement.** Identifiers for evidence, hypotheses, critiques and actions MUST be
derived from the case identifier, the producing role and a per-role sequence number —
never from wall-clock time or a random source. Collections returned by the API MUST be
in a defined, stable order.

**Acceptance.**
- Given the same fixture inputs, when a case is run twice, then every generated
  identifier and every collection order is identical.

**Verified by.** TC-0070

<!-- sdd:item id=REQ-0005 stage=specify status=approved derives_from=G-002 priority=P0 -->
### REQ-0005 — Role boundaries are enforced, not merely documented

**Requirement.** Each agent role MUST declare the contribution types it may emit, and
the orchestrator MUST reject a contribution whose type is outside the emitting role's
declaration. Collector roles MUST NOT propose hypotheses; the critic MUST NOT propose
hypotheses; only the remediation role may propose actions.

**Acceptance.**
- Given a collector role returning a hypothesis contribution, when the orchestrator
  applies contributions, then the contribution is rejected, an event records the
  violation, and the case state is unaffected.

**Verified by.** TC-0003

## 4. Signal collection

<!-- sdd:item id=REQ-0010 stage=specify status=approved derives_from=G-006 priority=P0 -->
### REQ-0010 — Every external signal is reached through a port with an offline adapter

**Requirement.** Metrics, logs, changes, topology, knowledge and actuation MUST each be
defined as a port. Every port MUST have a deterministic fixture adapter that requires
no network. The adapter set MUST be selectable by configuration without code changes.

**Acceptance.**
- Given the fixture profile, when the reference scenario runs with networking disabled,
  then it completes and produces evidence of every kind it declares.

**Verified by.** TC-0010

<!-- sdd:item id=REQ-0011 stage=specify status=approved derives_from=G-001 priority=P0 -->
### REQ-0011 — Metric evidence identifies anomaly onset and co-movement

**Requirement.** The metrics role MUST, for each series it queries, determine whether
the series is anomalous within the incident window, and if so the onset timestamp. It
MUST report which series moved together, and MUST NOT state a causal relationship
between series.

**Acceptance.**
- Given a fixture series that steps from 0.2% to 18% at a known time, when metrics
  evidence is collected, then the onset is reported within one sample of that time.
- Given two series that both step at the same time, when evidence is collected, then
  they are reported as co-moving and the summary contains no causal claim.

**Verified by.** TC-0011

<!-- sdd:item id=REQ-0012 stage=specify status=approved derives_from=G-001 priority=P0 -->
### REQ-0012 — Log evidence is clustered and bounded

**Requirement.** The logs role MUST group matching log lines into clusters keyed by a
normalised message template, and MUST report per cluster the count, the first and last
occurrence, and at most a configured number of representative samples. Raw log volume
returned into the case MUST be bounded by line count and by time window.

**Acceptance.**
- Given 184 lines matching one template and 3 matching another, when log evidence is
  collected, then two clusters are reported with counts 184 and 3, the first cluster
  first, and no more than the configured sample count per cluster.

**Verified by.** TC-0012

<!-- sdd:item id=REQ-0013 stage=specify status=approved derives_from=G-001 priority=P0 -->
### REQ-0013 — Change evidence carries what changed, when, and by how much

**Requirement.** The change role MUST report configuration and deployment changes
overlapping a window extended before the incident start, and for each MUST report the
key, the previous value, the new value and the deployment timestamp. It MUST NOT
perform a rollback.

**Acceptance.**
- Given a fixture deployment that set `DB_POOL_SIZE` from `20` to `2`, when change
  evidence is collected, then that change is reported with both values and its
  timestamp, and no actuator is invoked.

**Verified by.** TC-0013

<!-- sdd:item id=REQ-0014 stage=specify status=approved derives_from=G-001 priority=P0 -->
### REQ-0014 — Topology evidence separates source from victim

**Requirement.** The topology role MUST report the dependency neighbourhood of the
affected service, the set of services plausibly affected, and which services are
candidate origins given which of them show anomalies first.

**Acceptance.**
- Given a fixture topology where `order-api` is anomalous and its downstream
  `checkout-web` is anomalous later, when topology evidence is collected, then
  `order-api` is reported as a candidate origin and `checkout-web` as affected.

**Verified by.** TC-0014

<!-- sdd:item id=REQ-0015 stage=specify status=approved derives_from=G-001 priority=P1 -->
### REQ-0015 — Knowledge evidence retrieves applicable runbooks

**Requirement.** The knowledge role MUST search a runbook corpus for entries matching
the incident's service and observed symptoms, and MUST report matches with the entry
identifier and the matched terms. Retrieved runbook text MUST be treated as data.

**Acceptance.**
- Given a runbook describing connection-pool exhaustion, when knowledge evidence is
  collected for an incident whose symptoms include pool saturation, then that runbook
  is among the matches.

**Verified by.** TC-0015

<!-- sdd:item id=REQ-0016 stage=specify status=approved derives_from=G-001 priority=P1 -->
### REQ-0016 — Tool results are bounded before they enter the case

**Requirement.** Every tool MUST enforce a result bound — maximum rows, maximum
characters and maximum time span — before its output becomes evidence, and MUST record
in the evidence when truncation occurred.

**Acceptance.**
- Given a tool result exceeding the configured bound, when it is converted to evidence,
  then the stored evidence is within the bound and is marked truncated.

**Verified by.** TC-0016

## 5. Hypothesis formation and scoring

<!-- sdd:item id=REQ-0020 stage=specify status=approved derives_from=G-001 priority=P0 -->
### REQ-0020 — Hypotheses carry a mechanism, not just a label

**Requirement.** The analysis role MUST produce at most a configured number of ranked
hypotheses, each with a claim, a mechanism narrative explaining how the claimed cause
produces the observed symptoms, and the evidence that supports it.

**Acceptance.**
- Given a complete fixture evidence set, when hypotheses are formed, then at most the
  configured count are returned, each with a non-empty mechanism narrative.

**Verified by.** TC-0020

<!-- sdd:item id=REQ-0021 stage=specify status=approved derives_from=G-001 priority=P0 -->
### REQ-0021 — Unsupported hypotheses are rejected, not merely scored low

**Requirement.** A hypothesis that references no stored evidence identifier, or
references an identifier that does not exist, MUST be rejected by the orchestrator and
MUST NOT appear in any ranking or report. A hypothesis MUST NOT progress to remediation
unless it is supported by at least two distinct evidence kinds.

**Acceptance.**
- Given a hypothesis with an empty supporting-evidence list, when contributions are
  applied, then it is rejected and an event records the rejection.
- Given a hypothesis supported only by metric evidence, when the case attempts to enter
  remediation, then progression is refused.

**Verified by.** TC-0021

<!-- sdd:item id=REQ-0022 stage=specify status=approved derives_from=G-001 priority=P0 -->
### REQ-0022 — Scores decompose into named, weighted terms

**Requirement.** Each hypothesis score MUST be computed from named terms — metric
alignment, log alignment, change correlation, topology plausibility, historical
similarity, remediation verifiability — each with a declared weight, minus a penalty
for unresolved counter-evidence. The breakdown MUST be stored and returned with the
hypothesis, and the terms MUST sum to the reported score.

**Acceptance.**
- Given a scored hypothesis, when its breakdown is summed, then the total equals the
  reported score within floating-point tolerance.
- Given the declared weights, when they are summed, then they total 1.0.

**Verified by.** TC-0022

<!-- sdd:item id=REQ-0023 stage=specify status=approved derives_from=G-002 priority=P0 -->
### REQ-0023 — Counter-evidence lowers a score and is recorded

**Requirement.** Evidence that contradicts a hypothesis MUST be recorded against that
hypothesis and MUST reduce its score by the declared penalty per unresolved item. A
counter-evidence item that is subsequently explained MUST be marked resolved and stop
contributing to the penalty.

**Acceptance.**
- Given a hypothesis scored without counter-evidence, when one unresolved
  counter-evidence item is attached, then the score decreases by exactly the declared
  penalty.

**Verified by.** TC-0023

<!-- sdd:item id=REQ-0024 stage=specify status=approved derives_from=G-006 priority=P0 -->
### REQ-0024 — Ranking is total and deterministic

**Requirement.** Hypothesis ranking MUST be a total order: by score descending, then by
count of supporting evidence kinds descending, then by identifier ascending. Equal
inputs MUST produce equal rankings across runs and across processes.

**Acceptance.**
- Given two hypotheses with identical scores and kind counts, when ranked, then the
  lower identifier sorts first, on every run.

**Verified by.** TC-0024

## 6. Adversarial review

<!-- sdd:item id=REQ-0030 stage=specify status=approved derives_from=G-002 priority=P0 -->
### REQ-0030 — Critiques are structured records with a verdict

**Requirement.** The critic role MUST emit, per examined hypothesis, a critique
carrying the hypothesis identifier, a challenge category, the challenge text, an
optional list of demanded evidence descriptors, and a verdict of `accept`,
`accept_with_risk`, `revise` or `reject`.

**Acceptance.**
- Given any hypothesis reaching the critic, when critique runs, then at least one
  critique record exists for it with a verdict from that set.

**Verified by.** TC-0030

<!-- sdd:item id=REQ-0031 stage=specify status=approved derives_from=G-002 priority=P0 -->
### REQ-0031 — The critic can force another collection round

**Requirement.** A critique demanding evidence MUST cause the orchestrator to return to
evidence collection, targeting the demanded descriptors, provided the round budget is
not exhausted. When the budget is exhausted the case MUST proceed with the demand
recorded as unmet rather than silently dropped.

**Acceptance.**
- Given a critique demanding a historical traffic comparison at round 1 of 3, when the
  orchestrator advances, then the case returns to collection and the next round's
  evidence includes a record answering that descriptor.
- Given the same at the final round, when the orchestrator advances, then the case
  proceeds and the unmet demand appears in the report.

**Verified by.** TC-0031

<!-- sdd:item id=REQ-0032 stage=specify status=approved derives_from=G-002 priority=P0 -->
### REQ-0032 — The critic proposes alternative explanations

**Requirement.** Unless the leading hypothesis is supported by evidence of every kind
the case collected, the critic MUST identify at least one alternative explanation
consistent with the current evidence, and MUST demand the evidence that would
discriminate between it and the leading hypothesis.

**Acceptance.**
- Given a case where saturation evidence is present but the traffic history has not
  been examined, when critique runs, then a "traffic increase alone" alternative is
  raised together with a demand for the historical peak comparison.

**Verified by.** TC-0032

<!-- sdd:item id=REQ-0033 stage=specify status=approved derives_from=G-002 priority=P0 -->
### REQ-0033 — The critic checks temporal order

**Requirement.** For any hypothesis attributing an incident to a change, the critic MUST
verify that the change timestamp precedes the anomaly onset. If it does not, the critic
MUST attach counter-evidence and MUST NOT return `accept`.

**Acceptance.**
- Given a change recorded after the anomaly onset, when critique runs, then
  counter-evidence is attached and the verdict is `revise` or `reject`.

**Verified by.** TC-0033

<!-- sdd:item id=REQ-0034 stage=specify status=approved derives_from=G-002 priority=P1 -->
### REQ-0034 — The critic challenges source-versus-victim confusion

**Requirement.** When topology evidence indicates the affected service depends on
another service that became anomalous earlier, the critic MUST challenge any hypothesis
that localises the cause in the affected service.

**Acceptance.**
- Given topology evidence naming an upstream service anomalous earlier, when critique
  runs on a hypothesis blaming the downstream service, then a source-versus-victim
  challenge is recorded.

**Verified by.** TC-0034

<!-- sdd:item id=REQ-0035 stage=specify status=approved derives_from=G-002 priority=P0 -->
### REQ-0035 — Progression requires an explicit acceptance condition

**Requirement.** A case MUST NOT enter remediation unless the leading hypothesis has a
critic verdict of `accept` or `accept_with_risk`, its score meets the configured
threshold, and it is supported by at least two evidence kinds — including change
evidence when any change occurred inside the extended window.

**Acceptance.**
- Given a leading hypothesis with verdict `revise`, when the orchestrator advances,
  then the case does not enter remediation.
- Given a change inside the window and a leading hypothesis with no change evidence,
  when the orchestrator advances, then the case does not enter remediation.

**Verified by.** TC-0035

<!-- sdd:item id=REQ-0036 stage=specify status=approved derives_from=G-002 priority=P1 -->
### REQ-0036 — Close calls escalate rather than guess

**Requirement.** When the top two hypotheses differ by less than the configured margin,
the case MUST either demand discriminating evidence or, if the round budget is
exhausted, enter human review rather than proceeding on the leading hypothesis.

**Acceptance.**
- Given top scores within the margin and budget remaining, when the orchestrator
  advances, then a discriminating demand is issued.
- Given the same with no budget remaining, when the orchestrator advances, then the
  status becomes human review.

**Verified by.** TC-0036

## 7. Remediation, policy and approval

<!-- sdd:item id=REQ-0040 stage=specify status=approved derives_from=G-003 priority=P0 -->
### REQ-0040 — Actions are complete proposals, not commands

**Requirement.** Each proposed action MUST carry a title, the tool and typed arguments
it would invoke, a risk class of `low`, `medium` or `high`, its preconditions, a
rollback action, and the verification that would confirm its effect. An action lacking
a rollback or a verification MUST NOT be proposable above the low risk class.

**Acceptance.**
- Given the reference scenario, when remediation runs, then the restore-pool-size
  action is proposed with risk `medium`, a rollback restoring the previous value, and a
  verification naming the error-rate series.

**Verified by.** TC-0040

<!-- sdd:item id=REQ-0041 stage=specify status=approved derives_from=G-003 priority=P0 -->
### REQ-0041 — The policy engine allowlists targets and refuses everything else

**Requirement.** The policy engine MUST evaluate every action against an allowlist of
services, configuration keys, tools and value ranges, and MUST refuse any action
outside it. Arbitrary shell execution, free-form SQL, resource deletion and
cross-service bulk operations MUST be refused unconditionally.

**Acceptance.**
- Given an action targeting a service not on the allowlist, when it is evaluated, then
  it is refused with a reason naming the violated rule.
- Given an action whose tool is a shell invocation, when it is evaluated, then it is
  refused regardless of approval state.

**Verified by.** TC-0041

<!-- sdd:item id=REQ-0042 stage=specify status=approved derives_from=G-003 priority=P0 -->
### REQ-0042 — Medium and high risk actions halt at an approval gate

**Requirement.** An action of risk class `medium` or `high` MUST cause the case to
enter an awaiting-approval state and MUST NOT execute until an approval decision is
recorded. Low-risk actions MAY execute automatically when policy permits.

**Acceptance.**
- Given a medium-risk action, when the orchestrator advances, then the status is
  awaiting approval and no actuator has been invoked.

**Verified by.** TC-0042

<!-- sdd:item id=REQ-0043 stage=specify status=approved derives_from=G-003 priority=P0 -->
### REQ-0043 — Policy is re-checked immediately before execution

**Requirement.** The executor MUST re-evaluate policy against the action as it stands at
execution time, after approval, and MUST refuse execution if the action or the policy
changed such that it is no longer permitted.

**Acceptance.**
- Given an approved action whose arguments are altered after approval, when execution is
  attempted, then it is refused and the refusal is recorded.

**Verified by.** TC-0043

<!-- sdd:item id=REQ-0044 stage=specify status=approved derives_from=G-003 priority=P0 -->
### REQ-0044 — Retrieved content cannot become instruction

**Requirement.** Content retrieved from logs, commit messages, runbooks, tickets or any
other external source MUST enter the system only as evidence payload. It MUST NOT be
able to alter agent role declarations, tool selection, policy or approval state. The
executor MUST accept only typed actions and MUST reject any action assembled from
retrieved text.

**Acceptance.**
- Given a log line containing text instructing the system to execute a restart, when
  the case is processed, then no action is proposed from that text and no policy rule
  changes.

**Verified by.** TC-0044

<!-- sdd:item id=REQ-0045 stage=specify status=approved derives_from=G-003 priority=P0 -->
### REQ-0045 — Approvals and executions are audited

**Requirement.** Every approval decision MUST record the decision, the deciding
identity, the timestamp and any comment. Every execution MUST record the action, the
outcome, the duration and the resulting system response. Both MUST appear in the event
log and in the report.

**Acceptance.**
- Given an approved and executed action, when the audit trail is read, then it contains
  the approver identity, the decision and the execution outcome.

**Verified by.** TC-0045

<!-- sdd:item id=REQ-0046 stage=specify status=approved derives_from=G-003 priority=P1 -->
### REQ-0046 — A rejected action leads to a report, not a silent stop

**Requirement.** When an approval is rejected, the case MUST proceed to reporting with
the rejection, its reason and the un-executed action recorded.

**Acceptance.**
- Given a rejected approval, when the orchestrator advances, then the case reaches
  reporting and the report contains the rejected action and its reason.

**Verified by.** TC-0046

## 8. Verification and reporting

<!-- sdd:item id=REQ-0050 stage=specify status=approved derives_from=G-004 priority=P0 -->
### REQ-0050 — Executed actions are verified against the triggering signals

**Requirement.** After an action executes, the verification role MUST re-query the
signals that triggered the incident over a post-action window and MUST report, per
signal, the before value, the after value and whether it recovered, as evidence of kind
`verification`.

**Acceptance.**
- Given the reference scenario after restoring the pool size, when verification runs,
  then the error-rate series is reported as recovered with both values present.

**Verified by.** TC-0051

<!-- sdd:item id=REQ-0051 stage=specify status=approved derives_from=G-004 priority=P1 -->
### REQ-0051 — Failed recovery returns to investigation within budget

**Requirement.** If verification reports no recovery and the round budget is not
exhausted, the case MUST return to evidence collection and the acted-upon hypothesis
MUST be marked challenged. If the budget is exhausted, the case MUST proceed to
reporting with the failure recorded.

**Acceptance.**
- Given verification reporting no recovery with budget remaining, when the orchestrator
  advances, then the status returns to collecting and the hypothesis is challenged.

**Verified by.** TC-0052

<!-- sdd:item id=REQ-0052 stage=specify status=approved derives_from=G-001,G-002 priority=P0 -->
### REQ-0052 — The report explains what was ruled out and why

**Requirement.** The report MUST contain the incident summary, the affected scope, the
timeline, the accepted root cause with its evidence chain, the rejected alternatives
each with the reason for rejection, the critiques raised and their resolution, the
actions with their approval and execution status, the verification result, and the
unmet evidence demands if any.

**Acceptance.**
- Given a completed reference scenario, when the report is generated, then every listed
  section is present and the rejected-alternatives section names at least one
  alternative with a reason.

**Verified by.** TC-0053

<!-- sdd:item id=REQ-0053 stage=specify status=approved derives_from=G-004 priority=P0 -->
### REQ-0053 — The report is reproducible from the event log

**Requirement.** Regenerating the report from the event log alone MUST produce a
document equal to the one produced during the live run.

**Acceptance.**
- Given a completed case, when the report is regenerated from replayed events, then it
  is byte-identical to the live report.

**Verified by.** TC-0050

## 9. Orchestration, API and console

<!-- sdd:item id=REQ-0060 stage=specify status=approved derives_from=G-006 priority=P0 -->
### REQ-0060 — The investigation is an explicit, bounded state machine

**Requirement.** The case MUST progress through declared states with declared legal
transitions. Rounds MUST be bounded by a configured maximum, and no state MUST be
re-entered more times than the budget allows. An illegal transition MUST be refused and
recorded.

**Acceptance.**
- Given a case configured with three rounds and a critic that always demands more
  evidence, when it is driven to completion, then it terminates having performed
  exactly three collection rounds and reaches reporting.

**Verified by.** TC-0060

<!-- sdd:item id=REQ-0061 stage=specify status=approved derives_from=G-006 priority=P1 -->
### REQ-0061 — Collection fans out, analysis converges

**Requirement.** Collector roles MUST be invoked concurrently within a round, and their
contributions MUST be applied in a deterministic order independent of completion order.
Analysis, critique, remediation, execution and verification MUST be serial.

**Acceptance.**
- Given collectors that complete in varying orders across runs, when a round completes,
  then the resulting evidence identifiers and their order are identical every time.

**Verified by.** TC-0061

<!-- sdd:item id=REQ-0062 stage=specify status=approved derives_from=G-004 priority=P0 -->
### REQ-0062 — Every transition and tool call emits an event

**Requirement.** Each event MUST carry a monotonically increasing sequence number, the
case identifier, the actor, the event type, a summary, an optional payload reference,
and the duration for events representing work.

**Acceptance.**
- Given a completed case, when events are listed, then sequence numbers are contiguous
  from one, and every state transition and tool call is represented.

**Verified by.** TC-0062

<!-- sdd:item id=REQ-0063 stage=specify status=approved derives_from=G-006 priority=P0 -->
### REQ-0063 — The API exposes the full case lifecycle

**Requirement.** The service MUST expose endpoints to create a case from an alert, read
a case with its evidence, hypotheses, critiques and actions, list events, record an
approval decision, and retrieve the report as Markdown and as JSON. Requests MUST be
validated and MUST return a structured error naming the offending field.

**Acceptance.**
- Given a malformed alert missing the service field, when a case is created, then the
  response is `400` and names `service`.
- Given a completed case, when the report endpoint is called, then Markdown is returned
  with the documented content type.

**Verified by.** TC-0063

<!-- sdd:item id=REQ-0064 stage=specify status=approved derives_from=G-006 priority=P1 -->
### REQ-0064 — Progress streams live to connected clients

**Requirement.** The service MUST stream events to connected clients as they are
appended, MUST allow a client to resume from a known sequence number without loss, and
MUST not block case progress when a client is slow or disconnects.

**Acceptance.**
- Given a client subscribing from sequence 0 while a case runs, when the case
  completes, then the client has received every event exactly once in order.
- Given a client that disconnects mid-case, when the case continues, then it completes
  normally.

**Verified by.** TC-0064

<!-- sdd:item id=REQ-0065 stage=specify status=approved derives_from=G-006 priority=P1 -->
### REQ-0065 — The console makes the adversarial process visible

**Requirement.** The console MUST show, for a selected case: the agent timeline with
per-agent tool calls, the evidence grouped by kind, the ranked hypotheses with their
score breakdowns, the critiques with their verdicts and demands, the actions with their
risk class and approval state, an approval control, and the report.

**Acceptance.**
- Given a completed reference scenario, when the console page is rendered, then it
  contains the timeline, at least one critique with its verdict, and the approval
  control for the medium-risk action.

**Verified by.** TC-0065

## 10. Evaluation

<!-- sdd:item id=REQ-0080 stage=specify status=approved derives_from=G-005 priority=P1 -->
### REQ-0080 — The same case runs in three comparable modes

**Requirement.** The system MUST support running a fault case as a single agent with
all tools, as multiple agents without critique, and as the full flow — over identical
fixture inputs, so that differences are attributable to the flow rather than the data.

**Acceptance.**
- Given one fault case, when it is run in all three modes, then all three complete and
  each records the mode it ran in.

**Verified by.** TC-0080

<!-- sdd:item id=REQ-0081 stage=specify status=approved derives_from=G-005 priority=P1 -->
### REQ-0081 — Comparison metrics are computed from real runs

**Requirement.** The evaluation MUST compute, per mode: top-1 accuracy against the
declared expected root cause, top-3 coverage, evidence-kind completeness, the number of
hypotheses the critic corrected or rejected, and the number of blocked unapproved
actions. Numbers MUST come from executed runs, and the sample size MUST accompany them.

**Acceptance.**
- Given three fault cases run in three modes, when the evaluation report is produced,
  then it contains a per-mode row with all listed metrics and the sample size.

**Verified by.** TC-0081

<!-- sdd:item id=REQ-0082 stage=specify status=approved derives_from=G-005,G-006 priority=P1 -->
### REQ-0082 — The fault case library is declarative and reproducible

**Requirement.** Each fault case MUST be defined as data: the alert, the fixture signal
set, the expected root cause and the expected remediation. Adding a case MUST NOT
require changing agent code.

**Acceptance.**
- Given a new case file added to the library, when the evaluation runs, then the case is
  included without any code change.

**Verified by.** TC-0082

## 11. Non-functional requirements

<!-- sdd:item id=REQ-0090 stage=specify status=approved derives_from=G-006,G-008 priority=P0 -->
### REQ-0090 — Deterministic and offline by default

**Requirement.** With the fixture profile selected, the entire system MUST run with no
network access, no credentials and no model provider, and MUST produce identical output
for identical input across runs and processes.

**Acceptance.**
- Given the reference scenario run twice in separate processes, when the reports are
  compared, then they are byte-identical.

**Verified by.** TC-0070

<!-- sdd:item id=REQ-0091 stage=specify status=approved derives_from=G-008 priority=P0 -->
### REQ-0091 — No third-party runtime dependencies

**Requirement.** The service MUST compile and run using the Go standard library only.

**Acceptance.**
- `go.mod` declares no external module requirement.

**Verified by.** TC-9013

<!-- sdd:item id=REQ-0092 stage=specify status=approved derives_from=G-007 priority=P1 -->
### REQ-0092 — Delivered as a container image and a compose stack

**Requirement.** The deliverable MUST include a container image definition producing a
runnable service, and a compose stack bringing up the service, the console and the demo
environment. The image MUST NOT require build-time network access to a module proxy.

**Acceptance.**
- Given the repository, when the image definition is inspected, then it builds from a
  vendored-free standard-library module and declares a health check and a non-root user.

**Verified by.** TC-0090

<!-- sdd:item id=REQ-0093 stage=specify status=approved derives_from=G-007 priority=P1 -->
### REQ-0093 — Bilingual user manual accompanies the artifact

**Requirement.** The deliverable MUST include a user manual in English and Chinese
covering installation, the demo walkthrough, configuration, the API, the safety model
and troubleshooting, with both renderings structurally identical.

**Acceptance.**
- `sddctl lint` reports no bilingual finding for the manual pair.

**Verified by.** TC-0091

<!-- sdd:item id=REQ-0094 stage=specify status=approved derives_from=G-006 priority=P1 -->
### REQ-0094 — Case memory is bounded

**Requirement.** A case MUST bound the number of retained evidence items, hypotheses per
round, critiques per round and events, and MUST record when a bound caused truncation
rather than silently discarding.

**Acceptance.**
- Given a case driven past every configured bound, when it completes, then memory use
  stays bounded and the truncation is recorded in the event log.

**Verified by.** TC-0071

<!-- sdd:item id=REQ-0095 stage=specify status=approved derives_from=G-007 priority=P1 -->
### REQ-0095 — Published as a versioned release with a linux/amd64 image

**Requirement.** A tagged version MUST publish a GitHub release carrying a container
image runnable on `linux/amd64`, together with checksummed binaries for the same
platform. The image MUST be built from the tagged commit by an automated pipeline rather
than uploaded by hand, MUST be labelled with the commit it came from, and MUST pass the
same health check the compose stack relies on before the release is published.

The release MUST NOT be publishable from a tree that fails the delivery gate: a version
that ships is by definition one whose spec tree is sealed, traced and tested.

**Acceptance.**
- Given a tag matching `v*`, when the pipeline runs, then it builds a `linux/amd64`
  image, starts it, waits for `/healthz`, publishes the image to a registry under both
  the version tag and the commit SHA, and creates a release whose assets include the
  binaries, a loadable image tarball and a `SHA256SUMS` file.

The image tarball is not redundant with the registry. A registry package can be private,
can require a login the reader does not have, and can be pruned; an asset attached to the
release is downloadable by anyone who can see the release and loadable with
`docker load`. The release must remain usable without registry access.
- Given a tree that fails `sddctl gate --stage deliver`, when the pipeline runs, then it
  fails before anything is published.
- Given a maintainer who can run workflows but cannot push a tag, when they start the
  pipeline manually with a version, then it produces the same release and creates the tag
  itself.

The manual entry point exists because tag-push permission and workflow-run permission are
granted separately, and a release that only a tag-pusher can cut is a release some
maintainers cannot cut at all. Both entry points run the identical job; neither can skip
a gate.

**Verified by.** TC-0092

<!-- sdd:item id=REQ-0096 stage=specify status=approved derives_from=G-001 priority=P1 -->
### REQ-0096 — Live metric adapter over the Prometheus HTTP API

**Requirement.** The metric port MUST have a live adapter querying the Prometheus HTTP
API, returning the same `Series` shape the fixture adapter returns, so no code above the
port can tell which adapter served it. It MUST apply the port's bounds, MUST mark
truncation rather than silently dropping samples, and MUST fail with a typed error the
collector can record as a degraded source rather than as a crash.

A Prometheus response containing `NaN`, a stale marker, or a series with no samples MUST
be treated as absence of evidence, not as a zero value: a zero reading and no reading
support different conclusions.

**Acceptance.**
- Given a recorded `query_range` response, when the adapter maps it, then the resulting
  series matches the equivalent fixture series in shape, ordering and units.
- Given a response with more samples than the bound allows, when the adapter maps it,
  then the series is truncated and truncation is reported.
- Given an unreachable endpoint, when a collector queries it, then the collector records
  a degraded source and the investigation continues.

**Verified by.** TC-0100, TC-0101

<!-- sdd:item id=REQ-0097 stage=specify status=approved derives_from=G-001 priority=P1 -->
### REQ-0097 — Live log adapter over the container runtime

**Requirement.** The log port MUST have a live adapter reading container logs from the
container runtime, demultiplexing the runtime's stream framing, filtering by the query's
terms and window, and applying the port's bounds.

**Acceptance.**
- Given a multiplexed runtime log stream, when the adapter reads it, then stdout and
  stderr records are recovered with their correct stream labels and timestamps.
- Given a query with terms and a window, when the adapter searches, then only lines
  matching every term and falling inside the window are returned, bounded and marked.

**Verified by.** TC-0102

<!-- sdd:item id=REQ-0098 stage=specify status=approved derives_from=G-001,G-006 priority=P0 -->
### REQ-0098 — Adapter selection is configuration, and offline stays the default

**Requirement.** The adapter serving each port MUST be selected by a named profile at
startup. The default profile MUST be the fixture profile. No test in the suite may
require network access, a container runtime, or a Prometheus instance to pass.

**Acceptance.**
- Given no configuration, when the system starts, then every port is served by its
  fixture adapter.
- Given the full test suite, when it runs with no network, then it passes.

This requirement is the reason the live adapters are safe to add. An adapter set that
could quietly become the default would make the determinism the rest of the system rests
on (REQ-0090) conditional on the environment.

**Verified by.** TC-0103

<!-- sdd:item id=REQ-0099 stage=specify status=approved derives_from=G-002,G-005 priority=P0 -->
### REQ-0099 — Correctness must not depend on distrusting any particular explanation

**Requirement.** The fault catalog MUST contain at least one case whose loudest evidence
is non-causal, and at least one case whose correct answer is the explanation that the
reference scenario teaches the system to reject. The adversarial process MUST reach the
declared root cause in both.

**Why this is a requirement and not a test detail.** The reference scenario C1 is won by
demoting `sig-traffic-surge` from first place. A system that learned "the leading
explanation is wrong" or "traffic is never the cause" would score perfectly on C1, C2 and
C3 while having learned nothing. The critic's value is that it *discriminates*, and a
discriminator is only demonstrated by a case where it must decline to object.

**Acceptance.**
- Given a case whose log volume overwhelmingly matches one signature while the
  discriminating metric and change evidence refute it, when the full flow runs, then the
  declared root cause is accepted and the loud signature is not.
- Given that same case, when the flow runs, then the accepted cause is
  `sig-traffic-surge` — the explanation C1 demotes — proving the outcome is evidence-led
  rather than a fixed bias.

**Verified by.** TC-0104

<!-- sdd:item id=REQ-0100 stage=specify status=approved derives_from=G-005 priority=P1 -->
### REQ-0100 — A model-backed reasoner behind the same port

**Requirement.** The reasoner port MUST have a model-backed adapter that selects
signatures from the catalog and binds evidence to them. It MUST NOT invent claims outside
the catalog, and it MUST NOT supply scores: scoring stays deterministic, computed from the
evidence the adapter bound (ADR-007).

Every response MUST be treated as untrusted structured data. A hypothesis citing an
evidence identifier that is not in the snapshot MUST be dropped; a signature identifier
that is not in the catalog MUST be dropped; a verdict outside the closed set MUST be
dropped. A malformed or unreachable provider MUST surface as a typed error, never as a
silent empty result — an empty hypothesis list means "nothing matched", and a provider
failure must not be able to impersonate that.

**Acceptance.**
- Given a recorded provider response selecting a catalog signature with evidence
  identifiers from the snapshot, when the adapter maps it, then the resulting hypotheses
  carry scores computed by the deterministic scorer, not by the provider.
- Given a response citing an unknown evidence identifier, an unknown signature, or an
  invalid verdict, when the adapter maps it, then those items are dropped and the rest
  survive.
- Given an unreachable provider, when the adapter is called, then it returns a typed
  error rather than an empty result.

**Verified by.** TC-0105, TC-0106

## 12. Out of scope

| # | Excluded behaviour | Reason |
|---|---|---|
| S1 | Writing to a real production system | Charter N1 |
| S2 | Cloud provider and SaaS integrations | Charter N2 |
| S3 | Vector or graph storage | Charter N3 |
| S4 | Agent-authored shell or SQL | Charter N4, REQ-0041 |
| S5 | Authentication, authorisation and multi-tenancy | Charter N6 |
| S6 | Alert deduplication and incident correlation across cases | Not required by any goal; one alert produces one case |
