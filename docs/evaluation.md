## Single agent versus multi agent

All three modes run over identical fixture inputs, so a difference is attributable to the flow rather than to the data.

| Mode | Samples | Top-1 accuracy | Top-3 coverage | Mean evidence kinds | Mean rounds | Critic corrections | Actions held for approval |
|---|---|---|---|---|---|---|---|
| single | 6 | 17% | 83% | 5.0 | 1.0 | 0 | 6 |
| multi_no_critic | 6 | 17% | 83% | 5.0 | 1.0 | 0 | 0 |
| multi_with_critic | 6 | 100% | 100% | 5.8 | 2.3 | 47 | 5 |

### Per case

| Case | Mode | Expected | Top-1 | Correct | Rounds | Kinds | Status |
|---|---|---|---|---|---|---|---|
| C1 | single | `sig-db-pool-exhaustion` | `sig-traffic-surge` | no | 1 | 5 | closed |
| C2 | single | `sig-misconfigured-dependency` | `sig-traffic-surge` | no | 1 | 5 | closed |
| C3 | single | `sig-slow-query` | `sig-traffic-surge` | no | 1 | 5 | closed |
| C4 | single | `sig-traffic-surge` | `sig-traffic-surge` | yes | 1 | 5 | closed |
| C5 | single | `sig-db-pool-exhaustion` | `sig-traffic-surge` | no | 1 | 5 | closed |
| C6 | single | `sig-misconfigured-dependency` | `sig-traffic-surge` | no | 1 | 5 | closed |
| C1 | multi_no_critic | `sig-db-pool-exhaustion` | `sig-traffic-surge` | no | 1 | 5 | closed |
| C2 | multi_no_critic | `sig-misconfigured-dependency` | `sig-traffic-surge` | no | 1 | 5 | closed |
| C3 | multi_no_critic | `sig-slow-query` | `sig-traffic-surge` | no | 1 | 5 | closed |
| C4 | multi_no_critic | `sig-traffic-surge` | `sig-traffic-surge` | yes | 1 | 5 | closed |
| C5 | multi_no_critic | `sig-db-pool-exhaustion` | `sig-traffic-surge` | no | 1 | 5 | closed |
| C6 | multi_no_critic | `sig-misconfigured-dependency` | `sig-traffic-surge` | no | 1 | 5 | closed |
| C1 | multi_with_critic | `sig-db-pool-exhaustion` | `sig-db-pool-exhaustion` | yes | 2 | 6 | closed |
| C2 | multi_with_critic | `sig-misconfigured-dependency` | `sig-misconfigured-dependency` | yes | 2 | 6 | closed |
| C3 | multi_with_critic | `sig-slow-query` | `sig-slow-query` | yes | 2 | 6 | closed |
| C4 | multi_with_critic | `sig-traffic-surge` | `sig-traffic-surge` | yes | 2 | 5 | closed |
| C5 | multi_with_critic | `sig-db-pool-exhaustion` | `sig-db-pool-exhaustion` | yes | 3 | 6 | closed |
| C6 | multi_with_critic | `sig-misconfigured-dependency` | `sig-misconfigured-dependency` | yes | 3 | 6 | closed |

The single-agent mode is given the same tools and the same default queries as the multi-agent flow; it differs only in performing one pass with no adversarial round. It is a model of "one context window, one look", not of a weaker toolset.
