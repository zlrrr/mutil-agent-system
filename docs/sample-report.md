# Root cause report — inc-445761e8c3

## Summary

| Field | Value |
|---|---|
| Alert | OrderApiHighErrorRate |
| Service | order-api |
| Severity | P1 |
| Window | 2026-07-26T10:00:00Z/2026-07-26T10:27:00Z |
| Mode | multi_with_critic |
| Status | closed |
| Collection rounds | 2 |
| Events | 113 |

5xx rate above 10% for 5 minutes; p99 latency above 2s

## Impact

- Candidate origin: order-api
- Also affected: checkout-web
- Depends on: postgres,payment-api

## Timeline

| Seq | Time | Actor | Event | Detail |
|---|---|---|---|---|
| 1 | 10:07:01 | orchestrator | case_created | OrderApiHighErrorRate raised for order-api (P1) |
| 2 | 10:07:02 | orchestrator | state_changed | created -> triaging: alert accepted |
| 3 | 10:07:04 | orchestrator | agent_completed | triage: order-api severity P1 over 2026-07-26T10:00:00Z/2026-07-26T10:27:00Z; plan: metrics, logs, change, topology, knowledge |
| 4 | 10:07:06 | orchestrator | round_started | collection round 1 of 3 |
| 5 | 10:07:07 | orchestrator | state_changed | triaging -> collecting: investigation plan prepared |
| 7 | 10:07:10 | metrics | agent_completed | metrics returned 4 contribution(s) |
| 9 | 10:07:14 | logs | agent_completed | logs returned 2 contribution(s) |
| 11 | 10:07:18 | change | agent_completed | change returned 1 contribution(s) |
| 13 | 10:07:22 | topology | agent_completed | topology returned 1 contribution(s) |
| 15 | 10:07:26 | knowledge | agent_completed | knowledge returned 2 contribution(s) |
| 21 | 10:07:33 | logs | evidence_truncated | tool result exceeded its bound and was truncated |
| 23 | 10:07:35 | logs | evidence_truncated | tool result exceeded its bound and was truncated |
| 28 | 10:07:40 | orchestrator | state_changed | collecting -> hypothesising: evidence collected |
| 30 | 10:07:43 | analysis | agent_completed | analysis returned 3 contribution(s) |
| 31 | 10:07:45 | analysis | hypothesis_proposed | A traffic increase exceeded the service's capacity (score 0.49) |
| 32 | 10:07:46 | analysis | hypothesis_proposed | A reduction of the database connection pool size exhausted the pool under load (score 0.40) |
| 33 | 10:07:47 | analysis | hypothesis_proposed | The database became unavailable (score 0.35) |
| 34 | 10:07:48 | orchestrator | state_changed | hypothesising -> criticising: hypotheses formed |
| 36 | 10:07:51 | critic | agent_completed | critic returned 11 contribution(s) |
| 37 | 10:07:53 | critic | evidence_demanded | the critic requires database connection pool saturation metric |
| 38 | 10:07:54 | critic | evidence_demanded | the critic requires configuration changes for the service in the 30 minutes before onset |
| 39 | 10:07:55 | critic | evidence_demanded | the critic requires database availability metric |
| 40 | 10:07:56 | critic | evidence_demanded | the critic requires historical peak traffic comparison at equal load |
| 41 | 10:07:57 | critic | critique_raised | [revise] this explanation depends on connection pool reached saturation, but no metric evidence establishes it; until it does, the claim rests on the remaining evidence alone |
| 42 | 10:07:58 | critic | critique_raised | [revise] this explanation depends on the pool size was changed shortly before onset, but no change evidence establishes it; until it does, the claim rests on the remaining evidence alone |
| 43 | 10:07:59 | critic | critique_raised | [revise] this explanation depends on database availability signal dropped, but no metric evidence establishes it; until it does, the claim rests on the remaining evidence alone |
| 44 | 10:08:00 | critic | critique_raised | [revise] "The database became unavailable" is also consistent with the evidence collected so far, and nothing yet separates it from this explanation; an available database rules out an outage and leaves the connection errors to be explained another way |
| 45 | 10:08:01 | critic | critique_raised | [revise] "A reduction of the database connection pool size exhausted the pool under load" is also consistent with the evidence collected so far, and nothing yet separates it from this explanation; pool saturation distinguishes an exhausted pool from a database that is simply slow or unreachable |
| 46 | 10:08:02 | critic | critique_raised | [accept_with_risk] this explanation implies no action whose effect could be measured, so accepting it cannot be confirmed by acting on it |
| 47 | 10:08:03 | critic | critique_raised | [revise] the leading explanation is only 0.09 ahead of "A reduction of the database connection pool size exhausted the pool under load", which is inside the 0.15 margin; the ranking is not yet meaningful |
| 48 | 10:08:04 | orchestrator | round_started | collection round 2 of 3 |
| 49 | 10:08:05 | orchestrator | state_changed | criticising -> collecting: the critic requires 4 further piece(s) of evidence |
| 51 | 10:08:08 | metrics | agent_completed | metrics returned 7 contribution(s) |
| 53 | 10:08:12 | logs | agent_completed | logs returned 2 contribution(s) |
| 55 | 10:08:16 | change | agent_completed | change returned 2 contribution(s) |
| 57 | 10:08:20 | topology | agent_completed | topology returned 1 contribution(s) |
| 59 | 10:08:24 | knowledge | agent_completed | knowledge returned 4 contribution(s) |
| 65 | 10:08:31 | metrics | demand_satisfied | the demand for database connection pool saturation metric is answered |
| 67 | 10:08:33 | metrics | demand_satisfied | the demand for database availability metric is answered |
| 69 | 10:08:35 | metrics | demand_satisfied | the demand for historical peak traffic comparison at equal load is answered |
| 71 | 10:08:37 | logs | evidence_truncated | tool result exceeded its bound and was truncated |
| 73 | 10:08:39 | logs | evidence_truncated | tool result exceeded its bound and was truncated |
| 76 | 10:08:42 | change | demand_satisfied | the demand for configuration changes for the service in the 30 minutes before onset is answered |
| 82 | 10:08:48 | orchestrator | state_changed | collecting -> hypothesising: evidence collected |
| 84 | 10:08:51 | analysis | agent_completed | analysis returned 3 contribution(s) |
| 88 | 10:08:56 | orchestrator | state_changed | hypothesising -> criticising: hypotheses formed |
| 90 | 10:08:59 | critic | agent_completed | critic returned 3 contribution(s) |
| 91 | 10:09:01 | critic | critique_raised | [accept_with_risk] this explanation implies no action whose effect could be measured, so accepting it cannot be confirmed by acting on it |
| 92 | 10:09:02 | critic | critique_raised | [accept] no rule found an unsupported step, an unexplained alternative or a timing conflict in this explanation |
| 93 | 10:09:03 | critic | critique_raised | [accept] no rule found an unsupported step, an unexplained alternative or a timing conflict in this explanation |
| 94 | 10:09:04 | orchestrator | state_changed | criticising -> remediating: the leading hypothesis meets the acceptance condition |
| 96 | 10:09:07 | remediation | agent_completed | remediation returned 1 contribution(s) |
| 97 | 10:09:09 | remediation | action_proposed | Restore the database connection pool size (medium risk) |
| 98 | 10:09:10 | executor | approval_requested | a human decision is required before this action can run |
| 99 | 10:09:11 | orchestrator | state_changed | remediating -> awaiting_approval: a human decision is required |
| 100 | 10:09:13 | operator | approval_recorded | arena-demo approved the action: approved by the demo command |
| 101 | 10:09:14 | orchestrator | state_changed | awaiting_approval -> executing: approval recorded for a-remediation-001 |
| 102 | 10:09:18 | executor | action_executed | Restore the database connection pool size: applied: order-api DB_POOL_SIZE: "2" -> "20" |
| 103 | 10:09:19 | orchestrator | state_changed | executing -> verifying: action attempted |
| 105 | 10:09:22 | verification | agent_completed | verification returned 4 contribution(s) |
| 107 | 10:09:25 | verification | verification_recorded | http_5xx_rate recovered after the action |
| 109 | 10:09:27 | verification | verification_recorded | http_request_duration_p99 recovered after the action |
| 110 | 10:09:28 | orchestrator | state_changed | verifying -> reporting: recovery verified |
| 111 | 10:09:29 | orchestrator | report_generated | root cause report generated |
| 112 | 10:09:30 | orchestrator | state_changed | reporting -> closed: investigation closed |
| 113 | 10:09:31 | orchestrator | case_closed | case closed |

## Accepted root cause

**A reduction of the database connection pool size exhausted the pool under load**

Confidence 0.94, verdict `accept`, signature `sig-db-pool-exhaustion`.

### Mechanism

With the pool reduced, concurrent requests exceed the available connections. Requests queue waiting for a connection, the wait exceeds the acquisition timeout, and the service returns 500s. Latency rises before errors appear because queueing precedes timing out.

### Score breakdown

| Term | Weight | Value | Contribution |
|---|---|---|---|
| metric_alignment | 0.30 | 1.00 | 0.3000 |
| log_alignment | 0.25 | 1.00 | 0.2500 |
| change_correlation | 0.20 | 1.00 | 0.2000 |
| topology_plausibility | 0.10 | 1.00 | 0.1000 |
| historical_similarity | 0.10 | 0.40 | 0.0400 |
| remediation_verifiability | 0.05 | 1.00 | 0.0500 |
| _counter-evidence penalty_ | | 0 unresolved | -0.0000 |
| **total** | | | **0.9400** |

### Evidence chain

| Evidence | Kind | Source | Summary | Query |
|---|---|---|---|---|
| `e-change-003` | change | deploy-history-fixture | order-api changed DB_POOL_SIZE from "20" to "2" at 10:05:30 (release-2026.07.26-1) | `deploy-history:order-api?lookback=30m` |
| `e-knowledge-004` | knowledge | runbooks | runbook "Database connection pool exhaustion" matches 2 of 5 declared symptoms (connection timeout, pool) | `runbook:rb-db-pool` |
| `e-logs-001` | log | logs | 162 lines matched the template "db connection timeout after #ms (attempt #)", first at 10:07:12 and last at 10:17:56 | `logs:order-api?template="db connection timeout after #ms (attempt #)"` |
| `e-metrics-009` | metric | prometheus-fixture | db_pool_saturation rose from a baseline of 20.00% to a peak of 100.00% starting at 10:06:00; it reached its declared capacity of 100.00% | `promql:db_pool_saturation{service="order-api"}` |
| `e-topology-001` | topology | topology | order-api is the earliest anomalous service in its neighbourhood; checkout-web became anomalous later | `topology:order-api` |

## Rejected alternatives

### The database became unavailable — scored 0.37

Signature `sig-db-outage`, verdict `accept_with_risk`.

Why it was not accepted:

- [coverage_gap] this explanation depends on database availability signal dropped, but no metric evidence establishes it; until it does, the claim rests on the remaining evidence alone
- [unverifiable_remediation] this explanation implies no action whose effect could be measured, so accepting it cannot be confirmed by acting on it
- [unverifiable_remediation] this explanation implies no action whose effect could be measured, so accepting it cannot be confirmed by acting on it
- Scored 0.57 below the accepted explanation

### A traffic increase exceeded the service's capacity — scored 0.29

Signature `sig-traffic-surge`, verdict `accept`.

Why it was not accepted:

- Counter-evidence `e-metrics-011`: order-api served an equal peak of 405 rps two days earlier with a 5xx rate of 0.20% and p99 latency of 195ms, so this load level alone has been handled without error
- [alternative_explanation] "The database became unavailable" is also consistent with the evidence collected so far, and nothing yet separates it from this explanation; an available database rules out an outage and leaves the connection errors to be explained another way
- [alternative_explanation] "A reduction of the database connection pool size exhausted the pool under load" is also consistent with the evidence collected so far, and nothing yet separates it from this explanation; pool saturation distinguishes an exhausted pool from a database that is simply slow or unreachable
- [close_call] the leading explanation is only 0.09 ahead of "A reduction of the database connection pool size exhausted the pool under load", which is inside the 0.15 margin; the ranking is not yet meaningful
- Scored 0.65 below the accepted explanation


## Adversarial review

| Critique | Hypothesis | Rule | Verdict | Challenge |
|---|---|---|---|---|
| `c-critic-001` | `h-analysis-002` | coverage_gap | revise | this explanation depends on connection pool reached saturation, but no metric evidence establishes it; until it does, the claim rests on the remaining evidence alone |
| `c-critic-002` | `h-analysis-002` | coverage_gap | revise | this explanation depends on the pool size was changed shortly before onset, but no change evidence establishes it; until it does, the claim rests on the remaining evidence alone |
| `c-critic-003` | `h-analysis-003` | coverage_gap | revise | this explanation depends on database availability signal dropped, but no metric evidence establishes it; until it does, the claim rests on the remaining evidence alone |
| `c-critic-004` | `h-analysis-001` | alternative_explanation | revise | "The database became unavailable" is also consistent with the evidence collected so far, and nothing yet separates it from this explanation; an available database rules out an outage and leaves the connection errors to be explained another way |
| `c-critic-005` | `h-analysis-001` | alternative_explanation | revise | "A reduction of the database connection pool size exhausted the pool under load" is also consistent with the evidence collected so far, and nothing yet separates it from this explanation; pool saturation distinguishes an exhausted pool from a database that is simply slow or unreachable |
| `c-critic-006` | `h-analysis-003` | unverifiable_remediation | accept_with_risk | this explanation implies no action whose effect could be measured, so accepting it cannot be confirmed by acting on it |
| `c-critic-007` | `h-analysis-001` | close_call | revise | the leading explanation is only 0.09 ahead of "A reduction of the database connection pool size exhausted the pool under load", which is inside the 0.15 margin; the ranking is not yet meaningful |
| `c-critic-008` | `h-analysis-003` | unverifiable_remediation | accept_with_risk | this explanation implies no action whose effect could be measured, so accepting it cannot be confirmed by acting on it |
| `c-critic-009` | `h-analysis-002` | no_objection | accept | no rule found an unsupported step, an unexplained alternative or a timing conflict in this explanation |
| `c-critic-010` | `h-analysis-001` | no_objection | accept | no rule found an unsupported step, an unexplained alternative or a timing conflict in this explanation |

### Evidence the critic demanded

| Demand | Kind | Answered by | Descriptor |
|---|---|---|---|
| `d-critic-001` | metric | `e-metrics-009` | database connection pool saturation metric |
| `d-critic-002` | change | `e-change-003` | configuration changes for the service in the 30 minutes before onset |
| `d-critic-003` | metric | `e-metrics-010` | database availability metric |
| `d-critic-004` | metric | `e-metrics-011` | historical peak traffic comparison at equal load |

## Remediation

### Restore the database connection pool size

- Risk: **medium**
- Call: `set_config key=DB_POOL_SIZE service=order-api value=20`
- Rollback: `set_config key=DB_POOL_SIZE service=order-api value=2`
- Verified by: http_5xx_rate, http_request_duration_p99
- Precondition: the change that reduced the pool is confirmed by change evidence
- Precondition: the database itself is reachable
- Rationale: Restoring the previous pool size removes the queueing that produces the timeouts, and is reversible in one step.
- Approval: **approved** by arena-demo at 2026-07-26T10:09:12Z — approved by the demo command
- Execution: **executed** in 1s — applied: order-api DB_POOL_SIZE: "2" -> "20"

## Recovery verification

| Signal | Before | After | Recovered |
|---|---|---|---|
| http_5xx_rate | 0.1292 | 0.0030 | yes |
| http_request_duration_p99 | 1623.2143 | 190.0000 | yes |

## Unmet evidence demands

_all evidence the critic demanded was collected_

## What the multi-agent flow contributed

- Collection rounds: **2**
- Evidence kinds gathered: **6** (metric, log, change, topology, knowledge, verification)
- Evidence items: **28**
- Critiques raised: **10**, of which **8** were not a plain accept
- Evidence demanded by the critic: **4**, answered: **4**
- Actions held at the approval gate: **1**

