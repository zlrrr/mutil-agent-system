---
id: TASKS-<NNN>
lang: zh
counterpart: tasks.en.md
doc_version: 0.1.0
status: draft
stage: plan
---

# <特性名称> — 任务分解

## 规则

- 只有当任务的检查点测试通过时，该任务才算完成。
- 无法说清自己检查点的任务，说明拆解得还不够。
- 标记 `[P]` 的任务可以并行执行（文件互不相交，且无顺序依赖边）。

<!-- sdd:item id=T-001 stage=plan status=draft derives_from=DLD-0001 -->
### T-001 — <任务标题>

**实现。** DLD-XXXX

**文件。** `<该任务创建或修改的路径>`

**完成定义。**
- [ ] 存在 `// sdd:impl DLD-XXXX` 锚点
- [ ] TC-XXXX 通过
- [ ] `go vet ./...` 无告警

**阻塞于。** T-XXX | 无

**可并行。** 是 | 否

## 执行顺序

| 批次 | 任务 | 进入下一批次的门禁 |
|---|---|---|

## 进度记录

| 任务 | 开始 | 结束 | 检查点结果 |
|---|---|---|---|
