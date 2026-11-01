# Agora RTC 接入设计

---
创建时间: 2026-10-30T00:50:59+08:00
Base commit: d3b756478b3c1fda7cb8050f321c45e05475d8c4
最后修改时间: 2026-11-01T19:25:00+08:00
创建人: ranchangxin
状态: 已实现（仅 standalone）
---

本设计取代同编号的旧稿「RTC 包重写设计与实现计划」。旧稿描述的是"整包重写 + 分阶段实现"的计划，实际落地时换了形态：接入的是 Agora，包按 `service / binding / messenger / rtm bridge` 切分，并且为了隔离 cgo 重新划了包边界。基线从 `636252a` 重新设定为本文档的 Base commit；旧稿正文已删除，不再作为参考。

## 1. 目标与边界

一个 session 通过 Agora 频道接入 Voce 语音链路：

- 客户端上行音频 → ASR，服务端下行 TTS 音频 → 客户端；
- 字幕、打断、语音事件等非音频包通过 RTM 下发；
- 只影响接入层，不改动 `engine`、`schema`、Packet 格式和 DAG 调度。

明确不做：

- 视频、屏幕共享；
- 多人频道（一个频道只有服务端和一个客户端）；
- token 自动续期；
- Gateway 代理（gateway 二进制不装配、不转发这个端点）；
- 自动重连（见 §6）。

## 2. 架构

```text
Client ──HTTP──> AgoraHandler (POST /agora/{id}) ──> Service
                                                     │  SessionKey → *Binding
                                                     │  进程级 Once：RTC 引擎 + RTM 客户端
                                                     ▼
                                          Binding ──持有──> Session lease / PacketWriter
                                             │                 ▲
                                             │                 │ ReceivePacket（入站）
                                             │                 │ Write（出站）
                                             ▼                 │
                                      Agora channel ──RTC 音频──┘
                                             │
                                             └──RTM──> messenger ──> topic "events"

SessionManager ──删除会话──> Service.onSessionDeleted ──> Binding.close
```

| 文件 | 职责 |
| --- | --- |
| `config.go` | 配置、频道名/账号派生、启动校验 |
| `credentials.go` | RTC/RTM token 签发（`rtctokenbuilder2`、`rtmtokenbuilder2`） |
| `service.go` | SessionKey → binding 映射、进程级初始化、Attach、Close、会话删除观察者 |
| `binding.go` | 单会话适配：SDK 回调、入站聚合、出站节拍、pause/resume |
| `messenger.go` | 进程级 RTM 客户端 + 每会话 stream channel/topic |
| `agorartm.go`、`rtm_bridge.c/h` | 自写的 RTM cgo 封装（见 §5） |
| `handler.go` | `POST /agora/{id}`，错误到 HTTP 状态码的映射 |
| `errors.go` | 对外错误集合 |

## 3. 生命周期与所有权

`Service` 是常驻对象，生命周期只有 `Close()`：

- `NewService(cfg, sm)` 只做配置校验和删除观察者注册，不碰 SDK；
- 首次 `Attach` 才初始化 SDK（RTC 引擎 + RTM 客户端），初始化失败保留错误；
- `Close()` 拒绝新 Attach、释放所有 binding、关闭 RTM 客户端；`bindings` 用 `clear` 清空。

`Attach` 的顺序体现了所有权：

1. 查 map：已绑定直接返回缓存的 `Credentials`（幂等）；
2. `session.Bind()` 抢 lease —— 这是唯一互斥点，抢不到返回 409；
3. 初始化 SDK、签服务端 token、建连接并 `Connect`；
4. 签客户端 token，装 `SetWriter`，`Activate` 恢复 workflow；
5. 双检（服务是否关闭、会话是否还在）后入库；否则关掉刚建好的连接。

**binding 持有 lease，关闭时先释放连接再 `Unbind`**。`close(pause)` 的 `pause` 参数区分两种场景：服务关闭时暂停仍存活的会话，会话删除时不暂停（workflow 随后会被 `Stop`）。

会话删除经 `SessionManager.AddDeletedObserver` 回到 `Service.onSessionDeleted`：按 key 查、校验 session 对象身份，然后锁外关闭 binding，不等待 workflow。

## 4. 数据路径

### 4.1 入站音频

```text
SDK OnPlaybackAudioFrameBeforeMixing
  → 校验 uid / connected / peer / closed
  → 锁内聚合到 100ms 缓冲（engine.AudioBufferSize = 3200B）
  → 锁内 SetPayload 进池化 Packet
  → 锁外 Session.ReceivePacket(lease, packet)
  → ReleasePacket
```

- 只处理 `cred.UID` 对应客户端的音频，其他用户忽略；
- SDK 按 10ms 回调，缓冲满才提交；断线或用户离开时清掉半包；
- 锁只覆盖缓冲读写，`ReceivePacket` 会阻塞在 workflow 上，必须在锁外调用；
- Packet 只在调用期间借用，`ReceivePacket` 返回后立即归还。

### 4.2 出站音频

```text
Workflow.output → Session.outLoop → Binding.Write
  → TypeAudio：写入 buf.ChunkQueue（block 32000B，上限 32000B = 1s，满了丢弃）
  → sendLoop：10ms tick 读 320B（engine.AudioFrameSize）
  → PushAudioPcmData(frame, 16000, 1, 0)
```

- 发送协程在 `OnConnected` / `OnUserJoined` 时拉起，`running` 的 CAS 保证同一时刻只有一个循环；
- 掉拍不补发，避免把积压一次性推给 SDK；
- 不足一帧补静音，没有数据则跳过该拍；
- paused 期间协程不退出，只是每拍检查 `connected && peer` 后跳过发送。

### 4.3 控制消息

```text
Binding.Write（非音频） → messenger.publish
  → base64(Packet.Marshal())
  → PublishTopicMessage("events", ...)
```

- 选 base64 是因为 `Packet.Marshal` 的 size 字段必含 `\0`，而 RTM 的消息接口按 C 字符串传递，裸二进制会踩截断；
- 中断包先 `queue.Reset()` 清掉本地待播音频，再发 RTM；
- RTM 未登录或 topic 未就绪时直接丢弃（握手期间丢控制包是正常现象，失败路径另有日志）；
- `close()` 在 RTM 通道还在时发一条 `TypeClose`，然后才释放通道。

### 4.4 所有权约定

| 数据 | 持有方式 | 释放时机 |
| --- | --- | --- |
| SDK 回调 PCM | 同步复制进 binding 缓冲 | 回调结束即不再引用 |
| 入站 Packet | protocol 池申请 | `ReceivePacket` 返回后 `ReleasePacket` |
| Session 输出 Packet | 只在 `Write` 调用内借用 | 由 `outLoop` 释放，binding 不保留 |
| 出站音频 | 复制进 `ChunkQueue` | 发送、丢弃或 `Reset` 后归还块 |
| RTM 消息 | base64 字符串 | 发布调用返回后不再引用 |

## 5. cgo 边界

SDK 的 Go RTM 封装与随包发布的 C 头文件不同步：封装里用的是 `C_RTM_CONNECTION_STATE` / `C_RTM_CONNECTION_CHANGE_REASON`，而头文件已改名为 `C_RTM_LINK_STATE` / `C_RTM_LINK_STATE_CHANGE_REASON`，导致 `import agorartm` 无法编译。上游最新 tag（v2.6.6）依然如此，未发布的分支改了回调名但 RTC 侧落后，不能整体升级。

因此 RTM 走自写的最小 cgo 封装：

- `rtm_bridge.h` 声明 7 个回调与 `new_handler`；`rtm_bridge.c` 提供 39 个空实现，保证 SDK 调用任一回调都不会踩空指针，并把 `userData` 显式写回（构造函数不保留该参数）；
- Go 侧只用 `C.malloc` 出来的 id 做注册表索引，不把 Go 指针交给 C 长期持有；
- 不改动 `libs/` 下的 SDK，安装脚本重跑不会丢失这层封装。

包边界由构建方式保证：

- `internal/rtc/agora` 是唯一带 cgo 的业务包；
- `handler` 也放在这个包内，`biz/route` 只接收 `httpx.WrapHandlerFunc` 函数值，不 import 它；
- `make build-gateway` 使用 `CGO_ENABLED=0`，任何人把 cgo 依赖带进 gateway 都会编译失败。

## 6. 配置与凭证

配置位于顶层（与 `redis` 同级，属于外部服务凭证，不是进程设置）：

```yaml
agora:
  app_id: '<32 位十六进制>'
  certificate: '<32 位十六进制>'
```

派生规则与默认值：

| 项 | 默认 | 说明 |
| --- | --- | --- |
| 频道名 | `voce-<session-key>` | UUID 唯一，重建会话必然换频道 |
| 客户端账号 | `client-<session-key>` | 服务端签发，客户端不能自选 |
| 服务端账号 | `voce-agent` | 同一频道内的服务端身份 |
| token 有效期 | 24h | RTC token、权限、RTM token 取最小值 |

`AppID`/证书必须满足 token builder 的格式要求（32 位十六进制），启动即校验，避免请求期才发现。

## 7. 断线与恢复

- `OnReconnecting` / `OnConnectionLost` / `OnDisconnected` / `OnUserLeft`：关媒体闸门、清入站半包、`Workflow.Pause()`；
- `OnConnected` / `OnReconnected` / `OnUserJoined`：开闸门、`Workflow.Resume()`；
- 连接和 lease 不释放：Agora 自己重连，客户端也可以重新进频道；
- `OnConnectionFailure` 是终止态（SDK 停止重连），这里只暂停，不主动关闭——由 session 空闲清理回收。

暂停和恢复直接操作 `Workflow`，没有经过 `Session` 的封装，因为 `Session` 目前只提供"解绑时顺便暂停"的语义，无法在保留 lease 的前提下暂停。

## 8. 测试与验证

已有自动化测试：

- `credentials_test.go`：token 签发（含无证书场景）、有效期取最小值、配置校验、频道名/账号派生。

手工验证（本机，standalone）：

```bash
curl -X POST localhost:7001/sessions -d '{"name":"agora_smoke"}'
curl -X POST localhost:7001/agora/<session_id>
```

结果：返回 200 与凭证，服务端日志出现 `rtc connected reason=1`（JOIN_SUCCESS）；重复请求返回同一份凭证。

未覆盖：真实客户端音频、断线重连、RTM 发布确认。

## 9. 已知限制

| 限制 | 影响 |
| --- | --- |
| RTM 需要在 Agora 控制台为该项目开通 | 未开通时登录返回 `-10015 LOGIN_NOT_AUTHORIZED`，控制消息全部丢弃，音频不受影响 |
| 不做 token 续期 | 会话超过 24h 后凭证失效，需要重建会话 |
| 不做自动重连 | `FAILED` 后只能等 session 空闲清理 |
| 没有指标 | 队列积压、发布失败、连接状态只能靠日志 |
| 未验证 RTM 是否复制消息内容 | 依赖"异步发布必须先拷贝"的假设；base64 已规避二进制截断风险 |
| 单进程单 RTM 客户端 | 进程内所有会话共用一个服务端账号 |

## 10. 相关约束

- [连接层抽象设计](../02-connection-layer/design.md)：Session lease 与唯一 outLoop 的来源
- [Workflow 生命周期与背压](../../guides/workflow.md)
- [插件数据所有权](../../guides/plugin.md)
- [接入协议](../../reference/protocol.md)
- [Session lease / writer 接口](../../../internal/engine/binding.go)
- [RTM 回调结构](../../../internal/rtc/agora/rtm_bridge.h)
