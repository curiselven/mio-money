# 基金 E 账户导出导入

读取「基金E账户App投资者公募基金持有信息」，按「销售机构 + 交易账户」归组，映射到 Mio 账户后走现有快照/待确认流程。CLI 仍只预览；桌面应用可以直接导入。

## 输入文件

- 可以直接传 `.xlsx`，也可以传邮件里的外层 `.zip`。zip 内必须恰好有一个 `.xlsx`；多个会报错。
- 外层 zip 支持 WinZip AES 加密。CLI 优先读环境变量 `MIO_EFUND_PASSWORD`，没有时在终端无回显提示；桌面应用在导入页输入密码。
- 密码只用于内存解密，不保存、不进日志、错误信息或 JSON。解压只在内存中进行，不落临时文件。
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

每个「销售机构 + 交易账户」独立归组，绝不合并不同交易账户。`account_id` 有值时优先按账户 ID 匹配；没有 `account_id` 时才按 `account` 名称匹配。`account` 和 `account_id` 都为空表示未映射；`ignore: true` 表示本次忽略。

```json
{
  "mappings": [
    {
      "sales_org": "示例基金销售有限公司",
      "trade_account": "T0000001",
      "account": "长赢计划",
      "account_id": "acc_example",
      "ignore": false,
      "hint": {
        "fund_count": 3,
        "market_value_yuan": "1210.00",
        "top_funds": ["示例成长混合", "示例价值混合", "示例新基金"]
      }
    },
    {
      "sales_org": "另一示例基金销售有限公司",
      "trade_account": "T0000009",
      "account": "",
      "account_id": "",
      "ignore": true
    }
  ]
}
```

`hint` 是只读提示，映射匹配时忽略。桌面端保存映射和 CLI `--emit-map` 都使用这个文件，`0600` 权限、临时文件 + rename 原子写。配置文件不进仓库；仓库只提供虚构示例 [`examples/efund-map.example.json`](../examples/efund-map.example.json)。

## 桌面应用

```sh
go build -o bin/mio-desktop ./cmd/mio-desktop
./bin/mio-desktop
```

1. 顶部导航进入「导入」。
2. 选择基金E账户导出的 `.xlsx` 或 `.zip`；zip 会显示密码框。点「解析」。
3. 页面显示文件份额日期、打印时间、记录数校验、告警，以及每个「销售机构 · 交易账户」的基金只数、市值、前几只基金和可展开明细。
4. 每个交易账户选择「已有账户」「新建账户」或「本次忽略」。映射改动可点「保存映射」立即写入，导入时也会自动保存。
5. 页面按账户汇总：多个交易账户映射到同一账户时合并；同一基金代码在不同交易账户出现时也会合并份额和市值，差异计算使用同一份合并后的 items。
6. 预览展示与该账户最近已确认快照的新增/消失/清仓/份额变化/市值变化，以及导入后可能命中的异常规则。
7. 有 `blocks_write` 告警时「导入并确认」按钮禁用；仍有未决定交易账户时会显示原因，点击导入会先保存映射并重新校验，未决定时不会写入。
8. 点「导入并确认」：新建账户此时才创建（`fund`、平台为销售机构、`CNY`），每个目标账户生成一条快照；正常直接确认，命中异常规则进入「待确认」。
9. 导入结果按账户显示已生效、待确认、已跳过、失败。已生效的账户可以「撤销最近确认」，等价于现有 `undo_snapshot`。

同一账户如果已有同份额日期、来源 `efund_import` 的 confirmed 或 pending 快照，默认跳过并提示「已导入过」。V1 不提供覆盖；需要重导时先在桌面端撤销或丢弃原快照。

撤销只撤回该账户最近一次确认的快照，不会删除导入时新建的账户。

## 写入规则

- `as_of` 使用文件的份额日期，不使用今天。
- `source = efund_import`，`source_note` 记录份额日期、打印时间和交易账户。
- 每条基金一条 `item`：名称、代码、份额、金额（分），note 记录净值与净值日期；`cash_cents` 为空。
- 审计的 submit detail 记录 `来源 efund_import；份额日期 …；文件打印时间 …`。
- 写入复用 `domain.Service.Submit` → `Confirm`：异常规则不绕过；命中时留在「待确认」，由现有待确认页处理。
- `snapshots.source` 的 CHECK 约束通过迁移 `0006_efund_import_source.sql` 增加 `efund_import`。

## CLI

CLI 仍只解析和预览，不写数据库。

```sh
go build -o bin/mio ./cmd/mio

./bin/mio import efund --emit-map 持有信息.xlsx
./bin/mio import efund --emit-map --map /path/to/efund-map.json 基金导出.zip
./bin/mio import efund --emit-map --force 持有信息.xlsx   # 整体覆盖

./bin/mio import efund --preview 持有信息.xlsx
./bin/mio import efund --preview 基金导出.zip --map /path/to/efund-map.json --json
```

`--emit-map` 默认只合并：已有条目的 `account`/`account_id`/`ignore` 原样保留，`hint` 刷新；当前导出里不存在但配置里已有的条目也会保留。`--force` 才整体覆盖。

## 告警级别

| 级别 | 含义 |
|---|---|
| `info` | 正常提示，不影响写入，例如 QDII 净值日期与份额日期不一致 |
| `warning` | 需要人工确认，例如金额与份额×净值相差超过 1 分、非人民币币种、映射有问题 |
| `blocks_write` | 数据不完整，禁止写入，例如总记录数不符或缺失 |

具体告警：

- `record_count_mismatch` / `record_count_missing`：`blocks_write`。
- `value_mismatch`：`warning`。按四舍五入到分比较，允许 1 分误差。
- `currency`：`warning`。目前不换算非人民币币种。
- `share_date_mismatch`：`warning`。
- `nav_date_mismatch`：`info`。
- `mapping_account_missing` / `mapping_account_ambiguous`：`warning`。桌面端要求重新选择账户。
- `account_id_not_found` / `account_name_ambiguous`：CLI 预览告警。

## 暂未实现

- 覆盖已有同日期导入；目前只跳过并提示。
- CLI `--apply`。
- 调仓提醒。
