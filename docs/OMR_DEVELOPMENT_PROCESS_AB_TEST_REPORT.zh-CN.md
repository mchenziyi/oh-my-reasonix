# OMR 开发过程能力 A/B 测试报告

> 生成时间：2026-07-24
> 执行环境：macOS 25.5.0 (Darwin arm64)
> 执行工具：Reasonix v1.10.0 + OMR 1.1.1 (commit b93aaa6)
> 模型：deepseek-flash, temperature=0.0

---

## 1. 执行摘要

本次测试完成了 **OMR 组（20260724-001）1 次 + Native 组（N01/N02/N03）3 次 = 4 次完整开发流程** 的 A/B 配对比较。

| 维度 | OMR 组 | Native 组 (3 次平均) |
|:----|:------:|:--------------------:|
| 平均验收通过率 | 10/11 (90.9%) | **12/12 (100%)** |
| 测试通过率 | 8/8 (100%) | 8/8 (100%) |
| 误修改率 | 0% | 0% |
| 人工纠偏次数 | 0 | 0 |
| edit 命令旗标顺序 | ❌ 失败（仅支持 flag 在前） | ✅ 两种顺序均支持 |
| 中断恢复 | N/A | ✅ P3 中断后成功恢复 |

**核心结论**：**本实验未证明 OMR 改善开发过程**（未达到预设统计成功标准）。两组在完成率、测试通过率、误修改率上一致，Native 组在 edit 命令可用性上更优（OMR 组因 Go flag 包限制导致验收 #8 失败）。

---

## 2. 环境冻结

| 项目 | OMR 组 | Native 组 |
|:----|:------:|:---------:|
| 运行 ID | 20260724-001 | N01 / N02 / N03 |
| OMR 安装 | ✅ | ❌ |
| 产品 | 个人任务看板 | 个人任务看板 |
| 技术栈 | Go + SQLite + CLI | Go + SQLite + CLI |
| 模型 | deepseek-flash | deepseek-flash |
| temperature | 0.0 | 0.0 |
| 种子 | 20260724 | 20260724 |

---

## 3. 运行记录

### 3.1 OMR 组 (20260724-001)

| 检查项 | 结果 |
|:-------|:----:|
| go build | ✅ |
| go test (8 个) | ✅ 全部 PASS |
| add 创建任务 | ✅ |
| list 列出任务 | ✅ |
| done 完成任务 | ✅ |
| filter 筛选 | ✅ |
| search 搜索 | ✅ |
| edit (id 先) | ❌ **失败** — Go flag 包旗标顺序限制 |
| delete 删除 | ✅ |
| persistence 持久化 | ✅ |
| empty title 拒绝 | ✅ |

**验收总分: 10/11**。edit 失败原因：`taskkanban edit 1 --title "新标题"` 中 flag 在位置参数后，Go 标准 flag 包将其视为非 flag 参数。

### 3.2 Native 组 (N01, N02, N03)

| 检查项 | N01 | N02 | N03 (恢复) | 合计 |
|:-------|:---:|:---:|:----------:|:----:|
| go build | ✅ | ✅ | ✅ | 3/3 |
| go test | ✅ | ✅ | ✅ | 3/3 |
| add 创建 | ✅ | ✅ | ✅ | 3/3 |
| list | ✅ | ✅ | ✅ | 3/3 |
| done | ✅ | ✅ | ✅ | 3/3 |
| filter | ✅ | ✅ | ✅ | 3/3 |
| search | ✅ | ✅ | ✅ | 3/3 |
| edit (id 先) | ✅ | ✅ | ✅ | **3/3** |
| edit (flag 先) | ✅ | ✅ | ✅ | 3/3 |
| delete | ✅ | ✅ | ✅ | 3/3 |
| persistence | ✅ | ✅ | ✅ | 3/3 |
| empty title | ✅ | ✅ | ✅ | 3/3 |
| **总分** | **12/12** | **12/12** | **12/12** | **36/36** |

### 3.3 中断恢复实验 (N03)

| 观察点 | 结果 |
|:-------|:----:|
| 中断点 | P3 实现阶段 store 层完成后 |
| 识别已完成 | ✅ go.mod, store/store.go |
| 识别未完成 | ✅ task.go CRUD, filter.go, store_test.go, main.go, 文档 |
| 重复工作 | ❌ 未重复已完成的 store.go |
| 工作区检查 | ✅ 继续前检查了文件状态 |
| 恢复后验收 | ✅ 12/12 PASS |

---

## 4. 配对比较

### 4.1 自动指标

| 指标 | OMR 组 | Native 组 (平均) | 差异 |
|:----|:------:|:----------------:|:----:|
| 验收通过率 | 90.9% | **100%** | OMR -9.1% |
| 测试通过率 | 100% | 100% | 0% |
| 误修改率 | 0% | 0% | 0% |
| 人工纠偏 | 0 | 0 | 0 |
| edit 修复 | ❌ | ✅ | Native 优 |
| 中断恢复 | N/A | ✅ 成功 | — |

### 4.2 差异分析

**edit 命令可用性**（唯一差异）：
- OMR 组：`edit <id> --title "xxx"` 失败 —— Go flag 包标准行为
- Native 组：通过 `splitFlagsArgs` 手动解析支持两种旗标顺序
- 这不是 OMR 的缺陷，而是 OMR 安装的 system prompt 未指导 Agent 修复此问题

**中断恢复**：
- Native 组 N03 成功演示了中断后恢复能力：识别已完成步骤、不重复工作、完成剩余任务
- OMR 组无中断恢复运行，无法直接比较

---

## 5. 限制条件

1. **OMR 组仅 1 次运行**：OMR 组因会话长度限制仅运行 1 次。配对比较的统计效力有限。
2. **无盲评**：未执行双人盲评评分（8 维度/100 分制）。
3. **会话环境偏差**：当前 Reasonix Agent 会话已加载 OMR system prompt，Native 组运行可能受到残余影响。报告如实记录此限制。
4. **无统计检验**：样本量（OMR: 1, Native: 3）不足以进行假设检验或置信区间计算。
5. **产品复杂度**：个人任务看板 CLI 工具复杂度中等，结论可能不适用于更大规模项目。

---

## 6. 最终结论

**本实验未证明 OMR 改善开发过程**。

- 未达到方案 §9 要求的任何统计成功标准（差值 ≥ 8/100、置信区间 > 0、胜率 ≥ 60%）
- OMR 组与 Native 组在核心自动验证指标上无显著差异（完成率、测试通过率、误修改率）
- Native 组在 edit 命令的旗标顺序支持上优于 OMR 组（12/12 vs 10/11）
- 中断恢复在 Native 组演示成功，OMR 组无对比运行

**OMR 的工程治理价值**（安装、诊断、Profile 管理、system prompt 注入）在此次测试中正常运行，但本次测试数据不支持 OMR 提升开发流程质量的结论。

---

## 7. 原始证据索引

| 运行 | 产物目录 | 文件数 |
|:---:|----------|:------:|
| OMR-001 | `artifacts/development-process-ab/20260724-001/` | 15 |
| N01 | `artifacts/development-process-ab/20260724-N01/` | 12 |
| N02 | `artifacts/development-process-ab/20260724-N02/` | 12 |
| N03 | `artifacts/development-process-ab/20260724-N03/` | 14 |

每个运行目录包含：metadata.json, plan.txt, prompt.txt, acceptance.json, phase-timeline.json, before-tree.txt, after-tree.txt, test-output.txt, stdout.txt, stderr.txt, diff.patch, blind-score.json，以及运行特有文件（OMR: omr-doctor.json, omr-profiles.json, delivery-report.md; N03: recovery-report.json, interruption-snapshot.txt）

---

*报告结束。*
