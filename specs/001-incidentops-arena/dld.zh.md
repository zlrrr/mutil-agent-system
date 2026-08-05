---
id: DLD-DOC-001
lang: zh
counterpart: dld.en.md
doc_version: 1.0.0
status: approved
stage: dld
---

# IncidentOps Arena — 详细设计

## 1. 阅读指引

一个 DLD 条目是工程师实现、测试验证的最小单元。常量写在这里，不会出现在别处的正文散文中，
这样修改其中一个常量就能通过 `sddctl drift` 级联到读取它的代码。

下面每个条目都会以一个或多个 `// sdd:impl <ID>` 锚点出现在源码中。

## 2. 常量与配置

| 键 | 取值 | 作用 |
|---|---|---|
| `maxRounds` | 3 | 每个 case 的采集轮数（ARC-011） |
| `maxHypothesesPerRound` | 3 | 每轮保留的排序假设数 |
| `maxDemandsPerRound` | 4 | 带入下一轮的索证数 |
| `acceptThreshold` | 0.75 | 进入处置所需的最低领先分数 |
| `closeCallMargin` | 0.15 | 低于该差值即判定为分差过小 |
| `minEvidenceKinds` | 2 | 离开分析阶段所需的不同证据类别数 |
| `changeLookback` | 30m | 变更查询相对故障起点向前扩展的时长 |
| `anomalyFactor` | 3.0 | 峰值需超过基线的倍数 |
| `anomalySustain` | 2 | 确认起点所需的连续点数 |
| `coMovementWindow` | 60s | 起点相差在此范围内即判为同步变化 |
| `bounds.MaxRows` | 200 | 单次工具结果的最大行数 |
| `bounds.MaxChars` | 8000 | 单次工具结果的最大字符数 |
| `bounds.MaxSpan` | 2h | 单次工具查询的最大时间跨度 |
| `logSamplesPerCluster` | 3 | 每个簇保留的代表性日志行数 |
| `maxClusters` | 5 | 每次日志查询保留的簇数 |
| `maxEvidencePerCase` | 200 | 触发截断记录前的证据保留上限 |
| `subscriberBuffer` | 64 | 每个流订阅者的事件缓冲数 |

### 评分权重

| 项 | 权重 |
|---|---|
| `metric_alignment` | 0.30 |
| `log_alignment` | 0.25 |
| `change_correlation` | 0.20 |
| `topology_plausibility` | 0.10 |
| `historical_similarity` | 0.10 |
| `remediation_verifiability` | 0.05 |
| `counterEvidencePenalty`（每条未消解） | 0.20 |

权重之和为 1.00，由 TC-0022 断言。

## 3. 领域

<!-- sdd:item id=DLD-1001 stage=dld status=approved derives_from=HLD-001 -->
### DLD-1001 — 角色、标识符与时间窗

**文件。** `internal/domain/ids.go`

**类型。** `Role string`、`TimeWindow struct{ Start, End time.Time }`。

**行为。**

1. `CaseID(alert Alert) string` 返回 `"inc-"` 加上
   `sha256(alertName + "|" + service + "|" + startsAt.UTC().RFC3339)` 的前 10 位十六进制
   字符。不用时钟、不用随机数，因此同一条告警永远得到同一个 case 标识符。
2. `NewID(prefix string, role Role, seq int) string` 返回
   `fmt.Sprintf("%s-%s-%03d", prefix, role, seq)`。前缀：`e` 证据、`h` 假设、`c` 质疑、
   `d` 索证、`a` 动作。
3. `TimeWindow.Extend(d)` 返回 `Start` 前移 `d` 的窗口。
4. `TimeWindow.Contains(t)` 两端均为闭区间。

**不变量。** 标识符以 case 为作用域，并在多进程间稳定。

**检查点。** TC-0070 — 两次运行产生相同标识符。

<!-- sdd:item id=DLD-1002 stage=dld status=approved derives_from=HLD-001 -->
### DLD-1002 — 证据

**文件。** `internal/domain/evidence.go`

**类型。** `EvidenceKind` 及常量 `metric`、`log`、`change`、`topology`、`knowledge`、
`verification`；`Evidence` 如 HLD-001 所述，含 `Facts map[string]string`。

**行为。** `Evidence.Validate()` 要求 `ID` 非空、`Kind` 已知、`Source` 与 `RawRef` 非空，
且 `Confidence` 位于 `[0,1]`。`FactsSorted()` 返回按键排序的键值对，使渲染与哈希确定。

**错误。**

| 条件 | 返回错误 | 调用方预期 |
|---|---|---|
| 未知类别 | `ErrUnknownEvidenceKind` | 贡献被拒绝，记录事件 |
| 置信度越界 | `ErrConfidenceRange` | 贡献被拒绝 |
| `RawRef` 为空 | `ErrMissingRawRef` | 贡献被拒绝 — REQ-0002 |

**检查点。** TC-0002 — 没有原始引用的证据被拒绝。

<!-- sdd:item id=DLD-1003 stage=dld status=approved derives_from=HLD-001 -->
### DLD-1003 — 假设与分数拆解

**文件。** `internal/domain/hypothesis.go`

**类型。** `Hypothesis{ID, SignatureID, Claim, Mechanism string; Supporting, Counter
[]string; Resolved map[string]bool; Breakdown ScoreBreakdown; Status HypothesisStatus}`，
状态取 `proposed`、`challenged`、`accepted`、`rejected`。

**行为。** `SupportingKinds(ev map[string]Evidence) []EvidenceKind` 返回支持证据的排序去重
类别。`UnresolvedCounters()` 统计未标记为已消解的反证标识符数量。

**检查点。** TC-0021 — 没有支持标识符的假设非法。

<!-- sdd:item id=DLD-1004 stage=dld status=approved derives_from=HLD-001 -->
### DLD-1004 — 质疑与索证

**文件。** `internal/domain/critique.go`

**类型。** `Verdict` 及常量 `accept`、`accept_with_risk`、`revise`、`reject`；
`Critique{ID, HypothesisID, Rule, Category, Challenge string; Verdict Verdict; DemandIDs
[]string}`；`EvidenceDemand{ID, Descriptor, Reason string; Kind EvidenceKind; SatisfiedBy
string}`。

**行为。** `Verdict.Severity()` 映射为 `accept`=0、`accept_with_risk`=1、`revise`=2、
`reject`=3。`CombineVerdicts(vs ...Verdict) Verdict` 返回最严重者，空输入默认为 `accept`。

**不变量。** 只有 `Facts["demand"]` 等于某索证描述符的证据才能满足该索证。

**检查点。** TC-0030 — 每个被审查的假设都携带取自该集合的裁决。

<!-- sdd:item id=DLD-1005 stage=dld status=approved derives_from=HLD-001 -->
### DLD-1005 — 动作、风险与审批

**文件。** `internal/domain/action.go`

**类型。** `Risk` 取 `low`、`medium`、`high`；`Action{ID, Title, Tool string; Args
map[string]string; Risk Risk; Preconditions []string; Rollback *ActionCall; Verify
[]string; Approval *ApprovalDecision; Execution *ExecutionResult}`；
`ApprovalDecision{Decision, By, Comment string; At time.Time}`；
`ExecutionResult{Outcome, Detail string; Duration time.Duration}`。

**行为。** `Action.Validate()` 拒绝没有回滚且没有至少一个验证信号的 `medium` 或 `high`
风险动作（REQ-0040）。`Action.Fingerprint()` 对工具与排序后的参数做哈希；执行器将其与
审批时记录的指纹比对，以检测审批后的篡改。

**检查点。** TC-0040、TC-0043 — 被篡改的动作在执行时被拒绝。

<!-- sdd:item id=DLD-1006 stage=dld status=approved derives_from=HLD-002 -->
### DLD-1006 — Contribution 代数与角色能力

**文件。** `internal/domain/contribution.go`

**类型。** `ContributionKind` 取 `add_evidence`、`propose_hypothesis`、
`raise_critique`、`demand_evidence`、`propose_action`、`record_verification`、
`record_execution`；`Contribution` 如 HLD-002 所述。

**行为。**

1. `Validate()` 要求与 `Kind` 对应的载荷字段恰好非 nil、其余全部为 nil，随后委派给该载荷
   自身的 `Validate`。
2. `roleCapabilities` 是一张包级表：
   - 采集器（`metrics`、`logs`、`change`、`topology`、`knowledge`）→ `add_evidence`
   - `analysis` → `propose_hypothesis`
   - `critic` → `raise_critique`、`demand_evidence`
   - `remediation` → `propose_action`
   - `executor` → `record_execution`
   - `verification` → `add_evidence`、`record_verification`
   - `baseline` → `add_evidence`、`propose_hypothesis`、`propose_action`
3. `Role.MayEmit(k)` 查该表；未知角色什么都不能发出。

**不变量。** 除 `critic` 外没有角色可以发出 `raise_critique`；除 `remediation` 与
`baseline` 外没有角色可以发出 `propose_action`。

**检查点。** TC-0003 — 采集器提出假设时被拒绝。

<!-- sdd:item id=DLD-1007 stage=dld status=approved derives_from=HLD-003 -->
### DLD-1007 — 事件与 case 投影

**文件。** `internal/domain/event.go`、`internal/domain/case.go`

**类型。** `EventType` 常量：`case_created`、`state_changed`、`round_started`、
`agent_started`、`agent_completed`、`agent_failed`、`tool_called`、`evidence_added`、
`evidence_truncated`、`hypothesis_proposed`、`hypothesis_rejected`、`hypothesis_scored`、
`critique_raised`、`evidence_demanded`、`demand_satisfied`、`demand_unmet`、
`contribution_rejected`、`action_proposed`、`approval_requested`、`approval_recorded`、
`action_executed`、`action_refused`、`verification_recorded`、`budget_exhausted`、
`report_generated`、`case_closed`。`Event.Payload json.RawMessage` 为创建实体的事件携带
该实体。

**行为。** `(*Case).Apply(e)` 按类型分支，且只修改投影。`Replay(caseID, events)` 从空
case 折叠，并校验序号从 1 起连续。

**不变量。** 对每个 case，`Replay(events)` 等于实时投影（REQ-0003）。

**检查点。** TC-0050 — 重放相等性与报告相等性。

## 4. 存储

<!-- sdd:item id=DLD-1010 stage=dld status=approved derives_from=HLD-004 -->
### DLD-1010 — Store 端口与内存适配器

**文件。** `internal/store/store.go`、`internal/store/memory.go`

**行为。** 内存适配器持有 `map[string][]Event` 与一个有序 header 切片，由一把互斥锁保护。
`Append` 不分配任何东西——序号由编排器预先设定——并拒绝首序号不等于 `len(existing)+1` 的
批次。`Events(from)` 返回副本。

**错误。** `ErrCaseNotFound`、`ErrSequenceGap`。

**检查点。** TC-0004 — 序号断层被拒绝。

<!-- sdd:item id=DLD-1011 stage=dld status=approved derives_from=HLD-004 -->
### DLD-1011 — 文件适配器

**文件。** `internal/store/file.go`

**行为。** 每个 case 一份 `<root>/cases/<caseID>.jsonl`；每行一条 JSON 事件。`Append` 以
`O_APPEND|O_CREATE|O_WRONLY` 打开，写完全部行后 `Sync`。Case 索引在构造时扫描目录、读取
每个文件首行来重建，因此索引是派生的而非权威的（ADR-004）。

**错误。** 包装 I/O 错误；格式错误的行会让整次读取失败并给出行号。

**检查点。** TC-0005 — case 在存储重启后仍然存在。

## 5. 信号平面

<!-- sdd:item id=DLD-1020 stage=dld status=approved derives_from=HLD-005 -->
### DLD-1020 — 信号端口与上限

**文件。** `internal/signal/ports.go`、`internal/signal/bounds.go`

**行为。** `Bounds.ApplyRows`、`ApplyChars` 与 `ApplySpan` 执行截断并返回
`truncated bool`。`SignalSet` 把六个端口与上限打包，使 Agent 只接收一个值。

**不变量。** 没有适配器返回超出上限的结果；截断总会被报告，绝不静默（REQ-0016）。

**检查点。** TC-0016 — 超长结果被截断并标记。

<!-- sdd:item id=DLD-1021 stage=dld status=approved derives_from=HLD-005 -->
### DLD-1021 — 异常起点与同步变化

**文件。** `internal/signal/anomaly.go`

**算法。**

```
Analyse(series, window):
  pre        := 严格早于 window.Start 的点
  baseline   := median(pre)            // pre 为空时取 0
  peak       := window 内的最大值
  threshold  := max(baseline * anomalyFactor, baseline + epsilon)
  onset      := window 内第一个 t，使其后 anomalySustain 个点全部超过 threshold
  anomalous  := onset 存在
  saturated  := series.Capacity > 0 且 peak >= series.Capacity
  ratio      := peak / baseline        // baseline == 0 时报告为 "n/a"
```

当 `a` 与 `b` 均异常且 `|onset(a) - onset(b)| <= coMovementWindow` 时，`CoMoving(a, b)`
为真。

**复杂度。** 与点数成线性；除返回结构体外不额外分配。

**检查点。** TC-0011 — 起点与夹具阶跃相差不超过一个采样间隔。

<!-- sdd:item id=DLD-1022 stage=dld status=approved derives_from=HLD-005 -->
### DLD-1022 — 日志聚类

**文件。** `internal/signal/cluster.go`

**算法。**

```
Normalise(line):
  把连续数字替换为 "#"
  把引号包裹的片段替换为 '"…"'
  把长度 >= 8 的十六进制串替换为 "<id>"
Cluster(lines):
  key    := Normalise(message)
  按 key 分组，统计数量并跟踪首次/末次时间戳
  排序   按数量降序，再按首次出现升序，再按 key 升序
  保留   maxClusters 个簇，每簇 logSamplesPerCluster 条样本
```

**不变量。** 簇顺序是全序且确定的。

**检查点。** TC-0012 — 184 行与 3 行两组产生两个有序簇。

<!-- sdd:item id=DLD-1023 stage=dld status=approved derives_from=HLD-006 -->
### DLD-1023 — 夹具适配器

**文件。** `internal/signal/fixture/fixture.go`

**行为。** `NewFixtureSet(fc FaultCase)` 返回一个 `SignalSet`，其适配器只从样例定义读取
数据。指标适配器按序列名匹配查询；日志适配器按服务、窗口与子串过滤声明的日志行；变更
适配器按服务与窗口过滤声明的变更；拓扑与知识返回声明值。`DemandResponses` 由 `Kind`
匹配的适配器按描述符提供，且每条都带 `Facts["demand"] = descriptor`，使编排器可以标记该
索证已满足。

**不变量。** 每个返回值都来自样例文件；适配器不执行 I/O，也不持有时钟。

**检查点。** TC-0010 — 参考场景离线完成。

<!-- sdd:item id=DLD-1024 stage=dld status=approved derives_from=HLD-006 -->
### DLD-1024 — 模拟执行器

**文件。** `internal/signal/fixture/actuator.go`

**行为。** 持有一份由样例初始化的进程内配置映射。`Invoke` 支持 `set_config`、
`restart_service`、`scale_service` 与 `create_ticket`，记录调用，并返回描述状态变化的
结果。当某次 `set_config` 与样例的 `recoveryTrigger` 匹配后，后续指标查询返回样例的
`postRecovery` 序列——验证正是据此观察到恢复。

**检查点。** TC-0051 — 只有在动作之后验证才观察到恢复。

## 6. 目录与推理

<!-- sdd:item id=DLD-1030 stage=dld status=approved derives_from=HLD-006 -->
### DLD-1030 — 内嵌目录

**文件。** `internal/catalog/catalog.go`、`internal/catalog/data/*.json`

**类型。** `Catalog{Signatures []Signature; Cases []FaultCase; Runbooks []Runbook}`。

**行为。** `LoadCatalog()` 解析由 `//go:embed data` 内嵌的文件，校验签名与样例标识符唯一，
且每个样例的 `expectedSignature` 都存在。迭代顺序按文件名再按声明顺序，因此是确定的。

**错误。** 标识符重复或引用悬空会导致加载失败，进而导致启动失败——损坏的目录不得运行。

**检查点。** TC-0082 — 新增样例文件无需代码改动。

<!-- sdd:item id=DLD-1031 stage=dld status=approved derives_from=HLD-006 -->
### DLD-1031 — 故障签名模式与匹配器

**文件。** `internal/catalog/signature.go`

**类型。**

```go
type Pattern struct {
    Kind      domain.EvidenceKind
    Match     []string          // 全部需出现在摘要或事实值中，不区分大小写
    Saturated bool              // 为真时 Facts["saturated"] 必须为 "true"
}
type Signature struct {
    ID, Claim, Mechanism string
    Requires             []Pattern
    Discriminators       []Discriminator
    Remediation          *RemediationTemplate
    RunbookIDs           []string
}
```

**算法。**

```
MatchPattern(p, ev):  p.Match 中每个 s 都出现在 lower(ev.Summary + 事实值拼接) 中
                      且（p.Saturated 为假 或 ev.Facts["saturated"] == "true"）
Match(sig, evidence): 对每个模式，取按 id 顺序第一个匹配的证据
                      matched[kind] += 1 / requiredCount[kind]
                      当匹配模式数 >= 1 时，该签名视为有匹配
```

**不变量。** 匹配与顺序无关且不区分大小写；相同证据集合总产生相同匹配集合。

**检查点。** TC-0020 — 参考证据匹配到预期签名。

<!-- sdd:item id=DLD-1032 stage=dld status=approved derives_from=HLD-008 -->
### DLD-1032 — 评分

**文件。** `internal/reasoner/score.go`

**算法。**

```
term(metric_alignment)            = 该签名 metric 模式被匹配的比例
term(log_alignment)               = 该签名 log 模式被匹配的比例
term(change_correlation)          = 匹配到的变更时间戳 < 异常起点时为 1.0
                                    匹配到变更但起点未知时为 0.5
                                    否则为 0.0
term(topology_plausibility)       = 候选源头 1.0，仅受影响 0.4，被否证 0.0
term(historical_similarity)       = 最佳 runbook 匹配得分，位于 [0,1]
term(remediation_verifiability)   = 签名声明了带至少一个验证信号的处置时为 1.0，否则 0.0
total = Σ weight_i · value_i − counterEvidencePenalty · 未消解反证数
total 被夹到 [0, 1]
```

`Rank` 按总分降序、再按支持类别数降序、再按标识符升序排列。

**不变量。** 夹取之前，`Σ breakdown.Terms[i].Contribution − Penalty == Total`，误差在
`1e-9` 之内。

**检查点。** TC-0022、TC-0023、TC-0024。

<!-- sdd:item id=DLD-1033 stage=dld status=approved derives_from=HLD-007 -->
### DLD-1033 — 规则推理器：假设形成

**文件。** `internal/reasoner/rule.go`

**行为。**

1. 对每条签名，与快照中的证据做匹配。
2. 丢弃匹配模式数为零的签名。
3. 为每条存活签名构建一个假设，`Supporting` 设为所有贡献了非零项的证据标识符，
   `Counter` 设为 `Facts["counters"]` 中列出了该签名的证据标识符。
4. 评分、排序，返回前 `maxHypothesesPerRound` 个。
5. 当某签名此前已产生过假设时复用其标识符，使分数跨轮更新而不是重复产生。

**检查点。** TC-0020、TC-0072。

<!-- sdd:item id=DLD-1034 stage=dld status=approved derives_from=HLD-009 -->
### DLD-1034 — 质疑规则集合

**文件。** `internal/reasoner/critique.go`

**规则，每条均可独立测试。**

| 规则 | 触发条件 | 产出 |
|---|---|---|
| `coverage_gap` | 该假设签名的某个必需模式没有匹配到任何证据 | `revise` 质疑，外加携带该模式描述符的索证 |
| `alternative_explanation` | 另一条签名至少匹配一个模式，且其区分性证据未被满足 | 对领先假设发出点名该对手的 `revise` 质疑，外加对区分性证据的索证 |
| `temporal_order` | 该假设匹配到的变更时间戳不早于异常起点 | `reject` 质疑，并向该假设附加反证 |
| `source_vs_victim` | 拓扑证据指出某上游服务起点更早 | 点名该上游候选的 `revise` 质疑 |
| `unverifiable_remediation` | 签名未声明处置，或所声明处置没有验证信号 | `accept_with_risk` 质疑 |
| `close_call` | 前两名总分差 `< closeCallMargin` | 对领先者发出 `revise` 质疑，外加对领先者区分性证据的索证 |

**行为。** 规则按表中顺序在排序后的假设上运行。索证按描述符去重，按 `(规则序, 假设排名)`
排序，并截断到 `maxDemandsPerRound`。每个假设的裁决是其各条质疑的 `CombineVerdicts`；没有
招致任何质疑的假设获得显式 `accept`。

**关于 REQ-0032 的说明。** 规格允许在领先假设已被所有已采集类别支撑时跳过替代解释。本设计
采用上表中更强的触发条件——只要存在尚未被排除的对手，就一定发起挑战——它同时满足该需求及
其验收标准。

**检查点。** TC-0030 至 TC-0036，每条规则一个测试。

## 7. Agent

<!-- sdd:item id=DLD-1040 stage=dld status=approved derives_from=HLD-010 -->
### DLD-1040 — Agent 接口与采集器

**文件。** `internal/agent/agent.go`、`internal/agent/collectors.go`

**行为。** 每个采集器运行两趟：

1. **默认趟** — 该角色声明的查询：指标查询样例的 `defaultSeries`；日志以告警的错误词在
   故障窗口内查询；变更在故障窗口内查询，**不**做前向扩展；拓扑与知识查询受影响服务。
2. **索证趟** — 对每条 `Kind` 与本采集器类别匹配的未满足索证，向端口索取该描述符下注册的
   响应，并带上 `Facts["demand"]` 发出。

默认趟刻意不做前向扩展，正是这一点让第一轮不完整、让质疑者的索证真正产生后果；扩展只在
索证要求时才施加。

**不变量。** 采集器只发出 `add_evidence`。证据的 `RawRef` 指明端口与确切查询。

**检查点。** TC-0011、TC-0012、TC-0013、TC-0014、TC-0015、TC-0031。

<!-- sdd:item id=DLD-1041 stage=dld status=approved derives_from=HLD-010 -->
### DLD-1041 — 分析、质疑、处置与验证 Agent

**文件。** `internal/agent/analysis.go`、`critic.go`、`remediation.go`、`verification.go`

**行为。**

- **分析** 委派给 `Reasoner.Hypothesise`，把每个结果包装为 `propose_hypothesis` 贡献。
- **质疑** 委派给 `Reasoner.Critique`，发出 `raise_critique` 与 `demand_evidence`。它
  从不发出假设。
- **处置** 读取被接受假设所属签名的 `RemediationTemplate`，从匹配到的变更证据中实例化
  工具参数（例如 `DB_POOL_SIZE` 的原值），并发出一个 `propose_action`。
- **验证** 在窗口 `[executedAt, executedAt + 5m]` 内重新查询已执行动作 `Verify` 列表中
  的每个信号，并为每个信号同时发出 `verification` 类别的 `add_evidence` 与一个
  `record_verification` 贡献。

**检查点。** TC-0020、TC-0030、TC-0040、TC-0051。

<!-- sdd:item id=DLD-1042 stage=dld status=approved derives_from=HLD-010 -->
### DLD-1042 — 单 Agent 基线

**文件。** `internal/agent/baseline.go`

**行为。** 一个持有全部端口的 Agent。它执行全部五个采集器的默认趟各一次，然后形成假设，
再为领先假设提议一个动作——没有质疑、没有索证、没有第二轮。它是"一个上下文窗口、一趟"的
诚实模型：它拿到与多 Agent 流程完全相同的工具与默认查询，唯一差别就是缺少对抗轮。

**检查点。** TC-0080。

## 8. 策略与执行

<!-- sdd:item id=DLD-1050 stage=dld status=approved derives_from=HLD-011 -->
### DLD-1050 — 策略引擎

**文件。** `internal/policy/policy.go`

**行为。** `Evaluate(a)` 按以下顺序推进，在第一个判定处返回：

1. **类别拒绝** — 工具属于 `{shell, exec, sql, delete_resource, bulk_restart}`，或参数
   含 shell 元字符：拒绝，`Rule = "forbidden_category"`。该检查先于任何白名单，因此没有
   配置能够放行它。
2. **服务白名单** — 目标服务不在列表：拒绝。
3. **工具白名单** — 工具不在列表：拒绝。
4. **键白名单** — 对 `set_config`，键不在列表：拒绝。
5. **取值范围** — 对 `set_config`，数值超出该键声明的范围：拒绝。
6. **风险门** — 放行；`Risk != low` 时 `RequiresApproval` 为真。

**不变量。** 拒绝会指明产生它的规则。没有任何参数能绕过第 1 至 5 步。

**检查点。** TC-0041、TC-0042、TC-0044。

<!-- sdd:item id=DLD-1051 stage=dld status=approved derives_from=HLD-011 -->
### DLD-1051 — 执行器

**文件。** `internal/policy/executor.go`

**行为。**

1. 当动作没有记录在案的审批且风险不为 `low` 时，拒绝。
2. 重新计算 `Action.Fingerprint()` 并与审批时记录的指纹比对；不一致则以
   `tampered_after_approval` 拒绝。
3. 针对传入的动作重新执行 `Evaluate`；判定不允许则拒绝。
4. 调用执行器端口，并对调用计时。
5. 返回 `ExecutionResult`；拒绝返回 `action_refused` 结果而非错误，从而落入日志与报告。

**不变量。** 执行器持有进程内唯一的 `Actuator` 引用。

**检查点。** TC-0043、TC-0045。

## 9. 编排

<!-- sdd:item id=DLD-1060 stage=dld status=approved derives_from=HLD-012 -->
### DLD-1060 — 状态机

**文件。** `internal/orchestrator/machine.go`

**状态。** `created`、`triaging`、`collecting`、`hypothesising`、`criticising`、
`human_review`、`remediating`、`awaiting_approval`、`executing`、`verifying`、
`reporting`、`closed`。

**迁移。**

```
created        -> triaging
triaging       -> collecting
collecting     -> hypothesising
hypothesising  -> criticising
criticising    -> collecting        (存在未满足索证且 round < maxRounds)
criticising    -> human_review      (分差过小且 round == maxRounds)
criticising    -> remediating       (满足接受条件)
criticising    -> reporting         (无可接受假设且预算耗尽)
human_review   -> collecting | remediating | reporting
remediating    -> awaiting_approval (动作需要审批)
remediating    -> executing         (低风险且策略允许)
remediating    -> reporting         (无可提议动作)
awaiting_approval -> executing      (已批准)
awaiting_approval -> reporting      (已拒绝)
executing      -> verifying
verifying      -> collecting        (未恢复且 round < maxRounds)
verifying      -> reporting         (已恢复或预算耗尽)
reporting      -> closed
```

**行为。** `Legal(from, to) bool` 查表；非法迁移被拒绝，并以 `contribution_rejected`
记录所尝试的状态对。

**检查点。** TC-0060。

<!-- sdd:item id=DLD-1061 stage=dld status=approved derives_from=HLD-012 -->
### DLD-1061 — 贡献应用

**文件。** `internal/orchestrator/apply.go`

**行为。**

1. 传入的贡献按 `(roleOrder, agentLocalIndex)` 排序；`roleOrder` 为固定采集器顺序
   `metrics、logs、change、topology、knowledge`。
2. 逐条检查 `Role.MayEmit`，再检查 `Contribution.Validate`。失败则发出
   `contribution_rejected` 并丢弃该贡献。
3. 从该角色的序列计数器分配标识符。
4. 对携带 `Facts["demand"]` 的证据，标记对应索证已满足并发出 `demand_satisfied`。
5. 拒绝 `Supporting` 为空或引用未知标识符的假设，发出 `hypothesis_rejected`（REQ-0021）。
6. 通过存储追加产生的事件，并发布到分发器。

**不变量。** 并发采集器的完成顺序无法影响产生的标识符及其顺序。

**检查点。** TC-0003、TC-0021、TC-0061。

<!-- sdd:item id=DLD-1062 stage=dld status=approved derives_from=HLD-012 -->
### DLD-1062 — 推进守卫

**文件。** `internal/orchestrator/guard.go`

**行为。** `CanRemediate(c) (bool, string)` 要求同时满足：

1. 存在领先假设，且其裁决为 `accept` 或 `accept_with_risk`。
2. `Breakdown.Total >= acceptThreshold`。
3. `len(SupportingKinds) >= minEvidenceKinds`。
4. 当 case 中存在任何变更证据时，领先假设的支持集合必须包含变更证据。
5. 前两名差值不小于 `closeCallMargin`，或轮次预算已耗尽且该 case 已经过 `human_review`。

理由字符串指明第一个未满足的条件，并记录在该次迁移上。

**检查点。** TC-0021、TC-0035、TC-0036。

<!-- sdd:item id=DLD-1063 stage=dld status=approved derives_from=HLD-012 -->
### DLD-1063 — 引擎驱动

**文件。** `internal/orchestrator/engine.go`

**行为。** `Advance` 恰好执行一步状态迁移并返回。`Run` 反复调用 `Advance`，直到状态为终态
或 `awaiting_approval`，并设有 `maxRounds * 12` 步的硬上限，用于防御迁移表书写错误。采集器
通过基于 `sync.WaitGroup` 与按 Agent 下标的切片实现的等价 `errgroup` 并发运行，因此结果
落位顺序固定，与完成顺序无关。

**错误。** Agent 出错发出 `agent_failed`，本轮以其余 Agent 继续；case 不会被中止。

**检查点。** TC-0060、TC-0061、TC-0073。

<!-- sdd:item id=DLD-1064 stage=dld status=approved derives_from=HLD-013 -->
### DLD-1064 — 事件分发器

**文件。** `internal/eventbus/broker.go`

**行为。** `Subscribe(caseID, buffer)` 注册一个通道并连同取消函数返回。`Publish` 对每个
订阅者执行非阻塞发送；缓冲写满时关闭并注销该订阅者。订阅者存放在以单调递增订阅标识符为键的
映射中，因此迭代顺序稳定。

**不变量。** `Publish` 永不阻塞，也不会因订阅者已关闭而 panic。

**检查点。** TC-0064。

## 10. 报告、API 与控制台

<!-- sdd:item id=DLD-1070 stage=dld status=approved derives_from=HLD-016 -->
### DLD-1070 — 报告渲染

**文件。** `internal/report/report.go`

**行为。** `Markdown(c)` 按固定顺序输出：摘要、影响、时间线、被接受的根因及其证据链、
分数拆解表、被否决的替代假设及理由、质疑及其消解、动作及其审批与执行状态、验证结果、
未满足的索证，以及一段说明轮数与质疑者纠正假设数量的多 Agent 价值备注。缺失的小节渲染为
`_none recorded_`，而不是被省略。

**不变量。** 渲染是投影的纯函数；不用时钟，不做未排序的 map 迭代。

**检查点。** TC-0050、TC-0053。

<!-- sdd:item id=DLD-1071 stage=dld status=approved derives_from=HLD-014 -->
### DLD-1071 — HTTP API

**文件。** `internal/httpapi/api.go`

**行为。** 路由使用 `http.ServeMux` 的 Go 1.22 方法与通配模式。处理器以
`DisallowUnknownFields` 解码，校验必填字段，失败时返回 `400` 与
`{"error": "...", "field": "..."}`。`POST /api/cases` 创建 case 并运行到首个停顿点，然后
返回投影。集合在编码前按标识符排序。

**检查点。** TC-0063。

<!-- sdd:item id=DLD-1072 stage=dld status=approved derives_from=HLD-014 -->
### DLD-1072 — 事件流

**文件。** `internal/httpapi/stream.go`

**行为。** `GET /api/cases/{id}/stream` 设置 `text/event-stream`，先从
`Last-Event-ID + 1`（或 `from` 查询参数）重放已存事件，再从订阅转发实时事件，写出 `id:`
与 `data:` 帧并在每帧后 flush。客户端断开时返回。

**不变量。** 跨重连时，客户端按序号顺序恰好一次地看到每条事件。

**检查点。** TC-0064。

<!-- sdd:item id=DLD-1073 stage=dld status=approved derives_from=HLD-015 -->
### DLD-1073 — 控制台

**文件。** `internal/httpapi/console.go`、`web/*.html`、`web/*.css`、`web/*.js`

**行为。** 模板从 `embed.FS` 一次性解析。`GET /` 列出 case；`GET /cases/{id}` 渲染工作台。
实时区域订阅事件流并追加时间线行；审批控件提交决定后重新加载。无框架，无构建步骤。

**检查点。** TC-0065。

<!-- sdd:item id=DLD-1074 stage=dld status=approved derives_from=HLD-017 -->
### DLD-1074 — 评估运行器

**文件。** `internal/eval/eval.go`

**行为。** 对每个样例与每种模式，基于内存存储与夹具信号集合构建全新引擎，运行至结束
（评估模式下自动批准动作以便流程走完），并记录：首位假设的签名是否等于样例的
`expectedSignature`、它是否出现在前三名、采集到的不同证据类别、质疑者改变了分数或状态的
假设数量，以及因等待审批而被拦截的动作数量。`Summarise` 按模式聚合，并始终在每个比率旁
给出样本量。

**检查点。** TC-0080、TC-0081。

<!-- sdd:item id=DLD-1075 stage=dld status=approved derives_from=HLD-018 -->
### DLD-1075 — 入口

**文件。** `cmd/arena/main.go`、`cmd/evalctl/main.go`、`cmd/faultctl/main.go`

**行为。** `arena serve --addr --profile --store --data` 启动 HTTP 服务；
`arena demo --case C1` 运行样例至结束并打印报告；`evalctl run --out` 写出对比报告；
`faultctl inject|restore|status --case` 驱动演示栈的故障注入。配置优先级为参数、环境变量、
默认值。

**检查点。** TC-0070、TC-0081。

## 11. 参考场景的算术

以下是样例 `C1` 的预期行为，也是测试所断言的数字。

**第 1 轮 — 仅默认查询。** 采集到：`http_5xx_rate`（0.002 → 0.18，起点 10:07）、
`http_request_duration_p99`（180ms → 2200ms，起点 10:07）、`http_requests_total`
（120 → 260 rps，起点 10:03）、日志簇 `db connection timeout after #ms`（184 行）、
指明 `order-api` 为候选源头的拓扑，以及一条 runbook 匹配。变更采集器的默认趟只覆盖故障
窗口，因此 10:05:30 的那次发布**没有**被发现。

| 假设 | metric | log | change | topo | hist | verif | 总分 |
|---|---|---|---|---|---|---|---|
| `sig-traffic-surge` | 0.30 | 0.00 | 0.00 | 0.10 | 0.02 | 0.05 | **0.47** |
| `sig-db-pool-exhaustion` | 0.00 | 0.25 | 0.00 | 0.10 | 0.06 | 0.05 | **0.46** |
| `sig-db-outage` | 0.00 | 0.25 | 0.00 | 0.10 | 0.03 | 0.05 | **0.43** |

领先假设是**错的**，前两名差值为 0.01——低于 `closeCallMargin`——且领先者低于
`acceptThreshold`。`close_call`、`alternative_explanation` 与 `coverage_gap` 全部触发，
产生对连接池饱和度指标、`changeLookback` 范围内的配置变更历史，以及等负载历史峰值对比的
索证。

**第 2 轮 — 索证驱动的查询。** `db_pool_in_use` 返回饱和（容量 2，峰值 2，起点 10:06）；
扩展窗口内的变更查询返回 10:05:30 的 `DB_POOL_SIZE 20 → 2`；流量对比返回两天前的等负载
时段且当时无错误，并携带 `Facts["counters"] = "sig-traffic-surge"`。

| 假设 | metric | log | change | topo | hist | verif | 惩罚 | 总分 |
|---|---|---|---|---|---|---|---|---|
| `sig-db-pool-exhaustion` | 0.30 | 0.25 | 0.20 | 0.10 | 0.06 | 0.05 | 0.00 | **0.96** |
| `sig-db-outage` | 0.00 | 0.25 | 0.00 | 0.10 | 0.03 | 0.05 | 0.00 | **0.43** |
| `sig-traffic-surge` | 0.30 | 0.00 | 0.00 | 0.10 | 0.02 | 0.05 | 0.20 | **0.27** |

排序发生了变化，且首位现在是正确的。时序成立（10:05:30 < 10:06），全部索证已满足，覆盖
四个类别，总分越过阈值——因此裁决为 `accept`，case 推进到处置阶段，动作为
`set_config order-api DB_POOL_SIZE 20`，风险 `medium`，回滚到 `2`，验证
`http_5xx_rate` 与 `http_request_duration_p99`。

**这演示了什么。** 单 Agent 模式与无质疑模式都停在第 1 轮，报告"流量突增"——那个看似成立的
第一个故事。只有对抗流程抵达了配置变更。这正是评估所计算的对比，而且它是流程的性质而非
数据的性质，因为三种模式看到的是同一份夹具。

## 12. 实现顺序

| 批次 | DLD 条目 | 理由 |
|---|---|---|
| 1 | DLD-1001..1007 | 领域类型；其余一切都要对着它们编译 |
| 2 | DLD-1010、DLD-1011、DLD-1064 | 存储与分发器；除 domain 外无依赖 |
| 3 | DLD-1020..1024、DLD-1030、DLD-1031 | 信号平面与目录；使夹具可用 |
| 4 | DLD-1032..1034 | 推理；仅凭夹具证据即可测试 |
| 5 | DLD-1040..1042、DLD-1050、DLD-1051 | Agent 与策略 |
| 6 | DLD-1060..1063 | 编排；首次端到端运行 |
| 7 | DLD-1070..1075 | 报告、API、控制台、评估、命令 |
