# 压测说明 (Benchmark Guide)

Voce 的 Benchmark 工具旨在模拟高并发下的真实媒体流处理链路，不仅测试单点吞吐量，更专注于在高负载下维持 **极低且稳定的尾延迟 (Tail Latency)**。

[benchmark.go](../cmd/bench/main.go) [benchmark plugin](../internal/plugins/benchmark/plugin.go)

## 1. 压测设计 (Design Principles)

### Staggered Load (分批加载策略)

为了避免在测试开始瞬间产生巨大的并发冲击（Thundering Herd Problem），压测工具采用了 **Buckets (分批)** 机制：

- 5000 个用户被划分为 5 个 Bucket（每组 1000 人）。
- Bucket 之间的启动时间按发送间隔（Interval）进行错位平摊（如 50ms 间隔 / 5 组 = 每 10ms 启动一组）。
- 这种方式能更真实地模拟长连接服务的流量进入特征。

### 拓扑结构 (DAG Pipeline)

Benchmark 默认选用了 `benchmark` 这个特定的 Workflow， 其 DAG 路径如下：

![](../images/4.png)

- **转发节点 (Forwarder)**：纯透传，用于测试内部的节点调度。
- **记录节点 (Recorder)**：模拟 I/O 操作。它会将音频数据写入 `/dev/null`（模拟文件存储或分流）。
- 每个包在系统内部会经历 **4 次** 节点流转、状态切换和对象池回收。

### RTT 测量原理

- **数据包构造**：每个包大小为 1600 字节。
- **时间戳嵌入**：在音频 Payload 的前 8 个字节（BigEndian）嵌入发送时刻的 Unix 毫秒时间戳。
- **闭环计算**：当数据包经过 4 个 Node 并返回 Client 时，Client 通过解析时间戳计算端到端的 **RTT (Round Trip Time)**。

## 2. 压测数据回顾 (Performance Report)

**测试环境**：MacBook Pro M5 / 24GB RAM (本地回环)

| Users    | Duration | Packets   | Avg RTT | P95 RTT | P99 RTT   | Min/Max RTT |
| :------- | :------- | :-------- | :------ | :------ | :-------- | :---------- |
| **10**   | 30s      | 5,990     | 1 ms    | 2 ms    | 2 ms      | 0 / 6 ms    |
| **500**  | 30s      | 296,200   | 2 ms    | 3 ms    | 4 ms      | 0 / 12 ms   |
| **1000** | 1m       | 1,185,200 | 2 ms    | 5 ms    | 7 ms      | 0 / 30 ms   |
| **2000** | 1m       | 2,342,000 | 4 ms    | 7 ms    | 17 ms     | 0 / 45 ms   |
| **5000** | 1m       | 5,637,000 | 4 ms    | 11 ms   | **32 ms** | 0 / 61 ms   |

- **资源占用**：在 5000 并发（ 20,000 个 Node 活跃）下，堆内存分配保持在 **300MB** 左右。
- **稳定性**：得益于 **Zero-Allocation** 思路和对象池，系统几乎没有因 GC 引起的延迟抖动。

## 3. 如何使用 (Usage)

### 第一步：启动服务端

确保项目已编译并启动服务端：

### 第二步：运行压测工具

在项目根目录下通过 `go run` 启动：

```bash
go run cmd/bench/main.go -u 1000 -d 1m -b 5
```

### 可选参数说明

- `-u` (int): 并发用户数 (默认 10)
- `-d` (duration): 压测持续时间 (默认 20s)
- `-i` (duration): 每一个用户发送包的间隔 (默认 50ms)
- `-b` (int): 分批启动的桶数量 (默认 5)
- `-w` (string): 压测使用的 Workflow 名称 (默认 "benchmark")
- `-t` (string): 目标服务地址 (默认 "http://127.0.0.1:7001")
