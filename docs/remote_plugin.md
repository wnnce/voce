# 远程插件开发指南 (Remote Plugin)

> [!WARNING]
> **实验性功能 (Experimental)**：Remote Plugin 目前处于实验性阶段。其底层通信协议、SDK 接口和事件模型可能会在后续版本中发生不向后兼容的变更。目前仅推荐在非生产环境或探索性场景下使用。
> 此外，第一版仅支持 `Signal` 和 `Payload` 消息事件，暂不支持 `Audio` 和 `Video` 实时流的远程传输。

## 1. 核心概念 (Core Concepts)

Voce 的核心运行时由 Go 编写，而许多 AI 生态工具库（如 LangChain、LlamaIndex 等）主要由 Python 和 Node.js 提供。为了支持跨语言扩展，Voce 引入了 Remote Plugin 机制。

Remote Plugin 并非作为子进程启动，而是作为独立的 **Remote Plugin Server**（gRPC 服务）运行。Voce 主服务启动时会连接该 Server 并拉取插件元数据；在运行时，Go 端的 Adapter 会将远端插件包装为本地的 `engine.Plugin`，并通过双向事件流（Bidirectional Streaming）进行数据交互。

- **隔离性**：远端插件以独立进程运行，其崩溃或阻塞不会直接拖垮主服务。
- **一致性**：远端插件的生命周期、事件分发与取消机制，在协议层面与本地 Go 插件保持一致。

---

## 2. 生命周期与事件 (Lifecycle & Events)

远端插件与本地插件共享一致的生命周期模型，开发者可以在 SDK 中实现对应的钩子函数：

- `OnStart` / `OnReady` / `OnStop`：在 Workflow 的不同生命周期被调用。
- `OnPause` / `OnResume`：在客户端断连或主动暂停时触发。
- `OnSignal`：处理控制信令（如打断信号）。
- `OnPayload`：处理业务数据（如 ASR 文本、LLM 回复）。

> [!NOTE]
> **取消机制 (Cancellation)**
> 当 Go 端发生超时或干预从而中断当前节点的任务时，会通过 gRPC 流向远端下发 `Cancel` 事件。SDK 内部会捕获该事件并尝试取消对应的异步任务。

---

## 3. SDK 开发 (SDK Development)

官方目前提供了 Python 和 Node.js 的实验性 SDK。SDK 封装了底层的 gRPC 交互、Schema 序列化及日志回传机制。

### 3.1 Python 插件

源码路径：`sdks/remote_plugin/python/`

在 Python 中，通过继承 `AsyncPlugin` 并结合 `pydantic.BaseModel` 定义配置结构：

```python
import asyncio
from pydantic import BaseModel
from voce.core.plugin import AsyncPlugin
from voce.schema import Payload
from voce.app import App

# 1. 定义插件配置 (Configuration Schema)
class MyConfig(BaseModel):
    prompt: str = "Translate to English:"
    max_tokens: int = 100

# 2. 实现插件逻辑
class MyPlugin(AsyncPlugin[MyConfig]):
    
    async def on_start(self, flow):
        self.logger.info("Plugin started, initializing resources...")

    async def on_payload(self, flow, payload):
        text = payload.properties.get("text", "")
        self.logger.info(f"Received text: {text}")
        
        # 组装结果并发送至下游节点
        await flow.send_payload(Payload(name="translated_text", properties={
            "result": f"{self.config.prompt} {text}",
            "is_final": True,
        }))

# 3. 注册并启动 gRPC 服务
async def main():
    app = App()
    
    app.register_plugin(
        name="python.my_plugin",
        description="A Python plugin for translation",
        config_schema=MyConfig,
        plugin_class=MyPlugin
    )
    
    await app.serve(port=50051)

if __name__ == "__main__":
    asyncio.run(main())
```

### 3.2 Node.js 插件

源码路径：`sdks/remote_plugin/nodejs/`

Node.js 版本采用函数式定义，结合 Zod 库提供配置校验。

```typescript
import { z } from 'zod'
import { definePlugin, pluginRegistry } from '@voce/remote-plugin'

// 1. 定义配置 Schema
const MyConfigSchema = z.object({ 
    targetLanguage: z.string().default("en") 
})

// 2. 声明并定义插件
const myPlugin = definePlugin({
  name: 'node.my_plugin',
  description: 'A Node.js plugin',
  configSchema: MyConfigSchema,
  
  // 可选：声明 inputs / outputs 参与前端连线校验
  inputs: [ { type: 'payload', name: 'source_text' } ],
  outputs: [ { type: 'payload', name: 'processed_result' } ],
  
  // 3. setup 在 Workflow 创建实例时调用
  setup({ config, logger, instanceId }) {
    logger.info(`Instance ${instanceId} created.`)

    return {
      async onPayload(payload, flow) {
        await flow.sendPayload({
            name: "processed_result",
            properties: { done: true }
        })
      }
    }
  }
})

// 注册插件
pluginRegistry.register(myPlugin)
// 随后需要启动关联的 gRPC Server
```

---

## 4. 服务配置与集成 (Integration)

远端插件服务启动后，需要在 Voce 主服务的配置文件 `configs/config.yaml` 或 `configs/gateway.yaml` 中进行声明：

```yaml
# configs/config.yaml
remote_plugins:
  - name: "python_ai_server"
    address: "127.0.0.1:50051"
    enable: true
  - name: "node_utils_server"
    address: "127.0.0.1:50052"
    enable: true
```

Voce 启动时会自动通过 `Ping` 接口进行连接，并通过 `ListPlugins` 接口拉取可用插件列表。之后在 Workflow 配置或前端 UI 中即可直接选择 `python.my_plugin` 或 `node.my_plugin`。

---

## 5. 运行限制与注意事项 (Limitations & Notes)

在使用 Remote Plugin 时，需注意以下机制与限制：

1. **实例生命周期**：每次会话执行到远端节点时，都会在远端创建一个独立的 Instance。远端服务需自行管理这些并发实例的内存状态，在 Session 销毁时清理资源。
2. **日志上报**：通过 SDK 提供的 `logger` 打印的日志会异步透传至 Go 端，由主服务统一附带 Session ID 等上下文后输出。
3. **数据类型限制**：受限于 gRPC 序列化与网络开销，当前版本仅开放 `Signal` 和 `Payload`，不支持 `Audio` 和 `Video` 实时流的传输。
4. **异常传递**：远端抛出的未捕获异常会被 SDK 拦截，并转换为错误事件通知 Go 端，Go 端仅记录错误日志，不会自动销毁当前 Workflow。
