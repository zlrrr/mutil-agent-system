---
id: SPEC-000
lang: en
counterpart: framework.zh.md
doc_version: 1.0.0
status: approved
stage: specify
---

# 000 — SDD Governance Framework

This document holds the complete artifact chain for `sddctl`, the tool that enforces
the constitution. Because the tool is infrastructure rather than the product, its
goals, requirements, architecture, design and test cases live in one document; the
product specification in `specs/001-incidentops-arena/` uses one document per stage.

The chain is still fully traceable: every item below carries its own anchor and is
validated by the same engine it describes.

## 1. Goals

<!-- sdd:item id=G-901 stage=charter status=approved derives_from=CON-001,CON-002 priority=P0 -->
### G-901 — Every artifact is traceable from goal to source line

**Statement.** For any goal, a tool can enumerate the requirements, architecture
items, designs, tasks, source files and tests that realise it — and for any source
file, the goal that justifies it.

**Measure.** `sddctl matrix` prints a row per requirement with non-empty source and
test columns for every P0 requirement.

**Priority.** P0

<!-- sdd:item id=G-902 stage=charter status=approved derives_from=CON-003 priority=P0 -->
### G-902 — Any upstream change exposes its full downstream cost automatically

**Statement.** Editing an upstream artifact makes every transitive descendant
visibly stale, including the specific source and test files affected, and the tree
cannot be released until each is revisited.

**Measure.** Modifying one goal produces a non-empty stale set containing the
expected descendants, and `sddctl validate` exits non-zero until re-sealed.

**Priority.** P0

<!-- sdd:item id=G-903 stage=charter status=approved derives_from=CON-004 priority=P0 -->
### G-903 — Bilingual documentation cannot silently diverge

**Statement.** A change to one language rendering without the other is a build
failure, not a review comment.

**Measure.** `sddctl lint` reports an error for a missing counterpart, a version
mismatch, a differing item sequence or a differing heading skeleton.

**Priority.** P0

<!-- sdd:item id=G-904 stage=charter status=approved derives_from=CON-007,CON-008 priority=P1 -->
### G-904 — Governance runs offline, in-repo, with zero dependencies

**Statement.** The governance tool is a single Go binary built from this repository,
requires no network, no service and no third-party module.

**Measure.** `go build ./cmd/sddctl` succeeds with an empty `require` block in
`go.mod`, and every governance test passes with networking unavailable.

**Priority.** P1

## 2. Requirements

<!-- sdd:item id=REQ-9001 stage=specify status=approved derives_from=G-901 priority=P0 -->
### REQ-9001 — Item anchors are parsed from documents

**Requirement.** The engine MUST recognise `<!-- sdd:item id=… stage=… status=…
derives_from=… priority=… -->` anchors in markdown, associate each with the heading
that follows it, and capture the section body up to the next anchor or a heading at
the same or higher level.

**Acceptance.** Given a document with three anchors at heading level 3, when parsed,
then three items are returned with the correct identifiers, titles and non-overlapping
bodies.

**Verified by.** TC-9001

<!-- sdd:item id=REQ-9002 stage=specify status=approved derives_from=G-901 priority=P0 -->
### REQ-9002 — Illegal chain edges are rejected

**Requirement.** The engine MUST reject a `derives_from` edge whose parent stage is
not a configured legal parent of the child's stage, and MUST reject references to
identifiers that do not exist.

**Acceptance.** Given a `DLD` item deriving directly from a `REQ` item, when traced,
then an `illegalParentStage` error is reported.

**Verified by.** TC-9002

<!-- sdd:item id=REQ-9003 stage=specify status=approved derives_from=G-901 priority=P1 -->
### REQ-9003 — Derivation cycles are detected

**Requirement.** The engine MUST detect a cycle in the derivation graph and report
the participating identifiers rather than looping.

**Acceptance.** Given items A→B→C→A, when traced, then one error naming all three is
reported and the command terminates.

**Verified by.** TC-9003

<!-- sdd:item id=REQ-9004 stage=specify status=approved derives_from=G-903 priority=P0 -->
### REQ-9004 — Every document has a language counterpart

**Requirement.** The engine MUST report an error when a document exists in one
configured language but not the other, and when a markdown file in a document root
carries no language suffix and is not exempt.

**Acceptance.** Given `spec.en.md` with no `spec.zh.md`, when linted, then a
`bilingualMissingCounterpart` error is reported.

**Verified by.** TC-9004

<!-- sdd:item id=REQ-9005 stage=specify status=approved derives_from=G-903 priority=P0 -->
### REQ-9005 — Language renderings declare the same version and status

**Requirement.** The engine MUST report an error when the `doc_version` or `status`
front-matter fields of a document pair differ.

**Acceptance.** Given a pair at versions `1.0.0` and `1.1.0`, when linted, then a
`bilingualVersionMismatch` error is reported.

**Verified by.** TC-9005

<!-- sdd:item id=REQ-9006 stage=specify status=approved derives_from=G-903 priority=P0 -->
### REQ-9006 — Language renderings are structurally identical

**Requirement.** The engine MUST report an error when a document pair does not carry
the identical ordered sequence of item identifiers, or does not carry the identical
ordered sequence of heading levels.

**Acceptance.** Given one rendering with an extra section, when linted, then a
`bilingualStructureMismatch` error naming the divergence is reported.

**Verified by.** TC-9006

<!-- sdd:item id=REQ-9007 stage=specify status=approved derives_from=G-901 priority=P0 -->
### REQ-9007 — Code anchors resolve to existing design items

**Requirement.** The engine MUST scan the configured source roots for `sdd:impl` and
`sdd:verify` anchors in comments only, and MUST report an error when an anchor targets
a non-existent identifier or an identifier of a prefix that anchor kind may not target.

**Acceptance.** Given `// sdd:impl DLD-9999` with no such item, when traced, then an
`orphanCodeAnchor` error is reported at the correct file and line.

**Verified by.** TC-9007

<!-- sdd:item id=REQ-9008 stage=specify status=approved derives_from=G-902 priority=P0 -->
### REQ-9008 — The tree can be sealed to a content baseline

**Requirement.** The engine MUST record a content hash per item, covering every
language rendering plus the anchor attributes, and persist it as an accepted baseline.

**Acceptance.** Given a sealed tree with no edits, when drift is computed, then the
result is clean.

**Verified by.** TC-9008

<!-- sdd:item id=REQ-9009 stage=specify status=approved derives_from=G-902 priority=P0 -->
### REQ-9009 — Drift cascades to every transitive descendant including source files

**Requirement.** When an item's content differs from the sealed baseline, the engine
MUST mark it modified and MUST mark every transitive descendant stale, including the
source and test files carrying anchors that reach it.

**Acceptance.** Given a sealed tree in which one goal's body is edited, when drift is
computed, then the descendant requirement, architecture, design and task items appear
in the stale set and the implementing source file appears in the stale file set.

**Verified by.** TC-9009

<!-- sdd:item id=REQ-9010 stage=specify status=approved derives_from=G-902 priority=P1 -->
### REQ-9010 — Impact can be queried before editing

**Requirement.** The engine MUST answer, for any identifier, the full set of
descendant items and source files that a change would oblige the author to revisit —
without requiring the change to be made first.

**Acceptance.** Given a goal with a known subtree, when impact is queried, then the
returned sets equal that subtree.

**Verified by.** TC-9010

<!-- sdd:item id=REQ-9011 stage=specify status=approved derives_from=G-901 priority=P1 -->
### REQ-9011 — Stage gates assert entry preconditions

**Requirement.** The engine MUST evaluate the named preconditions declared for a
stage — approval status, absence of drift, coverage of parents by children, absence of
orphan anchors, absence of untested requirements and bilingual parity — and MUST fail
the gate when any is unmet or unrecognised.

**Acceptance.** Given a tree with one unimplemented design item, when the `implement`
gate is evaluated, then it fails and names that item.

**Verified by.** TC-9011

<!-- sdd:item id=REQ-9012 stage=specify status=approved derives_from=G-901 priority=P2 -->
### REQ-9012 — Traceability is reportable as a matrix and a graph

**Requirement.** The engine MUST render a requirement-to-source traceability matrix in
markdown and the derivation graph in Mermaid.

**Acceptance.** Given a tree in which a requirement reaches source and tests, when the
matrix is rendered, then that requirement's row is marked covered.

**Verified by.** TC-9012

<!-- sdd:item id=REQ-9013 stage=specify status=approved derives_from=G-904 priority=P1 -->
### REQ-9013 — The governance tool has no third-party dependencies

**Requirement.** The engine and its command MUST compile using the Go standard
library only.

**Acceptance.** `go.mod` contains no `require` directive for an external module.

**Verified by.** TC-9013

## 3. Architecture

<!-- sdd:item id=ARC-901 stage=architect status=approved derives_from=REQ-9001,REQ-9002,REQ-9003 -->
### ARC-901 — The artifact chain is a typed, configuration-declared DAG

**Element.** A directed acyclic graph whose node types are declared in
`.sdd/config.json` as ordered stages, each owning an identifier prefix and a set of
legal parent prefixes.

**Responsibility.** Decide whether a proposed edge between two artifacts is legal.

**Constraints.** The graph is never inferred from file layout or naming; only explicit
`derives_from` attributes create edges. Adding a stage is a configuration change, not
a code change.

**Realises.** REQ-9001, REQ-9002, REQ-9003

<!-- sdd:item id=ARC-902 stage=architect status=approved derives_from=REQ-9008,REQ-9009,REQ-9010 -->
### ARC-902 — Content hashing is the change-propagation primitive

**Element.** A per-item content hash, sealed into a lock file, over which staleness is
defined as reachability from a changed node.

**Responsibility.** Turn "something upstream changed" into an exact, enumerable set of
downstream work.

**Constraints.** Hashing is insensitive to trailing whitespace and blank-line runs, and
sensitive to everything else including anchor attributes. Staleness is cleared only by
an explicit re-seal, never as a side effect of any other command.

**Realises.** REQ-9008, REQ-9009, REQ-9010

<!-- sdd:item id=ARC-903 stage=architect status=approved derives_from=REQ-9004,REQ-9005,REQ-9006 -->
### ARC-903 — A document pair is one artifact with two renderings

**Element.** The `<base>.<lang>.md` pairing rule, plus a structural comparison of item
sequences and heading skeletons.

**Responsibility.** Guarantee that no section, requirement or decision exists in one
language only.

**Constraints.** The comparison is structural, never semantic: the engine asserts that
both renderings describe the same skeleton, and deliberately does not attempt to judge
translation quality. Item hashing spans both renderings, so a change in either cascades.

**Realises.** REQ-9004, REQ-9005, REQ-9006

<!-- sdd:item id=ARC-904 stage=architect status=approved derives_from=REQ-9007,REQ-9011,REQ-9012,REQ-9013 -->
### ARC-904 — Governance is a dependency-free binary inside the repository

**Element.** `cmd/sddctl` over `internal/sdd`, standard library only.

**Responsibility.** Make the constitution executable in CI, on a laptop and inside an
autonomous development loop, with identical results.

**Constraints.** No network access, no external service, no third-party module. Source
anchors are recognised in comments only, so that a string literal mentioning the
keyword cannot forge an anchor.

**Realises.** REQ-9007, REQ-9011, REQ-9012, REQ-9013

## 4. High level design

<!-- sdd:item id=HLD-901 stage=hld status=approved derives_from=ARC-901 -->
### HLD-901 — Configuration and stage model

**Purpose.** Load the governance configuration and answer stage/parent legality
questions.

**Public surface.**

```go
func LoadConfig(root string) (*Config, error)
func (c *Config) StageByPrefix(prefix string) (StageConfig, bool)
func (c *Config) IsLegalParent(childID, parentID string) bool
func (c *Config) SeverityOf(class string) Severity
```

**Failure behaviour.** A malformed or absent configuration is fatal; an unknown finding
class resolves to `error` so that new checks fail closed.

**Refines.** ARC-901

<!-- sdd:item id=HLD-902 stage=hld status=approved derives_from=ARC-904 -->
### HLD-902 — Finding and report model

**Purpose.** Represent violations uniformly and decide the process exit code.

**Public surface.**

```go
type Finding struct{ Class, Level, Subject, File, Message string; Line int }
func (r *Report) Add(cfg *Config, class, subject, file string, line int, format string, args ...any)
func (r *Report) HasErrors() bool
```

**Refines.** ARC-904

<!-- sdd:item id=HLD-903 stage=hld status=approved derives_from=ARC-901,ARC-903 -->
### HLD-903 — Document and source anchor parser

**Purpose.** Turn markdown and Go source into items, headings, front matter and code
anchors.

**Public surface.**

```go
func ParseDoc(root, path string) (*Doc, []*Item, error)
func ParseCodeAnchors(root string, codeRoots []string) ([]CodeAnchor, error)
```

**Failure behaviour.** Fenced code blocks are skipped so that examples inside
documentation cannot declare items.

**Refines.** ARC-901, ARC-903

<!-- sdd:item id=HLD-904 stage=hld status=approved derives_from=ARC-901,ARC-903 -->
### HLD-904 — Model assembly and graph traversal

**Purpose.** Merge the language renderings of each item, order items by stage, and
expose parent/child traversal including synthetic source-file leaves.

**Public surface.**

```go
func Load(root string) (*Model, error)
func (m *Model) DescendantsOf(ids ...string) []string
func (m *Model) AnchorsFor(kind, id string) []CodeAnchor
```

**Data owned.** The merged item table and the child index.

**Refines.** ARC-901, ARC-903

<!-- sdd:item id=HLD-905 stage=hld status=approved derives_from=ARC-901,ARC-903 -->
### HLD-905 — Lint and trace checks

**Purpose.** Apply the bilingual rules and the graph rules, producing findings.

**Public surface.**

```go
func (m *Model) Lint() *Report
func (m *Model) Trace() *Report
func (m *Model) Validate() *Report
```

**Refines.** ARC-901, ARC-903

<!-- sdd:item id=HLD-906 stage=hld status=approved derives_from=ARC-902 -->
### HLD-906 — Seal and drift

**Purpose.** Persist the accepted baseline and compute the stale set.

**Public surface.**

```go
func (m *Model) Seal() (*Lock, error)
func (m *Model) Drift() (*DriftResult, error)
func (m *Model) ImpactOf(ids ...string) (items []string, files []string)
```

**Failure behaviour.** A missing lock file is not an error; it reports an unsealed tree
so a fresh repository can be sealed for the first time.

**Refines.** ARC-902

<!-- sdd:item id=HLD-907 stage=hld status=approved derives_from=ARC-904 -->
### HLD-907 — Stage gates

**Purpose.** Evaluate the declared preconditions for entering a stage.

**Public surface.**

```go
func (m *Model) Gate(stage string) (*GateResult, error)
```

**Failure behaviour.** An unrecognised precondition fails the gate rather than being
ignored, so a typo in configuration cannot weaken a gate.

**Refines.** ARC-904

<!-- sdd:item id=HLD-908 stage=hld status=approved derives_from=ARC-904 -->
### HLD-908 — Matrix and graph rendering

**Purpose.** Render traceability for humans and for documents.

**Public surface.**

```go
func (m *Model) Matrix() []MatrixRow
func (m *Model) RenderMatrix() string
func (m *Model) RenderGraph() string
```

**Refines.** ARC-904

<!-- sdd:item id=HLD-909 stage=hld status=approved derives_from=ARC-904 -->
### HLD-909 — Command line interface

**Purpose.** Expose the engine as subcommands with consistent flags and exit codes.

**Public surface.** `validate`, `lint`, `trace`, `drift`, `seal`, `gate`, `impact`,
`matrix`, `graph`, `stats`.

**Failure behaviour.** Exit `0` clean, `1` on findings or a failed gate, `2` on a usage
or I/O error.

**Refines.** ARC-904

## 5. Detailed design

<!-- sdd:item id=DLD-0101 stage=dld status=approved derives_from=HLD-901 -->
### DLD-0101 — Configuration loader

**File.** `internal/sdd/config.go`

**Behaviour.** Read `.sdd/config.json`; reject empty stage lists, empty or duplicate
prefixes and unknown parent prefixes; default the lock path when unset. `PrefixOf`
splits an identifier at the first hyphen and returns empty for malformed input.

**Checkpoint.** TC-9002 — an illegal parent prefix is refused.

<!-- sdd:item id=DLD-0102 stage=dld status=approved derives_from=HLD-902 -->
### DLD-0102 — Finding, severity and report

**File.** `internal/sdd/finding.go`

**Behaviour.** Severity ordering `info < warn < error`; `Sorted` orders by descending
severity, then class, then subject; `Summary` tallies each level.

**Checkpoint.** TC-9014 — findings sort most-serious-first.

<!-- sdd:item id=DLD-0103 stage=dld status=approved derives_from=HLD-903 -->
### DLD-0103 — Markdown and source parser

**File.** `internal/sdd/parse.go`

**Behaviour.**

1. Consume optional `---` front matter as `key: value` pairs.
2. Track fenced code blocks and skip their contents entirely.
3. On an `sdd:item` anchor, close any open item at the current line and open a new one;
   the first heading after the anchor sets the item's title and heading level.
4. Close an open item at the next anchor or at the next heading whose level is less
   than or equal to the item's own.
5. For source, accept anchors only on comment lines (`//` or `*`).

**Invariants.** Item bodies never overlap. A document with no anchors yields no items.

**Checkpoint.** TC-9001 — anchors, titles and bodies are extracted correctly.

<!-- sdd:item id=DLD-0104 stage=dld status=approved derives_from=HLD-904 -->
### DLD-0104 — Model assembly

**File.** `internal/sdd/model.go`

**Behaviour.** Parse every document under the document roots in sorted order; merge
repeated identifiers as additional language occurrences, reporting a structure mismatch
when their attributes disagree; order items by stage rank then lexically; build the
child index, adding a synthetic `file:<path>` child for each code anchor.

**Complexity.** Linear in total document bytes; graph traversal linear in edges.

**Checkpoint.** TC-9009 — descendants include synthetic source-file nodes.

<!-- sdd:item id=DLD-0105 stage=dld status=approved derives_from=HLD-905 -->
### DLD-0105 — Lint and trace

**File.** `internal/sdd/check.go`

**Behaviour.** `Lint` groups documents by base path, requires each configured language,
and compares the primary rendering against each other rendering on version, status, item
sequence and heading-level skeleton. `Trace` validates stage/prefix agreement, parent
existence and legality, detects cycles by three-colour depth-first search, resolves code
anchors, and reports unimplemented designs, unverified test cases and untested
requirements.

**Checkpoint.** TC-9003, TC-9004, TC-9005, TC-9006, TC-9007.

<!-- sdd:item id=DLD-0106 stage=dld status=approved derives_from=HLD-906 -->
### DLD-0106 — Seal, drift and impact

**File.** `internal/sdd/drift.go`

**Behaviour.** `Seal` writes `{hash, stage, status, file}` per item plus a UTC
timestamp. `Drift` classifies each item as modified, added or removed, then computes
`DescendantsOf(modified ∪ removed)`, splitting synthetic `file:` nodes into the stale
file list. Items already reported as modified are not repeated in the stale list.

**Invariants.** A freshly sealed, unedited tree yields `Clean() == true`.

**Checkpoint.** TC-9008, TC-9009, TC-9010.

<!-- sdd:item id=DLD-0107 stage=dld status=approved derives_from=HLD-907 -->
### DLD-0107 — Stage gate evaluation

**File.** `internal/sdd/gate.go`

**Behaviour.** A precondition named `<stage>-approved` requires every item of that
stage's prefix to be `approved`, `superseded` or `deferred`. Named preconditions
`no-drift`, `no-orphan-goals`, `no-orphan-requirements`, `no-unimplemented-dld`,
`no-orphan-code`, `no-untested-requirements` and `bilingual-parity` are evaluated
directly. An unrecognised name is recorded in `Unknowns` and fails the gate.

**Checkpoint.** TC-9011.

<!-- sdd:item id=DLD-0108 stage=dld status=approved derives_from=HLD-908 -->
### DLD-0108 — Matrix and graph rendering

**File.** `internal/sdd/matrix.go`

**Behaviour.** For each requirement, partition its descendant set by prefix into
architecture, design, task and test columns; collect implementing source files from the
design items' `impl` anchors and test files from the test cases' `verify` anchors; mark
a row covered when both are non-empty. `RenderGraph` emits one Mermaid subgraph per
populated stage plus one edge per legal `derives_from`.

**Checkpoint.** TC-9012.

<!-- sdd:item id=DLD-0109 stage=dld status=approved derives_from=HLD-909 -->
### DLD-0109 — Command line interface

**File.** `cmd/sddctl/main.go`

**Behaviour.** Dispatch on `os.Args[1]`; parse shared flags `--root`, `--json`,
`--strict`, `--fail-on-stale`, `--stage`, `--out`. `validate`, `lint` and `trace` exit
`1` when any error finding is present, or when `--strict` and any warning is present.
`drift --fail-on-stale` exits `1` when the tree is not clean. `gate` exits `1` when the
gate fails.

**Checkpoint.** TC-9013 — the binary builds and runs with no external module.

## 6. Test cases

<!-- sdd:item id=TC-9001 stage=verify status=approved derives_from=REQ-9001 -->
### TC-9001 — Anchors, titles and bodies are parsed

**Level.** unit

**Steps.** Parse a fixture document with three level-3 anchors, one fenced code block
containing a decoy anchor, and front matter.

**Expected.** Exactly three items; titles have the identifier prefix stripped; the decoy
inside the fence produces no item; bodies do not overlap.

**Test function.** `TestParseDocExtractsItems` in `internal/sdd/parse_test.go`

<!-- sdd:item id=TC-9002 stage=verify status=approved derives_from=REQ-9002 -->
### TC-9002 — Illegal and unknown edges are reported

**Level.** unit

**Steps.** Build a fixture tree containing a `DLD` item deriving from a `REQ` item and
an item deriving from a non-existent identifier.

**Expected.** One `illegalParentStage` error and one `unknownParent` error.

**Test function.** `TestTraceRejectsIllegalEdges` in `internal/sdd/check_test.go`

<!-- sdd:item id=TC-9003 stage=verify status=approved derives_from=REQ-9003 -->
### TC-9003 — Cycles terminate with a report

**Level.** unit

**Steps.** Build a fixture tree with a three-item derivation cycle.

**Expected.** Tracing returns within the test timeout and reports the cycle.

**Test function.** `TestTraceDetectsCycle` in `internal/sdd/check_test.go`

<!-- sdd:item id=TC-9004 stage=verify status=approved derives_from=REQ-9004 -->
### TC-9004 — A missing counterpart is an error

**Level.** unit

**Steps.** Create a document root containing only the English rendering.

**Expected.** One `bilingualMissingCounterpart` error naming the base path.

**Test function.** `TestLintMissingCounterpart` in `internal/sdd/check_test.go`

<!-- sdd:item id=TC-9005 stage=verify status=approved derives_from=REQ-9005 -->
### TC-9005 — Version and status divergence is an error

**Level.** unit

**Steps.** Create a pair whose `doc_version` values differ.

**Expected.** One `bilingualVersionMismatch` error.

**Test function.** `TestLintVersionMismatch` in `internal/sdd/check_test.go`

<!-- sdd:item id=TC-9006 stage=verify status=approved derives_from=REQ-9006 -->
### TC-9006 — Structural divergence is an error

**Level.** unit

**Steps.** Create a pair where the Chinese rendering omits one item and adds one
heading.

**Expected.** A `bilingualStructureMismatch` error identifying the differing sequences.

**Test function.** `TestLintStructureMismatch` in `internal/sdd/check_test.go`

<!-- sdd:item id=TC-9007 stage=verify status=approved derives_from=REQ-9007 -->
### TC-9007 — Orphan code anchors are errors

**Level.** unit

**Steps.** Place a source file containing `// sdd:impl DLD-9999` and a string literal
mentioning the keyword.

**Expected.** One `orphanCodeAnchor` error at the comment's line; the string literal
produces no anchor.

**Test function.** `TestTraceOrphanCodeAnchor` in `internal/sdd/check_test.go`

<!-- sdd:item id=TC-9008 stage=verify status=approved derives_from=REQ-9008 -->
### TC-9008 — A sealed tree is clean

**Level.** unit

**Steps.** Seal a fixture tree and immediately compute drift.

**Expected.** `Clean()` is true and no findings are produced.

**Test function.** `TestSealThenDriftIsClean` in `internal/sdd/drift_test.go`

<!-- sdd:item id=TC-9009 stage=verify status=approved derives_from=REQ-9009 -->
### TC-9009 — Editing a goal cascades to descendants and source files

**Level.** integration

**Steps.** Seal a fixture tree whose goal reaches a requirement, an architecture item, a
design item and a source file; edit the goal's body; recompute drift.

**Expected.** The goal is `Modified`; the requirement, architecture and design items are
`Stale`; the implementing source file is in `StaleFile`; `Clean()` is false.

**Test function.** `TestDriftCascadesToCode` in `internal/sdd/drift_test.go`

<!-- sdd:item id=TC-9010 stage=verify status=approved derives_from=REQ-9010 -->
### TC-9010 — Impact is answerable without editing

**Level.** unit

**Steps.** Query the impact of a goal in an unmodified fixture tree.

**Expected.** The returned item and file sets equal the goal's known subtree.

**Test function.** `TestImpactOf` in `internal/sdd/drift_test.go`

<!-- sdd:item id=TC-9011 stage=verify status=approved derives_from=REQ-9011 -->
### TC-9011 — Gates fail on unmet and unknown preconditions

**Level.** unit

**Steps.** Evaluate the `implement` gate on a tree with one unimplemented design item;
then evaluate a gate declaring an unrecognised precondition.

**Expected.** The first fails naming the design item; the second fails with the
precondition listed in `Unknowns`.

**Test function.** `TestGateImplement` in `internal/sdd/gate_test.go`

<!-- sdd:item id=TC-9012 stage=verify status=approved derives_from=REQ-9012 -->
### TC-9012 — Matrix and graph render traceability

**Level.** unit

**Steps.** Render the matrix and the graph for a fixture tree with one fully covered
requirement.

**Expected.** The matrix row is marked covered and lists the source file; the graph
contains the goal-to-requirement edge.

**Test function.** `TestMatrixAndGraph` in `internal/sdd/matrix_test.go`

<!-- sdd:item id=TC-9013 stage=verify status=approved derives_from=REQ-9013 -->
### TC-9013 — The module declares no external dependency

**Level.** governance

**Steps.** Read `go.mod` from the repository root.

**Expected.** No `require` directive naming a module outside the standard library.

**Test function.** `TestNoExternalDependencies` in `internal/sdd/deps_test.go`

<!-- sdd:item id=TC-9014 stage=verify status=approved derives_from=REQ-9001 -->
### TC-9014 — Findings sort most serious first

**Level.** unit

**Steps.** Add findings of mixed severity to a report and sort.

**Expected.** Errors precede warnings, which precede info; ties break by class then
subject.

**Test function.** `TestReportSorted` in `internal/sdd/finding_test.go`
