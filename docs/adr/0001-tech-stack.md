# ADR-0001：V1 后端技术栈

状态：已接受（2026-11-18）

## 背景

Mio V1 只做三件事：存数据、校验、计算。采集交给用户自己的 Agent，通过 MCP 读写。仓库会开源，因此不能引入需要真实账号、私有服务或平台预览版的东西。

## 决定

### Go 1.26，单模块

后端、MCP 服务、将来的桌面端后端共用一个 Go 模块。V1 不拆模块，避免过早引入发布和版本管理成本。模块路径用仓库地址 `github.com/curiselven/mio-money`，保证开源后可以直接 `go install`。

### SQLite：modernc.org/sqlite

- 纯 Go，不需要 cgo。用户下载即用，不用装 Xcode 命令行工具或 GCC。
- 单文件数据库适合桌面应用，备份就是复制一个文件。
- 单进程、低并发场景，SQLite 足够；WAL 模式保证读写不互相阻塞。
- 不用 ORM：schema 和查询都是显式 SQL，开源后容易审计，也避免 ORM 与手工迁移两套模型。

### 手写 SQL 迁移 + schema_version

每个迁移是 `internal/store/migrations/NNNN_name.sql`，按文件名顺序执行，整个文件在一个事务里。`schema_version(version, applied_at)` 记录已执行版本。V1 不引入迁移框架：只有一张表、一个版本，框架带来的依赖和魔法大于收益。

### 金额与份额

- 金额一律 `int64` 分。JSON 和数据库都不出现浮点数。
- 份额用十进制字符串，写入时校验为普通十进制（不接受科学计数法），比较时用 `math/big.Rat` 归一化，避免 `0.1 + 0.2` 类误差。
- 币种不换算。V1 不接汇率，`get_net_worth` 的汇总假设同一币种；跨币种账户属于后续版本。

### 只存快照

持仓只记录「某账户在某天持有什么」，不存交易流水。前后两次快照对比得到交易，减少采集端要求，也避免券商流水格式差异。

负债账户同样走快照：`cash_cents` 填负数。净资产按「资产 − 负债」展示，读取时把负债的绝对值放进 `liabilities_cents`。

### 两步写入

`submit_snapshot` 只写 `pending` 快照并返回差异预览；`confirm_snapshot` 才把状态改成 `confirmed`。读取类工具只认 `confirmed`，所以 submit 永远不会影响正式数据。

撤销不是删除：把最近一次确认的快照状态改为 `undone`，上一条确认快照自然重新成为「当前」。`audit_log` 记录 submit / confirm / reject / undo，保留操作痕迹。

### 异常规则

四条规则在 confirm 时统一检查，submit 时只做预览提示：

1. 账户总市值相对上一次确认快照变化超过阈值（默认 20%，`--max-change` 可调）。负债总额取绝对值计算比例。
2. 上一次的持仓条目在本次完全消失。持仓显式写 `quantity: "0"` 视为清仓：预览会标注，允许 confirm；只有整条不出现才拦截。
3. 份额或市值为负数。
4. 快照币种与账户币种不一致。

文档里的快照和持仓条目都没有币种字段，但规则 4 需要数据来源。因此给快照加了一个可选 `currency`：留空时继承账户币种，只有显式写入不同币种才会触发。这是对文档的最小补充，不改变已有字段含义。

阈值只放在进程启动参数里，不落库：当前只有本地单用户，调整阈值重启 MCP 即可；将来桌面端需要在线调整时再进设置表。

### MCP：官方 Go SDK，只做 stdio

- `github.com/modelcontextprotocol/go-sdk v1.8.0`，钉死版本。官方维护，协议升级跟随上游，不用自己实现 JSON-RPC 和 schema。
- V1 只做 stdio：不开端口、不需要 token，Agent 配置一行命令即可。`--http` 按文档放到第二步。
- 工具输入输出用结构体 + `jsonschema` tag，SDK 自动生成 schema、校验参数、序列化结果，工具实现只关心 domain 调用。

### 数据库位置

默认 `os.UserConfigDir()/MioMoney/mio.db`：

- macOS：`~/Library/Application Support/MioMoney/mio.db`
- Linux：`~/.config/MioMoney/mio.db`

`--db` 可覆盖。数据库、日志、测试数据一律不进仓库；示例只用假数据。

## 后果

- 单连接（`SetMaxOpenConns(1)`）串行化写入，V1 足够；将来桌面端并发变高时再调整连接池和事务边界。
- `get_history` 的无账户模式对每个日期重算一次净资产，账户数量大时会变慢；V1 数据量小，先保持实现简单。
- 跨币种净资产尚不支持，`get_net_worth` 不做汇率折算。
- 异常拦截后没有桌面端可去，V1 只能拒绝；桌面端确认入口在下一个阶段补齐。
