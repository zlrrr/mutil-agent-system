---
id: TASKS-001
lang: en
counterpart: tasks.zh.md
doc_version: 1.0.0
status: approved
stage: plan
---

# IncidentOps Arena — Task Breakdown

## 1. Rules

- A task is complete only when its checkpoint tests pass and `go vet ./...` is clean.
- A task carries an `// sdd:impl` anchor for every DLD item it realises.
- Tasks marked parallelisable touch disjoint files and have no ordering edge.
- A task that cannot state its checkpoint is not decomposed enough.

## 2. Tasks

<!-- sdd:item id=T-001 stage=plan status=approved derives_from=DLD-1001,DLD-1002,DLD-1003,DLD-1004,DLD-1005 -->
### T-001 — Domain entities

**Implements.** DLD-1001, DLD-1002, DLD-1003, DLD-1004, DLD-1005

**Files.** `internal/domain/{ids,evidence,hypothesis,critique,action}.go`

**Definition of done.** Anchors present; TC-0002 passes; validation errors are typed and
distinguishable.

**Blocked by.** none. **Parallelisable.** no — everything compiles against this.

<!-- sdd:item id=T-002 stage=plan status=approved derives_from=DLD-1006,DLD-1007 -->
### T-002 — Contribution algebra, events and projection

**Implements.** DLD-1006, DLD-1007

**Files.** `internal/domain/{contribution,event,case}.go`

**Definition of done.** Role capability table complete; `Replay` folds from empty;
TC-0050 and the domain half of TC-0003 pass; `Leading()` skips only rejected
explanations, which TC-0110 exercises end to end.

**Blocked by.** T-001. **Parallelisable.** no.

<!-- sdd:item id=T-003 stage=plan status=approved derives_from=DLD-1010,DLD-1011,DLD-1064 -->
### T-003 — Store adapters and event broker

**Implements.** DLD-1010, DLD-1011, DLD-1064

**Files.** `internal/store/{store,memory,file}.go`, `internal/eventbus/broker.go`

**Definition of done.** TC-0004, TC-0005 pass; the broker never blocks under a full
buffer; `-race` clean.

**Blocked by.** T-002. **Parallelisable.** yes, with T-004.

<!-- sdd:item id=T-004 stage=plan status=approved derives_from=DLD-1020,DLD-1021,DLD-1022 -->
### T-004 — Signal ports, bounds, anomaly detection and clustering

**Implements.** DLD-1020, DLD-1021, DLD-1022

**Files.** `internal/signal/{ports,bounds,anomaly,cluster}.go`

**Definition of done.** TC-0011, TC-0012, TC-0016 pass; no adapter returns an unbounded
result.

**Blocked by.** T-002. **Parallelisable.** yes, with T-003.

<!-- sdd:item id=T-005 stage=plan status=approved derives_from=DLD-1030,DLD-1031 -->
### T-005 — Embedded catalog, signatures and the fault case library

**Implements.** DLD-1030, DLD-1031

**Files.** `internal/catalog/{catalog,signature,case}.go`, `internal/catalog/data/*.json`

**Definition of done.** TC-0082 passes; the catalog contains at least signatures for
connection-pool exhaustion, traffic surge, database outage, misconfigured dependency and
slow query, and at least three fault cases.

**Blocked by.** T-004. **Parallelisable.** no.

<!-- sdd:item id=T-006 stage=plan status=approved derives_from=DLD-1023,DLD-1024 -->
### T-006 — Fixture adapters and simulated actuator

**Implements.** DLD-1023, DLD-1024

**Files.** `internal/signal/fixture/{fixture,actuator}.go`

**Definition of done.** A signal set builds from a fault case; the actuator flips the
metric source to post-recovery series after the recovery trigger.

**Blocked by.** T-005. **Parallelisable.** no.

<!-- sdd:item id=T-007 stage=plan status=approved derives_from=DLD-1032,DLD-1033 -->
### T-007 — Scoring and rule reasoner

**Implements.** DLD-1032, DLD-1033

**Files.** `internal/reasoner/{reasoner,score,rule}.go`

**Definition of done.** TC-0020, TC-0022, TC-0023, TC-0024, TC-0072 pass; the round-1
and round-2 totals of the reference scenario match the DLD arithmetic to two decimals; the
adapter satisfies the shared contract suite (TC-0113) rather than only the interface.

**Blocked by.** T-006. **Parallelisable.** no.

<!-- sdd:item id=T-008 stage=plan status=approved derives_from=DLD-1034 -->
### T-008 — Critique rule ensemble

**Implements.** DLD-1034

**Files.** `internal/reasoner/critique.go`

**Definition of done.** TC-0030 through TC-0034 pass, one test per rule; TC-0110 passes,
so the ensemble displaces a fully supported wrong answer rather than only filling gaps;
the critic emits no hypothesis contribution.

**Blocked by.** T-007. **Parallelisable.** no.

<!-- sdd:item id=T-009 stage=plan status=approved derives_from=DLD-1040,DLD-1041,DLD-1042 -->
### T-009 — Agent roles and the single-agent baseline

**Implements.** DLD-1040, DLD-1041, DLD-1042

**Files.** `internal/agent/{agent,collectors,analysis,critic,remediation,verification,baseline}.go`

**Definition of done.** TC-0001, TC-0013, TC-0014, TC-0015, TC-0040 pass; collectors are
pure and emit only `add_evidence`.

**Blocked by.** T-008. **Parallelisable.** yes, with T-010.

<!-- sdd:item id=T-010 stage=plan status=approved derives_from=DLD-1050,DLD-1051 -->
### T-010 — Policy engine and executor

**Implements.** DLD-1050, DLD-1051

**Files.** `internal/policy/{policy,executor}.go`

**Definition of done.** TC-0041, TC-0043, TC-0044 pass; categorical denials hold with an
approval attached; the executor holds the only actuator reference.

**Blocked by.** T-006. **Parallelisable.** yes, with T-009.

<!-- sdd:item id=T-011 stage=plan status=approved derives_from=DLD-1060,DLD-1061,DLD-1062,DLD-1063 -->
### T-011 — Orchestrator

**Implements.** DLD-1060, DLD-1061, DLD-1062, DLD-1063

**Files.** `internal/orchestrator/{machine,apply,guard,engine}.go`

**Definition of done.** TC-0003, TC-0010, TC-0021, TC-0031, TC-0035, TC-0036, TC-0042,
TC-0045, TC-0046, TC-0051, TC-0052, TC-0060, TC-0061, TC-0062, TC-0070, TC-0071, TC-0073,
TC-0111 pass; first end-to-end run of the reference scenario.

**Blocked by.** T-009, T-010. **Parallelisable.** no.

<!-- sdd:item id=T-012 stage=plan status=approved derives_from=DLD-1070,DLD-1071,DLD-1072,DLD-1073 -->
### T-012 — Report, API, stream and console

**Implements.** DLD-1070, DLD-1071, DLD-1072, DLD-1073

**Files.** `internal/report/report.go`, `internal/httpapi/{api,stream,console}.go`,
`web/*`

**Definition of done.** TC-0053, TC-0063, TC-0064, TC-0065 pass; the report renders
identically from live and replayed projections.

**Blocked by.** T-011. **Parallelisable.** yes, with T-013.

<!-- sdd:item id=T-013 stage=plan status=approved derives_from=DLD-1074 -->
### T-013 — Evaluation runner

**Implements.** DLD-1074

**Files.** `internal/eval/eval.go`

**Definition of done.** TC-0080, TC-0081, TC-0114 pass; the summary reports sample size
beside every rate, and names the reasoner beside the mode.

**Blocked by.** T-011. **Parallelisable.** yes, with T-012.

<!-- sdd:item id=T-014 stage=plan status=approved derives_from=DLD-1075 -->
### T-014 — Entry points, container and demo stack

**Implements.** DLD-1075

**Files.** `cmd/arena/main.go`, `cmd/evalctl/main.go`, `cmd/faultctl/main.go`,
`deploy/docker/*`, `Makefile`, `.github/workflows/ci.yml`

**Definition of done.** TC-0090 and TC-0112 pass; `arena demo --case C1` prints the
report; every command that produces a comparable result accepts the same reasoner
selection; `make test` and `make sdd-validate` are green.

**Blocked by.** T-012, T-013. **Parallelisable.** no.

<!-- sdd:item id=T-015 stage=plan status=approved derives_from=DLD-1074,DLD-1075 -->
### T-015 — Reasoner selection and the shared contract

**Implements.** DLD-1074, DLD-1075

**Files.** `cmd/arena/main.go`, `cmd/evalctl/main.go`, `internal/eval/eval.go`,
`internal/reasoner/contract_test.go`

**Definition of done.** TC-0112, TC-0113 and TC-0114 pass; the reasoner flags are
registered by one helper shared across `arena serve`, `arena demo` and `evalctl run`; the
evaluation groups by `(mode, reasoner)`; the contract suite runs against both adapters and
fails if either stops satisfying it.

**Blocked by.** T-014. **Parallelisable.** no.

## 3. Execution order

| Wave | Tasks | Gate before the next wave |
|---|---|---|
| 1 | T-001, T-002 | `go build ./...` clean; TC-0002, TC-0050 pass |
| 2 | T-003, T-004 | TC-0004, TC-0005, TC-0011, TC-0012, TC-0016 pass |
| 3 | T-005, T-006 | TC-0082 passes; a fixture signal set builds from case `C1` |
| 4 | T-007, T-008 | Reference-scenario scores match the DLD arithmetic; TC-0020..TC-0034 pass |
| 5 | T-009, T-010 | TC-0001, TC-0040, TC-0041, TC-0043, TC-0044 pass |
| 6 | T-011 | Reference scenario runs end to end; the determinism test passes |
| 7 | T-012, T-013, T-014 | Full suite green under `-race`; `sddctl gate --stage deliver` passes |
| 8 | T-015 | Both reasoner adapters pass one contract; the evaluation reports per reasoner |

## 4. Progress log

Recorded per wave in `docs/checkpoints.en.md` and its Chinese counterpart, with the
command executed, the result and the date. A wave is not closed until its gate row is
written.
