# Mio 设计规范

适用于桌面端 `cmd/mio-desktop/assets/`。新页面和改版页面必须按此实现；颜色、间距、字号、圆角、阴影只用 `style.css` 的 `:root` CSS 变量，不在组件里写死值。

## 目录

- [气质与原则](#气质与原则)
- [Token](#token)
- [组件](#组件)
- [页面布局](#页面布局)
- [文案语气](#文案语气)
- [落地检查](#落地检查)

## 气质与原则

- 温暖、安静、可信：米白纸张底、陶土橙强调、墨色文字。不用纯白大面积铺底，不用高饱和渐变。
- 信息密度优先：金额和表格是主角；装饰让位于可读性。
- 层级靠字体粗细、留白和细边框区分，不靠大色块。
- 危险、警告、成功只在语义明确时出现；同屏最多一个主按钮。
- 动效只用于状态变化（展开、加载、提示），不做装饰性动画。

## Token

所有 token 定义在 `cmd/mio-desktop/assets/style.css` 的 `:root`，本节与之逐项对应。新增 token 时同步更新这里。

### 颜色

| Token | 值 | 用途 |
|---|---|---|
| `--bg` | `#f5eee3` | 页面底色 |
| `--paper` | `#fffbf5` | 面板、卡片底色 |
| `--white` | `#fff` | 输入框、主按钮文字 |
| `--ink` | `#3d3329` | 正文 |
| `--ink-strong` | `#2e251c` | 标题、强调数字 |
| `--ink-secondary` | `#5a4a3a` | 次按钮文字 |
| `--ink-warm` | `#7a6a58` | 弱化说明、明细行文字 |
| `--muted` | `#8d7b66` | 次要信息 |
| `--faint` | `#a08f7c` | 最弱信息、占位 |
| `--line` | `#efe3d3` | 常规分隔线 |
| `--line-soft` | `#f3eadd` | 表格行、卡片内分隔线 |
| `--border-strong` | `#e2d3bf` | 输入框、次按钮边框 |
| `--border-warn` | `#e8c79a` | 警告提示、警告按钮边框 |
| `--border-danger` | `#ebc6bc` | 错误提示、危险操作边框 |
| `--border-topbar` | `#e8dccb` | 顶栏下边框 |
| `--clay` | `#c8754b` | 品牌强调、主按钮、链接 |
| `--clay-soft` | `#fce3d3` | 强调底色、焦点外圈 |
| `--clay-hover` | `#b8663e` | 主按钮悬停 |
| `--clay-strong` | `#a9552f` | 选中态文字 |
| `--clay-focus` | `rgba(200, 117, 75, .22)` | 输入框焦点外圈 |
| `--green` | `#5e8c55` | 正向金额、成功 |
| `--red` | `#b4533f` | 负向金额、错误、危险 |
| `--amber` | `#c98a2b` | 警告强调 |
| `--amber-bg` | `#fff1c2` | 警告徽标、调整行底色 |
| `--amber-ink` | `#8a5a20` | 警告文字 |
| `--amber-deep` | `#7a5410` | 深警告文字 |
| `--amber-dark` | `#6b4a0e` | 调整行文字 |
| `--amber-bright` | `#9a6a12` | 调整备注 |
| `--gold` | `#d4a64a` | 占比条、辅助图表 |
| `--gold-line` | `#d9922e` | 调整行左侧标记 |
| `--stock` | `#e07a5f` | 待办计数徽标 |
| `--surface-muted` | `#f3eadd` | 次按钮、标签底色 |
| `--surface-soft` | `#fffdf9` | 更亮的面板/分组底 |
| `--surface-warn` | `#fff8e7` | 警告块底色 |
| `--surface-warn-soft` | `#fff1dc` | 轻警告条底色 |
| `--surface-danger` | `#fbede7` | 错误块底色 |
| `--surface-success` | `#eef5e8` | 成功徽标底色 |
| `--surface-note` | `#fcf3ea` | 添加行/注释底色 |
| `--hover-row` | `#fbf3e8` | 表格行悬停 |
| `--hover-sub` | `#faf3e8` | 明细行、统计块底色 |
| `--hover-soft` | `#ede0d0` | 次按钮悬停 |
| `--chart-grid` | `#f0e6d8` | 图表网格线 |

金额语义：正数用 `--green`，负数用 `--red`，借款/负债金额用 `--red`；中性金额用 `--ink`。

### 字体与字号

| Token | 值 | 用途 |
|---|---|---|
| `--font-sans` | 系统无衬线（PingFang SC 等） | 全部正文 |
| `--font-mono` | SF Mono / JetBrains Mono 等 | 金额、代码、日期、表格数字 |
| `--text-xs` | `10.5px` | 徽标、表格表头 |
| `--text-sm` | `12px` | 辅助文字、表单控件 |
| `--text-md` | `13px` | 页面正文 |
| `--text-lg` | `15px` | 卡片/分组标题 |
| `--text-xl` | `20px` | 次级大数字 |
| `--text-2xl` | `30px` | 首页核心数字 |

历史样式里仍有 `11px`、`11.5px`、`12.5px` 等字号，改版时向上述 scale 收敛，不要新增历史值。

### 间距

以 4px 为步进，半档用 `-5` 后缀（`--space-3` = 12px，`--space-2-5` = 10px）。

| Token | 值 | Token | 值 |
|---|---|---|---|
| `--space-0-5` | `2px` | `--space-3-5` | `14px` |
| `--space-1` | `4px` | `--space-4` | `16px` |
| `--space-1-5` | `6px` | `--space-4-5` | `18px` |
| `--space-2` | `8px` | `--space-5` | `20px` |
| `--space-2-5` | `10px` | `--space-5-5` | `24px` |
| `--space-3` | `12px` | `--space-6` | `28px` |
|  |  | `--space-7` | `36px` |

现有样式仍有少量 `1px/3px/5px/7px/9px` 的历史微调值；新代码不新增这类散值，改版时按相近 token 收敛。

### 布局列宽

| Token | 值 | 用途 |
|---|---|---|
| `--account-snapshot-col` | `156px` | 账户管理快照列：标签、金额、日期 |
| `--account-action-col` | `120px` | 账户管理操作列：编辑、新建组合 |

### 圆角

| Token | 值 | 用途 |
|---|---|---|
| `--radius-sm` | `8px` | 小标记 |
| `--radius-md` | `10px` | 按钮、输入框、提示块 |
| `--radius-lg` | `12px` | 分组卡片 |
| `--radius-xl` | `16px` | 页面面板 |
| `--radius-pill` | `999px` | 徽标、计数 |

### 阴影与焦点

| Token | 值 | 用途 |
|---|---|---|
| `--shadow-1` | 柔和双层阴影 | 面板 |
| `--shadow-2` | 陶土色投影 | 主按钮 |
| `--focus-ring` | 陶土色 3px 外圈 | 键盘焦点补充 |

键盘焦点必须有可见样式：按钮/输入框用 `outline: 2px solid var(--clay-focus)` 配 `border-color: var(--clay)`；文字按钮用 `outline` + `outline-offset`。

## 组件

### 卡片与面板

- `.panel` / `.card`：`--paper` 底、`--line` 边框、`--radius-xl`、`--shadow-1`。
- 面板头部 `.ph`：左侧 `<h2>`，右侧状态文字；标题 `--text-lg`/600，状态 `--text-sm`/`--muted`。
- 页面头部 `.page-head`：左侧 `.eyebrow` 短语 + `<h1>` + `.page-sub` 说明，右侧 `.page-note` / `.page-badge`。用于「导入」「账户管理」「待确认」这类一级页面。
- 组内卡片 `.efund-group`、`.account-block`：`--surface-soft` 底、`--radius-lg`、内边距 `--space-3`。
- 同层级面板间距 `--space-3`；页面区块间距 `--space-3` 到 `--space-4`。

### 按钮

基础类 `.btn`：次按钮。主操作加 `.pri`；危险操作加 `.danger`；轻警告加 `.warn`；紧凑场景加 `.small`。

| 类型 | 类名 | 视觉 |
|---|---|---|
| 主按钮 | `.btn.pri` | `--clay` 底、白字、`--shadow-2` |
| 次按钮 | `.btn` | `--surface-muted` 底、`--border-strong` 边、`--ink-secondary` 字 |
| 危险按钮 | `.btn.danger` | `--red` 底、白字 |
| 轻警告 | `.btn.warn` | 透明底、`--border-warn` 边、`--amber` 字 |
| 文字按钮 | `.btn.text` | 透明底、无边框、`--clay` 字；用于「查看」「去待确认」等行内跳转 |
| 禁用 | `[disabled]` | `opacity: .5`，去掉阴影，保留占位和文案 |
| 加载中 | `[disabled]` + 文案 | 文案改为「正在解析…」「正在导入…」，禁止重复点击；当前用业务 busy 标志实现 |

- 同一操作区只有一个 `.pri`。
- 按钮文案用动词开头：「保存映射」「导入并确认」「回首页」「再导入一个文件」。
- 破坏性操作需要二次确认（参考账户归档的「再点一次确认」）。

### 表单控件

- 输入框/下拉：`.in`，或表单内的 `input/select`；`--white` 底、`--border-strong` 边、`--radius-md`、高度 30px、`--text-sm`。
- 多行文本：`.reason`，最小高度 70px，可纵向拉伸。
- 标签在控件上方，间距 `--space-1`；说明文字 `.hint`/`.efund-hint` 用 `--faint`。
- 焦点：`outline: 2px solid var(--clay-soft)` + `border-color: var(--clay)`。
- 错误文案就近显示，颜色 `--red`，不要只靠边框颜色表达。

### 表格与金额

- 表格：`.accounts-table`、`.holdings`、`.schedule-table`、`.reconcile-table`。
- 表头 `--text-xs`、`--muted`、右对齐；首列或名称列用 `.l` 左对齐。
- 行使用 `--line-soft` 下边框；悬停 `--hover-row`；明细行 `--hover-sub` + `--ink-warm`。
- 金额/份额/日期用 `.num` 或 `.mono`：等宽字体 + `font-variant-numeric: tabular-nums`，表格内右对齐。
- 金额统一走 `money(cents)`：`Intl.NumberFormat("zh-CN", { style: "currency", currency: "CNY" })`，固定两位小数，带 `¥` 和千分位。
- 正负号：`signedMoney()` 用于变动额；颜色类 `.pos`/`.neg`，不要只靠颜色区分，符号也要保留。

### 首页账户表

- 占比列固定为「占总资产」：账户树最新已确认快照合计 ÷ 总资产。负债账户用负数，表示负债相当于总资产的比例。
- 父账户行显示整棵账户树的合计和占比，只计一次；展开后的组合行只显示自己的金额与日期，占比显示「—」，避免父子数字重复相加。
- 归档账户不出现在首页，也不计入总资产、净资产和资产配置。
- 每个账户取自己最新的已确认快照；父账户行日期取树内最新日期。

### 账户管理行

- 账户卡片头部 `.account-block-head` 与组合行 `.portfolio-list li` 共用三列网格：`minmax(0, 1fr)` + `--account-snapshot-col` + `--account-action-col`。
- 快照列固定宽度、右对齐，始终显示标签、金额、日期；组合行的金额和日期也放在这一列，不拼进名称文本，避免随标题、徽标、归属或按钮数量左右漂移。
- 金额和日期用 `.num`；未知值写「—」，仍占快照列。
- 920px 以下改为单列，左侧内容、快照和操作依次左对齐。
- 组合的归属跟随上级账户，列表和卡片一律显示上级的有效归属；上级为空时才显示「未归属」。
- 编辑组合时归属不可编辑，用只读说明「归属：跟随账户（X）」代替归属输入框；编辑账户时仍可修改归属，保存后级联到名下所有组合。

### 标签与徽标

- `.tag`：账户类型等中性标签，`--surface-muted` 底、`--radius-pill`。
- `.efund-tag`：导入状态标签，`--clay-soft` 强调底；变体：
  - `.muted-tag`：已忽略、历史状态。
  - `.warn-tag`：待确认、未映射、失败。
  - `.ok-tag`：`--surface-success` 底 + `--green` 字，用于「✓ 已导入」。
- `.badge`：块级状态徽标（如异常规则），`--amber-bg` 底。
- 徽标文案尽量 2–5 个字，不用句号。

### 折叠列表

- 账户明细：整行可点，同时提供真实 `<button class="account-toggle" aria-expanded="true|false">`。
- 指示符：收起 `▸`，展开 `▾`；指示符与文字间距 `--space-1-5`。
- 两级结构按「账户 → 组合 → 明细」逐级展开；父级收起时子级和子级明细都不显示，但保留本次会话的展开状态。
- 键盘：按钮必须可用 Tab 聚焦，Enter/Space 切换；`aria-expanded` 同步。
- 其他可折叠内容用原生 `<details class="efund-details">` + `<summary>`，默认收起。

### 步骤条

用于多步流程（选文件 → 预览映射 → 完成）。最多四步；导入页当前使用三步：

```html
<ol class="steps" aria-label="导入步骤">
  <li class="done"><span class="step-no">1</span>选择文件</li>
  <li class="on" aria-current="step"><span class="step-no">2</span>预览映射</li>
  <li><span class="step-no">3</span>完成</li>
</ol>
```

- 横向排列，序号圆点用 `--radius-pill`，连接线 1px `--line`；外层是 `--paper` 面板。
- 已完成：`--green` 序号 + `--surface-success` 底；当前：`--clay` 序号底 + 白字；未开始：`--muted` 文字 + `--line-soft` 底。
- 当前步骤必须有 `aria-current="step"`；步骤名使用短动词短语。

### 页面与表单分组

- 导入文件区 `.efund-upload-body`：左侧文件选择、可选密码、右侧解析按钮；字段标签 `.field-label` 在控件上方。
- 摘要数字 `.import-summary`：四列 `.summary-item`，标签 `.summary-label`，主值 `.summary-value`，补充 `.summary-note`。
- 账户表单 `.account-form`：两列网格；字段多时用 `.account-form.wide` 四列。按钮统一放在 `.form-actions`，右对齐。
- 账户管理的新建账户、新建组合、编辑表单默认收起，同一时间只开一个；由对应按钮打开，取消或保存成功后收起。编辑打开时保留滚动定位和「正在编辑：…」提示。
- 账户管理行展示类型、归属成员（空值写「未归属」）、组合数量、最新快照合计与日期、归档状态；未知值写「—」。没有组合时不显示数量文案，也不渲染空的组合列表。顶层账户显示账户树合计，组合行显示自己的快照。

### 提示、空状态与加载

| 状态 | 组件 | 规则 |
|---|---|---|
| 成功 / 一般提示 | `.status`（默认） | 顶部暖色轻提示条，文案确认结果；导入完成用面板汇总，不靠一闪而过的 toast |
| 错误 | `.status.error`、`.efund-issue`、`.efund-block` | `--surface-danger`/`--red`；文案说明原因和下一步 |
| 信息 | `.efund-warning.info` | `--surface-note`/`--line`/`--ink-warm`；小标签「提示」 |
| 警告 | `.efund-warning`、`.efund-tag.warn-tag` | `--surface-warn`/`--border-warn`/`--amber-deep`；小标签「注意」 |
| 阻止写入 | `.efund-warning.blocks_write` | `--surface-danger`/`--border-danger`/`--red`；小标签「将阻止写入」 |
| 成功徽标 | `.efund-tag.ok-tag` | 「✓ 已导入」 |
| 空状态 | `.empty` | 居中、`--muted`、一句话说明为什么空、下一步做什么 |
| 加载 | disabled 控件 + `.status` | 文案「正在解析文件…」「正在导入…」；按钮保持宽度，避免跳动 |

导入告警行使用 `.efund-warning-label` 显示严重程度，不显示 `[info]`、`[warning]` 等原始标记。
顶部 `.status` 普通提示 4 秒后自动隐藏，加载提示保留到结果替换，错误保留到下一次操作或切换页面；导入完成态用面板汇总，不再重复横幅。

### 导入完成态

- 所有组已生效、进入待确认或已忽略后，导入页切换为「导入完成」视图。
- 摘要四项：已生效组数与金额合计、待确认组数、已忽略组数、之前已导入/失败组数。
- 已生效行提供「撤销」（按 snapshot）；之前已导入且来源为 `efund_import` 的已确认行提供「撤销之前导入」，已有快照仍待确认时改为提示「这次导入还在待确认里」，其他来源只说明不在这里撤销；待确认行提供「去待确认」。
- 底部操作：`回首页`、有待确认时的 `去待确认`、`再导入一个文件`。
- 离开导入页、回首页或再导入时，清空文件、密码、预览 token 与会话结果。

## 页面布局

- 顶栏 `.topbar`：固定 52px，左侧品牌，中间导航，右侧全局状态。导航项选中用 `--clay` 下边框。
- 主内容 `.wrap`：最大宽度 1100px，水平居中，内边距 `--space-4 --space-5 --space-6`。
- 首页：KPI 四列 `.kpis`，下接图表 + 配置 `.row2`，再接账户表 `.table-wrap`。
- 双栏 `.row2`：主栏 `minmax(0, 1fr)`，侧栏 330px；920px 以下变单列。
- 桌面宽度优先；620px 以下表格允许横向滚动，按钮换行，表单单列。
- 页面内区块按「概览 → 明细 → 操作」排列，主操作放区块末尾或右上。

## 文案语气

- 中文、短句、直接说结果：「已保存」「映射未保存：…」「该账户已有同日期快照，已跳过」。
- 动词开头，不用「请点击下方按钮」这类废话。
- 错误文案包含原因和下一步；不暴露堆栈、路径、密码、证件信息。
- 金额、日期、数量用统一格式；未知值用「—」。
- 不用感叹号，不用夸张词（「超级」「重磅」）；确定的事用确定语气。
- 面向用户说「账户」「组合」「快照」「待确认」，不引入内部字段名。

## 落地检查

- 新代码不出现十六进制颜色、`rgb()/rgba()`、裸 `px` 间距和字号；先查本文件是否已有 token，没有再扩展 `:root` 并同步本文档。
- 金额走 `money()`/`signedMoney()`，用 `.num` 等宽右对齐。
- 可点击的非按钮元素必须有键盘等价物和可见焦点。
- 新逻辑能抽成纯函数时，放到 `assets/*.mjs` 并用 `node assets/*.test.mjs` 覆盖。
- 改版前后用 [`scripts/ui-preview`](../scripts/ui-preview/README.md) 的虚构数据预览对比截图；除明确要求外，视觉应保持稳定。
