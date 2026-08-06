---
id: TESTPLAN-001
lang: en
counterpart: test-plan.zh.md
doc_version: 1.0.0
status: approved
stage: verify
---

# IncidentOps Arena — Test Plan

## 1. Strategy

| Level | Scope | Runner | Must run offline |
|---|---|---|---|
| unit | One function or type against constructed values | `go test ./internal/...` | yes |
| integration | Two or more modules through their real interfaces | `go test ./internal/...` | yes |
| e2e | A whole case from alert to closed report | `go test ./internal/orchestrator` | yes |
| policy | Safety properties that must hold under adversarial input | `go test ./internal/policy` | yes |
| governance | Properties of the repository itself | `go test ./internal/sdd` | yes |
| delivery | The shipped artifact and its documentation | `go test ./internal/...`, `make image` | build only |

Every level except `delivery` runs with networking unavailable. No test starts a
container, opens a socket to an external host, or reads a credential.

## 2. Entry and exit criteria

**Entry.** The DLD item a test verifies is `approved` and sealed, and its test case is
written before the code.

**Exit (M1 gate).** All of:

1. `go build ./...` and `go vet ./...` clean.
2. `go test ./...` passes with `-race`, offline.
3. Every P0 requirement has at least one passing test (`sddctl gate --stage deliver`).
4. `sddctl validate` reports no error and no drift.
5. The reference scenario produces byte-identical reports across two processes.

## 3. Test cases

### 3.1 Domain contract

<!-- sdd:item id=TC-0001 stage=verify status=approved derives_from=REQ-0001 -->
#### TC-0001 — Agents do not mutate case state

**Level.** unit. **Verifies.** REQ-0001.

**Steps.** Snapshot a case, run every agent type against it, compare the case before and
after by deep equality.

**Expected.** The case is unchanged; each agent's only effect is its returned
contributions.

**Test function.** `TestAgentsArePure` in `internal/agent/agent_test.go`

<!-- sdd:item id=TC-0002 stage=verify status=approved derives_from=REQ-0002 -->
#### TC-0002 — Evidence validation and immutability

**Level.** unit. **Verifies.** REQ-0002.

**Steps.** Validate evidence with an empty raw reference, an unknown kind and a
confidence of 1.5; then store valid evidence and re-read the case.

**Expected.** `ErrMissingRawRef`, `ErrUnknownEvidenceKind` and `ErrConfidenceRange`
respectively; the stored evidence re-reads field-for-field identical.

**Test function.** `TestEvidenceValidation` in `internal/domain/evidence_test.go`

<!-- sdd:item id=TC-0003 stage=verify status=approved derives_from=REQ-0005 -->
#### TC-0003 — Out-of-role contributions are rejected

**Level.** integration. **Verifies.** REQ-0005.

**Steps.** Apply a `propose_hypothesis` contribution emitted by the `metrics` role and a
`raise_critique` emitted by `analysis`.

**Expected.** Both are rejected, a `contribution_rejected` event records each, and the
projection gains no hypothesis or critique.

**Test function.** `TestRoleCapabilityEnforced` in `internal/orchestrator/apply_test.go`

<!-- sdd:item id=TC-0004 stage=verify status=approved derives_from=REQ-0003 -->
#### TC-0004 — Event sequence integrity

**Level.** unit. **Verifies.** REQ-0003.

**Steps.** Append a batch whose first sequence number is not `len(existing)+1`; then
replay a log with a missing sequence number.

**Expected.** `ErrSequenceGap` on append; `Replay` returns an error naming the gap.

**Test function.** `TestSequenceGapRejected` in `internal/store/store_test.go`

<!-- sdd:item id=TC-0005 stage=verify status=approved derives_from=REQ-0002 -->
#### TC-0005 — A case survives a store restart

**Level.** integration. **Verifies.** REQ-0002.

**Steps.** Write a case through the file store, discard the instance, construct a new
store over the same directory, and read the case.

**Expected.** The case appears in the index and its events replay to an equal
projection.

**Test function.** `TestFileStoreRoundTrip` in `internal/store/file_test.go`

### 3.2 Signal collection

<!-- sdd:item id=TC-0010 stage=verify status=approved derives_from=REQ-0010 -->
#### TC-0010 — The reference scenario runs offline

**Level.** e2e. **Verifies.** REQ-0010.

**Steps.** Run case `C1` end to end with the fixture profile.

**Expected.** The case reaches `closed`, and evidence of kinds metric, log, change,
topology, knowledge and verification is present.

**Test function.** `TestReferenceScenarioOffline` in `internal/orchestrator/e2e_test.go`

<!-- sdd:item id=TC-0011 stage=verify status=approved derives_from=REQ-0011 -->
#### TC-0011 — Anomaly onset and co-movement

**Level.** unit. **Verifies.** REQ-0011.

**Steps.** Analyse a series stepping from 0.002 to 0.18 at a known sample; analyse two
series stepping together.

**Expected.** The onset is within one sample of the step; both series are reported
co-moving; no summary contains a causal claim word from the forbidden list.

**Test function.** `TestAnomalyOnset` in `internal/signal/anomaly_test.go`

<!-- sdd:item id=TC-0012 stage=verify status=approved derives_from=REQ-0012 -->
#### TC-0012 — Log clustering is bounded and ordered

**Level.** unit. **Verifies.** REQ-0012.

**Steps.** Cluster 184 lines matching one template and 3 matching another, plus a run
exceeding `maxClusters`.

**Expected.** Two clusters with counts 184 and 3 in that order, at most
`logSamplesPerCluster` samples each, and at most `maxClusters` clusters retained.

**Test function.** `TestLogClustering` in `internal/signal/cluster_test.go`

<!-- sdd:item id=TC-0013 stage=verify status=approved derives_from=REQ-0013 -->
#### TC-0013 — Change evidence reports both values and no rollback occurs

**Level.** integration. **Verifies.** REQ-0013.

**Steps.** Run the change collector over the extended window of case `C1` with a
recording actuator installed.

**Expected.** `DB_POOL_SIZE` is reported with `20`, `2` and the deployment timestamp;
the actuator recorded zero invocations.

**Test function.** `TestChangeCollector` in `internal/agent/collectors_test.go`

<!-- sdd:item id=TC-0014 stage=verify status=approved derives_from=REQ-0014 -->
#### TC-0014 — Topology separates origin from victim

**Level.** unit. **Verifies.** REQ-0014.

**Steps.** Collect topology evidence for a fixture where `order-api` is anomalous first
and `checkout-web` later.

**Expected.** `order-api` is a candidate origin; `checkout-web` is affected and is not a
candidate origin.

**Test function.** `TestTopologyCollector` in `internal/agent/collectors_test.go`

<!-- sdd:item id=TC-0015 stage=verify status=approved derives_from=REQ-0015 -->
#### TC-0015 — Runbook retrieval matches on symptoms

**Level.** unit. **Verifies.** REQ-0015.

**Steps.** Search the runbook corpus for an incident whose symptoms include pool
saturation.

**Expected.** The connection-pool runbook is among the matches, with its identifier and
matched terms recorded in the evidence facts.

**Test function.** `TestKnowledgeCollector` in `internal/agent/collectors_test.go`

<!-- sdd:item id=TC-0016 stage=verify status=approved derives_from=REQ-0016 -->
#### TC-0016 — Tool results are bounded and truncation is recorded

**Level.** unit. **Verifies.** REQ-0016.

**Steps.** Apply bounds to a result exceeding `MaxRows`, one exceeding `MaxChars` and a
query exceeding `MaxSpan`.

**Expected.** Each result is within its bound, `truncated` is true, and the resulting
evidence carries `Truncated`.

**Test function.** `TestBounds` in `internal/signal/bounds_test.go`

### 3.3 Hypothesis and scoring

<!-- sdd:item id=TC-0020 stage=verify status=approved derives_from=REQ-0020 -->
#### TC-0020 — Hypotheses are capped and carry a mechanism

**Level.** unit. **Verifies.** REQ-0020.

**Steps.** Hypothesise over the round-2 evidence set of case `C1`.

**Expected.** At most `maxHypothesesPerRound` hypotheses; each has a non-empty mechanism
and a non-empty supporting set; `sig-db-pool-exhaustion` is present.

**Test function.** `TestHypothesise` in `internal/reasoner/rule_test.go`

<!-- sdd:item id=TC-0021 stage=verify status=approved derives_from=REQ-0021 -->
#### TC-0021 — Unsupported hypotheses are rejected and coverage gates progression

**Level.** integration. **Verifies.** REQ-0021.

**Steps.** Apply a hypothesis with an empty supporting list and one referencing an
unknown identifier; then evaluate the remediation guard for a hypothesis supported only
by metric evidence.

**Expected.** Both hypotheses are rejected with `hypothesis_rejected` events and never
appear in a ranking; the guard refuses with a reason naming the kind count.

**Test function.** `TestUnsupportedHypothesisRejected` in
`internal/orchestrator/guard_test.go`

<!-- sdd:item id=TC-0022 stage=verify status=approved derives_from=REQ-0022 -->
#### TC-0022 — Score decomposes and weights sum to one

**Level.** unit. **Verifies.** REQ-0022.

**Steps.** Score a hypothesis over the round-2 evidence and sum its breakdown; sum the
declared weights.

**Expected.** Terms minus penalty equals the total within `1e-9`; the weights total
`1.0`; every term names its weight and value.

**Test function.** `TestScoreBreakdown` in `internal/reasoner/score_test.go`

<!-- sdd:item id=TC-0023 stage=verify status=approved derives_from=REQ-0023 -->
#### TC-0023 — Counter-evidence applies exactly one penalty

**Level.** unit. **Verifies.** REQ-0023.

**Steps.** Score a hypothesis with no counter-evidence, then with one unresolved
counter, then with that counter marked resolved.

**Expected.** The second total is lower by exactly `counterEvidencePenalty`; the third
equals the first.

**Test function.** `TestCounterEvidencePenalty` in `internal/reasoner/score_test.go`

<!-- sdd:item id=TC-0024 stage=verify status=approved derives_from=REQ-0024 -->
#### TC-0024 — Ranking is a total, stable order

**Level.** unit. **Verifies.** REQ-0024.

**Steps.** Rank hypotheses with equal scores and equal kind counts, presented in
shuffled input orders across 100 iterations.

**Expected.** The output order is identical every time and orders by identifier
ascending on ties.

**Test function.** `TestRankIsTotalOrder` in `internal/reasoner/score_test.go`

### 3.4 Adversarial review

<!-- sdd:item id=TC-0030 stage=verify status=approved derives_from=REQ-0030 -->
#### TC-0030 — Every examined hypothesis receives a verdict

**Level.** unit. **Verifies.** REQ-0030.

**Steps.** Critique the round-1 hypothesis set of case `C1`.

**Expected.** Every hypothesis has at least one critique; every verdict is one of the
four declared values; the critic emitted no hypothesis contribution.

**Test function.** `TestCritiqueProducesVerdicts` in `internal/reasoner/critique_test.go`

<!-- sdd:item id=TC-0031 stage=verify status=approved derives_from=REQ-0031 -->
#### TC-0031 — A demand forces another round, and an unmet demand is reported

**Level.** e2e. **Verifies.** REQ-0031.

**Steps.** Run case `C1` and inspect round transitions; then run a case whose demands
can never be satisfied, with `maxRounds` reached.

**Expected.** The first returns to `collecting` and round 2 contains evidence carrying
the demanded descriptor; the second terminates and the report lists the unmet demand.

**Test function.** `TestCriticForcesAnotherRound` in `internal/orchestrator/e2e_test.go`

<!-- sdd:item id=TC-0032 stage=verify status=approved derives_from=REQ-0032 -->
#### TC-0032 — An alternative explanation is raised with a discriminator

**Level.** unit. **Verifies.** REQ-0032.

**Steps.** Critique the round-1 set of case `C1`, where the traffic surge leads and the
traffic history has not been examined.

**Expected.** A critique of category `alternative_explanation` names a rival signature,
and a demand for the historical peak traffic comparison is emitted.

**Test function.** `TestAlternativeExplanationRule` in
`internal/reasoner/critique_test.go`

<!-- sdd:item id=TC-0033 stage=verify status=approved derives_from=REQ-0033 -->
#### TC-0033 — Temporal order is checked

**Level.** unit. **Verifies.** REQ-0033.

**Steps.** Critique a change-attributing hypothesis whose matched change is timestamped
after the anomaly onset.

**Expected.** Counter-evidence is attached and the verdict is `revise` or `reject`,
never `accept`.

**Test function.** `TestTemporalOrderRule` in `internal/reasoner/critique_test.go`

<!-- sdd:item id=TC-0034 stage=verify status=approved derives_from=REQ-0034 -->
#### TC-0034 — Source-versus-victim confusion is challenged

**Level.** unit. **Verifies.** REQ-0034.

**Steps.** Critique a hypothesis blaming a service whose topology evidence names an
upstream service anomalous earlier.

**Expected.** A critique of category `source_vs_victim` names the upstream candidate.

**Test function.** `TestSourceVsVictimRule` in `internal/reasoner/critique_test.go`

<!-- sdd:item id=TC-0035 stage=verify status=approved derives_from=REQ-0035 -->
#### TC-0035 — Progression requires the acceptance condition

**Level.** unit. **Verifies.** REQ-0035.

**Steps.** Evaluate the remediation guard for a leader with verdict `revise`; then for a
leader below `acceptThreshold`; then for a case with change evidence whose leader lacks
it.

**Expected.** All three are refused, each with a reason naming the failing condition.

**Test function.** `TestRemediationGuard` in `internal/orchestrator/guard_test.go`

<!-- sdd:item id=TC-0036 stage=verify status=approved derives_from=REQ-0036 -->
#### TC-0036 — Close calls demand or escalate

**Level.** integration. **Verifies.** REQ-0036.

**Steps.** Critique a set whose top two differ by less than `closeCallMargin`, with
budget remaining; then repeat with the budget exhausted.

**Expected.** The first emits a discriminating demand; the second drives the case to
`human_review`.

**Test function.** `TestCloseCallEscalates` in `internal/orchestrator/machine_test.go`

### 3.5 Remediation, policy and approval

<!-- sdd:item id=TC-0040 stage=verify status=approved derives_from=REQ-0040 -->
#### TC-0040 — Actions are complete proposals

**Level.** integration. **Verifies.** REQ-0040.

**Steps.** Run remediation on the accepted hypothesis of case `C1`; then validate an
action of risk `medium` with no rollback.

**Expected.** The proposed action is `set_config order-api DB_POOL_SIZE 20`, risk
`medium`, rollback to `2`, verifying `http_5xx_rate`; the incomplete action fails
validation.

**Test function.** `TestRemediationProposal` in `internal/agent/remediation_test.go`

<!-- sdd:item id=TC-0041 stage=verify status=approved derives_from=REQ-0041 -->
#### TC-0041 — The allowlist refuses everything outside it

**Level.** policy. **Verifies.** REQ-0041.

**Steps.** Evaluate actions targeting an unlisted service, an unlisted tool, an unlisted
configuration key, an out-of-range value, a shell tool, a `sql` tool, a
`delete_resource` and a `bulk_restart` — the last four also with an approval attached.

**Expected.** All are denied; each denial names the violated rule; the categorical
denials hold regardless of approval.

**Test function.** `TestPolicyDenies` in `internal/policy/policy_test.go`

<!-- sdd:item id=TC-0042 stage=verify status=approved derives_from=REQ-0042 -->
#### TC-0042 — Medium risk halts at the approval gate

**Level.** e2e. **Verifies.** REQ-0042.

**Steps.** Run case `C1` without recording a decision, with a recording actuator.

**Expected.** The case status is `awaiting_approval`, the actuator recorded zero
invocations, and an `approval_requested` event exists.

**Test function.** `TestApprovalGateHalts` in `internal/orchestrator/e2e_test.go`

<!-- sdd:item id=TC-0043 stage=verify status=approved derives_from=REQ-0043 -->
#### TC-0043 — Policy is re-checked immediately before execution

**Level.** policy. **Verifies.** REQ-0043.

**Steps.** Approve an action, alter its arguments, then execute; separately, approve an
action and then tighten the policy before executing.

**Expected.** The first is refused as `tampered_after_approval`; the second is refused by
the re-evaluation; both record `action_refused` and invoke no actuator.

**Test function.** `TestExecutorRechecksPolicy` in `internal/policy/executor_test.go`

<!-- sdd:item id=TC-0044 stage=verify status=approved derives_from=REQ-0044 -->
#### TC-0044 — Retrieved content cannot become instruction

**Level.** policy. **Verifies.** REQ-0044.

**Steps.** Run a case whose fixture log lines and runbook text contain instructions to
restart a service, disable policy and execute a shell command.

**Expected.** No action originates from that text, the policy allowlists are unchanged
after the run, and the executor rejects an action assembled from a log line.

**Test function.** `TestUntrustedContentIsData` in `internal/policy/injection_test.go`

<!-- sdd:item id=TC-0045 stage=verify status=approved derives_from=REQ-0045 -->
#### TC-0045 — Approvals and executions are audited

**Level.** integration. **Verifies.** REQ-0045.

**Steps.** Approve and execute the action of case `C1`, then read the events and the
report.

**Expected.** `approval_recorded` carries the decision, identity, timestamp and comment;
`action_executed` carries the outcome and duration; both appear in the report.

**Test function.** `TestAuditTrail` in `internal/orchestrator/e2e_test.go`

<!-- sdd:item id=TC-0046 stage=verify status=approved derives_from=REQ-0046 -->
#### TC-0046 — A rejected approval still produces a report

**Level.** e2e. **Verifies.** REQ-0046.

**Steps.** Run case `C1` and reject the approval.

**Expected.** The case reaches `closed`, no actuator was invoked, and the report contains
the rejected action with its reason.

**Test function.** `TestRejectedApprovalReports` in `internal/orchestrator/e2e_test.go`

### 3.6 Verification and reporting

<!-- sdd:item id=TC-0050 stage=verify status=approved derives_from=REQ-0003,REQ-0053 -->
#### TC-0050 — Replay reproduces the case and the report

**Level.** integration. **Verifies.** REQ-0003, REQ-0053.

**Steps.** Complete case `C1`, read its events, replay them into an empty projection,
and render the report from both.

**Expected.** The projections are deeply equal and the two reports are byte-identical.

**Test function.** `TestReplayEquality` in `internal/domain/case_test.go`

<!-- sdd:item id=TC-0051 stage=verify status=approved derives_from=REQ-0050 -->
#### TC-0051 — Verification observes recovery only after the action

**Level.** e2e. **Verifies.** REQ-0050.

**Steps.** Query the verification signals before executing, then approve, execute and
run verification.

**Expected.** Before: not recovered. After: `http_5xx_rate` recovered with both before
and after values present, recorded as evidence of kind `verification`.

**Test function.** `TestVerificationAfterRemediation` in
`internal/orchestrator/e2e_test.go`

<!-- sdd:item id=TC-0052 stage=verify status=approved derives_from=REQ-0051 -->
#### TC-0052 — Failed recovery returns to investigation within budget

**Level.** integration. **Verifies.** REQ-0051.

**Steps.** Run a case whose post-action signals do not recover, with budget remaining;
repeat with the budget exhausted.

**Expected.** The first returns to `collecting` and marks the hypothesis `challenged`;
the second reaches `reporting` with the failure recorded.

**Test function.** `TestFailedRecoveryReturns` in `internal/orchestrator/machine_test.go`

<!-- sdd:item id=TC-0053 stage=verify status=approved derives_from=REQ-0052 -->
#### TC-0053 — The report explains what was ruled out

**Level.** integration. **Verifies.** REQ-0052.

**Steps.** Render the report for a completed case `C1`.

**Expected.** Every declared section is present; the rejected-alternatives section names
`sig-traffic-surge` with its counter-evidence as the reason; the score breakdown table
lists all six terms.

**Test function.** `TestReportSections` in `internal/report/report_test.go`

### 3.7 Orchestration, API and console

<!-- sdd:item id=TC-0060 stage=verify status=approved derives_from=REQ-0060 -->
#### TC-0060 — The state machine is bounded and legal

**Level.** integration. **Verifies.** REQ-0060.

**Steps.** Run a case with a critic that always demands evidence; attempt an illegal
transition directly.

**Expected.** Exactly `maxRounds` collection rounds occur and the case reaches
`reporting`; the illegal transition is refused and recorded.

**Test function.** `TestStateMachineBounded` in `internal/orchestrator/machine_test.go`

<!-- sdd:item id=TC-0061 stage=verify status=approved derives_from=REQ-0061 -->
#### TC-0061 — Concurrent collection is order-independent

**Level.** integration. **Verifies.** REQ-0061.

**Steps.** Run a collection round 50 times with collectors delayed by varying amounts to
shuffle completion order, under `-race`.

**Expected.** The resulting evidence identifiers and their order are identical on every
run.

**Test function.** `TestCollectionDeterminism` in `internal/orchestrator/engine_test.go`

<!-- sdd:item id=TC-0062 stage=verify status=approved derives_from=REQ-0062 -->
#### TC-0062 — Every transition and tool call emits an event

**Level.** integration. **Verifies.** REQ-0062.

**Steps.** Complete case `C1` and inspect the event log.

**Expected.** Sequence numbers are contiguous from 1; every state entered has a
`state_changed` event; every collector run has `agent_started` and `agent_completed`
with a duration.

**Test function.** `TestEventCompleteness` in `internal/orchestrator/e2e_test.go`

<!-- sdd:item id=TC-0063 stage=verify status=approved derives_from=REQ-0063 -->
#### TC-0063 — The API validates and serves the lifecycle

**Level.** integration. **Verifies.** REQ-0063.

**Steps.** Exercise every endpoint over `httptest`, including a malformed alert missing
`service`, an unknown case identifier, and an unknown field in the body.

**Expected.** `400` naming `service`; `404` for the unknown case; `400` for the unknown
field; the report endpoint returns Markdown with `text/markdown; charset=utf-8`;
collections are identifier-ordered.

**Test function.** `TestAPILifecycle` in `internal/httpapi/api_test.go`

<!-- sdd:item id=TC-0064 stage=verify status=approved derives_from=REQ-0064 -->
#### TC-0064 — The stream is lossless across a reconnect and never blocks

**Level.** integration. **Verifies.** REQ-0064.

**Steps.** Subscribe from sequence 0 while a case runs; disconnect mid-case and resume
with `Last-Event-ID`; separately, let a subscriber's buffer overflow.

**Expected.** The client receives every event exactly once in order across the
reconnect; the overflowing subscriber is dropped and the case still completes.

**Test function.** `TestStreamResume` in `internal/httpapi/stream_test.go`

<!-- sdd:item id=TC-0065 stage=verify status=approved derives_from=REQ-0065 -->
#### TC-0065 — The console renders the adversarial process

**Level.** integration. **Verifies.** REQ-0065.

**Steps.** Render the workbench page for a completed case `C1`.

**Expected.** The HTML contains the timeline, evidence grouped by kind, the ranked
hypotheses with score breakdowns, at least one critique with its verdict, and the
approval control for the medium-risk action.

**Test function.** `TestConsoleRenders` in `internal/httpapi/console_test.go`

### 3.8 Determinism and bounds

<!-- sdd:item id=TC-0070 stage=verify status=approved derives_from=REQ-0004,REQ-0090 -->
#### TC-0070 — Identical input produces identical output

**Level.** e2e. **Verifies.** REQ-0004, REQ-0090.

**Steps.** Run case `C1` twice in independent engines and compare every generated
identifier, every collection order and the rendered report; then run the binary twice as
a subprocess and compare stdout.

**Expected.** All comparisons are byte-identical.

**Test function.** `TestDeterministicRun` in `internal/orchestrator/e2e_test.go`

<!-- sdd:item id=TC-0071 stage=verify status=approved derives_from=REQ-0094 -->
#### TC-0071 — Case memory is bounded

**Level.** integration. **Verifies.** REQ-0094.

**Steps.** Drive a case past `maxEvidencePerCase`, `maxHypothesesPerRound` and
`maxDemandsPerRound`.

**Expected.** Retained counts stay within their caps and an `evidence_truncated` event
records the truncation.

**Test function.** `TestCaseBounds` in `internal/orchestrator/apply_test.go`

<!-- sdd:item id=TC-0072 stage=verify status=approved derives_from=REQ-0024 -->
#### TC-0072 — Hypothesis identity is stable across rounds

**Level.** unit. **Verifies.** REQ-0024.

**Steps.** Hypothesise over round-1 evidence, then over round-2 evidence that includes
round 1.

**Expected.** A signature that produced a hypothesis in round 1 reuses its identifier in
round 2 with an updated score, rather than producing a duplicate.

**Test function.** `TestHypothesisIdentityStable` in `internal/reasoner/rule_test.go`

<!-- sdd:item id=TC-0073 stage=verify status=approved derives_from=REQ-0060 -->
#### TC-0073 — An agent failure does not abort the case

**Level.** integration. **Verifies.** REQ-0060.

**Steps.** Run a round with one collector returning an error.

**Expected.** An `agent_failed` event is recorded, the remaining collectors' evidence is
applied, and the case advances.

**Test function.** `TestAgentFailureIsolated` in `internal/orchestrator/engine_test.go`

<!-- sdd:item id=TC-0074 stage=verify status=approved derives_from=REQ-0001 -->
#### TC-0074 — Plane dependency direction is enforced

**Level.** governance. **Verifies.** REQ-0001.

**Steps.** Parse the import list of every package under `internal/` and compare against
the declared module ordering.

**Expected.** No reasoning-plane package imports a store, an adapter or the event log;
no signal-plane package imports the reasoning or control plane; `domain` imports nothing
from `internal/`.

**Test function.** `TestPlaneDependencies` in `internal/domain/arch_test.go`

### 3.9 Evaluation

<!-- sdd:item id=TC-0080 stage=verify status=approved derives_from=REQ-0080 -->
#### TC-0080 — Three modes run over identical inputs

**Level.** integration. **Verifies.** REQ-0080.

**Steps.** Run case `C1` in `single`, `multi_no_critic` and `multi_with_critic`.

**Expected.** All three complete and record their mode; the single and no-critic modes
rank `sig-traffic-surge` first; the full flow ranks `sig-db-pool-exhaustion` first.

**Test function.** `TestThreeModes` in `internal/eval/eval_test.go`

<!-- sdd:item id=TC-0081 stage=verify status=approved derives_from=REQ-0081 -->
#### TC-0081 — Metrics are computed from executed runs

**Level.** integration. **Verifies.** REQ-0081.

**Steps.** Run the full case library in all three modes and summarise.

**Expected.** Each mode row carries top-1 accuracy, top-3 coverage, evidence-kind
completeness, critic corrections, blocked actions and the sample size; the full flow's
top-1 accuracy exceeds the single-agent mode's.

**Test function.** `TestEvaluationSummary` in `internal/eval/eval_test.go`

<!-- sdd:item id=TC-0082 stage=verify status=approved derives_from=REQ-0082 -->
#### TC-0082 — The case library is declarative

**Level.** unit. **Verifies.** REQ-0082.

**Steps.** Load the catalog and assert that every case resolves its expected signature;
load an additional case from a test-only embedded set.

**Expected.** The catalog loads, references resolve, and the extra case is discovered
without any code change.

**Test function.** `TestCatalogLoad` in `internal/catalog/catalog_test.go`

### 3.10 Delivery

<!-- sdd:item id=TC-0090 stage=verify status=approved derives_from=REQ-0092 -->
#### TC-0090 — The container definition is self-contained

**Level.** delivery. **Verifies.** REQ-0092.

**Steps.** Parse `deploy/docker/Dockerfile` and the compose file.

**Expected.** The build stage runs no network fetch beyond the base images, a non-root
user is declared, a health check is declared, and the compose stack declares the API,
the demo services and their dependencies.

**Test function.** `TestDeliveryArtifacts` in `internal/httpapi/delivery_test.go`

<!-- sdd:item id=TC-0091 stage=verify status=approved derives_from=REQ-0093 -->
#### TC-0091 — The manual exists in both languages and stays in parity

**Level.** governance. **Verifies.** REQ-0093.

**Steps.** Run the bilingual lint over the documentation tree.

**Expected.** The manual pair exists, declares equal versions, and reports no parity
finding.

**Test function.** `TestManualBilingualParity` in `internal/sdd/manual_test.go`

<!-- sdd:item id=TC-0092 stage=verify status=approved derives_from=REQ-0095 -->
#### TC-0092 — The release pipeline publishes a gated linux/amd64 artifact

**Level.** delivery. **Verifies.** REQ-0095.

**Steps.** Parse `.github/workflows/release.yml`.

**Expected.** The workflow triggers on a `v*` tag and can also be started manually with a
version input; it runs the delivery gate before any
publishing step; it builds for `linux/amd64` explicitly; it starts the built image and
waits for `/healthz` before publishing; it tags the image with both the version and the
commit SHA; and it creates a release whose assets include a loadable image tarball and
`SHA256SUMS`.

The ordering assertion is the substantive one. A workflow that published first and
verified afterwards would satisfy every individual clause of REQ-0095 while defeating
its purpose, so the test compares byte offsets: the gate and the health check must both
appear before the first push to a registry.

**Test function.** `TestReleasePipeline` in `internal/httpapi/delivery_test.go`

### 3.11 Live signal adapters

<!-- sdd:item id=TC-0100 stage=verify status=approved derives_from=REQ-0096 -->
#### TC-0100 — The Prometheus adapter maps a range response to a series

**Level.** unit. **Verifies.** REQ-0096.

**Steps.** Serve a recorded `query_range` payload from an `httptest` server and query it.

**Expected.** Points appear in timestamp order with values parsed; labels and unit are
carried; the request sent `query`, `start`, `end` and `step`. A `NaN` sample is absent
from the series rather than present as `0`.

**Test function.** `TestPrometheusRange` in `internal/signal/prometheus/prometheus_test.go`

<!-- sdd:item id=TC-0101 stage=verify status=approved derives_from=REQ-0096 -->
#### TC-0101 — The Prometheus adapter bounds results and reports failure typed

**Level.** unit. **Verifies.** REQ-0096.

**Steps.** Serve a payload exceeding `MaxRows`, then an API error, then close the server
and query the dead endpoint.

**Expected.** The over-long series truncates to the most recent `MaxRows` points and
reports truncation; the API error and the unreachable endpoint both surface as
`signal.SourceError` naming the port, not as a generic error or a panic.

**Test function.** `TestPrometheusBoundsAndErrors` in
`internal/signal/prometheus/prometheus_test.go`

<!-- sdd:item id=TC-0102 stage=verify status=approved derives_from=REQ-0097 -->
#### TC-0102 — The container log adapter demultiplexes and filters

**Level.** unit. **Verifies.** REQ-0097.

**Steps.** Serve a multiplexed log stream, including a payload whose bytes contain the
frame header pattern, and search it with terms and a window.

**Expected.** stdout and stderr records are recovered with correct levels and timestamps;
the adversarial payload does not desynchronise the parser; only lines matching every term
and inside the window are returned, bounded and marked.

**Test function.** `TestContainerLogSearch` in
`internal/signal/containerlog/containerlog_test.go`

<!-- sdd:item id=TC-0103 stage=verify status=approved derives_from=REQ-0098 -->
#### TC-0103 — The default profile is the fixture profile

**Level.** integration. **Verifies.** REQ-0098.

**Steps.** Validate an empty configuration, the fixture profile, a complete live profile
and several incomplete ones; then build a signal set from an empty configuration, from the
fixture profile, and from an unknown profile name.

**Expected.** The empty configuration and the fixture profile both yield the fixture set;
the live profile substitutes only the metric and log ports; the unknown name is an error
naming the offending value rather than a silent fallback.

Validation is available without building, so an entry point rejects a bad profile at
start-up. Deferring the check to build time would let a server with a mistyped profile
start cleanly and then fail one case at a time — the failure arriving once someone is
already relying on it.

**Test function.** `TestProfileSelection` in `internal/signal/profile/profile_test.go`

### 3.12 Overfitting

<!-- sdd:item id=TC-0104 stage=verify status=approved derives_from=REQ-0099 -->
#### TC-0104 — The loudest evidence does not win, and neither does a fixed bias

**Level.** e2e. **Verifies.** REQ-0099.

**Steps.** Run case `C4` end to end in all three modes. Its logs are dominated by
database connection errors, while its metric and change evidence refute every database
explanation and support an unprecedented traffic peak.

**Expected.** The accepted root cause is `sig-traffic-surge`. Neither
`sig-db-pool-exhaustion` nor `sig-db-outage` is accepted despite carrying the highest log
volume in the case.

This is the case that keeps the evaluation honest. C1 is won by demoting
`sig-traffic-surge` from first place, so a system that had learned "the leading
explanation is wrong", or simply "traffic is never the cause", would still score
perfectly on C1, C2 and C3. Only a case where the critic must decline to object
distinguishes a discriminator from a bias.

**Test function.** `TestMisleadingLogsDoNotWin` in `internal/orchestrator/e2e_test.go`

### 3.13 Model reasoner

<!-- sdd:item id=TC-0105 stage=verify status=approved derives_from=REQ-0100 -->
#### TC-0105 — The adapter maps a provider response and scores it locally

**Level.** unit. **Verifies.** REQ-0100.

**Steps.** Serve a recorded provider response selecting a catalog signature with evidence
identifiers drawn from the snapshot, and hypothesise against it.

**Expected.** A hypothesis is returned whose claim and mechanism come from the catalog
signature, whose supporting evidence is exactly the cited identifiers, and whose score
decomposes into the declared weighted terms. Any score the provider offered is ignored.

**Test function.** `TestModelHypothesise` in `internal/reasoner/model/model_test.go`

<!-- sdd:item id=TC-0106 stage=verify status=approved derives_from=REQ-0100 -->
#### TC-0106 — Provider output is untrusted and failure is not silence

**Level.** unit. **Verifies.** REQ-0100.

**Steps.** Serve responses citing an unknown evidence identifier, an unknown signature,
an invalid verdict, and a claim of the adapter's own invention; then serve a non-200
status and an unreachable endpoint.

**Expected.** Each invalid item is dropped while valid items in the same response survive.
No provider-authored prose appears as a hypothesis claim. A transport or status failure
returns a `ProviderError`, never an empty result — an empty hypothesis list means "nothing
matched", and a failing provider must not be able to impersonate that conclusion.

**Test function.** `TestModelOutputIsUntrusted` in `internal/reasoner/model/model_test.go`

## 4. Coverage matrix

Generated by `sddctl matrix`; the authoritative version is
`docs/traceability-matrix.md`, regenerated on every governance run. Every P0 requirement
must show a non-empty test column before the M1 gate passes.

## 5. Checkpoint log

Recorded in `docs/checkpoints.en.md` and its Chinese counterpart, one row per
implementation wave, carrying the command run, the result and the date.
