# Web 对话页设计与实现计划

---
创建时间: 2026-11-01T14:45:01+08:00
Base commit: 636252a19ac589c4cf0b002eeced49d01bc87fe6
最后修改时间: 2026-11-01T22:50:00+08:00
创建人: ranchangxin
状态: 已全量实现并深度集成入 Studio 工作台 (Voice Playground)
---

实施记录（2026-11-01，第三轮）：架构演进与 Studio 融合落地。为了消除原有独立页面切换割裂、无法边看 DAG 编排边测试语音交互的痛点，将对话链路全量封装并升级为 **Voce Studio** 内置的 **Voice Playground**（右侧固定可调宽 Dock 抽屉面板）。新增功能与优化包括：
1. **参数覆盖 (Properties Overrides) 编辑器**：在 Playground 控制区集成 CodeMirror 6 JSON 编辑器与校验器，支持一键根据当前拓扑提取所有节点生成参数覆盖模板，在发起通话时传递给 `POST /api/v1/sessions` 完成运行时深度合并（deep-merge）。
2. **专属表面视觉层级**：引入语义化 `--bg-chat` 与 `--bg-chat-bubble` 变量，彻底解决此前会话流区域在深色模式下与纯黑 DAG 画布（`#09090b`）融为一体的对比度问题。
3. **架构复用**：保留 `/chat` 独立路由向后兼容；底层 Web Audio DSP（单上下文双 Worklet）、AEC 校验与降级、二进制封包编解码、`sessionApi` 及状态机在 Studio 与独立页中 100% 共享。

实施记录（2026-11-01，第二轮）：连接层改为注册制抽象，`RealtimeConnection` 只暴露 `open/publish/close`，入站通过 `handlers.onMessage` 回调，页面与控制器不引用任何具体传输。`websocket` 与 `demo` 是注册表里的两个 provider，UI 的连接选项由注册表生成。音频链路接入：`audio/engine.ts` 单 AudioContext + 两个 worklet，`audio/aec.ts` 负责约束校验与半双工降级。曾加过播放探测音的自检，因每次连接都会出声且引入过一次缓冲灌满的爆音，已整体删除，模式改为由 `getSettings()` 的真实值推断。

实施记录（2026-11-01，第一轮）：P0 已落地。新增路由 `react-router-dom` 与 `/chat` 页面，编辑页内容平移入 `pages/Editor/EditorPage.tsx`，两页按需加载（ChatPage 38KB，编辑器 667KB，reactflow 不进入对话包）。已实现：`realtime/packet.ts` 封包编解码、`realtime/session.ts` 会话 REST、`realtime/ws.ts` 二进制客户端、`realtime/demo.ts` 脚本包发生成器、三栏 UI、包日志、语音时间轴、延迟指标、视觉 token。音频采集/播放与 AEC 三层方案（§6）尚未开始。

实施中发现并修正了设计中的一个错误：浏览器无法发送 WebSocket 控制帧，因此保活不能用 Ping，改为每 20 秒 `POST /sessions/renew/{id}`（§5.5）。

`clients/web` 目前只有工作流管理和插件浏览功能，没有任何实时能力。本设计新增一个**独立的对话页**，用于在浏览器里对一个工作流发起语音对话，并处理这条链路上三个绕不开的问题：传输层的现实可用性、回声消除、以及会话生命周期与后端约束的对齐。

本设计依据当前 HEAD 的代码与本次讨论结论制定。工作区存在未提交的 RTC 重写内容（`internal/rtc/`、`biz/handler/rtc.go`、`pkg/buf/` 等），与本设计无关；文档中引用的后端行为均以 HEAD 已提交代码为准。

---

## 1. 结论

四条决策：

1. **对话页是独立路由 `/chat`，不复用编辑器布局。** 现有 `App.tsx` 把 `MainLayout + WorkflowPage` 硬编码成唯一视图，需要先引入路由壳。
2. **传输层只做 WebSocket，不接入 gRPC，也不提供传输切换项。** 浏览器 + grpc-web 在协议层不支持双向流，Gateway 也不代理 gRPC（§3）；既然只有一个可用传输，就不为它预留接口或置灰选项，避免出现只为分层好看的抽象（AGENTS.md §4.3）。
3. **AEC 是功能正确性需求，不是音质优化。** 回声会被服务端 `ten_vad` / ASR 当作真实用户输入，直接破坏打断语义。方案分两层：平台 AEC（主路径）→ 半双工降级，模式由 `getSettings()` 的真实值决定。
4. **打断由服务端单向驱动，页面不提供打断按钮。** 上行协议只接受音频与关闭（§5.3），前端做不出真正的服务端打断；因此页面不设打断控件，只在收到 `0x11` 时做出反应（清播放队列、截断气泡）。

另有一个**后端缺陷会成为本设计的前置依赖**，见 §13.1：`Session.LastActive` 创建时写秒、清理时比毫秒，会让"只连不发"的会话在 30 秒内被回收。

---

## 2. 目标与边界

### 2.1 目标

- 在浏览器里对一个已有工作流完成一轮完整的语音对话：建会话 → 建连接 → 上行麦克风 → 下行音频播放 → 字幕显示 → 挂断。
- 传输层可替换，且替换不影响对话 UI 与音频链路。
- 外放场景下不自激、不自打断。
- 把实时链路的延迟与丢包数据直接呈现在页面上，减少排查时翻 Prometheus 的次数。

### 2.2 非目标

- 不做多会话并行或多标签页同会话（后端 `session_id` 独占，第二连接拿 409/1008 是预期行为）。
- 不做移动端适配以外的响应式断点优化；桌面优先。
- 不实现服务端侧的入站打断协议，也不提供客户端打断控件，见 §5.3 与 §13.4。
- 不改 `internal/engine`、`internal/schema`、`internal/protocol`。本文列出的后端问题作为独立任务处理。
- 不引入服务端语音编解码。上行保持 PCM16 裸流，Opus 作为后续评估项（§13.3）。

---

## 3. 传输层可行性

### 3.1 事实核查

| 事实 | 依据 |
| --- | --- |
| 浏览器无法使用原生 gRPC | 浏览器不暴露 HTTP/2 frame 与 trailer |
| grpc-web 不支持客户端流与双向流，只支持 unary 与 server-streaming（且 server-streaming 仅限 `grpcwebtext`） | grpc-web 官方 README「Streaming Support」 |
| grpc-web 没有在 WebSocket 上做 full-duplex 的计划，full-duplex 要等 WebTransport | grpc-web `doc/streaming-roadmap.md` |
| 现有 `VoceService.RealtimeStream(stream Packet) returns (stream Packet)` 正是双向流 | `api/voce/v1/voce.proto` |
| Gateway 不代理 gRPC，实时流必须直连 Machine | `docs/reference/protocol.md` §3；`internal/gateway` 无任何 gRPC 代码 |

结论：**"前端支持 gRPC 连接层"在浏览器里不存在直连实现**，只能通过服务端桥接或改变契约形态。

### 3.2 四个选项

| 方案 | 做法 | 代价 | 结论 |
| --- | --- | --- | --- |
| **A. 浏览器仅 WS** | gRPC 保留给 TUI / SDK / 服务端客户端 | 前端只有一种传输 | **采用** |
| B. 服务端 WS↔gRPC 桥 | 新增 upgrade 端点，内部转 gRPC 流 | 多一跳、多一个需要保活与容量管理的组件 | 不采用 |
| C. 拆分 gRPC 契约 | 下行改 server-streaming，上行另开 unary | 破坏三语言 SDK 契约，违反渐进兼容原则 | 不采用 |
| D. 桌面壳 | Tauri / Electron 内跑原生 gRPC | 放弃浏览器前提；Electron 下 AEC 另有问题（§6.2） | 不采用 |

### 3.3 本设计的取舍

采用 **A**，并且**不提供传输切换项**。

~~~text
浏览器前端  ── WebSocket ──▶  /realtime/{id}
非浏览器客户端（TUI / SDK）── gRPC ──▶  RealtimeStream
~~~

不引入 gRPC，但**引入连接层抽象**，因为 `websocket` 与 `demo` 已经是两个实现，抽象由重复驱动而不是提前设计：

~~~ts
interface RealtimeConnection {
  readonly kind: ConnectionKind;
  open(target: ConnectionTarget, handlers: ConnectionHandlers): Promise<void>;
  publish(packet: Packet): void;   // 页面 → 连接
  close(): void;
}

interface ConnectionHandlers {
  onMessage: (packet: Packet) => void;              // 连接 → 页面
  onState: (state: ConnectionState, detail?: string) => void;
  onMalformed?: () => void;
}
~~~

传输通过 `registerConnection(provider)` 注册，UI 的连接选项由注册表生成。新增传输（服务端桥接、WebTransport，或以后真的接 gRPC）只改 provider 与注册表，控制器、store、页面均不动。

---

## 4. 页面架构

### 4.1 路由与页面隔离

当前 `clients/web` 没有路由依赖，`src/App.tsx` 直接渲染唯一视图。改造：

~~~text
src/main.tsx
  └── <BrowserRouter>
        ├── /            → pages/Editor/EditorPage.tsx   （现有 App.tsx 内容）
        └── /chat        → pages/Chat/ChatPage.tsx       （本次新增）
             ?workflow=<id>  预选工作流
~~~

- 新增依赖：`react-router-dom`（唯一硬性新增依赖，其余复用 MUI 7 / zustand / codemirror）。
- 对话页**不加载** `WorkflowPage`、`reactflow`、`@rjsf`，避免把编辑器主包体积带进对话路径。
- 入口：`components/Layout/WorkflowList.tsx` 的行内菜单（已有 `Copy` / `Trash2` / `MoreVertical` 结构）加一项「发起对话」，跳转 `/chat?workflow=<id>`。

### 4.2 布局与信息层级

桌面优先的三栏。按 HIG 的视觉层级要求（`layout.md` › Visual hierarchy：内容按相对重要性排序，最重要的靠近顶部与起始侧），把**对话本身**放在视觉中心，配置放左，诊断放右。

~~~text
┌─────────────────────────────────────────────────────────────────────┐
│ Voce Conversation      ● Active 00:32     sid 8f3a…      [挂断]     │
├────────────┬──────────────────────────────────┬─────────────────────┤
│ 会话配置   │              对话流              │  诊断               │
│ 280px      │              弹性                │  320px              │
│            │                                  │                     │
│ Workflow ▾ │   ┌────────────────────────┐     │ 包日志              │
│            │   │ 今天天气怎么样         │     │ 0x13 user_speech    │
│ 覆盖参数   │   └────────────────────────┘     │ 0x14 user_end       │
│ （JSON）   │        ┌──────────────────────┐  │ 0x12 caption ×7     │
│            │        │ 今天北京晴，气温 12… │  │ 0x01 audio 320B ×240│
│ 输入 ▾     │        └──────────────────────┘  │ 0x16 agent_end      │
│ 输出 ▾     │                                  │                     │
│            │   ▁▃▅█▅▃▁  助手说话中            │ 延迟                │
│            │                                  │ 末字→首字   412 ms  │
│            │ ┌────────────────────────────┐   │ 首字→首帧   188 ms  │
│ AEC ●正常  │ │    [开始对话]     [挂断]   │   │ 播放水位     42 ms  │
│ 残余 +2dB  │ └────────────────────────────┘   │                     │
│            │                                  │ AEC                 │
│            │ ══ 实时语音时间轴 ══════════════ │ EC ✓  NS ✓  AGC ✓   │
│            │ ▁▁███▁▁▁▁█████▁▁▁▁▁██▁▁▁▁▁███▁   │ 模式       平台 AEC │
└────────────┴──────────────────────────────────┴─────────────────────┘
~~~

**没有传输选择项，也没有打断按钮。** 传输固定为 WebSocket（§3.3）；打断由服务端驱动，页面上只呈现结果（§5.3）。

**没有“按住说话”。** 轮次判断由服务端 `ten_vad` / ASR 完成，连接建立后麦克风持续上行。客户端再强制一个“发言期”（push-to-talk）会与服务端的 VAD 判定相互矛盾：同一个语音段会被两端各判一次。

**签名元素：实时语音时间轴。** 底部一条横轴，把 user / agent 的 speech 区间、caption 到达点、打断标记、以及三段延迟（语音末 → 首字 → 首帧）画在同一条时间线上。选它作为整个页面唯一"花力气"的元素，理由是：这是语音运行时工具的独有信息结构，普通聊天界面里不存在，且它把项目最关心的实时指标变成了可读的形状。其余部分保持安静。

### 4.3 页面状态机

~~~text
idle
 └─ 点击开始 → creating       POST /sessions
      └─ 拿到 session_id → connecting     WebSocket /realtime/{id}
           └─ 连接建立 → active
                ├─ 连接断开 → reconnecting ─┬─ 成功 → active
                │                            └─ 超时/失败 → error
                └─ 点击挂断 → closing       DELETE /sessions/{id}
                     └─ closed
~~~

`paused` 不在前端状态机里：后端在连接断开时 `Unbind(lease, true)` 已经 pause 了工作流，重连同一 `session_id` 时 `OnOpen` → `Activate` 自动恢复（`internal/engine/binding.go:84`）。**前端不需要、也不应该主动发 Pause/Resume 包**。

### 4.4 视觉系统

延续 `src/theme/tokens.ts` 的 Apple 系统色取向（`--primary-color` 已是 System Blue），但补齐该文件缺少的语义层与对比度校验。

#### 颜色

`color.md` › Best practices 要求颜色在浅色、深色、提高对比度三种上下文中都可用，且不能用同一颜色表达不同含义。下表给每个角色一个值，并给出实测对比度（WCAG，按 HIG `accessibility.md` › Vision 的阈值：≤17 pt 需 4.5:1，≥18 pt 或 bold 需 3:1）。

| 角色 | 浅色 | 深色 | 用在哪里 | 对比度 |
| --- | --- | --- | --- | --- |
| canvas | `#f2f2f7` | `#000000` | 页面底 | — |
| surface | `#ffffff` | `#1c1c1e` | 面板、卡片 | — |
| surface-raised | `#ffffff` | `#2c2c2e` | 气泡、时间轴槽位 | — |
| separator | `rgba(0,0,0,.08)` | `rgba(255,255,255,.15)` | 分隔线 | — |
| label | `#1d1d1f` | `#ffffff` | 正文、标题 | 16.83 / 17.01 |
| label-secondary | `#6c6c70` | `#8e8e93` | 次要信息、单位 | 5.23 / 5.22 |
| label-tertiary | `#8e8e93` | `#6c6c70` | 仅装饰性文字，不做信息载体 | 3.62 / 3.13 |
| accent | `#0b62c4` | `#0a84ff` | 链接、强调文字 | 5.90 / 4.66 |
| accent-fill | `#007aff` | `#0a84ff` | 主按钮底色（成品字白，见下） | 白字 4.02 / 3.65 |
| signal-ok | `#248a3d` | `#30d158` | AEC 正常、连接正常 | 4.40 / 8.42 |
| signal-warn | `#b25000` | `#ff9f0a` | AEC 降级、重连中 | 5.20 / 8.28 |
| signal-danger | `#d70015` | `#ff453a` | 错误、会话被顶掉 | 5.38 / 4.99 |

三条由此推导出的硬约束：

- **`#86868b` 不能再用于承载信息的文字**。现有 token 在 `#ffffff` 上只有 3.62:1，在 `#f2f2f7` 上 3.25:1，低于 4.5:1。承载信息的次要文字改用 `#6c6c70`（5.23:1）；`#86868b` 降级为纯装饰（分组标题、占位说明）。
- **强调色作为文字时用 `#0b62c4`，不要用 `#007aff`**（后者 4.02:1，小字不达标）。
- **信号色永远不做文字颜色**。`#34c759` 在白底上是 2.22:1，`#ff9500` 是 2.20:1。信号色只用于填充的点/条，文字用 `label` 色并配图标——同时满足 `accessibility.md` › Vision「不能只用颜色传达信息」。

主按钮（开始对话 / 挂断）用 `accent-fill` 底 + 白色 semibold 15px 文字：白字在 `#007aff` 上 4.02:1，满足 bold ≥3:1；13px regular 白字在同一底色上不达标，禁止使用。

#### 字体

`typography.md` › Ensuring legibility：桌面默认 13 pt、最小 10 pt；`conveying hierarchy` 要求尽量减少字体数量。

| 用途 | 字号 / 字重 / 行高 | 字体 |
| --- | --- | --- |
| 页面标题 | 17 / 600 / 1.3 | system-ui |
| 对话正文 | 15 / 400 / 1.55 | system-ui |
| 面板标题 | 13 / 600 / 1.4 | system-ui |
| 次要信息 | 13 / 400 / 1.45 | system-ui |
| 数据与包日志 | 12 / 400 / 1.5 | `ui-monospace, SFMono-Regular, Menlo` |
| 分组眉标 | 11 / 600 / 1.2，letter-spacing .04em，大写 | system-ui |

12px 是数据区的下限；正文区不低于 13px。不使用 Light / Thin 字重（`typography.md` › Ensuring legibility：「avoid light font weights」）。

#### 间距、圆角、控件尺寸

- 4px 基准栅格：4 / 8 / 12 / 16 / 24 / 32。现有 `--spacing-unit: 8px` 保留为 2 格。
- 圆角：气泡 18px（贴对方的一角收到 6px）、面板 12px、控件 10px、时间轴槽 4px。
- 控件尺寸按 `accessibility.md` › Mobility：桌面默认 28×28、最小 20×20。落地值：图标按钮 28×28（命中区扩张到 36）、常规按钮 32 高、主操作 Dock 按钮 44 高。相邻控件的间距不小于 8px。

#### 深色模式

两套完整实现，语义色各带浅/深变体。现有 `hooks/useThemeControl.ts` + AppBar 的明暗切换按钮保留（属于编辑器页面既有决策），但对话页必须跟随同一开关且两套都测过。按 `dark-mode.md` › Best practices，应用内独立的外观设置本身不推荐；是否收敛为「跟随系统」已在 §13.5 定为不做，维持现状。

### 4.5 动效

按 `motion.md` › Best practices（动效要有目的、可关闭、不阻塞）与 `Providing feedback`（高频交互不加动效）：

| 场景 | 处理 |
| --- | --- |
| 新气泡插入 | 120ms 淡入 + 4px 上移，`prefers-reduced-motion` 时只保留淡入 |
| 助手说话电平 | 由音频驱动，非装饰动画；无可动效时退化为静态电平块 |
| 收到 Interrupter | **立即截断，无动画**。可取消的动效不应阻塞交互 |
| 字幕增量更新 | 不做打字机动画。`caption` 是全量覆盖，本身已有更新感知，高频动画反而干扰 |
| 状态切换（connecting → active） | AppBar 上的状态点 200ms 变色，无位移动画 |

### 4.6 无障碍

- 对话流容器 `role="log"`，`aria-live="polite"` **只挂在已完成（`is_final: true`）的消息上**。中间态字幕 `aria-live="off"`——否则屏幕阅读器会被 ASR 增量更新刷屏，这属于 `accessibility.md` › Cognitive「减少打断」的直接要求。
- 全部图标按钮带 `aria-label`；信号状态（AEC、连接态）文字 + 图标 + 颜色三重表达。
- 键盘：`Tab` 顺序 = 配置 → 对话 → Dock → 诊断；`Esc` 触发挂断确认；`Enter` / `Space` 只作用于当前焦点控件，不占用全局空格键（避免与屏幕阅读器、滚动等默认行为冲突）。
- 焦点可见：MUI 默认 focus ring 保留，不自定义 outline。
- 尊重 `prefers-reduced-motion` 与 `prefers-contrast: more`（后者切换 label-secondary 到 label）。

---

## 5. 数据契约

### 5.1 封包

对齐 `internal/protocol/packet.go`：

~~~text
偏移 0  'V' 0x56
偏移 1  'C' 0x43
偏移 2  Type    (uint8)
偏移 3  Encode  (uint8)  0x00 raw / 0x01 json
偏移 4  Size    (uint32, BigEndian)
偏移 8  Payload
~~~

`ws.binaryType = 'arraybuffer'`；接收侧先用 `UnmarshalHeader` 的语义校验（magic、size == len - 8）再切 Payload，不合法直接丢弃并计数，不抛异常中断连接。

### 5.2 出站类型（服务端 → 前端）

来源 `internal/plugins/sink/plugin.go`：

| Type | Encode | Payload | 前端处理 |
| --- | --- | --- | --- |
| `0x01` Audio | Raw | PCM16 16 kHz mono | 入播放缓冲 |
| `0x12` Caption | JSON | `{text, role, is_final}` | 覆盖式更新气泡（§8.1） |
| `0x11` Interrupter | — | 空 | 清播放队列、截断当前助手气泡、重置说话态 |
| `0x13` UserSpeechStart | — | 空 | 用户说话态开始 |
| `0x14` UserSpeechEnd | — | 空 | 用户说话态结束；记录延迟起点 |
| `0x15` AgentSpeechStart | — | 空 | 助手说话态开始 |
| `0x16` AgentSpeechEnd | — | 空 | 助手说话态结束 |
| `0x04` Close | — | 空 | 会话终止，转 `closed` |

`TypeError`（`0x02`）在 `internal/protocol` 中已定义，但当前 `sink` 插件不产出，前端按"可能出现的未知类型"处理：记录进包日志，不改状态。

### 5.3 入站类型（前端 → 服务端）

`internal/engine/binding.go:101` 的 `ReceivePacket` **只接受四种**：`TypeAudio`、`TypeClose`、`TypePause`、`TypeResume`，其余返回 `unknown session packet type`。

由此得出两条前端约束：

1. **上行只有音频和关闭**。Pause/Resume 虽然被接受，但那是 gateway↔machine 在用的；浏览器断开时后端已自动 pause，重连自动 resume，前端发了反而多一次无效状态变更。
2. **不提供客户端打断**。`TypeInterrupter` 不在入站白名单里，发了会被 `ReceivePacket` 拒。若要支持真正的客户端打断，需要改 `internal/engine/binding.go` 的入站分派，属于 engine 高风险区（AGENTS.md §7），本期不做（§13.4）。

打断对本页面是**单向的展示逻辑**：VAD 检测到用户开口或 ASR 判定后，经 `interrupter` 插件产出 `SignalInterrupter` → `sink` 发 `0x11`，前端收到后立即清队列并截断气泡。页面不提供任何发起打断的控件，也不做“先本地暂停再等同步”这类中间态。

### 5.4 音频参数

`internal/engine/engine.go:54`：`AudioSampleRate = 16000`、`AudioChannels = 1`、`AudioFrameSize = 320`（10ms 的 PCM16）。

- 上行：16 kHz / 1ch / 16-bit LE。采集按 20ms 分帧（640B），**每 5 帧合并成一个 100ms 包发送**（3200B），包数降为 1/5；后端不做帧对齐假设。
- 下行：同样格式，按到达顺序入环形缓冲，不做重排假设。
- 上行码率 = 16000 × 16 = 256 kbps。**不引入 Opus**：桌面与局域网环境下带宽不是瓶颈，引入编解码会让前端多一层 WebCodecs、后端多一个解码插件，与本期目标无关（§13.3）。

### 5.5 会话生命周期

| 动作 | 接口 | 前端处理 |
| --- | --- | --- |
| 建会话 | `POST /sessions` `{name, properties}` | `name` = workflow id；`properties` 来自配置面板 |
| 查状态 | `GET /sessions/health/{id}` | 重连前确认会话仍存在 |
| 续期 | `POST /sessions/renew/{id}` | 见下 |
| 销毁 | `DELETE /sessions/{id}` | 挂断按钮 |
| 实时流 | `GET /realtime/{id}`（WS） | 升级失败返回 409 表示会话已被占用 |

**活跃度维持**：空闲超时 1 分钟，清理 ticker 间隔 30 秒（`internal/engine/session.go:123`：`time.NewTicker(sm.timeout / 2)`）。

- 连接期间：每 20 秒调一次 `POST /sessions/renew/{id}`。**不能用 WebSocket Ping 帧**——`OnPing` 虽然会 `UpdateActivity()`，但那是控制帧，浏览器 JS 无法发送；发一个空二进制帧又会先在 `Packet.Unmarshal` 上报错，根本到不了 `ReceivePacket`，环节点不会刷新。20s 留了一次重试余量，不会被 30s ticker 卡到。
- 页面隐藏（`visibilitychange` → hidden）时浏览器会节流定时器，回到前台必须立刻补一次 renew，否则会话可能已被清理。
- 重连流程：`health` 确认存在 → 重连同一 `session_id` → 失败且会话不存在则回 `idle` 并提示「会话已过期」。

---

## 6. 音频链路与回声消除

### 6.1 为什么 AEC 是功能正确性问题

外放时扬声器的 TTS 会被麦克风收回，链路后果是：

~~~text
TTS 播放 → 麦克风 → 上行 TypeAudio
  → ten_vad 判定 UserSpeechStart
      → interrupter 插件产出 SignalInterrupter
          → 助手打断自己
  → ASR 把 TTS 内容转成文字 → LLM 收到自己上一轮输出
~~~

所以 AEC 失效的表现不是"有回音"，而是"AI 莫名打断自己 / 答非所问"，排查时极易误判成 VAD 或引擎参数问题。这属于 `docs/designs/02-connection-layer` 之外、但对实时正确性一票否决的约束。

### 6.2 平台 AEC 的事实核查

不能假设 `echoCancellation: true` 就等于"回声被消除"。已知情况：

| 事实 | 影响 |
| --- | --- |
| 2020 年前后 Chromium 只对 `RTCPeerConnection` 收到的音频做 AEC，不对 WebAudio 播放做取消（crbug 687574 / 40504498）。Mozilla Hubs 因此在 PR #2361 里加了 loopback workaround | 网上大部分"必须 loopback"的资料都已过时 |
| crbug 40504498「AEC when using Web Audio API」现标记为 Fixed；M111 的改动把**默认输出设备**确立为 `getUserMedia` 的 AEC 参考 | 现代 Chrome 下 WebAudio 播放可以被消除 |
| 该改动同时说明：一旦 `AudioContext` 显式 `setSinkId`，AEC 参考会切换到该 sink；非默认输出设备下 AEC 会失效（crbug 40254422） | 页面里的"输出设备选择器"是陷阱 |
| `echoCancellation` 是 best-effort 可约束属性，设备/驱动可能忽略；真实值要看 `track.getSettings()` | UI 必须显示实际生效值 |
| Safari / Android Firefox 会取消非 WebRTC 音频；桌面 Firefox 不稳定 | 跨浏览器行为不一致 |
| Electron 中 `echoCancellation: true` 完全无效（electron#47043） | 一旦做桌面壳，必须走兜底方案 |

结论：平台 AEC 作为主路径，但必须**回读实际生效值 + 可降级**，不能只在代码里写一个 constraint 就当作完成。

### 6.3 三层方案

~~~text
L1 平台 AEC（默认）
    └─ getUserMedia 约束 + getSettings() 校验，未启用则 ↓
L2 半双工门控（功能降级，UI 显式标注）
    └─ 仍不满足 ↓
L3 确定性 AEC
    ├─ WebRTC loopback（成本低，Chrome 专用）
    └─ WASM AEC3（成本最高，全平台确定）
~~~

#### L1 平台 AEC

```ts
const stream = await navigator.mediaDevices.getUserMedia({
  audio: {
    echoCancellation: { ideal: true },
    noiseSuppression: { ideal: true },
    autoGainControl: { ideal: true },
    channelCount: { ideal: 1 },
  },
});
```

用 `{ ideal: true }` 而不是裸布尔：裸值在部分实现里按 required 处理，被拒时连整个音轨都拿不到；`ideal` 让麦克风无论如何都能打开，AEC 尽力而为。

两条硬性约束：

1. **播放走原生 `<audio>` 元素**。worklet 只负责重采样和缓冲，输出接到 `MediaStreamAudioDestinationNode`，再交给 `document.createElement('audio')` 渲染。Chromium 的回声消除参考信号挂在媒体元素的输出链路上，纯 WebAudio 图不保证进入这条链路。这是让 AEC 真正生效的关键一步，不是可选项。
2. **全局只保留一个 `AudioContext`**，使用硬件默认采样率。采集与播放共享音频时钟；iOS 上多 context 并发会互相打断。

AEC 完全由浏览器负责，页面不加任何门控、不做静音、不重采样回声，也不上报"是否生效"——那条判断线已经证明没有价值。

#### L3 确定性 AEC

- **WebRTC loopback**：把 WebAudio 播放输出接进 `MediaStreamAudioDestinationNode`，绕一圈本地 `RTCPeerConnection` 再播放，让 Chrome 把它识别为"远端音频"从而生成正确的 AEC 参考。约 100 行，做成设置项「强制 loopback 模式」。这是 Hubs PR #2361 的做法，在 M111 之后多数场景不再必需，但作为跨版本保险成本很低。
- **WASM AEC3**：自带 far-end reference，全平台确定，但要处理 WASM 体积、CPU 占用、参考信号对齐。**仅在必须支持 Electron 时考虑**，浏览器场景不划算。

### 6.4 音频管线

~~~text
上行
  getUserMedia(AEC 在此生效)
    → MediaStreamAudioSourceNode
      → AudioWorklet "capture"
          48k → 16k 3:1 抽取（FIR 抗混叠，不能裸抽样）
          16-bit 打包，20ms = 640B
        → transport.send(TypeAudio)

下行
  onPacket(TypeAudio, PCM16@16k)
    → AudioWorklet "playback" 环形缓冲（60s 容量）
        16k → 48k 线性插值
      → destination（默认输出设备）

打断
  0x11 Interrupter
    → 环形缓冲整体清零（单条消息，原子）→ 下一帧直接是静音
~~~

要点：

- 采集与播放各自一个 worklet processor 文件，放 `src/audio/worklets/`，用 Vite 的 `?url` 引入。worklet 不能 `import` 业务模块，编解码逻辑必须在 processor 内自包含或经 `port.postMessage` 传入。
- 播放环形缓冲按 `AudioWorkletProcessor` 的 `process()` 回调持续消费。**不要用逐个 `AudioBufferSourceNode` 调度**——每包一个 source 会在包边界产生可听的缝隙，也无法在打断时干净丢弃。
- 缓冲策略必须在 worklet 里写死：**容量 60s**（16k × 60 × 4B ≈ 3.8MB），客户端负责暂存，不主动节流；只有写满时才丢最旧的样本，**不设低水位裁剪**——裁剪等于删除用户本该听到的音频。**`clear` 立即清零**读写指针，重建节点是不必要的。
- 收到 `0x11 Interrupter` / `0x04 Close` / 页面卸载都走同一个 `clear()`。消息在 worklet 的消息队列里有序，所以打断后到达的 `clear` 一定会清掉打断前已入队的那批写入。
- 被打断时不能有"残留尾巴"：如果缓冲里还有几百毫秒的音频没放完，用户听到的是助手把话说了一半，而字幕已经标了「已打断」。
- 诊断面板已整体移除，缓冲水位等指标不再上屏。
- 采集侧不使用 `AudioContext({ sampleRate: 16000 })`：那会额外开一个硬件上下文，且 16 kHz 非设备原生速率，重采样由浏览器在 AEC 之后完成，反而让"我到底控制了什么"变模糊。worklet 内自己做抽取更可测。

### 6.5 诊断面必须暴露的 AEC 信息

~~~text
EC ✓   NS ✓   AGC ✓          ← getSettings() 实际值
模式   平台 AEC / 半双工 / loopback
模式   平台 AEC               ← getSettings() 推断
上行 RMS  -32 dBFS           ← 实时
播放水位  42 ms
~~~

这是排查回声问题唯一的一手信息。缺了它，问题会以"AI 自己打断自己"的形式出现，排查成本极高。

---

## 7. 模块划分与依赖

~~~text
clients/web/src/
  router.tsx                      # 新增，/ 与 /chat
  pages/Editor/EditorPage.tsx     # 由现有 App.tsx 内容迁入
  pages/Chat/
    ChatPage.tsx                  # 三栏骨架 + 状态机装配
    panels/SessionConfigPanel.tsx
    panels/ConversationStream.tsx
    panels/DebugPanel.tsx
    panels/AudioDock.tsx
    panels/SpeechTimeline.tsx     # 签名元素
    components/MessageBubble.tsx
    components/LevelMeter.tsx
    components/SignalBadge.tsx
    hooks/useSessionController.ts
    hooks/useCapture.ts
    hooks/usePlayback.ts
  realtime/
    connection.ts                 # RealtimeConnection 契约 + provider 注册表
    providers/websocket.ts        # 注册制：websocket 传输
    providers/demo.ts             # 注册制：脚本包传输
    index.ts                      # 内置 provider 注册与导出
    packet.ts                     # 8B header 编解码 + 类型常量
    session.ts                    # create/health/renew/delete
  audio/
    engine.ts                     # AudioContext 单例、采集/播放工作线程
    aec.ts                        # 约束、getSettings 校验、半双工策略
public/worklets/                  # 必须是独立文件，不能被打包器内联
    capture-processor.js          # 48k→16k FIR 抽取 + Int16 分帧
    playback-processor.js         # 16k→48k 环形缓冲 + 打断清空
  store/useSessionStore.ts
~~~

分层约束：

- `realtime/` 不知道 DOM 音频存在，`audio/` 不知道传输存在。二者只在 `useSessionController` 汇合。
- 页面组件不直接构造传输，只通过 `useSessionController` 暴露的动作；控制器只依赖 `createConnection(kind)` 返回的接口。
- 连接层统一为「入站回调 + 出站 publish」：`open(target, { onMessage, onState, onMalformed })` 与 `publish(packet)`。新增传输只需实现该接口并在 `realtime/index.ts` 注册一行。

新增依赖清单：

| 包 | 用途 | 说明 |
| --- | --- | --- |
| `react-router-dom` | 路由 | 唯一硬性新增 |

不引入 `grpc-web` / `@connectrpc/connect-web`：浏览器侧不做 gRPC（§3.3）。

UI 库保持 MUI 7，不额外引入组件库：气泡、时间轴、电平条都是几行 MUI `Box` + SVG，引第三方聊天或图表库反而会带来一套无关的设计语言。

---

## 8. 交互细节

### 8.1 字幕合并

`internal/plugins/caption/plugin.go:92` 的 `builder.String()` 发送的是**累积全文**，不是增量。因此：

- `is_final: false` → 覆盖当前同一 `role` 的气泡文本，不追加。
- `is_final: true` → 关闭该气泡，后续文本开新气泡。
- 收到 `0x11` → 当前助手气泡打「已打断」标记，之后的文本开新气泡。
- 文本更新走 ref 累积 + `requestAnimationFrame` 提交，不直接 `setState`，避免高频渲染。

### 8.2 错误态

`feedback.md` › Best practices：反馈的呈现方式要匹配信息的重要性；频繁使用 alert 会削弱其效果。因此用三档：

| 情况 | 呈现 |
| --- | --- |
| 会话已被其他连接占用（409 / WS 1008） | 对话区内的替换式提示 + 「重新开始」，不用 Snackbar |
| 连接断开、正在重连 | AppBar 状态点转 warn + 倒计时文案，不弹窗 |
| 会话过期（health 404） | 对话区替换式提示 + 「新建会话」 |
| 麦克风权限被拒 | 配置面板内联说明 + 重试按钮，不能只弹一次 toast |

现有 `MessageProvider`（`Snackbar` + `Alert`）保留给配置保存之类的短反馈，不用于实时状态。

### 8.3 离开页面

`beforeunload` 与路由离开时：停止采集、关闭连接、`DELETE /sessions/{id}`。不做"保留会话等你回来"——后端 1 分钟空闲清理已经覆盖了误关场景，强行保留会让用户下次进来撞上 409。

---

## 9. 性能影响

前端新增，对后端热路径无影响。前端侧关注点：

| 项 | 目标 | 依据 |
| --- | --- | --- |
| 包解码 | 每包 0 次字符串分配，Payload 用 `Uint8Array` 视图不拷贝 | 与后端 `UnmarshalHeader` 零拷贝语义对齐 |
| 播放水位 | 稳定在 40–80 ms | 低于 40ms 易断续，高于 150ms 打断延迟可感 |
| 采集上行延迟 | 20ms 块 + worklet 传输，端到端 < 50ms | 待实测 |
| 字幕渲染 | ≤ 30 次/秒提交 React | rAF 节流 |
| 包日志 | 上限 500 条环形缓冲 | 避免长会话内存增长 |
| 对话主包体积 | 不引入 reactflow / @rjsf | 路由按需加载 |

均为设计目标，M1 完成后在真实设备上实测填充。

---

## 10. 测试设计

### 10.1 单元测试

- `realtime/packet.ts`：magic 错误、size 与实际长度不符、空 payload、超长 payload、随机字节的模糊测试。
- `realtime/session.ts`：health 404 时的状态迁移、renew 失败重试与上限。
- `audio/aec.ts`：`resolveAecMode` 的四条分支（EC 已启用 / 被拒 / 未报告 / 非默认 sink）与 `shouldDropUplink` 的门控规则。
- 字幕合并 reducer：`is_final` 翻转、打断后开新气泡、role 切换。

### 10.2 集成与手测

| 场景 | 判定 |
| --- | --- |
| 完整一轮对话（外放 70% 音量） | 无自发打断，字幕正确，音频连续无缝隙 |
| 用户开口打断（barge-in） | 收到 `0x11` 后 ≤ 100ms 静音，缓冲归零 |
| 拔网线 10 秒 | 自动重连成功，`health` 仍为 running/paused，会话未丢 |
| 双标签页同一 session | 第二个明确提示被占用，不转圈 |
| 后台标签页停留 2 分钟再回前台 | 会话存活（验证 renew 补偿） |
| 切换输入设备 | 重建采集、AEC 状态按新设备重算 |
| 系统「减少动态效果」开启 | 无位移动画，电平指示仍可读 |
| 键盘走完全流程 | 可完成开始→挂断，焦点始终可见 |

### 10.3 AEC 专项验收

- 在至少三类设备上跑真实对话：笔记本内置扬声器+麦克风、外接 USB 麦克风 + 显示器扬声器、耳机（外放与耳机的回声表现完全不同）。
- 用 `speex` 或项目现有 TTS 样本播放，录音后人工听回采内容。
- 标定 §6.3 的 12 dB 阈值：录制"通过"和"不通过"两组数据，确认阈值落在这两组之间。

---

## 11. 修改范围与兼容影响

| 区域 | 变更 |
| --- | --- |
| `clients/web/package.json` | 新增 `react-router-dom` |
| `clients/web/src/main.tsx` | 包一层 Router |
| `clients/web/src/App.tsx` | 内容迁入 `pages/Editor/EditorPage.tsx` |
| `clients/web/src/theme/tokens.ts` | 补语义色与对比度达标值（§4.4） |
| `clients/web/src/components/Layout/WorkflowList.tsx` | 菜单加「发起对话」入口 |
| `clients/web/src/{realtime,audio,pages/Chat,store}` | 新增 |
| 后端 | **无代码改动**。§13.1 的缺陷另开任务 |
| 构建 | 无需改 Makefile；`make build-web` 会带上新页面。注意 `clients/web/dist` 走 `go:embed`（`clients/assets.go`），改前端后必须重新 `make build-web` 才进二进制 |

对外协议无变化。`docs/reference/protocol.md` 无需修改，除非将来重新决定接入 gRPC（§13.2）或开放客户端打断（§13.4）。

---

## 12. 分阶段实施

| 阶段 | 内容 | 完成判据 |
| --- | --- | --- |
| **P0** | 路由改造、三栏骨架、状态机、包日志与时间轴（假数据）、视觉 token | **已完成** |
| **P1** | 音频 worklet、AEC 约束校验 + 半双工、WS 接通、字幕气泡、`0x11` 清队列 | **已完成** |
| **P2** | AEC 加固（L2 标定、L3 loopback）、设备切换、重连与 renew、错误态、无障碍收尾 | §10.2 全部通过 |

P1 必须包含回声处理，不能留到 P2。理由见 §6.1：回声的表现形式是"AI 自己打断自己"，延后处理会让 P1 的联调结论不可信。

不再有 P3：gRPC 接入与客户端打断都已排除（§13.2 / §13.4），剩下的协议一致性缺口（§13.6）是后端侧工作，不属于本设计范围。

---

## 13. 风险与待决问题

### 13.1 后端前置缺陷（阻塞 P1 验收）

**`Session.LastActive` 单位不一致。** 创建时 `session.LastActive.Store(time.Now().Unix())` 写的是**秒**（`internal/engine/session.go:197`），而 `UpdateActivity()`（`:296`）和 `Cleanup()`（`:264`）用的是**毫秒**。清理逻辑比较 `now(ms) - LastActive`，创建后未及时更新的会话差值约为 1.7×10¹² ms，远超 1 分钟超时。

后果：会话创建后若在第一个清理 tick（30 秒）之前没有触发 `UpdateActivity`，就会被回收。触发 `UpdateActivity` 的路径只有 `OnPing` 和 `ReceivePacket`——WS 的 `OnOpen`（`biz/realtime/socket.go`）不调用它。**浏览器只连接、不立刻发音频时，会话会在 30 秒内被杀。**

**gRPC 路径完全没有活跃度更新。** `biz/realtime/grpc.go` 从不调用 `UpdateActivity`，而 `docs/reference/protocol.md` §2.2 声称「gRPC 服务端会根据流状态自动维护活跃情况」。这条描述与实现不符，gRPC 客户端静默 30 秒以上同样会被回收。

前端可做的规避：每 20 秒 `POST /sessions/renew/{id}`，并对 `health` 404 做降级处理。但正确做法是后端修复——把创建时的 `Unix()` 改成 `UnixMilli()`，并在 WS `OnOpen` 调一次 `UpdateActivity`。建议作为独立修复先于 P1 落地。

同时记一笔不阻塞本设计的现象：gRPC 路径从不调用 `UpdateActivity`（`biz/realtime/grpc.go`），而 `docs/reference/protocol.md` §2.2 声称「gRPC 服务端会根据流状态自动维护活跃情况」，描述与实现不符。前端不接入 gRPC，因此不影响本设计，但属于需要单独修正的文档/实现偏差。

### 13.2 gRPC：不做

已定：浏览器侧不接入 gRPC，也不提供置灰选项或“桌面客户端可用”提示。理由见 §3.1–§3.3。若将来需要，先决定 §3.2 的 B（服务端桥接）或 D（桌面壳），再在本设计上增量扩展。

### 13.3 音频编码：不做 Opus

已定：上行、下行都保持 PCM16 16 kHz 单声道。不引入 WebCodecs 与 Opus。

256 kbps 对桌面与局域网不是问题，也是当前项目的既有形态（`engine.AudioFormat = "pcm"`）。若将来面向移动网络，再单独设计编解码方案。

### 13.4 客户端打断：不做

已定：页面不提供打断按钮，也不在客户端做"本地打断"（清队列 + 静音）的伪动作。打断完全由服务端驱动，前端只在收到 `0x11` 时反应。

这是出于两方面的考虑：一是上行协议没有打断类型，本地动作无法与服务端状态对齐，容易让用户以为已经中断而实际没有；二是避免后续需要时把"本地打断"错误地当成已实现能力。真正的客户端打断需要改 `internal/engine/binding.go` 的入站分派（engine 高风险区，AGENTS.md §7），另开设计。

### 13.5 明暗模式开关：维持现状

已定：不改变现有 `ThemeContext` + AppBar 切换按钮的交互模型。对话页跟随同一开关，两套主题都要完整实现并测对比度（§4.4）。

与 `dark-mode.md` › Best practices（不提供应用级外观设置）的抵触保留现状——收敛会改动编辑器页面，超出本设计范围。

### 13.6 协议一致性缺口

- `api/voce/v1/voce.proto` 的 `PacketType` 枚举缺 `TYPE_PAUSE`(5) / `TYPE_RESUME`(6)，但 `internal/protocol` 已定义。gRPC 客户端无法用枚举名表达这两个值。
- `clients/voce-tui/src/protocol/mod.rs` 定义了 `SessionId = 0x17`，后端 `internal/protocol` 无对应类型。

两处都不阻塞 WS 路径，但会在将来引入第二种传输或校准 TUI 时暴露。记录在此，不属于本设计范围。

---

## 14. 参考资料

项目内：

- `internal/protocol/packet.go` — 封包格式与类型定义
- `internal/engine/binding.go` — `ReceivePacket` 的入站类型白名单、lease 与 outLoop
- `internal/engine/session.go` — `LastActive`、清理 ticker、`Cleanup`
- `internal/plugins/sink/plugin.go` — 出站包类型的唯一产出口
- `internal/plugins/caption/plugin.go` — 字幕累积语义
- `biz/realtime/socket.go` — WS 服务端实现；`grpc.go` 仅供对照（前端不接入）
- `docs/reference/protocol.md` — 对外协议现状
- `clients/voce-tui/src/{protocol,session,audio}` — 已有的客户端侧实现，可直接对照
- `docs/designs/02-connection-layer/design.md` — 连接层抽象边界

外部：

- grpc-web「Streaming Support」与 `doc/streaming-roadmap.md` — 双向流限制
- crbug 40504498「AEC when using Web Audio API」、crbug 40254422 — AudioContext 与 AEC 参考
- Chromium 提交 `a2aea75`（M111）— 默认输出设备作为 AEC 参考；`setSinkId` 的影响
- electron#47043 — Electron 下 `echoCancellation` 无效
- Mozilla Hubs PR #2361 — WebAudio 播放的 AEC loopback workaround
