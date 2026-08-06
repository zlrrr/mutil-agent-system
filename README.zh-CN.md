# IncidentOps Arena

*[English](README.md)*

一个面向生产故障定位与处置的多 Agent 系统——其中有一个 Agent 被专门"雇来"反对其他人。

代价高昂的故障不是那些没人看的，而是那些有人看了、找到一个说得通的故事、然后就停下来的。
单个推理者会忠实复现这种失效：给它一份满是 `connection timeout` 的日志，它会自信地讲出
数据库宕机的故事——因为第一个自洽的故事就是它讲出来的那个。

因此本系统按来源拆分证据采集，并赋予一个角色结构性的反对权：它可以索取特定的补充证据，
从而把调查打回去再跑一轮；它也可以拒绝让某个结论继续推进。

## 它长什么样

参考场景，离线运行，约一秒完成：

```
第 1 轮 —— 五个采集器，默认查询
  1. 流量上涨超出容量 ............................................. 0.49   ← 错误
  2. 数据库连接池被耗尽 ........................................... 0.40
  3. 数据库不可用 ................................................. 0.35

  质疑者：前两名相差 0.09，落在 0.15 的间距内。连接池解释需要一个没人查过的
  饱和度指标，以及一次没人回看得够远因而没找到的变更。而且没有任何证据排除掉
  那些对手解释。
  → 发出四条索证，调查返回采集阶段

第 2 轮 —— 索证被满足
  1. 连接池从 20 被改为 2，导致连接池耗尽 ......................... 0.94   ← 正确
  2. 数据库不可用 ................................................. 0.37
  3. 流量上涨超出容量 ............................................. 0.29   （携带反证）

  → set_config order-api DB_POOL_SIZE 20，中风险，停在审批门
  → 批准之后：5xx 0.129 → 0.003，延迟 1623ms → 190ms，已恢复
```

质疑者不是给结果加了批注，而是**改变**了结果。

## 快速开始

```bash
make demo          # 参考场景端到端运行，离线，约 1 秒
make demo-gate     # 同上，但停在审批门
make eval          # 对比单 Agent、无质疑与完整流程
make serve         # API 与控制台，http://localhost:8080
make up            # 含在线目标与 Prometheus 的完整演示栈
```

使用 Docker，从已发布的 release 获取（`linux/amd64`）：

```bash
docker pull ghcr.io/zlrrr/mutil-agent-system:0.1.0
docker run --rm -p 8080:8080 ghcr.io/zlrrr/mutil-agent-system:0.1.0
```

或者自己构建：

```bash
docker build -f deploy/docker/Dockerfile -t incidentops-arena:0.1.0-mvp .
docker run --rm -p 8080:8080 incidentops-arena:0.1.0-mvp
```

然后打开 <http://localhost:8080>，选一个故障样例，按 **Open investigation**。

每个 release 还会把镜像以可加载的 tar 包形式附上，并为每个资产提供校验和，因此在没有
镜像仓库访问权时它依然可用——具体做法见[用户手册](docs/manual/user-manual.zh.md)。

## 架构

三个平面，依赖方向单向：控制平面依赖推理平面，推理平面依赖信号平面的*端口*——而永远不
依赖其适配器。有一条测试通过解析 import 来强制这一点。

```
控制平面   编排器 · 策略 · 事件日志 · Case 投影 · API/SSE/控制台
              │ 决定接下来发生什么；唯一写入方；唯一调用执行器的一方
推理平面   Agent 角色 · 推理器端口（rule | model）· 评分 · 质疑规则
              │ 决定什么是真的；永不触碰存储或适配器
信号平面   MetricSource · LogSource · ChangeSource · TopologySource ·
           KnowledgeSource · Actuator —— 每个都有离线夹具适配器
```

四个决策承载了大部分重量：

**Agent 返回值，而不是产生副作用。** 每个 Agent 都是 case 快照的纯函数，返回
`[]Contribution`——封闭集合 `AddEvidence`、`ProposeHypothesis`、`RaiseCritique`、
`DemandEvidence`、`ProposeAction`、`RecordVerification`、`RecordExecution`。编排器是唯一
写入方。三个性质由此免费得到：Agent 可表驱动测试、并行采集无需加锁即安全、事件日志自己
写自己。

**Case 是只追加日志的折叠。** 实时视图与重放视图是同一个函数，因此重放相等性是结构性的，
而不是靠测试硬凑出来的。

**推理位于端口之后，且默认实现是确定性的。** 一个基于声明式故障签名目录的规则引擎——不是
桩件。因此整个流程可以离线、可复现、无需模型服务地运行，这正是让测试套件与现场演示同时
可信的前提。基于模型的适配器实现同一个接口。

**策略是咽喉点，不是建议。** shell、SQL、删除与跨服务批量操作作为**类别**被拒绝，且发生在
查阅任何白名单之前。策略会在执行前的一刻针对动作当下的实际形态再次求值。

完整细节：[架构](specs/001-incidentops-arena/architecture.zh.md) ·
[概要设计](specs/001-incidentops-arena/hld.zh.md) ·
[详细设计](specs/001-incidentops-arena/dld.zh.md) ·
[决策记录](docs/adr/)

## Agent 角色

| 角色 | 可发出 | 职责 |
|---|---|---|
| `metrics` | 证据 | 异常起点、饱和度、同步变化——只讲时序，绝不讲因果 |
| `logs` | 证据 | 按归一化模板聚类，有界，并给出代表性样本 |
| `change` | 证据 | 配置与发布历史；绝不执行任何回滚 |
| `topology` | 证据 | 候选源头与受害者之分、影响半径 |
| `knowledge` | 证据 | Runbook 检索，严格作为数据处理 |
| `analysis` | 假设 | 带机理与可拆解分数的排序解释 |
| `critic` | 质疑、索证 | 挑战、索证、否决——且永不提出自己的答案 |
| `remediation` | 动作 | 完整提案：风险、回滚、前置条件、验证 |
| `executor` | 执行结果 | 执行器端口引用的唯一持有者 |
| `verification` | 证据、结果 | 动作之后重新查询触发信号 |

角色能力表由编排器强制执行，而非靠约定：采集器若提出假设，其贡献会被拒绝，违规会被记录。

## 结论，是算出来的

```bash
make eval
```

| 模式 | 样本量 | top-1 准确率 | top-3 覆盖率 | 证据类别数 | 轮数 |
|---|---|---|---|---|---|
| single | 5 | 20% | 80% | 5.0 | 1.0 |
| multi_no_critic | 5 | 20% | 80% | 5.0 | 1.0 |
| multi_with_critic | 5 | 100% | 100% | 5.8 | 2.2 |

这组样例中有两个在做特定的事。`C4` 的正确答案恰恰是 `sig-traffic-surge`——也就是参考场景
用整个第 2 轮去降级的那个解释——而两个基线都答对了它。它之所以存在，就是为了让一个仅仅学到
"领先的解释就是错的"的系统必然在某处失败；也正因如此，对抗流程的 100% 才是一个关于**区分
能力**的主张，而不是关于条件反射式反对的主张。`C5` 的告警落在故障的下游服务上；它是唯一一个
连基线的 top-3 里都没有正确答案的样例——这正是它们覆盖率为 80% 而非 100% 的原因。

两条注意事项，会与数字一同打印而不是被藏起来：样本是五个故障样例；单 Agent 基线拿到的是与
完整流程**相同的工具与相同的默认查询**。它建模的是"一个上下文窗口、看一眼"，而不是一套更弱
的工具——这是此处能给出的最公平对比，也是唯一能把结果归因于流程的对比方式。

## 规格驱动开发

这里的每一份制品都链接在同一条可机器校验的链上：

```
章程 → 目标 → 需求 → 架构 → 概要设计 → 详细设计 → 任务 → 代码
                        ↘ ADR      需求 → 测试用例 → 测试
```

`sddctl` 强制执行它。修改一个目标，所有下游制品都会显式失效直到被复查——包括具体的源文件：

```bash
make sdd-impact ID=G-002    # 修改这个目标会要求你复查哪些东西
make sdd-validate           # 双语一致性、溯源与漂移
make sdd-matrix             # 需求到源码的矩阵
```

所有文档均有中英文两份，且配对是被强制的：两种呈现必须携带相同版本、相同且顺序一致的条目
标识符、相同的标题骨架。只改其中一种语言会导致构建失败。

延伸阅读：[SDD 工作流](docs/sdd-workflow.zh.md) ·
[章程](.sdd/memory/constitution.zh.md) ·
[溯源矩阵](docs/traceability-matrix.md)

## 仓库结构

```
.sdd/                      章程、模板、治理配置、锁文件
specs/000-sdd-framework/   治理工具自身的规格链
specs/001-incidentops-arena/
                           目标章程、需求、架构、概要设计、详细设计、任务、测试计划
docs/                      用户手册、ADR、工作流、检查点、生成的报告
cmd/                       arena、evalctl、faultctl、sddctl、demo-order-api
internal/domain/           实体、contribution 代数、事件、case 投影
internal/signal/           六个端口、上限、异常检测、聚类、夹具
internal/catalog/          内嵌故障签名、runbook 与故障样例
internal/reasoner/         评分、规则推理器、六条质疑规则
internal/agent/            采集器、分析、质疑、处置、验证、基线
internal/policy/           白名单、类别拒绝、执行器
internal/orchestrator/     状态机、贡献应用、守卫、引擎
internal/httpapi/          REST、SSE、内嵌控制台
deploy/                    Dockerfile、compose 栈、Prometheus 配置
```

## 环境要求与状态

Go 1.22 或更高版本。无第三方依赖——`go.mod` 没有 `require` 块，CI 会对此做断言。

里程碑 M1 已完成：参考场景离线端到端跑通，全部既定测试在竞态检测器下通过，治理树校验通过
且无漂移。

```bash
make check     # vet、完整测试套件与治理门禁
```

## 文档

| 文档 | English | 中文 |
|---|---|---|
| 用户手册 | [en](docs/manual/user-manual.en.md) | [zh](docs/manual/user-manual.zh.md) |
| SDD 工作流 | [en](docs/sdd-workflow.en.md) | [zh](docs/sdd-workflow.zh.md) |
| 章程 | [en](.sdd/memory/constitution.en.md) | [zh](.sdd/memory/constitution.zh.md) |
| 目标章程 | [en](specs/001-incidentops-arena/charter.en.md) | [zh](specs/001-incidentops-arena/charter.zh.md) |
| 需求规格 | [en](specs/001-incidentops-arena/spec.en.md) | [zh](specs/001-incidentops-arena/spec.zh.md) |
| 架构设计 | [en](specs/001-incidentops-arena/architecture.en.md) | [zh](specs/001-incidentops-arena/architecture.zh.md) |
| 概要设计 | [en](specs/001-incidentops-arena/hld.en.md) | [zh](specs/001-incidentops-arena/hld.zh.md) |
| 详细设计 | [en](specs/001-incidentops-arena/dld.en.md) | [zh](specs/001-incidentops-arena/dld.zh.md) |
| 测试计划 | [en](specs/001-incidentops-arena/test-plan.en.md) | [zh](specs/001-incidentops-arena/test-plan.zh.md) |
| 任务分解 | [en](specs/001-incidentops-arena/tasks.en.md) | [zh](specs/001-incidentops-arena/tasks.zh.md) |

## 许可证

见 [LICENSE](LICENSE)。
