# 设计文档

每份设计文档使用独立目录，以两位序号作为前缀。正文放在目录内的 `design.md`，相关图示和附件放在同一目录下。

设计文档记录的是**当时的决策与依据**，默认不回改历史内容。实现发生变化时，在文档顶部标注状态（`已实现` / `部分实现` / `已废弃`），现状说明更新到 [`architecture/`](../architecture/) 或 [`guides/`](../guides/)。

| 编号 | 主题 | 状态 |
| --- | --- | --- |
| 01 | [Remote Plugin 初版设计](01-remote-plugin/design.md) | 已实现（Experimental） |
| 02 | [连接层抽象设计](02-connection-layer/design.md) | 部分实现（Session/WS/gRPC） |
| 03 | [Agora RTC 接入设计](03-agora-rtc/design.md) | 已实现（仅 standalone） |
| 04 | [Web 对话页设计与实现计划](04-web-conversation/design.md) | 已实现并集成至 Studio (Voice Playground) |
| 05 | [Web UI 重构设计 (Tailwind v4 + shadcn/ui + SCSS Tokens)](05-web-ui-refactor/design.md) | 已全量实施完成 (Studio 架构就绪) |

写作规范见 [`.agents/skills/write-design-doc`](../../.agents/skills/write-design-doc/SKILL.md)。返回[文档首页](../README.md)。
