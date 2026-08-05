---
id: DLD-DOC-<NNN>
lang: en
counterpart: dld.zh.md
doc_version: 0.1.0
status: draft
stage: dld
---

# <Feature> — Detailed Design

## Reading guide

A DLD item is the unit an engineer implements and a test verifies. It must contain
enough detail that two engineers would produce interchangeable implementations.

Every DLD item ends up as one or more `// sdd:impl <ID>` anchors in source.

<!-- sdd:item id=DLD-0001 stage=dld status=draft derives_from=HLD-001 -->
### DLD-0001 — <Unit of implementation>

**File.** `internal/<pkg>/<file>.go`

**Types.**

```go
type X struct { ... }
```

**Behaviour.**

1. <Step>
2. <Step>

**Invariants.** <What must hold before and after.>

**Errors.**

| Condition | Returned error | Caller expectation |
|---|---|---|

**Complexity / bounds.** <Time, memory, and hard caps.>

**Checkpoint.** TC-XXXX — <what the test asserts>

## Algorithms

### <Algorithm name>

<Pseudocode with exact constants. Constants belong here, not in prose.>

## Configuration surface

| Key | Type | Default | Effect |
|---|---|---|---|

## Implementation order

| Order | DLD items | Rationale |
|---|---|---|
