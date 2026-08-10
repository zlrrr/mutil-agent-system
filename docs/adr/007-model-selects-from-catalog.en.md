---
id: ADR-DOC-007
lang: en
counterpart: 007-model-selects-from-catalog.zh.md
doc_version: 1.0.0
status: approved
stage: decide
---

<!-- sdd:item id=ADR-007 stage=decide status=approved derives_from=ARC-005 -->
# ADR-007 — The model reasoner selects from the catalog and does not score

**Status.** accepted

**Date.** 2026-08-06

## Context

ADR-002 made the deterministic rule engine the default reasoner and promised a
model-backed adapter behind the same port. Building that adapter forces a question ADR-002
deferred: how much of the reasoning does the model get to do?

The tempting answer is "all of it" — let the model read the evidence and return whatever
explanations it finds, with its own confidence scores. That is what a language model is
good at, and it is the version that would handle a fault nobody anticipated.

It is also the version that quietly dismantles four properties the rest of the system
depends on.

**Scoring stops being auditable.** REQ-0022 requires every score to decompose into named
weighted terms that sum to the total, and the console renders that decomposition so a
reviewer can argue with it. A number a model asserts does not decompose. "0.85" from a
model is not a claim a reviewer can check; it is a claim they can only accept or reject.

**Refutation stops working.** The critique rules are expressed against a signature's
declared structure: what evidence it requires (`coverage_gap`), what would refute it
(`alternative_explanation`), whether its remediation can be verified
(`unverifiable_remediation`). A free-text claim declares none of that, so the critic has
nothing to test it against and every model hypothesis would sail through unchallenged —
inverting the entire point of the system.

**Remediation stops being possible.** An action comes from the signature's declared
remediation, with its allowlisted tool, its argument bounds, its rollback and its verify
signals. A model-invented cause has no remediation, so nothing downstream could act on it
even if it were correct.

**The untrusted-content boundary blurs.** REQ-0044 states that retrieved content is data
and never instructions. Model output is itself retrieved content — it is derived from
logs and runbooks that an attacker may have written. Letting it become a free-text claim
that flows into a report is the same category of mistake as letting a log line become an
action.

## Decision

The model reasoner **selects from the catalog and binds evidence to its selection**. It
does not invent claims, and it does not produce scores.

Concretely, it is asked to answer two questions:

1. Which of these declared signatures does this evidence support, and which evidence
   items support each?
2. Which hypotheses are weak, why, and what evidence would settle it?

Everything else stays deterministic. The score is computed by `reasoner.Score` from the
evidence the model bound, using the same weights the rule engine uses. Refutation,
remediation and verification all come from the catalog as before.

Every response passes the same validation as any other contribution: a hypothesis citing
an evidence identifier that does not exist is dropped, a signature identifier that is not
in the catalog is dropped, and a verdict outside the closed set is dropped. The
orchestrator's existing role and contribution checks apply unchanged (REQ-0005, REQ-0021).

## Consequences

**What this buys.** The substitution is genuinely behind the port: swapping the adapter
changes how explanations are *selected*, and changes nothing about how they are scored,
challenged, acted on or audited. Every test above the port keeps its meaning.

*Follow-up, 2026-08-10.* "Genuinely behind the port" was, until REQ-0104 and REQ-0105, a
claim rather than an observation: the adapter could be selected only from the service, and
no suite held both adapters to one behaviour. Both are now true of the code, and the
contract suite found its first defect on the run that introduced it — in the test, not the
adapter, but that is what a contract is for. A model
that hallucinates an evidence identifier produces a dropped hypothesis rather than a
fabricated citation in a report.

**What this costs, and it is a real cost.** A fault whose cause is not in the catalog
cannot be named by the model reasoner, only by adding a signature. This system will not
surprise you with an explanation nobody had written down. That is a deliberate trade: the
catalog is the closed vocabulary that makes scoring, refutation and remediation possible
at all, and an explanation that can be scored but not acted on, or acted on but not
audited, is worth less here than one that is merely absent.

**What would change this decision.** If the catalog grew large enough that coverage
stopped being the binding constraint, and if a scoring scheme existed that decomposed a
model's judgement into checkable terms, the first argument above would weaken. Neither
holds today.

**Revisit when.** A fault case arrives that the catalog cannot express, and expressing it
as a signature is found to be the harder path.
