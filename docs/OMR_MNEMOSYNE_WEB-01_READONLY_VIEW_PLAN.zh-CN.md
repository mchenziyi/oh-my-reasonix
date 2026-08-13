# OMR Mnemosyne WEB-01：只读记忆视图计划

状态：✅ 已实现（静态导出 MVP）

## 目标

提供一个不依赖 Reasonix、无服务端、可重复生成的本地只读 HTML，用于查看当前 Scope 中的 Memory Revision 及其关系。它是规范 Fact 的派生视图，不是新的事实源，也不修改 Memory Store、CURRENT 或任何 Generation。

## MVP 边界

- 输入仅来自已通过 FactStore 完整校验的 `MemoryRevision`。
- 输出包含 scope、memory type、id、revision、usage policy、标题、摘要、content hash 和关系边。
- 所有文本必须 HTML 转义；不输出命令、Prompt、绝对路径、凭据或模型思考。
- 输出字节按稳定排序生成；相同事实集合和显式时间输入必须得到相同内容。
- 不引入 Web 框架、数据库、JavaScript 或网络监听。
- 导出失败时不写入 Store；CLI 输出文件采用安全的不可变创建策略，已有不同内容的文件拒绝覆盖。

## 后续

真正的交互式 Web 页面、筛选、冻结记忆管理和关系图谱交互属于 WEB-02/WEB-03，不在本次 MVP 内。
