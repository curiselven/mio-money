(() => {
  "use strict";

  const page = new URLSearchParams(window.location.search).get("page") || "home";
  const w = (yuan) => Math.round(yuan * 100);
  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

  async function waitFor(check, timeout = 8000) {
    const started = Date.now();
    while (Date.now() - started < timeout) {
      if (check()) return;
      await sleep(50);
    }
    throw new Error("预览等待超时");
  }

  const overview = {
    as_of: "2026-10-18",
    net_cents: w(1260000),
    assets_cents: w(2546800),
    liabilities_cents: w(1286800),
    change_cents: w(24000),
    change_percent: 1.94,
    pending_count: 3,
    empty: false,
    history: [
      { as_of: "2026-05-18", value_cents: w(1102000) },
      { as_of: "2026-06-18", value_cents: w(1148000) },
      { as_of: "2026-07-18", value_cents: w(1196000) },
      { as_of: "2026-08-18", value_cents: w(1180000) },
      { as_of: "2026-09-18", value_cents: w(1236000) },
      { as_of: "2026-10-18", value_cents: w(1260000) },
    ],
    accounts: [
      {
        account_id: "acc_broker",
        name: "海岬证券",
        type: "stock",
        as_of: "2026-10-18",
        value_cents: 0,
        share_percent: 0,
        items: [],
        parent_id: "",
        has_children: true,
      },
      {
        account_id: "acc_growth",
        name: "长期成长组合",
        type: "stock",
        as_of: "2026-10-18",
        value_cents: w(386520.4),
        share_percent: 30.68,
        items: [
          { name: "虚构科技A", code: "600001", quantity: "4200", market_value_cents: w(226400.4) },
          { name: "虚构宽基ETF", code: "510001", quantity: "125000", market_value_cents: w(160120) },
        ],
        parent_id: "acc_broker",
        has_children: false,
      },
      {
        account_id: "acc_dividend",
        name: "红利组合",
        type: "stock",
        as_of: "2026-10-18",
        value_cents: w(612340.6),
        share_percent: 48.6,
        items: [
          { name: "虚构红利ETF", code: "510002", quantity: "188000", market_value_cents: w(612340.6) },
        ],
        parent_id: "acc_broker",
        has_children: false,
      },
      {
        account_id: "acc_fund",
        name: "基金E账户",
        type: "fund",
        as_of: "2026-10-18",
        value_cents: 0,
        share_percent: 0,
        items: [],
        parent_id: "",
        has_children: true,
      },
      {
        account_id: "acc_fund_core",
        name: "核心基金组合",
        type: "fund",
        as_of: "2026-10-18",
        value_cents: w(1505139),
        share_percent: 119.45,
        items: [
          { name: "虚构成长混合", code: "000001", quantity: "32000.50", market_value_cents: w(684205.5) },
          { name: "虚构稳健债券", code: "000003", quantity: "68000", market_value_cents: w(820933.5) },
        ],
        parent_id: "acc_fund",
        has_children: false,
      },
      {
        account_id: "acc_cash",
        name: "日常现金",
        type: "cash",
        as_of: "2026-10-18",
        value_cents: w(42800),
        share_percent: 3.4,
        cash_cents: w(42800),
        items: [],
        parent_id: "",
        has_children: false,
      },
      {
        account_id: "acc_loan",
        name: "住房贷款",
        type: "liability",
        as_of: "2026-10-18",
        value_cents: w(-1286800),
        share_percent: -102.13,
        items: [],
        parent_id: "",
        has_children: false,
        loan: {
          balance_cents: w(1286800),
          remaining_periods: 214,
          next_date: "2026-11-08",
          next_payment_cents: w(7286.42),
          remaining_interest_cents: w(278615.2),
          paid_periods: 26,
          paid_principal_cents: w(213200),
          paid_interest_cents: w(80300),
          paid_total_cents: w(293500),
        },
      },
    ],
  };

  const accountTree = {
    owners: ["林澈", "周岚"],
    accounts: [
      { id: "acc_broker", name: "海岬证券", type: "stock", currency: "CNY", owner: "林澈", parent_id: "", archived: false, has_snapshots: false, has_children: true },
      { id: "acc_fund", name: "基金E账户", type: "fund", currency: "CNY", owner: "林澈", parent_id: "", archived: false, has_snapshots: false, has_children: true },
      { id: "acc_cash", name: "日常现金", type: "cash", currency: "CNY", owner: "周岚", parent_id: "", archived: false, has_snapshots: true, has_children: false },
      { id: "acc_archive", name: "旧日储蓄", type: "other", currency: "CNY", owner: "林澈", parent_id: "", archived: true, has_snapshots: true, has_children: false },
    ],
    portfolios: [
      { id: "acc_growth", name: "长期成长组合", type: "stock", currency: "CNY", owner: "林澈", parent_id: "acc_broker", archived: false, has_snapshots: true, has_children: false },
      { id: "acc_dividend", name: "红利组合", type: "stock", currency: "CNY", owner: "林澈", parent_id: "acc_broker", archived: false, has_snapshots: true, has_children: false },
      { id: "acc_fund_core", name: "核心基金组合", type: "fund", currency: "CNY", owner: "林澈", parent_id: "acc_fund", archived: false, has_snapshots: true, has_children: false },
    ],
  };

  const emptyDiff = {
    added: [],
    removed: [],
    quantity_changed: [],
    value_changed: [],
    cleared: [],
    cash_before_cents: null,
    cash_after_cents: null,
    total_before_cents: 0,
    total_after_cents: 0,
    total_delta_cents: 0,
  };

  const pending = [
    {
      snapshot_id: "snap_preview_01",
      account_id: "acc_growth",
      account_name: "海岬证券 / 长期成长组合",
      as_of: "2026-10-18",
      source: "efund_import",
      anomalies: [
        { code: "total_change", message: "账户总市值相对上次变化 28.6%，超过阈值 20%" },
        { code: "item_disappeared", message: "上次持有的「虚构消费基金」本次整条消失" },
      ],
      diff: {
        added: [{ name: "虚构新能源基金", code: "000021", quantity: "1200.00", market_value_cents: w(18240) }],
        removed: [{ name: "虚构消费基金", code: "000022", quantity: "3000.00", market_value_cents: w(8420.25) }],
        quantity_changed: [{ name: "虚构成长混合", old_quantity: "8200.00", new_quantity: "9000.00", old_value_cents: w(41000), new_value_cents: w(45000), delta_value_cents: w(4000) }],
        value_changed: [{ name: "虚构宽基ETF", old_value_cents: w(62000), new_value_cents: w(58400.3), delta_value_cents: w(-3599.7) }],
        cleared: [],
        cash_before_cents: null,
        cash_after_cents: null,
        total_before_cents: w(106000),
        total_after_cents: w(128500),
        total_delta_cents: w(22500),
      },
    },
    {
      snapshot_id: "snap_preview_02",
      account_id: "acc_fund_core",
      account_name: "基金E账户 / 核心基金组合",
      as_of: "2026-10-18",
      source: "mcp",
      anomalies: [
        { code: "item_disappeared", message: "上次持有的「虚构稳健债券」本次整条消失" },
      ],
      diff: {
        ...emptyDiff,
        removed: [{ name: "虚构稳健债券", code: "000003", quantity: "68000", market_value_cents: w(820933.5) }],
        total_before_cents: w(1505139),
        total_after_cents: w(684205.5),
        total_delta_cents: w(-820933.5),
      },
    },
    {
      snapshot_id: "snap_preview_03",
      account_id: "acc_cash",
      account_name: "日常现金",
      as_of: "2026-10-17",
      source: "manual",
      anomalies: [{ code: "negative", message: "现金余额为负数" }],
      diff: {
        ...emptyDiff,
        cash_before_cents: w(42800),
        cash_after_cents: w(-1200),
        total_before_cents: w(42800),
        total_after_cents: w(-1200),
        total_delta_cents: w(-44000),
      },
    },
  ];

  function holding(seq, code, name, quantity, nav, cents, navDate = "2026-10-17") {
    return {
      seq,
      fund_code: code,
      fund_name: name,
      fund_account: "F" + code,
      sales_org: "云帆基金销售有限公司",
      trade_account: "90000001",
      quantity,
      share_date: "2026-10-18",
      nav,
      nav_date: navDate,
      market_value_cents: cents,
      currency: "人民币",
      currency_code: "CNY",
      dividend_method: "红利再投资",
    };
  }

  const importAccounts = [
    { id: "fund_top", name: "基金E账户", type: "fund", currency: "CNY", has_children: true },
    { id: "fund_core", name: "核心基金组合", type: "fund", currency: "CNY", parent_id: "fund_top", parent_name: "基金E账户", has_children: false },
    { id: "fund_pending", name: "稳健配置组合", type: "fund", currency: "CNY", parent_id: "fund_top", parent_name: "基金E账户", has_children: false },
    { id: "fund_done", name: "旧配置组合", type: "fund", currency: "CNY", parent_id: "fund_top", parent_name: "基金E账户", has_children: false },
    { id: "fund_legacy", name: "旧日基金账户", type: "fund", currency: "CNY", has_children: false },
    { id: "fund_spare", name: "备用基金账户", type: "fund", currency: "CNY", has_children: false },
    { id: "mixed_top", name: "混合策略账户", type: "mixed", currency: "CNY", has_children: false },
  ];

  const previewGroups = [
    {
      sales_org: "云帆基金销售有限公司",
      trade_account: "90000001",
      name: "林澈",
      mode: "existing",
      account_id: "fund_core",
      account_name: "核心基金组合",
      top_account_id: "fund_top",
      top_account_name: "基金E账户",
      portfolio_id: "fund_core",
      portfolio_name: "核心基金组合",
      direct: false,
      ignored: false,
      fund_count: 3,
      market_value_cents: w(68420.55),
      top_funds: ["虚构成长混合", "虚构宽基ETF", "虚构稳健债券"],
      holdings: [
        holding(1, "000001", "虚构成长混合", "28000.50", "1.2840", w(35952)),
        holding(2, "000002", "虚构宽基ETF", "14500.00", "1.2967", w(18802.15)),
        holding(3, "000003", "虚构稳健债券", "11400.00", "1.1988", w(13666.4)),
      ],
    },
    {
      sales_org: "青禾基金",
      trade_account: "90000002",
      name: "林澈",
      mode: "existing",
      account_id: "fund_spare",
      account_name: "备用基金账户",
      top_account_id: "fund_spare",
      top_account_name: "备用基金账户",
      direct: true,
      ignored: false,
      fund_count: 2,
      market_value_cents: w(15238),
      top_funds: ["虚构红利指数", "虚构货币基金"],
      holdings: [
        holding(4, "000004", "虚构红利指数", "9000.00", "1.0520", w(9468)),
        holding(5, "000005", "虚构货币基金", "5500.00", "1.0491", w(5770)),
      ],
    },
    {
      sales_org: "北岸基金",
      trade_account: "90000003",
      name: "周岚",
      mode: "existing",
      account_id: "fund_pending",
      account_name: "稳健配置组合",
      top_account_id: "fund_top",
      top_account_name: "基金E账户",
      portfolio_id: "fund_pending",
      portfolio_name: "稳健配置组合",
      direct: false,
      ignored: false,
      fund_count: 2,
      market_value_cents: w(41206.66),
      top_funds: ["虚构价值混合", "虚构短债基金"],
      holdings: [
        holding(6, "000006", "虚构价值混合", "20000.00", "1.3512", w(27024)),
        holding(7, "000007", "虚构短债基金", "12000.00", "1.1804", w(14182.66)),
      ],
    },
    {
      sales_org: "旧日平台",
      trade_account: "90000004",
      name: "周岚",
      mode: "existing",
      account_id: "fund_done",
      account_name: "旧配置组合",
      top_account_id: "fund_top",
      top_account_name: "基金E账户",
      portfolio_id: "fund_done",
      portfolio_name: "旧配置组合",
      direct: false,
      ignored: false,
      fund_count: 1,
      market_value_cents: w(22890),
      top_funds: ["虚构平衡基金"],
      holdings: [
        holding(8, "000008", "虚构平衡基金", "15000.00", "1.5260", w(22890)),
      ],
    },
    {
      sales_org: "远山基金",
      trade_account: "90000005",
      name: "林澈",
      mode: "unmapped",
      ignored: false,
      issue: "未选择账户",
      fund_count: 1,
      market_value_cents: w(9876.54),
      top_funds: ["虚构远山混合"],
      holdings: [
        holding(9, "000009", "虚构远山混合", "7500.00", "1.3169", w(9876.54)),
      ],
    },
    {
      sales_org: "测试平台",
      trade_account: "90000006",
      name: "林澈",
      mode: "ignore",
      ignored: true,
      fund_count: 1,
      market_value_cents: w(1234),
      top_funds: ["虚构测试基金"],
      holdings: [
        holding(10, "000010", "虚构测试基金", "1000.00", "1.2340", w(1234)),
      ],
    },
    {
      sales_org: "旧日券商",
      trade_account: "90000007",
      name: "林澈",
      mode: "existing",
      account_id: "fund_legacy",
      account_name: "旧日基金账户",
      direct: true,
      ignored: false,
      fund_count: 1,
      market_value_cents: w(31500),
      top_funds: ["虚构旧日混合"],
      holdings: [
        holding(11, "000011", "虚构旧日混合", "21000.00", "1.5000", w(31500)),
      ],
    },
  ];

  const targets = [
    {
      key: "id:fund_core",
      account_id: "fund_core",
      account_name: "核心基金组合",
      group_count: 1,
      fund_count: 3,
      market_value_cents: w(68420.55),
      previous_as_of: "2026-10-01",
      already_imported: false,
      diff: {
        ...emptyDiff,
        added: [{ name: "虚构稳健债券", code: "000003", quantity: "11400.00", market_value_cents: w(13666.4) }],
        value_changed: [{ name: "虚构成长混合", old_value_cents: w(34500), new_value_cents: w(35952), delta_value_cents: w(1452) }],
        total_before_cents: w(58000),
        total_after_cents: w(68420.55),
        total_delta_cents: w(10420.55),
      },
      anomalies: [],
      holdings: [
        { name: "虚构成长混合", code: "000001", quantity: "28000.50", market_value_cents: w(35952), note: "净值 1.2840" },
        { name: "虚构宽基ETF", code: "000002", quantity: "14500.00", market_value_cents: w(18802.15), note: "净值 1.2967" },
        { name: "虚构稳健债券", code: "000003", quantity: "11400.00", market_value_cents: w(13666.4), note: "净值 1.1988" },
      ],
    },
    {
      key: "id:fund_spare",
      account_id: "fund_spare",
      account_name: "备用基金账户",
      group_count: 1,
      fund_count: 2,
      market_value_cents: w(15238),
      previous_as_of: "2026-10-01",
      already_imported: false,
      diff: {
        ...emptyDiff,
        total_before_cents: w(11500),
        total_after_cents: w(15238),
        total_delta_cents: w(3738),
      },
      anomalies: [
        { code: "total_change", message: "账户总市值相对上次变化 32.4%，超过阈值 20%" },
      ],
      holdings: [
        { name: "虚构红利指数", code: "000004", quantity: "9000.00", market_value_cents: w(9468), note: "净值 1.0520" },
        { name: "虚构货币基金", code: "000005", quantity: "5500.00", market_value_cents: w(5770), note: "净值 1.0491" },
      ],
    },
    {
      key: "id:fund_pending",
      account_id: "fund_pending",
      account_name: "稳健配置组合",
      group_count: 1,
      fund_count: 2,
      market_value_cents: w(41206.66),
      previous_as_of: "2026-10-01",
      already_imported: true,
      imported_status: "efund_import / 待确认",
      existing_snapshot_id: "snap_pending_01",
      existing_snapshot_source: "efund_import",
      diff: {
        ...emptyDiff,
        total_before_cents: w(41206.66),
        total_after_cents: w(41206.66),
      },
      anomalies: [],
      holdings: [],
    },
    {
      key: "id:fund_done",
      account_id: "fund_done",
      account_name: "旧配置组合",
      group_count: 1,
      fund_count: 1,
      market_value_cents: w(22890),
      previous_as_of: "2026-10-01",
      already_imported: true,
      imported_status: "efund_import / 已确认",
      existing_snapshot_id: "snap_done_01",
      existing_snapshot_source: "efund_import",
      diff: {
        ...emptyDiff,
        total_before_cents: w(22890),
        total_after_cents: w(22890),
      },
      anomalies: [],
      holdings: [],
    },
    {
      key: "id:fund_legacy",
      account_id: "fund_legacy",
      account_name: "旧日基金账户",
      group_count: 1,
      fund_count: 1,
      market_value_cents: w(31500),
      previous_as_of: "2026-10-01",
      already_imported: true,
      imported_status: "mcp / 已确认",
      existing_snapshot_id: "snap_legacy_01",
      existing_snapshot_source: "mcp",
      diff: {
        ...emptyDiff,
        total_before_cents: w(31500),
        total_after_cents: w(31500),
      },
      anomalies: [],
      holdings: [],
    },
  ];

  const preview = {
    token: "preview-token",
    file_name: "持有信息-2026-10-18.xlsx",
    meta: {
      share_date: "2026-10-18",
      print_time: "2026-10-18 20:16:42",
      total_records: 10,
      actual_rows: 10,
      records_match: true,
      nav_date_mismatch_funds: 1,
    },
    warnings: [
      { code: "nav_date_mismatch", severity: "info", message: "1 只 QDII 基金净值日期与份额日期不一致" },
      { code: "value_mismatch", severity: "warning", message: "云帆基金销售有限公司合计金额相差 0.01 元" },
      { code: "record_count_mismatch", severity: "blocks_write", message: "文件记录数与实际行数不一致，写入时将被阻止" },
    ],
    groups: previewGroups,
    targets,
    accounts: importAccounts,
    can_import: true,
    block_reason: "",
    blocked_by_data: false,
    total_market_value_cents: w(179255.21),
    file_total_cents: w(158865.75),
    ignored_total_cents: w(1234),
    unmapped_total_cents: w(9876.54),
    mapping_error: "",
    total_holding_count: 8,
  };

  const donePreview = JSON.parse(JSON.stringify(preview));
  donePreview.groups[4] = {
    ...donePreview.groups[4],
    mode: "ignore",
    ignored: true,
    issue: "",
  };
  donePreview.targets = donePreview.targets.map((target) => ({
    ...target,
    already_imported: true,
    imported_status: target.account_id === "fund_legacy"
      ? "mcp / 已确认"
      : target.account_id === "fund_spare" || target.account_id === "fund_pending"
        ? "efund_import / 待确认"
        : "efund_import / 已确认",
  }));
  donePreview.can_import = false;

  let importFinished = false;
  const undoneSnapshots = new Set();

  function currentPreview() {
    const base = importFinished ? donePreview : preview;
    if (!undoneSnapshots.size) return base;
    const copy = JSON.parse(JSON.stringify(base));
    copy.targets = copy.targets.map((target) => undoneSnapshots.has(target.existing_snapshot_id)
      ? { ...target, already_imported: false, imported_status: "", existing_snapshot_id: "", existing_snapshot_source: "" }
      : target);
    copy.can_import = true;
    copy.block_reason = "";
    return copy;
  }

  const importResult = {
    summary: "本次导入：1 组生效，2 组待确认，2 组之前已导入",
    applied: 1,
    pending: 2,
    skipped: 2,
    failed: 0,
    results: [
      { account_id: "fund_core", account_name: "核心基金组合", status: "applied", snapshot_id: "snap_mock_01", market_value_cents: w(68420.55) },
      { account_id: "fund_spare", account_name: "备用基金账户", status: "pending", message: "进入待确认", anomalies: ["账户总市值相对上次变化 32.4%，超过阈值 20%"], market_value_cents: w(15238) },
      { account_id: "fund_pending", account_name: "稳健配置组合", status: "pending", message: "已存在待确认快照", market_value_cents: w(41206.66) },
      { account_id: "fund_done", account_name: "旧配置组合", status: "skipped", message: "之前已导入：该账户在 2026-10-18 已有 efund_import / 已确认 快照", existing_snapshot_id: "snap_done_01", existing_snapshot_source: "efund_import", market_value_cents: w(22890) },
      { account_id: "fund_legacy", account_name: "旧日基金账户", status: "skipped", message: "之前已导入：该账户在 2026-10-18 已有 mcp / 已确认 快照", existing_snapshot_id: "snap_legacy_01", existing_snapshot_source: "mcp", market_value_cents: w(31500) },
    ],
  };

  globalThis.wails = {
    Call: {
      async ByName(rawName, ...args) {
        const name = rawName.replace(/^main\./, "");
        switch (name) {
          case "OverviewService.Get":
            return overview;
          case "AccountService.List":
            return accountTree;
          case "PendingService.List":
            return pending;
          case "EfundService.Preview":
            return preview;
          case "EfundService.Remap":
            return preview;
          case "EfundService.Import":
            importFinished = true;
            return importResult;
          case "EfundService.Refresh":
            return currentPreview();
          case "EfundService.UndoSnapshot":
            undoneSnapshots.add(args[0]);
            return null;
          case "EfundService.ClearSession":
          case "PendingService.Approve":
          case "PendingService.Reject":
            return null;
          default:
            return null;
        }
      },
    },
  };

  async function openPage(target) {
    const nav = document.querySelector(`.nav-item[data-view="${target}"]`);
    await waitFor(() => nav);
    nav.click();
    await waitFor(() => !document.getElementById(`${target}-view`).hidden);
  }

  async function loadImport() {
    await openPage("efund");
    await waitFor(() => !document.getElementById("efund-parse").disabled || document.getElementById("efund-file"));
    const input = document.getElementById("efund-file");
    const file = new File(["mock"], "持有信息-2026-10-18.xlsx", {
      type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    });
    const transfer = new DataTransfer();
    transfer.items.add(file);
    input.files = transfer.files;
    input.dispatchEvent(new Event("change", { bubbles: true }));
    await waitFor(() => !document.getElementById("efund-parse").disabled);
    document.getElementById("efund-parse").click();
    await waitFor(() => document.querySelector("#efund-result .efund-group"));
    if (page !== "import-done") return;
    const run = [...document.querySelectorAll("#efund-result button")]
      .find((button) => button.textContent.trim() === "全部导入并确认");
    if (!run) throw new Error("找不到全部导入按钮");
    run.click();
    await waitFor(() => document.querySelector("#efund-result .efund-complete"), 12000);
  }

  async function boot() {
    try {
      if (page === "accounts" || page === "pending") await openPage(page);
      else if (page === "import" || page === "import-done") await loadImport();
      document.documentElement.dataset.previewReady = page;
      window.__uiPreviewReady = true;
    } catch (error) {
      const status = document.getElementById("status");
      status.hidden = false;
      status.className = "status error";
      status.textContent = `静态预览失败：${error.message}`;
      console.error(error);
    }
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", () => setTimeout(boot, 0));
  } else {
    setTimeout(boot, 0);
  }
})();
