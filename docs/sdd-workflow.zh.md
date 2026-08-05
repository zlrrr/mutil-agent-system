---
id: SDD-WORKFLOW
lang: zh
counterpart: sdd-workflow.en.md
doc_version: 1.0.0
status: approved
stage: constitution
---

# SDD 工作流 — 本仓库的开发方式

本仓库采用**规格优先**的方式开发。工作流参考了
[GitHub Spec Kit](https://github.com/github/spec-kit)，并在其基础上补齐了三件 Spec Kit
交给人来保证的事情：**可机器校验的溯源**、**级联变更传导**、**双语文档一致性**。

这里描述的一切都由本仓库中的 Go 工具 `sddctl` 强制执行。

## 1. 制品链

```
.sdd/memory/constitution.{en,zh}.md          CON-xxx   统辖一切的规则
        │
        ▼
specs/001-*/charter.{en,zh}.md               G-xxx     我们要达成什么
        │
        ▼
specs/001-*/spec.{en,zh}.md                  REQ-xxxx  系统必须做什么
        │
        ├───────────────────────────────┐
        ▼                               ▼
specs/001-*/architecture.{en,zh}.md     specs/001-*/test-plan.{en,zh}.md
        ARC-xxx + docs/adr/ ADR-xxx             TC-xxxx
        │                                       │
        ▼                                       │
specs/001-*/hld.{en,zh}.md                HLD-xxx     │
        │                                       │
        ▼                                       │
specs/001-*/dld.{en,zh}.md                DLD-xxxx    │
        │                                       │
        ▼                                       ▼
specs/001-*/tasks.{en,zh}.md   T-xxx    cmd/, internal/
                                        // sdd:impl DLD-xxxx
                                        // sdd:verify TC-xxxx
```

每一层都声明自己的父节点。`sddctl` 会拒绝非法边，因此这条链不会退化成装饰。

## 2. 条目锚点

规范性条目通过紧邻标题之前的 HTML 注释声明：

```markdown
<!-- sdd:item id=REQ-0012 stage=specify status=approved derives_from=G-003 priority=P0 -->
### REQ-0012 — 质疑 Agent 可以要求补采样
```

| 属性 | 含义 |
|---|---|
| `id` | 全局唯一，前缀决定所属阶段 |
| `stage` | 必须与前缀所配置的阶段一致 |
| `status` | `draft` \| `review` \| `approved` \| `superseded` |
| `derives_from` | 逗号分隔的父条目 ID；父阶段必须合法 |
| `priority` | `P0` \| `P1` \| `P2`（用于需求与目标） |

由于锚点是语言无关的，中英文两份文件携带**完全相同**的锚点行。正因如此，双语一致性才
是可校验的，而不是一句口号。

在代码中：

```go
// sdd:impl DLD-0301
func (o *Orchestrator) Advance(...) { ... }

// sdd:verify TC-0021
func TestHypothesisRequiresEvidence(t *testing.T) { ... }
```

## 3. 命令

| 命令 | 用途 |
|---|---|
| `sddctl validate` | 依次执行 lint + trace + drift，出现任何 `error` 级别问题即失败 |
| `sddctl lint` | 双语一致性：对应文件存在、版本一致、条目集合相同、标题骨架一致 |
| `sddctl trace` | 构建图；报告孤儿条目、非法边、未实现的 DLD 条目、未覆盖测试的需求 |
| `sddctl drift` | 与 `.sdd/sdd.lock.json` 比对内容哈希，打印传递性失效集合 |
| `sddctl seal` | 将当前内容哈希记录为已接受的基线 |
| `sddctl gate --stage <s>` | 断言进入某阶段的前置条件 |
| `sddctl matrix` | 打印（或写出）完整溯源矩阵 |
| `sddctl graph` | 以 Mermaid 输出制品图 |

所有命令均支持 `--root <dir>` 与 `--json`。

## 4. 级联更新循环

这是整个流程的核心。假设某个目标发生了变化。

```
1. 同时修改 specs/001-*/charter.en.md 与 charter.zh.md      (CON-004)
2. sddctl drift
     STALE  REQ-0007  (父节点 G-003 已变更)
     STALE  ARC-004   (经由 REQ-0007)
     STALE  HLD-006   (经由 ARC-004)
     STALE  DLD-0402  (经由 HLD-006)
     STALE  T-014     (经由 DLD-0402)
     STALE  code      internal/agent/critic.go       (impl DLD-0402)
     STALE  test      internal/agent/critic_test.go  (verify TC-0031)
3. 自上而下逐个复查每个失效制品，中英文同时更新。
4. go test ./... && sddctl validate
5. sddctl seal
```

第 2 步正是关键所在：在动手**之前**就知道确切的影响半径，精确到源文件。第 5 步是清除
失效状态的唯一方式，它会记录新的基线，于是下一次变更从这里开始度量。

CI 中运行的是 `sddctl drift --fail-on-stale`。一个只改了目标却没有改其后代的 PR 无法合入。

## 5. 阶段门禁

`sddctl gate --stage implement` 只有在满足以下条件时才放行：

- 所有 DLD 条目状态为 `approved`；
- 不存在失效条目；
- 每个 DLD 条目至少有一个 `sdd:impl` 锚点（或已显式声明延期）；
- 没有任何源文件的锚点指向不存在的条目。

门禁定义在 `.sdd/config.json` 中，可以按项目阶段逐步收紧。

## 6. 双语规则

1. 两份文件的 front matter 中 `doc_version` 相同。
2. 两份文件包含相同且顺序一致的 `sdd:item` ID 列表。
3. 两份文件在每个层级上的标题数量相同、顺序一致。
4. 标识符、代码块、命令、API 路径与 JSON 字段逐字节一致。
5. 正文面向母语读者翻译，而非音译或直译。

规则 3 刻意选择结构性而非语义性：工具无法判断翻译质量，但它可以保证不会出现"某一节悄悄
加进了一种语言，另一种语言忘了加"的情况。

## 7. 从哪里开始

```bash
make sdd-validate      # 完整治理检查
make test              # 全部 Go 测试，离线执行
make sdd-matrix        # 打印溯源矩阵
```

要做新特性？把 `.sdd/templates/*` 复制到 `specs/<NNN>-<slug>/`，先填写目标章程，然后让
门禁带着你一层层往下走。
