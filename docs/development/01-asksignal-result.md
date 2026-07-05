# 01 - AskSignal 结果收集（OnSignal 返回 Result）

> 状态：设计已确认，待实现
> 关联代码：`internal/engine/{interface,node,node_loop,node_scheduler,scheduler,multi,plugin}.go`、`internal/schema/{signal,result}.go`、`pkg/syncx/collector.go`

## 1. 背景与目标

当前引擎的信号（Signal）是单向、异步、fire-and-forget 的：发送方 `SendSignal` 把信号投递给下游节点后不关心结果。`Plugin.OnSignal` 的签名虽然已经返回 `schema.Result`，但 `processEvent` 直接丢弃了返回值；`Flow.AskSignal` / `AskSignalToPort` 接口也已声明返回 `*syncx.Collector[schema.Result]`，但 `baseNode` 的实现只是 `return nil`。

需求：某些**单纯的执行节点**希望在收到一个 Signal 后直接产出结果，发送方不必让结果在图里再绕一圈回来，而是能直接拿到**直接下游节点**的 `Result`。

目标语义：

- 通过 `AskSignal` 发送的信号，由 engine 层负责收集其**直接下游节点** `OnSignal` 返回的 `Result`；发送方通过 `Collector` 的 channel 拿到全部结果。
- 通过 `SendSignal` 流转的信号，忽略 `OnSignal` 的返回值（保持现有行为）。
- 如果被 ask 的下游节点选择继续将信号向下流转（在其 `OnSignal` 里调用 `SendSignal`），那么下游的聚合由**下游节点自己负责**——上游只收集直接下游的结果，不做递归收集。

## 2. 语义定义

| 发送方式 | OnSignal 返回值处理 | 收集范围 |
|---|---|---|
| `SendSignal` / `SendSignalToPort` | 丢弃 | 无 |
| `AskSignal` / `AskSignalToPort` | 收集到 Collector | 仅直接下游节点 |

"直接下游"= 发送节点 signal 路由表中的下一跳节点（`table.signals` 或 `portTable[port].signals`，`addNextNode` 已去重）。

下游若在其 `OnSignal` 内 `SendSignal` 继续流转，转发出去的是**裸信号**，不携带上游的 collector，因此不会把结果继续上灌——下游想聚合就自己调用 `AskSignal`。这天然形成"谁 ask，谁收集直接下游"的递归结构。

## 3. 关键约束：调度模式与并发模型

引擎有两种调度模式（`WorkflowConfig.SchedulerMode`，可随配置切换）：

- **thread-per-node**（`loopNode`）：每个节点独立 goroutine + 独立 channel 事件循环。
- **worker-pool**（`schedulerNode` + `Scheduler`）：固定数量 worker，多个节点共享 worker，按 hash 分配。

### 3.1 死锁的根因

worker-pool 下，若发送方在自己的 `OnSignal` 里**阻塞**等待 collector 结果，就占住了它所在的 worker。一旦某个需要执行才能 `Done()` 的下游节点被 hash 到**同一个 worker**，该 worker 无法去执行下游任务 → 自死锁。thread-per-node 没有此问题（各节点 goroutine 独立）。

### 3.2 被否决的方案（决策记录）

| 方案 | 结论 | 否决原因 |
|---|---|---|
| 发送方阻塞等待、下游走队列 | ✗ | worker-pool 自死锁；嵌套 ask 死锁 |
| ask 下游丢到一个专用 worker | ✗ | 嵌套 ask 时专用 worker 卡在外层 OnSignal 等内层 → 死锁；所有 ask 全局串行 |
| 每个下游各开一个 goroutine 执行 OnSignal | ✗ | 下游脱离自身事件循环，与该节点普通事件并发，破坏 worker-pool 串行保证 |
| 内联同步调用下游 OnSignal（不走调度器） | ✗ | ask 目标若同时有异步 signal 入边则并发；且 Node 的事件循环变成空转，"流节点/服务节点"概念混淆 |
| 回调形式回传结果 | ✗ | 回调触发时下游必然已在别处执行，并发/脱离上下文问题不变，只是换了拿结果的写法 |

根本矛盾：`AskSignal` 是同步"请求-响应"语义，而 Node 抽象是为异步流设计的。把同步调用塞进异步流 Node，必然在"占用调度单元（死锁）"与"脱离事件循环（并发）"之间二选一。

### 3.3 最终决策：异步消费约定

下游**照常走各自模式的正常调度**执行 `OnSignal`，不内联、不专用 worker、不禁用。死锁的唯一触发条件是"发送方阻塞占住 worker"，因此约定：

> **发送方必须异步消费 collector**：`AskSignal` 后立即返回（不在 `OnSignal` 调用栈内阻塞 `range collector.Chan()`），在独立 goroutine 中消费结果。

这样发送方不占用 worker → 下游（即使 hash 到同一 worker）能正常轮到执行、`Put`、`Done` → 发送方 goroutine 收到结果。两种模式下**下游执行路径完全一致**，唯一差别是 thread-per-node 允许同步等待、worker-pool 不允许，靠约定统一为"两种模式都异步消费"。

这是**软约束**（channel 无法在代码层强制异步），必须在接口文档中写成硬性要求。

## 4. 详细设计

### 4.1 传输层包装类型（node.go）

```go
type askSignal struct {
    schema.Signal                          // 嵌入接口，天然满足 schema.Signal，可走现有 chan schema.Signal
    collector *syncx.Collector[schema.Result]
}
```

`processEvent` 解包后**只把裸 `Signal`** 交给 `OnSignal`，因此下游 `SendSignal` 转发的是裸信号，collector 不再传播。

### 4.2 AskSignal 实现（baseNode，node.go）

- `AskSignal` 取 `n.table.signals`，`AskSignalToPort` 取 `n.portTable[port].signals`。
- collector 容量 = 直接下游节点数（`addNextNode` 已去重，每个下游至多 `Put` 一次，buffer 不会溢出）。
- 逐个 `next.Input(&askSignal{Signal: value, collector: c})` 投递，返回 collector。
- **无下游 / ctx 已取消 / 节点未运行**：返回 `NewCollector[schema.Result](0)`（构造即关闭的空 collector），调用方 `range` 立即结束，不会 nil panic。

### 4.3 processEvent 改造（node.go）

在 `case schema.Signal` 之前新增 `case *askSignal`：

```go
case *askSignal:
    defer func() { _ = v.collector.Done() }() // 放在 recover 之后仍会执行，panic 也兜住
    result := n.plugin.OnSignal(currentCtx, n, v.Signal)
    if result != nil {
        _ = v.collector.Put(result)
    }
```

- 转发型插件（如 `BuiltinPlugin.OnSignal` 转发后 `return nil`）只 `Done` 不 `Put`，collector 仍能正常关闭，发送方 `range` 只收到非 nil 结果。这是预期行为。

### 4.4 移除 signal / payload 的 100ms deadline

现状：`processEvent`（node.go）与 `Scheduler.execute`（scheduler.go）对 Signal、Payload 套用 `singleHandlerDeadline`（100ms）context。

变更：**取消 signal 与 payload 的超时 context，统一使用 `n.ctx`**，仅保留"耗时超过阈值"的**警告日志**（不再取消 context）。

原因：AskSignal 的下游若要做实质计算产出 Result，可能超过 100ms，deadline 会取消 context 导致结果不可用。既然是等待结果的路径，就应允许其耗时。Audio/Video 路径的行为不变。

### 4.5 Done 兜底（防止发送方 goroutine 永久挂起）

`askSignal` 的 collector 若始终不 `Done`，发送方消费 goroutine 会永久阻塞。所有会**丢弃信号而不执行 OnSignal** 的路径都必须识别 `*askSignal` 并补一次 `Done()`：

- `loopNode.Input`（node_loop.go）：节点已停止 / ctx 取消的早退丢弃分支；`SendWithContext` 失败分支。
- `loopNode.drainChannels`（node_loop.go）：目前只清理 audio/video，需一并排空 `signalChan` 并对残留的 `askSignal` 补 `Done()`。
- `schedulerNode.Input`（node_scheduler.go）/ `Scheduler` 的 drain 路径（scheduler.go `worker.drain`）：识别 `askSignal` 补 `Done()`。

### 4.6 测试桩

- `mock_flow.go`：`AskSignal/AskSignalToPort` 提供可用实现（可返回可编程结果的 collector 供测试断言）。
- `plugin_tester.go`：`InjectSignal` 暴露 `OnSignal` 返回的 `Result` 以便测试。

## 5. 改动点清单

| 文件 | 改动 |
|---|---|
| `internal/engine/node.go` | 新增 `askSignal` 类型；`baseNode.AskSignal/AskSignalToPort` 真实现；`processEvent` 解包 + 收集 + defer Done；移除 signal/payload deadline |
| `internal/engine/node_loop.go` | `Input` 丢弃路径、`drainChannels` 补 `askSignal` 的 Done 兜底 |
| `internal/engine/node_scheduler.go` | `Input` 丢弃路径补 Done 兜底 |
| `internal/engine/scheduler.go` | `execute` 移除 signal/payload deadline；`worker.drain` 补 Done 兜底 |
| `internal/engine/mock_flow.go` | `AskSignal` 可用实现 |
| `internal/engine/plugin_tester.go` | `InjectSignal` 暴露 Result |
| `internal/engine/interface.go` | 接口注释补充"必须异步消费 collector"约束 |

**不改动**：`Flow` 接口签名、`Plugin.OnSignal` 签名、`Collector` 实现、`schema` 定义。

## 6. 边界与风险

1. **异步消费是软约束**：无法在代码层强制。文档必须写明"worker-pool 下严禁在 OnSignal 调用栈内同步消费 collector"，并将异步消费作为两种模式统一的推荐姿势。违反则 worker-pool 下可能偶发死锁。
2. **异步消费自带并发**：发送方在独立 goroutine 消费结果时，若读写节点自身状态会与其事件循环并发（两种模式皆然，是"异步"本身带来的）。文档提示：消费 goroutine 内不要访问节点状态，或自行加锁；对"聚合后发出/落库"这类无状态用法无影响。
3. **Done 兜底覆盖率**：任一丢弃路径漏掉 `Done()` 都会导致发送方 goroutine 泄漏/挂起。需在实现与测试中逐路径覆盖。
4. **被 ask 的下游节点建议无状态**：契合"单纯执行节点"的定位。

## 7. 测试计划

- 两种调度模式下，`AskSignal` 收齐多个直接下游的 `Result`（数量、内容、channel 正常关闭）。
- 转发型下游（返回 nil）：collector 正常关闭，发送方只收到非 nil 结果。
- 节点停止 / ctx 取消 / channel 满：发送方消费 goroutine 不挂起（Done 兜底生效）。
- 下游 `OnSignal` panic：collector 仍 `Done`，发送方不挂起。
- 嵌套 ask：下游在 `OnSignal` 内再 `AskSignal`，各自独立收集，无死锁。
- 无下游节点：返回构造即关闭的空 collector，`range` 立即结束。
- （回归）移除 deadline 后，耗时 > 100ms 的 signal/payload 处理不再被取消，仅产生警告日志。
