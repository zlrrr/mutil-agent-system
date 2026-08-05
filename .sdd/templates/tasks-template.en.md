---
id: TASKS-<NNN>
lang: en
counterpart: tasks.zh.md
doc_version: 0.1.0
status: draft
stage: plan
---

# <Feature> — Task Breakdown

## Rules

- A task is complete only when its checkpoint test passes.
- A task that cannot state its checkpoint is not decomposed enough.
- Tasks marked `[P]` may run in parallel (disjoint files, no ordering edge).

<!-- sdd:item id=T-001 stage=plan status=draft derives_from=DLD-0001 -->
### T-001 — <Task title>

**Implements.** DLD-XXXX

**Files.** `<paths that this task creates or edits>`

**Definition of done.**
- [ ] `// sdd:impl DLD-XXXX` anchor present
- [ ] TC-XXXX passes
- [ ] `go vet ./...` clean

**Blocked by.** T-XXX | none

**Parallelisable.** yes | no

## Execution order

| Wave | Tasks | Gate before next wave |
|---|---|---|

## Progress log

| Task | Started | Finished | Checkpoint result |
|---|---|---|---|
