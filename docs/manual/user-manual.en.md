---
id: MANUAL-001
lang: en
counterpart: user-manual.zh.md
doc_version: 1.0.0
status: approved
stage: constitution
---

# IncidentOps Arena — User Manual

## 1. What this system is

IncidentOps Arena investigates a production alert with a team of agents that each hold
one kind of evidence, then makes one of them argue against the others.

The problem it addresses is not that nobody looks at an incident. It is that the first
coherent story tends to end the search. Given a log full of `connection timeout`, a
single reasoner will confidently narrate a database outage — and stop. Nothing in a
single pass is structurally obliged to ask *what else would produce this evidence?*

So the system splits evidence gathering by source, and pays one role to disagree. That
role has real authority: it can demand specific additional evidence, which sends the
investigation back for another round, and it can refuse to let a conclusion proceed to
remediation.

**What it is not.** It does not repair production systems by itself. Every action that
would change a target system above the low-risk class stops at an approval gate until a
person decides. It is not an "LLM application" either: the default reasoning strategy is
a deterministic rule engine over a fault signature catalog, which is why the whole thing
runs offline and reproducibly. A model-backed reasoner is a documented substitution
behind the same port — see section 9.

## 2. Installation

### 2.1 With Docker

The container is the recommended deliverable. It carries the API, the console, the
fault case catalog and the command line tools.

**From a published release** (`linux/amd64`). Every tagged version publishes an image
built from that exact commit:

```bash
docker pull ghcr.io/zlrrr/mutil-agent-system:0.1.0
docker run --rm -p 8080:8080 ghcr.io/zlrrr/mutil-agent-system:0.1.0
```

If you cannot reach the registry, the same image is attached to the release as a
loadable tarball, so the release stays usable without registry access:

```bash
gunzip -c incidentops-arena-0.1.0-linux-amd64-image.tar.gz | docker load
docker run --rm -p 8080:8080 ghcr.io/zlrrr/mutil-agent-system:0.1.0
```

Release assets are checksummed; verify them with `sha256sum -c SHA256SUMS` before
loading. Each image is also tagged with its commit SHA, so a version tag that moves
does not cost you the ability to name exactly what you ran.

**From source**, if you would rather build it yourself:

```bash
docker build -f deploy/docker/Dockerfile -t incidentops-arena:0.1.0-mvp .
docker run --rm -p 8080:8080 incidentops-arena:0.1.0-mvp
```

Then open <http://localhost:8080>.

The image runs as a non-root user, declares a health check on `/healthz`, and stores
case logs in the `/data` volume. Nothing inside it reaches the network.

### 2.2 From source

Go 1.22 or newer is the only requirement. The module has no third-party dependencies,
so there is nothing to download.

```bash
make build          # produces ./bin/{arena,evalctl,faultctl,sddctl,demo-order-api}
make check          # vet, the full test suite, and the governance gate
./bin/arena serve
```

### 2.3 The full demo stack

To drive the same scenario against a live service rather than fixture data:

```bash
make up             # arena, the demo order service, a traffic generator, Prometheus
```

| Service | Address | Purpose |
|---|---|---|
| arena | <http://localhost:8080> | API and console |
| order-api | <http://localhost:8081> | the demo target, with a configurable connection pool |
| prometheus | <http://localhost:9090> | scrapes the demo target and holds the alert rules |

`make down` stops the stack and removes its volumes.

## 3. The five-minute walkthrough

This is the reference scenario, `C1`. It runs offline and produces the same result
every time.

```bash
make demo
```

You will see the investigation reach the approval gate, approve its own action (the
demo command does this so the walkthrough completes), verify recovery, and print the
report. What happened, in order:

**Round 1 — the plausible wrong answer.** Five collectors run in parallel with their
default queries. Metrics finds the error rate, latency and request rate all elevated,
and finds the database availability gauge collapsing to zero at 10:07:00. Logs finds 184
database connection timeouts. Topology confirms `order-api` is the earliest anomalous
service. Knowledge matches two runbooks.

The analysis role ranks three explanations:

| Rank | Explanation | Score |
|---|---|---|
| 1 | The database became unavailable | 0.65 |
| 2 | A traffic increase exceeded capacity | 0.49 |
| 3 | The database connection pool was exhausted | 0.40 |

The leader is **wrong**, and it is worth being precise about how. It is not winning on a
gap: every requirement it declares — a collapsed availability signal, database
connection errors in the log — is satisfied by evidence actually in hand. The database
really did go down at 10:07. On this evidence it is the answer a careful reader gives.

**The critic intervenes.** The pool explanation requires a saturation metric that nobody
queried. It also requires a configuration change, and the default change query only
looks back to the alert. And nothing collected so far separates the rival explanations
from the leader.

The critic issues four demands:

- the database connection pool saturation metric
- configuration changes in the 30 minutes before onset
- the error rate duration compared with the database recovery time
- a historical traffic comparison at equal load

**Round 2 — the demands are answered.** The pool metric comes back saturated. The
extended change query finds `DB_POOL_SIZE` changed from `20` to `2` at 10:05:30, ninety
seconds before the pool saturated. The historical comparison shows an equal peak served
two days earlier with a 0.20% error rate — counter-evidence against the traffic
explanation. And the duration comparison shows the database was available again from
10:09:00 while the error rate stayed elevated until 10:27:00: eighteen further minutes
of failures that a two-minute outage cannot account for.

The ranking reorders:

| Rank | Explanation | Score | Note |
|---|---|---|---|
| 1 | The connection pool size was reduced, exhausting the pool | 0.94 | now supported by five evidence kinds |
| 2 | The database became unavailable | 0.47 | the outage ended 18 minutes before the errors did |
| 3 | A traffic increase exceeded capacity | 0.29 | carries counter-evidence, penalised 0.20 |

Both rivals end the case *refuted* rather than merely out-scored: each carries evidence
recorded against it, which is a stronger outcome than losing on points.

**The gate.** Remediation proposes `set_config order-api DB_POOL_SIZE 20` at medium
risk, with a rollback to `2` and verification against the error rate and latency. The
state machine stops. No actuator has been invoked. To see this for yourself:

```bash
make demo-gate      # runs to the gate and stops
```

**After approval.** The executor re-checks policy against the action as it now stands,
invokes the actuator, and the verification role re-queries both signals over a
post-action window. The error rate falls from a mean of 0.129 to 0.003 and latency from
1623ms to 190ms; both are reported as recovered. The report is written and the case
closes.

## 4. Using the console

Open <http://localhost:8080>, pick a fault case and press **Open investigation**.

| Region | What it shows |
|---|---|
| Summary | The alert, the accepted root cause, its confidence and the critic's verdict |
| Agent timeline | Every event as it happens, streamed live |
| Evidence | Grouped by kind, each item with its identifier and the exact query behind it |
| Hypotheses | Ranked, each with the full six-term score breakdown and its evidence chain |
| Adversarial review | Every critique with its rule, verdict and challenge, plus the evidence demanded |
| Remediation | The proposed action, its risk, rollback and verification, with the approval control |
| Report | The rendered root-cause report, also downloadable as Markdown or JSON |

Every evidence identifier on the page resolves to a `RawRef` — the query that produced
it. That is the point: a claim you cannot trace is a claim you cannot check.

## 5. The command line

```bash
arena serve   --addr :8080 --store memory|file --data ./data
arena demo    --case C1 --mode multi_with_critic --approve --out report.md
arena cases
evalctl run   --cases C1,C2 --out docs/evaluation.md
faultctl inject|restore|status --case C1 --target http://localhost:8081
sddctl        validate|lint|trace|drift|seal|gate|impact|matrix|graph
```

Configuration precedence is flag, then environment variable, then default:

| Variable | Default | Effect |
|---|---|---|
| `ARENA_ADDR` | `:8080` | listen address |
| `ARENA_STORE` | `memory` | `memory` or `file` |
| `ARENA_DATA` | `./data` | directory for the file store |
| `FAULTCTL_TARGET` | `http://localhost:8081` | demo service address |

## 6. The API

| Method and path | Purpose |
|---|---|
| `POST /api/cases` | Open a case from an alert, or from a `case_ref` alone |
| `GET /api/cases` | List cases, newest first |
| `GET /api/cases/{id}` | The full case projection |
| `GET /api/cases/{id}/events` | The event log, optionally `?from=N` |
| `GET /api/cases/{id}/stream` | Live server-sent events, resumable via `Last-Event-ID` |
| `POST /api/cases/{id}/actions/{actionID}/decision` | Record an approval decision |
| `GET /api/cases/{id}/report.md` | The report as Markdown |
| `GET /api/cases/{id}/report.json` | The report as JSON |
| `GET /api/catalog/cases` | The reproducible fault cases |
| `GET /healthz` | Health probe |

Opening a case from an alert:

```bash
curl -sS localhost:8080/api/cases -H 'content-type: application/json' -d '{
  "alert_name": "OrderApiHighErrorRate",
  "service": "order-api",
  "severity": "P1",
  "starts_at": "2026-07-26T10:07:00Z",
  "case_ref": "C1"
}'
```

Approving the action it proposes:

```bash
curl -sS -X POST \
  localhost:8080/api/cases/inc-445761e8c3/actions/a-remediation-001/decision \
  -H 'content-type: application/json' \
  -d '{"decision":"approved","by":"you","comment":"restore the pool size"}'
```

Validation errors return `400` with `{"error": "...", "field": "..."}`; an unknown case
returns `404`.

## 7. The safety model

This is the part worth reading carefully, because it is what makes the system safe to
point at anything.

**Read-only tools run autonomously. Writes do not.** Every action carries a risk class.
`low` may execute automatically; `medium` and `high` halt the state machine until a
person records a decision.

**Some things are refused as categories, before any allowlist is consulted.** Arbitrary
shell, free-form SQL, resource deletion and cross-service bulk operations are denied
unconditionally. No configuration can enable them, and an approval does not override
them.

**Policy is checked twice.** Once when an action is proposed, and again in the instant
before it executes — against the action exactly as it then stands. An action whose
arguments changed after approval is refused as tampered. A policy tightened after
approval refuses the action it previously allowed.

**Retrieved content is data, never instruction.** Log lines, commit messages, runbooks
and tickets enter only as evidence payloads. They cannot alter agent roles, tool
selection, policy or approval state. The executor accepts typed actions only. The
reference scenario's fixture deliberately contains a log line instructing the system to
run `rm -rf` and restart every service; it produces no action, and the test suite
asserts that.

**Every decision is auditable.** Approvals record the decision, the identity, the
timestamp and the comment. Executions record the outcome, the duration and the target's
response. Both appear in the event log and in the report.

The allowlist ships with these defaults and is configuration, not code:

| Category | Permitted |
|---|---|
| Services | `order-api`, `payment-api`, `inventory-api`, `checkout-web` |
| Tools | `set_config`, `restart_service`, `scale_service`, `create_ticket` |
| Configuration keys | `DB_POOL_SIZE`, `FEATURE_FLAG_SAFE_MODE`, `RATE_LIMIT_QPS`, `PAYMENT_URL` |
| Value ranges | `DB_POOL_SIZE` 1–200, `RATE_LIMIT_QPS` 1–10000 |

## 8. Evaluating the claim

The system claims that the adversarial flow beats a single pass. That claim is computed,
not asserted:

```bash
make eval
```

Three modes run over **identical** fixture inputs, so a difference is attributable to
the flow rather than to the data:

- `single` — one agent with every tool, one pass, no critique
- `multi_no_critic` — the collectors and the analysis role, no adversarial round
- `multi_with_critic` — the full flow

Three cases in that set do specific work. `C4`'s correct answer is `sig-traffic-surge` —
the explanation the reference scenario spends its whole second round demoting — and both
baselines get it right. It exists so that a system which had merely learned "the leading
explanation is wrong" would fail somewhere, which is what makes the adversarial flow's
100% a claim about discrimination rather than about reflexive objection. `C5` alerts on a
service downstream of the fault, so the answer is not in the alerting service at all.
`C6` contains a configuration change that fits the leading explanation perfectly and
landed four minutes *after* the errors began; only the ordering refutes it.

Read the numbers with two caveats, both of which the report prints alongside them.
First, the sample is six fault cases; a 100% top-1 accuracy on six cases is a
statement about six cases. Second, the single-agent baseline is given the same tools
and the same default queries as the full flow — it models "one context window, one
look", not a weaker toolset. That is the fairest baseline available here, and it is
also the one that makes the comparison meaningful.

## 9. Extending the system

**Adding a fault scenario** is a data change. Drop a `case-*.json` into
`internal/catalog/data/` declaring the alert, the fixture signals, the expected root
cause and the expected remediation. The evaluation picks it up with no code change.

**Adding a fault signature** is also data: declare the evidence patterns it requires,
the mechanism narrative it produces, the discriminating evidence that would refute it,
and optionally the remediation it implies.

**Substituting a model-backed reasoner** means implementing one interface:

```go
type Reasoner interface {
    Hypothesise(ctx context.Context, s domain.Snapshot) ([]domain.Hypothesis, error)
    Critique(ctx context.Context, s domain.Snapshot) ([]domain.Critique, []domain.EvidenceDemand, error)
}
```

Nothing above the port changes: the same contracts, the same state machine, the same
storage. Model output is treated as untrusted structured data and passes exactly the
same validation, evidence binding and policy checks as rule output. The deterministic
adapter remains the default for tests, because a suite cannot assert on a sampled
distribution.

That adapter is implemented. It works against any chat-completions style JSON API:

```bash
arena serve \
  --reasoner model \
  --model-endpoint https://your-provider.example/v1/chat/completions \
  --model-name your-model
```

The API key is read from `ARENA_MODEL_API_KEY`. There is deliberately no default
endpoint: no provider has been chosen for this project, and a default would be a
decision the code is not entitled to make on your behalf.

**What the model is asked to do, and what it is not.** It selects which of the catalog's
signatures the evidence supports and which evidence supports each. It does not invent
explanations, and it does not assign scores — those are computed locally by the same
scorer the rule engine uses, with the same weights. The reasoning behind that split is
[ADR-007](../adr/007-model-selects-from-catalog.en.md); the short version is that a
number a model asserts cannot be decomposed, refuted or acted on, and this system
requires all three.

Consequently the model reasoner cannot name a cause that is not in the catalog. That is
a real limitation, stated rather than hidden: adding an explanation means adding a
signature, which is a data change.

**When the provider fails**, the adapter returns a typed error rather than an empty
result. An empty hypothesis list means "nothing matched"; a provider that is down must
not be able to say that.

**Adding a live signal source** means implementing one of the six port interfaces in
`internal/signal` and registering it in `internal/signal/profile`.

### 9.1 Running against live signals

Two ports have live adapters: metrics read the Prometheus HTTP API, and logs read a
container runtime. The other four stay on fixtures.

The fixture profile is the default, and that is deliberate — a deployment that meant to
read live signals and silently read simulated ones would look healthy while proving
nothing. So the live profile must be named, and it refuses to start half-configured:

```bash
arena serve \
  --signal-profile live \
  --prometheus-url http://prometheus:9090 \
  --container-host unix:///var/run/docker.sock \
  --series-map ./series-map.json
```

Every flag also reads from an environment variable — `ARENA_SIGNAL_PROFILE`,
`ARENA_PROMETHEUS_URL`, `ARENA_CONTAINER_HOST`, `ARENA_SERIES_MAP` — which is how the
container image is configured.

The series map is what makes adding a metric configuration rather than code:

```json
{
  "series": {
    "http_error_rate": "rate(http_requests_total{status=~\"5..\"}[1m])",
    "db_pool_in_use": "db_pool_connections_in_use"
  },
  "units":      { "http_error_rate": "ratio" },
  "capacities": { "db_pool_in_use": 20 }
}
```

A misspelled profile name is rejected at start-up with exit code `2` and the offending
flag named. It is never treated as a request for the default.

**What a live adapter does when its backend is down.** It returns a typed error, the
collector records a degraded source, and the investigation continues with what it has. An
unreachable Prometheus never aborts a case: a partial investigation is worth more than
none. A `NaN`, a stale marker or an empty result becomes an *absent* sample rather than a
zero — a zero reading and no reading support different conclusions.

## 10. Troubleshooting

| Symptom | Cause and remedy |
|---|---|
| `fault case "X" is not in the catalog` | Run `arena cases` for the available identifiers |
| A case is stuck at `awaiting_approval` | That is the design. Approve or reject it in the console or through the decision endpoint |
| The console timeline stops updating | The stream reconnects on its own and resumes from its last sequence; reload if it does not |
| `case ... already exists` | The case identifier is derived from the alert, so the same alert reopens the same case. Change the alert's start time or use a different case |
| The demo stack starts but `faultctl` cannot reach the target | Check `docker compose ps`; `faultctl --target` must point at the published `order-api` port |
| `sddctl validate` reports stale items | An upstream specification changed. Run `sddctl drift` for the full impact set, update each artifact, then `sddctl seal` |
| `go test` fails on `TestPlaneDependencies` | A package imported across an architectural boundary. The failure names both packages and the rule |

## 11. How this repository is developed

Every artifact here — goals, requirements, architecture, designs, tasks, code and tests
— is linked in a single traceable chain, and the link is machine-checked. Changing a
goal makes every downstream artifact visibly stale until it is revisited.

```bash
make sdd-validate          # bilingual parity, traceability and drift
make sdd-impact ID=G-002   # what changing this goal would oblige you to revisit
make sdd-matrix            # regenerate the requirement-to-source matrix
```

The full process is documented in `docs/sdd-workflow.en.md`, and the rules it enforces
in `.sdd/memory/constitution.en.md`. Both exist in Chinese as well — that pairing is
itself one of the rules, and `sddctl lint` enforces it.
