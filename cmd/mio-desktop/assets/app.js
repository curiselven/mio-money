if (window.location.protocol === "wails:" || window.location.hostname === "wails.localhost") {
  await import("/wails/runtime.js");
}
import { accountEditValues, accountFormVisibility, accountOwnerNote, accountPortfolioView } from "./account_form.mjs";
import { accountRows, allocationTotals } from "./account_tree.mjs";
import { groupState, importSummary, mergeResults, portfolioChoices, priorImportUndo, targetState } from "./efund_flow.mjs";

const overviewView = document.getElementById("overview-view");
const pendingView = document.getElementById("pending-view");
const loanView = document.getElementById("loan-view");
const efundView = document.getElementById("efund-view");
const accountsView = document.getElementById("accounts-view");
const navLoan = document.getElementById("nav-loan");
const navItems = [...document.querySelectorAll(".nav-item")];
const statusBox = document.getElementById("status");
const refresh = document.getElementById("refresh");
const overviewEmpty = document.getElementById("overview-empty");
const overviewBody = document.getElementById("overview-body");
const accountsBody = document.getElementById("accounts");
const chartBox = document.getElementById("chart");
const chartNote = document.getElementById("chart-note");
const allocationBox = document.getElementById("allocation");
const accountCount = document.getElementById("account-count");
const netWorth = document.getElementById("net-worth");
const netAsOf = document.getElementById("net-asof");
const netChange = document.getElementById("net-change");
const assets = document.getElementById("assets");
const liabilities = document.getElementById("liabilities");
const assetCount = document.getElementById("asset-count");
const liabilityCount = document.getElementById("liability-count");
const nextLoan = document.getElementById("next-loan");
const nextLoanMeta = document.getElementById("next-loan-meta");
const pendingCount = document.getElementById("pending-count");
const list = document.getElementById("list");
const empty = document.getElementById("empty");
const cardTemplate = document.getElementById("card");
const loanHost = document.getElementById("loan-form-host");
const efundFileInput = document.getElementById("efund-file");
const efundFileName = document.getElementById("efund-file-name");
const efundPasswordWrap = document.getElementById("efund-password-wrap");
const efundPassword = document.getElementById("efund-password");
const efundParse = document.getElementById("efund-parse");
const efundResultRoot = document.getElementById("efund-result");
const efundUpload = document.getElementById("efund-upload");
const efundSteps = [...document.querySelectorAll("#efund-steps [data-step]")];
const accountsTree = document.getElementById("accounts-tree");
const accountCreatePanel = document.getElementById("account-create-panel");
const portfolioCreatePanel = document.getElementById("portfolio-create-panel");
const accountEditPanel = document.getElementById("account-edit-panel");
const accountCreateOpen = document.getElementById("account-create-open");
const portfolioCreateOpen = document.getElementById("portfolio-create-open");
const accountCreateForm = document.getElementById("account-create");
const portfolioCreateForm = document.getElementById("portfolio-create");
const accountEditForm = document.getElementById("account-edit");
const accountEditSelect = document.getElementById("account-edit-select");
const ownerOptions = document.getElementById("owner-options");

const sourceLabels = { mcp: "Agent 提交", screenshot: "截图", statement: "对账单", manual: "手动", efund_import: "基金E账户" };
const typeLabels = { cash: "现金", fund: "基金", stock: "股票", mixed: "混合", liability: "负债", other: "其他" };
const typeColors = { stock: "#E07A5F", fund: "#6E8B5B", cash: "#D4A64A", mixed: "#C98A2B", other: "#8D7B66" };
const efundMaxFileBytes = 32 * 1024 * 1024; // Keep in sync with efund.MaxInputBytes.
const principalRuleLabels = { new_schedule: "新月供本金", fixed_payment: "月供不变", old_schedule: "旧月供本金" };
const warningSeverityLabels = { info: "提示", warning: "注意", blocks_write: "将阻止写入" };
const moneyFormat = new Intl.NumberFormat("zh-CN", {
  style: "currency",
  currency: "CNY",
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

let currentView = "overview";
let busy = false;
const expandedAccounts = new Set();
let accountList = [];
let loanAccount = null;
let previewSeq = 0;
let previewTimer = 0;
let reconcileValues = new Map();
let reconcileSuggestions = [];
let efundFile = null;
let efundPreview = null;
let efundImportResult = null;
let efundOutcomes = new Map();
let efundBusy = false;
let efundRefreshError = "";
let efundSaveTimer = 0;
let accountTree = null;
let archiveArmed = false;
let statusTimer = 0;

async function call(method, ...args) {
  if (!globalThis.wails?.Call?.ByName) {
    throw new Error("Wails 运行时未就绪");
  }
  return globalThis.wails.Call.ByName(`main.${method}`, ...args);
}

function money(cents) {
  return moneyFormat.format(cents / 100);
}

function signedMoney(cents) {
  if (cents === 0) return money(0);
  return `${cents > 0 ? "+" : "-"}${money(Math.abs(cents))}`;
}

function signedPercent(percent) {
  return `${percent > 0 ? "+" : ""}${percent.toFixed(2)}%`;
}

function centsToYuan(cents) {
  const abs = Math.abs(cents);
  return `${Math.trunc(abs / 100)}.${String(abs % 100).padStart(2, "0")}`;
}

function yuanToCents(text) {
  const match = /^(\d+)(?:\.(\d{1,2}))?$/.exec(String(text).trim());
  if (!match) return null;
  return Number(match[1]) * 100 + Number((match[2] || "").padEnd(2, "0"));
}

function errorText(error) {
  return error?.message || error?.cause?.message || String(error);
}

function showStatus(text, kind = "ok") {
  clearTimeout(statusTimer);
  statusBox.textContent = text;
  statusBox.className = `status ${kind}`;
  statusBox.hidden = false;
  if (kind !== "error" && kind !== "loading") {
    statusTimer = setTimeout(hideStatus, 4000);
  }
}

function hideStatus() {
  clearTimeout(statusTimer);
  statusTimer = 0;
  statusBox.textContent = "";
  statusBox.hidden = true;
}

function setPendingCount(count) {
  pendingCount.textContent = String(count);
  pendingCount.hidden = count === 0;
}

function text(selector, value, root = document) {
  const element = root.querySelector(selector);
  if (element) element.textContent = value;
}

function localToday() {
  const now = new Date();
  return `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, "0")}-${String(now.getDate()).padStart(2, "0")}`;
}

/* ---------- overview ---------- */

function renderOverview(overview) {
  setPendingCount(overview.pending_count);
  overviewEmpty.hidden = !overview.empty;
  overviewBody.hidden = overview.empty;
  if (overview.empty) return;

  netWorth.textContent = money(overview.net_cents);
  netAsOf.textContent = overview.as_of ? `· 截至 ${overview.as_of}` : "";
  assets.textContent = money(overview.assets_cents);
  liabilities.textContent = money(overview.liabilities_cents);
  const assetAccounts = overview.accounts.filter((a) => a.type !== "liability");
  const liabilityAccounts = overview.accounts.filter((a) => a.type === "liability");
  assetCount.textContent = `${assetAccounts.length} 个账户`;
  liabilityCount.textContent = `${liabilityAccounts.length} 个账户`;
  accountCount.textContent = `${overview.accounts.length} 个账户`;

  if (overview.change_cents == null) {
    netChange.hidden = true;
  } else {
    const percent = overview.change_percent == null ? "" : `（${signedPercent(overview.change_percent)}）`;
    netChange.textContent = `${overview.change_cents >= 0 ? "▲" : "▼"} ${signedMoney(overview.change_cents)}${percent} · 较上一有数据日期`;
    netChange.className = `delta num ${overview.change_cents >= 0 ? "up" : "down"}`;
    netChange.hidden = false;
  }

  const activeLoans = overview.accounts.filter((a) => a.loan?.next_date);
  activeLoans.sort((a, b) => a.loan.next_date.localeCompare(b.loan.next_date));
  if (activeLoans.length > 0) {
    const loan = activeLoans[0].loan;
    nextLoan.textContent = money(loan.next_payment_cents);
    nextLoanMeta.textContent = `${loan.next_date} · 还剩 ${loan.remaining_periods} 期`;
  } else if (overview.accounts.some((a) => a.loan)) {
    nextLoan.textContent = "已还清";
    nextLoanMeta.textContent = "无在还贷款";
  } else {
    nextLoan.textContent = "—";
    nextLoanMeta.textContent = "暂无贷款计划";
  }

  renderAccounts(overview.accounts);
  renderAllocation(overview);
  renderChart(overview.history);
}

function cell(row, value, className = "") {
  const td = document.createElement("td");
  if (className) td.className = className;
  if (value instanceof Node) td.append(value);
  else td.textContent = value;
  return td;
}

function loanGroup(label, value, strong = true) {
  const span = document.createElement("span");
  span.append(document.createTextNode(`${label} `));
  const em = document.createElement(strong ? "em" : "span");
  em.textContent = value;
  if (strong) em.className = "num";
  span.append(em);
  return span;
}

function renderAccounts(accounts) {
  accountsBody.replaceChildren();
  if (accounts.length === 0) {
    const row = document.createElement("tr");
    const td = cell(row, "还没有账户", "l muted");
    td.colSpan = 5;
    row.append(td);
    accountsBody.append(row);
    return;
  }
  accountList = accounts;
  const rows = accountRows(accounts, expandedAccounts);
  for (const account of accounts) {
    const info = rows.get(account.account_id);
    const row = document.createElement("tr");
    row.dataset.accountRow = account.account_id;
    if (!info.root) {
      row.dataset.parent = account.parent_id;
      row.className = "account-row-child";
    }
    if (info.expandable) row.classList.add("account-expandable");
    const name = document.createElement("span");
    name.className = "account-name";
    if (info.expandable) {
      const toggle = document.createElement("button");
      toggle.type = "button";
      toggle.className = "account-toggle";
      toggle.setAttribute("aria-expanded", String(info.detailVisible));
      toggle.dataset.accountToggle = account.account_id;
      const caret = document.createElement("span");
      caret.className = "acc-caret";
      caret.textContent = info.detailVisible ? "▾" : "▸";
      toggle.append(caret, document.createTextNode(account.name));
      name.append(toggle);
    } else {
      name.textContent = account.name;
    }
    const tag = document.createElement("span");
    tag.className = "tag";
    tag.textContent = typeLabels[account.type] || account.type;
    name.append(tag);
    row.append(cell(row, name, "l"));
    const value = cell(row, money(info.totalCents), `num ${account.type === "liability" ? "neg" : ""}`);
    row.append(value);

    const share = document.createElement("span");
    if (info.root) {
      share.className = "num";
      share.textContent = `${info.totalShare.toFixed(1)}%`;
      const bar = document.createElement("span");
      bar.className = "shbar";
      const fillBar = document.createElement("i");
      if (account.type === "liability") fillBar.className = "n";
      fillBar.style.width = `${Math.max(1, Math.min(60, Math.abs(info.totalShare) * 0.6))}px`;
      bar.append(fillBar);
      share.append(bar);
    } else {
      share.className = "num faint";
      share.textContent = "—";
    }
    row.append(cell(row, share, "num"));
    row.append(cell(row, (info.root ? info.totalAsOf : account.as_of) || "—", "num faint"));
    const action = document.createElement("td");
    action.className = "account-action";
    if (account.type === "liability") {
      const button = document.createElement("button");
      button.type = "button";
      button.className = account.loan ? "btn small" : "btn small warn loan-setup";
      button.textContent = account.loan ? "修改还款计划" : "设为贷款…";
      button.addEventListener("click", () => openLoanView(account));
      action.append(button);
    }
    row.append(action);
    accountsBody.append(row);
    if (info.expandable) {
      row.addEventListener("click", (event) => {
        if (event.target.closest(".account-action")) return;
        if (expandedAccounts.has(account.account_id)) expandedAccounts.delete(account.account_id);
        else expandedAccounts.add(account.account_id);
        refreshAccountRows();
      });
    }

    for (const item of account.items || []) {
      const sub = document.createElement("tr");
      sub.className = "sub";
      sub.dataset.detailFor = account.account_id;
      if (!info.root) sub.dataset.parent = account.parent_id;
      const itemName = document.createElement("span");
      itemName.textContent = item.name;
      if (item.code) {
        const code = document.createElement("span");
        code.className = "num faint";
        code.textContent = ` ${item.code}`;
        itemName.append(code);
      }
      sub.append(cell(sub, itemName, "l"));
      sub.append(cell(sub, money(item.market_value_cents), "num"));
      sub.append(cell(sub, item.quantity ? `${item.quantity} 份` : "", "num"));
      sub.append(cell(sub, ""), cell(sub, ""));
      accountsBody.append(sub);
    }
    if (account.cash_cents != null) {
      const sub = document.createElement("tr");
      sub.className = "sub";
      sub.dataset.detailFor = account.account_id;
      if (!info.root) sub.dataset.parent = account.parent_id;
      sub.append(cell(sub, "现金", "l"));
      sub.append(cell(sub, money(account.cash_cents), "num"));
      sub.append(cell(sub, ""), cell(sub, ""), cell(sub, ""));
      accountsBody.append(sub);
    }

    if (account.loan) {
      const sub = document.createElement("tr");
      sub.className = "sub loan-sub";
      sub.dataset.detailFor = account.account_id;
      if (!info.root) sub.dataset.parent = account.parent_id;
      const td = document.createElement("td");
      td.className = "l";
      td.colSpan = 5;
      const lines = document.createElement("div");
      lines.className = "loan-lines";
      const current = document.createElement("div");
      current.className = "m";
      current.append(
        loanGroup("还剩", `${account.loan.remaining_periods} 期`),
        loanGroup("下期", account.loan.next_date || "已还清"),
        loanGroup("月供", account.loan.next_date ? money(account.loan.next_payment_cents) : "—"),
        loanGroup("剩余利息", money(account.loan.remaining_interest_cents)),
      );
      const paid = document.createElement("div");
      paid.className = "m";
      paid.append(
        loanGroup("已还", `${account.loan.paid_periods || 0} 期`),
        loanGroup("本金", money(account.loan.paid_principal_cents || 0)),
        loanGroup("利息", money(account.loan.paid_interest_cents || 0)),
        loanGroup("合计", money(account.loan.paid_total_cents || 0)),
      );
      lines.append(current, paid);
      td.append(lines);
      sub.append(td);
      accountsBody.append(sub);
    }
  }
  refreshAccountRows();
}

function refreshAccountRows() {
  const rows = accountRows(accountList, expandedAccounts);
  for (const row of accountsBody.querySelectorAll("tr[data-account-row]")) {
    const info = rows.get(row.dataset.accountRow);
    row.hidden = !info || !info.visible;
  }
  for (const row of accountsBody.querySelectorAll("tr[data-detail-for]")) {
    const info = rows.get(row.dataset.detailFor);
    row.hidden = !info || !info.detailVisible;
  }
  for (const toggle of accountsBody.querySelectorAll("[data-account-toggle]")) {
    const info = rows.get(toggle.dataset.accountToggle);
    const expanded = Boolean(info?.detailVisible);
    toggle.setAttribute("aria-expanded", String(expanded));
    const caret = toggle.querySelector(".acc-caret");
    if (caret) caret.textContent = expanded ? "▾" : "▸";
  }
}

function renderAllocation(overview) {
  allocationBox.replaceChildren();
  const totals = allocationTotals(overview.accounts);
  const total = [...totals.values()].reduce((sum, value) => sum + value, 0);
  if (total === 0) {
    const note = document.createElement("p");
    note.className = "muted";
    note.textContent = "暂无资产";
    allocationBox.append(note);
    return;
  }
  for (const type of ["stock", "fund", "cash", "mixed", "other"]) {
    const amount = totals.get(type) || 0;
    if (amount === 0) continue;
    const percent = (amount / total) * 100;
    const row = document.createElement("div");
    row.className = "al";
    const name = document.createElement("span");
    name.className = "name";
    name.textContent = typeLabels[type] || type;
    const bar = document.createElement("div");
    bar.className = "bar";
    const fill = document.createElement("i");
    fill.style.width = `${percent}%`;
    fill.style.background = typeColors[type] || "#8D7B66";
    bar.append(fill);
    const pct = document.createElement("span");
    pct.className = "pct num";
    pct.textContent = `${percent.toFixed(1)}%`;
    row.append(name, bar, pct);
    allocationBox.append(row);
  }
  const foot = document.createElement("div");
  foot.className = "alloc-foot";
  foot.append(document.createTextNode("资产负债率 "));
  const strong = document.createElement("strong");
  strong.className = "num";
  strong.textContent = overview.assets_cents > 0
    ? `${((overview.liabilities_cents / overview.assets_cents) * 100).toFixed(1)}%`
    : "—";
  foot.append(strong);
  allocationBox.append(foot);
}

/* ---------- charts ---------- */

const svgNS = "http://www.w3.org/2000/svg";

function svgEl(name, attributes) {
  const element = document.createElementNS(svgNS, name);
  for (const [key, value] of Object.entries(attributes)) {
    element.setAttribute(key, String(value));
  }
  return element;
}

function renderChart(points) {
  chartBox.replaceChildren();
  chartNote.textContent = points.length > 0 ? "近 " + points.length + " 个日期" : "";
  if (points.length === 0) {
    const note = document.createElement("p");
    note.className = "muted";
    note.textContent = "还没有已确认的快照";
    chartBox.append(note);
    return;
  }
  const width = 780;
  const height = 220;
  const pad = { top: 16, right: 16, bottom: 30, left: 98 };
  const innerWidth = width - pad.left - pad.right;
  const innerHeight = height - pad.top - pad.bottom;
  const values = points.map((point) => point.value_cents);
  let min = Math.min(...values);
  let max = Math.max(...values);
  if (min === max) {
    const bump = Math.max(Math.abs(min) * 0.05, 100);
    min -= bump;
    max += bump;
  }
  const x = (index) => points.length === 1
    ? pad.left + innerWidth / 2
    : pad.left + (index * innerWidth) / (points.length - 1);
  const y = (value) => pad.top + ((max - value) / (max - min)) * innerHeight;

  const svg = svgEl("svg", { viewBox: `0 0 ${width} ${height}`, class: "chart-svg", role: "img", "aria-label": "净资产历史曲线" });
  const defs = svgEl("defs", {});
  const gradient = svgEl("linearGradient", { id: "chartFill", x1: 0, y1: 0, x2: 0, y2: 1 });
  gradient.append(
    svgEl("stop", { offset: "0", "stop-color": "#C8754B", "stop-opacity": "0.25" }),
    svgEl("stop", { offset: "1", "stop-color": "#C8754B", "stop-opacity": "0" }),
  );
  defs.append(gradient);
  svg.append(defs);

  for (let i = 0; i < 3; i += 1) {
    const value = max - ((max - min) * i) / 2;
    svg.append(svgEl("line", {
      x1: pad.left, x2: width - pad.right, y1: y(value), y2: y(value), class: "grid-line",
    }));
    const label = svgEl("text", { x: pad.left - 10, y: y(value) + 4, class: "axis-label", "text-anchor": "end" });
    label.textContent = money(Math.round(value));
    svg.append(label);
  }

  const coords = points.map((point, index) => [x(index), y(point.value_cents)]);
  if (points.length > 1) {
    const baseline = pad.top + innerHeight;
    const area = [`M ${coords[0][0]},${baseline}`, ...coords.map(([cx, cy]) => `L ${cx},${cy}`),
      `L ${coords[coords.length - 1][0]},${baseline}`, "Z"].join(" ");
    svg.append(svgEl("path", { d: area, class: "chart-area" }));
    svg.append(svgEl("polyline", {
      points: coords.map(([cx, cy]) => `${cx},${cy}`).join(" "), class: "chart-line",
    }));
  }
  points.forEach((point, index) => {
    const [cx, cy] = coords[index];
    const dot = svgEl("circle", { cx, cy, r: 3.2, class: "chart-dot" });
    const tip = svgEl("title", {});
    tip.textContent = `${point.as_of} · ${money(point.value_cents)}`;
    dot.append(tip);
    svg.append(dot);
  });
  const labelIndexes = points.length === 1 ? [0] : [0, Math.floor((points.length - 1) / 2), points.length - 1];
  for (const index of labelIndexes) {
    const label = svgEl("text", {
      x: x(index),
      y: height - 8,
      class: "axis-label",
      "text-anchor": points.length === 1 ? "middle" : index === 0 ? "start" : index === points.length - 1 ? "end" : "middle",
    });
    label.textContent = points[index].as_of;
    svg.append(label);
  }
  chartBox.append(svg);
}

/* ---------- pending ---------- */

function addLine(element, label, value) {
  const item = document.createElement("li");
  const tag = document.createElement("span");
  tag.className = "tag-line";
  tag.textContent = label;
  const body = document.createElement("span");
  body.textContent = value;
  item.append(tag, body);
  element.append(item);
}

function renderDiff(element, diff) {
  for (const item of diff.added) addLine(element, "新增", `${item.name} ${item.quantity || "—"} 份 · ${money(item.market_value_cents)}`);
  for (const item of diff.removed) addLine(element, "消失", `${item.name}（原 ${money(item.market_value_cents)}）`);
  for (const item of diff.quantity_changed) addLine(element, "份额", `${item.name} ${item.old_quantity || "—"} → ${item.new_quantity || "—"}`);
  for (const item of diff.value_changed) addLine(element, "市值", `${item.name} ${money(item.old_value_cents)} → ${money(item.new_value_cents)}（${signedMoney(item.delta_value_cents)}）`);
  for (const item of diff.cleared) addLine(element, "清仓", `${item.name} ${item.old_quantity || "—"} → ${item.new_quantity || "0"}`);
  addLine(element, "总额", `${money(diff.total_before_cents)} → ${money(diff.total_after_cents)}（${signedMoney(diff.total_delta_cents)}）`);
}

function closePanels() {
  for (const card of list.children) {
    card.querySelector(".actions").hidden = false;
    card.querySelector(".confirm-panel").hidden = true;
    card.querySelector(".reason").value = "";
  }
}

async function runPending(action) {
  if (busy) return;
  busy = true;
  try {
    await action();
  } catch (error) {
    showStatus(errorText(error), "error");
  } finally {
    busy = false;
  }
}

function openPanel(card, review, mode) {
  closePanels();
  const panel = card.querySelector(".confirm-panel");
  const reasonField = panel.querySelector(".reason-field");
  const submit = panel.querySelector(".confirm-submit");
  const isReject = mode === "reject";
  card.querySelector(".actions").hidden = true;
  panel.hidden = false;
  panel.dataset.mode = mode;
  panel.querySelector(".confirm-text").textContent = isReject
    ? "确认丢弃这份快照？它不会写入正式数据。"
    : "确认将这份快照写入正式数据？命中的异常规则会记入日志，之后可撤回。";
  reasonField.hidden = !isReject;
  submit.textContent = isReject ? "确认丢弃" : "确认";
  submit.classList.toggle("danger", isReject);
  submit.onclick = () => {
    const reason = panel.querySelector(".reason").value.trim();
    runPending(async () => {
      if (isReject) {
        await call("PendingService.Reject", review.snapshot_id, reason);
        showStatus("已丢弃");
      } else {
        await call("PendingService.Approve", review.snapshot_id);
        showStatus("已确认生效");
      }
      await reloadPending();
    });
  };
  (isReject ? panel.querySelector(".reason") : submit).focus();
}

function renderPending(reviews) {
  list.replaceChildren();
  empty.hidden = reviews.length !== 0;
  setPendingCount(reviews.length);
  for (const review of reviews) {
    const card = cardTemplate.content.firstElementChild.cloneNode(true);
    card.querySelector(".account").textContent = review.account_name;
    card.querySelector(".meta").textContent = `${review.as_of} · 来源：${sourceLabels[review.source] || review.source}`;
    const anomalies = card.querySelector(".anomalies");
    for (const anomaly of review.anomalies) {
      const item = document.createElement("li");
      item.textContent = anomaly.message;
      anomalies.append(item);
    }
    renderDiff(card.querySelector(".diff"), review.diff);
    card.querySelector(".approve").addEventListener("click", () => openPanel(card, review, "approve"));
    card.querySelector(".reject").addEventListener("click", () => openPanel(card, review, "reject"));
    card.querySelector(".confirm-cancel").addEventListener("click", () => closePanels());
    list.append(card);
  }
}

async function reloadPending() {
  renderPending(await call("PendingService.List"));
}

/* ---------- loan form ---------- */

function loanField(label, input, className = "") {
  const wrapper = document.createElement("div");
  wrapper.className = `loan-field ${className}`.trim();
  const span = document.createElement("span");
  span.textContent = label;
  wrapper.append(span, input);
  return wrapper;
}

function inputElement(className, attributes) {
  const input = document.createElement("input");
  input.className = className;
  for (const [key, value] of Object.entries(attributes)) {
    input.setAttribute(key, String(value));
  }
  return input;
}

function segControl(name, values, current) {
  const seg = document.createElement("div");
  seg.className = `seg ${name}`;
  seg.dataset.value = current;
  for (const [value, label] of values) {
    const button = document.createElement("button");
    button.type = "button";
    button.dataset.seg = value;
    button.textContent = label;
    if (value === current) button.classList.add("on");
    seg.append(button);
  }
  return seg;
}

function segValue(form, name) {
  return form.querySelector(`.seg.${name.replace(/^\./, "")}`).dataset.value;
}

function setSeg(seg, value) {
  seg.dataset.value = value;
  for (const button of seg.querySelectorAll("button")) {
    button.classList.toggle("on", button.dataset.seg === value);
  }
}

function prepaymentsOf(form) {
  return JSON.parse(form.dataset.prepayments || "[]");
}

function readAnchorForm(form) {
  const principal = yuanToCents(form.querySelector(".loan-principal").value);
  if (principal == null) return { error: "本金请用元填写，最多两位小数" };
  const values = {
    account_id: form.dataset.accountId,
    anchor_date: form.querySelector(".loan-anchor").value.trim(),
    anchor_principal_cents: principal,
    annual_rate: form.querySelector(".loan-rate").value.trim(),
    method: segValue(form, "loan-method"),
    payment_day: Number(form.querySelector(".loan-day").value),
    remaining_periods: Number(form.querySelector(".loan-periods").value),
  };
  if (!values.anchor_date || !values.annual_rate || !values.payment_day || !values.remaining_periods) return {};
  return { values };
}

function readHistoryForm(form) {
  const principal = yuanToCents(form.querySelector(".loan-principal-history").value);
  if (principal == null) return { error: "原始本金请用元填写，最多两位小数" };
  const changes = [];
  for (const row of form.querySelectorAll(".rate-change-row")) {
    const date = row.querySelector(".rate-change-date").value.trim();
    const rate = row.querySelector(".rate-change-rate").value.trim();
    if (!date && !rate) continue;
    if (!date || !rate) return { error: "利率调整的生效日期和利率都要填写" };
    changes.push({
      date,
      annual_rate: rate,
      principal_rule: row.querySelector(".rate-change-principal").value,
    });
  }
  const values = {
    account_id: form.dataset.accountId,
    original_principal_cents: principal,
    disbursement_date: form.querySelector(".loan-disbursement").value.trim(),
    first_payment_date: form.querySelector(".loan-first-date").value.trim(),
    total_periods: Number(form.querySelector(".loan-total-periods").value),
    method: segValue(form, "loan-method"),
    initial_annual_rate: form.querySelector(".loan-initial-rate").value.trim(),
    rate_changes: changes,
    prepayments: prepaymentsOf(form),
  };
  if (!values.first_payment_date || !values.initial_annual_rate || !values.total_periods) return {};
  return { values };
}

function showLoanError(form, message) {
  const error = form.querySelector(".loan-error");
  error.textContent = message;
  error.hidden = !message;
}

function clearKpis(form) {
  for (const selector of [".pv-payment", ".pv-balance", ".pv-interest", ".pv-paid-periods", ".pv-paid-principal", ".pv-paid-interest", ".pv-paid-total"]) {
    text(selector, "—", form);
  }
  for (const selector of [".pv-payment-note", ".pv-balance-note", ".pv-interest-note"]) {
    text(selector, "", form);
  }
  form.querySelector(".schedule-panel").hidden = true;
  form.querySelector(".paid").hidden = true;
}

function applyHistoryPreview(form, result) {
  const next = result.next_payment;
  text(".pv-payment", next ? money(next.payment_cents) : "已还清", form);
  text(".pv-payment-note", next ? `第 ${next.period} 期 · 下期 ${next.date}` : "已无后续还款", form);
  text(".pv-balance", money(result.balance_cents), form);
  text(".pv-balance-note", `还剩 ${result.remaining_periods} 期`, form);
  text(".pv-interest", money(result.remaining_interest_cents), form);
  const last = result.periods?.[result.periods.length - 1];
  text(".pv-interest-note", last ? `至 ${last.date}` : "", form);
  const paid = form.querySelector(".paid");
  paid.hidden = false;
  text(".pv-paid-periods", `${result.paid_periods} 期`, form);
  text(".pv-paid-principal", money(result.paid_principal_cents), form);
  text(".pv-paid-interest", money(result.paid_interest_cents), form);
  text(".pv-paid-total", money(result.paid_total_cents), form);
  renderSchedule(form, result.periods || []);
  renderRateChart(form);
  renderReconcileRows(form, result);
}

function applyAnchorPreview(form, result, values) {
  form.querySelector(".paid").hidden = true;
  form.querySelector(".schedule-panel").hidden = true;
  text(".pv-payment", money(result.payment_cents), form);
  text(".pv-payment-note", values.method === "equal_principal"
    ? `首期 · 末期 ${money(result.last_payment_cents)}`
    : `下期 ${result.next_date}`, form);
  text(".pv-balance", "—", form);
  text(".pv-balance-note", "锚点模式按起算日计", form);
  text(".pv-interest", money(result.remaining_interest_cents), form);
  text(".pv-interest-note", `至 ${result.last_date}`, form);
}

function renderSchedule(form, periods) {
  const panel = form.querySelector(".schedule-panel");
  const body = form.querySelector(".schedule-body");
  body.replaceChildren();
  if (!periods.length) {
    panel.hidden = true;
    return;
  }
  panel.hidden = false;
  text(".schedule-note", `${periods.length} 期 · 黄色为利率调整当期`, form);
  const initialRate = form.querySelector(".loan-initial-rate").value.trim() || periods[0]?.annual_rate || "";
  const adjustments = adjustmentIndexes(periods, initialRate);
  periods.forEach((period, index) => {
    const tr = document.createElement("tr");
    if (adjustments.has(index)) tr.className = "adj";
    let rate = `${period.annual_rate}%`;
    if (period.rate_split) rate += " · 拆分计息";
    if (period.principal_rule) rate += ` · ${principalRuleLabels[period.principal_rule] || period.principal_rule}`;
    for (const value of [
      period.period,
      period.date,
      rate,
      money(period.payment_cents),
      money(period.principal_cents),
      money(period.interest_cents),
      money(period.balance_cents),
    ]) {
      const td = document.createElement("td");
      td.textContent = value;
      tr.append(td);
    }
    if (period.rate_split || period.principal_rule) tr.children[2].className = "adj-note";
    body.append(tr);
  });
}

function adjustmentIndexes(periods, initialRate) {
  const indexes = new Set();
  periods.forEach((period, index) => {
    const previous = index === 0 ? initialRate : periods[index - 1].annual_rate;
    if (period.principal_rule || (previous && period.annual_rate !== previous)) {
      indexes.add(index);
    }
  });
  return indexes;
}

function rateRows(form) {
  return [...form.querySelectorAll(".rate-change-row")];
}

function renumberRates(form) {
  rateRows(form).forEach((row, index) => {
    row.querySelector(".rate-ix").textContent = String(index + 1);
  });
}

function appendRateRow(form, change = {}) {
  const row = document.createElement("div");
  row.className = "rate-change-row";
  const ix = document.createElement("span");
  ix.className = "rate-ix num";
  const date = inputElement("rate-change-date", { type: "date", "aria-label": "利率生效日期" });
  date.value = change.date || "";
  const rate = inputElement("rate-change-rate", { type: "text", inputmode: "decimal", placeholder: "3.20", "aria-label": "新年利率" });
  rate.value = change.annual_rate || "";
  const rule = document.createElement("select");
  rule.className = "rate-change-principal";
  rule.setAttribute("aria-label", "当期本金算法");
  for (const [value, label] of [
    ["new_schedule", "新月供"],
    ["fixed_payment", "月供不变"],
    ["old_schedule", "旧月供"],
  ]) {
    const option = document.createElement("option");
    option.value = value;
    option.textContent = label;
    rule.append(option);
  }
  rule.value = change.principal_rule || "new_schedule";
  const remove = document.createElement("button");
  remove.type = "button";
  remove.className = "x";
  remove.textContent = "删除";
  remove.addEventListener("click", () => {
    row.remove();
    renumberRates(form);
    schedulePreview();
  });
  row.append(ix, date, rate, rule, remove);
  form.querySelector(".rate-change-list").append(row);
  renumberRates(form);
}

function renderRateChart(form) {
  const box = form.querySelector(".rate-chart");
  if (!box) return;
  box.replaceChildren();
  if (segValue(form, ".loan-mode") !== "history") return;
  const initial = Number(form.querySelector(".loan-initial-rate").value);
  const firstDate = form.querySelector(".loan-first-date").value;
  const changes = rateRows(form)
    .map((row) => ({ date: row.querySelector(".rate-change-date").value, rate: Number(row.querySelector(".rate-change-rate").value) }))
    .filter((change) => change.date && Number.isFinite(change.rate))
    .sort((a, b) => a.date.localeCompare(b.date));
  if (!firstDate || !Number.isFinite(initial)) {
    const note = document.createElement("p");
    note.className = "muted";
    note.textContent = "填写初始年利率和首期还款日后显示利率走势";
    box.append(note);
    return;
  }
  const points = [{ date: firstDate, rate: initial }, ...changes].sort((a, b) => a.date.localeCompare(b.date));
  const first = new Date(points[0].date).getTime();
  const last = new Date(points[points.length - 1].date).getTime();
  const span = Math.max(last - first, 86400000 * 30);
  const rates = points.map((point) => point.rate);
  let min = Math.min(...rates) - 0.2;
  let max = Math.max(...rates) + 0.2;
  if (max - min < 0.4) max = min + 0.4;
  const width = 420;
  const height = 150;
  const pad = { top: 12, right: 10, bottom: 22, left: 10 };
  const innerWidth = width - pad.left - pad.right;
  const innerHeight = height - pad.top - pad.bottom;
  const x = (date) => pad.left + ((new Date(date).getTime() - first) / span) * innerWidth;
  const y = (rate) => pad.top + ((max - rate) / (max - min)) * innerHeight;

  const svg = svgEl("svg", { viewBox: `0 0 ${width} ${height}`, role: "img", "aria-label": "利率走势" });
  for (let i = 0; i < 3; i += 1) {
    const yy = pad.top + (innerHeight * i) / 2;
    svg.append(svgEl("line", { x1: pad.left, x2: width - pad.right, y1: yy, y2: yy, class: "rate-grid" }));
  }
  let path = `M ${x(points[0].date)},${y(points[0].rate)}`;
  for (let i = 1; i < points.length; i += 1) {
    path += ` L ${x(points[i].date)},${y(points[i - 1].rate)} L ${x(points[i].date)},${y(points[i].rate)}`;
  }
  path += ` L ${width - pad.right},${y(points[points.length - 1].rate)}`;
  svg.append(svgEl("path", { d: path, class: "rate-plot" }));
  points.forEach((point, index) => {
    const label = svgEl("text", {
      x: x(point.date) + (index === 0 ? 3 : -2),
      y: y(point.rate) - 5,
      class: "rate-value",
      "text-anchor": index === 0 ? "start" : "end",
    });
    label.textContent = point.rate.toFixed(2);
    svg.append(label);
  });
  const firstLabel = svgEl("text", { x: pad.left, y: height - 6, class: "rate-label", "text-anchor": "start" });
  firstLabel.textContent = points[0].date.slice(0, 7);
  const lastLabel = svgEl("text", { x: width - pad.right, y: height - 6, class: "rate-label", "text-anchor": "end" });
  lastLabel.textContent = points[points.length - 1].date.slice(0, 7);
  svg.append(firstLabel, lastLabel);
  box.append(svg);
}

function renderReconcileRows(form, preview) {
  const listBox = form.querySelector(".reconcile-list");
  const periods = preview.periods || [];
  const initialRate = preview.history?.initial_annual_rate || form.querySelector(".loan-initial-rate").value.trim();
  const adjustments = [...adjustmentIndexes(periods, initialRate)].map((index) => periods[index]);
  const key = adjustments.map((period) => period.date).join(",");
  if (listBox.dataset.key === key) return;
  listBox.dataset.key = key;
  listBox.replaceChildren();
  if (adjustments.length === 0) {
    const note = document.createElement("p");
    note.className = "muted";
    note.textContent = "当前计划没有利率调整当期；可先添加利率调整。";
    listBox.append(note);
    return;
  }
  const today = localToday();
  for (const period of adjustments) {
    const saved = reconcileValues.get(period.date) || {};
    const row = document.createElement("div");
    row.className = "reconcile-row";
    row.dataset.date = period.date;
    const date = document.createElement("span");
    date.className = "r-date";
    date.textContent = `第 ${period.period} 期`;
    row.append(date);
    const fields = [
      ["月供（元）", "rc-payment", saved.payment ?? centsToYuan(period.payment_cents)],
      ["本金（元）", "rc-principal", saved.principal ?? centsToYuan(period.principal_cents)],
      ["利息（元）", "rc-interest", saved.interest ?? centsToYuan(period.interest_cents)],
      ["余额（可选）", "rc-balance", saved.balance ?? centsToYuan(period.balance_cents)],
    ];
    for (const [label, className, value] of fields) {
      const wrap = document.createElement("div");
      const lab = document.createElement("span");
      lab.className = "r-lab";
      lab.textContent = label;
      const input = inputElement(className, { type: "text", inputmode: "decimal", "aria-label": `${period.date} ${label}` });
      input.value = value;
      wrap.append(lab, input);
      row.append(wrap);
    }
    listBox.append(row);
  }
}

function readReconcileRow(row) {
  return {
    payment: row.querySelector(".rc-payment").value,
    principal: row.querySelector(".rc-principal").value,
    interest: row.querySelector(".rc-interest").value,
    balance: row.querySelector(".rc-balance").value,
  };
}

function reconcileSamples(form) {
  const rows = [...form.querySelectorAll(".reconcile-row")];
  if (rows.length === 0) return { error: "没有可对账的利率调整当期" };
  const samples = [];
  for (const row of rows) {
    const values = readReconcileRow(row);
    const payment = yuanToCents(values.payment);
    const principal = yuanToCents(values.principal);
    const interest = yuanToCents(values.interest);
    if (payment == null || principal == null || interest == null) {
      return { error: `${row.dataset.date} 的月供、本金、利息请用元填写完整` };
    }
    const sample = {
      date: row.dataset.date,
      payment_cents: payment,
      principal_cents: principal,
      interest_cents: interest,
    };
    if (String(values.balance).trim() !== "") {
      const balance = yuanToCents(values.balance);
      if (balance == null) return { error: `${row.dataset.date} 的余额请用元填写，最多两位小数` };
      sample.balance_cents = balance;
    }
    samples.push(sample);
  }
  return { samples };
}

function renderReconcileResult(form, result) {
  const box = form.querySelector(".reconcile-result");
  box.replaceChildren();
  const equalPrincipal = segValue(form, "loan-method") === "equal_principal";
  const table = document.createElement("table");
  table.className = "reconcile-table";
  const head = document.createElement("tr");
  for (const label of ["日期", "结果", "月供差", "本金差", "利息差", "余额差", "算法"]) {
    const th = document.createElement("th");
    th.textContent = label;
    head.append(th);
  }
  table.append(head);
  for (const sample of result.samples || []) {
    const row = document.createElement("tr");
    row.className = sample.matched ? "good" : "bad";
    for (const value of [
      sample.date,
      sample.matched ? "对上" : "未对上",
      signedMoney(sample.payment_diff_cents || 0),
      signedMoney(sample.principal_diff_cents || 0),
      signedMoney(sample.interest_diff_cents || 0),
      sample.balance_diff_cents == null ? "—" : signedMoney(sample.balance_diff_cents),
      equalPrincipal ? "—" : sample.principal_rule ? principalRuleLabels[sample.principal_rule] : "—",
    ]) {
      const td = document.createElement("td");
      td.textContent = value;
      row.append(td);
    }
    table.append(row);
  }
  box.append(table);
  const note = form.querySelector(".reconcile-note");
  note.textContent = `差额 = Mio − 银行 · 已对上 ${result.matched_count || 0}/${(result.samples || []).length}`;
  reconcileSuggestions = result.recommendations || [];
  const apply = form.querySelector(".reconcile-apply");
  apply.hidden = reconcileSuggestions.length === 0;
  if (reconcileSuggestions.length > 0) {
    const listEl = document.createElement("ul");
    listEl.className = "reconcile-suggestions";
    for (const rec of reconcileSuggestions) {
      const item = document.createElement("li");
      item.textContent = equalPrincipal
        ? `${rec.change_date} → ${rec.suggested_date}（等额本金只调整日期）`
        : `${rec.change_date} → ${rec.suggested_date} · ${principalRuleLabels[rec.principal_rule] || rec.principal_rule}`;
      listEl.append(item);
    }
    box.append(listEl);
  }
}

async function runReconcile(form) {
  if (segValue(form, ".loan-mode") !== "history") {
    showLoanError(form, "仅「从头算」模式可以按银行明细对账");
    return;
  }
  const read = readHistoryForm(form);
  if (read.error || !read.values) {
    showLoanError(form, read.error || "请先填写完整的贷款参数");
    return;
  }
  const samples = reconcileSamples(form);
  if (samples.error) {
    showLoanError(form, samples.error);
    return;
  }
  const button = form.querySelector(".reconcile-run");
  button.disabled = true;
  showLoanError(form, "");
  try {
    const result = await call("LoanService.Reconcile", { plan: read.values, samples: samples.samples });
    renderReconcileResult(form, result);
  } catch (error) {
    showLoanError(form, errorText(error));
  } finally {
    button.disabled = false;
  }
}

function applyReconcile(form) {
  let changed = 0;
  for (const rec of reconcileSuggestions) {
    const row = rateRows(form).find((candidate) => candidate.querySelector(".rate-change-date").value === rec.change_date);
    if (!row) continue;
    row.querySelector(".rate-change-date").value = rec.suggested_date;
    row.querySelector(".rate-change-principal").value = rec.principal_rule;
    changed += 1;
  }
  form.querySelector(".reconcile-apply").hidden = true;
  reconcileSuggestions = [];
  if (changed > 0) {
    showStatus("已把对账建议填回表单，请确认后保存");
    schedulePreview();
  } else {
    showLoanError(form, "没有找到对应的利率调整行，请检查当前日期");
  }
}

function schedulePreview() {
  clearTimeout(previewTimer);
  previewTimer = setTimeout(() => previewLoanForm(), 220);
}

async function previewLoanForm() {
  const form = document.querySelector(".loan-form");
  if (!form || !loanAccount) return;
  const mode = segValue(form, ".loan-mode");
  const read = mode === "history" ? readHistoryForm(form) : readAnchorForm(form);
  const save = form.querySelector(".loan-save");
  save.disabled = true;
  showLoanError(form, "");
  if (read.error) {
    clearKpis(form);
    showLoanError(form, read.error);
    return;
  }
  if (!read.values) {
    clearKpis(form);
    return;
  }
  const seq = ++previewSeq;
  try {
    if (mode === "history") {
      const result = await call("LoanService.PreviewHistory", read.values);
      if (seq !== previewSeq || segValue(form, ".loan-mode") !== mode) return;
      applyHistoryPreview(form, result);
    } else {
      const result = await call("LoanService.Preview", read.values);
      if (seq !== previewSeq || segValue(form, ".loan-mode") !== mode) return;
      applyAnchorPreview(form, result, read.values);
    }
    save.disabled = false;
  } catch (error) {
    if (seq !== previewSeq) return;
    clearKpis(form);
    showLoanError(form, errorText(error));
  }
}

function syncLoanMode(form) {
  const history = segValue(form, ".loan-mode") === "history";
  form.querySelector(".loan-anchor-fields").hidden = history;
  form.querySelector(".loan-history-fields").hidden = !history;
  form.querySelector(".loan-history-side").hidden = !history;
  form.querySelector(".reconcile-panel").hidden = !history;
  form.querySelector(".paid").hidden = !history;
  form.classList.toggle("anchor-mode", !history);
  if (!history) form.querySelector(".schedule-panel").hidden = true;
  text(".loan-mode-note", history ? "从头算" : "锚点模式", form);
  const hint = form.querySelector(".loan-hint");
  hint.textContent = history
    ? "保存后从首期开始按此计划重算，并按已过扣款日补齐负债快照。"
    : (loanAccount?.loan ? "从今天起按新条款重算，之前的期次不变。" : "");
  if (history) renderRateChart(form);
}

function buildLoanForm(account) {
  const shell = document.createElement("div");
  shell.innerHTML = `
    <div class="loan-head">
      <div><h1>房贷还款计划 · <span class="loan-account-name"></span></h1><p>逐期重算与银行对账 · 金额单位：元</p></div>
      <span class="faint">CNY</span>
    </div>
    <form class="loan-form" novalidate>
      <section class="panel preview-panel">
        <div class="ph"><h2>实时预览 · <span class="loan-preview-name"></span></h2><span>随输入实时更新</span></div>
        <div class="pv-grid">
          <div><div class="lab">当前月供</div><div class="kv hero num pv-payment">—</div><div class="pv-note pv-payment-note"></div></div>
          <div><div class="lab">剩余本金</div><div class="kv num pv-balance">—</div><div class="pv-note pv-balance-note"></div></div>
          <div><div class="lab">剩余利息</div><div class="kv num pv-interest">—</div><div class="pv-note pv-interest-note"></div></div>
        </div>
        <div class="paid">
          <div><div class="lab">截至今天已还</div><b class="num pv-paid-periods">—</b></div>
          <div><div class="lab">已还本金</div><b class="num pv-paid-principal">—</b></div>
          <div><div class="lab">已还利息</div><b class="num pv-paid-interest">—</b></div>
          <div><div class="lab">合计</div><b class="num pv-paid-total">—</b></div>
        </div>
      </section>
      <div class="form-grid">
        <section class="panel form-panel">
          <div class="form-panel-head"><h2>还款计划参数</h2><span class="loan-mode-note">锚点模式</span></div>
          <div class="loan-fields">
            <div class="loan-field"><span>模式</span><div class="loan-mode-slot"></div></div>
            <div class="loan-field"><span>还款方式</span><div class="loan-method-slot"></div></div>
            <div class="loan-anchor-fields"></div>
            <div class="loan-history-fields"></div>
          </div>
          <p class="loan-hint"></p>
          <p class="loan-error" hidden></p>
        </section>
        <section class="panel form-panel loan-history-side">
          <div class="form-panel-head"><h2>利率调整</h2><span>调整当期按实际天数拆分计息</span></div>
          <div class="rate-head"><span>#</span><span>生效日期</span><span>新年利率</span><span>当期本金</span><span></span></div>
          <div class="rate-change-list"></div>
          <button type="button" class="add-rate">+ 添加利率调整</button>
          <div class="rate-chart"></div>
        </section>
      </div>
      <section class="panel reconcile-panel">
        <div class="ph"><h2>贷款对账</h2><span>用银行 App 的几期数字反推调整日期和当期本金算法</span></div>
        <div class="reconcile-body">
          <p class="reconcile-intro">列出所有利率调整当期。已预填 Mio 当前值，请改成银行 App 的月供、本金、利息（可选余额），Mio 会判断是否一致，并给出日期 / 当期本金的建议。</p>
          <div class="reconcile-list"></div>
          <div class="reconcile-actions">
            <button type="button" class="btn pri reconcile-run">开始对账</button>
            <button type="button" class="btn reconcile-apply" hidden>采用建议</button>
            <span class="reconcile-note faint"></span>
          </div>
          <div class="reconcile-result"></div>
        </div>
      </section>
      <section class="panel schedule-panel">
        <div class="ph"><h2>每期明细</h2><span class="schedule-note"></span></div>
        <div class="schedule-scroll">
          <table class="schedule-table">
            <thead><tr><th class="l">期数</th><th>日期</th><th>利率</th><th>月供</th><th>本金</th><th>利息</th><th>余额</th></tr></thead>
            <tbody class="schedule-body"></tbody>
          </table>
        </div>
      </section>
      <div class="form-foot">
        <p>保存后从首期开始按此计划重算，并按已过扣款日补齐负债快照。</p>
        <div class="buttons">
          <button type="button" class="btn loan-cancel">取消</button>
          <button type="button" class="btn pri loan-save" disabled>确认保存</button>
        </div>
      </div>
    </form>
  `;
  shell.querySelector(".loan-account-name").textContent = account.name;
  shell.querySelector(".loan-preview-name").textContent = account.name;
  const form = shell.querySelector(".loan-form");
  form.dataset.accountId = account.account_id;
  form.dataset.prepayments = JSON.stringify(account.loan?.history?.prepayments || []);

  const existingHistory = account.loan?.history;
  const terms = account.loan?.terms;
  const today = localToday();
  const defaultPrincipal = account.cash_cents != null ? centsToYuan(account.cash_cents) : "";
  const mode = existingHistory ? "history" : "anchor";
  const method = terms?.method || existingHistory?.method || "equal_installment";
  form.querySelector(".loan-mode-slot").append(segControl("loan-mode", [["anchor", "锚点模式"], ["history", "从头算"]], mode));
  form.querySelector(".loan-method-slot").append(segControl("loan-method", [["equal_installment", "等额本息"], ["equal_principal", "等额本金"]], method));

  const anchor = inputElement("loan-anchor", { type: "date" });
  anchor.value = today;
  const principal = inputElement("loan-principal", { type: "text", inputmode: "decimal", placeholder: "1000000.00" });
  principal.value = terms ? centsToYuan(account.loan.balance_cents) : defaultPrincipal;
  const rate = inputElement("loan-rate", { type: "text", inputmode: "decimal", placeholder: "3.10" });
  rate.value = terms?.annual_rate || "";
  const day = inputElement("loan-day", { type: "number", min: 1, max: 28 });
  day.value = String(terms?.payment_day || 1);
  const periods = inputElement("loan-periods", { type: "number", min: 1 });
  periods.value = terms ? String(account.loan.remaining_periods) : "";
  form.querySelector(".loan-anchor-fields").append(
    loanField("起算日", anchor),
    loanField("起算日剩余本金（元）", principal),
    loanField("年利率（%）", rate),
    loanField("每月扣款日（1–28）", day),
    loanField("剩余期数", periods),
  );

  const originalPrincipal = inputElement("loan-principal-history", { type: "text", inputmode: "decimal", placeholder: "1000000.00" });
  originalPrincipal.value = existingHistory ? centsToYuan(existingHistory.original_principal_cents) : "";
  const disbursement = inputElement("loan-disbursement", { type: "date" });
  disbursement.value = existingHistory?.disbursement_date || "";
  const firstDate = inputElement("loan-first-date", { type: "date" });
  firstDate.value = existingHistory?.first_payment_date || today;
  const totalPeriods = inputElement("loan-total-periods", { type: "number", min: 1 });
  totalPeriods.value = existingHistory ? String(existingHistory.total_periods) : "";
  const initialRate = inputElement("loan-initial-rate", { type: "text", inputmode: "decimal", placeholder: "4.90" });
  initialRate.value = existingHistory?.initial_annual_rate || "";
  form.querySelector(".loan-history-fields").append(
    loanField("原始本金（元）", originalPrincipal),
    loanField("放款日（可选）", disbursement),
    loanField("首期还款日", firstDate),
    loanField("总期数", totalPeriods),
    loanField("初始年利率（%）", initialRate),
  );

  for (const change of existingHistory?.rate_changes || []) appendRateRow(form, change);
  if (!existingHistory?.rate_changes?.length) renumberRates(form);

  form.addEventListener("click", (event) => {
    const segButton = event.target.closest(".seg button");
    if (segButton) {
      const seg = segButton.closest(".seg");
      setSeg(seg, segButton.dataset.seg);
      if (seg.classList.contains("loan-mode")) syncLoanMode(form);
      schedulePreview();
    }
  });
  form.addEventListener("input", (event) => {
    if (event.target.closest(".reconcile-panel")) {
      const row = event.target.closest(".reconcile-row");
      if (row) reconcileValues.set(row.dataset.date, readReconcileRow(row));
      return;
    }
    if (event.target.closest(".rate-change-row")) renderRateChart(form);
    schedulePreview();
  });
  form.addEventListener("change", (event) => {
    if (event.target.closest(".reconcile-panel")) return;
    schedulePreview();
  });
  form.querySelector(".add-rate").addEventListener("click", () => {
    appendRateRow(form, {});
    const last = form.querySelector(".rate-change-row:last-child .rate-change-date");
    last?.focus();
    schedulePreview();
  });
  form.querySelector(".loan-cancel").addEventListener("click", () => {
    closeLoanView();
    showView("overview");
  });
  form.querySelector(".loan-save").addEventListener("click", () => saveLoan(form));
  form.querySelector(".reconcile-run").addEventListener("click", () => runReconcile(form));
  form.querySelector(".reconcile-apply").addEventListener("click", () => applyReconcile(form));

  syncLoanMode(form);
  return shell;
}

async function saveLoan(form) {
  const mode = segValue(form, ".loan-mode");
  const read = mode === "history" ? readHistoryForm(form) : readAnchorForm(form);
  if (read.error) {
    showLoanError(form, read.error);
    return;
  }
  if (!read.values) {
    showLoanError(form, "请先填写完整的贷款参数");
    return;
  }
  const save = form.querySelector(".loan-save");
  save.disabled = true;
  showLoanError(form, "");
  try {
    await call(mode === "history" ? "LoanService.SaveHistory" : "LoanService.Save", read.values);
    closeLoanView();
    await showView("overview");
    showStatus("还款计划已保存，已同步该账户负债快照");
  } catch (error) {
    showLoanError(form, errorText(error));
    save.disabled = false;
  }
}

function openLoanView(account) {
  loanAccount = account;
  reconcileValues = new Map();
  reconcileSuggestions = [];
  loanHost.replaceChildren(buildLoanForm(account));
  navLoan.hidden = false;
  showView("loan");
  previewLoanForm();
  window.scrollTo(0, 0);
}

function closeLoanView() {
  loanAccount = null;
  reconcileValues = new Map();
  reconcileSuggestions = [];
  loanHost.replaceChildren();
  navLoan.hidden = true;
}

/* ---------- efund import ---------- */

function efundNode(tag, className, value) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (value != null) node.textContent = value;
  return node;
}

function efundMetaItem(label, value, bad = false) {
  const span = efundNode("span", "meta-item");
  span.append(efundNode("span", "meta-label", label));
  span.append(efundNode("strong", bad ? "bad" : "", value));
  return span;
}

function efundSummaryItem(label, value, note, mono = false) {
  const item = efundNode("div", "summary-item");
  item.append(
    efundNode("span", "summary-label", label),
    efundNode("strong", "summary-value", value),
    efundNode("span", `summary-note${mono ? " num" : ""}`, note),
  );
  return item;
}

function setEfundStep(step) {
  const order = ["select", "preview", "done"];
  const current = Math.max(0, order.indexOf(step));
  for (const [index, item] of efundSteps.entries()) {
    const done = index < current || (step === "done" && index === current);
    item.classList.toggle("done", done);
    item.classList.toggle("on", index === current && step !== "done");
    if (index === current) item.setAttribute("aria-current", "step");
    else item.removeAttribute("aria-current");
  }
}

async function efundToBase64(file) {
  const bytes = new Uint8Array(await file.arrayBuffer());
  let binary = "";
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode(...bytes.subarray(i, Math.min(i + chunk, bytes.length)));
  }
  return btoa(binary);
}

function markEfundDirty() {
  showStatus("映射有改动，点击「保存映射」或直接导入即可保存");
}

function resetEfundSession() {
  const token = efundPreview?.token;
  clearTimeout(efundSaveTimer);
  efundFile = null;
  efundPreview = null;
  efundImportResult = null;
  efundOutcomes = new Map();
  efundBusy = false;
  efundRefreshError = "";
  efundFileInput.value = "";
  efundFileName.textContent = "未选择文件";
  efundPasswordWrap.hidden = true;
  efundPassword.value = "";
  efundParse.disabled = true;
  efundResultRoot.replaceChildren();
  efundUpload.hidden = false;
  setEfundStep("select");
  if (token) void call("EfundService.ClearSession", token).catch(() => {});
}

async function parseEfundFile() {
  if (!efundFile) return;
  const file = efundFile;
  if (file.size > efundMaxFileBytes) {
    showStatus("文件过大：超过 32 MB 上限", "error");
    return;
  }
  efundParse.disabled = true;
  efundPreview = null;
  efundImportResult = null;
  efundOutcomes = new Map();
  efundRefreshError = "";
  efundResultRoot.replaceChildren();
  efundUpload.hidden = false;
  setEfundStep("select");
  showStatus("正在解析文件…", "loading");
  let password = efundPassword.value;
  efundPassword.value = "";
  try {
    const encoded = await efundToBase64(file);
    const preview = await call("EfundService.Preview", encoded, password, file.name);
    password = "";
    if (efundFile !== file) return;
    efundPreview = preview;
    efundImportResult = null;
    renderEfund(efundPreview);
    showStatus("解析完成");
  } catch (error) {
    showStatus(errorText(error), "error");
  } finally {
    password = "";
    efundParse.disabled = !efundFile;
  }
}

function efundSelections() {
  return [...efundResultRoot.querySelectorAll(".efund-map-row")].map((row) => {
    const accountSelect = row.querySelector(".efund-account-select");
    const portfolioSelect = row.querySelector(".efund-portfolio-select");
    const selection = {
      sales_org: row.dataset.sales,
      trade_account: row.dataset.trade,
      mode: "unmapped",
    };
    if (accountSelect.value === "ignore") {
      selection.mode = "ignore";
      return selection;
    }
    if (accountSelect.value === "new") {
      selection.mode = "map";
      selection.account_name = row.querySelector(".efund-account-name").value.trim();
    } else if (accountSelect.value.startsWith("account:")) {
      selection.mode = "map";
      selection.account_id = accountSelect.value.slice("account:".length);
    } else {
      return selection;
    }
    const portfolio = portfolioSelect?.value || "none";
    if (portfolio === "direct") selection.direct = true;
    else if (portfolio === "new") selection.portfolio_name = row.querySelector(".efund-portfolio-name").value.trim();
    else if (portfolio.startsWith("portfolio:")) selection.portfolio_id = portfolio.slice("portfolio:".length);
    return selection;
  });
}

async function saveEfundMapping(silent = false) {
  if (!efundPreview || efundBusy) return false;
  const token = efundPreview.token;
  const selections = efundSelections();
  if (selections.some((selection) => selection.mode === "map" && !selection.account_id && !selection.account_name)) {
    showStatus("请填写新账户名称", "error");
    return false;
  }
  if (selections.some(efundSelectionIncomplete)) {
    showStatus("请选择组合或直接记在账户上", "error");
    return false;
  }
  try {
    const preview = await call("EfundService.Remap", token, selections);
    if (!efundPreview || efundPreview.token !== token) return false;
    efundPreview = preview;
    renderEfund(efundPreview);
    if (!silent) {
      showStatus(efundPreview.mapping_error ? `映射未保存：${efundPreview.mapping_error}` : "映射已保存");
    }
    return true;
  } catch (error) {
    showStatus(errorText(error), "error");
    return false;
  }
}

function scheduleEfundSave() {
  if (efundBusy) return;
  clearTimeout(efundSaveTimer);
  efundSaveTimer = setTimeout(async () => {
    const selections = efundSelections();
    const incomplete = selections.some(efundSelectionIncomplete);
    if (!incomplete) await saveEfundMapping(true);
  }, 350);
}

function efundSelectionIncomplete(selection) {
  if (selection.mode !== "map") return false;
  if (!selection.account_id && !selection.account_name) return true;
  return !selection.direct && !selection.portfolio_id && !selection.portfolio_name;
}

function efundModeBadge(group) {
  if (group.mode === "ignore") return efundNode("span", "efund-tag muted-tag", "已忽略");
  if (group.mode === "new") return efundNode("span", "efund-tag warn-tag", "新建账户");
  if (group.mode === "existing") return efundNode("span", "efund-tag", "已关联");
  return efundNode("span", "efund-tag warn-tag", "未映射");
}

function efundStateBadge(state) {
  if (state === "applied") return efundNode("span", "efund-tag ok-tag", "✓ 已导入");
  if (state === "skipped") return efundNode("span", "efund-tag muted-tag", "之前已导入");
  if (state === "pending") return efundNode("span", "efund-tag warn-tag", "待确认");
  if (state === "failed") return efundNode("span", "efund-tag warn-tag", "导入失败");
  return null;
}

function efundGroupBadge(group, preview) {
  const { state } = groupState(group, preview.targets, efundOutcomes);
  return efundStateBadge(state) || efundModeBadge(group);
}

function renderEfundGroup(group, preview) {
  const card = efundNode("article", "efund-group");
  const head = efundNode("div", "efund-group-head");
  const left = efundNode("div");
  left.append(
    efundNode("h3", "", `${group.sales_org} · ${group.trade_account}`),
    efundNode("p", "efund-sub", `${group.name} · ${group.fund_count} 只 · ${money(group.market_value_cents)}`),
  );
  if (group.top_funds.length) {
    left.append(efundNode("p", "efund-sub", `前几只：${group.top_funds.join("、")}`));
  }
  head.append(left, efundGroupBadge(group, preview));
  card.append(head);

  const row = efundNode("div", "efund-map-row");
  row.dataset.sales = group.sales_org;
  row.dataset.trade = group.trade_account;
  row.append(efundNode("label", "", "账户"));
  const accountSelect = efundNode("select", "efund-select efund-account-select");
  const topAccounts = preview.accounts.filter((account) => !account.parent_id);
  const options = [
    ["unmapped", "未选择"],
    ["ignore", "本次忽略"],
    ["new", "新建账户"],
  ];
  for (const [value, label] of options) {
    const option = document.createElement("option");
    option.value = value;
    option.textContent = label;
    accountSelect.append(option);
  }
  for (const account of topAccounts) {
    const option = document.createElement("option");
    option.value = `account:${account.id}`;
    option.textContent = account.name;
    accountSelect.append(option);
  }
  if (group.mode === "ignore") accountSelect.value = "ignore";
  else if (group.mode === "existing") accountSelect.value = `account:${group.top_account_id}`;
  else accountSelect.value = "unmapped";
  row.append(accountSelect);

  const nameInput = document.createElement("input");
  nameInput.type = "text";
  nameInput.className = "efund-account-name";
  nameInput.placeholder = "新账户名称";
  nameInput.hidden = accountSelect.value !== "new";
  row.append(nameInput);

  const portfolioSelect = efundNode("select", "efund-select efund-portfolio-select");
  const portfolioName = document.createElement("input");
  portfolioName.type = "text";
  portfolioName.className = "efund-portfolio-name";
  portfolioName.placeholder = "新组合名称";
  const portfolioNote = efundNode("span", "efund-hint");
  row.append(efundNode("label", "", "组合"), portfolioSelect, portfolioName, portfolioNote);

  function currentTop() {
    if (accountSelect.value.startsWith("account:")) {
      return preview.accounts.find((account) => account.id === accountSelect.value.slice("account:".length)) || null;
    }
    return null;
  }

  function fillPortfolioOptions(initialValue) {
    const top = currentTop();
    const choices = portfolioChoices(top);
    portfolioSelect.replaceChildren();
    const none = document.createElement("option");
    none.value = "none";
    none.textContent = "请选择组合";
    portfolioSelect.append(none);
    if (accountSelect.value === "new" || choices.allowDirect) {
      const direct = document.createElement("option");
      direct.value = "direct";
      direct.textContent = "直接记在账户上";
      portfolioSelect.append(direct);
    }
    if (accountSelect.value === "new" || choices.allowNew) {
      const create = document.createElement("option");
      create.value = "new";
      create.textContent = "新建组合";
      portfolioSelect.append(create);
    }
    portfolioNote.textContent = choices.note;
    portfolioNote.hidden = !choices.note;
    if (top) {
      for (const portfolio of preview.accounts.filter((account) => account.parent_id === top.id)) {
        const option = document.createElement("option");
        option.value = `portfolio:${portfolio.id}`;
        option.textContent = portfolio.name;
        portfolioSelect.append(option);
      }
    }
    if (initialValue && [...portfolioSelect.options].some((option) => option.value === initialValue)) {
      portfolioSelect.value = initialValue;
    } else if (accountSelect.value === "new") {
      portfolioSelect.value = "direct";
    } else if (portfolioSelect.value === "direct" && top?.has_children) {
      portfolioSelect.value = "none";
    }
    updatePortfolioInputs();
  }

  function updatePortfolioInputs() {
    const mappingVisible = accountSelect.value.startsWith("account:") || accountSelect.value === "new";
    portfolioSelect.hidden = !mappingVisible;
    portfolioName.hidden = !mappingVisible || portfolioSelect.value !== "new";
    if (!portfolioName.hidden) portfolioName.focus();
  }

  const initialPortfolio = group.direct
    ? "direct"
    : group.portfolio_id
      ? `portfolio:${group.portfolio_id}`
      : "none";
  fillPortfolioOptions(initialPortfolio);
  if (group.issue) row.append(efundNode("span", "efund-issue", group.issue));
  card.append(row);

  accountSelect.addEventListener("change", () => {
    nameInput.hidden = accountSelect.value !== "new";
    if (!nameInput.hidden) nameInput.focus();
    fillPortfolioOptions("");
    markEfundDirty();
    scheduleEfundSave();
  });
  nameInput.addEventListener("change", () => {
    markEfundDirty();
    scheduleEfundSave();
  });
  portfolioSelect.addEventListener("change", () => {
    updatePortfolioInputs();
    markEfundDirty();
    scheduleEfundSave();
  });
  portfolioName.addEventListener("change", () => {
    markEfundDirty();
    scheduleEfundSave();
  });

  if (group.holdings.length) {
    const details = efundNode("details", "efund-details");
    details.append(efundNode("summary", "", `持仓明细（${group.holdings.length} 条）`));
    const table = efundNode("table", "efund-holdings");
    const thead = document.createElement("thead");
    const headRow = document.createElement("tr");
    for (const label of ["基金", "代码", "份额", "净值", "市值", "净值日期"]) {
      headRow.append(efundNode("th", "", label));
    }
    thead.append(headRow);
    const tbody = document.createElement("tbody");
    for (const holding of group.holdings) {
      const tr = document.createElement("tr");
      tr.append(
        efundNode("td", "", holding.fund_name),
        efundNode("td", "num", holding.fund_code),
        efundNode("td", "num", holding.quantity),
        efundNode("td", "num", holding.nav),
        efundNode("td", "num", money(holding.market_value_cents)),
        efundNode("td", "num", holding.nav_date),
      );
      tbody.append(tr);
    }
    table.append(thead, tbody);
    details.append(table);
    card.append(details);
  }
  return card;
}

function renderEfundTarget(target, preview) {
  const outcome = efundOutcomes.get(target.account_id) || null;
  const state = targetState(target, outcome);
  const card = efundNode("article", "efund-target");
  const head = efundNode("div", "efund-target-head");
  const left = efundNode("div");
  const title = efundNode("h3", "", target.account_name);
  if (target.group_count > 1) title.append(efundNode("span", "efund-tag muted-tag", `合并 ${target.group_count} 个交易账户`));
  const badge = efundStateBadge(state);
  if (badge) title.append(badge);
  if (state === "skipped" && target.imported_status) {
    title.append(efundNode("span", "efund-tag muted-tag", target.imported_status));
  }
  left.append(title);
  left.append(efundNode("p", "efund-sub",
    `${target.fund_count} 只基金 · ${money(target.market_value_cents)}${target.previous_as_of ? ` · 对比 ${target.previous_as_of}` : " · 无历史快照"}`));
  if (target.cash_cents != null) {
    left.append(efundNode("p", "efund-sub", `现金沿用上次：${money(target.cash_cents)}`));
  }
  head.append(left);
  card.append(head);

  const diff = efundNode("ul", "diff");
  renderDiff(diff, target.diff);
  card.append(diff);

  if (target.holdings.length) {
    const details = efundNode("details", "efund-details");
    details.append(efundNode("summary", "", `账户持仓（合并后 ${target.holdings.length} 条）`));
    const table = efundNode("table", "efund-holdings");
    const thead = document.createElement("thead");
    const headRow = document.createElement("tr");
    for (const label of ["基金", "代码", "份额", "市值", "备注"]) {
      headRow.append(efundNode("th", "", label));
    }
    thead.append(headRow);
    const tbody = document.createElement("tbody");
    for (const item of target.holdings) {
      const tr = document.createElement("tr");
      tr.append(
        efundNode("td", "", item.name),
        efundNode("td", "num", item.code || ""),
        efundNode("td", "num", item.quantity || ""),
        efundNode("td", "num", money(item.market_value_cents)),
        efundNode("td", "", item.note || ""),
      );
      tbody.append(tr);
    }
    table.append(thead, tbody);
    details.append(table);
    card.append(details);
  }

  if (target.anomalies.length) {
    const warning = efundNode("div", "efund-warning");
    warning.append(efundNode("strong", "", "导入后会进入待确认："));
    for (const anomaly of target.anomalies) {
      warning.append(efundNode("div", "", `· ${anomaly.message}`));
    }
    card.append(warning);
  }
  const actions = efundNode("div", "efund-actions");
  if (target.group_count > 1) {
    actions.append(efundNode("span", "efund-hint", `该目标合并 ${target.group_count} 个交易账户，会一并导入`));
  }
  if (state === "applied" && outcome?.snapshot_id) {
    const undo = efundNode("button", "btn efund-undo-run", "撤销本次导入");
    undo.type = "button";
    undo.disabled = efundBusy;
    undo.addEventListener("click", () => undoEfund(outcome));
    actions.append(undo);
  } else if (state === "pending") {
    const go = efundNode("button", "btn text", "去待确认");
    go.type = "button";
    go.addEventListener("click", () => showView("pending"));
    actions.append(go);
    const done = efundNode("button", "btn", "已进入待确认");
    done.type = "button";
    done.disabled = true;
    actions.append(done);
  } else if (state === "skipped") {
    const done = efundNode("button", "btn efund-import-run", "之前已导入");
    done.type = "button";
    done.disabled = true;
    if (target.imported_status) done.title = `已在 ${target.imported_status} 中`;
    actions.append(done);
  } else if (state === "applied") {
    const done = efundNode("button", "btn efund-import-run", "已导入");
    done.type = "button";
    done.disabled = true;
    actions.append(done);
  } else {
    const button = efundNode("button", `btn ${state === "failed" ? "" : "pri "}efund-import-run`.trim(), state === "failed" ? "重试导入这一组" : "导入并确认这一组");
    button.type = "button";
    button.disabled = preview.blocked_by_data || efundBusy;
    if (preview.blocked_by_data) button.title = preview.block_reason;
    button.addEventListener("click", () => importEfundTarget(target));
    actions.append(button);
  }
  card.append(actions);
  return card;
}

function renderEfundResults() {
  if (!efundOutcomes.size) return null;
  const results = [...efundOutcomes.values()];
  const panel = efundNode("section", "panel efund-results");
  const count = (status) => results.filter((result) => result.status === status).length;
  const head = efundNode("div", "ph");
  head.append(
    efundNode("h2", "", "本次会话导入结果"),
    efundNode("span", "", `${results.length} 个目标已处理`),
  );
  panel.append(head);
  const body = efundNode("div", "efund-results-body");
  body.append(efundNode("p", "efund-result-message",
    `已生效 ${count("applied")} · 待确认 ${count("pending")} · 之前已导入 ${count("skipped")} · 失败 ${count("failed")} · 已撤销 ${count("undone")}`));
  if (efundRefreshError) {
    body.append(efundNode("p", "efund-warning", efundRefreshError));
  }
  for (const result of results) {
    const row = efundNode("div", "efund-result-row");
    const head = efundNode("div", "efund-result-head");
    const left = efundNode("div");
    const title = efundNode("h3", "", result.account_name);
    const labels = {
      applied: ["已生效", "efund-tag"],
      pending: ["待确认", "efund-tag warn-tag"],
      skipped: ["之前已导入", "efund-tag muted-tag"],
      failed: ["失败", "efund-tag warn-tag"],
      undone: ["已撤销", "efund-tag muted-tag"],
    };
    const label = labels[result.status] || [result.status, "efund-tag muted-tag"];
    title.append(efundNode("span", label[1], label[0]));
    left.append(title);
    if (result.message) left.append(efundNode("p", "efund-result-message", result.message));
    for (const anomaly of result.anomalies || []) {
      left.append(efundNode("p", "efund-result-message", `· ${anomaly}`));
    }
    head.append(left);

    const actions = efundNode("div", "efund-actions");
    if (result.status === "applied") {
      const undo = efundNode("button", "btn", "撤销本次快照");
      undo.type = "button";
      undo.disabled = efundBusy;
      undo.addEventListener("click", () => undoEfund(result));
      actions.append(undo);
    } else if (result.status === "pending") {
      const go = efundNode("button", "btn text", "去待确认");
      go.type = "button";
      go.addEventListener("click", () => showView("pending"));
      actions.append(go);
    }
    row.append(head, actions);
    body.append(row);
  }
  for (const warning of efundImportResult?.warnings || []) {
    body.append(efundNode("p", "efund-result-message", warning));
  }
  panel.append(body);
  return panel;
}

async function importEfund() {
  if (efundBusy) return;
  const saved = await saveEfundMapping(true);
  if (!saved) return;
  if (!efundPreview?.can_import) {
    showStatus(efundPreview?.block_reason || "当前不能导入", "error");
    return;
  }
  await runEfundImport(async () => call("EfundService.Import", efundPreview.token));
}

async function importEfundTarget(target) {
  if (efundBusy) return;
  const key = target.key;
  const saved = await saveEfundMapping(true);
  if (!saved) return;
  if (efundPreview?.blocked_by_data) {
    showStatus(efundPreview.block_reason || "文件校验未通过", "error");
    return;
  }
  if (!efundPreview?.targets.some((item) => item.key === key)) {
    showStatus("目标已变化，请保存映射后重试", "error");
    return;
  }
  await runEfundImport(async () => call("EfundService.ImportTarget", efundPreview.token, key));
}

async function runEfundImport(action) {
  const token = efundPreview?.token;
  if (!token) return;
  efundBusy = true;
  efundRefreshError = "";
  efundResultRoot.querySelectorAll("button, select, input").forEach((element) => {
    element.disabled = true;
  });
  showStatus("正在导入…", "loading");
  try {
    const result = await action();
    if (!efundPreview || efundPreview.token !== token) return;
    efundImportResult = result;
    mergeResults(efundOutcomes, efundImportResult?.results);
    try {
      const preview = await call("EfundService.Refresh", token);
      if (!efundPreview || efundPreview.token !== token) return;
      efundPreview = preview;
    } catch (error) {
      efundRefreshError = `预览刷新失败：${errorText(error)}`;
    }
    try {
      await reloadOverview();
    } catch (error) {
      efundRefreshError = `${efundRefreshError}${efundRefreshError ? "；" : ""}总览刷新失败：${errorText(error)}`;
    }
    try {
      await reloadPending();
    } catch (error) {
      efundRefreshError = `${efundRefreshError}${efundRefreshError ? "；" : ""}待确认刷新失败：${errorText(error)}`;
    }
    const summary = efundPreview
      ? importSummary(efundPreview.groups, efundPreview.targets, efundOutcomes)
      : null;
    if (summary?.done) hideStatus();
    else showStatus(efundImportResult.summary || "导入完成");
  } catch (error) {
    showStatus(errorText(error), "error");
  } finally {
    efundBusy = false;
    if (efundPreview?.token === token) renderEfund(efundPreview);
  }
}

async function undoEfundSnapshot(snapshotID, message, onSuccess) {
  if (efundBusy) return;
  const token = efundPreview?.token;
  if (!token || !snapshotID) return;
  efundBusy = true;
  efundRefreshError = "";
  try {
    await call("EfundService.UndoSnapshot", snapshotID);
    onSuccess?.();
    if (!efundPreview || efundPreview.token !== token) return;
    if (efundPreview) {
      try {
        const preview = await call("EfundService.Refresh", token);
        if (!efundPreview || efundPreview.token !== token) return;
        efundPreview = preview;
      } catch (error) {
        efundRefreshError = `预览刷新失败：${errorText(error)}`;
      }
    }
    try {
      await reloadOverview();
    } catch (error) {
      efundRefreshError = `${efundRefreshError}${efundRefreshError ? "；" : ""}总览刷新失败：${errorText(error)}`;
    }
    try {
      await reloadPending();
    } catch (error) {
      efundRefreshError = `${efundRefreshError}${efundRefreshError ? "；" : ""}待确认刷新失败：${errorText(error)}`;
    }
    showStatus(message);
  } catch (error) {
    showStatus(errorText(error), "error");
  } finally {
    efundBusy = false;
    if (efundPreview?.token === token) renderEfund(efundPreview);
  }
}

function undoEfund(result) {
  return undoEfundSnapshot(result.snapshot_id, "已撤销本次快照", () => {
    result.status = "undone";
    result.message = "已撤销该次导入快照";
  });
}

function undoPreviousEfund(entry) {
  const { snapshotID, canUndo } = priorImportUndo(entry);
  if (!canUndo) return;
  const accountID = entry.outcome?.account_id || entry.target?.account_id;
  return undoEfundSnapshot(snapshotID, "已撤销之前那次的导入，可重新导入该组", () => {
    if (accountID) efundOutcomes.delete(accountID);
  });
}

function efundGroupedRows(rows, state) {
  const grouped = new Map();
  for (const row of rows) {
    if (row.state !== state) continue;
    const key = row.outcome?.account_id || row.target?.account_id || `${row.group.sales_org}·${row.group.trade_account}`;
    const entry = grouped.get(key) || {
      name: row.target?.account_name || row.outcome?.account_name || row.group.name,
      outcome: row.outcome,
      target: row.target,
      groups: 0,
      cents: 0,
      message: row.outcome?.message || "",
    };
    entry.groups += 1;
    entry.cents += row.group.market_value_cents || 0;
    grouped.set(key, entry);
  }
  return [...grouped.values()];
}

function renderEfundComplete(preview, summary) {
  const { counts } = summary;
  const panel = efundNode("section", "panel efund-complete");
  const head = efundNode("div", "ph");
  head.append(efundNode("h2", "", "导入完成"), efundNode("span", "", preview.file_name));
  panel.append(head);

  const totals = efundNode("div", "import-summary");
  totals.append(
    efundSummaryItem("已生效", `${counts.applied} 组`, money(counts.appliedCents), true),
    efundSummaryItem("待确认", `${counts.pending} 组`, "需要逐条确认"),
    efundSummaryItem("已忽略", `${counts.ignored} 组`, "未写入数据"),
    efundSummaryItem("之前已导入 / 失败", `${counts.skipped + counts.failed} 组`, "未改变现有快照"),
  );
  panel.append(totals);
  if (efundRefreshError) panel.append(efundNode("p", "efund-warning", efundRefreshError));
  if (preview.mapping_error) {
    panel.append(efundNode("p", "efund-warning warning", `映射未保存：${preview.mapping_error}`));
  }

  const applied = efundGroupedRows(summary.rows, "applied");
  if (applied.length) {
    panel.append(efundNode("h3", "complete-group-title", `已生效 · ${applied.length} 个目标`));
  }
  for (const entry of applied) {
    const row = efundNode("div", "efund-result-row");
    const headRow = efundNode("div", "efund-result-head");
    const left = efundNode("div");
    const title = efundNode("h3", "", entry.name);
    title.append(efundNode("span", "efund-tag ok-tag", "✓ 已导入"));
    left.append(title, efundNode("p", "efund-result-message", `${entry.groups} 组 · ${money(entry.cents)}`));
    headRow.append(left);
    const actions = efundNode("div", "efund-actions");
    if (entry.outcome?.snapshot_id) {
      const undo = efundNode("button", "btn", "撤销");
      undo.type = "button";
      undo.disabled = efundBusy;
      undo.addEventListener("click", () => undoEfund(entry.outcome));
      actions.append(undo);
    }
    row.append(headRow, actions);
    panel.append(row);
  }

  const pending = efundGroupedRows(summary.rows, "pending");
  if (pending.length) {
    panel.append(efundNode("h3", "complete-group-title", `待确认 · ${pending.length} 个目标`));
  }
  for (const entry of pending) {
    const row = efundNode("div", "efund-result-row");
    const headRow = efundNode("div", "efund-result-head");
    const left = efundNode("div");
    const title = efundNode("h3", "", entry.name);
    title.append(efundNode("span", "efund-tag warn-tag", "待确认"));
    left.append(title);
    const message = entry.outcome?.status === "skipped"
      ? "这次导入还在待确认里"
      : entry.message || entry.target?.imported_status;
    if (message) left.append(efundNode("p", "efund-result-message", message));
    headRow.append(left);
    const actions = efundNode("div", "efund-actions");
    const go = efundNode("button", "btn text", "去待确认");
    go.type = "button";
    go.addEventListener("click", () => showView("pending"));
    actions.append(go);
    row.append(headRow, actions);
    panel.append(row);
  }

  const skipped = efundGroupedRows(summary.rows, "skipped");
  if (skipped.length) {
    panel.append(efundNode("h3", "complete-group-title", `之前已导入 · ${skipped.length} 个目标`));
  }
  for (const entry of skipped) {
    const row = efundNode("div", "efund-result-row");
    const headRow = efundNode("div", "efund-result-head");
    const left = efundNode("div");
    const title = efundNode("h3", "", entry.name);
    title.append(efundNode("span", "efund-tag muted-tag", "之前已导入"));
    left.append(title);
    const message = entry.message || entry.target?.imported_status || "之前已导入：该账户已有同日期快照";
    left.append(efundNode("p", "efund-result-message", message));
    const prior = priorImportUndo(entry);
    if (prior.pending) {
      left.append(efundNode("p", "efund-result-message", "这次导入还在待确认里"));
    } else if (prior.source && !prior.canUndo) {
      left.append(efundNode("p", "efund-result-message", "该日快照来自其他来源，不在这里撤销"));
    }
    headRow.append(left);
    const actions = efundNode("div", "efund-actions");
    if (prior.pending) {
      const go = efundNode("button", "btn text", "去待确认");
      go.type = "button";
      go.addEventListener("click", () => showView("pending"));
      actions.append(go);
    } else if (prior.canUndo) {
      const undo = efundNode("button", "btn", "撤销之前导入");
      undo.type = "button";
      undo.disabled = efundBusy;
      undo.addEventListener("click", () => undoPreviousEfund(entry));
      actions.append(undo);
    }
    row.append(headRow, actions);
    panel.append(row);
  }

  const actions = efundNode("div", "efund-actions");
  const home = efundNode("button", "btn", "回首页");
  home.type = "button";
  home.addEventListener("click", () => showView("overview"));
  actions.append(home);
  if (counts.pending > 0) {
    const go = efundNode("button", "btn pri", "去待确认");
    go.type = "button";
    go.addEventListener("click", () => showView("pending"));
    actions.append(go);
  }
  const again = efundNode("button", "btn", "再导入一个文件");
  again.type = "button";
  again.addEventListener("click", () => {
    resetEfundSession();
    efundFileInput.click();
  });
  actions.append(again);
  panel.append(actions);
  return panel;
}

function renderEfund(preview) {
  efundResultRoot.replaceChildren();
  if (!preview) return;
  const summary = importSummary(preview.groups, preview.targets, efundOutcomes);
  efundUpload.hidden = summary.done;
  setEfundStep(summary.done ? "done" : "preview");
  if (summary.done) {
    efundResultRoot.append(renderEfundComplete(preview, summary));
    return;
  }

  const metaPanel = efundNode("section", "panel efund-meta-panel");
  const metaHead = efundNode("div", "ph");
  metaHead.append(efundNode("h2", "", "文件信息"), efundNode("span", "", preview.file_name));
  metaPanel.append(metaHead);
  const meta = efundNode("div", "efund-meta efund-file-meta");
  meta.append(
    efundMetaItem("份额日期", preview.meta.share_date || "—"),
    efundMetaItem("打印时间", preview.meta.print_time || "—"),
  );
  if (preview.meta.total_records == null) {
    meta.append(efundMetaItem("记录数", `未提供，实际 ${preview.meta.actual_rows} 行`, true));
  } else {
    const bad = !preview.meta.records_match;
    const mark = bad ? "✗" : "✓";
    meta.append(efundMetaItem("记录数", `${preview.meta.total_records} / ${preview.meta.actual_rows} ${mark}`, bad));
  }
  if (preview.meta.nav_date_mismatch_funds > 0) {
    meta.append(efundMetaItem("净值日期不一致", `${preview.meta.nav_date_mismatch_funds} 只`, false));
  }
  metaPanel.append(meta);
  const totals = efundNode("div", "efund-meta efund-totals");
  totals.append(
    efundMetaItem("文件总计", money(preview.file_total_cents)),
    efundMetaItem("已映射小计", money(preview.total_market_value_cents)),
    efundMetaItem("已忽略", money(preview.ignored_total_cents)),
    efundMetaItem("未映射", money(preview.unmapped_total_cents)),
  );
  metaPanel.append(totals);

  if (preview.warnings.length) {
    const warnings = efundNode("ul", "efund-warnings");
    for (const warning of preview.warnings) {
      const item = efundNode("li", `efund-warning ${warning.severity}`);
      item.append(
        efundNode("span", "efund-warning-label", warningSeverityLabels[warning.severity] || "提示"),
        document.createTextNode(warning.message),
      );
      warnings.append(item);
    }
    metaPanel.append(warnings);
  }
  efundResultRoot.append(metaPanel);

  const groupsPanel = efundNode("section", "panel efund-groups-panel");
  const groupsHead = efundNode("div", "ph");
  groupsHead.append(efundNode("h2", "", "交易账户"), efundNode("span", "", `${preview.groups.length} 个`));
  groupsPanel.append(groupsHead);
  const unresolved = preview.groups.filter((group) => group.mode === "unmapped");
  const resolved = preview.groups.filter((group) => group.mode !== "unmapped");
  for (const group of unresolved) groupsPanel.append(renderEfundGroup(group, preview));
  if (resolved.length) {
    const details = efundNode("details", "efund-details efund-resolved");
    details.append(efundNode("summary", "", `已映射 / 已忽略（${resolved.length} 个，点击展开）`));
    for (const group of resolved) details.append(renderEfundGroup(group, preview));
    groupsPanel.append(details);
  }
  efundResultRoot.append(groupsPanel);

  if (preview.targets.length) {
    const targetsPanel = efundNode("section", "panel efund-targets-panel");
    const targetsHead = efundNode("div", "ph");
    targetsHead.append(
      efundNode("h2", "", "按账户汇总"),
      efundNode("span", "", `${money(preview.total_market_value_cents)} · ${preview.total_holding_count} 条持仓`),
    );
    targetsPanel.append(targetsHead);
    const targets = efundNode("div", "efund-targets");
    for (const target of preview.targets) targets.append(renderEfundTarget(target, preview));
    targetsPanel.append(targets);
    efundResultRoot.append(targetsPanel);
  }

  const actions = efundNode("section", "panel efund-write-panel");
  const actionsHead = efundNode("div", "ph");
  actionsHead.append(efundNode("h2", "", "写入"), efundNode("span", "", "走现有快照确认流程"));
  actions.append(actionsHead);
  if (!preview.can_import) {
    actions.append(efundNode("p", "efund-block", preview.block_reason || "当前不能导入"));
  } else {
    actions.append(efundNode("p", "efund-hint", "正常会直接生效；命中异常规则会进入「待确认」。"));
  }
  const buttons = efundNode("div", "efund-actions");
  const save = efundNode("button", "btn efund-save-map", "保存映射");
  save.type = "button";
  save.disabled = efundBusy;
  save.addEventListener("click", () => saveEfundMapping(false));
  const run = efundNode("button", "btn pri efund-import-run", "全部导入并确认");
  run.type = "button";
  run.disabled = !preview.can_import || efundBusy || !preview.targets.some((target) => !target.already_imported);
  if (preview.block_reason) run.title = preview.block_reason;
  run.addEventListener("click", importEfund);
  buttons.append(save, run);
  actions.append(buttons);
  efundResultRoot.append(actions);

  const results = renderEfundResults();
  if (results) efundResultRoot.append(results);
}

efundFileInput.addEventListener("change", () => {
  clearTimeout(efundSaveTimer);
  efundFile = efundFileInput.files?.[0] || null;
  efundFileInput.value = "";
  efundPreview = null;
  efundImportResult = null;
  efundOutcomes = new Map();
  efundRefreshError = "";
  efundResultRoot.replaceChildren();
  efundUpload.hidden = false;
  setEfundStep("select");
  efundFileName.textContent = efundFile ? efundFile.name : "未选择文件";
  efundPasswordWrap.hidden = !(efundFile && efundFile.name.toLowerCase().endsWith(".zip"));
  efundPassword.value = "";
  efundParse.disabled = !efundFile;
  if (efundFile) showStatus("文件已选择，点击「解析」");
});

efundParse.addEventListener("click", parseEfundFile);

/* ---------- account management ---------- */

function accountItem(id) {
  if (!accountTree) return null;
  return [...accountTree.accounts, ...accountTree.portfolios].find((item) => item.id === id) || null;
}

function accountLabel(item) {
  if (!item) return "";
  if (!item.parent_id) return `账户：${item.name}`;
  const parent = accountItem(item.parent_id);
  return `组合：${parent?.name || "?"} / ${item.name}`;
}

function showAccountForm(open) {
  const visible = accountFormVisibility(open);
  accountCreatePanel.hidden = !visible.create;
  portfolioCreatePanel.hidden = !visible.portfolio;
  accountEditPanel.hidden = !visible.edit;
}

function openAccountCreate() {
  showAccountForm("create");
  accountCreatePanel.scrollIntoView({ behavior: "smooth", block: "center" });
  accountCreateForm.elements.name.focus({ preventScroll: true });
}

function openPortfolioCreate(parentId) {
  const select = portfolioCreateForm.elements.parent_id;
  if (parentId && [...select.options].some((option) => option.value === parentId)) {
    select.value = parentId;
  }
  showAccountForm("portfolio");
  portfolioCreatePanel.scrollIntoView({ behavior: "smooth", block: "center" });
  portfolioCreateForm.elements.name.focus({ preventScroll: true });
}

function renderAccountTree(tree) {
  accountsTree.replaceChildren();
  if (!tree.accounts.length) {
    const emptyState = efundNode("div", "empty compact");
    emptyState.append(
      efundNode("strong", "", "还没有账户"),
      efundNode("span", "", "点上方「新建账户」开始，再添加组合。"),
    );
    accountsTree.append(emptyState);
    return;
  }
  for (const top of tree.accounts) {
    const block = efundNode("article", "account-block");
    const head = efundNode("div", "account-block-head");
    const left = efundNode("div");
    const title = efundNode("h3", "", top.name);
    if (top.archived) title.append(efundNode("span", "efund-tag muted-tag", "已归档"));
    left.append(title);
    const portfolios = accountPortfolioView(top, tree.portfolios);
    const meta = [typeLabels[top.type] || top.type, top.owner || "未归属"];
    if (portfolios.showCount) meta.push(`${portfolios.count} 个组合`);
    left.append(efundNode("p", "meta-line", meta.join(" · ")));
    head.append(left);
    const figures = efundNode("div", "account-figures");
    figures.append(
      efundNode("span", "account-figures-label", "最新快照合计"),
      efundNode("span", "account-value num", top.latest_value_cents == null ? "—" : money(top.latest_value_cents)),
      efundNode("span", "account-date num", top.latest_as_of || "—"),
    );
    head.append(figures);
    const actions = efundNode("div", "efund-actions");
    const edit = efundNode("button", "btn small", "编辑");
    edit.type = "button";
    edit.addEventListener("click", () => openAccountEditor(top.id));
    const add = efundNode("button", "btn small pri", "新建组合");
    add.type = "button";
    add.addEventListener("click", () => openPortfolioCreate(top.id));
    if (!top.archived && !top.has_snapshots) actions.append(add);
    actions.append(edit);
    head.append(actions);
    block.append(head);

    if (portfolios.items.length) {
      const list = efundNode("ul", "portfolio-list");
      for (const portfolio of portfolios.items) {
        const li = efundNode("li");
        const name = efundNode("span", "", portfolio.name);
        if (portfolio.archived) name.append(efundNode("span", "efund-tag muted-tag", "已归档"));
        if (portfolio.has_snapshots && portfolio.latest_value_cents == null) {
          name.append(efundNode("span", "efund-tag", "有快照"));
        }
        name.append(efundNode("span", "portfolio-meta", ` · ${typeLabels[portfolio.type] || portfolio.type} · ${portfolio.owner || "未归属"}`));
        const figures = efundNode("div", "portfolio-figures");
        figures.append(
          efundNode("span", "account-value num", portfolio.latest_value_cents == null ? "—" : money(portfolio.latest_value_cents)),
          efundNode("span", "account-date num", portfolio.latest_as_of || "—"),
        );
        const button = efundNode("button", "btn small", "编辑");
        button.type = "button";
        button.addEventListener("click", () => openAccountEditor(portfolio.id));
        li.append(name, figures, button);
        list.append(li);
      }
      block.append(list);
    }
    accountsTree.append(block);
  }
}

function fillAccountOptions(tree) {
  ownerOptions.replaceChildren();
  for (const owner of tree.owners) {
    const option = document.createElement("option");
    option.value = owner;
    ownerOptions.append(option);
  }
  const parent = portfolioCreateForm.elements.parent_id;
  parent.replaceChildren();
  for (const top of tree.accounts.filter((item) => !item.archived && !item.has_snapshots)) {
    const option = document.createElement("option");
    option.value = top.id;
    option.textContent = top.name;
    parent.append(option);
  }
  portfolioCreateOpen.disabled = parent.options.length === 0;
  portfolioCreateOpen.title = portfolioCreateOpen.disabled ? "先新建一个没有快照的账户" : "";
  accountEditSelect.replaceChildren();
  for (const item of [...tree.accounts, ...tree.portfolios]) {
    const option = document.createElement("option");
    option.value = item.id;
    option.textContent = accountLabel(item);
    accountEditSelect.append(option);
  }
  syncAccountEdit();
}

function syncAccountEdit() {
  archiveArmed = false;
  const item = accountItem(accountEditSelect.value) || accountItem(accountTree?.accounts[0]?.id);
  if (!item) return false;
  try {
    const values = accountEditValues(item);
    accountEditSelect.value = item.id;
    accountEditForm.classList.toggle("portfolio-mode", values.isPortfolio);
    const ownerNote = accountEditForm.querySelector(".account-owner-note");
    ownerNote.hidden = !values.isPortfolio;
    ownerNote.textContent = accountOwnerNote(item, accountTree?.accounts || []);
    accountEditForm.elements.name.value = values.name;
    const typeSelect = accountEditForm.elements.type;
    if (![...typeSelect.options].some((option) => option.value === values.type)) {
      throw new Error(`编辑表单缺少账户类型选项：${values.type}`);
    }
    typeSelect.value = values.type;
    accountEditForm.elements.currency.value = values.currency;
    accountEditForm.elements.owner.value = values.owner;
    const archive = accountEditForm.querySelector(".account-archive");
    archive.textContent = "归档";
    archive.classList.remove("danger");
    archive.disabled = item.archived || item.has_snapshots || (!values.isPortfolio && item.has_children);
    archive.title = archive.disabled ? "有快照或有组合的对象不能归档" : "";
    return true;
  } catch (error) {
    showStatus(`账户编辑表单加载失败：${errorText(error)}`, "error");
    return false;
  }
}

function openAccountEditor(id) {
  try {
    const item = accountItem(id);
    if (!item) throw new Error("找不到要编辑的账户");
    accountEditSelect.value = id;
    if (!syncAccountEdit()) return;
    showAccountForm("edit");
    accountEditForm.scrollIntoView({ behavior: "smooth", block: "center" });
    accountEditForm.elements.name.focus({ preventScroll: true });
    showStatus(`正在编辑：${accountLabel(item)}`);
  } catch (error) {
    showStatus(`打开编辑表单失败：${errorText(error)}`, "error");
  }
}

async function reloadAccounts() {
  accountTree = await call("AccountService.List");
  renderAccountTree(accountTree);
  fillAccountOptions(accountTree);
  if (efundPreview && !efundBusy) {
    const token = efundPreview.token;
    try {
      const preview = await call("EfundService.Refresh", token);
      if (efundPreview?.token === token) efundPreview = preview;
    } catch (_) {
      // The import preview can be re-parsed if the session expired.
    }
  }
}

accountCreateOpen.addEventListener("click", openAccountCreate);
portfolioCreateOpen.addEventListener("click", () => openPortfolioCreate());

accountCreateForm.querySelector(".account-form-cancel").addEventListener("click", () => {
  accountCreateForm.reset();
  accountCreateForm.elements.currency.value = "CNY";
  showAccountForm(null);
});
portfolioCreateForm.querySelector(".account-form-cancel").addEventListener("click", () => {
  portfolioCreateForm.elements.name.value = "";
  showAccountForm(null);
});
accountEditForm.querySelector(".account-form-cancel").addEventListener("click", () => {
  showAccountForm(null);
});

accountCreateForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const elements = accountCreateForm.elements;
  try {
    await call("AccountService.SaveAccount", {
      name: elements.name.value.trim(),
      type: elements.type.value,
      currency: elements.currency.value.trim() || "CNY",
      owner: elements.owner.value.trim(),
    });
    accountCreateForm.reset();
    accountCreateForm.elements.currency.value = "CNY";
    await reloadAccounts();
    showAccountForm(null);
    showStatus("账户已创建");
  } catch (error) {
    showStatus(errorText(error), "error");
  }
});

portfolioCreateForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const elements = portfolioCreateForm.elements;
  try {
    await call("AccountService.SavePortfolio", {
      parent_id: elements.parent_id.value,
      name: elements.name.value.trim(),
    });
    elements.name.value = "";
    await reloadAccounts();
    showAccountForm(null);
    showStatus("组合已创建");
  } catch (error) {
    showStatus(errorText(error), "error");
  }
});

accountEditSelect.addEventListener("change", syncAccountEdit);

accountEditForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const item = accountItem(accountEditSelect.value);
  if (!item) return;
  const elements = accountEditForm.elements;
  try {
    if (item.parent_id) {
      await call("AccountService.SavePortfolio", { id: item.id, name: elements.name.value.trim() });
    } else {
      await call("AccountService.SaveAccount", {
        id: item.id,
        name: elements.name.value.trim(),
        type: elements.type.value,
        currency: elements.currency.value.trim() || "CNY",
        owner: elements.owner.value.trim(),
      });
    }
    await reloadAccounts();
    showAccountForm(null);
    showStatus("修改已保存");
  } catch (error) {
    showStatus(errorText(error), "error");
  }
});

accountEditForm.querySelector(".account-archive").addEventListener("click", async () => {
  const item = accountItem(accountEditSelect.value);
  if (!item) return;
  const button = accountEditForm.querySelector(".account-archive");
  if (!archiveArmed) {
    archiveArmed = true;
    button.textContent = "再点一次确认归档";
    button.classList.add("danger");
    return;
  }
  try {
    if (item.parent_id) {
      await call("AccountService.SavePortfolio", { id: item.id, archived: true });
    } else {
      await call("AccountService.SaveAccount", { id: item.id, archived: true });
    }
    await reloadAccounts();
    showAccountForm(null);
    showStatus("已归档");
  } catch (error) {
    showStatus(errorText(error), "error");
    syncAccountEdit();
  }
});

/* ---------- navigation ---------- */

async function reloadOverview() {
  renderOverview(await call("OverviewService.Get"));
}

async function showView(view) {
  if (view === "loan" && !loanAccount) view = "overview";
  if (currentView === "efund" && view !== "efund") resetEfundSession();
  currentView = view;
  for (const item of navItems) {
    const active = item.dataset.view === view;
    item.classList.toggle("active", active);
    item.setAttribute("aria-selected", String(active));
  }
  overviewView.hidden = view !== "overview";
  pendingView.hidden = view !== "pending";
  loanView.hidden = view !== "loan";
  efundView.hidden = view !== "efund";
  accountsView.hidden = view !== "accounts";
  hideStatus();
  try {
    if (view === "overview") await reloadOverview();
    else if (view === "pending") await reloadPending();
    else if (view === "accounts") await reloadAccounts();
    else if (view === "efund" && efundPreview) renderEfund(efundPreview);
  } catch (error) {
    showStatus(errorText(error), "error");
  }
}

for (const item of navItems) {
  item.addEventListener("click", () => showView(item.dataset.view));
}

refresh.addEventListener("click", () => {
  if (currentView === "overview") showView("overview");
  else if (currentView === "pending") showView("pending");
  else if (currentView === "efund") showStatus("导入页不需要刷新，可继续当前预览");
  else if (currentView === "accounts") showView("accounts");
  else previewLoanForm();
});

showView("overview");
