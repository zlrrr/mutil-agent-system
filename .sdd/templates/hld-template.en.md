---
id: HLD-DOC-<NNN>
lang: en
counterpart: hld.zh.md
doc_version: 0.1.0
status: draft
stage: hld
---

# <Feature> — High Level Design

## Module map

| Module | Go package | Responsibility | Depends on |
|---|---|---|---|

<!-- sdd:item id=HLD-001 stage=hld status=draft derives_from=ARC-001 -->
### HLD-001 — <Module or cross-cutting mechanism>

**Purpose.** <One sentence.>

**Public surface.**

```go
// interface / type signatures only — no bodies at this stage
```

**Collaborators.** <Which modules it calls and is called by.>

**Data owned.** <What state it is the sole writer of.>

**Failure behaviour.** <What it does when a collaborator fails.>

**Refines.** ARC-XXX

## Data model

| Entity | Key fields | Owner module | Lifecycle |
|---|---|---|---|

## External interface summary

| Interface | Kind | Contract reference |
|---|---|---|

## Sequence — primary flow

```mermaid
sequenceDiagram
```

## Design review checklist

- [ ] Every ARC item has at least one refining HLD item
- [ ] No module has more than one reason to change
- [ ] Every port has an offline adapter
- [ ] Failure behaviour is specified for every external call
