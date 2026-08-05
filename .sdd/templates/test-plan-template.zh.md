---
id: TESTPLAN-<NNN>
lang: zh
counterpart: test-plan.en.md
doc_version: 0.1.0
status: draft
stage: verify
---

# <特性名称> — 测试计划

## 策略

| 层级 | 范围 | 执行器 | 是否必须离线可跑 |
|---|---|---|---|

## 准入 / 退出条件

**准入。** <何时可以开始测试。>

**退出（MVP 门禁）。** <精确、可计数的条件。>

<!-- sdd:item id=TC-0001 stage=verify status=draft derives_from=REQ-0001 -->
### TC-0001 — <测试用例标题>

**层级。** unit | integration | e2e | policy | governance

**验证。** REQ-XXXX

**前置条件。** <夹具与状态。>

**步骤。**
1. <步骤>

**预期。** <精确断言。不是"工作正常"。>

**测试函数。** `<path>_test.go` 中的 `TestXxx`

## 覆盖矩阵

| 需求 | 优先级 | 测试用例 | 状态 |
|---|---|---|---|

## 检查点记录

| 检查点 | 日期 | 命令 | 结果 | 备注 |
|---|---|---|---|---|
