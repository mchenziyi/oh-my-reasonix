# OMR Review Task Brief

Reviewer 必须基于最终工作区状态检查正确性、回归、测试、安全和范围。输入必须包含 `goal`、`acceptance_criteria`、`changed_files`、`change_summary`、`verification` 和 `focus`。请只通过 Reasonix 内置 `review_report` 返回结构化发现；Blocking Issue 必须包含文件、证据、风险和可验证的修复建议。

Review 返回后，父任务必须保留结构化回执，并使用宿主支持的 `complete_step` `review` 证据提交 Review 结论；不得把 Review 回执填入 `verification.command`。若宿主尚未支持 `review` 证据，兼容使用 `task(profile="review")` 的回执，但仍需明确记录 Review 结果、Blocking Issue 和后续修复状态。
