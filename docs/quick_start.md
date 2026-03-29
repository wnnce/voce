# 快速开始（Quick Start）

项目支持两种运行方式：本地编译运行和 Docker 容器化部署。

## 1. 克隆项目

```bash
git clone https://github.com/wnnce/voce.git
cd voce
```

---

## 2. 方式一：本地编译运行

### 2.1 环境准备

确保本地已安装以下工具：

- **Go** (1.25+)
- **Node.js** & **pnpm** (用于构建前端工作台)
- **Rust** & **Cargo** (用于构建 TUI 客户端)

### 2.2 编译项目

Voce 使用 `Makefile` 管理多端构建。只需一条指令即可编译服务端和客户端：

```bash
# 自动执行：pnpm 阶段生成前端静态资源 -> 嵌入到 Go 二进制中 -> 构建 Go 服务端 -> 构建 Rust TUI
make build-all
```

### 2.3 配置文件

服务端启动依赖 `configs/config.yaml`。可以从此模版创建一个：

```bash
mkdir -p configs && cp config.yaml.example configs/config.yaml
```

### 2.4 启动服务

```bash
# 在根目录启动服务端
./bin/voce -c configs/config.yaml
```

---

## 3. 方式二：Docker 运行

可以选择使用 `Docker Compose` 或直接使用 `docker run`。

### 3.1 使用 Docker Compose

预置了 `docker-compose.yml`，可以一键启动完整的 Voce 环境。

1.  **准备配置**：
    参考 2.3 节，确保 `configs/config.yaml` 已经准备好。
2.  **启动**：
    ```bash
    docker-compose up -d
    ```
    _将自动映射 7001 (Web UI) 和 7002 (Streaming API) 端口。_

### 3.2 使用 docker run 构建并运行

如果想手动构建镜像并启动容器：

```bash
# 构建镜像
docker build -t voce:latest .

# 运行容器
docker run -d \
  --name voce-server \
  -p 7001:7001 -p 7002:7002 \
  -v $(pwd)/configs/config.yaml:/app/config.yaml:ro \
  -v $(pwd)/configs/workflows:/app/configs/workflows \
  voce:latest
```

---

## 4. 修改编排配置 (Web UI)

服务成功启动后（本地 7001 端口），访问 [http://localhost:7001](http://localhost:7001)。

- **功能**：可视化修改节点、配置插件参数。
- **存储**：所有的编排修改会自动落盘进入 `configs/workflows` 目录。

---

## 5. 启动客户端 (TUI 对话)

打开另一个终端运行交互式 TUI 客户端，连接服务端开启对话：

```bash
# 启动终端交互界面
./bin/voce-tui
```

> **提示**：连接建立时，支持通过 JSON 配置动态覆盖 `plugin` 预设。
