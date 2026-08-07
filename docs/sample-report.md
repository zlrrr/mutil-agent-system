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
| Events | 115 |

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
| 7 | 10:07:10 | metrics | agent_completed | metrics returned 5 contribution(s) |
| 9 | 10:07:14 | logs | agent_completed | logs returned 2 contribution(s) |
| 11 | 10:07:18 | change | agent_completed | change returned 1 contribution(s) |
| 13 | 10:07:22 | topology | agent_completed | topology returned 1 contribution(s) |
| 15 | 10:07:26 | knowledge | agent_completed | knowledge returned 2 contribution(s) |
| 22 | 10:07:34 | logs | evidence_truncated | tool result exceeded its bound and was truncated |
| 24 | 10:07:36 | logs | evidence_truncated | tool result exceeded its bound and was truncated |
| 29 | 10:07:41 | orchestrator | state_changed | collecting -> hypothesising: evidence collected |
| 31 | 10:07:44 | analysis | agent_completed | analysis returned 3 contribution(s) |
| 32 | 10:07:46 | analysis | hypothesis_proposed | The database became unavailable (score 0.65) |
| 33 | 10:07:47 | analysis | hypothesis_proposed | A traffic increase exceeded the service's capacity (score 0.49) |
| 34 | 10:07:48 | analysis | hypothesis_proposed | A reduction of the database connection pool size exhausted the pool under load (score 0.40) |
| 35 | 10:07:49 | orchestrator | state_changed | hypothesising -> criticising: hypotheses formed |
| 37 | 10:07:52 | critic | agent_completed | critic returned 11 contribution(s) |
| 38 | 10:07:54 | critic | evidence_demanded | the critic requires database connection pool saturation metric |
| 39 | 10:07:55 | critic | evidence_demanded | the critic requires configuration changes for the service in the 30 minutes before onset |
| 40 | 10:07:56 | critic | evidence_demanded | the critic requires error rate duration compared with the database recovery time |
| 41 | 10:07:57 | critic | evidence_demanded | the critic requires historical peak traffic comparison at equal load |
| 42 | 10:07:58 | critic | critique_raised | [revise] this explanation depends on connection pool reached saturation, but no metric evidence establishes it; until it does, the claim rests on the remaining evidence alone |
| 43 | 10:07:59 | critic | critique_raised | [revise] this explanation depends on the pool size was changed shortly before onset, but no change evidence establishes it; until it does, the claim rests on the remaining evidence alone |
| 44 | 10:08:00 | critic | critique_raised | [revise] "A reduction of the database connection pool size exhausted the pool under load" is also consistent with the evidence collected so far, and nothing yet separates it from this explanation; pool saturation distinguishes an exhausted pool from a database that is simply slow or unreachable |
| 45 | 10:08:01 | critic | critique_raised | [revise] "A reduction of the database connection pool size exhausted the pool under load" is also consistent with the evidence collected so far, and nothing yet separates it from this explanation; errors that outlast the database's own unavailability cannot be explained by that unavailability, however convincing the coincidence looked |
| 46 | 10:08:02 | critic | critique_raised | [revise] "A traffic increase exceeded the service's capacity" is also consistent with the evidence collected so far, and nothing yet separates it from this explanation; if an equal or greater load was served without error previously, load alone does not explain the failure |
| 47 | 10:08:03 | critic | critique_raised | [accept_with_risk] this explanation implies no action whose effect could be measured, so accepting it cannot be confirmed by acting on it |
| 48 | 10:08:04 | critic | critique_raised | [accept] no rule found an unsupported step, an unexplained alternative or a timing conflict in this explanation |
| 49 | 10:08:05 | orchestrator | round_started | collection round 2 of 3 |
| 50 | 10:08:06 | orchestrator | state_changed | criticising -> collecting: the critic requires 4 further piece(s) of evidence |
| 52 | 10:08:09 | metrics | agent_completed | metrics returned 8 contribution(s) |
| 54 | 10:08:13 | logs | agent_completed | logs returned 2 contribution(s) |
| 56 | 10:08:17 | change | agent_completed | change returned 2 contribution(s) |
| 58 | 10:08:21 | topology | agent_completed | topology returned 1 contribution(s) |
| 60 | 10:08:25 | knowledge | agent_completed | knowledge returned 4 contribution(s) |
| 67 | 10:08:33 | metrics | demand_satisfied | the demand for database connection pool saturation metric is answered |
| 69 | 10:08:35 | metrics | demand_satisfied | the demand for error rate duration compared with the database recovery time is answered |
| 71 | 10:08:37 | metrics | demand_satisfied | the demand for historical peak traffic comparison at equal load is answered |
| 73 | 10:08:39 | logs | evidence_truncated | tool result exceeded its bound and was truncated |
| 75 | 10:08:41 | logs | evidence_truncated | tool result exceeded its bound and was truncated |
| 78 | 10:08:44 | change | demand_satisfied | the demand for configuration changes for the service in the 30 minutes before onset is answered |
| 84 | 10:08:50 | orchestrator | state_changed | collecting -> hypothesising: evidence collected |
| 86 | 10:08:53 | analysis | agent_completed | analysis returned 3 contribution(s) |
| 90 | 10:08:58 | orchestrator | state_changed | hypothesising -> criticising: hypotheses formed |
| 92 | 10:09:01 | critic | agent_completed | critic returned 3 contribution(s) |
| 93 | 10:09:03 | critic | critique_raised | [accept_with_risk] this explanation implies no action whose effect could be measured, so accepting it cannot be confirmed by acting on it |
| 94 | 10:09:04 | critic | critique_raised | [accept] no rule found an unsupported step, an unexplained alternative or a timing conflict in this explanation |
| 95 | 10:09:05 | critic | critique_raised | [accept] no rule found an unsupported step, an unexplained alternative or a timing conflict in this explanation |
| 96 | 10:09:06 | orchestrator | state_changed | criticising -> remediating: the leading hypothesis meets the acceptance condition |
| 98 | 10:09:09 | remediation | agent_completed | remediation returned 1 contribution(s) |
| 99 | 10:09:11 | remediation | action_proposed | Restore the database connection pool size (medium risk) |
| 100 | 10:09:12 | executor | approval_requested | a human decision is required before this action can run |
| 101 | 10:09:13 | orchestrator | state_changed | remediating -> awaiting_approval: a human decision is required |
| 102 | 10:09:15 | operator | approval_recorded | arena-demo approved the action: approved by the demo command |
| 103 | 10:09:16 | orchestrator | state_changed | awaiting_approval -> executing: approval recorded for a-remediation-001 |
| 104 | 10:09:20 | executor | action_executed | Restore the database connection pool size: applied: order-api DB_POOL_SIZE: "2" -> "20" |
| 105 | 10:09:21 | orchestrator | state_changed | executing -> verifying: action attempted |
| 107 | 10:09:24 | verification | agent_completed | verification returned 4 contribution(s) |
| 109 | 10:09:27 | verification | verification_recorded | http_5xx_rate recovered after the action |
| 111 | 10:09:29 | verification | verification_recorded | http_request_duration_p99 recovered after the action |
| 112 | 10:09:30 | orchestrator | state_changed | verifying -> reporting: recovery verified |
| 113 | 10:09:31 | orchestrator | report_generated | root cause report generated |
| 114 | 10:09:32 | orchestrator | state_changed | reporting -> closed: investigation closed |
| 115 | 10:09:33 | orchestrator | case_closed | case closed |

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
| `e-metrics-011` | metric | prometheus-fixture | db_pool_saturation rose from a baseline of 20.00% to a peak of 100.00% starting at 10:06:00; it reached its declared capacity of 100.00% | `promql:db_pool_saturation{service="order-api"}` |
| `e-topology-001` | topology | topology | order-api is the earliest anomalous service in its neighbourhood; checkout-web became anomalous later | `topology:order-api` |

## Rejected alternatives

### The database became unavailable — scored 0.47

Signature `sig-db-outage`, verdict `accept_with_risk`.

Why it was not accepted:

- Counter-evidence `e-metrics-012`: the database availability signal returned at 10:09:00 and stayed up, while the elevated error rate continued until 10:27:00 — 18 further minutes of failures with the database available throughout
- [alternative_explanation] "A reduction of the database connection pool size exhausted the pool under load" is also consistent with the evidence collected so far, and nothing yet separates it from this explanation; pool saturation distinguishes an exhausted pool from a database that is simply slow or unreachable
- [alternative_explanation] "A reduction of the database connection pool size exhausted the pool under load" is also consistent with the evidence collected so far, and nothing yet separates it from this explanation; errors that outlast the database's own unavailability cannot be explained by that unavailability, however convincing the coincidence looked
- [alternative_explanation] "A traffic increase exceeded the service's capacity" is also consistent with the evidence collected so far, and nothing yet separates it from this explanation; if an equal or greater load was served without error previously, load alone does not explain the failure
- [unverifiable_remediation] this explanation implies no action whose effect could be measured, so accepting it cannot be confirmed by acting on it
- [unverifiable_remediation] this explanation implies no action whose effect could be measured, so accepting it cannot be confirmed by acting on it
- Scored 0.47 below the accepted explanation

### A traffic increase exceeded the service's capacity — scored 0.29

Signature `sig-traffic-surge`, verdict `accept`.

Why it was not accepted:

- Counter-evidence `e-metrics-013`: order-api served an equal peak of 405 rps two days earlier with a 5xx rate of 0.20% and p99 latency of 195ms, so this load level alone has been handled without error
- Scored 0.65 below the accepted explanation


## Adversarial review

| Critique | Hypothesis | Rule | Verdict | Challenge |
|---|---|---|---|---|
| `c-critic-001` | `h-analysis-003` | coverage_gap | revise | this explanation depends on connection pool reached saturation, but no metric evidence establishes it; until it does, the claim rests on the remaining evidence alone |
| `c-critic-002` | `h-analysis-003` | coverage_gap | revise | this explanation depends on the pool size was changed shortly before onset, but no change evidence establishes it; until it does, the claim rests on the remaining evidence alone |
| `c-critic-003` | `h-analysis-001` | alternative_explanation | revise | "A reduction of the database connection pool size exhausted the pool under load" is also consistent with the evidence collected so far, and nothing yet separates it from this explanation; pool saturation distinguishes an exhausted pool from a database that is simply slow or unreachable |
| `c-critic-004` | `h-analysis-001` | alternative_explanation | revise | "A reduction of the database connection pool size exhausted the pool under load" is also consistent with the evidence collected so far, and nothing yet separates it from this explanation; errors that outlast the database's own unavailability cannot be explained by that unavailability, however convincing the coincidence looked |
| `c-critic-005` | `h-analysis-001` | alternative_explanation | revise | "A traffic increase exceeded the service's capacity" is also consistent with the evidence collected so far, and nothing yet separates it from this explanation; if an equal or greater load was served without error previously, load alone does not explain the failure |
| `c-critic-006` | `h-analysis-001` | unverifiable_remediation | accept_with_risk | this explanation implies no action whose effect could be measured, so accepting it cannot be confirmed by acting on it |
| `c-critic-007` | `h-analysis-002` | no_objection | accept | no rule found an unsupported step, an unexplained alternative or a timing conflict in this explanation |
| `c-critic-008` | `h-analysis-001` | unverifiable_remediation | accept_with_risk | this explanation implies no action whose effect could be measured, so accepting it cannot be confirmed by acting on it |
| `c-critic-009` | `h-analysis-003` | no_objection | accept | no rule found an unsupported step, an unexplained alternative or a timing conflict in this explanation |
| `c-critic-010` | `h-analysis-002` | no_objection | accept | no rule found an unsupported step, an unexplained alternative or a timing conflict in this explanation |

### Evidence the critic demanded

| Demand | Kind | Answered by | Descriptor |
|---|---|---|---|
| `d-critic-001` | metric | `e-metrics-011` | database connection pool saturation metric |
| `d-critic-002` | change | `e-change-003` | configuration changes for the service in the 30 minutes before onset |
| `d-critic-003` | metric | `e-metrics-012` | error rate duration compared with the database recovery time |
| `d-critic-004` | metric | `e-metrics-013` | historical peak traffic comparison at equal load |

## Remediation

### Restore the database connection pool size

- Risk: **medium**
- Call: `set_config key=DB_POOL_SIZE service=order-api value=20`
- Rollback: `set_config key=DB_POOL_SIZE service=order-api value=2`
- Verified by: http_5xx_rate, http_request_duration_p99
- Precondition: the change that reduced the pool is confirmed by change evidence
- Precondition: the database itself is reachable
- Rationale: Restoring the previous pool size removes the queueing that produces the timeouts, and is reversible in one step.
- Approval: **approved** by arena-demo at 2026-07-26T10:09:14Z — approved by the demo command
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
- Evidence items: **30**
- Critiques raised: **10**, of which **7** were not a plain accept
- Evidence demanded by the critic: **4**, answered: **4**
- Actions held at the approval gate: **1**

