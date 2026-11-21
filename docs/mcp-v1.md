# Mio Money V1 · MCP 接口草稿

> 状态：草稿，待确认。2026-11-18。

## 定位

Mio 只负责存数据、校验和计算；识图、读券商这类采集工作交给用户自己的 Agent（Codex、Claude 等），Agent 通过 MCP 读写 Mio。

## 已定的原则

1. **持仓只存快照**：每次记录「某账户在某天持有什么」，不存交易流水；交易由前后两次快照对比得出。
2. **两步写入**：Agent 先 `submit_snapshot` 拿到差异预览，用户在对话里确认后，Agent 再 `confirm_snapshot` 才真正生效。只调用 submit 永远不会改动正式数据。
3. **异常拦截**：命中下列任一规则时，`confirm_snapshot` 拒绝执行，必须由用户在 Mio 桌面端确认：
   - 账户总市值相对上一次变化超过 20%（阈值可配置，先用两周再调）；
   - 上一次有的持仓这次整条消失（显式填写 `quantity: "0"` 视为清仓，不算异常）；
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
| parent_id | string | 否 | 上级账户 id；非空表示这是组合 |
| platform | string | 否 | 平台标识，如 alipay / qieman / broker |
| type | enum | 是 | cash / fund / stock / mixed / liability / other |
| currency | string | 是 | 默认 CNY |
| owner | string | 否 | 我 / 家人 / 共同 |
| archived | bool | 否 | 停用 |

账户最多两级：顶层账户（`parent_id` 为空）下可以挂多个组合，组合不能再有下级。组合（`parent_id` 非空）的 `owner` 始终等于上级账户的 `owner`；新建组合时未显式给出的 `type`/`currency` 继承上级。更新组合时不单独接受 `owner`，修改上级归属会级联到名下组合；`list_accounts` 返回有效归属。

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
| `set_loan_terms(account_id, anchor_date, anchor_principal_cents, annual_rate, method, payment_day, remaining_periods)` | 设置/重设贷款条款；账户必须是 liability。anchor_date 当天没有快照时自动写一条 confirmed 快照 |
| `set_loan_history(account_id, original_principal_cents, disbursement_date?, first_payment_date, total_periods, method, initial_annual_rate, rate_changes?, prepayments?)` | 「从头算」：按首期开始完整重放还款计划；会替换该账户的锚点条款 |

### 读取
| 工具 | 作用 |
|---|---|
| `list_accounts()` | 账户列表及各自最近快照日期 |
| `get_holdings(account_id?)` | 当前（最近一次已确认）持仓，可按账户过滤 |
| `get_net_worth(as_of?)` | 净资产 = 资产 − 负债，按类型和账户分组 |
| `get_history(account_id?, from?, to?)` | 净资产或账户市值的时间序列 |
| `list_pending()` | 待确认的快照 |
| `get_loan(account_id, schedule?)` | 贷款条款 + 当前剩余本金、剩余期数、下期月供/本金/利息、剩余总利息；`schedule:true` 返回未来全部期次 |
| `get_loan_history(account_id, from?, to?, schedule?)` | 完整历史计划的当前状态（含已还本金/利息/合计/提前还款合计）与每期明细；`from`/`to` 按期号（1 起）切片，`schedule:true` 返回全部 |
| `reconcile_loan_history(account_id, samples)` | 只读对账：用银行 App 抄来的几期月供/本金/利息（可选余额）反推利率调整的生效日与当期本金算法，返回每期是否对上、差额和建议后的条款 |

## V1 不做

交易流水、行情抓取、汇率换算、预算记账、云同步。

## 贷款/还款计划（2026-11-19）

负债账户可以带一份还款条款（表 `loans`，一个账户一条）：

| 字段 | 说明 |
|---|---|
| anchor_date | 起算日；起算日之后的扣款日按 payment_day 逐月推算 |
| anchor_principal_cents | 起算日剩余本金（整数分，正数） |
| annual_rate | 年利率字符串，如 `"3.10"` 表示 3.10%，范围 0–30 |
| method | `equal_installment`（等额本息）/ `equal_principal`（等额本金） |
| payment_day | 每月扣款日，1–28 |
| remaining_periods | 起算日之后还剩多少期 |

- 月利率 = 年利率 / 12 / 100，利息与月供都四舍五入到分；等额本息最后一期结清剩余本金，等额本金尾差放最后一期。
- 改利率或提前还款：用当时的剩余本金重新 `set_loan_terms`，覆盖旧条款。
- `mio --loan-sync` 跑一次并按 JSON 输出摘要；stdio MCP 与 `--http` 启动时也会自动跑一次。它把上次已确认快照之后、到今天的每个已过扣款日写成一条 confirmed 快照（`as_of=扣款日`，`cash_cents=-剩余本金`，`source=loan_schedule`），同一 `as_of` 已有快照则跳过，可重复执行；这类快照不经过 20% 异常规则。
- `set_loan_terms` 写入的起算快照同样走 `loan_schedule`；`submit_snapshot` 不接受这个 source。

### 从头算（完整历史）

一个贷款账户同一时间只有一种模式：保存锚点条款会删除完整历史，反之亦然；已写入的快照都保留。

- 每期利息 = 期初剩余本金 × 当期年利率 / 12，四舍五入到分。
- 等额本息在首期和每个利率生效后的第一期，用当时剩余本金、剩余期数、新月利率重算月供；等额本金同样在这些点重算每期本金。
- 利率生效日落在两期之间时，当期按 30 天拆分：新利率天数 = 还款日 − 生效日的实际天数，旧利率天数 = 30 − 新利率天数（新超过 30 取 30、旧取 0）；月供从当期起按新利率重算，当期本金 = 新月供 − 拆分利息。生效日正好是还款日时本期仍全用旧利率，下一期起用新利率。同一期多次调整按时间分段，各段天数合计 30 天。
- 每条利率调整可带 `principal_rule`（只影响等额本息的调整当期）：`new_schedule`（默认，本金 = 新月供 − 新月利率整月利息）、`fixed_payment`（实扣 = 新月供）、`old_schedule`（本金 = 旧月供 − 旧月利率整月利息）。下期起都使用新月供；等额本金忽略该字段。旧数据迁移后统一为 `new_schedule`。
- 提前还款支持 `shorten_periods`（月供不变、缩短期限）和 `reduce_payment`（期限不变、重算月供）；日期在两期之间时在下一期生效，与还款日同日时在当期月供后扣减。
- 可选 `disbursement_date`（放款日）必须早于首期还款日。填了它时，第 1 期利息按「实际天数/360」计算（本金 × 年利率 × 天数 / 360，四舍五入到分）；等额本息第 1 期本金仍按整月规则拆分，所以只有第 1 期还款额变化，之后各期与余额不变。不填则与整月口径一致。

### 贷款对账

`reconcile_loan_history` 不写库。Agent 可以从银行 App 截图读出利率调整前后的几期数字，传入 `samples`：

```json
{"account_id": "acc_...", "samples": [
  {"date": "2026-01-20", "payment_cents": 400000, "principal_cents": 100000, "interest_cents": 300000}
]}
```

对落在利率调整当期的样本，会在原生效日 ±10 天（限制在该期 `prev < d <= cur`）和三种 `principal_rule` 中搜索，优先不改日期、其次用默认算法；多个样本按时间顺序处理，前一处修正会影响后面的余额。返回的建议通过 `set_loan_history` 由用户确认后写入。
- `--loan-sync` 与桌面/MCP 启动时的自动同步会按计划从首期补写已过扣款日的负债快照。

## 已定（2026-11-18）

- **负债也走快照**：type=liability 的账户每次只记当时还欠多少（cash_cents 填负数），和资产用同一套流程；还款计划以后再加。
- **MCP 传输**：同一个程序两种都支持。默认本地 stdio（不开端口、开箱即用）；`--http` 已实现，用 Streamable HTTP 提供本机 HTTP MCP，带 token 和 CIDR 来源限制，供其他机器上的 Agent 使用。`launchd` 开机自启留作后续步骤。
