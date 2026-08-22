# Voce 网关设计与架构 (Gateway Architecture)

> [!WARNING]
> **实验性功能 (Experimental)**：网关模式目前仍处于实验性阶段，旨在探索分布式部署能力。在当前阶段，其稳定性和生产可用性不作任何保证。

本文档描述了 Voce 网关的设计思想、核心组件以及在集群模式下的运行机制。

---

## 设计初衷 (Design Philosophy)

在传统的单机模式下，Voce 的运行时 (Engine) 直接面对客户端连接。但在生产环境下，这种架构面临以下挑战：
1.  **扩容受限**：单机资源有限，无法处理超大规模的并发会话。
2.  **状态粘连**：语音交互是有状态的，客户端断线重连必须回到原来的执行节点以恢复 Context。
3.  **Pod 波动**：在后端 Pod 重启或漂移时，需要一个层来屏蔽这些变化。

**Gateway (网关)** 层的引入，旨在将“接入层”与“执行层”剥离，实现会话持久化与水平扩展。

---

## 技术选型 (Technical Selection)

*   **网络框架 (Networking)**: 选用 [nbio](https://github.com/lesismal/nbio)。理由是其非阻塞模型在处理大量长连接时具有极低的内存占用，且支持内存池复用，非常适合语音流这种高频、小包的场景。
*   **并发控制**: 大量使用原子操作 (`sync/atomic`) 替代互斥锁 (`sync.Mutex`)。在 `Session` 状态切换、数据包转发等热点路径上，尽量减少上下文切换。
*   **内存管理**: 引入 `pkg/pool` 与 `pkg/buf`，对转发过程中的字节切片进行预分配与回收，降低 GC 压力。

---

## 核心作用 (Core Roles)

*   **流量分发 (Load Balancing)**：根据后端节点的负载情况，将新创建的会话分配到最合适的 Pod。
*   **会话路由 (Session Routing)**：维护 `SessionID -> Machine` 的映射，确保特定的会话始终路由到正确的后端。
*   **连接池复用 (Connection Pooling)**：网关与每个 Pod 建立持久的连接池，避免了为每个用户连接都建立独立底层链路的开销。
*   **状态感知 (Health Mastery)**：实时感知 Pod 的在线、挂起、下线状态；在重连窗口内保留会话，心跳超时后清理关联会话。

---

## 实现细节 (Implementation)

### 机器注册与监控 (Control Plane)
后端 Pod 启动时，会通过 WebSocket 向网关的 `/register` 接口发起连接。
*   **心跳检测**：网关通过该长连接定期发送 Ping 帧，若连续失败多次，则将该机器标记为 `Suspended`。
*   **缓冲机制**：机器处于 `Suspended` 状态时，网关会保持现有的 Session 不释放，等待 Pod 重连或超时清理。

### 数据连接池 (Data Plane)
网关与每个注册的 Machine 维护独立的动态 **Data Connection Pool**。
*   **动态扩缩容**：每个 Pool 启动时保留最小连接数。当单条连接承载的 Session 达到目标阈值时，网关会建立新的数据连接；空闲连接在超时后会被回收，但不会低于最小连接数。
*   **Session Binding**：Session 创建后会绑定到一条数据连接。后续客户端上行数据直接使用该 Binding，不会为每个 Packet 重新选择连接，保证同一方向上的时序性。
*   **双向独立路由**：Machine 接收到数据连接后，会独立管理入站连接，并为 Workflow 输出选择稳定的回程连接。上行和下行不要求使用同一条物理连接，双方通过 SessionKey 识别数据归属。
*   **内部协议封装**：
    数据流进入 Pool 时，网关会在标准二进制报文前增加 **16 字节的 SessionKey 前缀**，以便 Pod 识别数据归属。

### 会话生命周期 (Session Lifecycle)
1.  **创建**：客户端调用网关 `/sessions`，网关从 `MachineManager` 选出最空闲的机器并透传请求。
2.  **关联**：网关记录 SessionKey 与 Machine 的关系，并在该 Machine 的 Data Connection Pool 中创建 Session Binding。
3.  **转发**：网关在客户端 WebSocket 与机器数据链路之间进行双向透明转发。
4.  **暂停**：客户端 WebSocket 普通断开时，网关向 Machine 下发 Pause，保留 Session 与连接 Binding，以支持短暂断线后的恢复。
5.  **销毁**：Pod 下发 Close 包、客户端显式关闭或会话空闲超时时，网关清理 Session、Machine 归属与连接 Binding。

---

## 详细设计 (Detailed Design)

### 连接池与 Session 粘连 (Pool & Affinity)
为了保证同一个 Session 的音频流时序性，网关通过显式 Binding 实现“会话粘连”：
*   **最小负载选择**：新 Session 会分配到当前负载最低且未达到硬上限的活跃连接。Pool 维护连接负载、`SessionKey -> pooledConnection` 路由和 `SessionKey -> SessionBinding` 映射。
*   **稳定路径**：Binding 创建后，同一 Session 的客户端上行数据始终经由同一个 Connection 发送。连接扩容或回收不会迁移已有 Session。
*   **容量控制**：`pool_target_sessions_per_connection` 是触发扩容的目标水位；`pool_max_sessions_per_connection` 是单连接硬上限；`pool_max_connections` 限制单个 Machine 可建立的数据连接总数。
*   **空闲回收**：Session 全部解绑后的连接进入空闲状态。超过 `pool_idle_timeout` 后，Pool 将其关闭并回收，保留不少于 `pool_min_connections` 条连接。

> **注意**：Machine 侧的连接管理与网关 Pool 是两套独立职责。网关负责客户端数据进入 Machine 的连接 Binding；Machine 负责 Workflow 输出返回网关时的连接选择。它们不要求同一 Session 的双向数据使用同一条物理 WebSocket。

### 异常容错策略 (Fault Tolerance)
*   **指数退避 (Exponential Backoff)**：当网关与 Pod 的数据链路断开时，会自动进入重连循环，重连时间间隔从 500ms 开始指数增加（最大 10s），直至 Pod 恢复。
*   **Binding 保留**：数据连接重连期间，Session Binding 仍指向原 Connection 对象。连接恢复后，后续数据继续使用该路径；实时数据在连接不可用期间可能被丢弃，避免产生无界积压。
*   **终止优先**：Machine 回传 Close 包表示该 Session 已终止。网关会先标记 Session 为终态，再关闭客户端连接并清理本地路由，避免将终止误判为可恢复断线而回发 Pause。
*   **平滑迁移 (未来计划)**：目前 Pod 崩溃会导致正在转发的数据丢失。未来计划通过网关侧的小规模环形缓冲区，在 Pod 短暂断开时缓存关键信号。

---

## 风险 (Risks)

在决定开启网关模式前，请知悉以下潜在风险：

1.  **单点故障 (SPOF)**：
    当前网关节点本身是单机的。如果网关进程崩溃，所有的会话链路都将中断。在生产环境建议通过外部 LB (如 Nginx 或 K8s Ingress) 挂载多个网关副本（需要共享 Session 状态存储，目前版本暂未原生支持，需结合 Redis 或类似方案）。
2.  **资源开销 (Overhead)**：
    网关需要维护大量活跃的 WebSocket 连接（客户端侧与 Pod 侧）。虽然使用了 `nbio` 等高性能非阻塞网络库，但在极高并发下，句柄数 (fd) 和内存分配仍是关键瓶颈。
3.  **网络时延 (Latency Hop)**：
    网关的引入增加了一层代理转发，虽然在内网环境下该损耗极低，但对于极致低延迟要求的 ASR/TTS 场景，仍需评估额外的 RT 损耗。
4.  **配置同步 (Configuration Sync)**：
    系统已提供基于 **Redis** 的 `WorkflowConfigManager` 实现。在多 Pod 分布式部署时，**必须** 开启 Redis 存储模式以确保所有节点之间的工作流配置实时同步。默认的文件存储模式仅适用于单机调试。
5.  **缓冲区堆积**：
    当 Pod 处于 `Suspended`状态时，客户端若继续发送大量音频数据，网关会因为无法转发而丢弃数据包。
