---
id: SDD-WORKFLOW
lang: en
counterpart: sdd-workflow.zh.md
doc_version: 1.0.0
status: approved
stage: constitution
---

# SDD Workflow — How this repository is developed

This repository is developed **specification-first**. The workflow is inspired by
[GitHub Spec Kit](https://github.com/github/spec-kit) and extended with three things
Spec Kit leaves to the human: **machine-checked traceability**, **cascading change
propagation**, and **bilingual document parity**.

Everything described here is enforced by `sddctl`, a Go tool in this repository.

## 1. The artifact chain

```
.sdd/memory/constitution.{en,zh}.md          CON-xxx   the rules that govern everything
        │
        ▼
specs/001-*/charter.{en,zh}.md               G-xxx     what we are trying to achieve
        │
        ▼
specs/001-*/spec.{en,zh}.md                  REQ-xxxx  what the system must do
        │
        ├───────────────────────────────┐
        ▼                               ▼
specs/001-*/architecture.{en,zh}.md     specs/001-*/test-plan.{en,zh}.md
        ARC-xxx + docs/adr/ ADR-xxx             TC-xxxx
        │                                       │
        ▼                                       │
specs/001-*/hld.{en,zh}.md                HLD-xxx     │
        │                                       │
        ▼                                       │
specs/001-*/dld.{en,zh}.md                DLD-xxxx    │
        │                                       │
        ▼                                       ▼
specs/001-*/tasks.{en,zh}.md   T-xxx    cmd/, internal/
                                        // sdd:impl DLD-xxxx
                                        // sdd:verify TC-xxxx
```

Each level declares its parents. `sddctl` refuses illegal edges, so the chain cannot
rot into decoration.

## 2. Item anchors

A normative item is declared with an HTML comment immediately before its heading:

```markdown
<!-- sdd:item id=REQ-0012 stage=specify status=approved derives_from=G-003 priority=P0 -->
### REQ-0012 — Critic agent can demand re-sampling
```

| Attribute | Meaning |
|---|---|
| `id` | Globally unique, prefix determines the stage |
| `stage` | Must match the prefix's configured stage |
| `status` | `draft` \| `review` \| `approved` \| `superseded` |
| `derives_from` | Comma-separated parent IDs; parent stage must be legal |
| `priority` | `P0` \| `P1` \| `P2` (requirements and goals) |

Because the anchor is language-neutral, both the English and Chinese file carry the
*identical* anchor line. This is what makes bilingual parity checkable rather than
aspirational.

In code:

```go
// sdd:impl DLD-0301
func (o *Orchestrator) Advance(...) { ... }

// sdd:verify TC-0021
func TestHypothesisRequiresEvidence(t *testing.T) { ... }
```

## 3. Commands

| Command | Purpose |
|---|---|
| `sddctl validate` | Runs lint + trace + drift and fails on any `error`-severity finding |
| `sddctl lint` | Bilingual parity: counterpart exists, versions match, item sets identical, heading skeletons match |
| `sddctl trace` | Builds the graph; reports orphans, illegal edges, unimplemented DLD items, untested requirements |
| `sddctl drift` | Compares content hashes against `.sdd/sdd.lock.json` and prints the transitive stale set |
| `sddctl seal` | Records the current content hashes as the accepted baseline |
| `sddctl gate --stage <s>` | Asserts the preconditions for entering a stage |
| `sddctl matrix` | Prints (or writes) the full traceability matrix |
| `sddctl graph` | Emits the artifact graph as Mermaid |

All commands accept `--root <dir>` and `--json`.

## 4. The cascading update loop

This is the heart of the process. Suppose a goal changes.

```
1. Edit specs/001-*/charter.en.md  AND  charter.zh.md      (CON-004)
2. sddctl drift
     STALE  REQ-0007  (parent G-003 changed)
     STALE  ARC-004   (via REQ-0007)
     STALE  HLD-006   (via ARC-004)
     STALE  DLD-0402  (via HLD-006)
     STALE  T-014     (via DLD-0402)
     STALE  code      internal/agent/critic.go       (impl DLD-0402)
     STALE  test      internal/agent/critic_test.go  (verify TC-0031)
3. Revisit each stale artifact top-down, in both languages.
4. go test ./... && sddctl validate
5. sddctl seal
```

Step 2 is the whole point: **before** touching anything you know the exact blast
radius, down to the source file. Step 5 is the only way to clear staleness, and it
records a new baseline, so the next change is measured from here.

`sddctl drift --fail-on-stale` is what CI runs. A pull request that changes a goal
but not its descendants cannot merge.

## 5. Stage gates

`sddctl gate --stage implement` refuses to pass unless:

- every DLD item is `approved`,
- nothing is stale,
- every DLD item has at least one `sdd:impl` anchor (or is explicitly deferred),
- no source file carries an anchor pointing at a non-existent item.

The gates are declared in `.sdd/config.json` and can be tightened per project phase.

## 6. Bilingual rules

1. Both files carry the same `doc_version` in front matter.
2. Both files contain the same ordered list of `sdd:item` IDs.
3. Both files contain the same number of headings at each level, in the same order.
4. Identifiers, code blocks, commands, API paths and JSON keys are byte-identical.
5. Prose is translated for a native reader, not transliterated.

Rule 3 is deliberately structural rather than semantic: a tool cannot judge
translation quality, but it can guarantee that no section was quietly added to one
language and forgotten in the other.

## 7. Where to start

```bash
make sdd-validate      # full governance check
make test              # all Go tests, offline
make sdd-matrix        # print the traceability matrix
```

New feature? Copy `.sdd/templates/*` into `specs/<NNN>-<slug>/`, fill the charter
first, and let the gates walk you down the chain.
