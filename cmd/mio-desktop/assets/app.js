if (window.location.protocol === "wails:" || window.location.hostname === "wails.localhost") {
  await import("/wails/runtime.js");
}

const overviewView = document.getElementById("overview-view");
const pendingView = document.getElementById("pending-view");
const loanView = document.getElementById("loan-view");
const efundView = document.getElementById("efund-view");
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

const sourceLabels = { mcp: "Agent 提交", screenshot: "截图", statement: "对账单", manual: "手动", efund_import: "基金E账户" };
const typeLabels = { cash: "现金", fund: "基金", stock: "股票", mixed: "混合", liability: "负债", other: "其他" };
const typeColors = { stock: "#E07A5F", fund: "#6E8B5B", cash: "#D4A64A", mixed: "#C98A2B", other: "#8D7B66" };
const principalRuleLabels = { new_schedule: "新月供本金", fixed_payment: "月供不变", old_schedule: "旧月供本金" };
const moneyFormat = new Intl.NumberFormat("zh-CN", {
  style: "currency",
  currency: "CNY",
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

let currentView = "overview";
let busy = false;
let loanAccount = null;
let previewSeq = 0;
let previewTimer = 0;
let reconcileValues = new Map();
let reconcileSuggestions = [];
let efundFile = null;
let efundPreview = null;
let efundImportResult = null;

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
  statusBox.textContent = text;
  statusBox.className = `status ${kind}`;
  statusBox.hidden = false;
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
  for (const account of accounts) {
    const row = document.createElement("tr");
    const name = document.createElement("span");
    name.className = "account-name";
    name.textContent = account.name;
    const tag = document.createElement("span");
    tag.className = "tag";
    tag.textContent = typeLabels[account.type] || account.type;
    name.append(tag);
    row.append(cell(row, name, "l"));
    const value = cell(row, money(account.value_cents), `num ${account.type === "liability" ? "neg" : ""}`);
    row.append(value);

    const share = document.createElement("span");
    share.className = "num";
    share.textContent = `${account.share_percent.toFixed(1)}%`;
    const bar = document.createElement("span");
    bar.className = "shbar";
    const fillBar = document.createElement("i");
    if (account.type === "liability") fillBar.className = "n";
    fillBar.style.width = `${Math.max(1, Math.min(60, Math.abs(account.share_percent) * 0.6))}px`;
    bar.append(fillBar);
    share.append(bar);
    row.append(cell(row, share, "num"));
    row.append(cell(row, account.as_of || "—", "num faint"));
    const action = document.createElement("td");
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

    for (const item of account.items) {
      const sub = document.createElement("tr");
      sub.className = "sub";
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
    if (account.items.length === 0 && account.cash_cents == null) {
      const sub = document.createElement("tr");
      sub.className = "sub";
      sub.append(cell(sub, "还没有持仓明细", "l muted"), cell(sub, ""), cell(sub, ""), cell(sub, ""), cell(sub, ""));
      accountsBody.append(sub);
    }
    if (account.cash_cents != null) {
      const sub = document.createElement("tr");
      sub.className = "sub";
      sub.append(cell(sub, "现金", "l"));
      sub.append(cell(sub, money(account.cash_cents), "num"));
      sub.append(cell(sub, ""), cell(sub, ""), cell(sub, ""));
      accountsBody.append(sub);
    }

    if (account.loan) {
      const sub = document.createElement("tr");
      sub.className = "sub loan-sub";
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
}

function renderAllocation(overview) {
  allocationBox.replaceChildren();
  const totals = new Map();
  for (const account of overview.accounts) {
    if (account.type === "liability" || account.value_cents <= 0) continue;
    totals.set(account.type, (totals.get(account.type) || 0) + account.value_cents);
  }
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
  const span = efundNode("span");
  span.append(document.createTextNode(`${label} `));
  span.append(efundNode("strong", bad ? "bad" : "", value));
  return span;
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

async function parseEfundFile() {
  if (!efundFile) return;
  efundParse.disabled = true;
  efundPreview = null;
  efundImportResult = null;
  efundResultRoot.replaceChildren();
  showStatus("正在解析文件…");
  let password = efundPassword.value;
  efundPassword.value = "";
  try {
    const encoded = await efundToBase64(efundFile);
    efundPreview = await call("EfundService.Preview", encoded, password, efundFile.name);
    password = "";
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
    const select = row.querySelector(".efund-select");
    const selection = {
      sales_org: row.dataset.sales,
      trade_account: row.dataset.trade,
      mode: "unmapped",
    };
    if (select.value === "ignore") {
      selection.mode = "ignore";
    } else if (select.value === "new") {
      selection.mode = "new";
      selection.account_name = row.querySelector(".efund-account-name").value.trim();
    } else if (select.value.startsWith("existing:")) {
      selection.mode = "existing";
      selection.account_id = select.value.slice("existing:".length);
    }
    return selection;
  });
}

async function saveEfundMapping(silent = false) {
  if (!efundPreview) return false;
  const selections = efundSelections();
  if (selections.some((selection) => selection.mode === "new" && !selection.account_name)) {
    showStatus("请填写新账户名称", "error");
    return false;
  }
  try {
    efundPreview = await call("EfundService.Remap", efundPreview.token, selections);
    renderEfund(efundPreview);
    if (!silent) showStatus("映射已保存");
    return true;
  } catch (error) {
    showStatus(errorText(error), "error");
    return false;
  }
}

function efundModeBadge(group) {
  if (group.mode === "ignore") return efundNode("span", "efund-tag muted-tag", "已忽略");
  if (group.mode === "new") return efundNode("span", "efund-tag warn-tag", "新建账户");
  if (group.mode === "existing") return efundNode("span", "efund-tag", "已关联");
  return efundNode("span", "efund-tag warn-tag", "未映射");
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
  head.append(left, efundModeBadge(group));
  card.append(head);

  const row = efundNode("div", "efund-map-row");
  row.dataset.sales = group.sales_org;
  row.dataset.trade = group.trade_account;
  row.append(efundNode("label", "", "映射到"));
  const select = efundNode("select", "efund-select");
  const options = [
    ["unmapped", "未选择"],
    ["ignore", "本次忽略"],
    ["new", "新建账户"],
  ];
  for (const [value, label] of options) {
    const option = document.createElement("option");
    option.value = value;
    option.textContent = label;
    select.append(option);
  }
  for (const account of preview.accounts) {
    const option = document.createElement("option");
    option.value = `existing:${account.id}`;
    option.textContent = `${account.name}${account.platform ? ` · ${account.platform}` : ""}`;
    select.append(option);
  }
  if (group.mode === "ignore") select.value = "ignore";
  else if (group.mode === "new") select.value = "new";
  else if (group.mode === "existing") select.value = `existing:${group.account_id}`;
  else select.value = "unmapped";
  row.append(select);

  const nameInput = document.createElement("input");
  nameInput.type = "text";
  nameInput.className = "efund-account-name";
  nameInput.placeholder = `新账户名称（平台：${group.sales_org}）`;
  nameInput.value = group.mode === "new" ? group.account_name : "";
  nameInput.hidden = group.mode !== "new";
  row.append(nameInput);
  if (group.issue) row.append(efundNode("span", "efund-issue", group.issue));
  card.append(row);

  select.addEventListener("change", () => {
    nameInput.hidden = select.value !== "new";
    if (!nameInput.hidden) nameInput.focus();
    markEfundDirty();
  });
  nameInput.addEventListener("input", markEfundDirty);
  nameInput.addEventListener("change", markEfundDirty);

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

function renderEfundTarget(target) {
  const card = efundNode("article", "efund-target");
  const head = efundNode("div", "efund-target-head");
  const left = efundNode("div");
  const title = efundNode("h3", "", target.account_name);
  if (target.new_account) title.append(efundNode("span", "efund-tag warn-tag", "新建"));
  if (target.group_count > 1) title.append(efundNode("span", "efund-tag muted-tag", `合并 ${target.group_count} 个交易账户`));
  if (target.already_imported) title.append(efundNode("span", "efund-tag muted-tag", `已导入过（${target.imported_status}）`));
  left.append(title);
  left.append(efundNode("p", "efund-sub",
    `${target.fund_count} 只基金 · ${money(target.market_value_cents)}${target.previous_as_of ? ` · 对比 ${target.previous_as_of}` : " · 无历史快照"}`));
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
  return card;
}

function renderEfundResults() {
  if (!efundImportResult) return null;
  const panel = efundNode("section", "panel efund-results");
  panel.append(efundNode("h3", "", "导入结果"));
  for (const result of efundImportResult.results) {
    const row = efundNode("div", "efund-result-row");
    const head = efundNode("div", "efund-result-head");
    const left = efundNode("div");
    const title = efundNode("h3", "", result.account_name);
    const labels = {
      applied: ["已生效", "efund-tag"],
      pending: ["待确认", "efund-tag warn-tag"],
      skipped: ["已跳过", "efund-tag muted-tag"],
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
      const undo = efundNode("button", "btn", "撤销最近确认");
      undo.type = "button";
      undo.addEventListener("click", () => undoEfund(result));
      actions.append(undo);
    } else if (result.status === "pending") {
      const go = efundNode("button", "btn", "去待确认");
      go.type = "button";
      go.addEventListener("click", () => showView("pending"));
      actions.append(go);
    }
    row.append(head, actions);
    panel.append(row);
  }
  for (const warning of efundImportResult.warnings || []) {
    panel.append(efundNode("p", "efund-result-message", warning));
  }
  return panel;
}

async function importEfund() {
  const saved = await saveEfundMapping(true);
  if (!saved) return;
  if (!efundPreview?.can_import) {
    showStatus(efundPreview?.block_reason || "当前不能导入", "error");
    return;
  }
  showStatus("正在导入…");
  try {
    efundImportResult = await call("EfundService.Import", efundPreview.token);
    efundPreview = await call("EfundService.Refresh", efundPreview.token);
    renderEfund(efundPreview);
    await reloadOverview();
    try {
      await reloadPending();
    } catch (_) {
      // Pending view is refreshed on demand if it cannot be loaded now.
    }
    showStatus("导入完成");
  } catch (error) {
    showStatus(errorText(error), "error");
  }
}

async function undoEfund(result) {
  try {
    await call("EfundService.Undo", result.account_id);
    result.status = "undone";
    result.message = "已撤销该账户最近一次确认快照";
    if (efundPreview) {
      efundPreview = await call("EfundService.Refresh", efundPreview.token);
      renderEfund(efundPreview);
    }
    await reloadOverview();
    try {
      await reloadPending();
    } catch (_) {
      // Ignore a pending refresh failure; the overview still updates.
    }
    showStatus("已撤销最近一次确认快照");
  } catch (error) {
    showStatus(errorText(error), "error");
  }
}

function renderEfund(preview) {
  efundResultRoot.replaceChildren();
  if (!preview) return;

  const metaPanel = efundNode("section", "panel");
  const metaHead = efundNode("div", "ph");
  metaHead.append(efundNode("h2", "", "文件信息"), efundNode("span", "", preview.file_name));
  metaPanel.append(metaHead);
  const meta = efundNode("div", "efund-meta");
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

  if (preview.warnings.length) {
    const warnings = efundNode("ul", "efund-warnings");
    for (const warning of preview.warnings) {
      const text = `[${warning.severity}] ${warning.message}` +
        (warning.severity === "blocks_write" ? "（写入时将被阻止）" : "");
      warnings.append(efundNode("li", `efund-warning ${warning.severity}`, text));
    }
    metaPanel.append(warnings);
  }
  efundResultRoot.append(metaPanel);

  const groupsPanel = efundNode("section", "panel");
  const groupsHead = efundNode("div", "ph");
  groupsHead.append(efundNode("h2", "", "交易账户"), efundNode("span", "", `${preview.groups.length} 个`));
  groupsPanel.append(groupsHead);
  for (const group of preview.groups) groupsPanel.append(renderEfundGroup(group, preview));
  efundResultRoot.append(groupsPanel);

  if (preview.targets.length) {
    const targetsPanel = efundNode("section", "panel");
    const targetsHead = efundNode("div", "ph");
    targetsHead.append(
      efundNode("h2", "", "按账户汇总"),
      efundNode("span", "", `${money(preview.total_market_value_cents)} · ${preview.total_holding_count} 条持仓`),
    );
    targetsPanel.append(targetsHead);
    const targets = efundNode("div", "efund-targets");
    for (const target of preview.targets) targets.append(renderEfundTarget(target));
    targetsPanel.append(targets);
    efundResultRoot.append(targetsPanel);
  }

  const actions = efundNode("section", "panel");
  const actionsHead = efundNode("div", "ph");
  actionsHead.append(efundNode("h2", "", "写入"), efundNode("span", "", "走现有快照确认流程"));
  actions.append(actionsHead);
  if (!preview.can_import) {
    actions.append(efundNode("p", "efund-block", preview.block_reason || "当前不能导入"));
  } else {
    actions.append(efundNode("p", "efund-hint", "正常会直接生效；命中异常规则会进入「待确认」。"));
  }
  const buttons = efundNode("div", "efund-actions");
  const save = efundNode("button", "btn", "保存映射");
  save.type = "button";
  save.addEventListener("click", () => saveEfundMapping(false));
  const run = efundNode("button", "btn pri", "导入并确认");
  run.type = "button";
  run.disabled = preview.blocked_by_data;
  if (preview.block_reason) run.title = preview.block_reason;
  run.addEventListener("click", importEfund);
  buttons.append(save, run);
  actions.append(buttons);
  efundResultRoot.append(actions);

  const results = renderEfundResults();
  if (results) efundResultRoot.append(results);
}

efundFileInput.addEventListener("change", () => {
  efundFile = efundFileInput.files?.[0] || null;
  efundPreview = null;
  efundImportResult = null;
  efundResultRoot.replaceChildren();
  efundFileName.textContent = efundFile ? efundFile.name : "未选择文件";
  efundPasswordWrap.hidden = !(efundFile && efundFile.name.toLowerCase().endsWith(".zip"));
  efundPassword.value = "";
  efundParse.disabled = !efundFile;
  if (efundFile) showStatus("文件已选择，点击「解析」");
});

efundParse.addEventListener("click", parseEfundFile);

/* ---------- navigation ---------- */

async function reloadOverview() {
  renderOverview(await call("OverviewService.Get"));
}

async function showView(view) {
  if (view === "loan" && !loanAccount) view = "overview";
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
  statusBox.hidden = true;
  try {
    if (view === "overview") await reloadOverview();
    else if (view === "pending") await reloadPending();
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
  else previewLoanForm();
});

showView("overview");
