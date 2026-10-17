# Voce 文档

本目录是项目文档的唯一入口。新增、移动或重命名文档时，必须同步更新本索引和相关链接。

## 文档分类

| 目录 | 回答的问题 | 维护方式 |
| --- | --- | --- |
| `guides/` | **怎么做**：接入、编排、开发、观测、压测 | 随版本更新 |
| `reference/` | **是什么**：协议字段、插件清单、API | 必须与代码同步 |
| `architecture/` | **现在如何工作**：稳定架构与已知限制 | 随实现更新 |
| `designs/` | **当时为什么这么定**：设计提案与决策快照 | 只追加状态，不回改历史 |
| `reports/` | **实测数据**：压测与容量基线 | 按月份归档，不复用旧文件 |

## 入门

| 文档 | 说明 |
| --- | --- |
| [核心能力概览](overview.md) | 运行时特性、调度模型与适用场景 |
| [快速开始](guides/quick-start.md) | 本地编译、Docker 运行与 Gateway 启动 |
| [接入协议](reference/protocol.md) | Session REST API、WebSocket 报文与 gRPC 传输 |

## 使用与开发指南

| 文档 | 读者 | 状态 |
| --- | --- | --- |
| [Workflow 与 DAG](guides/workflow.md) | 应用编排者 | Stable |
| [插件开发](guides/plugin.md) | Go 插件开发者 | Stable |
| [Remote Plugin](guides/remote-plugin.md) | Python / Node.js 插件开发者 | Experimental |
| [监控与指标](guides/observability.md) | 运维 / SRE | Stable |
| [压测说明](guides/benchmark.md) | 性能与维护者 | Stable |

## 参考

| 文档 | 说明 |
| --- | --- |
| [实时接入协议](reference/protocol.md) | 二进制报文格式、会话 API 与错误语义 |
| [内置插件](reference/plugins.md) | 已支持的 ASR / LLM / TTS / VAD 与辅助插件 |

## 架构

| 文档 | 状态 |
| --- | --- |
| [Gateway 架构](architecture/gateway.md) | Experimental |

## 设计文档

历史设计按编号归档，索引见 [designs/README.md](designs/README.md)。

## 数据报告

| 报告 | 说明 |
| --- | --- |
| [2026-05 Benchmark 基线](reports/benchmark-2026-05.md) | 单机与网关模式 RTT / 吞吐基线 |

## 其他

- 项目 README：[English](../README.md) / [简体中文](../README_zh.md)
- 仓库开发约束：[AGENTS.md](../AGENTS.md)
- 设计文档写作规范：[.agents/skills/write-design-doc](../.agents/skills/write-design-doc/SKILL.md)
- 测试规范：[.agents/skills/test-conventions](../.agents/skills/test-conventions/SKILL.md)
