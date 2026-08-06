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
| 8 | REQ-0095 (release publishing) | `go test ./internal/httpapi/ && sddctl gate --stage deliver` | pass | TC-0092 |
| 9 | REQ-0096..0098 (M4 live adapters) | `go test -race ./... && sddctl gate --stage deliver` | pass | TC-0100, TC-0101, TC-0102, TC-0103 |
| 10 | REQ-0099 (overfitting case C4) | `go test ./... && go run ./cmd/evalctl run` | pass | TC-0104 |
| 11 | REQ-0100 (model reasoner) | `go test -race ./... && sddctl gate --stage deliver` | pass | TC-0105, TC-0106 |

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
| D9 | TC-0092 | The release pipeline's ordering assertion matched the file's own header comment, which mentions `docker push` — so it read a comment as a publishing step and failed a correct workflow | The assertion strips whole-line comments first: prose describing a pipeline cannot publish anything. Confirmed live by moving the publish step above the gates, which fails the test, and restoring it, which passes |
| D10 | `TestPlaneDependencies` | The three new adapter packages were not in the dependency table, so nothing constrained what they could import | They were assigned ranks: the live adapters sit beside the fixture adapter as alternatives to it, and profile selection ranks above all adapters and below every port consumer |
| D11 | Manual CLI check | `arena serve --signal-profile prod` started cleanly. An unknown profile was only rejected when the first case was created, so a mistyped deployment looked healthy and then failed one case at a time, once someone was relying on it | `profile.Config.Validate()` was added and is called at start-up; the entry point exits 2 naming the offending flag |
| D12 | Review of TC-0102 | The "adversarial payload" in the log test contained header-like bytes but no newline, so a naive line-oriented parser would have passed it too — the test asserted nothing the framed parser uniquely provides | The payload became a genuine multi-line record whose continuation begins with header bytes, and the assertion now requires both halves in one record |
| D13 | Building C4 | `sig-traffic-surge` scored 1.00 on the only term it claims and was still capped near 0.55, because terms it never required were charged as zeros. Any explanation requiring one evidence kind is therefore unacceptable at the 0.75 threshold no matter how strong its support | **Not fixed — see the known issue below.** The obvious fix makes things worse, and shipping the wrong fix would have been worse than shipping the defect |
| D14 | `sddctl lint` | The new detailed-design item reused `DLD-1034`, which the critique-rules item already held. The tool refused the tree rather than letting two designs answer to one identifier | Renumbered to `DLD-1035`; the source anchor followed |
| D15 | `sddctl gate --stage architect` | REQ-0100 had no architecture item deriving from it — the same class of gap as D7, caught the same way | ARC-005, which owns the reasoner port, was extended to claim it |

## Known issues

**Scoring charges an explanation for evidence it never claimed (D13).** A signature's
score sums six weighted terms, but three of them — metric, log and change alignment —
only apply to a signature that requires that kind of evidence. `sig-traffic-surge`
requires one metric. With that metric perfectly matched it reaches 0.47, and its
theoretical maximum is 0.55, so it can never cross the 0.75 acceptance threshold. It can
be *ranked* first, and in C4 it is; it can never be *acted on*.

The obvious fix is to renormalise over the terms that apply. It was implemented and
measured, and it is wrong:

| Case | Leader before | Leader after | Effect |
|---|---|---|---|
| C1 round 1 | `sig-traffic-surge` 0.49 | `sig-traffic-surge` 0.89 | above threshold, gap 0.45 — the critic never fires |
| C1 final | `sig-db-pool-exhaustion` 0.94 | 0.94 | correct, but reached without the adversarial round |

Renormalising makes a one-requirement explanation trivially near-certain: matching its
single requirement is, by construction, matching everything it asked for. The reference
scenario then accepts the wrong answer in round one with high confidence, which destroys
the demonstration the entire project rests on. Four tests caught this — the close-call
rule, the round-count assertions in both mode comparisons, and the escalation test — and
the change was reverted rather than the tests adjusted to accommodate it.

The real defect is in the catalog, not the arithmetic: "traffic rose" is not evidence
that traffic *caused* the outage. The explanation should require the historical
comparison showing the load exceeded what was previously served — the very evidence C1
uses to refute it and C4 uses to confirm it. That is a modelling change to
`signatures.json` with its own cascade, and it is deferred rather than rushed alongside a
new fault case.

**Consequence today.** C4 ranks the correct cause first and refutes both rivals with
counter-evidence, which is what TC-0104 asserts and what the overfitting test needs. It
stops below the acceptance threshold rather than proposing its declared remediation. The
case declares `expected_remediation` because that is the correct action; the system does
not currently reach it.

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
| Container image builds on linux/amd64 | `docker build -f deploy/docker/Dockerfile` (CI, run 5) | pass |
| The image starts and serves | health check + `/api/version` (CI, run 5) | pass — `healthy after 1s`, version `0.1.0-mvp`, cases C1–C3 |

The last two rows were previously recorded here as *not executed*, because the
development environment has no Docker daemon. They have now run on CI against commit
`13209ba`, so the entry moved out of "what was not done" rather than being left to imply
a verification that had not happened.

## What was not done, and why

Recorded here rather than implied by absence.

| Item | Status | Reason |
|---|---|---|
| A configured model provider | Adapter implemented and tested; no provider configured | Open question Q1 in the charter remains open: choosing a vendor is not this loop's decision to make. `arena serve --reasoner model --model-endpoint ... --model-name ...` works against any chat-completions API; there is deliberately no default endpoint |
| Fault cases C5–C6 | Not written | Milestone M5. C4, the misleading-log case, is written and is the one that mattered: it is what distinguishes a discriminating critic from a biased one. C5 (victim versus source) and C6 remain |
| Multi-tenant authentication | Not implemented | Charter non-goal N6 |

## Escalations

One was raised and has since been resolved. It is recorded rather than deleted, because
CON-010 asks for what the loop did, not only for where it ended up.

**Raised: pushing to the remote was blocked by an organisation policy.** `git push`
returned `403`, and the API stated the reason directly — *GitHub access is not enabled
for this session. An org admin must connect the Claude GitHub App for this
organization.* Read access worked throughout (`git ls-remote` and the API's read
endpoints both succeeded), and the agent proxy reported no relay failures, so this was
an authorisation boundary rather than a network or credential fault. All three write
paths were exercised and each was refused:

| Path | Result while blocked |
|---|---|
| `git push` over HTTPS | `403` |
| `POST /repos/.../git/refs` with the session token | `403 GitHub access is not enabled for this session` |
| The GitHub App integration | `403 Resource not accessible by integration` |

Because the refusal was issued at the authorisation layer, retrying it and routing
around it were both wrong: the loop stopped, wrote the blocker into this log so it would
travel with the artifact rather than live only in a conversation, and named the single
action that would clear it.

**Resolved: write access was granted and the branch pushed.** `git push -u origin
claude/project-spec-architecture-78dmtj` now succeeds. Local and remote report
`0 0` for ahead/behind, so the remote carries every commit, in order, with its message —
no history was collapsed and nothing was re-transmitted file by file.

**Resolved: the release was cut, by the route the block forced us to build.** Pushing a
*tag* returned `403` where branch pushes succeeded, so the write grant was branch-scoped.
Every other route was refused too:

| Path | Result |
|---|---|
| `git push origin v0.1.0` | `403` (surfaced as a sideband disconnect; `--verbose` shows the real status) |
| `POST /repos/.../git/tags` with the session token | `403 Write access to this GitHub API path is not permitted through this proxy` |
| The GitHub App integration (`workflow dispatch`) | `403 Resource not accessible by integration` |

Because the blocker was specifically the *tag*, the workflow gained a manual entry point
rather than being left dependent on the one permission that was missing. A maintainer
then merged the branch and ran that entry point: workflow run `31041990777` completed all
eighteen steps, publishing `ghcr.io/zlrrr/mutil-agent-system:0.1.0` and release `v0.1.0`
with its binaries, a loadable image tarball and `SHA256SUMS`.

The lesson is worth keeping rather than deleting with the blocker: the escape hatch built
under the constraint is now the ordinary way to cut a release from a browser.

This and the push block above are the only points in the milestone where the autonomous
loop needed authority it did not have (CON-010). No other decision required it: every
ambiguity was resolvable from the charter, the requirements or the constitution. The
three open questions the charter records (Q1–Q3) did not block any M1 work, and each
proceeded under the default the charter states.
