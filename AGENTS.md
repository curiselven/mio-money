# 项目说明

## 结构

- `cmd/mio/`：CLI。
- `cmd/mio-desktop/`：Wails 桌面端；前端在 `assets/`（`app.js`、`style.css`、`*.mjs`）。
- `internal/domain/`：账户、快照、确认、贷款、对账等领域逻辑。
- `internal/importer/`：基金 E 账户 xlsx/zip 解析与映射。
- `internal/store/`：SQLite 存储与迁移。
- `docs/`：导入说明、ADR、设计规范。

## 检查命令

```sh
gofmt -l .
GOPROXY=off go vet ./...
GOPROXY=off go test ./...
GOPROXY=off go build ./cmd/mio-desktop ./cmd/mio
node --input-type=module --check < cmd/mio-desktop/assets/app.js
node cmd/mio-desktop/assets/account_form.test.mjs
node cmd/mio-desktop/assets/account_tree.test.mjs
node cmd/mio-desktop/assets/efund_flow.test.mjs
```

## 约束

- 只用虚构数据。不要读取、写入或启动真实用户数据：不碰 `~/Library/Application Support/MioMoney`，不启动已安装的 `/Applications/Mio.app`。
- 不 commit、push、merge，除非用户明确要求。
- UI 新页面和改版必须遵循 `docs/design-system.md`；颜色、间距、字号只用 `style.css` 的 `:root` 变量。
- 改业务行为前先确认需求；前端纯逻辑优先抽到 `cmd/mio-desktop/assets/*.mjs` 并加 node 测试。
