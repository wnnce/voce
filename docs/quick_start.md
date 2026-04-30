# 快速开始 (Quick Start)

项目支持两种运行模式：**单机模式 (Standalone)** 和 **集群/网关模式 (Gateway Cluster)**。

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
- **FFmpeg** (libswresample & libavutil)
- **Node.js** & **pnpm** (用于构建前端工作台)
- **Rust** & **Cargo** (用于构建 TUI 客户端)

### 2.2 编译项目

Voce 使用 `Makefile` 管理构建。`make build-all` 会自动构建前端、服务端、网关以及 TUI 客户端：

```bash
make build-all
```

### 2.3 准备配置文件

根据运行模式准备对应的配置：

*   **单机模式 (Standalone)**:
    ```bash
    cp examples/voce-standalone.yaml.example configs/config.yaml
    ```
*   **网关模式 (Experimental)**:
    ```bash
    cp examples/gateway.yaml.example configs/gateway.yaml
    cp examples/voce-gateway.yaml.example configs/voce-gateway.yaml
    ```

### 2.4 启动服务

*   **单机模式**: 
    ```bash
    ./bin/voce -c configs/config.yaml
    ```
*   **网关模式 (Experimental)**:
    需启动两个进程：
    ```bash
    # 终端 1: 启动网关
    ./bin/voce-gateway -c configs/gateway.yaml
    # 终端 2: 启动业务服务
    ./bin/voce -c configs/voce-gateway.yaml
    ```

---

## 3. 方式二：Docker 运行

### 3.1 使用 Docker Compose (推荐)

*   **单机模式**: 
    ```bash
    docker-compose up -d
    ```
*   **网关模式 (Experimental)**: 
    ```bash
    docker-compose -f docker-compose.gateway.yml up -d
    ```

### 3.2 使用 docker run

```bash
# 构建镜像
docker build -t voce:latest .

# 运行单机容器
docker run -d \
  --name voce-server \
  -p 7001:7001 -p 7002:7002 \
  -v $(pwd)/configs/config.yaml:/app/config.yaml:ro \
  -v $(pwd)/configs/workflows:/app/configs/workflows \
  voce:latest
```

---

## 4. 修改编排配置 (Web UI)

访问 [http://localhost:7001](http://localhost:7001)。

- **功能**：可视化修改节点、配置插件参数。
- **存储**：所有的编排修改会自动落盘进入 `configs/workflows` 目录。

---

## 5. 启动客户端 (TUI 对话)

```bash
# 启动终端交互界面
./bin/voce-tui
```

> **提示**：连接建立时，支持通过 JSON 配置动态覆盖 `plugin` 预设。
