---
id: ADR-DOC-008
lang: en
counterpart: 008-role-strategy-ports.zh.md
doc_version: 1.0.0
status: approved
stage: decide
---

<!-- sdd:item id=ADR-008 stage=decide status=approved derives_from=ARC-005 -->
# ADR-008 — Judgement-bearing roles get a strategy port; measurement roles do not

**Status.** accepted

**Date.** 2026-08-10

## Context

`project.md` §21.1 specifies a system prompt for each of nine agents. This project shipped
two: analysis and critic route through the `Reasoner` port, and the other roles are
deterministic Go. That gap, and the reasoning behind it, is recorded as D34 — a scope
change taken without escalation.

Closing the gap is not simply "add a prompt to each role". The roles differ in kind:

- **Triage** decides what to investigate and over what window. That is a judgement.
- **Collection** decides what to look at. Today it decides nothing: the metrics collector
  asks the source for every series it offers and analyses all of them. The choice of
  evidence — which in this system determines the first-round error, and therefore what the
  critic has to overturn — is made by the *fixture*, not by any agent.
- **Analysis** and **critic** already route through a strategy port.
- **Remediation** produces a typed action drawn from the catalog. The policy engine can
  only gate what it can parse, and REQ-0041 forbids agent-authored shell and SQL. A
  natural-language step here does not add judgement; it adds an unparseable surface in
  front of the only component that touches a real system.
- **Verification** compares signal values before and after an action. That is a
  measurement. A model asked "did it recover" can disagree with the arithmetic, and if it
  can, the recovery claim is no longer evidence.
- **Executor** never reasons. It authorises and invokes.

So the honest question is not "how many agents have prompts" but "which decisions are
judgements". Answering it with a count would optimise for a description of the system
rather than for the system.

## Decision

Every role whose output is a **judgement** gets a strategy port, with a deterministic
adapter as the default and a model adapter selectable by configuration — generalising
ARC-005 from one port to a family. Roles whose output is a **measurement** or a **typed
action** stay deterministic and gain no port.

| Role | Output | Strategy port |
|---|---|---|
| triage | what to investigate, over what window | yes — new |
| collection | which series and log terms to query | yes — new |
| analysis | which signatures the evidence supports | yes — exists (ARC-005) |
| critic | what is wrong with the leading explanation | yes — exists (ARC-005) |
| remediation | a typed action from the catalog | no — REQ-0041 |
| verification | whether the signals recovered | no — arithmetic |
| executor | authorise and invoke | no |

Every model adapter's output stays untrusted structured data. A planner that names a
series the source does not offer has that entry dropped; a planner that returns nothing
falls back to the deterministic plan, because a provider that fails must not be able to
blind the investigation.

**This deviates from `project.md` §21.1 and the deviation is deliberate and flagged.**
Unlike D34, it is recorded here as a decision the goal's owner may overrule, not as one
this project settles quietly. The argument against nine prompts is a safety argument
about two specific roles, and if the goal is a nine-prompt system for its own sake, that
argument loses and this ADR should be superseded.

## Consequences

### Positive

- Collection becomes an actual decision. Today "what did round one look at" is a property
  of the fixture; afterwards it is a property of the agent, and the reference scenario's
  first-round error becomes something a strategy can get right or wrong.
- The deterministic path is preserved exactly, so CON-010's autonomous loop keeps the
  ground truth it needs and every existing test keeps its meaning.
- The comparison the evaluation computes extends from "with and without a critic" to
  "with and without judgement at each stage", which is a sharper instrument.

### Negative

- More ports means more adapters to keep behaviourally compatible. Each needs the contract
  treatment REQ-0105 gave the reasoner, or it repeats D35.
- A model that plans collection can plan it badly, and a badly-planned round one is
  cheaper to reach than a well-planned one. This is measurable and should be measured
  rather than assumed either way.

### Neutral

- The system remains describable as "a multi-agent system whose reasoning strategy is
  pluggable". What changes is how much of the investigation the pluggable part covers.

## Alternatives considered

| Alternative | Why it was attractive | Why it lost |
|---|---|---|
| A prompt for all nine roles | Matches `project.md` literally; maximum surface agreement with "AI-powered agentic" | Puts natural language in front of the policy engine and turns a recovery measurement into an opinion. It buys a description at the cost of the safety argument the approval gate rests on |
| Keep only the reasoner port | Smallest change; already built | Leaves the evidence-selection decision in the fixture, which is the single most consequential choice in the flow and the one least defensible as "not an agent decision" |
| One port for all roles | Fewer types | The roles' inputs and outputs have nothing in common; a union type would be a switch statement wearing an interface |
