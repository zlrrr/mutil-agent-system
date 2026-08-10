---
id: TASKS-001
lang: zh
counterpart: tasks.en.md
doc_version: 1.0.0
status: approved
stage: plan
---

# IncidentOps Arena — 任务分解

## 1. 规则

- 只有当任务的检查点测试通过、且 `go vet ./...` 无告警时，该任务才算完成。
- 任务为它实现的每个 DLD 条目携带一个 `// sdd:impl` 锚点。
- 标记为可并行的任务触碰互不相交的文件，且无顺序依赖边。
- 无法说清自己检查点的任务，说明拆解得还不够。

## 2. 任务

<!-- sdd:item id=T-001 stage=plan status=approved derives_from=DLD-1001,DLD-1002,DLD-1003,DLD-1004,DLD-1005 -->
### T-001 — 领域实体

**实现。** DLD-1001、DLD-1002、DLD-1003、DLD-1004、DLD-1005

**文件。** `internal/domain/{ids,evidence,hypothesis,critique,action}.go`

**完成定义。** 锚点齐全；TC-0002 通过；校验错误带类型且可区分。

**阻塞于。** 无。**可并行。** 否——其余一切都要对着它编译。

<!-- sdd:item id=T-002 stage=plan status=approved derives_from=DLD-1006,DLD-1007 -->
### T-002 — Contribution 代数、事件与投影

**实现。** DLD-1006、DLD-1007

**文件。** `internal/domain/{contribution,event,case}.go`

**完成定义。** 角色能力表完整；`Replay` 从空状态折叠；TC-0050 与 TC-0003 的领域部分通过；
`Leading()` 只跳过被拒绝的解释，这一点由 TC-0110 端到端验证。

**阻塞于。** T-001。**可并行。** 否。

<!-- sdd:item id=T-003 stage=plan status=approved derives_from=DLD-1010,DLD-1011,DLD-1064 -->
### T-003 — 存储适配器与事件分发器

**实现。** DLD-1010、DLD-1011、DLD-1064

**文件。** `internal/store/{store,memory,file}.go`、`internal/eventbus/broker.go`

**完成定义。** TC-0004、TC-0005 通过；缓冲写满时分发器永不阻塞；`-race` 无告警。

**阻塞于。** T-002。**可并行。** 是，与 T-004。

<!-- sdd:item id=T-004 stage=plan status=approved derives_from=DLD-1020,DLD-1021,DLD-1022 -->
### T-004 — 信号端口、上限、异常检测与聚类

**实现。** DLD-1020、DLD-1021、DLD-1022

**文件。** `internal/signal/{ports,bounds,anomaly,cluster}.go`

**完成定义。** TC-0011、TC-0012、TC-0016 通过；没有适配器返回无界结果。

**阻塞于。** T-002。**可并行。** 是，与 T-003。

<!-- sdd:item id=T-005 stage=plan status=approved derives_from=DLD-1030,DLD-1031 -->
### T-005 — 内嵌目录、签名与故障样例库

**实现。** DLD-1030、DLD-1031

**文件。** `internal/catalog/{catalog,signature,case}.go`、`internal/catalog/data/*.json`

**完成定义。** TC-0082 通过；目录中至少包含连接池耗尽、流量突增、数据库宕机、依赖配置错误
与慢查询的签名，以及至少三个故障样例。

**阻塞于。** T-004。**可并行。** 否。

<!-- sdd:item id=T-006 stage=plan status=approved derives_from=DLD-1023,DLD-1024 -->
### T-006 — 夹具适配器与模拟执行器

**实现。** DLD-1023、DLD-1024

**文件。** `internal/signal/fixture/{fixture,actuator}.go`

**完成定义。** 可由故障样例构建信号集合；执行器在恢复触发之后把指标源切换为恢复后序列。

**阻塞于。** T-005。**可并行。** 否。

<!-- sdd:item id=T-007 stage=plan status=approved derives_from=DLD-1032,DLD-1033 -->
### T-007 — 评分与规则推理器

**实现。** DLD-1032、DLD-1033

**文件。** `internal/reasoner/{reasoner,score,rule}.go`

**完成定义。** TC-0020、TC-0022、TC-0023、TC-0024、TC-0072 通过；参考场景第 1 轮与第 2 轮
的总分与 DLD 算术在两位小数上一致；该适配器满足共享契约测试（TC-0113），而不只是满足接口。

**阻塞于。** T-006。**可并行。** 否。

<!-- sdd:item id=T-008 stage=plan status=approved derives_from=DLD-1034 -->
### T-008 — 质疑规则集合

**实现。** DLD-1034

**文件。** `internal/reasoner/critique.go`

**完成定义。** TC-0030 至 TC-0034 通过，每条规则一个测试；TC-0110 通过，即该集合能把一个
证据齐备的错误答案挤下来，而不只是补上空缺；质疑者不发出任何假设类贡献。

**阻塞于。** T-007。**可并行。** 否。

<!-- sdd:item id=T-009 stage=plan status=approved derives_from=DLD-1040,DLD-1041,DLD-1042 -->
### T-009 — Agent 角色与单 Agent 基线

**实现。** DLD-1040、DLD-1041、DLD-1042

**文件。** `internal/agent/{agent,collectors,analysis,critic,remediation,verification,baseline}.go`

**完成定义。** TC-0001、TC-0013、TC-0014、TC-0015、TC-0040 通过；采集器为纯函数且只发出
`add_evidence`。

**阻塞于。** T-008。**可并行。** 是，与 T-010。

<!-- sdd:item id=T-010 stage=plan status=approved derives_from=DLD-1050,DLD-1051 -->
### T-010 — 策略引擎与执行器

**实现。** DLD-1050、DLD-1051

**文件。** `internal/policy/{policy,executor}.go`

**完成定义。** TC-0041、TC-0043、TC-0044 通过；附带审批时类别拒绝依然成立；执行器持有唯一
的执行器端口引用。

**阻塞于。** T-006。**可并行。** 是，与 T-009。

<!-- sdd:item id=T-011 stage=plan status=approved derives_from=DLD-1060,DLD-1061,DLD-1062,DLD-1063 -->
### T-011 — 编排器

**实现。** DLD-1060、DLD-1061、DLD-1062、DLD-1063

**文件。** `internal/orchestrator/{machine,apply,guard,engine}.go`

**完成定义。** TC-0003、TC-0010、TC-0021、TC-0031、TC-0035、TC-0036、TC-0042、TC-0045、
TC-0046、TC-0051、TC-0052、TC-0060、TC-0061、TC-0062、TC-0070、TC-0071、TC-0073、TC-0111
通过；参考场景首次端到端跑通。

**阻塞于。** T-009、T-010。**可并行。** 否。

<!-- sdd:item id=T-012 stage=plan status=approved derives_from=DLD-1070,DLD-1071,DLD-1072,DLD-1073 -->
### T-012 — 报告、API、推流与控制台

**实现。** DLD-1070、DLD-1071、DLD-1072、DLD-1073

**文件。** `internal/report/report.go`、`internal/httpapi/{api,stream,console}.go`、
`web/*`

**完成定义。** TC-0053、TC-0063、TC-0064、TC-0065 通过；报告从实时投影与重放投影渲染结果
完全一致。

**阻塞于。** T-011。**可并行。** 是，与 T-013。

<!-- sdd:item id=T-013 stage=plan status=approved derives_from=DLD-1074 -->
### T-013 — 评估运行器

**实现。** DLD-1074

**文件。** `internal/eval/eval.go`

**完成定义。** TC-0080、TC-0081、TC-0114 通过；汇总在每个比率旁给出样本量，并在模式旁标明
推理器。

**阻塞于。** T-011。**可并行。** 是，与 T-012。

<!-- sdd:item id=T-014 stage=plan status=approved derives_from=DLD-1075 -->
### T-014 — 入口、容器与演示栈

**实现。** DLD-1075

**文件。** `cmd/arena/main.go`、`cmd/evalctl/main.go`、`cmd/faultctl/main.go`、
`deploy/docker/*`、`Makefile`、`.github/workflows/ci.yml`

**完成定义。** TC-0090 与 TC-0112 通过；`arena demo --case C1` 打印报告；每一个产出可比较
结果的命令都接受同一套推理器选择方式；`make test` 与 `make sdd-validate` 为绿。

**阻塞于。** T-012、T-013。**可并行。** 否。

<!-- sdd:item id=T-015 stage=plan status=approved derives_from=DLD-1074,DLD-1075 -->
### T-015 — 推理器选择与共享契约

**实现。** DLD-1074、DLD-1075

**文件。** `cmd/arena/main.go`、`cmd/evalctl/main.go`、`internal/eval/eval.go`、
`internal/reasoner/contract_test.go`

**完成定义。** TC-0112、TC-0113、TC-0114 通过；推理器参数由同一个 helper 注册并共享给
`arena serve`、`arena demo` 与 `evalctl run`；评测按 `(模式, 推理器)` 分组；契约测试套件对
两个适配器都运行，任一适配器不再满足它即失败。

**阻塞于。** T-014。**可并行。** 否。

<!-- sdd:item id=T-016 stage=plan status=approved derives_from=DLD-1036,DLD-1037 -->
### T-016 — 规划器端口与按方案采集

**实现。** DLD-1036、DLD-1037

**文件。** `internal/agent/plan/plan.go`、`internal/agent/collectors.go`、
`internal/orchestrator/engine.go`、`internal/domain/case.go`

**完成定义。** TC-0115、TC-0116、TC-0117 通过；两个规划器适配器对同一份契约负责，且该契约是
与端口**同时**写出来的而不是事后补的；确定性规划器复现端口引入之前的行为，因此现有测试套件
原样通过；实际执行的方案可从事件日志还原；remediation 与 verification 不接受任何策略参数。

**阻塞于。** T-015。**可并行。** 否。

## 3. 执行顺序

| 批次 | 任务 | 进入下一批次的门禁 |
|---|---|---|
| 1 | T-001、T-002 | `go build ./...` 无告警；TC-0002、TC-0050 通过 |
| 2 | T-003、T-004 | TC-0004、TC-0005、TC-0011、TC-0012、TC-0016 通过 |
| 3 | T-005、T-006 | TC-0082 通过；可由样例 `C1` 构建夹具信号集合 |
| 4 | T-007、T-008 | 参考场景分数与 DLD 算术一致；TC-0020..TC-0034 通过 |
| 5 | T-009、T-010 | TC-0001、TC-0040、TC-0041、TC-0043、TC-0044 通过 |
| 6 | T-011 | 参考场景端到端跑通；确定性测试通过 |
| 7 | T-012、T-013、T-014 | 全套测试在 `-race` 下为绿；`sddctl gate --stage deliver` 通过 |
| 8 | T-015 | 两个推理器适配器通过同一份契约；评测按推理器分别报告 |
| 9 | T-016 | 确定性规划器不改变任何行为；实际执行的方案落在事件日志上 |

## 4. 进度记录

按批次记录在 `docs/checkpoints.en.md` 及其中文对应文件中，包含所执行命令、结果与日期。
在门禁行写下之前，该批次不算关闭。
