---
id: CHECKPOINTS
lang: zh
counterpart: checkpoints.en.md
doc_version: 1.0.0
status: approved
stage: constitution
---

# 检查点记录

CON-005 要求：任何设计主张若没有明确的检查点、测试用例标识符以及该测试的实际运行记录，就
不得标记为"完成"。CON-010 要求自主循环记录它做了什么、以及留下了什么没做。本文就是这份
记录。

## 批次门禁

每个批次只有在其门禁命令以零码退出时才关闭。

| 批次 | 任务 | 门禁命令 | 结果 | 备注 |
|---|---|---|---|---|
| 0 | SDD 框架 | `go run ./cmd/sddctl validate` | 通过 | 65 个条目、40 个锚点、无问题。引擎首先抓到的是它**自己**的引导违规——`sddctl` 源码先于其 DLD 条目存在，报告为 9 条 `orphanCodeAnchor` 错误——通过编写 `specs/000-sdd-framework` 解决 |
| 1 | T-001、T-002 | `go build ./... && go test ./internal/domain/` | 通过 | TC-0002、TC-0003、TC-0050、TC-0070、TC-0074 |
| 2 | T-003、T-004 | `go test -race ./internal/store/ ./internal/eventbus/ ./internal/signal/` | 通过 | TC-0004、TC-0005、TC-0011、TC-0012、TC-0016、TC-0064 |
| 3 | T-005、T-006 | `go test ./internal/catalog/` | 通过 | TC-0082；5 条签名、3 个故障样例、5 篇 runbook |
| 4 | T-007、T-008 | `go test ./internal/reasoner/` | 通过 | TC-0020、TC-0022、TC-0023、TC-0024、TC-0030 至 TC-0036、TC-0072 |
| 5 | T-009、T-010 | `go test ./internal/agent/ ./internal/policy/` | 通过 | TC-0001、TC-0013、TC-0014、TC-0015、TC-0040、TC-0041、TC-0043、TC-0044 |
| 6 | T-011 | `go test -race ./internal/orchestrator/` | 通过 | TC-0010、TC-0021、TC-0031、TC-0035、TC-0042、TC-0045、TC-0046、TC-0051、TC-0060、TC-0061、TC-0062、TC-0070、TC-0071、TC-0073 |
| 7 | T-012、T-013、T-014 | `make check` | 通过 | TC-0053、TC-0063、TC-0064、TC-0065、TC-0080、TC-0081、TC-0090、TC-0091 |

## 检查点抓到的缺陷

以下是门禁抓到的问题，以及由此产生的改动。之所以记录，是因为一个从不失败的检查点不算
检查点。

| # | 发现方 | 问题 | 处理 |
|---|---|---|---|
| D1 | 批次 0 的 `sddctl trace` | 治理工具自身的源码先于它所实现的设计条目存在 | 先编写覆盖 DLD-0101..0109 的 `specs/000-sdd-framework`，再继续 |
| D2 | 首次端到端运行 | 变更采集器的默认趟使用了分析窗口（起点比告警早七分钟），因此它立刻就找到了配置变更，质疑者的索证也就失去了意义 | 默认趟改为从告警时刻起查询；只有当索证要求时才施加扩展回看（DLD-1040） |
| D3 | 首次端到端运行 | `change_correlation` 拿变更与 case 中**最早**的任意异常比较。流量序列动得更早，于是真正的原因得分为零 | 改为与该假设**自身**支持指标的起点比较（`HypothesisOnset`，DLD-1032） |
| D4 | 首次端到端运行 | 所有裁决都是空：`CombineVerdicts` 把未设置的裁决视为最严重，于是它压过了所有真实判定 | 跳过未设置的裁决；"尚未判定"不再压过一个决定 |
| D5 | 首次端到端运行 | 第 1 轮的 `revise` 裁决延续到第 2 轮，因此回应了挑战的证据永远无法解除它 | 裁决不再跨轮携带；质疑者针对本轮证据重新审查每个假设（DLD-1033） |
| D6 | TC-0062 | 分诊步骤发出的 `agent_completed` 没有耗时，事件日志中的工作记录因此不完整 | 分诊现在会对自身计时，并给出它计划扇出的采集器清单 |
| D7 | `sddctl gate --stage architect` | 有 13 条需求没有任何架构条目派生自它——这是真实的覆盖缺口，不是工具产物 | 扩展 ARC-003、ARC-004、ARC-006、ARC-010、ARC-011 与 ARC-014 以认领它们 |
| D8 | TC-0091 | REQ-0091（无第三方依赖）没有已执行的测试，因为验证它的那个测试只链接到框架自身的需求 | TC-9013 现在同时派生自 REQ-9013 与 REQ-0091 |

## 已执行的级联更新

CON-003 要求上游变更必须一路跟到每个后代。本里程碑期间执行了两次级联。

| 触发 | 影响集合 | 动作 |
|---|---|---|
| 架构覆盖缺口（D7） | ARC 条目及其"实现需求"正文 | 双语同时更新锚点与正文；重跑 `sddctl lint`；重新封存规格树 |
| 实测分数与 DLD 预估算术不符 | DLD-DOC-001 第 11 节，双语 | 把设计中的算例修正为实现实际产出的数值（第 1 轮：0.49 / 0.40 / 0.35；第 2 轮：0.94 / 0.37 / 0.29）。设计的**定性**主张——领先者是错的、差值低于间距、随后发生重排——依然成立 |

第二次级联值得直说：详细设计原本预估的是 0.47 / 0.46 / 0.43 与 0.96 / 0.43 / 0.27。实现
产出了不同数字，因为 runbook 相似度项的解析方式与预估假设不同。我们修正了设计文档，而不是
让这些数字悄悄失效——这正是 CON-003 存在的意义。

## 最终验证

| 检查 | 命令 | 结果 |
|---|---|---|
| 构建 | `go build ./...` | 通过 |
| Vet | `go vet ./...` | 通过 |
| 格式 | `gofmt -l cmd internal` | 无输出 |
| 测试 | `go test ./...` | 通过 |
| 竞态检测下的测试 | `go test -race ./...` | 通过 |
| 无第三方依赖 | `go.mod` 无 `require`；无 `go.sum` | 通过 |
| 治理 | `go run ./cmd/sddctl validate` | 通过，无问题 |
| 交付门禁 | `go run ./cmd/sddctl gate --stage deliver` | 通过 |
| 参考场景 | `go run ./cmd/arena demo --case C1` | 通过 |
| 评估 | `go run ./cmd/evalctl run` | 通过 |

## 哪些没做，以及为什么

在此明确记录，而不是靠"没提"来暗示。

| 项目 | 状态 | 原因 |
|---|---|---|
| 容器镜像的构建与运行 | 定义已写出并由 TC-0090 断言；未实际执行 | 开发环境中没有可用的 Docker 守护进程。Dockerfile、compose 栈以及"构建镜像并做健康检查"的 CI 作业均已就位；镜像作业运行在有守护进程的机器上 |
| 在线 Prometheus 与容器日志适配器 | 端口已定义、夹具适配器已实现、在线适配器未实现 | 里程碑 M4。参考场景与整套测试刻意不依赖它们（CON-007） |
| 基于模型的推理器 | 端口已定义并有文档；未接入适配器 | 章程中的开放问题 Q1：尚未选定服务商。确定性适配器是设计上的默认，而非遗漏（ADR-002） |
| 故障样例 C4–C6 | 未编写 | 里程碑 M5。三个样例足以满足 M1 门禁；尤其是日志误导样例是用来检验过拟合的，而只有当目录不再与它同步生长时，这项检验才公平 |
| 多租户认证 | 未实现 | 章程非目标 N6 |
| 把分支推送到 GitHub | 被阻塞 | 见下方升级记录。所有提交都已存在于本地分支 `claude/project-spec-architecture-78dmtj`；内容没有丢失，但远端尚未收到 |

## 升级记录

有一条，且它超出了规格自身能够解决的授权范围。

**推送到远端被组织策略阻断。** `git push` 返回 `403`，GitHub API 直接给出了原因：

> GitHub access is not enabled for this session. An org admin must connect the Claude
> GitHub App for this organization.

读取是可用的——`git ls-remote` 与 API 的读取端点都成功——因此这是一条授权边界，而不是网络
或凭据故障，重试没有意义。三条写入路径均已尝试，且都被拒绝：

| 路径 | 结果 |
|---|---|
| 经 HTTPS 的 `git push` | `403` |
| 用会话令牌调用 `POST /repos/.../git/refs` | `403 GitHub access is not enabled for this session` |
| GitHub App 集成 | `403 Resource not accessible by integration` |

代理未报告任何中继失败，说明传输环节没有丢包；这次拒绝是在授权层被明确发出的。因此，
改用 API 逐文件写出不只是不现实——它同样不被允许；何况那样做还会把九次提交压成一次，
丢弃掉本身就属于本里程碑交付内容的提交历史。

**维护者需要做什么。** 为该组织连接 Claude GitHub App，或为本会话授予
`zlrrr/mutil-agent-system` 的写权限，然后重新执行：

```bash
git push -u origin claude/project-spec-architecture-78dmtj
```

所有提交都已按顺序、连同提交信息完成。该分支就其现状而言已经完整。

本里程碑期间没有其他决策需要规格之外的授权：每一处歧义都能由章程、需求或宪章解决。
章程记录的三个开放问题（Q1–Q3）没有阻塞任何 M1 工作，各自都按章程声明的默认假设推进。
