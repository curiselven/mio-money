# 基金 E 账户导出导入（V1：只解析和预览）

V1 读取「基金E账户App投资者公募基金持有信息」xlsx，按「销售机构 + 交易账户」归组，并和 Mio 已有快照做只读对比。**不会写数据库。**

## 输入文件

- 只接受已经解密的 `.xlsx`；原始 AES 加密 zip 尚未支持。
- 使用单 sheet，第一张 sheet 即视为持有信息。
- 第 1 行标题、第 2–3 行姓名/证件信息会被忽略：不进入结构、不打印、不写日志或 JSON。
- 第 4 行按表头名定位列；`资产情况（结算币种）` 中的换行会被忽略。
- 基金代码保持字符串（保留前导 0）。金额按整数分解析，份额/净值用十进制字符串和 `math/big.Rat` 比较，不使用浮点。
- 数据结束后从尾部读取「总记录数」和「打印时间」。数据行数不一致、份额×净值与金额相差超过 1 分、非人民币币种都会出现在预览告警里。
- QDII 的净值日期可能早于份额日期，预览按份额日期归组并提示不一致的基金只数。

## 映射配置

默认读取数据库同目录的 `efund-map.json`：

- macOS：`~/Library/Application Support/MioMoney/efund-map.json`
- Linux：`~/.config/MioMoney/efund-map.json`

也可以用 `--map` 显式指定。配置文件不进仓库；仓库只提供虚构示例 [`examples/efund-map.example.json`](../examples/efund-map.example.json)。

```json
{
  "mappings": [
    {
      "sales_org": "示例基金销售有限公司",
      "trade_account": "T0000001",
      "account": "长赢计划"
    }
  ]
}
```

`account` 是 Mio 账户名。能匹配到已有账户就显示「已关联」；匹配不到则标记为「新组合」。每个「销售机构 + 交易账户」独立归组，绝不合并不同交易账户。未映射的交易账户归入「<销售机构>·未分组」，并在预览里单独列出交易账户、基金只数和市值。

## 用法

```sh
go build -o bin/mio ./cmd/mio

./bin/mio import efund --preview 持有信息.xlsx
./bin/mio import efund --preview 持有信息.xlsx --map /path/to/efund-map.json --json
./bin/mio import efund --preview 持有信息.xlsx --db /tmp/mio-demo.db
```

预览包含：

- 文件元信息：份额日期、打印时间、总记录数校验、净值日期不一致数量。
- 按组合汇总：组合名、对应 Mio 账户或「新组合」、基金只数、市值。
- 总计，以及醒目的未映射交易账户列表。
- 校验告警。
- 已关联账户若存在最近一次确认快照，复用 `domain.ComputeDiff` 展示新增、消失/清仓、份额变化和市值变化。

数据库以只读方式打开；数据库不存在时仍可预览，只是没有快照对比。`--json` 输出与文本预览相同的信息，便于其他工具消费。

## 暂未实现

- AES 加密 zip 解密（密码为证件号后六位）。
- 写入快照、待确认/确认流程；下一步会复用现有 `submit_snapshot` / `confirm_snapshot` 模型。
- 调仓提醒。
