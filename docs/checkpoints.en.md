---
id: CHECKPOINTS
lang: en
counterpart: checkpoints.zh.md
doc_version: 1.0.0
status: approved
stage: constitution
---

# Checkpoint log

CON-005 requires that no design claim ships as "done" without a named checkpoint, a test
case identifier, and a recorded run of that test. CON-010 requires that an autonomous
loop record what it did and what it left undone. This is that record.

## Wave gates

Each wave closed only when its gate command exited zero.

| Wave | Tasks | Gate command | Result | Notes |
|---|---|---|---|---|
| 0 | SDD framework | `go run ./cmd/sddctl validate` | pass | 65 items, 40 anchors, no findings. The engine caught its own bootstrap violation first — `sddctl` source existed before its DLD items, reported as 9 `orphanCodeAnchor` errors — which was resolved by writing `specs/000-sdd-framework` |
| 1 | T-001, T-002 | `go build ./... && go test ./internal/domain/` | pass | TC-0002, TC-0003, TC-0050, TC-0070, TC-0074 |
| 2 | T-003, T-004 | `go test -race ./internal/store/ ./internal/eventbus/ ./internal/signal/` | pass | TC-0004, TC-0005, TC-0011, TC-0012, TC-0016, TC-0064 |
| 3 | T-005, T-006 | `go test ./internal/catalog/` | pass | TC-0082; 5 signatures, 3 fault cases, 5 runbooks |
| 4 | T-007, T-008 | `go test ./internal/reasoner/` | pass | TC-0020, TC-0022, TC-0023, TC-0024, TC-0030 through TC-0036, TC-0072 |
| 5 | T-009, T-010 | `go test ./internal/agent/ ./internal/policy/` | pass | TC-0001, TC-0013, TC-0014, TC-0015, TC-0040, TC-0041, TC-0043, TC-0044 |
| 6 | T-011 | `go test -race ./internal/orchestrator/` | pass | TC-0010, TC-0021, TC-0031, TC-0035, TC-0042, TC-0045, TC-0046, TC-0051, TC-0060, TC-0061, TC-0062, TC-0070, TC-0071, TC-0073 |
| 7 | T-012, T-013, T-014 | `make check` | pass | TC-0053, TC-0063, TC-0064, TC-0065, TC-0080, TC-0081, TC-0090, TC-0091 |

## Defects found by the checkpoints

These are the problems the gates caught, and what changed as a result. They are recorded
because a checkpoint that never fails is not a checkpoint.

| # | Found by | Problem | Resolution |
|---|---|---|---|
| D1 | `sddctl trace` at wave 0 | The governance tool's own source existed before the design items it implements | Wrote `specs/000-sdd-framework` covering DLD-0101..0109 before proceeding |
| D2 | First end-to-end run | The change collector's default pass used the analysis window, which starts seven minutes before the alert — so it found the configuration change immediately and the critic's demand was pointless | The default pass now queries from the alert onward; the extended lookback is applied only when a demand asks for it (DLD-1040) |
| D3 | First end-to-end run | `change_correlation` compared the change against the *earliest* anomaly anywhere in the case. The traffic series moved first, so a genuine cause scored zero | Comparison is now against the onset of the hypothesis's own supporting metrics (`HypothesisOnset`, DLD-1032) |
| D4 | First end-to-end run | Every verdict came back empty: `CombineVerdicts` treated an unset verdict as maximally severe, so it outranked every real judgement | Unset verdicts are skipped; "not yet judged" no longer outranks a decision |
| D5 | First end-to-end run | A round-1 `revise` verdict persisted into round 2, so evidence that answered a challenge could never clear it | The verdict is no longer carried across rounds; the critic re-examines every hypothesis against the round's evidence (DLD-1033) |
| D6 | TC-0062 | The triage step emitted `agent_completed` with no duration, so the event log's work records were incomplete | Triage now measures itself and names the collectors it plans to fan out to |
| D7 | `sddctl gate --stage architect` | 13 requirements had no architecture item deriving from them — a real coverage gap, not a tooling artefact | ARC-003, ARC-004, ARC-006, ARC-010, ARC-011 and ARC-014 were extended to claim them |
| D8 | TC-0091 | REQ-0091 (no third-party dependencies) had no executed test, because the test verifying it was linked only to the framework's own requirement | TC-9013 now derives from both REQ-9013 and REQ-0091 |

## Cascading updates performed

CON-003 requires that an upstream change be followed down to every descendant. Two
cascades were performed during this milestone.

| Trigger | Impact set | Action |
|---|---|---|
| Architecture coverage gap (D7) | ARC items and their `Realises` prose | Anchors and prose updated in both languages; `sddctl lint` re-run; tree re-sealed |
| Measured scores differed from the DLD's predicted arithmetic | DLD-DOC-001 section 11, both languages | The design's worked example was corrected to the values the implementation produces (round 1: 0.49 / 0.40 / 0.35; round 2: 0.94 / 0.37 / 0.29). The design's *qualitative* claims — the wrong leader, the sub-margin gap, the reordering — held |

The second cascade is worth stating plainly: the detailed design predicted 0.47 / 0.46 /
0.43 and 0.96 / 0.43 / 0.27. The implementation produced different numbers because the
runbook-similarity term resolves differently than the estimate assumed. The design
document was corrected rather than the numbers being quietly left stale, which is what
CON-003 exists to force.

## Final verification

| Check | Command | Result |
|---|---|---|
| Build | `go build ./...` | pass |
| Vet | `go vet ./...` | pass |
| Formatting | `gofmt -l cmd internal` | no output |
| Tests | `go test ./...` | pass |
| Tests under the race detector | `go test -race ./...` | pass |
| No third-party dependencies | `go.mod` has no `require`; no `go.sum` | pass |
| Governance | `go run ./cmd/sddctl validate` | pass, no findings |
| Delivery gate | `go run ./cmd/sddctl gate --stage deliver` | pass |
| Reference scenario | `go run ./cmd/arena demo --case C1` | pass |
| Evaluation | `go run ./cmd/evalctl run` | pass |

## What was not done, and why

Recorded here rather than implied by absence.

| Item | Status | Reason |
|---|---|---|
| Container image build and run | Definition written and asserted by TC-0090; not executed | No Docker daemon is available in the development environment. The Dockerfile, the compose stack and the CI job that builds and health-checks the image are all present; the image job runs on a machine that has a daemon |
| Live Prometheus and container-log adapters | Ports defined, fixture adapters implemented, live adapters not | Milestone M4. The reference scenario and the whole test suite deliberately do not depend on them (CON-007) |
| Model-backed reasoner | Port defined and documented; no adapter wired | Open question Q1 in the charter: no provider has been chosen. The deterministic adapter is the default by design, not by omission (ADR-002) |
| Fault cases C4–C6 | Not written | Milestone M5. Three cases are enough for M1's gate; the misleading-log case in particular exists to test overfitting, which is a fair test only once the catalog has stopped growing alongside it |
| Multi-tenant authentication | Not implemented | Charter non-goal N6 |

## Escalations

None. No decision during this milestone required authority outside the specification:
every ambiguity was resolvable from the charter, the requirements or the constitution.
The three open questions the charter records (Q1–Q3) did not block any M1 work, and each
proceeded under the default the charter states.
