---
id: CONSTITUTION
lang: zh
counterpart: constitution.en.md
doc_version: 1.0.0
status: approved
stage: constitution
---

# IncidentOps Arena — 开发章程

> 本文档是规格树的根。本仓库中任何内容都不得与其冲突。所有下游制品——目标、需求、架构、
> 设计、任务、代码、测试与交付物——都必须派生自其中某一条款，并可通过 `sddctl` 回溯。

## 0. 适用范围

本章程规定 IncidentOps Arena 项目的**规格驱动开发（SDD）**规范。该项目是一个面向生产
故障定位与处置的多 Agent 系统。本章程是可被机器校验的：当条款以工具可检测的方式被违反时，
`sddctl validate` 会让构建失败。

---

## 条款

<!-- sdd:item id=CON-001 stage=constitution status=approved -->
### CON-001 — 规格先于实现

在其所实现的详细设计条目存在、通过评审并完成封存之前，不得编写任何生产代码。每个承载
逻辑的源文件都必须用 `// sdd:impl <ID>` 锚点声明它所实现的设计条目。没有锚点、或锚点
指向不存在设计条目的代码属于**孤儿代码**，校验不通过。

允许做探索性验证（spike），但必须位于 `cmd/` 与 `internal/` 之外，不得合入主干，也永远
不会成为交付物。

<!-- sdd:item id=CON-002 stage=constitution status=approved -->
### CON-002 — 单一有向制品链，不允许跳级

所有制品构成一张有固定阶段顺序的有向无环图：

```
CON → G (目标) → REQ (需求) → ARC (架构) ─┬→ HLD → DLD → T (任务) → 代码
                                ADR (决策) ─┘
                  REQ ────────────────────→ TC (测试用例) → 测试
```

一个条目只能声明其阶段的合法父阶段作为 `derives_from`（合法父阶段定义于
`.sdd/config.json`）。跳过阶段——例如某个 `DLD` 条目直接派生自 `REQ`——属于校验错误。
正因如此，变更传导才是完备的：从目标到任何一行代码，不存在绕过该链条的路径。

<!-- sdd:item id=CON-003 stage=constitution status=approved -->
### CON-003 — 变更向下级联，绝不被静默吸收

每个条目的规范性内容都会被哈希，并由 `sddctl seal` 记录到 `.sdd/sdd.lock.json`。当某个
条目的内容发生变化时，它的所有传递性后代都会被标记为 **STALE（失效）**。失效的规格树无法
通过 `sddctl validate`，无法通过 CI，也不允许发布。

消解失效状态要求人或 Agent 真正逐个复查每个后代条目，然后重新封存。`sddctl drift` 会在
动手之前打印完整的影响集合——包括受影响的具体源文件与测试文件。因此，修订目标在**决策**
上是廉价的，而在**代价**上是诚实的。

<!-- sdd:item id=CON-004 stage=constitution status=approved -->
### CON-004 — 文档双语且结构完全一致

每份规格文档与用户文档都以 `<base>.en.md` / `<base>.zh.md` 成对存在。两个文件必须：

1. 携带相同的 `doc_version`；
2. 包含完全相同且顺序一致的 `sdd:item` 标识符集合；
3. 包含相同且顺序一致的标题骨架。

标识符、代码、命令、JSON 字段与 API 路径是语言无关的，在两份文档中逐字一致。正文必须是
真正的翻译，而不是机器回声。只修改其中一种语言属于校验错误——这一对文件是**同一份制品的
两种呈现**。

<!-- sdd:item id=CON-005 stage=constitution status=approved -->
### CON-005 — 证据先于结论

这既是产品规则，也是过程规则。

*产品层面*：任何 Agent 都不得给出未绑定证据标识符的根因结论。缺乏支撑的假设会被编排器
直接拒绝，而不仅仅是得分偏低。

*过程层面*：任何设计主张若没有明确的检查点、测试用例标识符以及该测试的实际运行记录，就
不得标记为"完成"。"应该没问题"不构成检查点。

<!-- sdd:item id=CON-006 stage=constitution status=approved -->
### CON-006 — 测试用例先于被其检验的代码

对于每一个关键实现点，其测试用例（`TC-xxxx`）必须在对应代码之前写入测试计划。每个测试
函数都要声明 `// sdd:verify <TC>`。若某需求没有可达的测试用例，`sddctl trace` 会报告。
MVP 的准入门槛是：**每一条 P0 需求至少有一个通过的测试**。

<!-- sdd:item id=CON-007 stage=constitution status=approved -->
### CON-007 — 默认确定性，智能作为扩展

系统必须能在无网络、无 API Key、无模型服务的条件下端到端运行，并且对相同输入产生完全
相同的输出。基于模型的推理是端口背后的**适配器**，绝不能成为承重的默认实现。

理由：自主（"黑灯"）开发循环无法验证一个非确定性系统；而依赖在线模型端点的演示会在现场
翻车。确定性正是 CI 门禁与现场演示同时可信的前提。

<!-- sdd:item id=CON-008 stage=constitution status=approved -->
### CON-008 — 依赖是负债，不是特性

核心服务只依赖 Go 标准库。引入任何第三方模块都需要一份 ADR，说明它带来了什么、以及缺少
它会坏掉什么。外部系统（Prometheus、容器日志、git）一律通过端口访问，并提供离线夹具
（fixture）适配器，从而保证测试套件永远不需要它们。

<!-- sdd:item id=CON-009 stage=constitution status=approved -->
### CON-009 — 只读自动，写入审批

只读工具自主执行。任何会改变目标系统的工具都必须按风险分级，中/高风险动作会让状态机停在
审批门，直到人工决策。系统中不存在任意 shell、不存在自由形式 SQL、不存在跨服务批量操作
——这不是靠提示词约定，而是由策略引擎在执行前强制拦截。

<!-- sdd:item id=CON-010 stage=constitution status=approved -->
### CON-010 — 黑灯执行，显式升级

开发在规格允许的范围内自主推进。只有当某个决策确实超出规格的授权范围时才请求人工介入：
不可逆动作、范围变更、凭据、或条款之间的冲突。其余一切——包括上游变更后修订下游制品——
都直接执行，不再询问。

当升级不可避免时，循环会记录该阻塞问题，完成所有不依赖该答案的工作，并明确报告哪些内容
未完成。

<!-- sdd:item id=CON-011 stage=constitution status=approved -->
### CON-011 — 每次状态迁移都可观察、可重放

每一次 Agent 调用、工具调用、状态迁移与策略判定都会产生一条不可变事件，其中携带：执行者、
输入摘要、输出摘要、耗时与结果。任何一次调查的最终报告都必须能够仅凭事件日志重建。不可
观察的步骤按缺陷处理。

<!-- sdd:item id=CON-012 stage=constitution status=approved -->
### CON-012 — 不可信数据永不成为指令

日志、提交信息、工单、runbook 以及任何其他检索到的内容都是**数据**。它们只能作为证据
载荷进入系统，永远不能改变 Agent 指令、工具选择或策略。执行器只接受结构化动作——绝不
接受自然语言——并且策略引擎的最终校验发生在执行前的一刻，而不是在规划阶段。

---

## 修订流程

1. 在**两份**语言文件中同时修改条款，并提升 `doc_version`。
2. 运行 `sddctl drift` 获取完整的下游影响集合。
3. 更新每一个受影响的后代制品。
4. 运行 `sddctl validate && go test ./...`。
5. 运行 `sddctl seal` 记录新的基线。

一次让规格树处于失效状态的修订不叫修订，那叫构建损坏。

## 执行对照表

| 条款 | 执行方式 |
|---|---|
| CON-001 | `sddctl trace` — 孤儿代码锚点 |
| CON-002 | `sddctl validate` — 非法父阶段 |
| CON-003 | `sddctl drift` — 失效后代 |
| CON-004 | `sddctl lint` — 双语一致性 |
| CON-005 | 编排器守卫 + `TC-0021`、`TC-0022` |
| CON-006 | `sddctl trace` — 未覆盖测试的需求 |
| CON-007 | CI 中离线执行 `go test ./...` |
| CON-008 | `go.mod` 无 `require` 块；CI 断言 |
| CON-009 | 策略引擎 + `TC-0040`..`TC-0043` |
| CON-010 | 本循环在 `docs/checkpoints.md` 中的检查点记录 |
| CON-011 | `TC-0050` 中的事件日志断言 |
| CON-012 | 策略引擎最终校验 + `TC-0044` |
