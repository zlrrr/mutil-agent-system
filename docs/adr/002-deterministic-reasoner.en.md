---
id: ADR-DOC-002
lang: en
counterpart: 002-deterministic-reasoner.zh.md
doc_version: 1.0.0
status: approved
stage: decide
---

<!-- sdd:item id=ADR-002 stage=decide status=approved derives_from=ARC-005 -->
# ADR-002 — A deterministic rule reasoner as the default, model adapter behind the port

**Status.** accepted

**Date.** 2026-08-05

## Context

This is a multi-agent system, and the obvious implementation of an agent is a call to a
language model. Two requirements make that a poor *default*.

REQ-0090 demands identical output for identical input, offline. A sampled model cannot
provide it, and pinning temperature to zero does not either — providers change, and
tests that assert on generated prose are assertions about a vendor's next release.

CON-010 demands that development proceed without a human in the loop. An autonomous
loop verifies its own work by running tests; if the tests cannot distinguish a
regression from a resampling, the loop has no ground truth and cannot proceed.

There is also a demo consideration, which matters less but is real: a system whose
central mechanism depends on a reachable endpoint fails in exactly the situation it is
meant to impress.

The temptation is to make the deterministic path a stub — a hard-coded answer for the
demo scenario, with "the real system would call a model" in the README. That would be
dishonest work: the offline mode would prove nothing about the flow.

## Decision

We will define a `Reasoner` port covering hypothesis formation and critique. The
default adapter is a genuine rule-based inference engine over a declarative fault
signature catalog: each signature declares the evidence patterns it requires, the
mechanism narrative it produces, and the discriminating evidence that would refute it.
A model-backed adapter implements the same port and is selected by configuration.

Model output, when used, is treated as untrusted structured data and passes exactly the
same contribution validation, evidence-binding and policy checks as rule output.

## Consequences

### Positive

- The full flow — collection, hypothesis, critique, demand, remediation, approval,
  verification, report — is exercised offline by the test suite, so every requirement
  above the reasoner is verifiable without a provider.
- The adversarial mechanism becomes *testable per rule*: each critique rule fires on a
  constructed fixture, which is impossible to assert reliably against generated text.
- Signatures are data (ADR-006), so extending coverage is a data change reviewable as a
  diff.
- Substituting a model changes no contract, no state transition and no stored schema.

### Negative

- The rule engine generalises only as far as its catalog. A fault whose signature is
  absent will not be hypothesised, where a model might have guessed usefully.
- The catalog is a maintenance surface, and it can overfit the demo scenarios — an
  acknowledged risk (R2) mitigated by reporting sample size and by the misleading-log
  case.
- Two adapters must be kept behaviourally compatible, which needs a shared contract
  test.

  *Follow-up, 2026-08-10.* That test was not written when this decision was taken, and
  the model adapter was reachable only from the service — so for the whole of M1..M5 the
  central claim above was untestable from any entry point that produces a comparable
  result. REQ-0104 and REQ-0105 make it testable. A consequence recorded and not built is
  a decision only half-taken.

### Neutral

- The system is best described as a multi-agent system whose reasoning strategy is
  pluggable, not as an "LLM application". The manual states this plainly rather than
  implying model involvement that the default does not have.

## Alternatives considered

| Alternative | Why it was attractive | Why it lost |
|---|---|---|
| Model-backed reasoning as the only implementation | Matches expectations of a multi-agent system; better generalisation | Forfeits REQ-0090 and leaves the autonomous loop without ground truth. Tests would assert on prose |
| Model with temperature zero and response caching | Determinism in practice; keeps model reasoning central | Reproducible only until the cache misses or the provider changes. A cache is not a specification |
| Deterministic stub returning a fixed answer for the demo | Trivial to build; demo looks identical | Proves nothing about the flow. Every requirement above the reasoner would be untested |
| Model with rule-based post-validation | Generalisation plus safety | Validation catches malformed output, not sampling variance. The determinism requirement is about the *ranking*, which validation does not stabilise |

## Revisit trigger

The evaluation shows the rule catalog failing to rank correctly on cases outside the
signature set, *and* a provider is available whose output can be pinned well enough to
keep the test suite meaningful. At that point the model adapter becomes the default for
deployment while the rule adapter remains the default for tests.
