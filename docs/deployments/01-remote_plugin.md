# Remote Plugin 初版设计

本文档描述 Voce Remote Plugin 的初版设计。目标是在不破坏现有 Go Runtime 边界的前提下，让 Python、Node.js 等语言可以编写 Voce 插件。

Remote Plugin 不是把脚本作为 Go 子进程启动，而是由独立的 Remote Plugin Server 承载插件实现。Voce 主服务通过 gRPC 与远端服务通信。

---

## 1. 设计目标

Remote Plugin 的核心目标：

- 支持非 Go 语言编写 Voce 插件。
- 保持 Go Runtime 作为唯一的 DAG 调度、Session 生命周期和数据流所有者。
- 让远端插件复用现有插件生命周期语义。
- 为 Python / Node.js SDK 提供稳定协议边界。
- 避免把高频音视频数据直接放入初版 remote plugin 链路。

第一版只支持：

- `OnSignal`
- `OnPayload`

暂不支持：

- `OnAudio`
- `OnVideo`
- 零拷贝媒体对象传输
- 远端实例状态透明恢复

原因是 Audio / Video 涉及高频数据、引用计数、对象池和实时背压语义，初版通过 gRPC 远程传输成本较高，也容易破坏当前 runtime 的内存模型。

---

## 2. 总体架构

Remote Plugin 由三部分组成：

1. **Voce Go Runtime**
   - 负责 workflow、DAG、session、节点调度和 schema 生命周期。
   - 通过本地 adapter 把 remote plugin 包装成普通 `engine.Plugin`。

2. **Remote Plugin Server**
   - 独立进程，可以由 Python、Node.js 或其它语言实现。
   - 负责插件注册、实例创建、实例销毁和事件处理。

3. **Remote Plugin SDK**
   - 为 Python / Node.js 提供插件基类/定义方法、schema 编解码、日志回传和 gRPC server 骨架。
   - 插件开发者只需要实现业务逻辑。

示意：

```text
Client
  |
  v
Voce Runtime
  |
  | DAG event: Signal / Payload
  v
Remote Plugin Adapter (Go)
  |
  | gRPC bidirectional stream
  v
Remote Plugin Server
  |
  v
Remote Plugin Instance
```

Go 侧仍然只看到一个普通插件：

```text
engine.Plugin
  OnStart
  OnReady
  OnPause
  OnResume
  OnStop
  OnSignal
  OnPayload
```

其中 `OnAudio` / `OnVideo` 在 remote plugin 初版中不开放。

---

## 3. 运行时边界

Remote Plugin 不应成为第二套 runtime。

Go Runtime 负责：

- workflow 加载和校验
- DAG 拓扑和节点调度
- signal 优先级
- payload fan-out
- session 生命周期
- pause / resume / stop
- 本地背压策略
- schema 只读语义
- 取消事件（Cancel）下发

Remote Plugin Server 负责：

- 暴露插件元数据
- 创建插件实例
- 执行远端插件回调
- 返回事件处理结果
- 上报插件日志

远端插件不能直接控制 DAG，也不能绕过 Go Runtime 向任意节点发送数据。它只能通过协议返回本次事件产生的输出。

---

## 4. 插件发现

Voce 启动时读取 remote server 配置，并向每个 remote server 拉取插件列表。

Remote server 需要返回与本地插件一致的元数据：

- `name`
- `description`
- `schema`
- `inputs`
- `outputs`
- `ports`
- `multi_track`

Go 侧会把这些 remote plugin 注册为 `PluginBuilder`，使它们可以和本地插件一样参与 workflow 校验与前端展示。

多 remote server 场景通过 `PluginResource` 的 `namespace` 隔离插件来源。`namespace` 不会被拼接到插件名中，Go 侧会通过 `namespace + plugin_name` 定位具体插件；如果调用方不指定 namespace 且多个资源中存在同名插件，则应视为歧义。

推荐为 remote server 配置稳定 namespace：

```text
python
node
```

也可以让远端插件自身使用带前缀的插件名，例如 `python.text_filter`、`node.webhook`，但这是插件名本身的一部分，不是 Go 侧自动拼接结果。

---

## 5. 实例生命周期

Remote plugin 的实例生命周期与本地插件保持一致。

### 5.1 创建实例

当 workflow 启动并构建到 remote plugin 节点时，Go adapter 调用 remote server：

```text
CreateInstance(plugin_name, instance_id, config)
```

remote server 根据插件名称和配置创建一个插件实例，并返回创建结果。

`instance_id` 由 Go Runtime 生成，用于在后续流、日志和销毁请求中定位实例。

### 5.2 建立事件流

实例创建成功后，Go adapter 与该实例建立一个独占的 gRPC 双向流：

```text
RunInstance(stream RuntimeMessage) returns (stream RuntimeMessage)
```

一个 remote plugin instance 对应一条独占 stream。这样可以避免不同实例之间的事件和日志相互串扰。

### 5.3 销毁实例

workflow 停止或 session 销毁时，Go adapter 调用：

```text
DestroyInstance(instance_id)
```

remote server 应释放该实例关联的资源。

如果 stream 已经异常断开，Go 侧仍可尝试发送销毁请求；remote server 应将销毁操作设计为幂等。

---

## 6. 事件模型

初版支持以下输入事件：

- `start`
- `ready`
- `pause`
- `resume`
- `stop`
- `signal`
- `payload`
- `cancel`

其中对插件开发者开放的业务回调主要是：

- `OnSignal`
- `OnPayload`

生命周期事件用于让 SDK 触发插件生命周期方法，保持和本地插件模型一致。当 Go 端取消某次执行时，会发送 `cancel` 事件。

每条运行时消息都有 `message_id`。如果一条消息由另一条输入事件触发，则通过 `correlation_id` 关联原始消息。

`message_id` 使用字符串，由发送方生成：

- Go 侧可以使用 UUID。
- Remote 侧也可以使用 UUID 或 `<server_id>_<seq>`。
- `correlation_id` 为空字符串表示无关联。

Go 侧发送输入事件时分配 `message_id`：

```json
{
  "message_id": "uuid-1",
  "correlation_id": "",
  "type": "RUNTIME_MESSAGE_TYPE_PAYLOAD",
  "payload": {
    "name": "asr_result",
    "properties": {
      "text": "你好",
      "is_final": true
    }
  }
}
```

remote plugin 处理事件后，应返回：

- 零个或多个输出事件
- 可选 ack
- 一个最终 report

输出事件可以是：

- `emit_signal`
- `emit_payload`
- `emit_log`

remote 返回的输出消息使用新的 `message_id`，并通过 `correlation_id` 指向触发它的输入事件。

`ack` 表示远端已接收事件，`report` 表示远端已完成该事件的主处理流程。

---

## 7. Ack / Report 机制

Go Runtime 需要知道远端插件是否已经接收并处理完某个输入事件。因此每个输入事件可以先返回 ack，并且必须最终返回 report。

Ack 示例：

```json
{
  "message_id": "remote-uuid-1",
  "correlation_id": "uuid-1",
  "type": "RUNTIME_MESSAGE_TYPE_ACK"
}
```

Report 示例：

```json
{
  "message_id": "remote-uuid-2",
  "correlation_id": "uuid-1",
  "type": "RUNTIME_MESSAGE_TYPE_REPORT",
  "report": {
    "status": "REPORT_STATUS_OK"
  }
}
```

错误示例：

```json
{
  "message_id": "remote-uuid-2",
  "correlation_id": "uuid-1",
  "type": "RUNTIME_MESSAGE_TYPE_REPORT",
  "report": {
    "status": "REPORT_STATUS_ERROR",
    "error": {
      "code": "plugin_error",
      "message": "failed to process payload"
    }
  }
}
```

Report 状态建议：

| 状态 | 说明 |
|------|------|
| `REPORT_STATUS_OK` | 远端回调正常完成 |
| `REPORT_STATUS_ERROR` | 远端 SDK 捕获到未处理异常，当前实例进入失败语义 |
| `REPORT_STATUS_CANCELED` | 事件因上下文取消或实例关闭终止 |

`report(error)` 是协议层的正常 stream 消息，不代表 gRPC stream 本身异常；但它的业务语义不是“可恢复业务失败”，而是远端插件回调发生未处理异常。Go adapter 收到 `REPORT_STATUS_ERROR` 后，应将本次 `doCall` 视为失败，并把该 remote plugin instance 标记为 failed，后续事件不再发送给该实例，等待生命周期清理。

如果业务逻辑需要表达可恢复失败，应通过普通 `emit_payload` / `emit_signal` 输出结构化结果，而不是使用 `REPORT_STATUS_ERROR`。如果该次调用超时或由于某些原因被取消，Go 端会通过 stream 下发 `CancelEvent`。

`correlation_id` 只表达因果关系，不等同于 pending event 的硬约束。remote plugin 可以在 report 后继续发送异步输出，此类输出仍可携带原始 `correlation_id`，但不再影响原事件的完成状态。

---

## 8. 输出语义

Remote plugin 不能直接访问 Go 侧 `engine.Flow`。

远端插件通过 stream 返回输出消息：

```json
{
  "message_id": "remote-uuid-3",
  "correlation_id": "uuid-1",
  "type": "RUNTIME_MESSAGE_TYPE_EMIT_PAYLOAD",
  "emit_payload": {
    "payload": {
      "name": "llm_chunk",
      "properties": {
        "sentence": "你好",
        "is_final": false
      }
    }
  }
}
```

Go adapter 收到后转换为本地 schema：

```go
flow.SendPayload(payload.ReadOnly())
```

Signal 输出同理：

```go
flow.SendSignal(signal.ReadOnly())
```

输出消息可以在 report 前发送多条，用于支持流式 payload。

如果 remote plugin 在后台任务中异步输出，也应分配新的 `message_id`。如果该输出由某次输入事件触发，可以继续填写 `correlation_id`；如果是定时器、订阅等主动事件，可以将 `correlation_id` 留空。

---

## 9. 日志回传

Remote Plugin SDK 应向插件注入标准 logger。

日志应同时支持：

- 输出到 remote server 本地 stdout / stderr
- 通过 gRPC stream 回传给 Go Runtime

日志消息示例：

```json
{
  "type": "RUNTIME_MESSAGE_TYPE_EMIT_LOG",
  "instance_id": "inst_xxx",
  "message_id": "remote-uuid-4",
  "correlation_id": "uuid-1",
  "emit_log": {
    "level": "LOG_LEVEL_INFO",
    "message": "remote plugin handled payload",
    "fields": {
      "plugin": "python.text_filter"
    }
  }
}
```

Go 侧接收到日志后，使用 `slog` 重新输出，并补充 session、node、plugin、instance 等上下文字段。

日志回传不能阻塞插件主处理路径。SDK 应使用缓冲队列或非阻塞写入，避免日志流量影响实时链路。

---

## 10. 心跳与重连

初版心跳不使用长连接，Go 主服务定期向每个 Remote Plugin Server 发送一元 `Ping` 请求。

心跳用途：

- 判断 remote server 是否存活。
- 发现服务不可用或网络异常。
- 服务从不可用恢复后，重新拉取插件列表。

`Ping` 请求应设置较短超时，避免 remote server 卡住时阻塞本地运行时管理逻辑。

当 remote server 挂掉：

- 已创建的 remote plugin instance 视为失败。
- 对应双向流关闭。
- Go adapter 应让当前实例进入不可用状态。
- 当前事件返回错误或取消。

当 remote server 重新上线：

- `Ping` 请求恢复成功。
- Go manager 重新拉取插件列表。
- 新 session 可以创建新的 remote plugin instance。
- 已失败的旧实例不做透明恢复。

原因是远端插件实例的内存状态已经丢失，透明重连会制造错误的一致性假象。

---

## 11. 故障语义

### 11.1 事件处理失败

remote plugin 返回 `report(error)` 时：

- Go adapter 记录错误日志。
- 当前事件视为处理失败。
- 初版不自动停止整个 workflow。

### 11.2 stream 异常断开

如果实例双向流断开：

- 当前 pending event 标记为 `canceled` 或 `error`。
- 后续事件不再发送给该实例。
- Go adapter 应避免永久阻塞上游节点。

### 11.3 remote server 进程重启

remote server 重启后，所有旧 instance 均视为丢失。

Go manager 可以重连并刷新插件列表，但不会把旧 instance 绑定到新连接。

---

## 12. Proto 核心接口

具体字段以 `api/plugin/v1/plugin.proto` 为准。核心接口形态如下：

```proto
service RemotePluginService {
  rpc Ping(PingRequest) returns (PingResponse);
  rpc ListPlugins(ListPluginsRequest) returns (ListPluginsResponse);
  rpc CreateInstance(CreateInstanceRequest) returns (CreateInstanceResponse);
  rpc DestroyInstance(DestroyInstanceRequest) returns (DestroyInstanceResponse);
  rpc RunInstance(stream RuntimeMessage) returns (stream RuntimeMessage);
}
```

核心消息：

```proto
message RuntimeMessage {
  string instance_id = 1;
  string message_id = 2;
  string correlation_id = 3;
  RuntimeMessageType type = 4;
  map<string, string> metadata = 5;

  oneof body {
    LifecycleEvent lifecycle = 10;
    SignalEvent signal = 11;
    PayloadEvent payload = 12;
    EmitSignal emit_signal = 13;
    EmitPayload emit_payload = 14;
    EmitLog emit_log = 15;
    EventReport report = 16;
    EventAck ack = 17;
    CancelEvent cancel = 18;
  }
}
```

其中 `CancelEvent` 允许 Go 端针对某一 `correlation_id` 进行远端取消。

---

## 13. SDK 设计

Python / Node.js SDK 至少应提供：

- 插件基类（Python）或函数式定义（Node.js）
- 配置 schema 定义方式（Pydantic / Zod）
- 插件元数据注册
- Remote Plugin Server 启动入口
- Payload / Signal 编解码
- logger 注入
- ack / report 自动处理
- 异常捕获并转换为 `report(error)`

### Python 插件示例形态：

```python
from pydantic import BaseModel
from voce.core.plugin import AsyncPlugin
from voce.schema import Payload

class MyConfig(BaseModel):
    threshold: float = 0.5

class MyPlugin(AsyncPlugin[MyConfig]):
    async def on_payload(self, flow, payload):
        text = payload.properties.get("text", "")
        await flow.send_payload(Payload(name="llm_chunk", properties={
            "sentence": text.upper(),
            "is_final": True,
        }))
```

### Node.js 插件示例形态：

```typescript
import { z } from 'zod'
import { definePlugin } from '@voce/remote-plugin'

const MyConfigSchema = z.object({ threshold: z.number().default(0.5) })

export const myPlugin = definePlugin({
  name: 'my-plugin',
  configSchema: MyConfigSchema,
  setup({ config, logger }) {
    return {
      async onPayload(payload, flow) {
        logger.info(`Received payload: ${payload.name}`)
        await flow.sendPayload(payload)
      }
    }
  }
})
```

开发者不应直接处理 gRPC stream。SDK 负责把协议消息转换成插件生命周期和回调。

---

## 14. 模块结构

Go 侧模块：

```text
api/plugin/v1/
  plugin.proto      # gRPC 协议文件

internal/remote/
  manager.go        # remote server 管理、心跳、重连、插件列表刷新
  builder.go        # remote PluginBuilder
  plugin.go         # remote plugin adapter，实现 engine.Plugin
  client.go         # gRPC client 封装
  call.go           # 调用封装（支持超时和 Cancel 下发）
```

SDK 模块存放位置：

```text
sdks/remote_plugin/python/
sdks/remote_plugin/nodejs/
```

---

## 15. 初版限制

初版明确不解决：

- Audio / Video remote 传输。
- 远端实例状态恢复。
- 远端插件之间直接通信。
- 远端插件动态热更新。
- 插件级别的强资源隔离。
- 跨语言 SDK 的完全一致运行时能力。

这些限制是为了先落地稳定的 Payload / Signal 远程插件能力。

---

## 16. 后续演进方向

可选演进方向：

- 支持 Remote Plugin 权限和资源限制。
- 支持 plugin server 动态更新插件列表。
- 为 Payload 提供更强 schema 校验。
- 增加事件超时和 per-plugin 限流。
- 支持批量事件或压缩传输。
- 在明确性能边界后评估 Audio / Video remote 支持。

Remote Plugin 的长期目标不是替代 Go 插件，而是为不适合用 Go 实现的能力提供跨语言扩展通道。
