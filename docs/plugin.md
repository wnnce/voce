# 插件开发指南 (Plugin Development)

Voce 的核心能力在于灵活的插件扩展。所有处理逻辑（ASR, LLM, TTS 等）都以插件的形式注册到服务中。

## 1. 插件配置与 Schema 自动渲染

通过实现 `PluginConfig` 接口，前端工作台可根据 `Schema()` 方法生成的 `jsonschema` 自动渲染表单，并在 `OnStart` 时自动注入配置。

```go
type PluginConfig interface {
    Schema() *jsonschema.Schema
    Decode(data []byte) error
}


type MyConfig struct {
    Token  string   `json:"token" jsonschema:"title=访问令牌,description=API调用凭证"`
    Models []string `json:"models" jsonschema:"title=可选模型,description=支持的模型列表"`
}

func (c *MyConfig) Schema() *jsonschema.Schema {
    return jsonschema.Reflect(c) // 自动为前端生成表单描述
}

func (c *MyConfig) Decode(data []byte) error {
    return sonic.Unmarshal(data, c) // 解析下发的 JSON 配置
}
```

## 2. 可变性模型：ReadOnly 与 Mutable

Voce 采用 **“发送即冻结”** 的数据模式，主要涉及两个状态：

- **ReadOnly (只读)**：插件通过 `OnPayload/OnAudio/OnVideo` 接收到的数据默认都是**只读**的。确保多个下游节点可以并发读取同一份内存。
- **Mutable (可写)**：如果想修改收到的数据再转发，或者创建一个全新的数据。调用 `Mutable()` 方法会根据当前对象的冻结状态决定是否返回一个新的副本。

### 2.1 只读强制执行与防御性设计

为了防止由于持有过期指针导致的数据竞态（Data Race），Voce 强制执行只读契约。一旦对象调用了 `.ReadOnly()` 变为只读状态：

- **通用属性修改 (`Set`)**：调用 `Set(key, value)` 将会返回 `schema.ErrReadOnly` 错误。
- **媒体数据修改 (`SetBytes`, `SetYUV`, `SetSampleRate` 等)**：调用这些方法将直接触发 **`panic`**。
  - **设计初衷**：媒体数据的篡改通常意味着严重的逻辑错误或内存破坏风险。通过 `panic` 可以在开发阶段强制拦截违规代码。

```go
func (p *MyPlugin) OnPayload(ctx context.Context, flow engine.Flow, payload schema.Payload) {
    // 获取一个可写的副本 (如果已冻结，底层会自动做一次 Shallow Clone)
    mutable := payload.Mutable()

    // 修改数据
    if err := mutable.Set("processed", true); err != nil {
        p.Logger().Error("failed to set property", "error", err)
        return
    }

    // 调用 .ReadOnly() 标记为只读状态并发出
    // 此后 mutable 指针所指向的对象已被“冻结”，不可再修改
    flow.SendPayload(mutable.ReadOnly())
}
```

## 3. 引用计数与内存管理 (Reference Counting)

Voce 的媒体数据（Audio/Video）存储在对象池中以减少 GC 压力。为了确保内存安全且零拷贝，开发者必须遵循引用计数协议：

### 核心规则

- **谁申请，谁释放 (Create & Release)**：如果你使用 `schema.NewAudio` 或 `audio.Mutable()` 手动创建了数据或可写副本。在调用 `flow.SendAudio` 后，必须紧接着调用一次 `.Release()`（通常使用 `defer au.Release()`）。
- **同步接收是安全的 (Safe Buffering)**：在 `OnAudio/OnVideo` 等回调中，可以保证数据的生命周期完整覆盖整个函数执行过程。在函数返回后，上层会自动调用 `Release`。
- **异步处理必须增加计数 (Async Ownership)**：在插件内部开启了**额外的 goroutine** 异步处理收到的媒体数据，或者需要将接收到的媒体数据作为变量保存，必须手动调用 `.Retain()`。

### 代码示例

#### A. 发送自定义生成的音频数据 (如 TTS)

```go
func (p *TTSPlugin) sendAudio(flow engine.Flow, buf []byte) {
    au := schema.NewAudio(schema.AudioTTS, engine.AudioSampleRate, engine.AudioChannels)
    // 重要：函数返回前必须释放，NewAudio 初始化计数为 1
    defer au.Release()

    au.SetBytes(buf)
    // SendAudio 内部会根据下游 Consumer 数量自动 AddRetain()
    flow.SendAudio(au.ReadOnly())
}
```

#### B. 在协程中异步处理接收的数据

```go
func (p *MyPlugin) OnAudio(ctx context.Context, flow engine.Flow, audio schema.Audio) {
    // 错误做法：直接在协程里使用。函数返回后，引擎会回收该内存，导致协程读到乱码。
    // go func() { process(audio) }()

    // 正确做法：
    audio.Retain() // 增加引用计数，手动持有内存所有权
    go func() {
        defer audio.Release() // 处理完成后释放
        p.slowTask(audio)
    }()
}
```

## 4. 插件生命周期 (Plugin Lifecycle)

插件通过 `Plugin` 接口可以介入 Workflow 的完整生命周期。以下是各个钩子的触发时机与作用：

- **`OnStart(ctx context.Context, flow Flow) error`**：
  - **触发时机**：Workflow 启动阶段。
  - **作用**：用于初始化插件内部资源（如建立网络连接、初始化状态等）。
  - **规则**：若返回 `error`，Workflow 将终止启动并向客户端上报错误。

- **`OnReady(ctx context.Context, flow Flow)`**：
  - **触发时机**：Workflow 所有节点均已启动成功，进入正常运行状态之后。
  - **作用**：表示系统已准备好接收并处理实时流数据。

- **`OnPause(ctx context.Context)`**：
  - **触发时机**：当客户端断开连接（如由于网络波动进入重连态）或主动触发暂停指令时触发。
  - **系统行为**：Workflow 会停止源头数据的输入。
  - **数据行为**：对于已经存在于节点处理队列（Channel）中的历史数据，依然会照常执行下发；但对于处于 `MultiTrackPlugin` 异步轨道缓冲区中的残留数据，会自动进行丢弃。
  - **插件职责**：插件应当在此处保存当前的会话状态（如 LLM 的历史 Context），停止不必要的计算任务，等待恢复。

- **`OnResume(ctx context.Context, flow Flow)`**：
  - **触发时机**：Workflow 从暂停状态恢复为运行状态。
  - **作用**：用于恢复插件的处理逻辑。

- **`OnStop()`**：
  - **触发时机**：Workflow 被正常销毁或被系统强制回收时触发。
  - **作用**：最后的数据清理工作。此方法调用后，插件实例将被释放。

## 5. 同步插件示例 (Sync Plugin)

同步插件直接运行在节点的主事件循环中，上层确保所有的 OnXXX 回调都是串行调用的，适用于耗时较短的逻辑（如文本清洗、数据转换、或者简单的 IO 操作）。

> **运行限制**：为了保证整个 DAG 处理链路的实时性，针对同步插件有严格的执行时间监控（默认阈值为 **100ms**，可配置）：
>
> - **Signal / Payload**：传入的 `ctx` 带有 100ms 的硬性 Deadline，超时将触发 Context Canceled。
> - **Audio / Video**：若单次处理耗时超过 100ms，会记录 `handler execution slow` 警告日志。
> - 如果处理逻辑可能大幅超过该阈值，请务必使用下文提到的**异步插件**模式。

```go
type FilterPlugin struct {
    engine.BuiltinPlugin
    keyword string
}

func NewFilterPlugin(cfg *FilterConfig) engine.Plugin {
    return &FilterPlugin{keyword: cfg.Keyword}
}

func (p *FilterPlugin) OnPayload(ctx context.Context, flow engine.Flow, payload schema.Payload) {
    text := schema.GetAs[string](payload, "text", "")

    if !strings.Contains(text, p.keyword) {
        flow.SendPayload(payload) // 直接透传
        return
    }

    // 命中敏感词，获取 Mutable 副本进行清洗
    mutable := payload.Mutable()
    mutable.Set("text", strings.ReplaceAll(text, p.keyword, "***"))

    flow.SendPayload(mutable.ReadOnly())
}
```

## 6. 异步插件示例 (Multi-Track Async Plugin)

异步插件通过 `MultiTrackPlugin` 进行包装，运行在独立协程中。与普通节点的单线程循环不同，`MultiTrackPlugin` 为 Audio、Video 和 Payload 开启了**并行的处理轨道**。

> **并发安全警告**：如果插件配置了多个轨道（例如同时开启了 Audio 和 Payload 轨道），那么 `OnAudio` 和 `OnPayload` 可能会在不同的协程中**同时被调用**。此时，插件实现必须是**线程安全**的。

```go
type LLMPlugin struct {
    engine.BuiltinPlugin
    client *http.Client
}

func NewLLMPlugin(cfg *LLMConfig) engine.Plugin {
    p := &LLMPlugin{client: &http.Client{}}

    // 使用 MultiTrackPlugin 进行异步包装
    return engine.NewMultiTrackPlugin(p,
        // 为 Payload 开启异步轨道：缓冲区 128，阻塞模式，监听打断信号
        engine.WithPayloadTrack(128, engine.BlockIfFull, schema.SignalInterrupter),
        // 为 Audio 开启异步轨道：缓冲区 64，满时丢弃最新帧 (保证实时性)
        engine.WithAudioTrack(64, engine.DropNewest),
    )
}

func (p *LLMPlugin) OnPayload(ctx context.Context, flow engine.Flow, data schema.Payload) {
    // 此处运行在 Payload 轨道的独立协程中，可以执行阻塞调用
    // 如果收到 SignalInterrupter 信号，当前执行的 ctx 会被 Canceled
    resp, err := p.client.Post("https://api.openai.com/v1/chat/completions", ...)
    // ...
}
```

### 关键特性

- **并行解耦**：慢速的 Payload 处理（如 LLM 推理）不会阻塞实时的音频流传输。
- **任务打断**：通过配置 `signals`，可以在接收到特定信号时立即取消当前正在运行的任务（Cancel Context）并清空缓冲区。
- **背压策略 (Drop Strategy)**：
  - `BlockIfFull`: 缓冲区满时阻塞上游，确保数据不丢失。
  - `DropNewest`: 缓冲区满时丢弃最新进入的数据包，适用于实时性要求极高的音频/视频流。

## 7. 插件注册 (Registration)

Voce 使用 Go 的 `init()` 机制实现插件的注册：

1.  **代码内注册**：在插件包的 `init()` 函数中调用 `engine.RegisterPlugin` 定义元数据与输入/输出契约（Property）。

```go
func init() {
    if err := engine.RegisterPlugin(NewPlugin, engine.PluginMetadata{
        Name: "my_plugin",
        Description: "一个简单的文本转换插件示例",
        // 定义输入契约：期望上级输出 payload.name 为 source 的数据
        // 且必须携带一个 string 类型的 text 字段
        Inputs: engine.NewPropertyBuilder().
            AddPayload("source", "text", engine.TypeString, true).
            Build(),
        // 定义输出契约：输出 payload.name 为 dest 的数据
        // 且一定会携带一个 string 类型的 result 字段
        Outputs: engine.NewPropertyBuilder().
            AddPayload("dest", "result", engine.TypeString, true).
            Build(),
    }); err != nil {
        panic(err)
    }
}
```

2.  **触发匿名导入**：新建插件包后，必须在 **`internal/plugins/init.go`** 中通过匿名导入引入该插件包，否则其 `init()` 函数不会执行。

```go
package plugins

import (
    // ... 其他插件
    _ "github.com/wnnce/voce/internal/plugins/my_plugin" // 必须手动导入以便触发注册逻辑
)
```

## 8. 单元测试 (Testing)

Voce 提供了一套 **`PluginTester`** 测试脚手架，支持开发时在脱离完整 DAG 环境的情况下对单个插件进行单元测试。

- **动态捕获回调**：通过 `tester.OnPayload(func(port int, p schema.Payload) { ... })` 捕获插件输出。其中 `port` 默认为 `0`（广播端口）。
- **Activity 追踪机制**：`PluginTester` 会自动追踪插件的生命周期，每发生一次数据流转，都会触发内部 Activity 计数。
- **自动退出保护**：调用 `tester.Wait()` 会阻塞测试进程。如果持续 **10 秒**（默认值，可配置）没有任何数据活动，则会自动判定为超时退出。
- **手动结束**：获取到预期数据后，调用 `tester.Done()` 可立即结束阻塞。

```go
func TestMyPlugin(t *testing.T) {
    // 初始化插件
    p := NewPlugin(engine.EmptyPluginConfig{})

    // 创建 Tester 实例
    tester := engine.NewPluginTester(t, p)

    // 注册输出捕获回调：当插件通过 flow.SendPayload 发送数据时触发
    tester.OnPayload(func(port int, payload schema.Payload) {
        assert.Equal(t, "expected_result", schema.GetAs[string](payload, "text", ""))

        // 获取到预期数据之后调用 Done() 标记完成
        tester.Done()
    })

    // 启动插件生命周期
    tester.Start()

    // 注入测试数据
    mockInput := schema.NewPayload(schema.PayloadASRResult)
    mockInput.Set("text", "hello world")
    tester.InjectPayload(mockInput.ReadOnly())

    // 阻塞等待 直到超时或者调用 tester.Done()
    tester.Wait()

    tester.Stop()
}
```
