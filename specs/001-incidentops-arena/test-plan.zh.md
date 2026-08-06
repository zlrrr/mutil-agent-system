---
id: TESTPLAN-001
lang: zh
counterpart: test-plan.en.md
doc_version: 1.0.0
status: approved
stage: verify
---

# IncidentOps Arena — 测试计划

## 1. 策略

| 层级 | 范围 | 执行器 | 是否必须离线可跑 |
|---|---|---|---|
| unit | 针对构造值检验单个函数或类型 | `go test ./internal/...` | 是 |
| integration | 两个及以上模块经由真实接口协作 | `go test ./internal/...` | 是 |
| e2e | 从告警到关闭报告的完整 case | `go test ./internal/orchestrator` | 是 |
| policy | 在对抗性输入下必须成立的安全性质 | `go test ./internal/policy` | 是 |
| governance | 仓库自身的性质 | `go test ./internal/sdd` | 是 |
| delivery | 交付制品及其文档 | `go test ./internal/...`、`make image` | 仅构建 |

除 `delivery` 外的每一层都在无网络环境下运行。没有任何测试会启动容器、向外部主机开套接字，
或读取凭据。

## 2. 准入与退出条件

**准入。** 某测试所验证的 DLD 条目状态为 `approved` 且已封存，且其测试用例先于代码写出。

**退出（M1 门禁）。** 全部满足：

1. `go build ./...` 与 `go vet ./...` 无告警。
2. `go test ./...` 在 `-race` 下离线通过。
3. 每条 P0 需求至少有一个通过的测试（`sddctl gate --stage deliver`）。
4. `sddctl validate` 无 error 且无漂移。
5. 参考场景在两个进程中产生逐字节一致的报告。

## 3. 测试用例

### 3.1 领域契约

<!-- sdd:item id=TC-0001 stage=verify status=approved derives_from=REQ-0001 -->
#### TC-0001 — Agent 不修改 case 状态

**层级。** unit。**验证。** REQ-0001。

**步骤。** 对一个 case 快照，运行每一种 Agent，并按深度相等比较运行前后的 case。

**预期。** case 保持不变；每个 Agent 的唯一效果就是它返回的 contribution。

**测试函数。** `internal/agent/agent_test.go` 中的 `TestAgentsArePure`

<!-- sdd:item id=TC-0002 stage=verify status=approved derives_from=REQ-0002 -->
#### TC-0002 — 证据校验与不可变性

**层级。** unit。**验证。** REQ-0002。

**步骤。** 分别校验原始引用为空、类别未知、置信度为 1.5 的证据；随后存入合法证据并重新
读取 case。

**预期。** 依次得到 `ErrMissingRawRef`、`ErrUnknownEvidenceKind` 与
`ErrConfidenceRange`；已存证据重新读取时逐字段一致。

**测试函数。** `internal/domain/evidence_test.go` 中的 `TestEvidenceValidation`

<!-- sdd:item id=TC-0003 stage=verify status=approved derives_from=REQ-0005 -->
#### TC-0003 — 越权贡献被拒绝

**层级。** integration。**验证。** REQ-0005。

**步骤。** 应用一条由 `metrics` 角色发出的 `propose_hypothesis`，以及一条由 `analysis`
发出的 `raise_critique`。

**预期。** 两者都被拒绝，各有一条 `contribution_rejected` 事件记录，且投影中不会新增任何
假设或质疑。

**测试函数。** `internal/orchestrator/apply_test.go` 中的 `TestRoleCapabilityEnforced`

<!-- sdd:item id=TC-0004 stage=verify status=approved derives_from=REQ-0003 -->
#### TC-0004 — 事件序号完整性

**层级。** unit。**验证。** REQ-0003。

**步骤。** 追加一个首序号不等于 `len(existing)+1` 的批次；随后重放一份缺失序号的日志。

**预期。** 追加时返回 `ErrSequenceGap`；`Replay` 返回指明断层的错误。

**测试函数。** `internal/store/store_test.go` 中的 `TestSequenceGapRejected`

<!-- sdd:item id=TC-0005 stage=verify status=approved derives_from=REQ-0002 -->
#### TC-0005 — Case 在存储重启后仍存在

**层级。** integration。**验证。** REQ-0002。

**步骤。** 通过文件存储写入一个 case，丢弃该实例，在同一目录上构造新存储，然后读取该 case。

**预期。** 该 case 出现在索引中，且其事件重放得到相等的投影。

**测试函数。** `internal/store/file_test.go` 中的 `TestFileStoreRoundTrip`

### 3.2 信号采集

<!-- sdd:item id=TC-0010 stage=verify status=approved derives_from=REQ-0010 -->
#### TC-0010 — 参考场景离线运行

**层级。** e2e。**验证。** REQ-0010。

**步骤。** 以夹具档端到端运行样例 `C1`。

**预期。** case 到达 `closed`，且 metric、log、change、topology、knowledge 与
verification 各类别证据均存在。

**测试函数。** `internal/orchestrator/e2e_test.go` 中的 `TestReferenceScenarioOffline`

<!-- sdd:item id=TC-0011 stage=verify status=approved derives_from=REQ-0011 -->
#### TC-0011 — 异常起点与同步变化

**层级。** unit。**验证。** REQ-0011。

**步骤。** 分析一条在已知采样点从 0.002 阶跃到 0.18 的序列；再分析两条同时阶跃的序列。

**预期。** 起点与阶跃相差不超过一个采样间隔；两条序列被报告为同步变化；没有任何摘要包含
禁用清单中的因果断言词。

**测试函数。** `internal/signal/anomaly_test.go` 中的 `TestAnomalyOnset`

<!-- sdd:item id=TC-0012 stage=verify status=approved derives_from=REQ-0012 -->
#### TC-0012 — 日志聚类有界且有序

**层级。** unit。**验证。** REQ-0012。

**步骤。** 对 184 行匹配一个模板、3 行匹配另一个模板的日志聚类，并另加一组超过
`maxClusters` 的输入。

**预期。** 得到两个簇，数量依次为 184 与 3，每簇样本不超过 `logSamplesPerCluster`，保留
簇数不超过 `maxClusters`。

**测试函数。** `internal/signal/cluster_test.go` 中的 `TestLogClustering`

<!-- sdd:item id=TC-0013 stage=verify status=approved derives_from=REQ-0013 -->
#### TC-0013 — 变更证据报告双值且不发生回滚

**层级。** integration。**验证。** REQ-0013。

**步骤。** 在装有记录型执行器的前提下，对样例 `C1` 的扩展窗口运行变更采集器。

**预期。** `DB_POOL_SIZE` 被报告为 `20`、`2` 及发布时间戳；执行器记录到零次调用。

**测试函数。** `internal/agent/collectors_test.go` 中的 `TestChangeCollector`

<!-- sdd:item id=TC-0014 stage=verify status=approved derives_from=REQ-0014 -->
#### TC-0014 — 拓扑区分源头与受害者

**层级。** unit。**验证。** REQ-0014。

**步骤。** 对 `order-api` 先异常、`checkout-web` 后异常的夹具采集拓扑证据。

**预期。** `order-api` 为候选源头；`checkout-web` 为受影响方且不是候选源头。

**测试函数。** `internal/agent/collectors_test.go` 中的 `TestTopologyCollector`

<!-- sdd:item id=TC-0015 stage=verify status=approved derives_from=REQ-0015 -->
#### TC-0015 — Runbook 检索按症状匹配

**层级。** unit。**验证。** REQ-0015。

**步骤。** 针对症状包含连接池饱和的故障检索 runbook 语料。

**预期。** 连接池 runbook 出现在匹配结果中，其标识符与命中词记录在证据事实里。

**测试函数。** `internal/agent/collectors_test.go` 中的 `TestKnowledgeCollector`

<!-- sdd:item id=TC-0016 stage=verify status=approved derives_from=REQ-0016 -->
#### TC-0016 — 工具结果有界且截断被记录

**层级。** unit。**验证。** REQ-0016。

**步骤。** 分别对超过 `MaxRows` 的结果、超过 `MaxChars` 的结果、以及超过 `MaxSpan` 的查询
施加上限。

**预期。** 每个结果都在上限内，`truncated` 为真，且产生的证据带有 `Truncated` 标记。

**测试函数。** `internal/signal/bounds_test.go` 中的 `TestBounds`

### 3.3 假设与评分

<!-- sdd:item id=TC-0020 stage=verify status=approved derives_from=REQ-0020 -->
#### TC-0020 — 假设数量受限且携带机理

**层级。** unit。**验证。** REQ-0020。

**步骤。** 对样例 `C1` 第 2 轮的证据集合形成假设。

**预期。** 假设数量不超过 `maxHypothesesPerRound`；每个都有非空机理与非空支持集合；
`sig-db-pool-exhaustion` 在其中。

**测试函数。** `internal/reasoner/rule_test.go` 中的 `TestHypothesise`

<!-- sdd:item id=TC-0021 stage=verify status=approved derives_from=REQ-0021 -->
#### TC-0021 — 无支撑假设被拒绝，覆盖度守住推进

**层级。** integration。**验证。** REQ-0021。

**步骤。** 应用一个支持列表为空的假设与一个引用未知标识符的假设；随后对仅由指标证据支撑的
假设求值处置守卫。

**预期。** 两个假设都以 `hypothesis_rejected` 事件被拒绝且永不出现在排序中；守卫以指明
类别数的理由拒绝推进。

**测试函数。** `internal/orchestrator/guard_test.go` 中的
`TestUnsupportedHypothesisRejected`

<!-- sdd:item id=TC-0022 stage=verify status=approved derives_from=REQ-0022 -->
#### TC-0022 — 分数可拆解且权重和为一

**层级。** unit。**验证。** REQ-0022。

**步骤。** 对第 2 轮证据下的某假设评分并对其拆解求和；对声明的权重求和。

**预期。** 各项减惩罚等于总分，误差在 `1e-9` 内；权重总和为 `1.0`；每项都给出其权重与取值。

**测试函数。** `internal/reasoner/score_test.go` 中的 `TestScoreBreakdown`

<!-- sdd:item id=TC-0023 stage=verify status=approved derives_from=REQ-0023 -->
#### TC-0023 — 反证恰好施加一次惩罚

**层级。** unit。**验证。** REQ-0023。

**步骤。** 依次对无反证、有一条未消解反证、以及该反证被标记为已消解的假设评分。

**预期。** 第二次总分正好低 `counterEvidencePenalty`；第三次与第一次相等。

**测试函数。** `internal/reasoner/score_test.go` 中的 `TestCounterEvidencePenalty`

<!-- sdd:item id=TC-0024 stage=verify status=approved derives_from=REQ-0024 -->
#### TC-0024 — 排序是全序且稳定

**层级。** unit。**验证。** REQ-0024。

**步骤。** 对分数与类别数均相同的假设，以 100 次打乱后的输入顺序分别排序。

**预期。** 输出顺序每次都相同，且并列时按标识符升序。

**测试函数。** `internal/reasoner/score_test.go` 中的 `TestRankIsTotalOrder`

### 3.4 对抗评审

<!-- sdd:item id=TC-0030 stage=verify status=approved derives_from=REQ-0030 -->
#### TC-0030 — 每个被审查的假设都获得裁决

**层级。** unit。**验证。** REQ-0030。

**步骤。** 对样例 `C1` 第 1 轮的假设集合执行质疑。

**预期。** 每个假设至少有一条质疑；每个裁决都属于声明的四个取值之一；质疑者未发出任何
假设类贡献。

**测试函数。** `internal/reasoner/critique_test.go` 中的 `TestCritiqueProducesVerdicts`

<!-- sdd:item id=TC-0031 stage=verify status=approved derives_from=REQ-0031 -->
#### TC-0031 — 索证强制追加一轮，未满足索证被报告

**层级。** e2e。**验证。** REQ-0031。

**步骤。** 运行样例 `C1` 并检查轮次迁移；随后运行一个索证永远无法满足、且已达 `maxRounds`
的样例。

**预期。** 前者返回 `collecting`，且第 2 轮包含携带被索取描述符的证据；后者终止，且报告
列出未满足的索证。

**测试函数。** `internal/orchestrator/e2e_test.go` 中的 `TestCriticForcesAnotherRound`

<!-- sdd:item id=TC-0032 stage=verify status=approved derives_from=REQ-0032 -->
#### TC-0032 — 提出替代解释并附带区分性索证

**层级。** unit。**验证。** REQ-0032。

**步骤。** 对样例 `C1` 第 1 轮集合执行质疑，此时流量突增领先且流量历史尚未被检查。

**预期。** 出现一条类别为 `alternative_explanation` 的质疑并点名对手签名，同时发出对历史
峰值流量对比的索证。

**测试函数。** `internal/reasoner/critique_test.go` 中的 `TestAlternativeExplanationRule`

<!-- sdd:item id=TC-0033 stage=verify status=approved derives_from=REQ-0033 -->
#### TC-0033 — 校验时间先后

**层级。** unit。**验证。** REQ-0033。

**步骤。** 对一个归因于变更、但所匹配变更时间戳晚于异常起点的假设执行质疑。

**预期。** 附加反证，且裁决为 `revise` 或 `reject`，绝不为 `accept`。

**测试函数。** `internal/reasoner/critique_test.go` 中的 `TestTemporalOrderRule`

<!-- sdd:item id=TC-0034 stage=verify status=approved derives_from=REQ-0034 -->
#### TC-0034 — 挑战"根因与受害者"混淆

**层级。** unit。**验证。** REQ-0034。

**步骤。** 对一个归咎于某服务、而拓扑证据指出其上游更早异常的假设执行质疑。

**预期。** 出现一条类别为 `source_vs_victim` 的质疑并点名该上游候选。

**测试函数。** `internal/reasoner/critique_test.go` 中的 `TestSourceVsVictimRule`

<!-- sdd:item id=TC-0035 stage=verify status=approved derives_from=REQ-0035 -->
#### TC-0035 — 推进需要满足接受条件

**层级。** unit。**验证。** REQ-0035。

**步骤。** 依次对裁决为 `revise` 的领先者、低于 `acceptThreshold` 的领先者、以及存在变更
证据但领先者不含变更证据的 case 求值处置守卫。

**预期。** 三者都被拒绝，且各自给出指明失败条件的理由。

**测试函数。** `internal/orchestrator/guard_test.go` 中的 `TestRemediationGuard`

<!-- sdd:item id=TC-0036 stage=verify status=approved derives_from=REQ-0036 -->
#### TC-0036 — 分差过小时索证或升级

**层级。** integration。**验证。** REQ-0036。

**步骤。** 对前两名差值小于 `closeCallMargin` 且仍有预算的集合执行质疑；随后在预算耗尽的
情况下重复。

**预期。** 前者发出区分性索证；后者把 case 驱动到 `human_review`。

**测试函数。** `internal/orchestrator/machine_test.go` 中的 `TestCloseCallEscalates`

### 3.5 处置、策略与审批

<!-- sdd:item id=TC-0040 stage=verify status=approved derives_from=REQ-0040 -->
#### TC-0040 — 动作是完整提案

**层级。** integration。**验证。** REQ-0040。

**步骤。** 对样例 `C1` 被接受的假设运行处置；随后校验一个无回滚的 `medium` 风险动作。

**预期。** 提议动作为 `set_config order-api DB_POOL_SIZE 20`，风险 `medium`，回滚到 `2`，
验证 `http_5xx_rate`；不完整的动作校验失败。

**测试函数。** `internal/agent/remediation_test.go` 中的 `TestRemediationProposal`

<!-- sdd:item id=TC-0041 stage=verify status=approved derives_from=REQ-0041 -->
#### TC-0041 — 白名单之外一律拒绝

**层级。** policy。**验证。** REQ-0041。

**步骤。** 分别求值目标为未列服务、未列工具、未列配置键、超范围取值、shell 工具、`sql`
工具、`delete_resource` 与 `bulk_restart` 的动作——后四者另附带一份审批。

**预期。** 全部拒绝；每条拒绝都指明被违反的规则；类别拒绝在有审批时同样成立。

**测试函数。** `internal/policy/policy_test.go` 中的 `TestPolicyDenies`

<!-- sdd:item id=TC-0042 stage=verify status=approved derives_from=REQ-0042 -->
#### TC-0042 — 中风险停在审批门

**层级。** e2e。**验证。** REQ-0042。

**步骤。** 在装有记录型执行器、且不记录任何决定的前提下运行样例 `C1`。

**预期。** case 状态为 `awaiting_approval`，执行器记录到零次调用，且存在一条
`approval_requested` 事件。

**测试函数。** `internal/orchestrator/e2e_test.go` 中的 `TestApprovalGateHalts`

<!-- sdd:item id=TC-0043 stage=verify status=approved derives_from=REQ-0043 -->
#### TC-0043 — 执行前一刻重新校验策略

**层级。** policy。**验证。** REQ-0043。

**步骤。** 批准一个动作、改动其参数、再执行；另外，批准一个动作后先收紧策略再执行。

**预期。** 前者以 `tampered_after_approval` 被拒绝；后者被重新求值拒绝；两者都记录
`action_refused` 且未调用任何执行器。

**测试函数。** `internal/policy/executor_test.go` 中的 `TestExecutorRechecksPolicy`

<!-- sdd:item id=TC-0044 stage=verify status=approved derives_from=REQ-0044 -->
#### TC-0044 — 检索到的内容不能成为指令

**层级。** policy。**验证。** REQ-0044。

**步骤。** 运行一个夹具日志行与 runbook 文本包含"重启服务、关闭策略、执行 shell 命令"指示
的 case。

**预期。** 没有任何动作来源于该文本，运行后策略白名单不变，且执行器拒绝由日志行拼装的动作。

**测试函数。** `internal/policy/injection_test.go` 中的 `TestUntrustedContentIsData`

<!-- sdd:item id=TC-0045 stage=verify status=approved derives_from=REQ-0045 -->
#### TC-0045 — 审批与执行留有审计

**层级。** integration。**验证。** REQ-0045。

**步骤。** 批准并执行样例 `C1` 的动作，随后读取事件与报告。

**预期。** `approval_recorded` 携带决定、身份、时间戳与备注；`action_executed` 携带结果与
耗时；二者都出现在报告中。

**测试函数。** `internal/orchestrator/e2e_test.go` 中的 `TestAuditTrail`

<!-- sdd:item id=TC-0046 stage=verify status=approved derives_from=REQ-0046 -->
#### TC-0046 — 审批被拒绝仍产出报告

**层级。** e2e。**验证。** REQ-0046。

**步骤。** 运行样例 `C1` 并拒绝审批。

**预期。** case 到达 `closed`，未调用任何执行器，且报告包含被拒绝的动作及其理由。

**测试函数。** `internal/orchestrator/e2e_test.go` 中的 `TestRejectedApprovalReports`

### 3.6 验证与报告

<!-- sdd:item id=TC-0050 stage=verify status=approved derives_from=REQ-0003,REQ-0053 -->
#### TC-0050 — 重放重现 case 与报告

**层级。** integration。**验证。** REQ-0003、REQ-0053。

**步骤。** 完成样例 `C1`，读取其事件，重放进空投影，并分别从两者渲染报告。

**预期。** 两个投影深度相等，两份报告逐字节一致。

**测试函数。** `internal/domain/case_test.go` 中的 `TestReplayEquality`

<!-- sdd:item id=TC-0051 stage=verify status=approved derives_from=REQ-0050 -->
#### TC-0051 — 只有在动作之后验证才观察到恢复

**层级。** e2e。**验证。** REQ-0050。

**步骤。** 先在执行前查询验证信号，再批准、执行并运行验证。

**预期。** 之前：未恢复。之后：`http_5xx_rate` 恢复，前后两个取值均存在，并以
`verification` 类别记录为证据。

**测试函数。** `internal/orchestrator/e2e_test.go` 中的
`TestVerificationAfterRemediation`

<!-- sdd:item id=TC-0052 stage=verify status=approved derives_from=REQ-0051 -->
#### TC-0052 — 恢复失败时在预算内返回调查

**层级。** integration。**验证。** REQ-0051。

**步骤。** 在仍有预算时运行一个动作后信号不恢复的 case；随后在预算耗尽时重复。

**预期。** 前者返回 `collecting` 并把假设标记为 `challenged`；后者带着失败记录到达
`reporting`。

**测试函数。** `internal/orchestrator/machine_test.go` 中的 `TestFailedRecoveryReturns`

<!-- sdd:item id=TC-0053 stage=verify status=approved derives_from=REQ-0052 -->
#### TC-0053 — 报告解释了排除了什么

**层级。** integration。**验证。** REQ-0052。

**步骤。** 为已完成的样例 `C1` 渲染报告。

**预期。** 声明的每个小节均存在；被否决替代假设小节点名 `sig-traffic-surge` 并以其反证为
理由；分数拆解表列出全部六项。

**测试函数。** `internal/report/report_test.go` 中的 `TestReportSections`

### 3.7 编排、API 与控制台

<!-- sdd:item id=TC-0060 stage=verify status=approved derives_from=REQ-0060 -->
#### TC-0060 — 状态机有界且合法

**层级。** integration。**验证。** REQ-0060。

**步骤。** 用一个始终索证的质疑者运行 case；再直接尝试一次非法迁移。

**预期。** 恰好发生 `maxRounds` 轮采集且 case 到达 `reporting`；非法迁移被拒绝并记录。

**测试函数。** `internal/orchestrator/machine_test.go` 中的 `TestStateMachineBounded`

<!-- sdd:item id=TC-0061 stage=verify status=approved derives_from=REQ-0061 -->
#### TC-0061 — 并发采集与顺序无关

**层级。** integration。**验证。** REQ-0061。

**步骤。** 在 `-race` 下，让采集器以不同延迟打乱完成顺序，重复运行一轮采集 50 次。

**预期。** 每次运行产生的证据标识符及其顺序都相同。

**测试函数。** `internal/orchestrator/engine_test.go` 中的 `TestCollectionDeterminism`

<!-- sdd:item id=TC-0062 stage=verify status=approved derives_from=REQ-0062 -->
#### TC-0062 — 每次迁移与工具调用都产生事件

**层级。** integration。**验证。** REQ-0062。

**步骤。** 完成样例 `C1` 并检查事件日志。

**预期。** 序号从 1 起连续；进入的每个状态都有 `state_changed` 事件；每次采集器运行都有带
耗时的 `agent_started` 与 `agent_completed`。

**测试函数。** `internal/orchestrator/e2e_test.go` 中的 `TestEventCompleteness`

<!-- sdd:item id=TC-0063 stage=verify status=approved derives_from=REQ-0063 -->
#### TC-0063 — API 校验并提供生命周期

**层级。** integration。**验证。** REQ-0063。

**步骤。** 通过 `httptest` 检验每个端点，包括缺少 `service` 的畸形告警、未知 case 标识符，
以及请求体中的未知字段。

**预期。** 返回 `400` 且指明 `service`；未知 case 返回 `404`；未知字段返回 `400`；报告
端点返回 Markdown 且 content type 为 `text/markdown; charset=utf-8`；集合按标识符排序。

**测试函数。** `internal/httpapi/api_test.go` 中的 `TestAPILifecycle`

<!-- sdd:item id=TC-0064 stage=verify status=approved derives_from=REQ-0064 -->
#### TC-0064 — 推流跨重连无损且永不阻塞

**层级。** integration。**验证。** REQ-0064。

**步骤。** 在 case 运行期间从序号 0 订阅；中途断开并用 `Last-Event-ID` 续传；另外，让某个
订阅者的缓冲溢出。

**预期。** 客户端跨重连按顺序恰好一次收到每条事件；溢出的订阅者被丢弃，而 case 仍然完成。

**测试函数。** `internal/httpapi/stream_test.go` 中的 `TestStreamResume`

<!-- sdd:item id=TC-0065 stage=verify status=approved derives_from=REQ-0065 -->
#### TC-0065 — 控制台渲染对抗过程

**层级。** integration。**验证。** REQ-0065。

**步骤。** 为已完成的样例 `C1` 渲染工作台页面。

**预期。** HTML 中包含时间线、按类别分组的证据、带分数拆解的排序假设、至少一条带裁决的
质疑，以及针对该中风险动作的审批控件。

**测试函数。** `internal/httpapi/console_test.go` 中的 `TestConsoleRenders`

### 3.8 确定性与边界

<!-- sdd:item id=TC-0070 stage=verify status=approved derives_from=REQ-0004,REQ-0090 -->
#### TC-0070 — 相同输入产生相同输出

**层级。** e2e。**验证。** REQ-0004、REQ-0090。

**步骤。** 在两个独立引擎中各运行一次样例 `C1`，比较每个生成标识符、每个集合顺序与渲染的
报告；再以子进程方式运行二进制两次并比较标准输出。

**预期。** 所有比较均逐字节一致。

**测试函数。** `internal/orchestrator/e2e_test.go` 中的 `TestDeterministicRun`

<!-- sdd:item id=TC-0071 stage=verify status=approved derives_from=REQ-0094 -->
#### TC-0071 — Case 内存有界

**层级。** integration。**验证。** REQ-0094。

**步骤。** 把 case 驱动超过 `maxEvidencePerCase`、`maxHypothesesPerRound` 与
`maxDemandsPerRound`。

**预期。** 保留数量都在各自上限内，且有 `evidence_truncated` 事件记录该次截断。

**测试函数。** `internal/orchestrator/apply_test.go` 中的 `TestCaseBounds`

<!-- sdd:item id=TC-0072 stage=verify status=approved derives_from=REQ-0024 -->
#### TC-0072 — 假设标识跨轮稳定

**层级。** unit。**验证。** REQ-0024。

**步骤。** 先对第 1 轮证据形成假设，再对包含第 1 轮的第 2 轮证据形成假设。

**预期。** 第 1 轮已产生过假设的签名，在第 2 轮复用其标识符并更新分数，而不是产生重复项。

**测试函数。** `internal/reasoner/rule_test.go` 中的 `TestHypothesisIdentityStable`

<!-- sdd:item id=TC-0073 stage=verify status=approved derives_from=REQ-0060 -->
#### TC-0073 — Agent 失败不会中止 case

**层级。** integration。**验证。** REQ-0060。

**步骤。** 运行一轮采集，其中一个采集器返回错误。

**预期。** 记录一条 `agent_failed` 事件，其余采集器的证据被应用，且 case 继续推进。

**测试函数。** `internal/orchestrator/engine_test.go` 中的 `TestAgentFailureIsolated`

<!-- sdd:item id=TC-0074 stage=verify status=approved derives_from=REQ-0001 -->
#### TC-0074 — 平面依赖方向被强制

**层级。** governance。**验证。** REQ-0001。

**步骤。** 解析 `internal/` 下每个包的 import 列表，并与声明的模块次序比对。

**预期。** 没有推理平面的包 import 存储、适配器或事件日志；没有信号平面的包 import 推理或
控制平面；`domain` 不 import `internal/` 下任何东西。

**测试函数。** `internal/domain/arch_test.go` 中的 `TestPlaneDependencies`

### 3.9 评估

<!-- sdd:item id=TC-0080 stage=verify status=approved derives_from=REQ-0080 -->
#### TC-0080 — 三种模式在相同输入上运行

**层级。** integration。**验证。** REQ-0080。

**步骤。** 以 `single`、`multi_no_critic` 与 `multi_with_critic` 运行样例 `C1`。

**预期。** 三者均完成并记录各自模式；单 Agent 与无质疑模式把 `sig-traffic-surge` 排在
首位；完整流程把 `sig-db-pool-exhaustion` 排在首位。

**测试函数。** `internal/eval/eval_test.go` 中的 `TestThreeModes`

<!-- sdd:item id=TC-0081 stage=verify status=approved derives_from=REQ-0081 -->
#### TC-0081 — 指标由已执行的运行计算得出

**层级。** integration。**验证。** REQ-0081。

**步骤。** 以三种模式运行完整样例库并汇总。

**预期。** 每个模式行都携带 top-1 准确率、top-3 覆盖率、证据类别完整度、质疑者纠正数、
被拦截动作数与样本量；完整流程的 top-1 准确率高于单 Agent 模式。

**测试函数。** `internal/eval/eval_test.go` 中的 `TestEvaluationSummary`

<!-- sdd:item id=TC-0082 stage=verify status=approved derives_from=REQ-0082 -->
#### TC-0082 — 样例库是声明式的

**层级。** unit。**验证。** REQ-0082。

**步骤。** 加载目录并断言每个样例都能解析到其预期签名；再从仅用于测试的内嵌集合加载一个
额外样例。

**预期。** 目录加载成功，引用可解析，且额外样例在无任何代码改动的情况下被发现。

**测试函数。** `internal/catalog/catalog_test.go` 中的 `TestCatalogLoad`

### 3.10 交付

<!-- sdd:item id=TC-0090 stage=verify status=approved derives_from=REQ-0092 -->
#### TC-0090 — 容器定义自包含

**层级。** delivery。**验证。** REQ-0092。

**步骤。** 解析 `deploy/docker/Dockerfile` 与 compose 文件。

**预期。** 构建阶段除基础镜像外不发起网络拉取，声明了非 root 用户与健康检查，且 compose
栈声明了 API、演示服务及其依赖关系。

**测试函数。** `internal/httpapi/delivery_test.go` 中的 `TestDeliveryArtifacts`

<!-- sdd:item id=TC-0091 stage=verify status=approved derives_from=REQ-0093 -->
#### TC-0091 — 手册中英双语存在且保持一致

**层级。** governance。**验证。** REQ-0093。

**步骤。** 对文档树运行双语 lint。

**预期。** 手册文档对存在、版本一致，且不报告任何一致性问题。

**测试函数。** `internal/sdd/manual_test.go` 中的 `TestManualBilingualParity`

<!-- sdd:item id=TC-0092 stage=verify status=approved derives_from=REQ-0095 -->
#### TC-0092 — 发布流水线产出经门禁把关的 linux/amd64 制品

**层级。** delivery。**验证。** REQ-0095。

**步骤。** 解析 `.github/workflows/release.yml`。

**预期。** 该工作流由 `v*` 标签触发，也可以带版本号入参手动启动；在任何发布步骤之前
运行交付门禁；显式针对
`linux/amd64` 构建；在发布之前启动所构建的镜像并等待 `/healthz`；同时以版本号与提交
SHA 两种标签标注镜像；并创建一个资产中包含可加载镜像 tar 包与 `SHA256SUMS` 的
release。

其中关于顺序的断言才是实质性的。一个先发布、后验证的工作流可以满足 REQ-0095 的每一条
单独条款，却恰恰瓦解了它的目的，因此测试比较的是字节偏移：门禁与健康检查都必须出现在
第一次向镜像仓库推送之前。

**测试函数。** `internal/httpapi/delivery_test.go` 中的 `TestReleasePipeline`

### 3.11 在线信号适配器

<!-- sdd:item id=TC-0100 stage=verify status=approved derives_from=REQ-0096 -->
#### TC-0100 — Prometheus 适配器把 range 响应映射为序列

**层级。** unit。**验证。** REQ-0096。

**步骤。** 用 `httptest` 服务器提供一份录制的 `query_range` 载荷并查询它。

**预期。** 采样点按时间戳顺序出现且取值已解析；标签与单位被带上；请求发出了 `query`、
`start`、`end` 与 `step`。`NaN` 采样点在序列中是缺失的，而不是以 `0` 的形式存在。

**测试函数。** `internal/signal/prometheus/prometheus_test.go` 中的 `TestPrometheusRange`

<!-- sdd:item id=TC-0101 stage=verify status=approved derives_from=REQ-0096 -->
#### TC-0101 — Prometheus 适配器施加上限并以带类型的方式报告失败

**层级。** unit。**验证。** REQ-0096。

**步骤。** 先提供超过 `MaxRows` 的载荷，再提供一个 API 错误，然后关闭服务器并查询这个
已死的端点。

**预期。** 过长的序列被截断为最近的 `MaxRows` 个采样点并报告截断；API 错误与不可达端点
都表现为指明端口的 `signal.SourceError`，而不是一个泛化错误或 panic。

**测试函数。** `internal/signal/prometheus/prometheus_test.go` 中的
`TestPrometheusBoundsAndErrors`

<!-- sdd:item id=TC-0102 stage=verify status=approved derives_from=REQ-0097 -->
#### TC-0102 — 容器日志适配器完成解复用与过滤

**层级。** unit。**验证。** REQ-0097。

**步骤。** 提供一段多路复用的日志流，其中包含一条载荷字节里恰好出现帧头模式的记录，然后
带关键词与时间窗检索它。

**预期。** stdout 与 stderr 记录连同正确的级别与时间戳被还原；那条对抗性载荷不会让解析器
失步；只返回同时匹配所有关键词、且落在时间窗内的行，并施加上限与标记。

**测试函数。** `internal/signal/containerlog/containerlog_test.go` 中的
`TestContainerLogSearch`

<!-- sdd:item id=TC-0103 stage=verify status=approved derives_from=REQ-0098 -->
#### TC-0103 — 默认配置档就是夹具档

**层级。** integration。**验证。** REQ-0098。

**步骤。** 先校验空配置、夹具档、一份完整的在线档以及若干不完整的在线档；再分别用空配置、
夹具档、以及一个未知的 profile 名构建信号集合。

**预期。** 空配置与夹具档都产出夹具集合；在线档只替换指标端口与日志端口；未知名称是一个
指明出错取值的错误，而不是静默回退。

校验不需要先构建即可完成，因此入口可以在启动时就拒掉错误的 profile。把这项检查推迟到构建
时，会让一个 profile 拼错的服务先干净地启动、然后一个 case 接一个 case 地失败——而失败恰好
在已经有人依赖它的时候才到来。

**测试函数。** `internal/signal/profile/profile_test.go` 中的 `TestProfileSelection`

### 3.12 过拟合

<!-- sdd:item id=TC-0104 stage=verify status=approved derives_from=REQ-0099 -->
#### TC-0104 — 最响亮的证据不会取胜，固定的偏见也不会

**层级。** e2e。**验证。** REQ-0099。

**步骤。** 以三种模式端到端运行样例 `C4`。它的日志被数据库连接错误压倒性占据，而其指标与
变更证据否定了每一个数据库类解释，并支持一次前所未有的流量峰值。

**预期。** 被接受的根因是 `sig-traffic-surge`。尽管 `sig-db-pool-exhaustion` 与
`sig-db-outage` 在该 case 中拥有最高的日志量，二者都不会被接受。

正是这个样例让评估保持诚实。C1 的取胜方式是把 `sig-traffic-surge` 从第一名降下来，因此
一个学到"领先的解释就是错的"、或者干脆学到"流量从来不是原因"的系统，照样能在 C1、C2、C3
上拿满分。只有存在一个"质疑者必须选择不反对"的样例，才能把"会区分"与"有偏见"区分开。

**测试函数。** `internal/orchestrator/e2e_test.go` 中的 `TestMisleadingLogsDoNotWin`

### 3.13 模型推理器

<!-- sdd:item id=TC-0105 stage=verify status=approved derives_from=REQ-0100 -->
#### TC-0105 — 适配器映射服务商响应并在本地打分

**层级。** unit。**验证。** REQ-0100。

**步骤。** 提供一份录制的服务商响应，它选择了某个目录签名并给出取自快照的证据标识符，
然后据此形成假设。

**预期。** 返回的假设，其主张与机理来自目录签名，其支持证据恰好是被引用的那些标识符，
其分数可拆解为所声明的加权项。服务商给出的任何分数都被忽略。

**测试函数。** `internal/reasoner/model/model_test.go` 中的 `TestModelHypothesise`

<!-- sdd:item id=TC-0106 stage=verify status=approved derives_from=REQ-0100 -->
#### TC-0106 — 服务商输出不可信，且失败不等于沉默

**层级。** unit。**验证。** REQ-0100。

**步骤。** 依次提供引用未知证据标识符、未知签名、非法裁决、以及适配器自行发明主张的响应；
随后提供一个非 200 状态码与一个不可达端点。

**预期。** 每个非法条目被丢弃，而同一响应中的合法条目保留。没有任何服务商撰写的散文会作为
假设主张出现。传输或状态失败返回 `ProviderError`，绝不返回空结果——空假设列表意味着"没有
任何匹配"，出故障的服务商不得冒充这个结论。

**测试函数。** `internal/reasoner/model/model_test.go` 中的 `TestModelOutputIsUntrusted`

<!-- sdd:item id=TC-0107 stage=verify status=approved derives_from=REQ-0101 -->
#### TC-0107 — 受害者的告警找到上游原因

**层级。** e2e。**验证。** REQ-0101。

**步骤。** 端到端运行样例 `C5`。它的告警指向 `checkout-web`，而后者依赖 `order-api`；
`order-api` 早三分钟就已异常，真正的故障也在它身上。

**预期。** 被接受的解释是定位在 `order-api` 的那个，拟议动作指向 `order-api` 而不是发出
告警的服务。`source_vs_victim` 质疑出现在所提出的质疑中，且不会去挑战那个本就把原因归于
上游的解释。

**测试函数。** `internal/orchestrator/e2e_test.go` 中的
`TestVictimAlertReachesUpstreamCause`

<!-- sdd:item id=TC-0108 stage=verify status=approved derives_from=REQ-0102 -->
#### TC-0108 — 晚于起点的变更因时序被拒绝

**层级。** e2e。**验证。** REQ-0102。

**步骤。** 端到端运行样例 `C6`。它包含一次发生在错误开始**之后**四分钟的 `DB_POOL_SIZE`
变更，而且连接池确实处于饱和——因此连接池解释在指标、日志与变更三方面都匹配，唯一能反驳
它的就是时序。

**预期。** 连接池解释被拒绝，`temporal_order` 出现在质疑之中，被接受的原因是那个变更早于
起点的依赖配置错误。

**测试函数。** `internal/orchestrator/e2e_test.go` 中的 `TestPostOnsetChangeIsRejected`

<!-- sdd:item id=TC-0109 stage=verify status=approved derives_from=REQ-0011 -->
#### TC-0109 — 塌陷的序列被判为异常

**层级。** unit。**验证。** REQ-0011。

**步骤。** 分析一条稳定在 1、并在已知采样点跌到 0 的可用性量表，以及一条跌到其基线一小部分
的吞吐量序列。

**预期。** 两者都被报告为异常并给出下跌起点，且携带 `collapsed` 事实。仅有单个采样点下探的
序列不会被报告为塌陷，因此噪声不会变成结论。

这个用例之所以存在，是因为该检测器此前只认得"增长"。`sig-db-outage` 要求它的可用性指标
发生下跌，而这一点被表达为 `saturated`——后者由"已声明的容量"算出，可任何可用性量表都没有
容量，于是该要求不可满足，这条签名永远无法被采集器产出的任何证据完整匹配。

**测试函数。** `internal/signal/anomaly_test.go` 中的 `TestCollapseIsAnomalous`

## 4. 覆盖矩阵

由 `sddctl matrix` 生成；权威版本位于 `docs/traceability-matrix.md`，每次治理运行时重新
生成。在 M1 门禁通过之前，每条 P0 需求的测试列都必须非空。

## 5. 检查点记录

记录在 `docs/checkpoints.en.md` 及其中文对应文件中，每个实现批次一行，包含所执行命令、
结果与日期。
