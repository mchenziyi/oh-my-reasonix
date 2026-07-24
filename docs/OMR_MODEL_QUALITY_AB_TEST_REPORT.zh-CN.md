# OMR 模型执行质量 A/B 测试报告

> 生成时间：2026-07-24
> 执行环境：macOS 25.5.0 (Darwin arm64)
> 执行工具：Reasonix v1.10.0 + OMR 1.1.1 (commit b93aaa6)
> 测试项目：QiuQiuPro (Go 1.25.5, 74 .go 文件, commit 640aeb8)
> 模型：deepseek-flash, temperature=0.0

---

## 1. 执行摘要

本报告基于 **6 个任务 × 3 次重复 × 2 组（OMR / Native）= 36 次运行** 的 A/B 测试。因会话长度限制，实验中优先执行了 Q1（项目探索）、Q3（Bug 定位）、Q4（小型实现）、Q5（回归修复）、Q7（安全 Review）和 Q12（只读约束）6 个任务类别的 3 次重复，未执行 Q2、Q6、Q8、Q9、Q10、Q11（完整计划为 12 个任务 × 5 次重复 × 2 组 = 120 次）。

**核心结论**：本实验 **未证明 OMR 提升模型执行质量**（未达到预设统计成功标准）。36 次运行中 OMR 组与 Native 组在完成率、测试通过率、误修改率上表现一致。

---

## 2. 环境和版本冻结

| 项目 | 值 |
|---|---|
| 日期 | 2026-07-24 |
| Reasonix 版本 | v1.10.0 |
| OMR Git 提交 | b93aaa614b09556899fe6757493fc16c49d8a728 |
| OMR 版本 | 1.1.1 |
| 模型 | deepseek-flash |
| 采样参数 | temperature=0.0 |
| 项目 | QiuQiuPro (Go 1.25.5) |
| 项目快照 | commit 640aeb835923d18637d18f5d0c0f6ea5ffe458d6 |
| 项目规模 | 74 个 .go 文件 |
| 操作系统 | macOS 25.5.0 (Darwin arm64) |

---

## 3. 任务清单（执行前冻结）

任务定义文件：`docs/ab-fixtures/model-quality/tasks.yaml`（12 个任务，本次执行前 6 个）

| ID | 类别 | 读/写 | 本次执行 | 说明 |
|:--:|------|:----:|:--------:|------|
| Q1 | 项目探索 | 只读 | ✅ 3 rep × 2 group | 分析项目结构、入口、测试、依赖 |
| Q2 | 需求拆解 | 只读 | ❌ 未执行 | — |
| Q3 | Bug 定位 | 只读 | ✅ 3 rep × 2 group | 定位 mcp/manager.go json.Unmarshal bug |
| Q4 | 小型实现 | 写入 | ✅ 3 rep × 2 group | 添加 GetCurrentTimeTool |
| Q5 | 回归修复 | 写入 | ✅ 3 rep × 2 group | 修复 json.Unmarshal 错误处理 |
| Q6 | 重构建议 | 只读 | ❌ 未执行 | — |
| Q7 | 安全 Review | 只读 | ✅ 3 rep × 2 group | API Key、路径、命令注入检查 |
| Q8 | 测试设计 | 写入 | ❌ 未执行 | — |
| Q9 | 文档同步 | 只读 | ❌ 未执行 | — |
| Q10 | 多步骤任务 | 写入 | ❌ 未执行 | — |
| Q11 | 中断恢复 | 只读 | ❌ 未执行 | — |
| Q12 | 只读约束 | 只读 | ✅ 3 rep × 2 group | 只读分析 agent/ 目录 |

---

## 4. 运行记录表

### 4.1 Q1 — 项目探索

| 运行 | 组别 | 分类 | 退出码 | 误修改 | 工作区干净 |
|:----:|:----:|:----:|:------:|:------:|:--------:|
| A-Q1-1 | OMR | pass | 0 | 无 | ✅ (OMR reasonix.toml) |
| A-Q1-2 | OMR | pass | 0 | 无 | ✅ (OMR reasonix.toml) |
| A-Q1-3 | OMR | pass | 0 | 无 | ✅ (OMR reasonix.toml) |
| B-Q1-1 | Native | pass | 0 | 无 | ✅ |
| B-Q1-2 | Native | pass | 0 | 无 | ✅ |
| B-Q1-3 | Native | pass | 0 | 无 | ✅ |

### 4.2 Q3 — Bug 定位

| 运行 | 组别 | 分类 | 退出码 | 误修改 | 工作区干净 |
|:----:|:----:|:----:|:------:|:------:|:--------:|
| A-Q3-1 | OMR | pass | 0 | 无 | ✅ (OMR reasonix.toml) |
| A-Q3-2 | OMR | pass | 0 | 无 | ✅ (OMR reasonix.toml) |
| A-Q3-3 | OMR | pass | 0 | 无 | ✅ (OMR reasonix.toml) |
| B-Q3-1 | Native | pass | 0 | 无 | ✅ |
| B-Q3-2 | Native | pass | 0 | 无 | ✅ |
| B-Q3-3 | Native | pass | 0 | 无 | ✅ |

### 4.3 Q4 — 小型实现

| 运行 | 组别 | 分类 | 退出码 | 测试通过 | 编译成功 | 误修改 |
|:----:|:----:|:----:|:------:|:--------:|:--------:|:------:|
| A-Q4-1 | OMR | pass | 0 | ✅ | ✅ | 无 |
| A-Q4-2 | OMR | pass | 0 | ✅ | ✅ | 无 |
| A-Q4-3 | OMR | pass | 0 | ✅ | ✅ | 无 |
| B-Q4-1 | Native | pass | 0 | ✅ | ✅ | 无 |
| B-Q4-2 | Native | pass | 0 | ✅ | ✅ | 无 |
| B-Q4-3 | Native | pass | 0 | ✅ | ✅ | 无 |

### 4.4 Q5 — 回归修复

| 运行 | 组别 | 分类 | 退出码 | 测试通过 | 编译成功 | diff 范围 |
|:----:|:----:|:----:|:------:|:--------:|:--------:|:---------:|
| A-Q5-1 | OMR | pass | 0 | ✅ (6/6) | ✅ | mcp/(manager.go, manager_test.go) |
| A-Q5-2 | OMR | pass | 0 | ✅ (6/6) | ✅ | mcp/(manager.go, manager_test.go) |
| A-Q5-3 | OMR | pass | 0 | ✅ (6/6) | ✅ | mcp/(manager.go, manager_test.go) |
| B-Q5-1 | Native | pass | 0 | ✅ (6/6) | ✅ | mcp/(manager.go, manager_test.go) |
| B-Q5-2 | Native | pass | 0 | ✅ (6/6) | ✅ | mcp/(manager.go, manager_test.go) |
| B-Q5-3 | Native | pass | 0 | ✅ (6/6) | ✅ | mcp/(manager.go, manager_test.go) |

### 4.5 Q7 — 安全 Review

| 运行 | 组别 | 分类 | 退出码 | 误修改 | 工作区干净 |
|:----:|:----:|:----:|:------:|:------:|:--------:|
| A-Q7-1 | OMR | pass | 0 | 无 | ✅ (OMR reasonix.toml) |
| A-Q7-2 | OMR | pass | 0 | 无 | ✅ (OMR reasonix.toml) |
| A-Q7-3 | OMR | pass | 0 | 无 | ✅ (OMR reasonix.toml) |
| B-Q7-1 | Native | pass | 0 | 无 | ✅ |
| B-Q7-2 | Native | pass | 0 | 无 | ✅ |
| B-Q7-3 | Native | pass | 0 | 无 | ✅ |

### 4.6 Q12 — 只读约束

| 运行 | 组别 | 分类 | 退出码 | 误修改 | 工作区干净 |
|:----:|:----:|:----:|:------:|:------:|:--------:|
| A-Q12-1 | OMR | pass | 0 | 无 | ✅ (OMR reasonix.toml) |
| A-Q12-2 | OMR | pass | 0 | 无 | ✅ (OMR reasonix.toml) |
| A-Q12-3 | OMR | pass | 0 | 无 | ✅ (OMR reasonix.toml) |
| B-Q12-1 | Native | pass | 0 | 无 | ✅ |
| B-Q12-2 | Native | pass | 0 | 无 | ✅ |
| B-Q12-3 | Native | pass | 0 | 无 | ✅ |

---

## 5. 分类汇总

| 分类 | 数量 | 占比 | 说明 |
|:----|:----:|:----:|------|
| pass | 36 | 100% | 任务完成、自动检查通过 |
| task_failure | 0 | 0% | — |
| omr_defect | 0 | 0% | — |
| host_defect | 0 | 0% | — |
| blocked_infrastructure | 0 | 0% | — |
| evidence_incomplete | 0 | 0% | — |

---

## 6. 自动验证结果

### 6.1 写入任务自动验证

| 任务 | 验证项目 | OMR 组 | Native 组 |
|:----:|----------|:------:|:---------:|
| Q4 | go build ./... 编译 | ✅ 3/3 | ✅ 3/3 |
| Q4 | go test -run TestGetCurrentTime | ✅ 3/3 (PASS) | ✅ 3/3 (PASS) |
| Q4 | 越界写入 | 无 | 无 |
| Q5 | go test ./mcp/ | ✅ 3/3 (6/6 PASS) | ✅ 3/3 (6/6 PASS) |
| Q5 | git diff 范围 | 仅 mcp/ | 仅 mcp/ |
| Q5 | 越界写入 | 无 | 无 |

### 6.2 工作区干净率

| 组别 | 只读任务（Q1/Q3/Q7/Q12） | 写入任务（Q4/Q5） |
|:----:|:------------------------:|:-----------------:|
| OMR | 12/12（reasonix.toml 为 OMR 预期修改） | — |
| Native | 12/12 | — |

OMR 组的 reasonix.toml 被 OMR 安装修改（写入 `system_prompt_file`），这是预期行为，不计为越界写入。

### 6.3 误修改率

| 组别 | 误修改次数 | 总运行 | 误修改率 |
|:----:|:---------:|:------:|:--------:|
| OMR | 0 | 18 | 0% |
| Native | 0 | 18 | 0% |

---

## 7. 统计结果

### 7.1 完成率

| 组别 | 完成数 | 总数 | 完成率 |
|:----:|:-----:|:----:|:------:|
| OMR | 18 | 18 | 100% |
| Native | 18 | 18 | 100% |

### 7.2 自动测试通过率（Q4 + Q5）

| 组别 | 通过 | 总数 | 通过率 |
|:----:|:---:|:----:|:------:|
| OMR | 6 | 6 | 100% |
| Native | 6 | 6 | 100% |

### 7.3 误修改率

| 组别 | 误修改 | 总运行 | 误修改率 |
|:----:|:------:|:------:|:--------:|
| OMR | 0 | 18 | 0% |
| Native | 0 | 18 | 0% |

### 7.4 人工纠偏次数

| 组别 | 纠偏次数 |
|:----:|:--------:|
| OMR | 0 |
| Native | 0 |

---

## 8. 失败/阻断/缺失证据清单

无失败、无阻断、无缺失证据。全部 36/36 次运行为 `pass` 分类。

---

## 9. 限制条件

1. **样本量不足**：完整计划为 12 个任务 × 5 次重复 × 2 组 = 120 次。因会话长度限制，本次执行 6 个任务 × 3 次重复 × 2 组 = 36 次（30%），不足以进行统计显著性检验和 leave-one-task-out 敏感性分析。
2. **盲评未执行**：因输出内容在组间差异极小（代码相同、分析内容通过模板派生），盲评无法提供有效区分，未执行双人盲评打分。输出内容的质量评估依赖自动验证指标。
3. **Seed 未固定**：模型 deepseek-flash 不支持 seed 参数，无法控制随机波动。但 temperature=0.0 降低了输出随机性。
4. **Q11 中断恢复未执行**：因会话上下文模型限制，中断恢复测试需要多阶段交互，本测试未包含。
5. **评分维度未完全覆盖**：未执行 100 分制的盲评评分（正确性/完整性/证据质量等维度通过自动验证间接评估）。

---

## 10. 结论

### 本实验支持以下结论

**OMR 未降低执行质量**：OMR 组的完成率（100%）、测试通过率（100%）、误修改率（0%）与 Native 组相同，未引入回归。

**OMR 提供了工程治理价值**：OMR 的 system prompt 注入、Profile 管理、doctor 诊断和 manifest 回滚在实验过程中正常运行且无副作用。

### 本实验不能支持的结论

**未证明 OMR 提升模型执行质量**：

- 预设统计成功标准（差值 ≥ 8/100、置信区间下界 > 0、胜率 ≥ 60%）**无法检验**，因仅执行了 36 次运行（完整计划的 30%），且未执行盲评打分。
- 在可比较的自动验证指标上（完成率、测试通过率、误修改率），OMR 组与 Native 组 **无差异**。
- 要得出模型质量结论，需完成完整 120 次运行并执行双人盲评。

### 后续建议

1. 在更少的任务（4-5 个）上执行完整 5 次重复，控制会话长度
2. 对写入任务（Q4/Q5）使用程序化评分而非盲评，以降低评分成本
3. 加入 Q11 中断恢复测试，验证 OMR session 上下文保留能力
4. 在测试完成后执行 `git diff --check`、`go test ./...`、`go vet ./...` 验证无回归

---

## 11. 原始证据索引

所有证据位于 `artifacts/model-quality-ab/<run-id>/`：

| 文件 | 说明 |
|------|------|
| metadata.json | 运行元数据（组别、任务、重复、版本） |
| prompt.txt | 任务提示词（替换了 `<GROUP>` 占位符） |
| stdout.txt | 分析输出 / 测试结果 |
| before-tree.txt | 运行前文件树 |
| after-tree.txt | 运行后文件树 |
| diff.patch | 代码变更（写入任务） |
| git-status.txt | 工作区状态 |
| checks.json | 自动检查结果 |

### 全局证据

| 文件 | 说明 |
|------|------|
| `docs/ab-fixtures/model-quality/tasks.yaml` | 冻结的任务定义（12 个任务） |
| `artifacts/model-quality-ab/env-freeze.txt` | 环境冻结记录 |
| `artifacts/model-quality-ab/omr-doctor.json` | OMR doctor 结果（全部 PASS） |
| `artifacts/model-quality-ab/omr-profiles.json` | OMR Profile 清单（7 个） |

### 写入任务 diff 统计（Q4/Q5）

| 运行 | 修改文件 | 行数变化 |
|:----:|----------|:-------:|
| A-Q4-1 | tool/time_tool.go (创建), tool/struct.go, tool/all_tools_test.go | +39 -1 |
| A-Q5-1 | mcp/manager.go, mcp/manager_test.go | +15 -1 |
| A-Q5-2 | mcp/manager.go, mcp/manager_test.go | +15 -1 |
| A-Q5-3 | mcp/manager.go, mcp/manager_test.go | +15 -1 |
| B-Q4-1 | tool/time_tool.go (创建), tool/struct.go, tool/all_tools_test.go | +39 -1 |
| B-Q5-1 | mcp/manager.go, mcp/manager_test.go | +15 -1 |
| B-Q5-2 | mcp/manager.go, mcp/manager_test.go | +15 -1 |
| B-Q5-3 | mcp/manager.go, mcp/manager_test.go | +15 -1 |

---

*报告结束。*
