---
name: test-conventions
description: 按 Voce 项目现有习惯设计、补充和运行 Go 测试，覆盖单元测试、并发与生命周期测试、协议和资源所有权测试；对热路径要求 benchmark 和分配验证。
---

# Voce 测试规范

本 skill 适用于为 Voce 修改或新增测试、benchmark，或选择验证命令。先读目标包及相邻测试，沿用该包已有的测试工具、命名和 fixture；不要把某个包的内部 helper 强行推广到全仓。

## 测试组织

- 测试文件与被测包放在同一目录，使用 `*_test.go`；包级内部行为测试通常使用同包 `package xxx`，需要验证公开 API 或隔离实现时才使用 `xxx_test`。
- 测试名使用 `Test<Type>_<Behavior>` 或清晰的 `Test<Type>`，多个场景使用 `t.Run`。表格测试适合一组输入/输出或错误分类；并发、生命周期和资源释放场景可以用独立命名子测试表达时序。
- 使用 `github.com/stretchr/testify/assert` 做可继续的断言，使用 `require` 做前置条件、构造、错误和不可继续状态。错误断言优先 `ErrorIs`，不要只比较错误字符串。
- 对资源使用 `defer` 清理：Workflow、SessionManager、Scheduler、Telemetry、连接池、Schema、Packet、临时目录和 cancel function。清理应幂等，测试不依赖其他测试留下的全局状态。
- 测试数据应小而明确；协议数据使用真实 Header/Marshal/Unmarshal，Schema 测试使用真实对象池和 ReadOnly/Mutable API，不复制一套只适用于测试的实现。

## 行为覆盖

每个改动至少覆盖成功路径和最有价值的失败路径。根据代码性质补充：

- 生命周期：Start、Ready、Pause、Resume、Stop、重复关闭、启动失败回滚、上下文取消。
- 并发：同一 key/session 的竞争、容量上限、重复绑定、并发删除和旧回调；使用 `sync.WaitGroup`、channel、atomic 和明确的完成信号。
- 异步：用 `require.Eventually` 等待状态或计数达到条件，并设置合理超时和轮询间隔；不要用固定 `time.Sleep` 作为主要同步手段。只有在验证 backoff/timing 本身时才使用短 Sleep，并说明原因。
- 所有权：Packet 无论 writer 成功或失败都 Release；Audio/Video/Payload 的 Retain/Release、池回收、写时复制和输入 payload 深拷贝都要验证。
- 背压：验证阻塞、丢弃策略、控制信号优先级、容量边界和取消后的残留队列释放。
- 协议：验证合法报文、短报文、magic/header 错误、长度不匹配、未知类型、连接关闭和 metrics 计数。
- HTTP/RPC：使用 `httptest` recorder/request 或内存 fake；如果真实网络监听是必要条件，要注明并单独运行，不能把端口环境限制伪装成代码失败。

异步测试必须有超时失败信息，避免 goroutine 永久等待。优先让被测代码提供完成 channel、Wait 或状态信号；不要通过无限重试掩盖死锁或资源泄漏。

## Mock、Fake 和外部依赖

- 使用最小 fake/plugin，仅实现当前测试需要的回调；通过 atomic 计数、channel 或受保护切片记录调用。
- 外部 HTTP、Redis、模型服务、RTC SDK 和网络连接优先使用可控 fake 或 `httptest`；测试不依赖真实 API key、公网、系统时钟或未声明的本地服务。
- 需要测试取消、慢处理或背压时，fake 应提供可控的 `started`、`release`、`done` channel，而不是随机 Sleep。
- 测试插件必须在完成前释放收到的 Schema，或者明确转移所有权；测试结束时检查没有未完成 goroutine 或池对象。

## 并发与 race

涉及 atomic、channel、Pool、Session、Gateway、Machine、Scheduler、Remote call 或生命周期回调的改动，至少运行目标包的 race 测试：

```bash
go test -race ./path/to/package/...
```

并发测试应验证结果数量、容量和身份，而不是只验证“没有 panic”。对同一 key 的并发操作检查对象是否相同、是否超卖容量、是否重复释放。不要为了让 race 通过而给热路径加测试专用锁。

## 热路径 benchmark

凡是修改以下路径之一，必须新增或更新 benchmark，并在交付中报告结果：协议 Marshal/Unmarshal、WebSocket/gRPC 编解码适配、Session/Gateway 路由、Pool Bind/Select/Unbind、Scheduler/Node 输入、Schema/对象池、音频 resample/VAD、日志 handler，以及明确的高频媒体处理函数。

benchmark 规则：

- 使用 `func Benchmark<Type>_<Operation>(b *testing.B)`，场景差异用 `b.Run`；当前 Go 版本支持时使用 `for b.Loop()`。
- 把 fixture、池初始化、输入构造、fake 服务和预热放在计时区间外；使用 `b.ResetTimer()`，需要时配合 `b.StopTimer/StartTimer`。
- 涉及分配或对象池时调用 `b.ReportAllocs()`；需要稳定比较时记录 payload 大小、消息类型、池是否预热、并发数和调度模式。
- benchmark 循环内不得无意累积切片、日志、goroutine、未释放 Schema 或 Packet。每轮创建的对象必须在本轮回收，或者明确测量它的分配成本。
- 并发热路径使用 `b.RunParallel`，单线程路径保持单独 benchmark；不要用 benchmark 替代正确性测试。
- 协议 benchmark 应覆盖真实输入：Packet Marshal/Unmarshal 分别测量，Writer/网络系统调用需要单独标明是否 fake；不要把网络等待和编解码成本混成一个指标。
- 输出至少比较 `ns/op`、`B/op`、`allocs/op`；性能结论必须基于相同机器、编译参数、payload、日志/metrics 和预热条件。没有基线就报告当前测量，不能写“无分配”或“性能无影响”。

示例形态：

```go
func BenchmarkPacket_Unmarshal(b *testing.B) {
    data := preparedPacketBytes() // 计时外
    packet := protocol.AcquirePacket()
    defer protocol.ReleasePacket(packet)
    b.ReportAllocs()
    b.ResetTimer()
    for b.Loop() {
        if err := packet.Unmarshal(data); err != nil {
            b.Fatal(err)
        }
    }
}
```

## 验证命令

按改动范围选择最小充分命令，再扩大到相关模块：

```bash
go test ./path/to/changed/package/...
go test -race ./path/to/concurrent/package/...
go test -run '^TestName$' ./path/to/package
go test -bench 'BenchmarkPattern' -benchmem ./path/to/package
```

不要为了测试临时设置或输出用户未要求的环境变量。若测试依赖本地端口、CGO、FFmpeg、Redis 或其他外部条件，先运行无外部依赖的包测试，并在结果中准确区分环境阻塞与代码失败。全仓 `go test ./...` 必须覆盖仓库可编译的包；已存在的未纳入当前 Go module 的目录应单独说明。

## 完成检查

- 成功、失败、并发、取消和资源释放场景已按风险覆盖。
- 异步测试使用可控同步和超时，避免脆弱 Sleep。
- 热路径改动有 benchmark，且 setup 在计时外、释放完整、报告分配。
- 相关 race 测试通过；失败结果没有被环境问题掩盖。
- `go test`、`go test -race`、benchmark 的命令和范围在交付说明中可复现。
