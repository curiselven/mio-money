# Mio Money V1 · MCP 接口草稿

> 状态：草稿，待确认。2026-11-18。

## 定位

Mio 只负责存数据、校验和计算；识图、读券商这类采集工作交给用户自己的 Agent（Codex、Claude 等），Agent 通过 MCP 读写 Mio。

## 已定的原则

1. **持仓只存快照**：每次记录「某账户在某天持有什么」，不存交易流水；交易由前后两次快照对比得出。
2. **两步写入**：Agent 先 `submit_snapshot` 拿到差异预览，用户在对话里确认后，Agent 再 `confirm_snapshot` 才真正生效。只调用 submit 永远不会改动正式数据。
3. **异常拦截**：命中下列任一规则时，`confirm_snapshot` 拒绝执行，必须由用户在 Mio 桌面端确认：
   - 账户总市值相对上一次变化超过 20%（阈值可配置，先用两周再调）；
   - 上一次有的持仓这次整条消失；
   - 份额或市值为负数；
   - 币种与账户设置不一致。
4. **可撤回**：每次确认都写一条操作日志，`undo_snapshot` 可撤回最近一次确认。
5. **金额用整数「分」存储**，份额用字符串十进制存储，避免浮点误差。

## 数据模型（最小）

### account（账户）
| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| id | string | 是 | 系统生成 |
| name | string | 是 | 如「支付宝」「且慢」「华泰证券」 |
| platform | string | 否 | 平台标识，如 alipay / qieman / broker |
| type | enum | 是 | cash / fund / stock / mixed / liability / other |
| currency | string | 是 | 默认 CNY |
| owner | string | 否 | 我 / 家人 / 共同 |
| archived | bool | 否 | 停用 |

### snapshot（持仓快照）
| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| id | string | 是 | 系统生成 |
| account_id | string | 是 | |
| as_of | date | 是 | 数据对应的日期（不是录入时间） |
| status | enum | 是 | pending / confirmed / rejected / undone |
| source | enum | 是 | screenshot / mcp / statement / manual |
| source_note | string | 否 | 如「投资账本 MCP」「支付宝基金持有页截图」 |
| cash_cents | int64 | 否 | 账户内现金余额 |
| items | list | 是 | 见下表；空列表表示清仓 |
| created_at / confirmed_at | datetime | — | 系统记录 |

### snapshot item（单条持仓）
| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| name | string | 是 | 基金/股票名称 |
| code | string | 否 | 有代码就填，用于和上次对齐 |
| quantity | decimal string | 否 | 份额/股数 |
| market_value_cents | int64 | 是 | 当前市值 |
| cost_cents | int64 | 否 | 持仓成本，拿不到可不填 |
| note | string | 否 | |

对齐规则：先按 code 匹配，没有 code 时按 name 匹配。

## MCP 工具（V1）

### 写入
| 工具 | 作用 |
|---|---|
| `submit_snapshot(account_id, as_of, source, items, cash_cents?)` | 提交一份待确认快照，返回 snapshot_id + 差异预览（新增 / 消失 / 份额变化 / 市值变化 / 总额变化）+ 是否命中异常规则 |
| `confirm_snapshot(snapshot_id)` | 确认生效；命中异常规则时拒绝并提示去桌面端 |
| `reject_snapshot(snapshot_id, reason?)` | 丢弃 |
| `undo_snapshot(account_id)` | 撤回该账户最近一次确认 |
| `upsert_account(...)` | 新建或修改账户 |

### 读取
| 工具 | 作用 |
|---|---|
| `list_accounts()` | 账户列表及各自最近快照日期 |
| `get_holdings(account_id?)` | 当前（最近一次已确认）持仓，可按账户过滤 |
| `get_net_worth(as_of?)` | 净资产 = 资产 − 负债，按类型和账户分组 |
| `get_history(account_id?, from?, to?)` | 净资产或账户市值的时间序列 |
| `list_pending()` | 待确认的快照 |

## V1 不做

交易流水、行情抓取、汇率换算、预算记账、云同步。

## 已定（2026-11-18）

- **负债也走快照**：type=liability 的账户每次只记当时还欠多少（cash_cents 填负数），和资产用同一套流程；还款计划以后再加。
- **MCP 传输**：同一个程序两种都支持。默认本地 stdio（不开端口、开箱即用，V1 先做）；`--http` 开关提供本机 HTTP（带 token 和来源限制，供其他机器上的 Agent 使用，第二步做）。
