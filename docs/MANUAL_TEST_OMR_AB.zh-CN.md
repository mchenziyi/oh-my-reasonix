# OMR 人工体验与 A/B 对照测试

本文比较两种运行方式：A 组安装 oh-my-reasonix（OMR）后使用 Reasonix；B 组不安装 OMR，只使用原生 Reasonix。目标是观察项目级约束、Profile、Review 证据、升级诊断和运行记录是否更稳定，而不是证明 OMR 在所有任务上都更强。

## 一、测试原则

1. 两组使用同一台机器、同一个 Reasonix 版本和同一个模型配置。
2. 两组从同一份项目初始快照复制，A 组产生的代码、Prompt 或配置不得进入 B 组。
3. 每个任务使用完全相同的提示词；成本允许时每组运行三次并取中位数。
4. 记录原始输出、耗时、是否修改文件、是否调用 Profile/Review，以及失败后的恢复情况。
5. 模型随机性、网络超时和项目自身偶发错误不要直接归因于 OMR。
6. 将项目内容发送给模型服务前，确认已获得授权。

## 二、准备环境

先从 OMR 仓库构建临时测试二进制，再记录版本和模型：

```bash
cd /path/to/oh-my-reasonix
go build -o /tmp/omr-manual ./cmd/omr
/tmp/omr-manual version
reasonix --version
```

```text
日期：
Reasonix 版本：
OMR 版本/提交：
模型：
项目语言与规模：
```

准备两个隔离目录（SOURCE_PROJECT 必须是没有 OMR 安装产物的干净快照）：

```bash
SOURCE_PROJECT=/path/to/source-project
cp -R "$SOURCE_PROJECT" /tmp/omr-ab-omr
cp -R "$SOURCE_PROJECT" /tmp/omr-ab-native
```

/tmp/omr-ab-omr 是 A 组，/tmp/omr-ab-native 是 B 组。B 组不要执行任何 omr 安装或升级命令。

## 三、A 组安装和验证

```bash
cd /tmp/omr-ab-omr
/tmp/omr-manual init --project-dir . --dry-run
/tmp/omr-manual init --project-dir .
/tmp/omr-manual doctor --project-dir .
/tmp/omr-manual profile list --project-dir . --json
```

dry-run 确认只涉及项目目录后再安装。预期：Doctor 无 blocking error；配置指向生成 Prompt；Manifest、源文件 Hash 和备份存在；未修改全局 PATH、API Key 或 Reasonix 二进制。

保存证据：

```bash
/tmp/omr-manual doctor --project-dir . --json > /tmp/omr-ab-a-doctor.json
/tmp/omr-manual profile list --project-dir . --json > /tmp/omr-ab-a-profiles.json
```

## 四、统一体验任务

以下提示词在 A、B 两组逐字执行。每次任务开始前回到对应目录，不要把另一组的结果贴给当前会话。

### T1：项目探索与测试入口

```text
请只读分析当前项目：列出主要模块、程序入口、测试入口、关键依赖和潜在高风险区域。不要修改文件。结论必须引用实际文件路径，并区分已确认事实与推测。
```

记录：路径证据、事实/推测区分、是否误改文件。

### T2：需求拆解与实施计划

```text
请把“为当前项目增加一个可配置的健康检查命令”拆成最小实施任务。先列假设、影响文件、风险、验收条件和测试命令，再开始修改。暂时不要修改代码。
```

记录：是否先澄清边界，是否有可执行验收和回滚条件。

### T3：故障定位

```text
请定位当前项目中一个最值得优先修复的测试或构建问题。先收集证据并给出根因，再提出最小修复方案；不要为了证明结论而大范围重构。
```

记录：症状/根因区分、复现命令、修改范围。

### T4：Review 与安全边界

```text
请对当前项目做一次只读代码 Review，重点检查输入校验、路径越界、敏感信息泄露、错误处理和测试缺口。每个问题给出严重级别、文件路径、证据和修复建议；没有证据不要猜测。
```

记录：问题是否有文件证据，是否把普通风格问题误报为安全漏洞。

### T5：失败后恢复

先让一个任务中途停止，再发送：

```text
请从上次中断的位置继续。先说明已完成和未完成的步骤，不要重复已完成的工作；继续前先检查工作区状态。
```

记录：是否重复执行、是否识别未完成步骤、是否保留上下文。若原生 Reasonix 不支持恢复，记为“宿主限制”，不要归因于 OMR。

### T6：结构化事件（可选）

A 组执行：

```bash
/tmp/omr-manual run --project-dir . --events-jsonl /tmp/omr-ab-a-events.jsonl --json "请只读列出当前项目的测试入口并给出运行命令"
```

B 组用 Reasonix 原生命令完成同一任务并保存终端输出。如果宿主不支持结构化事件流，记录“不支持”，不要伪造对照结果。

## 五、统一记录表

评分 0～5：0=未完成，3=基本可用，5=稳定且证据充分。

| 任务 | 组别 | 完成度 | 证据质量 | 是否误改文件 | 人工纠偏次数 | 耗时 | Token/成本 | 备注 |
|---|---|---:|---:|---|---:|---:|---:|---|
| T1 | OMR / Native | | | | | | | |
| T2 | OMR / Native | | | | | | | |
| T3 | OMR / Native | | | | | | | |
| T4 | OMR / Native | | | | | | | |
| T5 | OMR / Native | | | | | | | |
| T6 | OMR / Native | | | | | | | |

另记：doctor 结果、可见 Profile、Prompt/配置漂移、运行失败、恢复是否成功、人工介入次数。

## 六、对比结论

分别计算任务完成率、证据充分率、误修改率、恢复成功率、平均人工介入次数，以及在条件一致时的成本和耗时。使用以下模板：

```text
在 Reasonix <版本>、模型 <模型>、项目 <项目> 上，OMR 相对 Native：
- 任务完成率：<A> vs <B>
- 证据充分率：<A> vs <B>
- 误修改率：<A> vs <B>
- 恢复成功率：<A> vs <B>
- 平均人工介入：<A> vs <B>
- 成本/耗时：<A> vs <B>
结论：OMR 在 <项目级约束/Profile/Review/诊断/记录> 上有明显帮助；
在 <宿主原生能力或模型波动> 上没有足够证据证明优势。
```

测试结束前保留 Doctor JSON、事件文件和两组原始记录。只在临时目录中清理；真实项目先执行 git status --short，不要删除 OMR 备份.
