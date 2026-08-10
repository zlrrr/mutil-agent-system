## Single agent versus multi agent

All three modes run over identical fixture inputs, so a difference is attributable to the flow rather than to the data.

| Reasoner | Planner | Mode | Samples | Top-1 accuracy | Top-3 coverage | Mean evidence kinds | Mean rounds | Critic corrections | Actions held for approval |
|---|---|---|---|---|---|---|---|---|---|
| `rule` | `rule` | single | 6 | 17% | 83% | 5.2 | 1.0 | 0 | 4 |
| `rule` | `rule` | multi_no_critic | 6 | 17% | 83% | 5.2 | 1.0 | 0 | 1 |
| `rule` | `rule` | multi_with_critic | 6 | 100% | 100% | 6.0 | 2.3 | 54 | 6 |

### Per case

| Case | Reasoner | Planner | Mode | Expected | Top-1 | Correct | Rounds | Kinds | Status |
|---|---|---|---|---|---|---|---|---|---|
| C1 | `rule` | `rule` | single | `sig-db-pool-exhaustion` | `sig-db-outage` | no | 1 | 5 | closed |
| C2 | `rule` | `rule` | single | `sig-misconfigured-dependency` | `sig-upstream-outage` | no | 1 | 5 | closed |
| C3 | `rule` | `rule` | single | `sig-slow-query` | `sig-cold-cache` | no | 1 | 5 | closed |
| C4 | `rule` | `rule` | single | `sig-traffic-surge` | `sig-traffic-surge` | yes | 1 | 6 | closed |
| C5 | `rule` | `rule` | single | `sig-db-pool-exhaustion` | `sig-traffic-surge` | no | 1 | 5 | closed |
| C6 | `rule` | `rule` | single | `sig-misconfigured-dependency` | `sig-traffic-surge` | no | 1 | 5 | closed |
| C1 | `rule` | `rule` | multi_no_critic | `sig-db-pool-exhaustion` | `sig-db-outage` | no | 1 | 5 | closed |
| C2 | `rule` | `rule` | multi_no_critic | `sig-misconfigured-dependency` | `sig-upstream-outage` | no | 1 | 5 | closed |
| C3 | `rule` | `rule` | multi_no_critic | `sig-slow-query` | `sig-cold-cache` | no | 1 | 5 | closed |
| C4 | `rule` | `rule` | multi_no_critic | `sig-traffic-surge` | `sig-traffic-surge` | yes | 1 | 6 | closed |
| C5 | `rule` | `rule` | multi_no_critic | `sig-db-pool-exhaustion` | `sig-traffic-surge` | no | 1 | 5 | closed |
| C6 | `rule` | `rule` | multi_no_critic | `sig-misconfigured-dependency` | `sig-traffic-surge` | no | 1 | 5 | closed |
| C1 | `rule` | `rule` | multi_with_critic | `sig-db-pool-exhaustion` | `sig-db-pool-exhaustion` | yes | 2 | 6 | closed |
| C2 | `rule` | `rule` | multi_with_critic | `sig-misconfigured-dependency` | `sig-misconfigured-dependency` | yes | 2 | 6 | closed |
| C3 | `rule` | `rule` | multi_with_critic | `sig-slow-query` | `sig-slow-query` | yes | 2 | 6 | closed |
| C4 | `rule` | `rule` | multi_with_critic | `sig-traffic-surge` | `sig-traffic-surge` | yes | 2 | 6 | closed |
| C5 | `rule` | `rule` | multi_with_critic | `sig-db-pool-exhaustion` | `sig-db-pool-exhaustion` | yes | 3 | 6 | closed |
| C6 | `rule` | `rule` | multi_with_critic | `sig-misconfigured-dependency` | `sig-misconfigured-dependency` | yes | 3 | 6 | closed |

The single-agent mode is given the same tools and the same default queries as the multi-agent flow; it differs only in performing one pass with no adversarial round. It is a model of "one context window, one look", not of a weaker toolset.
