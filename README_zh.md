# Voce

**支持打断与流式处理的实时语音 AI 流水线引擎（ASR → LLM → TTS）**

Voce 是一个基于 Go 开发的**个人探索项目**，用于研究如何构建：

- 低延迟（low-latency）
- 高并发（high-concurrency）
- 流式（streaming）

的实时 AI 处理系统。

当前主要聚焦于语音对话链路（ASR → LLM → TTS），但整体架构设计目标是逐步扩展为：

> **一个通用的实时多模态编排引擎（multimodal orchestration runtime）**

---

## ⚡ Highlights

- 实时语音 AI 流水线（ASR → LLM → TTS）
- 可立即打断正在进行的 LLM / TTS 生成（流式打断）
- 数据在不同的 node 和 track 中独立处理，无阻塞
- 内置 realtime 系统的 backpressure 与丢包策略
- 5000 并发会话下，P99 延迟低于 50ms

---

## 🤔 Why

这个项目最初是为了解决一些实际遇到的问题：

- 实时语音对话链路中延迟波动较大
- 下游节点变慢时系统容易堆积甚至 OOM
- 打断（interrupt）在流式系统中很难处理干净
- 异步调用（例如 LLM、TTS）会产生过期结果

因此 Voce 更像是一个：

> **系统设计实验（system-level prototype）**

用于探索：

- 实时流系统如何调度
- Go 中如何降低分配和 GC 抖动
- DAG 编排是否适合实时 AI pipeline

---

## 🧠 Current Capabilities

目前，Voce 主要面向纯语音、基于 Socket 的实时交互场景。
通过 Plugin + 声明式 DAG 编排，可以实现例如：

- **全双工语音对话**

  ```text
  Socket -> ASR -> Interrupter -> LLM -> TTS -> Socket
  ```

- **实时同声传译**

  ```text
  Socket -> ASR -> Translate -> TTS -> Socket
  ```

> 当前内置插件与 Socket 主要围绕对话场景设计。

---

## 🔮 Future Direction

这个项目未来可能探索的方向（不保证实现）：

- WebRTC transport plugin（基于 RTC 的实时音视频接入）
- 实时对话中的语音指令识别和情感检测
- 更通用的实时编排 runtime（不仅限对话场景）

👉 以上方向主要用于探索和实验，不构成正式 **roadmap**。

---

## 🧩 Built-in Plugins

Voce 目前支持包括 ASR、LLM、TTS 在内的多种实时处理插件，完整列表及配置说明请参阅：

👉 [内置插件列表](docs/plugins_list.md)

---

## 🗺️ Built-in Workflow

- benchmark：用于压测
- realtime_voice：大模型的全双工实时语音对话

## 📦 Project Structure

```text
.
├── biz/                # 会话 / WebSocket / RESTful
├── internal/
│   ├── engine/         # DAG 调度与运行时
│   ├── protocol/       # 自定义通信协议
│   ├── schema/         # 数据模型（Audio / Video / Payload / Signal）
│   ├── plugins/        # 插件系统
│   └── ...
├── pkg/                # 工具包
├── cmd/
│   ├── voce/           # 服务端入口
│   └── bench/          # 压测工具
├── clients/
│   ├── web/            # Web编排界面
│   └── voce-tui/       # 终端客户端
```

---

## 🚀 Quick Start

### 依赖准备

Voce 的音频处理部分依赖 **FFmpeg (libswresample 与 libavutil)** 开发库，编译前请确保已安装：

- **macOS**: `brew install ffmpeg`
- **Ubuntu/Debian**: `sudo apt-get install libswresample-dev libavutil-dev`

### 1. 本地编译运行

```bash
git clone https://github.com/wnnce/voce.git && cd voce

make build-all

mkdir -p configs && cp config.yaml.example configs/config.yaml

./bin/voce -c configs/config.yaml
```

### 2. Docker 部署

```bash
git clone https://github.com/wnnce/voce.git && cd voce

mkdir -p configs && cp config.yaml.example configs/config.yaml

docker-compose up -d

make build-tui
```

更多详细信息请参阅 **[快速开始指南](docs/quick_start.md)**。

---

### Web 编排界面

浏览器访问 [localhost:7001](http://localhost:7001)，编排或者修改节点配置。

![](images/2.png)

### TUI 客户端

运行终端 tui 体验全双工对话

```bash
./bin/voce-tui
```

![img.png](images/3.png)

---

## 🧱 Core Design

### 1. ReadOnly / Mutable 模型

默认只读，修改时 Copy-on-Write：

```go
mutable := payload.Mutable()
mutable.Set("processed", true)
flow.SendPayload(mutable.ReadOnly())
```

---

### 2. Low-Allocation 思想

- 对象池
- 引用计数
- 内存复用

👉 目标：减少 GC 抖动

---

### 3. Signal 优先级调度

系统控制信号（如暂停）和信令信号优先于媒体数据。

---

### 4. Backpressure

慢节点会触发丢包或者 Canceled。

---

## 📊 Benchmark

环境：MacBook Pro M5 / 24GB RAM

| Users    | Duration | Packets   | Avg  | P95   | P99   | MIN/MAX   |
| :------- | :------- | :-------- | :--- | :---- | :---- | :-------- |
| **10**   | 30s      | 5,990     | 1 ms | 2 ms  | 2 ms  | 0 / 6 ms  |
| **500**  | 30s      | 296,200   | 2 ms | 3 ms  | 4 ms  | 0 / 12 ms |
| **1000** | 1m       | 1,185,200 | 2 ms | 5 ms  | 7 ms  | 0 / 30 ms |
| **2000** | 1m       | 2,342,000 | 4 ms | 7 ms  | 17 ms | 0 / 45 ms |
| **5000** | 1m       | 5,637,000 | 4 ms | 11 ms | 32 ms | 0 / 61 ms |

👉 内存约 300MB，GC pause 稳定

![tui](images/1.png)

---

## ⚠️ Project Status

> 本项目是面向实时 AI 系统的个人工程探索项目。

- 核心设计与架构相对稳定
- 当前不以生产环境可用为目标
- 不承诺明确的 roadmap 或持续维护

---

## 📚 Lessons Learned

- Copy-on-write 配合引用计数在 DAG 调度的场景下效果很好
- 控制信号（如 interruption）优先于媒体数据，对实时交互非常重要
- Backpressure 是长连接流式系统的基础能力，而不是可选优化
- 在 Go 中减少 allocation 对尾延迟（tail latency）提示非常明显

---

## 🔗 Links

- [核心特性](docs/key_features.md)
- [插件开发指南](docs/plugin.md)
- [快速开始](docs/quick_start.md)
- [接入协议](docs/protocol.md)
- [内置插件列表](docs/plugins_list.md)
- [压测说明](docs/benchmark.md)

## 💡 灵感来源（Inspiration）

Voce 的部分设计灵感来源于 TEN Framework。

尤其是在将实时处理流程抽象为图结构（graph-based orchestration），以及通过结构化数据流解耦组件这两点上，对早期设计产生了影响。

当前这个仓库更接近一次基于这些经验的重新设计与实现。
