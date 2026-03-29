# Voce 实时接入协议 (Real-time Integration Protocol)

本文档定义了 Voce 语音交互系统的会话管理流程与实时双向流传输标准。

---

## 1. REST API 通用规范 (Common API Standards)

Voce 的管理接口（如会话创建、状态查询）遵循标准的 RESTful 风格，并使用统一的响应体封装。

### 1.1 响应报文格式 (JSON Structure)

所有 HTTP 接口均返回如下结构的 JSON：

```json
{
  "code": 200,          // 业务状态码，通常与 HTTP 状态码保持一致
  "message": "ok",      // 错误提示或成功消息
  "timestamp": 1711610000, // 服务端处理请求的毫秒级时间戳
  "data": { ... }       // 业务负载数据 (T)
}
```

### 1.2 状态码约定 (HTTP Status Codes)

Voce 遵循 HTTP 协议状态码规范：

- **200 OK**: 请求成功。
- **400 Bad Request**: 参数校验失败或 JSON 格式错误。
- **401 Unauthorized**: API KEY 或验证令牌无效。
- **404 Not Found**: 资源（如 SessionID）不存在。
- **500 Internal Server Error**: 系统内部插件处理异常。

---

## 2. 会话生命周期与续期 (Session Lifecycle & Renewal)

Voce 遵循 **“先控制，后数据”** 的原则。所有的长连接流必须关联一个已存在的会话 (Session)。

### 2.1 创建会话 (Handshake / Create Session)

- **方法**: `POST`
- **路径**: `/sessions`
- **请求体 (Request)**:
  ```json
  {
    "name": "workflow_name", // 对应配置中的 Workflow ID
    "session_id": "optional", // 可选，由调用方指定
    "properties": {
      // 动态配置注入，可覆盖节点默认参数
      "deepgram_asr": { "language": "zh-CN" }
    }
  }
  ```
- **响应 (Response)**: `{ "code": 200, "data": { "session_id": "..." } }`

### 2.2 会话续期 (Session Renewal)

Voce 的会话有 **空闲超时机制 (默认 1 分钟)**。

- **长连接期间**:
  - **WebSocket**: 客户端必须定期发送标准 WebSocket **Ping** 帧，服务端收到后会更新该会话的活跃时间。
  - **gRPC**: 流连接建立后，服务端会根据流状态自动维护活跃情况，**无需手动发送心跳请求**。
- **断开连接后**: 如果长连接因网络等原因断开，客户端必须通过 HTTP 接口手动续期以防止会话被销毁：
  - **方法**: `POST`
  - **路径**: `/sessions/renew/{session_id}`
  - **说明**: 成功调用后，会话的生命周期将重置。

---

## 3. 传输协议定义 (Dual-Protocol Support)

Voce 目前同时支持 **WebSocket** 与 **gRPC** 两种流式连接。

### 3.1 方案 A：WebSocket (Binary Custom Packet)

基于自定义字节流封包，固定头部长度 **8 字节**。

#### **数据报头部结构 (Packet Header)**

| 字段 (Field)      | 长度 (Len) | 说明 (Description)                            |
| :---------------- | :--------- | :-------------------------------------------- |
| **Magic1 (0x56)** | 1B         | 固定字符 'V'                                  |
| **Magic2 (0x43)** | 1B         | 固定字符 'C'                                  |
| **Type**          | 1B         | 包类型 (Audio: 0x01, Text: 0x03, Close: 0x04) |
| **Encode**        | 1B         | 载荷编码 (Raw/Binary: 0x00, JSON: 0x01)       |
| **Size**          | 4B         | Payload 长度 (BigEndian Uint32)               |
| **Payload**       | N B        | 实际业务数据 (长度由 Size 决定)               |

### 3.2 方案 B：gRPC (Bidirectional Streaming)

- **服务定义**: `api/voce/v1/voce.proto`
- **连接元数据 (Metadata)**: 连接建立时，客户端必须在 Context Metadata 中携带 `session_id: <ID>`。

---

## 4. 边界处理与独占机制 (Boundaries & Errors)

### 4.1 独占连接 (Acquire & Release)

- **并发限制**: 一个 `session_id` 同时只能持有一个活跃的长连接。
- **策略**: **Exclusive Lock (优先占用保护)**。如果连接 A 存活，连接 B 带着同 ID 接入，B 将收到 `429/ResourceExhausted` 或 WS `1008` 错误。

### 4.2 强制销毁会话 (Terminate Session)

- **方法**: `DELETE`
- **路径**: `/sessions/{session_id}`
- **说明**: 停止该会话相关的 Workflow 并释放所有资源。
