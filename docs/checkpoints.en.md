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
| 12 | REQ-0101 (victim case C5) | `go test -race ./... && go run ./cmd/evalctl run` | pass | TC-0107 |
| 13 | REQ-0102 (post-onset change, C6) | `go test -race ./... && sddctl gate --stage deliver` | pass | TC-0108 |
| 14 | REQ-0011 (collapse detection) | `go test -race ./... && sddctl gate --stage deliver` | pass | TC-0109 |

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
| D16 | Building C5 | `source_vs_victim` had never fired in any end-to-end run: every earlier case alerts on the service that is also the origin. It was unit-tested and otherwise dormant — a rule nobody had watched work | C5 alerts on a victim. Running it exposed D17 and D18 below |
| D17 | C5, first run | The rule challenged and demanded nothing, so the case closed after one round on a `revise` verdict having never looked upstream. A challenge no evidence can answer is a veto, not a critique | The rule now demands every unanswered requirement descriptor in the catalog, which is what lets an upstream explanation form |
| D18 | C5, second run | The rule also challenged the explanation that correctly blamed the upstream, using the topology evidence that supported it. The first fix attempt read intent from the signature's remediation service — but every signature in this catalog remediates `order-api`, so every explanation looked upstream-aware and the rule stopped firing entirely | The skip is decided from evidence: a demand response records the service it concerns in a `subject` fact, and a hypothesis resting on upstream evidence is not describing a victim |
| D19 | C5, second run | The change and log demand responses filtered by the *alert's* service, so a victim's alert could never retrieve its upstream's history — the assumption that the alerting service is the subject was baked into the fixture, not just the rule | Demand responses may name a `Service`, defaulting to the alert's |

### Found while preparing the C1 redesign

| # | Problem | Resolution |
|---|---|---|
| D24 | The anomaly detector only recognised growth. A metric *collapsing* — an availability gauge to zero, throughput going flat, a queue draining — was invisible to it, and those are among the most diagnostic signals an incident produces | Collapse is detected as the mirror of a rise, with the same sustain requirement so a single dipping sample stays noise, and `Summary()` says the series *fell* rather than misreporting it as a rise |
| D25 | `sig-db-outage` required its availability metric to have "dropped" and expressed that as `saturated`, which is computed from a declared capacity that no availability gauge has. **The requirement was unsatisfiable**: the signature could never be fully matched by anything the collectors produce, and nothing had noticed because no case exercises a database outage | Patterns gained a general `Facts` condition — the form `saturated` should always have had — and the signature now requires `collapsed`. All six cases are unchanged, because none has a collapsing gauge; the signature is simply honest now instead of impossible |

### Defects C6 exposed

| # | Problem | Resolution |
|---|---|---|
| D20 | `temporal_order` was the last critique rule never to have fired end to end. C6 made it fire, and immediately showed the check was too narrow: it compares a blamed change against `HypothesisOnset`, which measures an explanation against its *own* metrics. A pool that was shrunk and then saturated is internally coherent while explaining nothing about an incident that began four minutes earlier, so the objection evaporated as soon as the demanded pool metric arrived | The rule now also rejects an explanation whose own evidence begins more than `coMovementWindow` after the alert: a symptom that postdates the incident is downstream of whatever caused it |
| D21 | The rejected explanation still ranked first and was reported as the answer, in a report that recorded its rejection on the same page | `Leading()` returns the highest-scoring hypothesis the critic has not rejected; the ranking still shows the rejected one first with its verdict beside it. "This fit best and here is why we rejected it" is worth more to a reader than hiding it |
| D22 | Remediation acted on the top-ranked hypothesis, so it proposed changing the very setting the critic had just ruled out | The remediation role uses `Snapshot.Leading()`, and the evaluation's top-1 metric likewise reports the accepted explanation rather than one the system explicitly refused to draw |
| D23 | With two changes in the extended window, the change demand returned only the last — so a deploy landing *after* onset hid the change that preceded it, and the only change capable of causing anything never became evidence | The demand asks for changes before onset, so the response now answers with one: the latest change preceding the alert. The post-onset change still arrives through the default pass, where it belongs |

### Defects the C1 redesign exposed

| # | Problem | Resolution |
|---|---|---|
| D26 | `alternative_explanation` demanded the database availability metric that round one had *already collected*. `demandAlreadyAnswered` only recognises evidence that arrived as the answer to a demand, so evidence gathered by a case's default queries was invisible to it. C1 spent a whole third collection round re-fetching a series the first round had read | A discriminator is also skipped when the rival's own matched requirements already supply it. Whether the critic holds the separating evidence must not depend on how it was obtained. C1 returned to two rounds |
| D27 | The new discriminator was demanded in C4 too, where no source can answer it — and an unanswered demand means the same `revise` critique is raised on the leader every round. C4 never accepted anything: it burned its budget, escalated to human review, and reported the wrong cause. **A rule with a demand nothing can satisfy is a veto with extra steps** — the same shape as D17, arriving through a different door | A rival carrying unresolved counter-evidence is skipped: the evidence has already argued against it, so it is not the unexamined alternative the rule exists to catch. C4 returned to accepting `sig-traffic-surge` |
| D28 | Found while fixing D27: a demand that one collection round failed to answer is counted as a reason to run another round, every round, until the budget is gone. C5 has carried four such demands since it was written. Recorded unfixed in the previous change, then fixed in the next one | Demands carry the round they were first raised in; `afterCritique` returns to collection only for demands raised in the current round, and every rule stops re-issuing a descriptor a previous round already attempted. Unmet demands are still reported, now distinguishing "budget exhausted" from "no source could answer it". C4 back to two rounds, C5 stops repeating three demands per round, mean rounds 2.50 → 2.33, all six cases still correct |
| D29 | Found while writing TC-0111 for D28: the first version of the fix touched only three of the four rules that raise demands. `source_vs_victim` kept re-issuing its three upstream descriptors every round, and the C4-only test passed anyway, because C4's repeat came through a rule the D27 fix had already silenced | The test asserts over C4 *and* C5, since different rules raise their demands and each decides separately whether to re-ask. Mutating the fix back out now fails on C5, which is what the C4-only version could not do |

### Defects the C2 and C3 redesigns exposed

| # | Problem | Resolution |
|---|---|---|
| D30 | `sig-traffic-surge` matched a request-rate series that had *not risen*: the pattern matches the metric's name, while its label asserts "request rate rose sharply". In C2 and C3 traffic is flat, and the signature still won round one — by 0.01 and 0.03 over the correct answer. Their headline results were coin tosses that happened to land wrong | Not fixed by tightening the signature, which is D13 and still open. Fixed where it mattered: C2 and C3 now reach a first answer that is well-supported and wrong for a real reason, so the ranking no longer turns on a signature matching a metric that did not move |
| D31 | `source_vs_victim` demands every unanswered requirement descriptor in the *whole catalog*, so its demand list grows with the catalog while `maxDemandsPerRound` stays at 4. Adding two signatures displaced the evidence C5 needed into later rounds, and the victim case stopped reaching its cause — a case broke because of signatures it has nothing to do with | Demands are ordered by how much of each signature the case already supports: leads first, shots in the dark last. The rule's reach still scales with the catalog, which is worth watching, but the cap now spends itself on the most-supported explanations rather than on alphabetical order |
| D32 | `Leading()` skipped `revise` as well as `reject`, so the reported answer depended on the severity of an *unfinished* objection rather than on the evidence. C5 reported a 0.09 explanation as its root cause while a 0.84 one sat at the top of the ranking, because the weaker one had attracted a milder critique | Only `reject` is skipped — that was D21's actual finding. Whether an explanation may be *acted on* is a separate question, and `CanRemediate` already asks it separately |
| D33 | `alternative_explanation` raised a demand-backed challenge in the *final* round, where no collection round remains to answer it. The leader stayed in `revise` for ever and the case ended unable to act on its own best explanation — a veto delivered by timing rather than by content, the fourth variant of D17 | The rule is silent once the budget is spent. What went unexamined is recorded as an unmet demand, and a genuine near-tie is still escalated by `close_call`, which exists for exactly that |

### M6 — the model reasoner becomes measurable

| # | Problem | Resolution |
|---|---|---|
| D34 | **The deterministic reasoner was adopted without escalation.** `project.md` specifies Python, LangGraph and a system prompt per agent; ADR-002 replaced that with a Go rule engine, justified partly by REQ-0090 (byte-identical output) — a requirement this project wrote for itself and that the goal document does not contain. CON-010 requires that a *scope change* be escalated rather than decided; swapping the reference architecture's central technology is a scope change. The decision was recorded honestly in the ADR and never put to the person who set the goal | Escalated late, in the session that found it. The rule engine stays the *test* substrate, because CON-010's autonomous loop genuinely needs a ground truth a sampled model cannot provide; whether it stays the *product* default is now an open decision for the goal's owner (M7) rather than one this project settles in an ADR |
| D35 | ADR-002 listed "two adapters must be kept behaviourally compatible, which needs a shared contract test" as a consequence, and that test was never written. For five milestones the adapters were held only to a shared Go interface — a *shape*. Worse, the selection was reachable from `arena serve` alone: the one command producing no comparison. The claim "substituting a model changes no contract" was therefore unfalsifiable from any entry point that produces a result | REQ-0104 puts the selection on `serve`, `demo` and `evalctl` alike; REQ-0105 holds both adapters to one behavioural suite; REQ-0106 carries the adapter name onto every case and every evaluation row. `evalctl run --reasoner rule,model` now produces the comparison in one invocation |
| D36 | The contract suite found a defect on the run that introduced it — in the test rather than the adapter: it asserted that an unreachable provider errors, while passing zero hypotheses, and a critique of nothing is correctly a no-op. The first version therefore proved nothing about the failure path | The suite gives the unreachable adapter real hypotheses to criticise. A contract test that cannot fail is the same category of mistake as the missing test it replaced |

### M7a — the planner port

| # | Problem | Resolution |
|---|---|---|
| D37 | **Collection decided nothing.** Every collector asked its source for everything on offer and analysed all of it, so "what did round one look at" was a property of the fixture's `default_series` rather than of any agent. That is the choice which determines the first-round error, and therefore what the critic has to overturn (REQ-0103) — the most consequential decision in the flow was the one decision no role was accountable for, and no amount of adding prompts elsewhere would have closed it | A planner port with a deterministic default that reproduces "ask for everything" exactly, so the port arrived as a refactor with the whole existing suite as its regression check. The executed plan is recorded on the case, so "why did round one not look at X" is answerable from the log rather than by re-running |
| D38 | Found by TC-0115 on its first run: the assertion "every analysed series was one the plan asked for" failed on `db_pool_saturation`, which arrives through the critic's demand rather than the plan. The test was right to fail — the boundary had not been stated anywhere | Demand-driven collection is deliberately outside the plan, and now says so in DLD-1037: a demand names the evidence it wants, which is the point of the critic holding that power, and routing it through the planner would let a strategy veto the critic |

### M7b — the model planner

| # | Problem | Resolution |
|---|---|---|
| D39 | The planner port arrived in M7a with no contract suite, which is exactly the debt ADR-002 left and D35 recorded: an interface two adapters compile against constrains signatures, not behaviour. ADR-008 had already named the risk — "each needs the contract treatment REQ-0105 gave the reasoner, or it repeats D35" | TC-0117 was written *with* the second adapter rather than after it, and REQ-0105 was broadened from "the reasoner adapters" to "every strategy port's adapters", so the obligation attaches to the next port automatically instead of depending on someone remembering |
| D40 | `sddctl validate` found DLD-1037 with no `sdd:impl` anchor: the collectors had been changed to execute the plan, but nothing in the source claimed the design item. A design item nothing implements and an implementation no design item claims are the same defect seen from two ends | Anchor added. Worth noting the tool caught it and not a person — the whole point of the anchors is that "I refactored the code and forgot the spec" is a mechanical failure rather than a judgement one |

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

The real defect looked like it was in the catalog rather than the arithmetic: "traffic
rose" is not evidence that traffic *caused* the outage, so the explanation should require
the historical comparison showing the load exceeded what was previously served — the very
evidence C1 uses to refute it and C4 uses to confirm it.

**That was implemented and measured too, and it is also wrong — for a more interesting
reason.** `sig-traffic-surge` gained two further requirements: the load must exceed any
previously served level (a fact condition on the comparison evidence), and the service
must report shedding. This raised its ceiling from 0.55 to 0.80, so acceptance became
reachable rather than impossible, and C4 rose from 0.47 to 0.72 with an `accept` verdict.
C2, C3, C5 and C6 were unaffected.

Then two tests failed, and what they said matters more than the change:

> the single-agent mode reached the correct root cause; the comparison would demonstrate nothing
> multi_no_critic reached the correct root cause; the comparison would be vacuous

A better-specified signature is not attractive enough to win round one of C1 — so the
critic has nothing left to correct, and the baselines solve the reference scenario
unaided. **C1's headline result depends on `sig-traffic-surge` being under-specified.**
The demonstration rests on a modelling weakness, not on a property of single-pass
reasoning, and fixing the weakness dissolves the demonstration.

That is worth knowing precisely, and it is not something a scoring patch can resolve. The
real work is to redesign C1 so its round-one error is wrong for a reason that survives a
well-specified catalog — a plausible explanation that a careful reasoner would still
reach first and still have to abandon.

**That redesign is now done (REQ-0103, TC-0110), so the third attempt was made — and it
fails for a third reason.** C1's round-one error is a real two-minute database outage,
so it no longer depends on `sig-traffic-surge` being weak: with the signature tightened,
C1's baselines still answer `sig-db-outage` and still fail. The blocker moved.

`sig-traffic-surge` gained the same two requirements as before, raising its ceiling from
0.55 to 0.80. The measured result across all six cases:

| Case | Mode | Before | After |
|---|---|---|---|
| C1 | multi_with_critic | correct, 2 rounds | correct, 3 rounds |
| C2, C3 | multi_with_critic | correct, 2 rounds | correct, 3 rounds |
| C4 | multi_with_critic | correct, 3 rounds | **wrong** — `sig-db-pool-exhaustion` |
| C2, C3 | single, multi_no_critic | wrong | **correct** — the baselines solve them |

Every case ran to the three-round ceiling, and C4 — the overfitting guard — broke. The
cause is D28: the two new requirements introduce demands (`load shedding log sample`, the
historical comparison as a *requirement* rather than a discriminator) that most cases
cannot answer, and an unanswerable demand is retried every round until the budget is
gone. C2 and C3 becoming solvable by a single pass is the same D13 lesson arriving from
the other side: their headline results also rested on `sig-traffic-surge` being the cheap
first answer.

The change was measured against all six cases and reverted, for the third time.

**Both prerequisites were then completed — D28 fixed, C2 and C3 redesigned — and the
fourth attempt was made.** It comes closest, and it still fails.

On the headline metric it works: all six cases correct with the critic, and the baselines
drop from 1/6 to 0/6, because C4's baseline had been getting the right answer for the
wrong reason. Underneath, three things broke. The two added requirements put two more
coverage-gap demands into every round, and at four demands per round the discriminating
evidence is displaced: C1, C2 and C3 stopped *refuting* their round-one leaders and merely
out-scored them — the exact property REQ-0103 exists to hold. Raising the cap to six
restores the refutations and breaks other things instead: C1 acquires a permanently unmet
demand for a load-shedding log it does not contain, three unit tests that encode
catalog-specific rankings fail, and **C4 still cannot act on its own accepted
explanation.**

That last point is decisive. C4 being unable to act is the consequence D13 describes; a
fix that costs a configuration change, three rewritten tests and a permanent unmet demand
in the reference scenario, and *still* does not deliver the thing it was for, is not a
fix. Reverted, for the fourth time.

**Consequence today.** C4 ranks the correct cause first and refutes both rivals with
counter-evidence, which is what TC-0104 asserts and what the overfitting test needs. It
stops below the acceptance threshold rather than proposing its declared remediation. The
case declares `expected_remediation` because that is the correct action; the system does
not currently reach it.

**Attempts, in order.** Renormalising the weights (wrong: makes a one-requirement
explanation trivially near-certain, so C1 accepts the wrong answer in round one).
Tightening the signature (wrong: removes the round-one error C1 exists to demonstrate).
Tightening it again after the C1 redesign removed that objection (wrong: the added
requirements create demands no case can answer, and D28 turns those into exhausted round
budgets — C4 breaks and C2 and C3 become solvable without a critic). Tightening it a third
time with both prerequisites met (wrong: the added demands starve the discriminating
evidence at four per round, and raising the cap trades that for a permanent unmet demand,
three rewritten tests, and a C4 that still cannot act). All four were implemented, measured
against all six cases, and reverted on the evidence rather than argued about.

**What four attempts have established.** The defect is in the arithmetic, not the catalog:
a signature is charged for evidence kinds it never claimed. Every attempt so far has tried
to work around that by making signatures claim more, and each has failed somewhere
different — which is itself the finding. The next attempt should change how the score
treats a term the signature does not require, and must be measured against the property
REQ-0103 states rather than against top-1 accuracy, because top-1 stayed at 100% through an
attempt that had quietly dismantled the refutations.

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
