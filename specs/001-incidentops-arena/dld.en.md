---
id: DLD-DOC-001
lang: en
counterpart: dld.zh.md
doc_version: 1.0.0
status: approved
stage: dld
---

# IncidentOps Arena — Detailed Design

## 1. Reading guide

A DLD item is the unit an engineer implements and a test verifies. Constants live here,
never in prose elsewhere, so that changing one cascades through `sddctl drift` to the
code that reads it.

Every item below becomes one or more `// sdd:impl <ID>` anchors in source.

## 2. Constants and configuration

| Key | Value | Effect |
|---|---|---|
| `maxRounds` | 3 | Collection rounds per case (ARC-011) |
| `maxHypothesesPerRound` | 3 | Ranked hypotheses retained per round |
| `maxDemandsPerRound` | 4 | Evidence demands carried into the next round |
| `acceptThreshold` | 0.75 | Minimum leading score to enter remediation |
| `closeCallMargin` | 0.15 | Top-two gap below which the case is a close call |
| `minEvidenceKinds` | 2 | Distinct kinds required to leave analysis |
| `changeLookback` | 30m | Extension before incident start for change queries |
| `anomalyFactor` | 3.0 | Peak must exceed baseline by this factor |
| `anomalySustain` | 2 | Consecutive points required to confirm onset |
| `coMovementWindow` | 60s | Onset spread within which series are co-moving |
| `collapseFactor` | 0.25 | Share of baseline a series must fall below to count as collapsed |
| `bounds.MaxRows` | 200 | Rows any single tool result may carry |
| `bounds.MaxChars` | 8000 | Characters any single tool result may carry |
| `bounds.MaxSpan` | 2h | Time span any single tool query may cover |
| `logSamplesPerCluster` | 3 | Representative lines retained per cluster |
| `maxClusters` | 5 | Clusters retained per log query |
| `maxEvidencePerCase` | 200 | Retention cap before truncation is recorded |
| `subscriberBuffer` | 64 | Events buffered per stream subscriber |

### Scoring weights

| Term | Weight |
|---|---|
| `metric_alignment` | 0.30 |
| `log_alignment` | 0.25 |
| `change_correlation` | 0.20 |
| `topology_plausibility` | 0.10 |
| `historical_similarity` | 0.10 |
| `remediation_verifiability` | 0.05 |
| `counterEvidencePenalty` (per unresolved item) | 0.20 |

Weights sum to 1.00, asserted by TC-0022.

## 3. Domain

<!-- sdd:item id=DLD-1001 stage=dld status=approved derives_from=HLD-001 -->
### DLD-1001 — Roles, identifiers and time windows

**File.** `internal/domain/ids.go`

**Types.** `Role string`, `TimeWindow struct{ Start, End time.Time }`.

**Behaviour.**

1. `CaseID(alert Alert) string` returns `"inc-"` followed by the first 10 hex characters
   of `sha256(alertName + "|" + service + "|" + startsAt.UTC().RFC3339)`. No clock, no
   randomness, so the same alert always yields the same case identifier.
2. `NewID(prefix string, role Role, seq int) string` returns
   `fmt.Sprintf("%s-%s-%03d", prefix, role, seq)`. Prefixes: `e` evidence, `h`
   hypothesis, `c` critique, `d` demand, `a` action.
3. `TimeWindow.Extend(d)` returns a window with `Start` moved back by `d`.
4. `TimeWindow.Contains(t)` is inclusive of both ends.

**Invariants.** Identifiers are case-scoped and stable across processes.

**Checkpoint.** TC-0070 — two runs produce identical identifiers.

<!-- sdd:item id=DLD-1002 stage=dld status=approved derives_from=HLD-001 -->
### DLD-1002 — Evidence

**File.** `internal/domain/evidence.go`

**Types.** `EvidenceKind` with constants `metric`, `log`, `change`, `topology`,
`knowledge`, `verification`; `Evidence` as specified in HLD-001, with
`Facts map[string]string`.

**Behaviour.** `Evidence.Validate()` requires a non-empty `ID`, a known `Kind`, a
non-empty `Source` and `RawRef`, and `Confidence` in `[0,1]`. `FactsSorted()` returns
key-ordered pairs so rendering and hashing are deterministic.

**Errors.**

| Condition | Returned error | Caller expectation |
|---|---|---|
| Unknown kind | `ErrUnknownEvidenceKind` | Contribution rejected, event recorded |
| Confidence out of range | `ErrConfidenceRange` | Contribution rejected |
| Empty `RawRef` | `ErrMissingRawRef` | Contribution rejected — REQ-0002 |

**Checkpoint.** TC-0002 — evidence without a raw reference is refused.

<!-- sdd:item id=DLD-1003 stage=dld status=approved derives_from=HLD-001 -->
### DLD-1003 — Hypothesis and score breakdown

**File.** `internal/domain/hypothesis.go`

**Types.** `Hypothesis{ID, SignatureID, Claim, Mechanism string; Supporting, Counter
[]string; Resolved map[string]bool; Breakdown ScoreBreakdown; Status HypothesisStatus}`
with statuses `proposed`, `challenged`, `accepted`, `rejected`.

**Behaviour.** `SupportingKinds(ev map[string]Evidence) []EvidenceKind` returns the
sorted distinct kinds of the supporting evidence. `UnresolvedCounters()` counts counter
identifiers not marked resolved.

**Checkpoint.** TC-0021 — a hypothesis with no supporting identifiers is invalid.

<!-- sdd:item id=DLD-1004 stage=dld status=approved derives_from=HLD-001 -->
### DLD-1004 — Critique and evidence demand

**File.** `internal/domain/critique.go`

**Types.** `Verdict` with constants `accept`, `accept_with_risk`, `revise`, `reject`;
`Critique{ID, HypothesisID, Rule, Category, Challenge string; Verdict Verdict; DemandIDs
[]string}`; `EvidenceDemand{ID, Descriptor, Reason string; Kind EvidenceKind; SatisfiedBy string;
Round int}`. `Round` is the collection round the demand was first raised in, which is
what separates a demand nobody has tried to answer from one a round attempted and no
source could supply (DLD-1063).

**Behaviour.** `Verdict.Severity()` maps `accept`=0, `accept_with_risk`=1, `revise`=2,
`reject`=3. `CombineVerdicts(vs ...Verdict) Verdict` returns the most severe, defaulting
to `accept` for an empty input.

**Invariants.** A demand is satisfied only by evidence whose `Facts["demand"]` equals its
descriptor.

**Checkpoint.** TC-0030 — every examined hypothesis carries a verdict from the set.

<!-- sdd:item id=DLD-1005 stage=dld status=approved derives_from=HLD-001 -->
### DLD-1005 — Action, risk and approval

**File.** `internal/domain/action.go`

**Types.** `Risk` with `low`, `medium`, `high`; `Action{ID, Title, Tool string; Args
map[string]string; Risk Risk; Preconditions []string; Rollback *ActionCall; Verify
[]string; Approval *ApprovalDecision; Execution *ExecutionResult}`;
`ApprovalDecision{Decision, By, Comment string; At time.Time}`;
`ExecutionResult{Outcome, Detail string; Duration time.Duration}`.

**Behaviour.** `Action.Validate()` refuses `medium` or `high` risk without both a
rollback and at least one verify signal (REQ-0040). `Action.Fingerprint()` hashes tool
plus sorted arguments; the executor compares it against the approved fingerprint to
detect post-approval tampering.

**Checkpoint.** TC-0040, TC-0043 — a tampered action is refused at execution.

<!-- sdd:item id=DLD-1006 stage=dld status=approved derives_from=HLD-002 -->
### DLD-1006 — Contribution algebra and role capabilities

**File.** `internal/domain/contribution.go`

**Types.** `ContributionKind` with `add_evidence`, `propose_hypothesis`,
`raise_critique`, `demand_evidence`, `propose_action`, `record_verification`,
`record_execution`; `Contribution` as in HLD-002.

**Behaviour.**

1. `Validate()` requires exactly the payload field matching `Kind` to be non-nil and all
   others nil, then delegates to the payload's own `Validate`.
2. `roleCapabilities` is a package-level table:
   - collectors (`metrics`, `logs`, `change`, `topology`, `knowledge`) → `add_evidence`
   - `analysis` → `propose_hypothesis`
   - `critic` → `raise_critique`, `demand_evidence`
   - `remediation` → `propose_action`
   - `executor` → `record_execution`
   - `verification` → `add_evidence`, `record_verification`
   - `baseline` → `add_evidence`, `propose_hypothesis`, `propose_action`
3. `Role.MayEmit(k)` consults the table; an unknown role may emit nothing.

**Invariants.** No role may emit `raise_critique` except `critic`; no role may emit
`propose_action` except `remediation` and `baseline`.

**Checkpoint.** TC-0003 — a collector proposing a hypothesis is rejected.

<!-- sdd:item id=DLD-1007 stage=dld status=approved derives_from=HLD-003 -->
### DLD-1007 — Events and case projection

**File.** `internal/domain/event.go`, `internal/domain/case.go`

**Types.** `EventType` constants: `case_created`, `state_changed`, `round_started`,
`agent_started`, `agent_completed`, `agent_failed`, `tool_called`, `evidence_added`,
`evidence_truncated`, `hypothesis_proposed`, `hypothesis_rejected`, `hypothesis_scored`,
`critique_raised`, `evidence_demanded`, `demand_satisfied`, `demand_unmet`,
`contribution_rejected`, `action_proposed`, `approval_requested`, `approval_recorded`,
`action_executed`, `action_refused`, `verification_recorded`, `budget_exhausted`,
`report_generated`, `case_closed`. `Event.Payload json.RawMessage` carries the entity
for events that create one.

**Behaviour.** `(*Case).Apply(e)` switches on type and mutates only the projection.
`Replay(caseID, events)` folds from an empty case and validates that sequence numbers
are contiguous from 1.

`Leading()` returns the highest-scoring hypothesis the critic has not rejected, and
`Snapshot.Leading()` mirrors it so an agent and the control plane agree on which
explanation is the case's answer. Score measures how well the evidence fits; a verdict
measures whether the explanation is admissible. An explanation can fit beautifully and
still be impossible, so reporting the top-ranked one as the answer in a report that also
records its rejection would make the system contradict itself. The ranking is left
untouched: it still shows the rejected explanation first, with its verdict and
counter-evidence beside it.

Only `reject` is skipped. `revise` says the critic wants more work, not that the
explanation is inadmissible, and skipping it too made the reported answer depend on the
severity of an unfinished objection rather than on the evidence: a case whose best
explanation scored 0.84 under a `revise` reported a 0.09 explanation instead, because that
one had attracted a milder critique. Whether an explanation may be *acted on* is a
different question, asked separately by `CanRemediate` (DLD-1062), which does require a
permitting verdict.

**Invariants.** `Replay(events)` equals the live projection for every case
(REQ-0003).

**Checkpoint.** TC-0050 — replay equality and report equality.

## 4. Storage

<!-- sdd:item id=DLD-1010 stage=dld status=approved derives_from=HLD-004 -->
### DLD-1010 — Store port and in-memory adapter

**File.** `internal/store/store.go`, `internal/store/memory.go`

**Behaviour.** The memory adapter holds `map[string][]Event` plus an ordered header
slice, guarded by one mutex. `Append` assigns nothing — sequence numbers arrive already
set by the orchestrator — and rejects a batch whose first sequence is not
`len(existing)+1`. `Events(from)` returns a copy.

**Errors.** `ErrCaseNotFound`, `ErrSequenceGap`.

**Checkpoint.** TC-0004 — a sequence gap is rejected.

<!-- sdd:item id=DLD-1011 stage=dld status=approved derives_from=HLD-004 -->
### DLD-1011 — File-backed adapter

**File.** `internal/store/file.go`

**Behaviour.** One `<root>/cases/<caseID>.jsonl` per case; each line is one JSON event.
`Append` opens with `O_APPEND|O_CREATE|O_WRONLY`, writes all lines, then `Sync`. The
case index is rebuilt by scanning the directory at construction and reading the first
line of each file, so the index is derived, not authoritative (ADR-004).

**Errors.** Wraps I/O errors; a malformed line fails the whole read with the line number.

**Checkpoint.** TC-0005 — a case survives a store restart.

## 5. Signal plane

<!-- sdd:item id=DLD-1020 stage=dld status=approved derives_from=HLD-005 -->
### DLD-1020 — Signal ports and bounds

**File.** `internal/signal/ports.go`, `internal/signal/bounds.go`

**Behaviour.** `Bounds.ApplyRows`, `ApplyChars` and `ApplySpan` truncate and return a
`truncated bool`. `SignalSet` bundles the six ports plus the bounds so an agent receives
one value.

`SourceError` is the typed failure every live adapter returns, carrying the port name,
the endpoint and the underlying cause. It exists here rather than in an adapter package
because a collector must be able to recognise a degraded source without importing the
adapter that produced it (ARC-001).

**Invariants.** No adapter returns a result exceeding bounds; truncation is always
reported, never silent (REQ-0016).

**Checkpoint.** TC-0016 — an over-long result is truncated and marked.

<!-- sdd:item id=DLD-1021 stage=dld status=approved derives_from=HLD-005 -->
### DLD-1021 — Anomaly onset and co-movement

**File.** `internal/signal/anomaly.go`

A collapse is detected as the mirror of a rise, and is just as much a finding. An
availability gauge falling to zero, a throughput series going flat, a queue draining —
a detector that only recognises growth is blind to all of them, and `sig-db-outage`
expressed its requirement as `saturated`, which is computed from a declared capacity no
availability gauge has. The requirement was therefore unsatisfiable and the signature
could never be fully matched by anything the collectors produce.

The sustain requirement applies to a collapse exactly as to a rise, so a single dipping
sample is noise rather than a finding, and a series whose baseline was already at zero
has not fallen. `Summary()` says the series *fell*: describing a collapse as a rise would
misreport what the data shows, which is the one thing that text must never do.

**Algorithm.**

```
Analyse(series, window):
  pre        := points strictly before window.Start
  baseline   := median(pre)            // 0 when pre is empty
  peak       := max(points within window)
  threshold  := max(baseline * anomalyFactor, baseline + epsilon)
  onset      := first t in window where the next anomalySustain points all exceed threshold
  floor      := baseline * collapseFactor              // only when baseline > baselineFloor
  collapse   := first t in window where the next anomalySustain points all sit at or below floor
  collapsed  := collapse exists
  anomalous  := onset exists or collapsed
  onset      := the earlier of onset and collapse
  saturated  := series.Capacity > 0 and peak >= series.Capacity
  ratio      := peak / baseline        // reported as "n/a" when baseline == 0
```

`CoMoving(a, b)` is true when both are anomalous and `|onset(a) - onset(b)| <=
coMovementWindow`.

**Complexity.** Linear in points; no allocation beyond the returned struct.

**Checkpoint.** TC-0011 — onset is within one sample of the fixture's step.

<!-- sdd:item id=DLD-1022 stage=dld status=approved derives_from=HLD-005 -->
### DLD-1022 — Log clustering

**File.** `internal/signal/cluster.go`

**Algorithm.**

```
Normalise(line):
  replace runs of digits with "#"
  replace quoted segments with '"…"'
  replace hex runs of length >= 8 with "<id>"
Cluster(lines):
  key    := Normalise(message)
  group  by key, counting and tracking first/last timestamps
  sort   by count desc, then first-seen asc, then key asc
  keep   maxClusters clusters, logSamplesPerCluster samples each
```

**Invariants.** Cluster order is total and deterministic.

**Checkpoint.** TC-0012 — 184 and 3 line groups produce two ordered clusters.

<!-- sdd:item id=DLD-1023 stage=dld status=approved derives_from=HLD-006 -->
### DLD-1023 — Fixture adapters

**File.** `internal/signal/fixture/fixture.go`

**Behaviour.** `NewFixtureSet(fc FaultCase)` returns a `SignalSet` whose adapters read
only from the case definition. The metric adapter matches a query by series name; the
log adapter filters declared lines by service, window and substring; the change adapter
filters declared changes by service and window; topology and knowledge return declared
values. `DemandResponses` are served by the adapter whose `Kind` matches, keyed by
descriptor, and each carries `Facts["demand"] = descriptor` so the orchestrator can mark
the demand satisfied.

A demand response may name a `Service` and a `Logs` term. `Service` defaults to the
alert's service, which is right until the alerting service turns out to be a victim: then
the change history and log lines that settle the case belong to something upstream of it,
and a response scoped to the alert would return nothing. A response that names a service
records it in a `subject` fact, which is how a later rule can tell whether an explanation
rests on evidence from the upstream (DLD-1034).

**Invariants.** Every returned value derives from the case file; the adapter performs no
I/O and holds no clock.

**Checkpoint.** TC-0010 — the reference scenario completes offline.

<!-- sdd:item id=DLD-1024 stage=dld status=approved derives_from=HLD-006 -->
### DLD-1024 — Simulated actuator

**File.** `internal/signal/fixture/actuator.go`

**Behaviour.** Holds an in-process configuration map seeded from the case. `Invoke`
supports `set_config`, `restart_service`, `scale_service` and `create_ticket`, records
the call, and returns a result describing the state change. After a `set_config` that
matches the case's `recoveryTrigger`, subsequent metric queries return the case's
`postRecovery` series, which is how verification observes recovery.

**Checkpoint.** TC-0051 — verification observes recovery only after the action.

<!-- sdd:item id=DLD-1025 stage=dld status=approved derives_from=HLD-019 -->
### DLD-1025 — Prometheus metric adapter

**File.** `internal/signal/prometheus/prometheus.go`

**Types.** `Source{endpoint string; client *http.Client; step time.Duration}`,
`Options{Step, Timeout time.Duration; Client *http.Client; SeriesMap map[string]string}`.

**Behaviour.** `Range` issues `GET /api/v1/query_range` with `query`, `start`, `end` and
`step`. The `query` is resolved from `SeriesMap` — a declarative name-to-PromQL mapping,
so adding a series is configuration rather than code. The response
(`{"status","data":{"resultType","result":[{"metric":{},"values":[[ts,"val"],...]}]}}`)
is decoded with `encoding/json` into `signal.Series`, sorted by timestamp.

A sample whose value parses to `NaN` is dropped rather than recorded as zero; the point
is absent, and absence is what the reasoner must see. A `status` of `error` becomes a
`signal.SourceError` carrying the API's `errorType` and `error` fields.

`SeriesNames` issues `GET /api/v1/label/__name__/values`, filtered to the names present
in `SeriesMap` so the adapter never advertises series it cannot resolve.

Bounds are applied after decoding through `Bounds.ApplyRowsPoints`, which keeps the
*earliest* points. That end is the load-bearing one: onset detection (DLD-1021) and the
`change_correlation` term (DLD-1032) both read the start of the window, so discarding the
head of a series would silently move a hypothesis's apparent onset.

**Checkpoint.** TC-0100, TC-0101

<!-- sdd:item id=DLD-1026 stage=dld status=approved derives_from=HLD-019 -->
### DLD-1026 — Container log adapter

**File.** `internal/signal/containerlog/containerlog.go`

**Types.** `Source{host string; client *http.Client}`, `Options{Timeout time.Duration;
Client *http.Client; Container func(service string) string}`.

**Behaviour.** `Search` issues `GET /containers/{id}/logs?stdout=1&stderr=1&timestamps=1`
against the runtime's HTTP API, over a Unix socket when `host` begins `unix://` — dialled
through a custom `http.Transport.DialContext`, which is why no client library is needed.

The runtime frames its stream as an 8-byte header per record: byte 0 is the stream type
(1 = stdout, 2 = stderr), bytes 4–7 are the payload length, big-endian. `readFrames`
consumes header-then-payload pairs, so a message containing the header's byte pattern
cannot desynchronise the parser — which reading line-by-line would allow.

Each record's leading RFC3339Nano timestamp is parsed into `LogLine.At`; the stream type
becomes `Level` (`stderr` → `error`, `stdout` → `info`) unless the message itself carries
a recognised level. Lines are filtered to those inside `q.Window` containing every term
in `q.Terms`, case-insensitively, then bounded.

**Checkpoint.** TC-0102

<!-- sdd:item id=DLD-1027 stage=dld status=approved derives_from=HLD-019 -->
### DLD-1027 — Profile selection

**File.** `internal/signal/profile/profile.go`

**Types.** `Profile string` with constants `Fixture` and `Live`; `Config{Profile,
PrometheusURL, ContainerHost string; SeriesMap map[string]string}`.

**Behaviour.** `Build` returns the fixture set for `Fixture` and for the empty string —
the zero value is the safe value, so a missing configuration cannot silently select a
live backend. For `Live` it substitutes the Prometheus and container-log adapters for the
metric and log ports and leaves the remaining four ports on their fixture adapters, since
M4 covers only those two.

An unknown profile name is an error naming the offending value and the legal set; it is
never treated as a request for the default, because a typo that silently falls back to
fixtures would make a live deployment look healthy while reading simulated data.

**Checkpoint.** TC-0103

<!-- sdd:item id=DLD-1035 stage=dld status=approved derives_from=HLD-020 -->
### DLD-1035 — Model reasoner adapter

**File.** `internal/reasoner/model/model.go`

**Types.** `Reasoner{endpoint, model string; client *http.Client; cat *catalog.Catalog;
cfg reasoner.Config}`, `Options{Timeout time.Duration; Client *http.Client; APIKey
string; Temperature float64}`, `ProviderError{Endpoint string; Status int; Err error}`.

**Behaviour.** `Name()` returns `"model"`. `Hypothesise` builds a request describing the snapshot's evidence — each
item's identifier, kind, source and summary — and the catalog's signature identifiers with
their claims. It asks the provider which signatures the evidence supports and which
evidence identifiers support each. The response is decoded as
`{"selections":[{"signature_id","evidence_ids":[],"reasoning"}]}`.

Each selection is then validated and scored locally:

1. A `signature_id` absent from the catalog is dropped.
2. An `evidence_ids` entry absent from the snapshot is dropped from that selection; a
   selection left with no evidence is dropped entirely, because REQ-0021 would reject it
   downstream anyway and dropping here keeps the reason legible.
3. The remaining evidence is turned into a `catalog.MatchResult` and scored by
   `reasoner.Score` with the configured weights. **The provider supplies no score**
   (ADR-007): a number a model asserts does not decompose, and REQ-0022 requires that it
   does.
4. Claim and mechanism come from the catalog signature, not from the response, so no
   provider text reaches a report as a causal claim.

`Critique` follows the same shape against
`{"critiques":[{"hypothesis_id","category","challenge","verdict","demands":[]}]}`, dropping
any entry whose verdict is outside the closed set or whose hypothesis identifier is
unknown.

**Errors.** A transport failure, a non-200 status or an undecodable body returns
`ProviderError`. An empty but well-formed response is a legitimate "nothing matched" and
returns an empty slice with no error — the two must stay distinguishable.

**Checkpoint.** TC-0105, TC-0106

## 6. Catalog and reasoning

<!-- sdd:item id=DLD-1030 stage=dld status=approved derives_from=HLD-006 -->
### DLD-1030 — Embedded catalog

**File.** `internal/catalog/catalog.go`, `internal/catalog/data/*.json`

**Types.** `Catalog{Signatures []Signature; Cases []FaultCase; Runbooks []Runbook}`.

**Behaviour.** `LoadCatalog()` parses files embedded with `//go:embed data`, validating
that signature and case identifiers are unique and that every case's
`expectedSignature` exists. Ordering follows file name then declaration order, so
iteration is deterministic.

**Errors.** A duplicate identifier or dangling reference fails the load, which fails
startup — a broken catalog must not run.

**Checkpoint.** TC-0082 — adding a case file requires no code change.

<!-- sdd:item id=DLD-1031 stage=dld status=approved derives_from=HLD-006 -->
### DLD-1031 — Fault signature schema and matcher

**File.** `internal/catalog/signature.go`

**Types.**

```go
type Pattern struct {
    Kind      domain.EvidenceKind
    Match     []string          // all must appear in summary or fact values, case-insensitive
    Saturated bool              // when true, Facts["saturated"] must be "true"
}
type Signature struct {
    ID, Claim, Mechanism string
    Requires             []Pattern
    Discriminators       []Discriminator
    Remediation          *RemediationTemplate
    RunbookIDs           []string
}
```

**Algorithm.**

```
MatchPattern(p, ev):  every s in p.Match appears in lower(ev.Summary + joined fact values)
                      and (not p.Saturated or ev.Facts["saturated"] == "true")
Match(sig, evidence): for each pattern, the first matching evidence (in id order)
                      returns matched[kind] += 1 / requiredCount[kind]
                      the signature matches at all when >= 1 pattern matched
```

**Invariants.** Matching is order-independent and case-insensitive; the same evidence set
always yields the same match set.

**Checkpoint.** TC-0020 — the reference evidence matches the expected signatures.

<!-- sdd:item id=DLD-1032 stage=dld status=approved derives_from=HLD-008 -->
### DLD-1032 — Scoring

**File.** `internal/reasoner/score.go`

**Algorithm.**

```
term(metric_alignment)            = fraction of the signature's metric patterns matched
term(log_alignment)               = fraction of its log patterns matched
term(change_correlation)          = 1.0 when a matched change timestamp < anomaly onset
                                    0.5 when a change matched but onset is unknown
                                    0.0 otherwise
term(topology_plausibility)       = 1.0 candidate origin, 0.4 affected only, 0.0 contradicted
term(historical_similarity)       = best runbook match score in [0,1]
term(remediation_verifiability)   = 1.0 when the signature declares a remediation with
                                    at least one verify signal, else 0.0
total = Σ weight_i · value_i − counterEvidencePenalty · unresolvedCounters
total is clamped to [0, 1]
```

`Rank` orders by total descending, then by supporting-kind count descending, then by
identifier ascending.

**Invariants.** `Σ breakdown.Terms[i].Contribution − Penalty == Total` before clamping,
within `1e-9`.

**Checkpoint.** TC-0022, TC-0023, TC-0024.

<!-- sdd:item id=DLD-1033 stage=dld status=approved derives_from=HLD-007 -->
### DLD-1033 — Rule reasoner: hypothesis formation

**File.** `internal/reasoner/rule.go`

**Behaviour.**

0. `Name()` returns `"rule"`; the model adapter returns `"model"`. The name is stored on
   the case at creation and travels into every evaluation row (REQ-0106).
1. For every signature, match against the snapshot's evidence.
2. Discard signatures with zero matched patterns.
3. Build one hypothesis per surviving signature, with `Supporting` set to the identifiers
   of every evidence item that contributed a non-zero term, and `Counter` set to the
   identifiers of evidence whose `Facts["counters"]` lists the signature.
4. Score, rank, and return the first `maxHypothesesPerRound`.
5. Reuse an existing hypothesis identifier when the signature already produced one, so
   scores update across rounds rather than duplicating.

**Checkpoint.** TC-0020, TC-0072.

<!-- sdd:item id=DLD-1034 stage=dld status=approved derives_from=HLD-009 -->
### DLD-1034 — Critique rule ensemble

**File.** `internal/reasoner/critique.go`

**Rules, each independently testable.**

| Rule | Trigger | Emits |
|---|---|---|
| `coverage_gap` | A required pattern of the hypothesis's signature matched no evidence | Critique `revise` plus a demand carrying that pattern's descriptor |
| `alternative_explanation` | Another signature matches at least one pattern, carries no unresolved counter-evidence, and has a discriminator that is neither already answered nor already supplied by its own matched requirements | Critique `revise` on the leading hypothesis naming the rival, plus a demand for the discriminator |
| `temporal_order` | Either the hypothesis's own evidence begins more than `coMovementWindow` after the alert, or its matched change timestamp is not earlier than the anomaly onset | Critique `reject`, and counter-evidence attached to the hypothesis |
| `source_vs_victim` | Topology evidence names an upstream service whose onset is earlier, **and** the hypothesis rests on no evidence whose `subject` fact is that service | Critique `revise` naming the upstream candidate, plus a demand for every unanswered requirement descriptor in the catalog |
| `unverifiable_remediation` | The signature declares no remediation, or one with no verify signal | Critique `accept_with_risk` |
| `close_call` | Top-two total gap `< closeCallMargin` | Critique `revise` on the leader, plus a demand for the leader's discriminator |

**Why `temporal_order` checks two things.** The change ordering compares a blamed change
against `HypothesisOnset`, which measures an explanation against its *own* supporting
metrics. That is what lets a genuine cause survive an unrelated series moving first
(DLD-1032) — and it is also a hole: a pool that was shrunk and then saturated is
internally coherent while explaining nothing about an incident that began four minutes
earlier. The first condition closes it by comparing the explanation's own onset against
the alert, because an explanation whose symptom postdates the incident is downstream of
whatever caused it.

**Why the rules stop asking.** A demand visible in the snapshot at critique time was
raised in an earlier round — the critic runs once per round and its demands are applied
afterwards — so a collection round has already been aimed at it. If it is still
unanswered, no source in this case can supply it. `coverage_gap` keeps its critique in
that situation and drops only the demand, because a requirement nothing establishes stays
unestablished whether or not anyone can go and look. The rules that challenge the
*leader* drop both, because a challenge whose evidence can never arrive holds the case in
`revise` forever (REQ-0031, REQ-0101).

**Why `alternative_explanation` stops at the budget edge.** With no round left, a demand
can never be answered, so the challenge that carries it can never clear — the case ends
unable to act on its own best explanation because of a question it was not given the
chance to ask. The rule stays silent instead; what went unexamined is recorded as an unmet
demand, and a genuine near-tie is still escalated by the rule that exists for that.

**Why `source_vs_victim` orders its demands by existing support.** It asks for the whole
catalog's unanswered requirement descriptors, so its demand list grows every time a
signature is added, and `maxDemandsPerRound` then decides which the investigation actually
pursues. Ordering by signature identifier let that decision fall out of alphabetical
accident: adding two signatures displaced the evidence a victim case needed, and it never
reached its cause. Signatures the evidence already partly supports come first — those are
leads; the rest are shots in the dark.

**Why `alternative_explanation` has two further skips.** The rule exists to stop a leader being
accepted while an equally consistent rival stands unexamined, so both skips ask the same
question: is this rival still unexamined?

*Already in hand.* `demandAlreadyAnswered` only recognises evidence that arrived as the
answer to a demand, so evidence a case collects by default is invisible to it. A rival
whose discriminating requirement its own match already satisfies would therefore be
demanded again, spending a whole collection round re-fetching a series the first round
read. Whether the critic holds the separating evidence must not depend on how it was
obtained.

*Already countered.* A rival the collected evidence argues against — one carrying
unresolved counter-evidence — has been examined and has lost ground. Continuing to demand
what would separate it asks for work the case has done. Worse, when no source can supply
that evidence the demand is re-issued every round, and the leader can never be accepted
at all: the case burns its round budget and escalates a question the evidence had already
settled.

**Why `source_vs_victim` carries both a skip and demands.** Without the skip it also
challenges the explanation that already blames the upstream, using the very topology
evidence that supports it. Without the demands it says "you may be looking at the wrong
service" and asks for nothing, so the investigation closes on a `revise` verdict instead
of redirecting — a challenge no evidence can answer is a veto, not a critique (REQ-0101).

The skip is decided from the evidence, not from the signature. A signature's remediation
names a service by catalog convention rather than by inference — every signature here
remediates `order-api` — so reading intent from that field would make every explanation
look upstream-aware. Evidence answering a demand records the service it concerns in a
`subject` fact, and that is what the rule reads.

**Behaviour.** Rules run in table order over the ranked hypotheses. Demands are
deduplicated by descriptor, ordered by `(rule order, hypothesis rank)`, and capped at
`maxDemandsPerRound`. The per-hypothesis verdict is `CombineVerdicts` over its critiques;
a hypothesis attracting no critique receives an explicit `accept`.

**Note on REQ-0032.** The specification permits skipping the alternative when the leader
is supported by every collected kind. This design uses the stronger trigger above — a
rival that nothing has yet ruled out always earns a challenge — which satisfies the
requirement and its acceptance criterion.

**Checkpoint.** TC-0030 through TC-0036, one test per rule.

## 7. Agents

<!-- sdd:item id=DLD-1040 stage=dld status=approved derives_from=HLD-010 -->
### DLD-1040 — Agent interface and collector agents

**File.** `internal/agent/agent.go`, `internal/agent/collectors.go`

**Behaviour.** Each collector runs two passes:

1. **Default pass** — the queries declared for its role: metrics queries the case's
   `defaultSeries`; logs queries the incident window with the alert's error terms;
   change queries the incident window *without* the lookback extension; topology and
   knowledge query the affected service.
2. **Demand pass** — for each unsatisfied demand whose `Kind` matches the collector's
   kind, ask the port for the response registered under that descriptor and emit it with
   `Facts["demand"]` set.

The deliberate omission of the lookback extension in the default pass is what makes the
first round incomplete and the critic's demand consequential; the extension is applied
only when a demand asks for it.

**Invariants.** A collector emits only `add_evidence`. Evidence carries
`RawRef` naming the port and the exact query.

**Checkpoint.** TC-0011, TC-0012, TC-0013, TC-0014, TC-0015, TC-0031.

<!-- sdd:item id=DLD-1041 stage=dld status=approved derives_from=HLD-010 -->
### DLD-1041 — Analysis, critic, remediation and verification agents

**File.** `internal/agent/analysis.go`, `critic.go`, `remediation.go`, `verification.go`

**Behaviour.**

- **Analysis** delegates to `Reasoner.Hypothesise` and wraps each result in a
  `propose_hypothesis` contribution.
- **Critic** delegates to `Reasoner.Critique`, emitting `raise_critique` and
  `demand_evidence`. It never emits a hypothesis.
- **Remediation** reads the accepted hypothesis's signature `RemediationTemplate`,
  materialises tool arguments from the matched change evidence (for example the previous
  value of `DB_POOL_SIZE`), and emits one `propose_action`.
- **Verification** re-queries every signal named in the executed action's `Verify` list
  over the window `[executedAt, executedAt + 5m]`, and emits both `add_evidence` of kind
  `verification` and a `record_verification` contribution per signal.

**Checkpoint.** TC-0020, TC-0030, TC-0040, TC-0051.

<!-- sdd:item id=DLD-1042 stage=dld status=approved derives_from=HLD-010 -->
### DLD-1042 — Single-agent baseline

**File.** `internal/agent/baseline.go`

**Behaviour.** One agent holding every port. It performs the default pass of all five
collectors once, then hypothesises, then proposes an action for the leading hypothesis —
with no critique, no demand and no second round. It is the honest model of "one context
window, one pass": it is given the same tools and the same default queries as the
multi-agent flow, and differs only in the absence of the adversarial round.

**Checkpoint.** TC-0080.

## 8. Policy and execution

<!-- sdd:item id=DLD-1050 stage=dld status=approved derives_from=HLD-011 -->
### DLD-1050 — Policy engine

**File.** `internal/policy/policy.go`

**Behaviour.** `Evaluate(a)` proceeds in this order and returns on the first decision:

1. **Categorical denial** — tool in `{shell, exec, sql, delete_resource, bulk_restart}`
   or arguments containing a shell metacharacter: denied, `Rule = "forbidden_category"`.
   Checked before any allowlist so no configuration can enable it.
2. **Service allowlist** — target service not listed: denied.
3. **Tool allowlist** — tool not listed: denied.
4. **Key allowlist** — for `set_config`, key not listed: denied.
5. **Value range** — for `set_config`, numeric value outside the declared range for that
   key: denied.
6. **Risk gate** — allowed; `RequiresApproval` is true when `Risk != low`.

**Invariants.** A denial names the rule that produced it. No parameter bypasses steps 1
to 5.

**Checkpoint.** TC-0041, TC-0042, TC-0044.

<!-- sdd:item id=DLD-1051 stage=dld status=approved derives_from=HLD-011 -->
### DLD-1051 — Executor

**File.** `internal/policy/executor.go`

**Behaviour.**

1. Refuse when the action has no recorded approval and its risk is not `low`.
2. Recompute `Action.Fingerprint()` and compare with the fingerprint recorded at
   approval; a mismatch is refused as `tampered_after_approval`.
3. Re-run `Evaluate` against the action as it stands; refuse if not allowed.
4. Invoke the actuator, timing the call.
5. Return an `ExecutionResult`; a refusal returns an `action_refused` outcome rather than
   an error, so it lands in the log and the report.

**Invariants.** The executor holds the only `Actuator` reference in the process.

**Checkpoint.** TC-0043, TC-0045.

## 9. Orchestration

<!-- sdd:item id=DLD-1060 stage=dld status=approved derives_from=HLD-012 -->
### DLD-1060 — State machine

**File.** `internal/orchestrator/machine.go`

**States.** `created`, `triaging`, `collecting`, `hypothesising`, `criticising`,
`human_review`, `remediating`, `awaiting_approval`, `executing`, `verifying`,
`reporting`, `closed`.

**Transitions.**

```
created        -> triaging
triaging       -> collecting
collecting     -> hypothesising
hypothesising  -> criticising
criticising    -> collecting        (demands raised this round are unsatisfied
                                     and round < maxRounds)
criticising    -> human_review      (close call and round == maxRounds)
criticising    -> remediating       (acceptance condition met)
criticising    -> reporting         (no acceptable hypothesis and budget exhausted)
human_review   -> collecting | remediating | reporting
remediating    -> awaiting_approval (action requires approval)
remediating    -> executing         (low risk and policy allows)
remediating    -> reporting         (no proposable action)
awaiting_approval -> executing      (approved)
awaiting_approval -> reporting      (rejected)
executing      -> verifying
verifying      -> collecting        (not recovered and round < maxRounds)
verifying      -> reporting         (recovered or budget exhausted)
reporting      -> closed
```

**Behaviour.** `Legal(from, to) bool` consults a table; an illegal transition is refused
and recorded as `contribution_rejected` with the attempted pair.

**Checkpoint.** TC-0060.

<!-- sdd:item id=DLD-1061 stage=dld status=approved derives_from=HLD-012 -->
### DLD-1061 — Contribution application

**File.** `internal/orchestrator/apply.go`

**Behaviour.**

1. Sort incoming contributions by `(roleOrder, agentLocalIndex)`; `roleOrder` is the
   fixed collector order `metrics, logs, change, topology, knowledge`.
2. For each: check `Role.MayEmit`, then `Contribution.Validate`. A failure emits
   `contribution_rejected` and the contribution is discarded.
3. Assign an identifier from the per-role sequence counter.
4. For evidence carrying `Facts["demand"]`, mark the matching demand satisfied and emit
   `demand_satisfied`.
4a. A demand keeps the identity *and the round* of its first appearance: re-raising a
   descriptor reuses the existing identifier and preserves `Round`, so repetition cannot
   make a demand look newly asked (DLD-1063).
5. Reject a hypothesis whose `Supporting` is empty or references an unknown identifier,
   emitting `hypothesis_rejected` (REQ-0021).
6. Append the resulting events through the store and publish them to the broker.

**Invariants.** Completion order of concurrent collectors cannot affect the resulting
identifiers or their order.

**Checkpoint.** TC-0003, TC-0021, TC-0061.

<!-- sdd:item id=DLD-1062 stage=dld status=approved derives_from=HLD-012 -->
### DLD-1062 — Progression guards

**File.** `internal/orchestrator/guard.go`

**Behaviour.** `CanRemediate(c) (bool, string)` requires all of:

1. A leading hypothesis exists and its verdict is `accept` or `accept_with_risk`.
2. `Breakdown.Total >= acceptThreshold`.
3. `len(SupportingKinds) >= minEvidenceKinds`.
4. When any change evidence exists in the case, the leading hypothesis's supporting set
   includes change evidence.
5. The top-two gap is at least `closeCallMargin`, or the round budget is exhausted and
   the case has passed through `human_review`.

The reason string names the first unmet condition and is recorded on the transition.

**Checkpoint.** TC-0021, TC-0035, TC-0036.

<!-- sdd:item id=DLD-1063 stage=dld status=approved derives_from=HLD-012 -->
### DLD-1063 — Engine driver

**File.** `internal/orchestrator/engine.go`

**Behaviour.** `Advance` performs exactly one state step and returns. `Run` calls
`Advance` until the state is terminal or `awaiting_approval`, with a hard iteration cap
of `maxRounds * 12` steps as a defence against a transition-table mistake. Collectors run
concurrently through an `errgroup`-equivalent built on `sync.WaitGroup` and a slice
indexed by agent, so results land in a fixed order regardless of completion.

**Errors.** An agent error emits `agent_failed` and the round continues with the
remaining agents; the case is not aborted.

**Round accounting.** `afterCritique` returns to collection only for demands *raised in
the current round*. A demand raised earlier and still unsatisfied has already had a
collection round aimed at it and came back empty; counting it again spends the whole
budget re-asking a question no source in this case can answer. Every unsatisfied demand
is still reported, with a reason distinguishing "round budget exhausted" from "no source
in this case could answer it" — the demand is never silently dropped (REQ-0031). The
demand carries the round it was first raised in so re-raising cannot reset that record.

**Checkpoint.** TC-0060, TC-0061, TC-0073, TC-0111.

<!-- sdd:item id=DLD-1064 stage=dld status=approved derives_from=HLD-013 -->
### DLD-1064 — Event broker

**File.** `internal/eventbus/broker.go`

**Behaviour.** `Subscribe(caseID, buffer)` registers a channel and returns it with a
cancel function. `Publish` performs a non-blocking send per subscriber; on a full buffer
it closes and unregisters that subscriber. Subscribers are stored in a map keyed by a
monotonically increasing subscription identifier so iteration order is stable.

**Invariants.** `Publish` never blocks and never panics on a closed subscriber.

**Checkpoint.** TC-0064.

## 10. Reporting, API and console

<!-- sdd:item id=DLD-1070 stage=dld status=approved derives_from=HLD-016 -->
### DLD-1070 — Report rendering

**File.** `internal/report/report.go`

**Behaviour.** `Markdown(c)` emits, in fixed order: summary, impact, timeline, accepted
root cause with its evidence chain, score breakdown table, rejected alternatives with
reasons, critiques and their resolution, actions with approval and execution status,
verification results, unmet demands, and a multi-agent value note stating the round count
and the number of hypotheses the critic corrected. Absent sections render as
`_none recorded_` rather than being omitted.

**Invariants.** Rendering is a pure function of the projection; no clock, no map
iteration without sorting.

**Checkpoint.** TC-0050, TC-0053.

<!-- sdd:item id=DLD-1071 stage=dld status=approved derives_from=HLD-014 -->
### DLD-1071 — HTTP API

**File.** `internal/httpapi/api.go`

**Behaviour.** Routing uses `http.ServeMux` with Go 1.22 method-and-wildcard patterns.
Handlers decode with `DisallowUnknownFields`, validate required fields, and return
`{"error": "...", "field": "..."}` with `400` on failure. `POST /api/cases` creates and
runs the case to its first halt, then returns the projection. Collections are sorted by
identifier before encoding.

**Checkpoint.** TC-0063.

<!-- sdd:item id=DLD-1072 stage=dld status=approved derives_from=HLD-014 -->
### DLD-1072 — Event stream

**File.** `internal/httpapi/stream.go`

**Behaviour.** `GET /api/cases/{id}/stream` sets `text/event-stream`, replays stored
events from `Last-Event-ID + 1` (or from the `from` query parameter), then forwards live
events from a subscription, writing `id:` and `data:` frames and flushing after each. It
returns when the client disconnects.

**Invariants.** A client sees every event exactly once, in sequence order, across a
reconnect.

**Checkpoint.** TC-0064.

<!-- sdd:item id=DLD-1073 stage=dld status=approved derives_from=HLD-015 -->
### DLD-1073 — Console

**File.** `internal/httpapi/console.go`, `web/*.html`, `web/*.css`, `web/*.js`

**Behaviour.** Templates parsed once from `embed.FS`. `GET /` lists cases; `GET
/cases/{id}` renders the workbench. The live region subscribes to the stream and appends
timeline rows; the approval control posts the decision and reloads. No framework, no
build step.

**Checkpoint.** TC-0065.

<!-- sdd:item id=DLD-1074 stage=dld status=approved derives_from=HLD-017 -->
### DLD-1074 — Evaluation runner

**File.** `internal/eval/eval.go`

**Behaviour.** For each case and each mode, build a fresh engine over a memory store and
a fixture signal set, run to completion (auto-approving actions in evaluation mode so the
flow completes), and record: whether the top hypothesis's signature equals the case's
`expectedSignature`, whether it appears in the top three, the distinct evidence kinds
collected, the number of hypotheses whose score or status the critic changed, and the
number of actions blocked pending approval. `Summarise` aggregates per mode and always
emits the sample size alongside each rate.

**Reasoner attribution.** `Options{Reasoner string; NewReasoner func(*catalog.Catalog)
reasoner.Reasoner}` selects the strategy; a zero `NewReasoner` means the rule adapter and
the name defaults to `"rule"`. The name is carried on every `CaseOutcome` and every
`ModeSummary`, and `Summarise` groups by `(mode, reasoner)` rather than by mode alone —
otherwise a run that mixed strategies would average them into a number describing neither
(REQ-0106).

**Checkpoint.** TC-0080, TC-0081, TC-0114.

<!-- sdd:item id=DLD-1075 stage=dld status=approved derives_from=HLD-018 -->
### DLD-1075 — Entry points

**File.** `cmd/arena/main.go`, `cmd/evalctl/main.go`, `cmd/faultctl/main.go`

**Behaviour.** `arena serve --addr --profile --store --data` starts the HTTP server;
`arena demo --case C1` runs a case to completion and prints the report; `evalctl run
--out` writes the comparison report; `faultctl inject|restore|status --case` drives the
demo stack's fault injection. Configuration precedence is flag, then environment
variable, then default.

**Reasoner selection is shared, not per-command.** `--reasoner rule|model`,
`--model-endpoint` and `--model-name` are parsed by one helper registered on `arena
serve`, `arena demo` and `evalctl run` alike, reading `ARENA_REASONER`,
`ARENA_MODEL_ENDPOINT`, `ARENA_MODEL_NAME` and `ARENA_MODEL_API_KEY` (REQ-0104). A command
that could not select the strategy could not produce a comparable result, and the entry
point that *can* select it — the server — is the one that produces no comparison at all.
The API key is read only from the environment: a credential on a command line lands in the
shell history and in `ps` output.

`evalctl run --reasoner rule,model` runs the whole matrix once per named adapter, so a
single invocation produces the comparison rather than requiring two runs and a diff.

**Checkpoint.** TC-0070, TC-0081, TC-0112.

## 11. The reference scenario, arithmetic

This is the expected behaviour of case `C1`, and the numbers the tests assert.

**Round 1 — default queries only.** Collected: `http_5xx_rate` (0.002 → 0.18, onset
10:07), `http_request_duration_p99` (180ms → 2200ms, onset 10:07), `http_requests_total`
(120 → 260 rps, onset 10:03), `db_up` (baseline 1, collapsed to 0 at 10:07, recovering at
10:09), the log cluster `db connection timeout after #ms` (184 lines), topology naming
`order-api` as candidate origin, and a runbook match. The change collector's default pass
covers the incident window only, so the 10:05:30 deployment is *not* found.

| Hypothesis | metric | log | change | topo | hist | verif | total |
|---|---|---|---|---|---|---|---|
| `sig-db-outage` | 0.30 | 0.25 | 0.00 | 0.10 | 0.00 | 0.00 | **0.65** |
| `sig-traffic-surge` | 0.30 | 0.00 | 0.00 | 0.10 | 0.04 | 0.05 | **0.49** |
| `sig-db-pool-exhaustion` | 0.00 | 0.25 | 0.00 | 0.10 | 0.00 | 0.05 | **0.40** |

The leading hypothesis is **wrong**, and the shape of its wrongness is the point of the
scenario. It is not leading on a gap: `sig-db-outage` requires a collapsed availability
signal and database connection errors in the log, and round one has both. Every
requirement it declares is satisfied by evidence in hand, its metric and log terms are
1.0, and it clears the runner-up by 0.16 — outside `closeCallMargin`, so this is a
confident answer rather than a coin toss. It is the answer a careful reasoner gives on
this evidence, and it is still wrong.

The leader remains below `acceptThreshold`. `alternative_explanation`, `coverage_gap` and
`unverifiable_remediation` fire, producing demands for the pool saturation metric, the
configuration change history over `changeLookback`, the error-rate duration compared with
the database recovery time, and a historical peak traffic comparison at equal load. The
database availability metric is *not* demanded: `sig-db-outage`'s own matched requirement
already supplies it, and demanding evidence the case holds would spend a round returning
what round one read.

**Round 2 — demand-driven queries.** `db_pool_saturation` returns saturated (capacity
1.0, peak 1.0, onset 10:06); the change query over the extended window returns
`DB_POOL_SIZE 20 → 2` at 10:05:30; the traffic comparison returns an equal-load period
two days earlier with no errors, carrying `Facts["counters"] = "sig-traffic-surge"`; and
the duration comparison reports availability restored at 10:09:00 against errors
continuing to 10:27:00, carrying `Facts["counters"] = "sig-db-outage"`.

| Hypothesis | metric | log | change | topo | hist | verif | penalty | total |
|---|---|---|---|---|---|---|---|---|
| `sig-db-pool-exhaustion` | 0.30 | 0.25 | 0.20 | 0.10 | 0.04 | 0.05 | 0.00 | **0.94** |
| `sig-db-outage` | 0.30 | 0.25 | 0.00 | 0.10 | 0.02 | 0.00 | 0.20 | **0.47** |
| `sig-traffic-surge` | 0.30 | 0.00 | 0.00 | 0.10 | 0.04 | 0.05 | 0.20 | **0.29** |

The ranking has changed and the top-1 is now correct. Note what did *not* happen to
`sig-db-outage`: its metric and log terms are unchanged, because the outage was real and
the evidence for it still stands. What removed it from first place is a counter-penalty —
it is refuted, not out-measured. Temporal order holds (10:05:30 < 10:06), every demand is
satisfied, coverage is four kinds, and the total clears the threshold — so the verdict is
`accept` and the case proceeds to remediation with `set_config order-api DB_POOL_SIZE
20`, risk `medium`, rollback to `2`, verifying `http_5xx_rate` and
`http_request_duration_p99`.

**What this demonstrates.** The single-agent and no-critic modes stop at round 1 and
report the database outage — the first coherent story, fully evidenced. Only the
adversarial flow asks how long the outage lasted and reaches the configuration change.
This is the comparison the evaluation computes, and it is a property of the flow rather
than of the data, because all three modes see the same fixture. It is also deliberately
harder than the version it replaces: a round-one error that merely lacked evidence would
let the critic win by filling a gap, which says less about adversarial review than
overturning an answer that had everything it asked for.

## 12. Implementation order

| Wave | DLD items | Rationale |
|---|---|---|
| 1 | DLD-1001..1007 | Domain types; everything else compiles against them |
| 2 | DLD-1010, DLD-1011, DLD-1064 | Store and broker; no dependencies beyond domain |
| 3 | DLD-1020..1024, DLD-1030, DLD-1031 | Signal plane and catalog; enables fixtures |
| 4 | DLD-1032..1034 | Reasoning; testable against fixture evidence alone |
| 5 | DLD-1040..1042, DLD-1050, DLD-1051 | Agents and policy |
| 6 | DLD-1060..1063 | Orchestration; first end-to-end run |
| 7 | DLD-1070..1075 | Report, API, console, evaluation, commands |
