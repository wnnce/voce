# 连接层抽象设计

---
创建时间: 未知
Base commit: 85c80420b671e1ed523fd128cb0d0c56962253d7
最后修改时间: 2026-10-17T15:53:52Z
创建人: ranchangxin
状态: 部分实现，Session lease/writer 与 WebSocket、gRPC、Gateway 已迁移，RTC 待接入
---

> 本次重写把基线更新为当前 HEAD。上一版基线（`255d392`）描述的 `BaseConnection` 实现从未提交，仓库里不存在，因此本设计按「从零引入」来写。

当前实现已落地 Session lease/writer、统一 ReceivePacket、唯一 outLoop，以及 WebSocket、gRPC、Gateway 三条协议接入的迁移。RTC 尚未接入。

## 1. 结论

连接层由各协议实现自己承担。跨协议真正一致的只有三件事——连接占用、输入语义分派、输出消费——全部收在 Session 上。

推荐的边界是：

~~~text
WebSocketConnection / GRPCConnection / RTCConnection
    ├── 协议握手、解帧、编码
    ├── 中断自己的底层 IO
    └── 调用 Session.ReceivePacket、提供 PacketWriter

Session
    ├── lease 与连接切换
    ├── ReceivePacket 统一输入语义
    ├── Workflow.output 的唯一消费者
    ├── 当前 PacketWriter
    └── Pause / Resume / Stop 生命周期

Workflow
    └── DAG 调度、Schema 路由和出站 Packet 队列
~~~

边界依据：一段逻辑只在某一个协议里有意义（gws 的 Writev、protobuf 的 Send、池连接的 SessionKey 帧头），就留在那个协议里；只有各协议语义完全一致的部分（输入分派、输出消费、lease 仲裁）才进 Session。

## 2. 角色与职责

| 角色 | 职责 |
| --- | --- |
| Session | 保存会话和 Workflow 状态，仲裁连接 lease，接收统一输入，消费 Workflow 输出 |
| 协议连接实现 | WebSocket/gRPC/RTC 各自握手、解帧、编码、关闭和中断底层 IO |
| PacketWriter | 一个很小的写出能力，将 Packet 写到具体连接 |
| Workflow | 产生 Packet、执行 DAG、负责运行时背压和 Packet 入队 |

连接实现不持有 Session。连接入口创建具体连接对象，并把它和 Session 的 lease、ReceivePacket 回调、PacketWriter 装配起来。

## 3. 数据路径

### 3.1 入站

```text
客户端字节
  → 协议连接实现解帧
        WebSocket : gws binary 帧 → Packet.Unmarshal
        gRPC      : pb.Packet 字段
        Gateway   : 池内连接帧头(session key) + Packet
  → Session.ReceivePacket 的 lease 准入检查
  → Session.ReceivePacket(*protocol.Packet) 唯一语义分派点
        TypeAudio → 池化 Audio → Workflow.SendToHead
        TypeClose → SessionManager.RemoveSession
        TypePause → Workflow.Pause（仅 Gateway 模式接受）
        TypeResume → Workflow.Resume（同上）
  → Session 完成本次输入语义处理
```

关键点：`protocol.Type` 的判断**只在 Session 里写一次**。现在 WS、gRPC、池内通道各写一遍，已经漂移（WS 没实现 Pause/Resume，池内通道实现了）。

### 3.2 出站

~~~text
Plugin.Publish / Flow.Write
    → Workflow.Write
    → Workflow.output
    → Session.outLoop
    → 当前 PacketWriter
    → Release Packet
~~~

Session 是唯一的 Workflow.output consumer。WebSocket、gRPC、Gateway 和 RTC 都不再各自启动一个 output loop。

如果所谓 Session.Write 是“把 Packet 写到当前连接”，它内部仍然必须调用当前 PacketWriter；Session 不可能凭空完成 WebSocket Writev、gRPC Send 或 RTC PCM 推流。因此建议把 Session 的输出能力设计为 Bind/SetWriter/Unbind 管理的 sessionBinding，而不是让插件直接调用网络写入。

**当前实现**：`Unbind` 取消 ctx 并等 outLoop 退出后，`Workflow.output` 中尚未取走的 Packet 会留在 channel 里，下一条连接的 outLoop 会把它们发出去。也就是说现在是「续播」语义，不是「丢弃」。

这依赖两个前提：Pause 之后不再产生新输出，残留量级只有断开瞬间的尾巴；以及 `Workflow.output` 只有 1024 容量，长期断线会丢新包。

**建议策略**（尚未实现）：过期媒体按类型丢弃，或通过 output epoch 标记 stale；是否保留控制消息单独定义；重连后只发送新一代输出。如果产品需要可靠续播，应增加专门的可恢复事件队列，不能把 `Workflow.output` 当作持久缓存。

### 3.3 包的所有权

| 阶段 | 谁持有 | 谁释放 |
| --- | --- | --- |
| 解帧后 → `Session.ReceivePacket` | 协议连接实现 | 协议连接实现（`defer ReleasePacket`） |
| `Workflow.Write` 之后 | Workflow | outLoop |
| `writer.WritePacket` 期间 | 仍归 outLoop | outLoop（写完立即释放） |

没有「积压队列」这种第二套所有权：`sessionBinding` 不接管包，`outLoop` 是唯一的出站释放点。留在 `Workflow.output` 里没人取走的包，由 `Workflow.Stop` 统一回收。

`WritePacket` 返回后不得再引用 payload。需要异步发送的连接层（RTC）必须自己复制。这条约定让「写失败要不要释放」这类问题只有一个答案：**永远由 outLoop 释放**。

## 4. Session lease 与输入入口

Session 直接提供连接 lease 和统一输入入口：

~~~go
type Lease uint64

type PacketWriter func(*protocol.Packet) error

func (s *Session) Bind() (Lease, error)
func (s *Session) SetWriter(lease Lease, writer PacketWriter) bool
func (s *Session) Activate(lease Lease) error
func (s *Session) ReceivePacket(lease Lease, packet *protocol.Packet) error
func (s *Session) Unbind(lease Lease, pause bool) bool
~~~

Session 直接接收解帧后的 `*protocol.Packet`，不再额外创建 `Input/InputKind`。连接实现负责解码，Session 统一分派 `packet.Type`：

- Audio：创建池化 Audio Schema，复制 Packet payload，送入 Workflow head，失败时 Release；
- Close：删除 Session；
- Pause/Resume：按接入模式检查并修改 Workflow 状态；
- 所有输入更新活动时间并校验 lease。

连接层在调用结束后负责 ReleasePacket；Session 只在同步调用期间借用 Packet，不保存 Packet 或 payload。未来 RTC 如果直接拥有 `schema.Audio`，再增加专用媒体入口，不为了当前协议抽象出第二套 Input 类型。

这解决了当前 WebSocket、gRPC、Machine pool 各自 switch PacketType 的漂移，也让 RTC 可以把 PCM 回调转换成同一种 Audio 输入。

`Activate` 只做一件事：如果 Workflow 处于 Paused，就 Resume。它单独存在是因为 `SetWriter` 要在 Upgrade 之后才能调用，而 Resume 必须在连接确认可用之后。

`Unbind` 的 `pause` 参数区分两种断开：连接正常断开传 `true`（保留会话，Pause 之后可重连），握手失败回滚传 `false`（不暂停，因为这条连接从未服务过）。

## 5. 连接关闭顺序

协议连接对象自己负责中断底层 IO；Session 负责 lease 和 Workflow 生命周期：

~~~text
协议实现中断 Read/Write
    → Session 拒绝新的 ReceivePacket
    → Session 摘掉 PacketWriter，取消并等待 outLoop 退出
    → 当前 lease 仍有效时 Pause Workflow
    → 释放 lease
    → 允许下一条连接 Attach
~~~

只有当前 lease 可以 ReceivePacket、SetWriter 和 Unbind。被拒绝的连接没有 lease，因此不能 Pause 或释放别人的 Session。

不能在释放 lease 后让旧 outLoop 继续运行：旧 loop 仍可能从 Workflow.output 取走新连接的数据。实现上是两者都做：`Unbind` 先把 owner 的 CAS 目标设为哨兵值 `stoppingLease`，此时 `Bind` 必然失败；再取消 ctx、等 `outDone` 关闭；最后才把 owner 归零为空闲。这样「旧 loop 已退出」和「新连接可以接入」之间没有可乘之机。

`done` 可能为 nil——`Bind` 成功但 `SetWriter` 从未调用时（例如 WebSocket Upgrade 失败）就没有 loop 可等。这种情况必须跳过等待，否则 `<-nil` 会永久阻塞。

各协议的中断方式本来就不同：WebSocket 关 socket、gRPC 依赖 RPC context 取消与 handler 返回、Gateway 物理连接不能为单个 Session 关闭、RTC 调 SDK Leave/Release。这些差异留在协议实现里，Session 只管 lease 和 Workflow 生命周期。

## 6. 协议实现

### 6.1 WebSocket

~~~text
Connect
    → LoadSession
    → Session.Bind() 预留 lease
    → Upgrade
    → Session.SetWriter(lease, withPacketWrite(socket))
    → ReadLoop 解帧
    → Session.ReceivePacket
~~~

WebSocket 只做 gws binary frame、Packet.Unmarshal、Writev、Ping/Pong 和 socket 关闭。它不直接调用 Workflow.SendToHead、Pause 或 Resume。

~~~go
// Open 阶段装配一次，之后每包只执行一次函数调用。
func (s *SocketHandler) withPacketWrite(socket *gws.Conn) engine.PacketWriter {
    return func(packet *protocol.Packet) error {
        header := packet.Header()
        payload := packet.Payload
        realtimeMetrics.websocketBytesSent.Add(realtimeMetricContext, int64(len(header)+len(payload)))
        return socket.Writev(gws.OpcodeBinary, header, payload)
    }
}
~~~

`Writev` 失败时 gws 内部会关闭底层 TCP，`ReadLoop` 随即报错退出并触发 `OnClose`，由 `OnClose` 调 `Unbind` 收尾。所以 writer 不需要自己处理关闭，也不需要返回「连接已死」这种语义。

### 6.2 gRPC

gRPC 解析 metadata，创建 stream writer，Bind Session，然后 Recv protobuf 并转换成 Packet。输出由 PacketWriter 闭包完成。

gRPC 的 Recv 退出必须单独验证：取消一个派生 context 不一定解除阻塞的 Recv。Session 删除、Send 失败或 stream context 取消必须能让 handler 返回并释放 lease，不能通过提前解绑遗留 reader goroutine。

### 6.3 Gateway

Machine 的物理连接池继续由 ConnectionManager 管理。会话创建时装配一个闭包 writer，把输出交给池里的连接：

~~~go
func (h *SessionHandler) withPacketWriter(ctx context.Context, key protocol.SessionKey) engine.PacketWriter {
    var conn *machine.Connection // 闭包变量，只有 outLoop 一个 goroutine 访问
    return func(packet *protocol.Packet) error {
        if conn == nil || conn.State() != protocol.ConnectionActive {
            conn = h.cm.Select(key)
        }
        if conn == nil {
            return nil // 池里暂时没有连接，丢这一帧，下个包重试
        }
        return conn.Write(key, packet)
    }
}
~~~

三个约定：

- **不需要锁**。`conn` 只被 outLoop 串行访问，加锁只会制造「这里需要并发保护」的错觉。
- **不缓存 ctx 之外的任何池状态**。`conn` 失效时重新 `Select`，ConnectionManager 内部会剔除不活跃连接。
- **不提供 Close**。writer 只负责写，Session route 的释放由已有的 `ConnectionManager.onSessionDelete` observer 完成——它在会话删除时写 `TypeClose` 包并 `Release(key)`。物理连接绝不能被单个会话关闭。

`biz/handler/session.go` 的固定 `sessionWriteLoop` 已删除，由 Session 的唯一 outLoop 替代。Gateway 入站解帧后按 SessionKey 找到 Session，再调用 `Session.ReceivePacket`；Session 不需要知道池和物理连接。

### 6.4 RTC

RTC 自己管理 PCM 缓存、分帧、固定推流时钟和 SDK 资源。它通过 Session.Bind/ReceivePacket/Unbind 接入。RTC PacketWriter 在 WritePacket 返回前必须复制异步队列需要的字节，因为 Session 随后会 Release Packet。

## 7. 修改范围

| 范围 | 修改 |
| --- | --- |
| internal/engine/binding.go（新增） | `Lease`、`PacketWriter`、`sessionBinding`、`Bind`/`SetWriter`/`Activate`/`ReceivePacket`/`Unbind`、唯一 `outLoop` |
| internal/engine/session.go | 删除 `busy`，增加 `closeWriter`；`Stop`/`Cleanup`/`RemoveSession` 先停输出再停 Workflow |
| biz/realtime/socket.go | 删除本地 output loop 与 Workflow 直接操作，只保留解帧、writer、socket 生命周期 |
| biz/realtime/grpc.go | 删除 writeLoop/readLoop/wg，输入/输出全部通过 Session |
| biz/realtime/handler.go | Upgrade 前 `Bind`，忙返回 409；Upgrade 失败 `Unbind(lease, false)` |
| biz/handler/session.go | 删除固定 sessionWriteLoop，改用池路由闭包 writer |
| 测试与 benchmark | `binding_test.go`：lease 归属、入站 Audio 分派、唯一消费者；`grpc_bench_test.go`：writer 热路径 |

`internal/machine` 不参与这次改动：路由释放留在已有的 `onSessionDelete` observer 里，物理连接池和入站解帧都不变。

本次新增的公共 API 只有 `internal/engine/binding.go` 里的 lease 和 PacketWriter，不引入新的公共连接类型或全局注册表。

## 8. 性能与测试

热路径要求：

- WebSocket 继续 Header + Writev，不调用 Packet.Marshal；
- writer 在装配时构造一次，每包只执行一次函数调用，不新建 closure、context 或适配对象；
- Packet 和 payload 只在 ReceivePacket 调用期间借用，Audio Schema 继续使用对象池；
- Session outLoop 是唯一 Packet Release 点；
- Gateway 闭包 writer 缓存 Session route，不在每包重新 `Select`；
- RTC 只复制异步队列真正需要的数据。

必须更新 Session `ReceivePacket`、`outLoop` writer 和 gRPC writer 的 benchmark，报告 ns/op、B/op、allocs/op。功能测试覆盖：

- 并发 Bind：一个 lease 成功，其他连接不能影响当前 Session；
- Upgrade 失败：lease 回滚，不 Pause；
- 旧 lease Unbind：不能释放新连接；
- Audio/Close/Pause/Resume：统一输入语义和所有权；
- 唯一 outLoop：同一 Session 不能有两个 output consumer；
- writer 成功/失败：Packet 都由 Session 释放；
- writer 返回错误：outLoop 只记日志并继续，不退出（退出由 Unbind 决定）；
- 断线重连：旧 loop 退出后新 lease 接管，过期媒体按策略处理；
- Session 删除/超时：拒绝新 Bind 并回收输出；
- Gateway writer：无可用连接时丢包不报错；只释放 Session route，不关闭共享物理连接；
- gRPC 阻塞 Recv：handler 能返回，无永久 goroutine。

## 9. 对外行为变化

| 变化 | 说明 |
| --- | --- |
| WebSocket 忙时返回 HTTP 409 | 原来是 Upgrade 成功后 Close 1008。需要在 Upgrade 之前 `Bind`，因此 `docs/reference/protocol.md` 和 web / TUI 客户端都要支持 409 |
| `TypePause` / `TypeResume` 对 WebSocket 生效 | 语义统一后，WS 客户端发送这两个包会真正暂停/恢复 Workflow；以前被忽略 |
| 会话删除不再依赖 `TypeClose` 包通知 Gateway | 由 `onSessionDelete` observer 写 Close 包并释放 route，与媒体队列解耦 |
| 断线期间的残留输出会续播 | 见 §3.2，当前实现是保留到下次连接 |
| 输出逻辑失败不再终止会话 | writer 单次失败只记日志，避免一次临时写错误让会话永久静音 |

## 10. 最终建议

最终形态是「Session binding + 协议连接适配器」：

~~~text
协议连接
    → 解码
    → Session.ReceivePacket

Workflow.output
    → Session.outLoop
    → PacketWriter
    → 协议编码/发送
~~~

保留的最小抽象是 Session 的 lease、ReceivePacket、PacketWriter，以及每个协议自己的连接实现。

不要把 Session.Write 设计成插件直接写网络的方法；Session 应该控制唯一出站消费者，Workflow 仍然负责产生 Packet。这样能统一输入和输出生命周期，同时保持 engine 的调度、背压和 Packet 所有权边界。
