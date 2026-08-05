---
id: SPEC-000
lang: zh
counterpart: framework.en.md
doc_version: 1.0.0
status: approved
stage: specify
---

# 000 — SDD 治理框架

本文档承载 `sddctl` 的完整制品链——它是强制执行章程的工具。由于该工具属于基础设施而非
产品本身，其目标、需求、架构、设计与测试用例集中在一份文档中；产品规格
（`specs/001-incidentops-arena/`）则每个阶段一份文档。

制品链依然是完全可溯源的：下面每个条目都携带自己的锚点，并由它所描述的同一套引擎校验。

## 1. 目标

<!-- sdd:item id=G-901 stage=charter status=approved derives_from=CON-001,CON-002 priority=P0 -->
### G-901 — 从目标到源码行全程可溯源

**陈述。** 对任意目标，工具都能枚举出实现它的需求、架构条目、设计、任务、源文件与测试；
反过来，对任意源文件，也能追出为其提供依据的目标。

**度量。** `sddctl matrix` 为每条需求输出一行，且所有 P0 需求的源码列与测试列均非空。

**优先级。** P0

<!-- sdd:item id=G-902 stage=charter status=approved derives_from=CON-003 priority=P0 -->
### G-902 — 任何上游变更都会自动暴露其完整下游代价

**陈述。** 修改上游制品会让所有传递性后代显式失效，包括受影响的具体源文件与测试文件；
在逐一复查之前，规格树不得发布。

**度量。** 修改一个目标会产生包含预期后代的非空失效集合；在重新封存之前，
`sddctl validate` 以非零码退出。

**优先级。** P0

<!-- sdd:item id=G-903 stage=charter status=approved derives_from=CON-004 priority=P0 -->
### G-903 — 双语文档不可能静默分叉

**陈述。** 只改一种语言而不改另一种，是构建失败，而不是一条评审意见。

**度量。** 对于缺失对应文件、版本不一致、条目序列不同或标题骨架不同的情况，
`sddctl lint` 均报告 error。

**优先级。** P0

<!-- sdd:item id=G-904 stage=charter status=approved derives_from=CON-007,CON-008 priority=P1 -->
### G-904 — 治理工具离线、内建、零依赖

**陈述。** 治理工具是由本仓库构建出的单个 Go 二进制，不需要网络、不需要服务、不需要
任何第三方模块。

**度量。** 在 `go.mod` 的 `require` 块为空的前提下 `go build ./cmd/sddctl` 成功，
且在无网络环境下所有治理测试通过。

**优先级。** P1

## 2. 需求

<!-- sdd:item id=REQ-9001 stage=specify status=approved derives_from=G-901 priority=P0 -->
### REQ-9001 — 从文档中解析条目锚点

**需求。** 引擎必须（MUST）识别 markdown 中的 `<!-- sdd:item id=… stage=… status=…
derives_from=… priority=… -->` 锚点，将其与紧随其后的标题关联，并采集该小节正文，直到
下一个锚点或同级/更高级别的标题为止。

**验收。** 给定一份含三个三级标题锚点的文档，当解析时，则返回三个条目，标识符、标题正确，
且正文互不重叠。

**验证方式。** TC-9001

<!-- sdd:item id=REQ-9002 stage=specify status=approved derives_from=G-901 priority=P0 -->
### REQ-9002 — 拒绝非法链路边

**需求。** 当 `derives_from` 的父条目所属阶段不是子条目阶段的合法父阶段时，引擎必须拒绝
该边；引用不存在的标识符同样必须拒绝。

**验收。** 给定一个直接派生自 `REQ` 条目的 `DLD` 条目，当执行 trace 时，则报告
`illegalParentStage` 错误。

**验证方式。** TC-9002

<!-- sdd:item id=REQ-9003 stage=specify status=approved derives_from=G-901 priority=P1 -->
### REQ-9003 — 检测派生环

**需求。** 引擎必须检测派生图中的环，并报告参与其中的标识符，而不是陷入死循环。

**验收。** 给定 A→B→C→A 的条目，当执行 trace 时，则报告一条包含三者的错误，且命令正常
结束。

**验证方式。** TC-9003

<!-- sdd:item id=REQ-9004 stage=specify status=approved derives_from=G-903 priority=P0 -->
### REQ-9004 — 每份文档都有语言对应件

**需求。** 当某文档只存在于一种已配置语言而缺少另一种时，或当文档根目录下的 markdown
文件没有语言后缀且不在豁免列表中时，引擎必须报告错误。

**验收。** 给定存在 `spec.en.md` 而没有 `spec.zh.md`，当执行 lint 时，则报告
`bilingualMissingCounterpart` 错误。

**验证方式。** TC-9004

<!-- sdd:item id=REQ-9005 stage=specify status=approved derives_from=G-903 priority=P0 -->
### REQ-9005 — 语言呈现声明相同的版本与状态

**需求。** 当一对文档的 `doc_version` 或 `status` front matter 字段不同时，引擎必须报告
错误。

**验收。** 给定版本分别为 `1.0.0` 与 `1.1.0` 的一对文档，当执行 lint 时，则报告
`bilingualVersionMismatch` 错误。

**验证方式。** TC-9005

<!-- sdd:item id=REQ-9006 stage=specify status=approved derives_from=G-903 priority=P0 -->
### REQ-9006 — 语言呈现在结构上完全一致

**需求。** 当一对文档的条目标识符有序序列不一致，或标题层级有序序列不一致时，引擎必须
报告错误。

**验收。** 给定其中一种呈现多出一个小节，当执行 lint 时，则报告指明分歧的
`bilingualStructureMismatch` 错误。

**验证方式。** TC-9006

<!-- sdd:item id=REQ-9007 stage=specify status=approved derives_from=G-901 priority=P0 -->
### REQ-9007 — 代码锚点解析到存在的设计条目

**需求。** 引擎必须扫描已配置的源码根目录，仅在注释中识别 `sdd:impl` 与 `sdd:verify`
锚点；当锚点指向不存在的标识符，或指向该锚点类型不允许的前缀时，必须报告错误。

**验收。** 给定 `// sdd:impl DLD-9999` 且不存在该条目，当执行 trace 时，则在正确的文件与
行号上报告 `orphanCodeAnchor` 错误。

**验证方式。** TC-9007

<!-- sdd:item id=REQ-9008 stage=specify status=approved derives_from=G-902 priority=P0 -->
### REQ-9008 — 规格树可封存为内容基线

**需求。** 引擎必须为每个条目记录一份内容哈希，覆盖其全部语言呈现以及锚点属性，并将其
持久化为已接受的基线。

**验收。** 给定一棵刚封存且未修改的规格树，当计算漂移时，则结果为 clean。

**验证方式。** TC-9008

<!-- sdd:item id=REQ-9009 stage=specify status=approved derives_from=G-902 priority=P0 -->
### REQ-9009 — 漂移级联至所有传递性后代，包括源文件

**需求。** 当某条目内容与封存基线不同时，引擎必须将其标记为 modified，并必须将所有传递性
后代标记为 stale，包括通过锚点可达它的源文件与测试文件。

**验收。** 给定一棵已封存的规格树，修改其中一个目标的正文，当计算漂移时，则其后代的需求、
架构、设计与任务条目出现在失效集合中，实现该链路的源文件出现在失效文件集合中。

**验证方式。** TC-9009

<!-- sdd:item id=REQ-9010 stage=specify status=approved derives_from=G-902 priority=P1 -->
### REQ-9010 — 可在动手修改前查询影响面

**需求。** 对任意标识符，引擎必须能回答：一旦变更，作者需要复查的全部后代条目与源文件
集合——且无需先真的做出这次变更。

**验收。** 给定一个子树已知的目标，当查询影响面时，则返回的集合等于该子树。

**验证方式。** TC-9010

<!-- sdd:item id=REQ-9011 stage=specify status=approved derives_from=G-901 priority=P1 -->
### REQ-9011 — 阶段门禁校验准入前置条件

**需求。** 引擎必须评估某阶段声明的具名前置条件——审批状态、无漂移、父条目被子条目覆盖、
无孤儿锚点、无未测需求、双语一致——并在任一条件未满足或无法识别时让门禁失败。

**验收。** 给定一棵存在一个未实现设计条目的规格树，当评估 `implement` 门禁时，则门禁失败
并指名该条目。

**验证方式。** TC-9011

<!-- sdd:item id=REQ-9012 stage=specify status=approved derives_from=G-901 priority=P2 -->
### REQ-9012 — 溯源可输出为矩阵与图

**需求。** 引擎必须以 markdown 渲染需求到源码的溯源矩阵，并以 Mermaid 渲染派生图。

**验收。** 给定某需求已到达源码与测试的规格树，当渲染矩阵时，则该需求所在行被标记为已覆盖。

**验证方式。** TC-9012

<!-- sdd:item id=REQ-9013 stage=specify status=approved derives_from=G-904 priority=P1 -->
### REQ-9013 — 治理工具无第三方依赖

**需求。** 引擎及其命令必须仅使用 Go 标准库即可编译。

**验收。** `go.mod` 中不含任何指向外部模块的 `require` 指令。

**验证方式。** TC-9013

## 3. 架构

<!-- sdd:item id=ARC-901 stage=architect status=approved derives_from=REQ-9001,REQ-9002,REQ-9003 -->
### ARC-901 — 制品链是由配置声明的带类型有向无环图

**元素。** 一张有向无环图，其节点类型在 `.sdd/config.json` 中声明为有序阶段，每个阶段
拥有一个标识符前缀与一组合法父前缀。

**职责。** 判定两个制品之间提出的边是否合法。

**约束。** 图绝不从文件布局或命名推断；只有显式的 `derives_from` 属性才产生边。新增一个
阶段是配置变更，不是代码变更。

**实现需求。** REQ-9001, REQ-9002, REQ-9003

<!-- sdd:item id=ARC-902 stage=architect status=approved derives_from=REQ-9008,REQ-9009,REQ-9010 -->
### ARC-902 — 内容哈希是变更传导的原语

**元素。** 逐条目的内容哈希，封存于锁文件中；失效性被定义为"从已变更节点出发的可达性"。

**职责。** 把"上游有东西变了"转化为一个精确、可枚举的下游工作集合。

**约束。** 哈希对行尾空白与连续空行不敏感，对其他一切（包括锚点属性）敏感。失效状态只能
通过显式的重新封存清除，绝不会作为任何其他命令的副作用被清掉。

**实现需求。** REQ-9008, REQ-9009, REQ-9010

<!-- sdd:item id=ARC-903 stage=architect status=approved derives_from=REQ-9004,REQ-9005,REQ-9006 -->
### ARC-903 — 一对文档是同一份制品的两种呈现

**元素。** `<base>.<lang>.md` 配对规则，外加对条目序列与标题骨架的结构性比对。

**职责。** 保证任何小节、需求或决策都不会只存在于一种语言中。

**约束。** 比对是结构性的，绝非语义性的：引擎断言两种呈现描述的是同一副骨架，并刻意不去
判断翻译质量。条目哈希跨越两种呈现，因此任一语言的变更都会级联。

**实现需求。** REQ-9004, REQ-9005, REQ-9006

<!-- sdd:item id=ARC-904 stage=architect status=approved derives_from=REQ-9007,REQ-9011,REQ-9012,REQ-9013 -->
### ARC-904 — 治理是仓库内的零依赖二进制

**元素。** 基于 `internal/sdd` 的 `cmd/sddctl`，仅使用标准库。

**职责。** 让章程在 CI、笔记本以及自主开发循环中都可执行，且结果完全一致。

**约束。** 不访问网络、不依赖外部服务、不引入第三方模块。源码锚点只在注释中被识别，因此
提及该关键字的字符串字面量无法伪造锚点。

**实现需求。** REQ-9007, REQ-9011, REQ-9012, REQ-9013

## 4. 概要设计

<!-- sdd:item id=HLD-901 stage=hld status=approved derives_from=ARC-901 -->
### HLD-901 — 配置与阶段模型

**目的。** 加载治理配置，并回答阶段/父级合法性问题。

**公开接口面。**

```go
func LoadConfig(root string) (*Config, error)
func (c *Config) StageByPrefix(prefix string) (StageConfig, bool)
func (c *Config) IsLegalParent(childID, parentID string) bool
func (c *Config) SeverityOf(class string) Severity
```

**失败行为。** 配置缺失或格式错误是致命错误；未知的问题类别解析为 `error`，使新增检查
默认收紧（fail closed）。

**细化自。** ARC-901

<!-- sdd:item id=HLD-902 stage=hld status=approved derives_from=ARC-904 -->
### HLD-902 — 问题与报告模型

**目的。** 统一表示违规项，并决定进程退出码。

**公开接口面。**

```go
type Finding struct{ Class, Level, Subject, File, Message string; Line int }
func (r *Report) Add(cfg *Config, class, subject, file string, line int, format string, args ...any)
func (r *Report) HasErrors() bool
```

**细化自。** ARC-904

<!-- sdd:item id=HLD-903 stage=hld status=approved derives_from=ARC-901,ARC-903 -->
### HLD-903 — 文档与源码锚点解析器

**目的。** 把 markdown 与 Go 源码转换为条目、标题、front matter 与代码锚点。

**公开接口面。**

```go
func ParseDoc(root, path string) (*Doc, []*Item, error)
func ParseCodeAnchors(root string, codeRoots []string) ([]CodeAnchor, error)
```

**失败行为。** 跳过围栏代码块内容，使文档中的示例无法声明条目。

**细化自。** ARC-901, ARC-903

<!-- sdd:item id=HLD-904 stage=hld status=approved derives_from=ARC-901,ARC-903 -->
### HLD-904 — 模型装配与图遍历

**目的。** 合并每个条目的多语言呈现，按阶段排序条目，并提供包含源文件合成叶子节点的
父/子遍历能力。

**公开接口面。**

```go
func Load(root string) (*Model, error)
func (m *Model) DescendantsOf(ids ...string) []string
func (m *Model) AnchorsFor(kind, id string) []CodeAnchor
```

**独占数据。** 合并后的条目表与子节点索引。

**细化自。** ARC-901, ARC-903

<!-- sdd:item id=HLD-905 stage=hld status=approved derives_from=ARC-901,ARC-903 -->
### HLD-905 — Lint 与 trace 检查

**目的。** 施加双语规则与图规则，产出问题清单。

**公开接口面。**

```go
func (m *Model) Lint() *Report
func (m *Model) Trace() *Report
func (m *Model) Validate() *Report
```

**细化自。** ARC-901, ARC-903

<!-- sdd:item id=HLD-906 stage=hld status=approved derives_from=ARC-902 -->
### HLD-906 — 封存与漂移

**目的。** 持久化已接受基线，并计算失效集合。

**公开接口面。**

```go
func (m *Model) Seal() (*Lock, error)
func (m *Model) Drift() (*DriftResult, error)
func (m *Model) ImpactOf(ids ...string) (items []string, files []string)
```

**失败行为。** 锁文件缺失不算错误；此时报告"未封存"，以便新仓库可以首次封存。

**细化自。** ARC-902

<!-- sdd:item id=HLD-907 stage=hld status=approved derives_from=ARC-904 -->
### HLD-907 — 阶段门禁

**目的。** 评估进入某阶段所声明的前置条件。

**公开接口面。**

```go
func (m *Model) Gate(stage string) (*GateResult, error)
```

**失败行为。** 无法识别的前置条件让门禁失败而非被忽略，因此配置中的拼写错误不会削弱门禁。

**细化自。** ARC-904

<!-- sdd:item id=HLD-908 stage=hld status=approved derives_from=ARC-904 -->
### HLD-908 — 矩阵与图渲染

**目的。** 面向人与文档呈现溯源关系。

**公开接口面。**

```go
func (m *Model) Matrix() []MatrixRow
func (m *Model) RenderMatrix() string
func (m *Model) RenderGraph() string
```

**细化自。** ARC-904

<!-- sdd:item id=HLD-909 stage=hld status=approved derives_from=ARC-904 -->
### HLD-909 — 命令行接口

**目的。** 以子命令形式暴露引擎，保持一致的参数与退出码。

**公开接口面。** `validate`、`lint`、`trace`、`drift`、`seal`、`gate`、`impact`、
`matrix`、`graph`、`stats`。

**失败行为。** 退出码 `0` 表示干净，`1` 表示存在问题或门禁失败，`2` 表示用法或 I/O 错误。

**细化自。** ARC-904

## 5. 详细设计

<!-- sdd:item id=DLD-0101 stage=dld status=approved derives_from=HLD-901 -->
### DLD-0101 — 配置加载器

**文件。** `internal/sdd/config.go`

**行为。** 读取 `.sdd/config.json`；拒绝空阶段列表、空前缀或重复前缀、未知父前缀；在
未设置时为锁文件路径填入默认值。`PrefixOf` 在第一个连字符处切分标识符，格式非法时返回空。

**检查点。** TC-9002 — 非法父前缀被拒绝。

<!-- sdd:item id=DLD-0102 stage=dld status=approved derives_from=HLD-902 -->
### DLD-0102 — 问题、严重级别与报告

**文件。** `internal/sdd/finding.go`

**行为。** 严重级别顺序为 `info < warn < error`；`Sorted` 按严重级别降序、再按类别、再按
主体排序；`Summary` 汇总各级别数量。

**检查点。** TC-9014 — 问题按最严重优先排序。

<!-- sdd:item id=DLD-0103 stage=dld status=approved derives_from=HLD-903 -->
### DLD-0103 — Markdown 与源码解析器

**文件。** `internal/sdd/parse.go`

**行为。**

1. 消费可选的 `---` front matter，按 `key: value` 解析。
2. 跟踪围栏代码块，并完全跳过其内容。
3. 遇到 `sdd:item` 锚点时，在当前行关闭已打开的条目并开启新条目；锚点之后的第一个标题
   决定该条目的标题与标题层级。
4. 在下一个锚点处，或在层级小于等于该条目自身层级的下一个标题处，关闭已打开的条目。
5. 对源码，仅接受注释行（`//` 或 `*`）上的锚点。

**不变量。** 条目正文永不重叠。无锚点的文档产出零个条目。

**检查点。** TC-9001 — 锚点、标题与正文被正确提取。

<!-- sdd:item id=DLD-0104 stage=dld status=approved derives_from=HLD-904 -->
### DLD-0104 — 模型装配

**文件。** `internal/sdd/model.go`

**行为。** 按排序顺序解析文档根目录下的所有文档；将重复标识符合并为额外的语言呈现，若其
属性不一致则报告结构不匹配；条目先按阶段序、再按字典序排列；构建子节点索引，并为每个代码
锚点添加一个合成的 `file:<path>` 子节点。

**复杂度。** 与文档总字节数成线性；图遍历与边数成线性。

**检查点。** TC-9009 — 后代集合包含合成的源文件节点。

<!-- sdd:item id=DLD-0105 stage=dld status=approved derives_from=HLD-905 -->
### DLD-0105 — Lint 与 trace

**文件。** `internal/sdd/check.go`

**行为。** `Lint` 按基础路径对文档分组，要求每种已配置语言都存在，并将主语言呈现与其他
每种呈现在版本、状态、条目序列与标题层级骨架上逐项比对。`Trace` 校验阶段与前缀是否一致、
父条目是否存在及是否合法，用三色深度优先搜索检测环，解析代码锚点，并报告未实现的设计、
未验证的测试用例与未测的需求。

**检查点。** TC-9003、TC-9004、TC-9005、TC-9006、TC-9007。

<!-- sdd:item id=DLD-0106 stage=dld status=approved derives_from=HLD-906 -->
### DLD-0106 — 封存、漂移与影响面

**文件。** `internal/sdd/drift.go`

**行为。** `Seal` 为每个条目写入 `{hash, stage, status, file}` 以及一个 UTC 时间戳。
`Drift` 将每个条目归类为 modified、added 或 removed，随后计算
`DescendantsOf(modified ∪ removed)`，并把合成的 `file:` 节点分离到失效文件列表。已作为
modified 报告的条目不会在 stale 列表中重复出现。

**不变量。** 刚封存且未编辑的规格树满足 `Clean() == true`。

**检查点。** TC-9008、TC-9009、TC-9010。

<!-- sdd:item id=DLD-0107 stage=dld status=approved derives_from=HLD-907 -->
### DLD-0107 — 阶段门禁评估

**文件。** `internal/sdd/gate.go`

**行为。** 名为 `<stage>-approved` 的前置条件要求该阶段前缀下的所有条目状态为
`approved`、`superseded` 或 `deferred`。具名前置条件 `no-drift`、`no-orphan-goals`、
`no-orphan-requirements`、`no-unimplemented-dld`、`no-orphan-code`、
`no-untested-requirements` 与 `bilingual-parity` 直接求值。无法识别的名称记入 `Unknowns`
并让门禁失败。

**检查点。** TC-9011。

<!-- sdd:item id=DLD-0108 stage=dld status=approved derives_from=HLD-908 -->
### DLD-0108 — 矩阵与图渲染

**文件。** `internal/sdd/matrix.go`

**行为。** 对每条需求，按前缀把其后代集合切分为架构、设计、任务与测试列；从设计条目的
`impl` 锚点收集实现源文件，从测试用例的 `verify` 锚点收集测试文件；两者皆非空时该行标记
为已覆盖。`RenderGraph` 为每个有条目的阶段输出一个 Mermaid 子图，并为每条合法
`derives_from` 输出一条边。

**检查点。** TC-9012。

<!-- sdd:item id=DLD-0109 stage=dld status=approved derives_from=HLD-909 -->
### DLD-0109 — 命令行接口

**文件。** `cmd/sddctl/main.go`

**行为。** 依据 `os.Args[1]` 分发；解析公共参数 `--root`、`--json`、`--strict`、
`--fail-on-stale`、`--stage`、`--out`。`validate`、`lint`、`trace` 在存在任何 error 级
问题时退出码为 `1`；指定 `--strict` 且存在任何 warn 时同样为 `1`。
`drift --fail-on-stale` 在树不干净时退出码为 `1`。`gate` 在门禁失败时退出码为 `1`。

**检查点。** TC-9013 — 二进制在无外部模块的情况下构建并运行。

## 6. 测试用例

<!-- sdd:item id=TC-9001 stage=verify status=approved derives_from=REQ-9001 -->
### TC-9001 — 锚点、标题与正文被解析

**层级。** unit

**步骤。** 解析一份夹具文档，其中含三个三级标题锚点、一个包含诱饵锚点的围栏代码块，以及
front matter。

**预期。** 恰好三个条目；标题已剥离标识符前缀；围栏内的诱饵不产生条目；正文互不重叠。

**测试函数。** `internal/sdd/parse_test.go` 中的 `TestParseDocExtractsItems`

<!-- sdd:item id=TC-9002 stage=verify status=approved derives_from=REQ-9002 -->
### TC-9002 — 非法边与未知边被报告

**层级。** unit

**步骤。** 构造一棵夹具树，其中含一个派生自 `REQ` 条目的 `DLD` 条目，以及一个派生自不
存在标识符的条目。

**预期。** 一条 `illegalParentStage` 错误与一条 `unknownParent` 错误。

**测试函数。** `internal/sdd/check_test.go` 中的 `TestTraceRejectsIllegalEdges`

<!-- sdd:item id=TC-9003 stage=verify status=approved derives_from=REQ-9003 -->
### TC-9003 — 环会终止并被报告

**层级。** unit

**步骤。** 构造一棵含三条目派生环的夹具树。

**预期。** trace 在测试超时内返回，并报告该环。

**测试函数。** `internal/sdd/check_test.go` 中的 `TestTraceDetectsCycle`

<!-- sdd:item id=TC-9004 stage=verify status=approved derives_from=REQ-9004 -->
### TC-9004 — 缺失对应件是错误

**层级。** unit

**步骤。** 创建一个只包含英文呈现的文档根目录。

**预期。** 一条指明基础路径的 `bilingualMissingCounterpart` 错误。

**测试函数。** `internal/sdd/check_test.go` 中的 `TestLintMissingCounterpart`

<!-- sdd:item id=TC-9005 stage=verify status=approved derives_from=REQ-9005 -->
### TC-9005 — 版本与状态分歧是错误

**层级。** unit

**步骤。** 创建一对 `doc_version` 取值不同的文档。

**预期。** 一条 `bilingualVersionMismatch` 错误。

**测试函数。** `internal/sdd/check_test.go` 中的 `TestLintVersionMismatch`

<!-- sdd:item id=TC-9006 stage=verify status=approved derives_from=REQ-9006 -->
### TC-9006 — 结构分歧是错误

**层级。** unit

**步骤。** 创建一对文档，其中中文呈现少一个条目、多一个标题。

**预期。** 一条指出序列差异的 `bilingualStructureMismatch` 错误。

**测试函数。** `internal/sdd/check_test.go` 中的 `TestLintStructureMismatch`

<!-- sdd:item id=TC-9007 stage=verify status=approved derives_from=REQ-9007 -->
### TC-9007 — 孤儿代码锚点是错误

**层级。** unit

**步骤。** 放置一个源文件，其中含 `// sdd:impl DLD-9999` 以及一个提及该关键字的字符串
字面量。

**预期。** 在注释所在行报告一条 `orphanCodeAnchor` 错误；字符串字面量不产生锚点。

**测试函数。** `internal/sdd/check_test.go` 中的 `TestTraceOrphanCodeAnchor`

<!-- sdd:item id=TC-9008 stage=verify status=approved derives_from=REQ-9008 -->
### TC-9008 — 已封存的树是干净的

**层级。** unit

**步骤。** 封存一棵夹具树，随即计算漂移。

**预期。** `Clean()` 为真，且不产生任何问题。

**测试函数。** `internal/sdd/drift_test.go` 中的 `TestSealThenDriftIsClean`

<!-- sdd:item id=TC-9009 stage=verify status=approved derives_from=REQ-9009 -->
### TC-9009 — 修改目标会级联到后代与源文件

**层级。** integration

**步骤。** 封存一棵夹具树，其目标可达一条需求、一个架构条目、一个设计条目与一个源文件；
修改该目标正文；重新计算漂移。

**预期。** 该目标为 `Modified`；需求、架构与设计条目为 `Stale`；实现源文件出现在
`StaleFile` 中；`Clean()` 为假。

**测试函数。** `internal/sdd/drift_test.go` 中的 `TestDriftCascadesToCode`

<!-- sdd:item id=TC-9010 stage=verify status=approved derives_from=REQ-9010 -->
### TC-9010 — 无需修改即可回答影响面

**层级。** unit

**步骤。** 在未修改的夹具树上查询某目标的影响面。

**预期。** 返回的条目集合与文件集合等于该目标的已知子树。

**测试函数。** `internal/sdd/drift_test.go` 中的 `TestImpactOf`

<!-- sdd:item id=TC-9011 stage=verify status=approved derives_from=REQ-9011 -->
### TC-9011 — 门禁在条件未满足或无法识别时失败

**层级。** unit

**步骤。** 在存在一个未实现设计条目的树上评估 `implement` 门禁；再评估一个声明了无法识别
前置条件的门禁。

**预期。** 前者失败并指名该设计条目；后者失败，且该前置条件列在 `Unknowns` 中。

**测试函数。** `internal/sdd/gate_test.go` 中的 `TestGateImplement`

<!-- sdd:item id=TC-9012 stage=verify status=approved derives_from=REQ-9012 -->
### TC-9012 — 矩阵与图呈现溯源关系

**层级。** unit

**步骤。** 为一棵含一条完全覆盖需求的夹具树渲染矩阵与图。

**预期。** 该矩阵行被标记为已覆盖并列出源文件；图中包含目标到需求的边。

**测试函数。** `internal/sdd/matrix_test.go` 中的 `TestMatrixAndGraph`

<!-- sdd:item id=TC-9013 stage=verify status=approved derives_from=REQ-9013,REQ-0091 -->
### TC-9013 — 模块未声明任何外部依赖

**层级。** governance

**步骤。** 读取仓库根目录的 `go.mod`。

**预期。** 不存在任何指向标准库之外模块的 `require` 指令。

**测试函数。** `internal/sdd/deps_test.go` 中的 `TestNoExternalDependencies`

<!-- sdd:item id=TC-9014 stage=verify status=approved derives_from=REQ-9001 -->
### TC-9014 — 问题按最严重优先排序

**层级。** unit

**步骤。** 向报告中加入不同严重级别的问题并排序。

**预期。** error 先于 warn，warn 先于 info；同级按类别、再按主体排序。

**测试函数。** `internal/sdd/finding_test.go` 中的 `TestReportSorted`
