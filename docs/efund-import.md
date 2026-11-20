# 基金 E 账户导出导入（V1：只解析和预览）

V1 读取「基金E账户App投资者公募基金持有信息」，按「销售机构 + 交易账户」归组，并和 Mio 已有快照做只读对比。**不会写数据库。**

## 输入文件

- 可以直接传 `.xlsx`，也可以传邮件里的外层 `.zip`。zip 内必须恰好有一个 `.xlsx`；多个会报错。
- 外层 zip 支持 WinZip AES 加密。密码优先读环境变量 `MIO_EFUND_PASSWORD`；没有时在终端无回显提示。stdin 不是终端时只接受环境变量。密码不会出现在日志、错误或 JSON 里。
- 解压只在内存中进行，不落临时文件。
- 使用单 sheet，第一张 sheet 即视为持有信息。
- 第 1 行标题、第 2–3 行姓名/证件信息会被忽略：不进入结构、不打印、不写日志或 JSON。
- 表头按名称定位，忽略所有空白；`资产情况（结算币种）` 中的换行不影响匹配。
- 基金代码保持字符串（保留前导 0）。金额按整数分解析，份额/净值用十进制字符串和 `math/big.Rat` 比较，不使用浮点。
- 支持序号为 `1`、`1.0`、`1E0` 等 Excel 数值单元格；非整数或非数字行视为数据结束。
- QDII 的净值日期可能早于份额日期，预览按份额日期归组并提示不一致的基金只数。

## 映射配置

默认映射路径与数据库同目录：

- macOS 默认数据库：`~/Library/Application Support/MioMoney/mio.db`
- 默认映射：`~/Library/Application Support/MioMoney/efund-map.json`
- Linux 默认映射：`~/.config/MioMoney/efund-map.json`

每个「销售机构 + 交易账户」独立归组，绝不合并不同交易账户。`account_id` 有值时优先按账户 ID 匹配；ID 找不到会告警并当作新组合，不会回退到名称匹配。没有 `account_id` 时才按 `account` 名称匹配。`account` 和 `account_id` 都为空表示未映射，组名显示为 `<销售机构>·<交易账户>·未分组`。

```json
{
  "mappings": [
    {
      "sales_org": "示例基金销售有限公司",
      "trade_account": "T0000001",
      "account": "长赢计划",
      "account_id": "",
      "hint": {
        "fund_count": 3,
        "market_value_yuan": "1210.00",
        "top_funds": ["示例成长混合", "示例价值混合", "示例新基金"]
      }
    }
  ]
}
```

`hint` 是只读提示，映射匹配时忽略。配置文件不进仓库；仓库只提供虚构示例 [`examples/efund-map.example.json`](../examples/efund-map.example.json)。

## 用法

```sh
go build -o bin/mio ./cmd/mio

# 1. 生成映射骨架（默认写到数据库同目录的 efund-map.json）
./bin/mio import efund --emit-map 持有信息.xlsx
./bin/mio import efund --emit-map --map /path/to/efund-map.json 基金导出.zip

# 2. 编辑 account 或 account_id；已有条目保留，补新增条目
./bin/mio import efund --emit-map --force 持有信息.xlsx   # 整体覆盖

# 3. 预览（读默认映射；也可以显式指定）
./bin/mio import efund --preview 持有信息.xlsx
./bin/mio import efund --preview 基金导出.zip --map /path/to/efund-map.json --json
```

`--emit-map` 默认只合并：已有条目的 `account`/`account_id` 原样保留，`hint` 刷新；当前导出里不存在但配置里已有的条目也会保留。`--force` 才整体覆盖。文件以 `0600` 权限原子写入（临时文件 + rename）。

预览包含：

- 文件元信息：份额日期、打印时间、总记录数校验、净值日期不一致数量。
- 按组合汇总：组合名、对应 Mio 账户或「新组合」、基金只数、市值。
- 总计，以及醒目的未映射交易账户列表。
- 校验告警。
- 已关联账户若存在最近一次确认快照，复用 `domain.ComputeDiff` 展示新增、消失/清仓、份额变化和市值变化。

数据库以只读方式打开；数据库不存在时仍可预览，只是没有快照对比。`--json` 输出与文本预览相同的信息，便于其他工具消费。

## 告警级别

| 级别 | 含义 |
|---|---|
| `info` | 正常提示，不影响写入，例如 QDII 净值日期与份额日期不一致 |
| `warning` | 需要人工确认，例如金额与份额×净值相差超过 1 分、非人民币币种、映射账户 ID 不存在 |
| `blocks_write` | 数据不完整，未来写入步骤必须阻止，例如总记录数不符或缺失 |

具体告警：

- `record_count_mismatch` / `record_count_missing`：`blocks_write`。预览文本会在告警后注明「写入时将被阻止」。
- `value_mismatch`：`warning`。按四舍五入到分比较，允许 1 分误差；超过 1 分告警。
- `currency`：`warning`。目前不换算非人民币币种。
- `share_date_mismatch`：`warning`。数据行份额日期不一致，按最新日期归组。
- `nav_date_mismatch`：`info`。QDII 常见。
- `account_id_not_found`：`warning`。映射 ID 不在库中，按新组合处理。
- `account_name_ambiguous`：`warning`。库中有多个同名账户，不自动关联。

## 暂未实现

- 写入快照、待确认/确认流程；下一步会复用现有 `submit_snapshot` / `confirm_snapshot` 模型。
- 调仓提醒。
