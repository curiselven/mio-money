# Mio Money

Money in One：把散落在券商、支付宝、且慢、有知有行、雪球等各处的家庭资产汇到一处，并通过 MCP 让用户自己的 AI Agent 读写和分析。

- 后端 Go，桌面端 Wails v3

## 开发

```sh
go test ./...
go run ./cmd/mio --db /tmp/mio.db
```

默认数据库在 `~/Library/Application Support/MioMoney/mio.db`（Linux 为 `~/.config/MioMoney/mio.db`），`--db` 可覆盖。异常阈值默认 20%，用 `--max-change` 调整。

## MCP

默认使用 stdio。构建后把可执行文件路径配进 Agent：

```sh
go build -o bin/mio ./cmd/mio
```

```json
{
  "mcpServers": {
    "mio": {
      "command": "/绝对路径/bin/mio"
    }
  }
}
```

接口定义见 `docs/mcp-v1.md`，选型理由见 `docs/adr/0001-tech-stack.md`。

想直接看 MCP 在 stdin/stdout 上来回的 JSON，运行 `go run ./examples/stdio-demo`：它会在临时目录构建服务、建临时库，依次演示建账户、提交快照、确认和读取净资产，结束后清理。

### HTTP

同一个二进制加 `--http` 可提供本机 Streamable HTTP MCP，供其他机器上的 Agent 使用：

```sh
MIO_HTTP_TOKEN='至少 24 个字符的随机 token' go run ./cmd/mio --http \
  --addr 0.0.0.0:7420 --allow '192.168.1.0/24' --session-idle 30m
```

`--addr` 默认 `127.0.0.1:7420`，`--allow` 默认只允许回环地址 `127.0.0.1/32,::1/128`。要从其他机器访问，需同时监听非回环地址并把对方网段加入 `--allow`。每个请求都要带 `Authorization: Bearer <token>`；来源 IP 只取 TCP 对端地址，不信任 `X-Forwarded-For`。token 从环境变量 `MIO_HTTP_TOKEN` 读取，为空或短于 24 个字符时拒绝启动。

`--session-idle` 默认 `30m`，必须大于 0。会话闲置超过该时长后自动过期，客户端下次请求会重新握手。

MCP 客户端配置示例：

```json
{
  "mcpServers": {
    "mio": {
      "url": "http://192.168.1.10:7420",
      "headers": {
        "Authorization": "Bearer 至少24个字符的随机token"
      }
    }
  }
}
```

日志只写 stderr，不记录 token。`launchd` 开机自启留作后续步骤。
