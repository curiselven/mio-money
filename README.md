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

V1 只支持 stdio。构建后把可执行文件路径配进 Agent：

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
