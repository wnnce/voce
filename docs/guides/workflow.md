# Workflow 与 DAG 编排

Voce 通过 JSON 配置定义处理链路，运行时将其解析为 DAG（有向无环图）并驱动节点执行。本文档说明 Workflow、Graph、Node、Edge 的核心概念及其运行机制。

## 1. 基本概念

### Workflow

Workflow 是运行时实例。它持有一张已验证的 Graph，负责：

- 初始化所有节点（构建插件实例）
- 建立节点间的数据连接
- 管理生命周期（启动、暂停、恢复、停止）
- 收集所有节点的输出数据并通过统一出口下发

每个客户端连接对应一个独立的 Workflow 实例，实例之间完全隔离。

### Graph

Graph 是 Workflow 的静态拓扑描述。`BuildGraph` 在 Workflow 创建前完成以下校验：

1. 验证 `head` 节点存在
2. 检查节点 ID 唯一性、插件是否已注册
3. 检测自环边
4. 校验边两端节点的输入/输出契约是否匹配
5. 对节点做拓扑排序，确定启动顺序

校验通过后生成 `Graph`，包含排好序的 `OrderedNodes` 列表。

### Node

Node 是插件在 DAG 中的运行时载体。每个 Node 持有一个 Plugin 实例，负责接收上游数据、调用插件处理逻辑、将结果转发给下游。

Node 有两种运行模式（由 Workflow 的调度策略决定）：

- **loopNode**：独占一个 goroutine，自带事件循环和各类型独立 channel
- **schedulerNode**：不启动独立 goroutine，将事件提交给共享的 Scheduler 调度执行

### Edge

Edge 定义节点间的有向连接，指定数据流向和传输类型。

## 2. 配置结构

一个完整的 Workflow 配置示例：

```json
{
  "id": "a268a51b-...",
  "name": "realtime_voice",
  "version": "1.0.0",
  "head": "node_asr",
  "scheduler_mode": "thread-per-node",
  "scheduler_workers": 0,
  "nodes": [
    {
      "id": "node_asr",
      "name": "asr",
      "plugin": "qwen_asr",
      "config": { "model": "qwen3-asr-flash-realtime" }
    },
    {
      "id": "node_llm",
      "name": "llm",
      "plugin": "openai_llm",
      "config": { "model": "qwen-flash" }
    },
    {
      "id": "node_tts",
      "name": "tts",
      "plugin": "minimax_tts",
      "config": { "voice_id": "male-qn-qingse" }
    },
    {
      "id": "node_sink",
      "name": "sink",
      "plugin": "sink",
      "config": {}
    }
  ],
  "edges": [
    { "source": "node_asr", "source_port": 0, "target": "node_llm", "type": 2 },
    { "source": "node_llm", "source_port": 0, "target": "node_tts", "type": 2 },
    { "source": "node_tts", "source_port": 0, "target": "node_sink", "type": 3 }
  ]
}
```

### 字段说明

| 字段 | 说明 |
|------|------|
| `id` | Workflow 唯一标识 |
| `name` | 名称，用于路由匹配 |
| `version` | 版本号 |
| `head` | 入口节点的 ID。外部数据通过 `SendToHead` 进入 DAG |
| `scheduler_mode` | 调度策略，见下文 |
| `scheduler_workers` | Worker 数量（仅 `worker-pool` 模式有效） |
| `nodes` | 节点列表 |
| `edges` | 边列表 |

### Node 字段

| 字段 | 说明 |
|------|------|
| `id` | 节点唯一 ID，边引用时使用 |
| `name` | 节点名称，用于日志和按名查找 |
| `plugin` | 插件类型名称，必须是已注册的插件 |
| `config` | 插件配置，JSON 格式，由插件的 `PluginConfig.Decode` 解析 |
| `metadata` | 前端使用的元数据（如节点在画布上的位置），运行时不消费 |

### Edge 字段

| 字段 | 说明 |
|------|------|
| `source` | 源节点 ID |
| `source_port` | 源端口号。`0` 为广播端口（默认），`1-11` 为定向端口 |
| `target` | 目标节点 ID |
| `type` | 数据类型：`1` = Signal，`2` = Payload，`3` = Audio，`4` = Video |

> 端口机制：同一个插件可以通过不同端口将数据发送给不同的下游节点。`SendPayload` 广播到端口 0 的所有下游，`SendPayloadToPort(port, data)` 只发送到指定端口的下游。

## 3. 数据流转

DAG 中流转的数据分为四种类型，每种有不同的传输语义：

| 类型 | 说明 | 缓冲策略 |
|------|------|----------|
| **Signal** | 控制信令（如打断信号）| 优先队列，阻塞写入 |
| **Payload** | 业务数据（如 ASR 识别文本、LLM 回复）| 阻塞写入 |
| **Audio** | 音频帧 | 非阻塞，缓冲区满时丢弃 |
| **Video** | 视频帧 | 非阻塞，缓冲区满时丢弃 |

Signal 和 Payload 保证不丢失。Audio 和 Video 在背压时主动丢帧，防止实时链路因阻塞导致延迟堆积。

### 双优先级调度

每个节点的事件循环（或 Scheduler Worker）优先消费 Signal，再处理其他类型。这确保打断信号等控制指令在数据拥塞时仍能及时送达。

## 4. 契约校验

节点之间不能随意连接。每个插件在注册时声明了 `Inputs` 和 `Outputs`（通过 `PropertyBuilder`），BuildGraph 会在连边时校验两端契约是否兼容。

校验规则：

- 按 Edge 的 `type` 字段筛选对应前缀（`signal` / `data` / `audio`）的 Property
- 上游的某个输出 Property 必须与下游的某个输入 Property 匹配（名称 + 字段）
- 下游声明为 `Required` 的字段，上游必须提供且类型一致
- 下游的 Property Name 为空时视为通配，匹配任意同前缀的上游输出

如果校验失败，Workflow 无法创建，错误信息会指明哪条边、哪个字段不满足。

## 5. 拓扑排序与启动顺序

`BuildGraph` 使用 Kahn 算法对节点做拓扑排序。排序结果保证：上游节点在下游节点之前启动。

如果图中存在反馈环（feedback cycle），排序会输出警告日志并将环中节点追加到排序末尾。Voce 不禁止反馈环，但开发者需要自行确保环内节点不会产生无限循环。

## 6. 调度策略

Workflow 支持两种节点调度模式：

### thread-per-node（默认）

每个节点启动一个独立的 goroutine 运行事件循环。节点拥有独立的 channel 集合：

- `signalChan` (容量 12)
- `payloadChan` (容量 24)
- `audioChan` (容量 64)
- `videoChan` (容量 24)
- `ctrlChan` (容量 8)

适用于节点数量不多、需要最大化节点间隔离性的场景。

### worker-pool

所有节点共享一组 Worker goroutine。通过 FNV Hash 将每个节点绑定到固定的 Worker，保证同一节点的事件在同一 Worker 上串行执行。

Worker 数量由 `scheduler_workers` 配置：
- 设为 `0` 或超出节点数量时，使用自适应公式：`ceil((nodeCount + 3) / 4)`
- 每个 Worker 维护高优先级和普通优先级两个队列（默认容量 128）

适用于节点数量多但大部分节点处理逻辑轻量的场景，减少 goroutine 数量和上下文切换。

> **注意**：同一 Worker 上的节点共享执行时间。如果某个同步插件阻塞了 80ms，该 Worker 上的其他节点也会被阻塞 80ms。因此在 worker-pool 模式下，有阻塞操作的插件必须使用 `MultiTrackPlugin` 包装为异步模式。

## 7. 输出与背压

所有节点通过 `flow.Publish` / `flow.PublishFull` 向客户端发送数据时，数据会写入 Workflow 的统一输出 channel（容量 1024）。

当输出 channel 满时（通常意味着下游网络写入速度跟不上），Workflow 会直接丢弃新的 Packet 并记录警告日志，防止节点因阻塞而停滞。

## 8. 生命周期

```
Pending → Starting → Running ⇄ Paused → Stopped
```

| 状态 | 说明 |
|------|------|
| `Pending` | 已创建，尚未启动 |
| `Starting` | 正在按拓扑顺序启动各节点（调用 `OnStart`）|
| `Running` | 所有节点启动完成，开始处理数据 |
| `Paused` | 暂停状态，源头停止输入，已在队列中的数据继续处理 |
| `Stopped` | 已销毁，所有资源已释放 |

启动阶段如果某个节点的 `OnStart` 返回错误，已启动的节点会按逆序回滚 `Stop`，Workflow 直接进入 `Stopped` 状态。

全部启动成功后，调用每个节点的 `OnReady`，通知插件系统已就绪。
