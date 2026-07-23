# OMR-T05～T09：后续优化总开发计划

## 1. 总目标

在 OMR-T04 之后，一次性完成 OMR 仓库内剩余的 5 组独立优化：

- T05：质量与成本可观测性；
- T06：安装与升级体验；
- T07：工具生态 Profile；
- T08：开发体验增强；
- T09：规则和配置兼容性。

这些任务只实现 OMR 能独立负责的配置、Prompt、资产、报告、CLI 和离线验证能力。Reasonix 原生已经提供或尚未公开稳定接口的能力，不在 OMR 中复制。

## 2. 当前基线

开始前必须确认：

- T01～T04 已完成；
- OMR Profile、Category、Claude 兼容层、质量回放和安装链路测试均通过；
- 当前已推送的 main 是唯一基线；
- 保留被忽略的 omr 二进制和 .reasonix/ 本地产物，不纳入提交。

每个阶段都必须先阅读现有实现和测试，先写失败测试或 Fixture，再做最小修改。

## 3. 执行顺序

必须按以下顺序执行：

1. T05：质量与成本可观测性；
2. T06：安装与升级体验；
3. T07：工具生态 Profile；
4. T08：开发体验增强；
5. T09：规则和配置兼容性。

如果某个阶段发现依赖 Reasonix 私有接口，立即将该子项标记为 BLOCKED，继续执行不依赖该接口的部分，不得伪造结果。

---

## 4. T05：质量与成本可观测性

### 目标

统一 Runtime、Replay、Native/OMR 对照报告，让失败原因、成本和证据可以稳定比较。

### 实现任务

1. 统一报告字段：
   - schema_version；
   - fixture_id；
   - run_id（明确是 OMR 合成 ID，不冒充 Reasonix Session ID）；
   - execution_mode；
   - qualified_completion；
   - failure_category；
   - retry_count；
   - stall_reason；
   - review_block_count；
   - token/cost；
   - verification_evidence；
   - native_omr_pair 信息。
2. 为缺失、未知和不可用指标定义明确表示，不用零值伪装成功。
3. 增加稳定 JSON 快照和 Schema 迁移测试。
4. 扩展成本门禁、重试、停滞、Review 阻断和证据缺失 Fixture。
5. 确保正常回放、失败回放和基础设施失败的分类不会混淆。
6. 报告中不得写入 Prompt 原文、API Key、绝对用户路径或 Reasonix 私有 Session 内容。

### 验收

- 旧报告可读取或有明确迁移错误；
- JSON 字段顺序/内容稳定；
- 33+ 现有质量 Fixture 和新增 Fixture 全部通过；
- Native/OMR 没有配对证据时明确标记 unavailable；
- 成本超限、重试超限、停滞和 Review 阻断均有回归测试。

---

## 5. T06：安装与升级体验

### 目标

让用户能安全预览、升级、回滚和诊断 OMR，不自动修改全局环境。

### 实现任务

1. 增加最低 Reasonix 版本和兼容矩阵检查：
   - version；
   - 支持的 CLI 能力；
   - 不满足时给出 actionable warning/error。
2. 增加 omr version --json：
   - OMR 版本；
   - Prompt/资产版本；
   - Manifest schema；
   - Reasonix 检测版本；
   - 兼容状态。
3. 增强 upgrade --dry-run：
   - Prompt 段级变化；
   - Profile/资产变化；
   - 配置影响；
   - 备份位置；
   - 用户修改冲突；
   - 预计写入文件。
4. 增加备份保留、回滚和失败恢复测试。
5. 增加仅提示的更新检查，不自动下载、安装或修改 PATH。
6. 同步 README、INSTALL、Release 和卸载文档。
7. 保证 dry-run 零写入，冲突时零写入，升级失败可恢复。

### 验收

- 旧版本 Reasonix、缺失 binary、版本格式异常均有测试；
- version JSON 可解析且无敏感信息；
- dry-run 输出完整且零写入；
- 用户修改的 Prompt/Profile/配置不会被静默覆盖；
- 升级失败后文件、权限和备份状态恢复；
- 不修改全局 PATH、API Key 或用户级配置。

---

## 6. T07：工具生态 Profile

### 目标

评估并在 Reasonix 能力明确时提供 LSP、AST、Git、Browser/Playwright 和 MCP 相关 Profile；宿主不支持时只记录调查结果。

### 实现任务

1. 对每类工具做能力探测和兼容性矩阵：
   - LSP；
   - AST/AST-Grep；
   - Git；
   - Browser/Playwright；
   - Skill 内嵌 MCP。
2. 只为已确认可执行的宿主能力增加 Profile 资产。
3. Profile 必须声明：
   - 工具依赖；
   - 只读/写入边界；
   - 输入和输出；
   - 缺少工具时的降级行为；
   - 风险和人工复核条件。
4. 工具不可用时，Doctor 给出 UNSUPPORTED/WARN，不生成不可执行资产。
5. 增加资产嵌入、安装、Hash、Profile list 和 Doctor 测试。
6. 不执行真实浏览器、外部网络或用户项目写入作为单元测试前提。

### 验收

- 宿主支持的 Profile 可安装、列出、校验和回滚；
- 宿主不支持的能力明确记录为调查/跳过；
- 不存在“安装成功但无法执行”的假 Profile；
- 不引入 Reasonix 私有 API 猜测。

---

## 7. T08：开发体验增强

### 目标

在 Prompt/配置层增强 OMR 的开发体验，不复制 Reasonix 后台状态机。

### 实现任务

1. 评估显式增强模式：
   - 默认模式与增强模式的差异；
   - 配置开关；
   - Prompt 片段；
   - Doctor 检查。
2. 评估 Ralph Loop：
   - 只实现可验证的 Prompt/质量循环约束；
   - 明确最大迭代次数和停止条件；
   - 不复制宿主 Task/Session 状态机。
3. 增加 Comment Checker 规则资产或质量 Fixture：
   - 禁止无证据 TODO；
   - 检查注释与实现不一致；
   - 输出可审计证据。
4. 评估用户级/项目级配置优先级，不自动写入用户目录。
5. 评估交互通知配置，仅实现静态配置和提示，不接管桌面通知。
6. 增加配置 Schema、Prompt 稳定性和离线回放测试。

### 验收

- 所有增强能力可关闭；
- 默认配置行为不变；
- Ralph/Comment Checker 不依赖私有 Session/Hook；
- 停止条件、最大循环和失败处理有测试；
- 不因注释检查而静默降低功能测试要求。

---

## 8. T09：规则和配置兼容性

### 目标

完善配置格式、规则来源和跨平台兼容性，让迁移和编辑体验稳定可预期。

### 实现任务

1. 完善配置 Schema 自动生成和版本标识。
2. 完善 JSONC 文档、注释、错误位置和编辑器提示信息。
3. 评估 .agents/skills 兼容：
   - 只在字段和语义明确时导入；
   - 不支持的字段进入兼容性报告；
   - 保持 dry-run、冲突、回滚和敏感信息保护。
4. 固定用户级/项目级配置优先级，并测试冲突。
5. 增加跨平台路径、权限、大小写和换行符测试。
6. 增加配置迁移前后快照，保证无关字段保留。
7. 规则来源、Profile ID、Category 和 Prompt 路径必须做规范化与安全校验。

### 验收

- TOML/JSONC 解析和迁移测试通过；
- 错误包含文件、字段和位置；
- 跨平台路径不越界、不读取绝对路径；
- 用户级配置不会在未经授权时被写入；
- 迁移失败可回滚；
- 未知字段不会静默改变语义。

---

## 9. 每阶段通用门禁

每个 T05～T09 阶段完成前必须执行：

~~~bash
gofmt -w <changed-go-files>
git diff --check
go test ./...
go vet ./...
go build ./...
go run ./cmd/omr doctor --json --project-dir <temp-project>
go run ./cmd/omr profile list --json --project-dir <temp-project>
go run ./cmd/omr benchmark quality --replay --min-qualified-rate 1
~~~

若阶段涉及安装/升级，还必须执行：

~~~bash
go run ./cmd/omr init --project-dir <temp-project> --dry-run
go run ./cmd/omr upgrade --project-dir <temp-project> --dry-run
go run ./cmd/omr uninstall --project-dir <temp-project> --dry-run
~~~

所有命令结果必须记录退出码和关键输出。禁止删除测试、降低断言或用合成结果冒充真实宿主行为。

## 10. 提交策略

可以一次性完成开发，但必须按阶段提交，建议提交标题：

- T05: quality/cost observability；
- T06: install/upgrade UX；
- T07: tool ecosystem profiles；
- T08: developer experience；
- T09: rules/config compatibility。

每个提交都应只包含对应阶段的代码、资产、测试和文档。完成全部阶段后再做一次总体验收。

## 11. 明确禁止

- 不复制 Reasonix 的 Todo、Session、Hook、Task、权限、后台任务或恢复状态机；
- 不读取 ~/.reasonix/projects、桌面应用数据库、私有事件文件或内部锁；
- 不修改全局 PATH、API Key、用户级配置或 Reasonix 二进制；
- 不把 OMR 合成 run_id 冒充 Reasonix Session ID；
- 不安装无法执行的工具 Profile；
- 不进行未经用户授权的真实项目内容上传或外部网络调用；
- 保留被忽略的 omr 和 .reasonix/ 本地产物，不纳入提交。

## 12. 总完成标准

- T05～T09 每个阶段均有实现、测试、文档和提交证据；
- 所有通用门禁通过；
- 现有质量回放不回归；
- 安装、升级、卸载、配置迁移和 Profile 路由保持向后兼容；
- 所有不支持能力有明确 unsupported/blocked 说明；
- 没有静默覆盖、敏感信息泄漏、路径越界或伪造宿主状态。

## 13. 交给 Reasonix Agent 的执行指令

请按 T05 → T06 → T07 → T08 → T09 顺序执行。允许一次性完成全部任务，但必须每个阶段先写失败测试/Fixture，再做最小实现并独立提交。每阶段结束输出修改文件、测试命令及结果、兼容性变化、风险和未实现项。遇到真实 Reasonix 客户端或宿主私有接口依赖，标记 BLOCKED 并继续不依赖该接口的工作，不得猜测或伪造结果。

