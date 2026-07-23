# OMR-T10：可选 Web/Docs MCP 兼容层开发计划

## 目标

为 OMR 增加可选的 Web/Docs MCP 配置与兼容性报告，让研究类 Profile 可以使用用户明确配置的文档搜索服务，同时保持默认离线、安全和可回滚。

## 范围

只实现：

- MCP 配置发现和只读导入；
- 服务器能力/风险报告；
- Profile 对 MCP 的可选引用；
- dry-run、冲突、回滚、Schema 和脱敏测试；
- 文档与示例。

不实现：

- OMR 自带网络爬虫；
- 默认绑定某个商业供应商；
- 自动下载 MCP Server；
- 自动读取或打印 API Key；
- 在没有宿主能力时生成不可执行 Profile；
- 复制 Reasonix MCP 运行时或权限状态机。

## 分步任务

### T10-01：能力与配置调查

1. 阅读当前 Reasonix MCP 配置格式和 CLI 帮助。
2. 确认 stdio、HTTP/SSE 等实际支持的传输方式。
3. 列出可验证能力：文档搜索、网页抓取、代码搜索、版本过滤。
4. 对未知能力标记 unknown，不猜测兼容性。

### T10-02：可选配置模型

增加项目级 OMR MCP 配置，要求：

- 默认 disabled；
- 服务器名称、transport、command/URL、能力标签；
- 环境变量只允许引用名称，不保存值；
- 支持 enabled/disabled；
- 配置 Schema、TOML/JSONC 解析和错误位置；
- 项目配置优先级明确。

### T10-03：兼容性与风险报告

增加 config validate、doctor 和 dry-run 报告：

- server；
- transport；
- capability；
- compatible/unknown/unsupported；
- command/URL 风险；
- 所需环境变量名称；
- 是否需要用户确认。

报告不得包含 token、密钥、环境变量值、完整命令敏感参数或不必要绝对路径。

### T10-04：Profile 可选引用

只给 omr-research 和明确需要文档检索的 Profile 增加可选引用：

- 未启用 MCP 时行为不变；
- MCP 不可用时降级为无 MCP 研究；
- Prompt 中明确区分事实、来源和未知；
- 不把 MCP 可用性写成 Reasonix 原生保证。

### T10-05：自动化验证

增加 Fixture/测试：

- 默认不启用；
- 合法 stdio 配置；
- unknown/unsupported transport；
- 缺少 command/URL；
- 敏感字段脱敏；
- dry-run 零写入；
- 冲突和回滚；
- MCP 不可用时 Profile 仍可安装；
- 人类与 JSON 报告一致。

不执行真实外部网络请求作为测试前提；使用 Mock 或静态能力描述。

### T10-06：文档

更新 README、安装文档、配置 Schema、Profile 说明和 OMR Todo：

- 明确这是可选能力；
- 给出自托管/本地 MCP 示例；
- 说明网络、成本、隐私和凭证责任；
- 不把第三方服务写成 OMR 依赖。

## 完成标准

- 默认安装不启用任何 Web/Docs MCP；
- 配置、报告、dry-run、回滚和脱敏测试通过；
- Reasonix 不支持的传输不会生成可执行假配置；
- go test ./...、go vet ./...、go diff --check 和 CLI Smoke 通过；
- 真实网络验证另行授权，不阻塞本地交付。

## 交给 Reasonix Agent 的指令

先完成 T10-01，确认 Reasonix 当前 MCP 能力后再实现 T10-02～T10-06。若宿主格式或能力不稳定，停止对应实现并报告 BLOCKED，不要猜测接口或内置第三方服务。
