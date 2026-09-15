# 监控与指标 (Monitoring & Metrics)

Voce 使用 OpenTelemetry Metrics 采集进程和业务指标，并通过 Prometheus exposition format 以 HTTP pull 模式暴露。业务模块自行定义指标，`internal/telemetry` 只负责初始化 MeterProvider、Prometheus exporter 和 Go runtime instrumentation。

本文档描述 OpenTelemetry 监控方案。部署该方案后，旧的 JSON `/monitor` 接口不再提供；Gateway 的 `/state` 保留为按需查看机器与连接池明细的管理接口，不是时序指标接口。

---

## 1. 抓取方式 (Pull Model)

Standalone Voce、Gateway 和每个 Gateway Worker 都暴露同一个端点：

```text
GET /metrics
```

例如：

```bash
# Standalone Voce 或 Gateway Worker
curl -s http://127.0.0.1:7001/metrics

# Gateway
curl -s http://127.0.0.1:7001/metrics
```

端口由各进程配置决定。Gateway 与 Worker 可以使用不同端口，但指标路径保持为 `/metrics`。

Prometheus 应独立抓取 Gateway 和每个 Worker：

```text
Prometheus
  ├── Gateway /metrics
  ├── Worker A /metrics
  ├── Worker B /metrics
  └── Worker N /metrics
```

Gateway 不聚合 Worker 的指标，也不代理 Worker 的 `/metrics`。这样每条指标都保留产生它的实例，横向扩容后仍可按实例、服务角色和部署环境查询。

### Prometheus 配置示例

```yaml
scrape_configs:
  - job_name: voce-gateway
    static_configs:
      - targets: ["gateway:7001"]

  - job_name: voce-worker
    static_configs:
      - targets: ["voce-worker-1:7003", "voce-worker-2:7003"]
```

生产环境通常由 Kubernetes Service Discovery、Consul 或其它服务发现机制维护 targets。`job`、`instance`、集群和区域标签应由部署侧统一添加，不应写入业务指标标签。

---

## 2. 命名与标签 (Naming & Labels)

Voce 在 OpenTelemetry 中使用点分隔的指标名，例如：

```text
gateway.pool.connections.active
```

Prometheus exporter 使用 `voce` namespace，并将名称转换为下划线形式：

```text
voce_gateway_pool_connections_active
```

Counter 在 Prometheus 中带 `_total` 后缀；UpDownCounter 导出为 Gauge。例如：

```text
gateway.pool.dials                     -> voce_gateway_pool_dials_total
gateway.pool.connections.active        -> voce_gateway_pool_connections_active
```

所有 Voce 指标附带 OpenTelemetry resource 信息，例如：

```text
service_name
deployment_environment_name
```

Prometheus exporter 还会附加 `otel_scope_name`。当不同模块使用了相同的 OTel 指标名时，必须使用该标签区分来源。

例如 `sessions.active` 同时存在于 Worker Engine 与 Gateway：

```promql
# Worker 中正在运行的 Workflow Session
voce_sessions_active{otel_scope_name="github.com/wnnce/voce/internal/engine"}

# Gateway 中维护的路由 Session
voce_sessions_active{otel_scope_name="github.com/wnnce/voce/internal/gateway"}
```

不要为 `session_id`、`machine_id`、workflow 实例 ID、客户端 IP 等无界值添加指标标签。这些值会造成高基数问题，应保留在日志、Trace 或 Gateway `/state` 管理快照中。

---

## 3. 指标清单 (Metric Inventory)

### 3.1 Go Runtime

OpenTelemetry Go runtime instrumentation 自动采集 goroutine、堆内存、GC 目标、分配次数和 GOMAXPROCS 等进程指标。它们的确切名称随 instrumentation 的兼容模式变化，查询时应以实际 `/metrics` 输出为准。

这些指标用于识别运行时层面的异常，例如：

- goroutine 数持续增长；
- 已用内存持续增长且无法回落；
- GC 压力或调度延迟异常。

业务代码不依赖具体 runtime 指标名，也不再维护旧的 `runtime.ReadMemStats` JSON 字段。

### 3.2 Schema 对象

| Prometheus 指标 | 类型 | 说明 |
| --- | --- | --- |
| `voce_schema_audio_objects_active` | Gauge | 当前存活的 Audio 对象数量。 |
| `voce_schema_video_sd_objects_active` | Gauge | 当前存活的 SD Video 对象数量。 |
| `voce_schema_video_hd_objects_active` | Gauge | 当前存活的 HD Video 对象数量。 |
| `voce_schema_video_fhd_objects_active` | Gauge | 当前存活的 FHD Video 对象数量。 |

这些指标用于观察引用计数和对象释放是否平衡。它们不等同于业务会话数量。

### 3.3 Session

| Prometheus 指标 | Scope | 类型 | 说明 |
| --- | --- | --- | --- |
| `voce_sessions_active` | `github.com/wnnce/voce/internal/engine` | Gauge | Worker 中当前存活的 Workflow Session。 |
| `voce_sessions_active` | `github.com/wnnce/voce/internal/gateway` | Gauge | Gateway 当前维护的 Session 路由。 |

Gateway Session 数与 Worker Workflow Session 数在稳定状态通常接近，但不要求严格相等：网络断开、机器超时和清理窗口都可能造成短暂差异。

### 3.4 Worker 客户端流量

| Prometheus 指标 | 类型 | 说明 |
| --- | --- | --- |
| `voce_realtime_websocket_bytes_received_total` | Counter | Worker 从直连 WebSocket 客户端收到的字节数。 |
| `voce_realtime_websocket_bytes_sent_total` | Counter | Worker 写入直连 WebSocket 客户端的字节数。 |
| `voce_realtime_grpc_bytes_received_total` | Counter | Worker 从 gRPC 客户端收到的音频 payload 字节数。 |
| `voce_realtime_grpc_bytes_sent_total` | Counter | Worker 发送到 gRPC 客户端的 payload 字节数。 |

这些指标只描述客户端直接连接 Worker 的链路。Gateway 模式下客户端 WebSocket 流量应查看 Gateway client 指标。

### 3.5 Gateway 客户端 WebSocket

| Prometheus 指标 | 类型 | 说明 |
| --- | --- | --- |
| `voce_gateway_client_websocket_connections_active` | Gauge | Gateway 当前活跃的客户端 WebSocket 连接数。 |
| `voce_gateway_client_websocket_bytes_received_total` | Counter | Gateway 从客户端收到的合法二进制包字节数。 |
| `voce_gateway_client_websocket_bytes_sent_total` | Counter | Gateway 成功写入客户端的字节数。 |
| `voce_gateway_client_websocket_packets_received_total` | Counter | Gateway 从客户端收到的合法二进制包数。 |
| `voce_gateway_client_websocket_packets_sent_total` | Counter | Gateway 成功写入客户端的包数。 |
| `voce_gateway_client_websocket_write_errors_total` | Counter | Gateway 向客户端写数据或 Pong 时发生的错误数。 |

### 3.6 Gateway-Machine 数据连接池

Gateway 侧指标的前缀为 `voce_gateway_pool_`：

| 后缀 | 类型 | 说明 |
| --- | --- | --- |
| `connections_active` | Gauge | 当前已建立的数据连接数。 |
| `connections_connecting` | Gauge | 正在建立或重连的数据连接数。 |
| `pending_dials` | Gauge | 已预留但尚未创建完成的拨号数。 |
| `sessions_routed` | Gauge | 当前绑定至数据连接的 Gateway Session 数。 |
| `dials_total` | Counter | 数据连接拨号尝试次数。 |
| `reconnects_total` | Counter | 活跃连接断开后进入重连的次数。 |
| `bytes_received_total` / `bytes_sent_total` | Counter | Gateway-Machine 内部链路的收发字节数。 |
| `packets_received_total` / `packets_sent_total` | Counter | Gateway-Machine 内部链路的收发包数。 |
| `write_errors_total` | Counter | Gateway 向 Machine 数据连接写入失败次数。 |

### 3.7 Machine 数据连接池

Worker Machine 侧指标的前缀为 `voce_machine_pool_`：

| 后缀 | 类型 | 说明 |
| --- | --- | --- |
| `connections_active` | Gauge | Worker 接收 Gateway 数据连接的活跃数量。 |
| `sessions_routed` | Gauge | Worker 当前路由到回程数据连接的 Session 数。 |
| `bytes_received_total` / `bytes_sent_total` | Counter | Worker 数据连接池的收发字节数。 |
| `packets_received_total` / `packets_sent_total` | Counter | Worker 数据连接池的收发包数。 |
| `write_errors_total` | Counter | Worker 数据连接池写入失败次数。 |

Gateway pool 与 Machine pool 是同一条内部链路两端的观测。它们的字节和包数可用于交叉核对，但不能在全局总流量中直接相加。

---

## 4. 常用查询 (PromQL Examples)

```promql
# Gateway 客户端上行字节速率，单位 bytes/s
sum(rate(voce_gateway_client_websocket_bytes_received_total[5m]))

# Gateway-Machine 内部链路写失败速率
sum(rate(voce_gateway_pool_write_errors_total[5m]))

# 最近 5 分钟发生过重连的 Gateway 实例
sum by (instance) (increase(voce_gateway_pool_reconnects_total[5m])) > 0

# 每条 Gateway 数据连接平均承载的路由 Session
sum(voce_gateway_pool_sessions_routed)
/
clamp_min(sum(voce_gateway_pool_connections_active), 1)

# 某 Worker 的当前 Workflow Session 数
sum by (instance) (
  voce_sessions_active{otel_scope_name="github.com/wnnce/voce/internal/engine"}
)
```

Counter 应使用 `rate()` 或 `increase()` 观察变化速率。Gauge 应直接查询当前值或使用 `max_over_time()`、`avg_over_time()` 等时间窗口函数。

---

## 5. Gateway 状态快照 (Gateway State)

Gateway 的 `/state` 返回当前机器、活跃机器、客户端连接、Session 数以及每个 Machine 的连接池快照：

```bash
curl -s http://127.0.0.1:7001/state | jq
```

该接口适合排查单个 Machine、某条连接的当前状态或连接池分布。它不是 Prometheus 指标接口：响应会随请求即时生成，不能用于趋势、聚合或告警。

---

## 6. 告警建议 (Alerting)

以下规则适合作为初始告警集合：

1. Prometheus 无法抓取 Gateway 或 Worker。
2. Gateway `/state` 中没有活跃 Machine，但仍存在客户端连接或路由 Session。当前该条件需要通过管理接口或外部探针检查；`gateway.machines.active` 指标尚未提供。
3. `voce_gateway_pool_write_errors_total` 或 `voce_machine_pool_write_errors_total` 持续增长。
4. `voce_gateway_pool_reconnects_total` 在短时间内持续增长。
5. `pending_dials` 长时间不归零，且没有新的 active connection。
6. Go runtime 内存或 goroutine 数持续增长并超过部署基线。

阈值应根据实例规格、工作流类型、连接池配置和实际流量基线设定。不要把压测结果中的历史延迟数据直接作为线上告警阈值。
