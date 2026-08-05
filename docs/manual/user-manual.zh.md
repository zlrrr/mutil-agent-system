---
id: MANUAL-001
lang: zh
counterpart: user-manual.en.md
doc_version: 1.0.0
status: approved
stage: constitution
---

# IncidentOps Arena — 用户手册

## 1. 这个系统是什么

IncidentOps Arena 用一组各自掌握一类证据的 Agent 来调查生产告警，然后让其中一个 Agent
去反驳其余所有 Agent。

它要解决的问题不是"没人看故障"，而是"第一个自洽的故事往往就终结了搜索"。给出一份满是
`connection timeout` 的日志，单个推理者会自信地讲出数据库宕机的故事——然后停下。在单趟
流程里，没有任何结构性的东西强迫它去问：*还有什么别的原因也会产生这些证据？*

于是本系统按来源拆分证据采集，并且**付钱让一个角色去反对**。这个角色拥有真实权力：它可以
索取特定的补充证据，从而把调查打回去再跑一轮；它也可以拒绝让某个结论推进到处置阶段。

**它不是什么。** 它不会自行修复生产系统。任何会改变目标系统、且高于低风险等级的动作，都会
停在审批门前，直到有人做出决定。它也不是一个"LLM 应用"：默认推理策略是基于故障签名目录的
确定性规则引擎——正因如此，整套系统才能离线且可复现地运行。基于模型的推理器是同一端口背后
一次有文档的替换，见第 9 节。

## 2. 安装

### 2.1 使用 Docker

容器是推荐的交付形态。它自带 API、控制台、故障样例目录与命令行工具。

**从已发布的 release 获取**（`linux/amd64`）。每个打了标签的版本都会发布一个由该提交
精确构建出的镜像：

```bash
docker pull ghcr.io/zlrrr/mutil-agent-system:0.1.0
docker run --rm -p 8080:8080 ghcr.io/zlrrr/mutil-agent-system:0.1.0
```

如果你访问不了镜像仓库，同一个镜像也以可加载的 tar 包形式挂在 release 上，因此没有
仓库访问权时 release 依然可用：

```bash
gunzip -c incidentops-arena-0.1.0-linux-amd64-image.tar.gz | docker load
docker run --rm -p 8080:8080 ghcr.io/zlrrr/mutil-agent-system:0.1.0
```

release 资产均带校验和；加载前请用 `sha256sum -c SHA256SUMS` 校验。每个镜像还会额外
打上其提交 SHA 标签，这样即便版本标签发生移动，你也依然能准确指明自己跑的是哪一份。

**从源码构建**，如果你更愿意自己来：

```bash
docker build -f deploy/docker/Dockerfile -t incidentops-arena:0.1.0-mvp .
docker run --rm -p 8080:8080 incidentops-arena:0.1.0-mvp
```

然后打开 <http://localhost:8080>。

镜像以非 root 用户运行，在 `/healthz` 上声明健康检查，并把 case 日志存放在 `/data` 卷中。
其内部不会访问网络。

### 2.2 从源码构建

唯一要求是 Go 1.22 或更高版本。本模块没有第三方依赖，因此没有任何东西需要下载。

```bash
make build          # 产出 ./bin/{arena,evalctl,faultctl,sddctl,demo-order-api}
make check          # vet、完整测试套件与治理门禁
./bin/arena serve
```

### 2.3 完整演示栈

如果要针对真实服务（而非夹具数据）驱动同一个场景：

```bash
make up             # arena、演示订单服务、流量生成器、Prometheus
```

| 服务 | 地址 | 用途 |
|---|---|---|
| arena | <http://localhost:8080> | API 与控制台 |
| order-api | <http://localhost:8081> | 演示目标，连接池大小可配置 |
| prometheus | <http://localhost:9090> | 采集演示目标并承载告警规则 |

`make down` 停止该栈并删除其卷。

## 3. 五分钟走查

这是参考场景 `C1`。它离线运行，且每次产生完全相同的结果。

```bash
make demo
```

你会看到调查推进到审批门、批准自己的动作（演示命令这样做是为了让走查走完）、验证恢复、
打印报告。按顺序发生了这些事：

**第 1 轮 —— 那个看似成立的错误答案。** 五个采集器以各自的默认查询并行运行。指标发现错误率、
延迟与请求速率同时升高。日志发现 184 条数据库连接超时。拓扑确认 `order-api` 是最早异常的
服务。知识匹配到两篇 runbook。

分析角色给出三个解释的排序：

| 排名 | 解释 | 分数 |
|---|---|---|
| 1 | 流量上涨超出容量 | 0.49 |
| 2 | 数据库连接池被耗尽 | 0.40 |
| 3 | 数据库不可用 | 0.35 |

领先者是**错的**，而且它错得情有可原：流量确实涨了，而且此刻没有任何证据与这个故事矛盾。

**质疑者介入。** 四条独立规则被触发。前两名相差 0.09，落在 0.15 的"分差过小"区间内，因此
这个排序尚不具备意义。连接池解释需要一个没人查过的饱和度指标。它还需要一次配置变更，而默认
的变更查询只回看到告警时刻。此外，没有任何证据排除掉那些对手解释。

质疑者发出四条索证：

- 数据库连接池饱和度指标
- 故障起点前 30 分钟内的配置变更
- 数据库可用性指标
- 等负载下的历史流量对比

**第 2 轮 —— 索证被满足。** 连接池指标返回"已饱和"。扩展后的变更查询发现 `DB_POOL_SIZE`
在 10:05:30 从 `20` 改为 `2`，比连接池饱和早 90 秒。历史对比显示两天前曾以同等峰值提供
服务，错误率仅 0.20%——这构成了针对流量解释的反证。

排序发生重排：

| 排名 | 解释 | 分数 | 说明 |
|---|---|---|---|
| 1 | 连接池大小被下调，导致连接池耗尽 | 0.94 | 现由五类证据支撑 |
| 2 | 数据库不可用 | 0.37 | 可用性指标显示数据库正常 |
| 3 | 流量上涨超出容量 | 0.29 | 携带反证，被惩罚 0.20 |

**门禁。** 处置角色提议 `set_config order-api DB_POOL_SIZE 20`，中风险，回滚到 `2`，并以
错误率与延迟作为验证。状态机停下。此时没有调用过任何执行器。你可以亲自验证：

```bash
make demo-gate      # 运行到门禁处停止
```

**批准之后。** 执行器针对动作当下的实际形态重新校验策略，调用执行器端口，随后验证角色在
动作后窗口内重新查询两个信号。错误率的均值从 0.129 降到 0.003，延迟从 1623ms 降到 190ms；
两者都被报告为已恢复。报告写出，case 关闭。

## 4. 使用控制台

打开 <http://localhost:8080>，选一个故障样例，按 **Open investigation**。

| 区域 | 展示内容 |
|---|---|
| 摘要 | 告警、被接受的根因、其置信度与质疑者的裁决 |
| Agent 时间线 | 每条事件实时推送 |
| 证据 | 按类别分组，每条都带标识符与其背后的确切查询 |
| 假设 | 排序展示，每个都带完整的六项分数拆解与证据链 |
| 对抗评审 | 每条质疑及其规则、裁决与挑战内容，以及被索取的证据 |
| 处置 | 拟议动作及其风险、回滚与验证，附审批控件 |
| 报告 | 渲染后的根因报告，也可下载 Markdown 或 JSON |

页面上每个证据标识符都能解析出一个 `RawRef`——也就是产生它的那条查询。这正是关键所在：
一条你无法回溯的主张，就是一条你无法核查的主张。

## 5. 命令行

```bash
arena serve   --addr :8080 --store memory|file --data ./data
arena demo    --case C1 --mode multi_with_critic --approve --out report.md
arena cases
evalctl run   --cases C1,C2 --out docs/evaluation.md
faultctl inject|restore|status --case C1 --target http://localhost:8081
sddctl        validate|lint|trace|drift|seal|gate|impact|matrix|graph
```

配置优先级为参数、环境变量、默认值：

| 变量 | 默认值 | 作用 |
|---|---|---|
| `ARENA_ADDR` | `:8080` | 监听地址 |
| `ARENA_STORE` | `memory` | `memory` 或 `file` |
| `ARENA_DATA` | `./data` | 文件存储目录 |
| `FAULTCTL_TARGET` | `http://localhost:8081` | 演示服务地址 |

## 6. API

| 方法与路径 | 用途 |
|---|---|
| `POST /api/cases` | 由告警创建 case，或仅凭 `case_ref` 创建 |
| `GET /api/cases` | 列出 case，最新在前 |
| `GET /api/cases/{id}` | 完整的 case 投影 |
| `GET /api/cases/{id}/events` | 事件日志，可选 `?from=N` |
| `GET /api/cases/{id}/stream` | 实时 server-sent events，可用 `Last-Event-ID` 续传 |
| `POST /api/cases/{id}/actions/{actionID}/decision` | 记录审批决定 |
| `GET /api/cases/{id}/report.md` | Markdown 报告 |
| `GET /api/cases/{id}/report.json` | JSON 报告 |
| `GET /api/catalog/cases` | 可复现故障样例列表 |
| `GET /healthz` | 健康探针 |

由告警创建 case：

```bash
curl -sS localhost:8080/api/cases -H 'content-type: application/json' -d '{
  "alert_name": "OrderApiHighErrorRate",
  "service": "order-api",
  "severity": "P1",
  "starts_at": "2026-07-26T10:07:00Z",
  "case_ref": "C1"
}'
```

批准它提议的动作：

```bash
curl -sS -X POST \
  localhost:8080/api/cases/inc-445761e8c3/actions/a-remediation-001/decision \
  -H 'content-type: application/json' \
  -d '{"decision":"approved","by":"you","comment":"restore the pool size"}'
```

校验错误返回 `400` 与 `{"error": "...", "field": "..."}`；未知 case 返回 `404`。

## 7. 安全模型

这一节值得仔细读，因为正是它让这套系统可以安全地指向任何目标。

**只读工具自主运行，写入不行。** 每个动作都带风险等级。`low` 可以自动执行；`medium` 与
`high` 会让状态机停下，直到有人记录决定。

**有些东西是作为"类别"被拒绝的，且发生在查阅任何白名单之前。** 任意 shell、自由形式 SQL、
资源删除与跨服务批量操作被无条件拒绝。没有任何配置能放行它们，审批也无法覆盖它们。

**策略被检查两次。** 一次在动作被提议时，另一次在它执行前的一刻——针对动作那一刻的实际
形态。审批之后参数被改动的动作会以"被篡改"拒绝。审批之后被收紧的策略会拒绝它先前允许的
动作。

**检索到的内容是数据，绝非指令。** 日志行、提交信息、runbook 与工单只能作为证据载荷进入。
它们无法改变 Agent 角色、工具选择、策略或审批状态。执行器只接受带类型的动作。参考场景的
夹具中**刻意**放入了一行日志，指示系统执行 `rm -rf` 并重启所有服务；它不会产生任何动作，
测试套件对此做了断言。

**每个决定都可审计。** 审批记录决定、身份、时间戳与备注。执行记录结果、耗时与目标的响应。
两者都出现在事件日志与报告中。

白名单默认如下，且属于配置而非代码：

| 类别 | 允许项 |
|---|---|
| 服务 | `order-api`、`payment-api`、`inventory-api`、`checkout-web` |
| 工具 | `set_config`、`restart_service`、`scale_service`、`create_ticket` |
| 配置键 | `DB_POOL_SIZE`、`FEATURE_FLAG_SAFE_MODE`、`RATE_LIMIT_QPS`、`PAYMENT_URL` |
| 取值范围 | `DB_POOL_SIZE` 1–200，`RATE_LIMIT_QPS` 1–10000 |

## 8. 评估该结论

本系统主张"对抗流程优于单趟流程"。这个主张是**算出来的**，不是宣称的：

```bash
make eval
```

三种模式在**完全相同**的夹具输入上运行，因此差异可归因于流程而非数据：

- `single` —— 一个 Agent 拥有全部工具，一趟，无质疑
- `multi_no_critic` —— 采集器与分析角色，无对抗轮
- `multi_with_critic` —— 完整流程

看这些数字时请带上两条注意，报告也会把它们打印在旁边。第一，样本是三个故障样例；三个样例
上的 100% top-1 准确率，只是关于这三个样例的陈述。第二，单 Agent 基线拿到的是与完整流程
**相同的工具与相同的默认查询**——它建模的是"一个上下文窗口、看一眼"，而不是一套更弱的工具。
这是此处能给出的最公平基线，也正是让这个对比有意义的原因。

## 9. 扩展系统

**新增故障场景**是一次数据变更。把一个 `case-*.json` 放进 `internal/catalog/data/`，声明
告警、夹具信号、预期根因与预期处置。评估会在无任何代码改动的情况下纳入它。

**新增故障签名**同样是数据：声明它所需的证据模式、它产出的机理叙述、能够反驳它的区分性
证据，以及可选的它所隐含的处置。

**替换为基于模型的推理器**只需实现一个接口：

```go
type Reasoner interface {
    Hypothesise(ctx context.Context, s domain.Snapshot) ([]domain.Hypothesis, error)
    Critique(ctx context.Context, s domain.Snapshot) ([]domain.Critique, []domain.EvidenceDemand, error)
}
```

端口之上的一切都不改变：同样的契约、同样的状态机、同样的存储。模型输出被视为不可信的结构化
数据，与规则输出一样要通过完全相同的校验、证据绑定与策略检查。确定性适配器仍然是测试的默认
选择，因为测试套件无法对一个采样分布做断言。

**新增在线信号源**只需实现 `internal/signal` 中六个端口接口之一，并在 `internal/arena` 中
选择它。

## 10. 故障排查

| 现象 | 原因与处理 |
|---|---|
| `fault case "X" is not in the catalog` | 运行 `arena cases` 查看可用标识符 |
| Case 卡在 `awaiting_approval` | 这是设计如此。在控制台或通过决定端点批准或拒绝它 |
| 控制台时间线停止更新 | 推流会自行重连并从上次序号续传；若未恢复请刷新页面 |
| `case ... already exists` | Case 标识符由告警派生，因此同一条告警会重新打开同一个 case。改变告警起始时间或换一个样例 |
| 演示栈启动了但 `faultctl` 连不上目标 | 检查 `docker compose ps`；`faultctl --target` 必须指向已发布的 `order-api` 端口 |
| `sddctl validate` 报告失效条目 | 上游规格发生了变化。运行 `sddctl drift` 查看完整影响集合，逐个更新制品，然后 `sddctl seal` |
| `go test` 在 `TestPlaneDependencies` 上失败 | 某个包跨架构边界做了 import。失败信息会指出两个包与被违反的规则 |

## 11. 本仓库的开发方式

这里的每一份制品——目标、需求、架构、设计、任务、代码与测试——都链接在一条可溯源的链上，
且这条链是被机器校验的。修改一个目标会让所有下游制品显式失效，直到它们被逐一复查。

```bash
make sdd-validate          # 双语一致性、溯源与漂移
make sdd-impact ID=G-002   # 修改这个目标会要求你复查哪些东西
make sdd-matrix            # 重新生成需求到源码的矩阵
```

完整流程记录在 `docs/sdd-workflow.zh.md`，它所强制的规则记录在
`.sdd/memory/constitution.zh.md`。两者同样都有英文版本——这种成对本身就是规则之一，并由
`sddctl lint` 强制执行。
