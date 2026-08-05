---
id: SPEC-<NNN>
lang: zh
counterpart: spec.en.md
doc_version: 0.1.0
status: draft
stage: specify
---

# <特性名称> — 需求规格

## 阅读指引

需求描述的是**做什么**，绝不是**怎么做**。指名了某个包、某个库或某个函数的需求，应当
下沉到设计阶段。

每条需求都必须可测试：它陈述的是一个测试能够断言的可观察条件。

## 术语表

| 术语 | 定义 |
|---|---|

## 功能需求

<!-- sdd:item id=REQ-0001 stage=specify status=draft derives_from=G-001 priority=P0 -->
### REQ-0001 — <需求标题>

**需求。** 系统必须（MUST）<可观察行为>。

**理由。** <为什么，并关联到父目标。>

**验收。**
- 给定 <前置条件>，当 <动作> 时，则 <可观察结果>。

**验证方式。** TC-XXXX

## 非功能需求

<!-- sdd:item id=REQ-09XX stage=specify status=draft derives_from=G-00X priority=P1 -->
### REQ-09XX — <质量属性>

**需求。** <有边界、可度量的质量约束。>

**验收。** <阈值及其度量方式。>

**验证方式。** TC-XXXX

## 范围之外

| # | 排除的行为 | 原因 |
|---|---|---|

## 溯源摘要

| 需求 | 父目标 | 优先级 | 测试用例 |
|---|---|---|---|
