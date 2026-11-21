# 基金 E 账户导出导入

桌面端读取「基金E账户App投资者公募基金持有信息」，按「账户 → 组合」映射后走现有快照/待确认流程；CLI 仍只解析和预览。

## 输入文件

- 支持 `.xlsx` 和邮件里的外层 `.zip`；zip 内必须恰好有一个 `.xlsx`。
- 外层 zip 支持 WinZip AES。CLI 优先读 `MIO_EFUND_PASSWORD`，否则终端无回显提示；桌面端在导入页输入密码。
- 密码只用于内存解密，不保存、不进日志/错误/JSON；解压不落临时文件。
- 第 1 行标题、第 2–3 行姓名/证件信息会被忽略：不进入结构、不打印、不写日志或 JSON。
- 表头按名称定位，忽略所有空白；基金代码若为纯数字且不足 6 位会左补 0。
- 支持序号 `1`、`1.0`、`1E0` 等 Excel 数值单元格。
- 外层 zip 最多 1000 个条目、解压后 xlsx 最多 20MB；sheet 最多 100000 行、16384 列。超限直接报错。

## 账户与组合

账户是顶层对象（`parent_id` 为空）；组合挂在账户下（`parent_id` 非空），只允许两级。快照写在组合上；没有组合的账户也可以直接记持仓。

- 已有任何快照的账户不能再创建组合。
- 有子组合的账户不能直接提交快照。
- 导入目标只能是 `fund` 或 `mixed` 类型的账户/组合。
- 组合继承父账户的类型、币种和成员；成员（owner）始终跟随上级账户，不能单独修改。

桌面端「账户管理」可以新建/改名账户，新建/改名组合，以及归档没有快照、没有组合的空对象。归属成员复用 `owner` 自由文本，输入框会给出已有成员选项。新建组合写入上级账户的归属；修改上级账户归属会级联到名下所有组合（包括清空）；组合编辑页只读显示「归属：跟随账户（X）」。

## 映射配置

默认与数据库同目录：

- macOS 默认数据库：`~/Library/Application Support/MioMoney/mio.db`
- 默认映射：`~/Library/Application Support/MioMoney/efund-map.json`
- Linux 默认映射：`~/.config/MioMoney/efund-map.json`

映射含销售机构、交易账号等敏感数据，不要提交进仓库。

`account_id`/`account` 指向最终写入快照的组合或账户；`parent_account` 只用于阅读。`ignore: true` 表示本次忽略。

```json
{
  "mappings": [
    {
      "sales_org": "示例基金销售有限公司",
      "trade_account": "T0000001",
      "account": "示例组合",
      "account_id": "acc_example",
      "parent_account": "示例账户",
      "ignore": false,
      "hint": {
        "fund_count": 3,
        "market_value_yuan": "1210.00",
        "top_funds": ["示例成长混合", "示例价值混合", "示例新基金"]
      }
    }
  ]
}
```

旧映射如果指向没有子组合的顶层账户，仍可直接使用。桌面端选择后会立即创建新账户/组合并保存映射；导入失败不会删除用户主动创建的对象。映射写失败不会阻断导入，预览和导入结果会提示「映射未保存」。

## 桌面应用

```sh
go build -o bin/mio-desktop ./cmd/mio-desktop
./bin/mio-desktop
```

1. 顶部进入「导入 · 基金E账户」，选择 xlsx/zip；zip 输入密码后解析。
   页面步骤条依次显示「选择文件 → 预览映射 → 完成」，完成态收起文件选择区。
2. 页面显示文件总计、已映射小计、已忽略、未映射，以及记录数、告警和每个交易账户明细。
3. 每个交易账户先选账户（已有顶层账户或新建），再选组合（已有组合、新建组合，或账户没有组合时直接记账）。选择/名称确认后自动保存映射并立即创建对象。
4. 已映射/已忽略的组会移到下方并默认折叠；未映射组留在上方。
5. 按目标账户/组合汇总基金只数、市值、差异和异常。多个交易账户映射到同一目标会合并；同一基金代码会合并份额和市值。
6. 文件级 `blocks_write`（记录数不符、非人民币、份额日期不一致）阻止所有导入。组级映射问题只影响该组。
7. 每个已映射目标可「导入并确认这一组」，也可「全部导入并确认」。目标下多个交易账户会一并导入。
8. 正常直接生效；命中异常规则进入「待确认」。同一目标同一份额日期已有任何来源的 confirmed/pending 快照时标记为「之前已导入」并跳过；预览和结果会带已有快照的 id 与来源。
9. 已生效的导入可按本次 snapshot 精确撤销；被撤后快照变为 `undone`。不是账户最新确认快照时会报错，不会撤错。待确认的目标去「待确认」页处理。
10. 「之前已导入」的组：已有快照仍是 pending 时不提供撤销，改为提示「这次导入还在待确认里」并可「去待确认」。已有快照来源是 `efund_import` 且已确认时提供「撤销之前导入」，沿用同一条撤销规则（只撤仍是该账户最新已确认的快照，否则显示原因）；撤销后该组可重新导入。来源是其他快照时不提供撤销，只说明「该日快照来自其他来源，不在这里撤销」。
11. 逐组导入后，已处理组显示「✓ 已导入 / 待确认 / 已忽略 / 之前已导入」，目标按钮变为「撤销」或不可用。所有组都已处理或忽略后，页面进入「导入完成」：摘要显示已生效组数与金额合计、待确认组数、已忽略组数、之前已导入/失败组数；已生效目标可撤销，之前已导入且来源为 `efund_import` 的已确认目标可撤销，已有快照待确认或本次进入待确认的目标可去待确认页；可「回首页」「去待确认」或「再导入一个文件」。

离开导入页、点「回首页」或「再导入一个文件」会清空已选文件、密码输入、预览 token 和本轮导入结果；重新进入导入页是干净初始态。映射本身仍按原规则保存到 `efund-map.json`。

如果目标上一条确认快照有非零现金，导入会沿用该 `cash_cents`，并在 `source_note` 注明「现金沿用上次」。

## 写入规则

- `as_of` 使用文件份额日期。
- `source = efund_import`；`source_note` 记录份额日期、打印时间、交易账户（以及现金沿用）。
- 每条基金一条 item：名称、代码、份额、金额（分），note 记录净值与净值日期；`cash_cents` 为空或沿用上次。
- submit audit detail 记录来源、份额日期、文件打印时间。
- 复用 `domain.Service.Submit` → `Confirm`，不绕过异常规则。
- `snapshots.source` CHECK 通过迁移 `0006_efund_import_source.sql` 增加 `efund_import`。
- 账户层级通过迁移 `0007_account_parent.sql` 增加 `accounts.parent_id`。

## CLI

CLI 只解析和预览，不写数据库。

```sh
go build -o bin/mio ./cmd/mio

./bin/mio import efund --emit-map 持有信息.xlsx
./bin/mio import efund --emit-map --map /path/to/efund-map.json 基金导出.zip
./bin/mio import efund --emit-map --force 持有信息.xlsx

./bin/mio import efund --preview 持有信息.xlsx
./bin/mio import efund --preview 基金导出.zip --map /path/to/efund-map.json --json
```

CLI 预览显示「文件总计 / 已映射小计 / 已忽略 / 未映射」；已忽略的组标记为「已忽略」，不算未映射。

## 告警级别

| 级别 | 含义 |
|---|---|
| `info` | 正常提示，例如 QDII 净值日期与份额日期不一致 |
| `warning` | 需要人工确认，例如金额相差超过 1 分、映射有问题 |
| `blocks_write` | 数据不完整，禁止任何导入，例如记录数不符、非人民币、文件内份额日期不一致 |

具体告警：

- `record_count_mismatch` / `record_count_missing`：`blocks_write`。
- `currency`：`blocks_write`，暂不换汇。
- `share_date_mismatch`：`blocks_write`。
- `value_mismatch`：`warning`，允许 1 分误差。
- `nav_date_mismatch`：`info`。
- `mapping_target_invalid` / `mapping_save_failed`：`warning`。
- `account_id_not_found` / `account_name_ambiguous`：CLI 预览告警。

## 迁移 0006 回退

首选备份恢复。需要手工回退 0006 时，先关闭 Mio 并备份数据库，再在 `sqlite3` 中执行（会删除 `efund_import` 快照）：

```sql
PRAGMA foreign_keys=OFF;
BEGIN;
DELETE FROM snapshot_items WHERE snapshot_id IN (SELECT id FROM snapshots WHERE source='efund_import');
DELETE FROM snapshots WHERE source='efund_import';
CREATE TABLE snapshots_rollback (
  id TEXT PRIMARY KEY,
  account_id TEXT NOT NULL REFERENCES accounts(id),
  as_of TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending','confirmed','rejected','undone')),
  source TEXT NOT NULL CHECK (source IN ('screenshot','mcp','statement','manual','loan_schedule')),
  source_note TEXT NOT NULL DEFAULT '',
  currency TEXT NOT NULL DEFAULT '',
  cash_cents INTEGER,
  created_at TEXT NOT NULL,
  confirmed_at TEXT
);
INSERT INTO snapshots_rollback SELECT id, account_id, as_of, status, source, source_note, currency, cash_cents, created_at, confirmed_at FROM snapshots;
DROP TABLE snapshots;
ALTER TABLE snapshots_rollback RENAME TO snapshots;
CREATE INDEX snapshots_by_account ON snapshots(account_id, status, as_of);
CREATE INDEX snapshots_by_status ON snapshots(status, confirmed_at);
DELETE FROM schema_version WHERE version=6;
COMMIT;
PRAGMA foreign_keys=ON;
```

0007 的 `parent_id` 可被旧代码忽略；彻底回退同样优先恢复备份。

## 暂未实现

- 覆盖已有同日期导入；目前只跳过。
- CLI `--apply`。
- 调仓提醒。
