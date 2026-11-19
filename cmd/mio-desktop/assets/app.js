if (window.location.protocol === "wails:" || window.location.hostname === "wails.localhost") {
  await import("/wails/runtime.js");
}

const title = document.getElementById("title");
const subtitle = document.getElementById("subtitle");
const status = document.getElementById("status");
const refresh = document.getElementById("refresh");
const overviewView = document.getElementById("overview-view");
const overviewBody = document.getElementById("overview-body");
const overviewEmpty = document.getElementById("overview-empty");
const pendingView = document.getElementById("pending-view");
const list = document.getElementById("list");
const empty = document.getElementById("empty");
const cardTemplate = document.getElementById("card");
const accountsBox = document.getElementById("accounts");
const chartBox = document.getElementById("chart");
const netWorth = document.getElementById("net-worth");
const netAsOf = document.getElementById("net-asof");
const netChange = document.getElementById("net-change");
const assets = document.getElementById("assets");
const liabilities = document.getElementById("liabilities");
const pendingCount = document.getElementById("pending-count");
const tabs = [...document.querySelectorAll(".tab")];

const sourceLabels = { mcp: "Agent 提交", screenshot: "截图", statement: "对账单", manual: "手动" };
const typeLabels = { cash: "现金", fund: "基金", stock: "股票", mixed: "混合", liability: "负债", other: "其他" };
const views = {
  overview: {
    title: "总览",
    subtitle: "只统计已确认快照，金额币种为 CNY。",
    box: overviewView,
  },
  pending: {
    title: "待确认快照",
    subtitle: "被异常规则拦下的记录，确认生效后才会写入正式数据。",
    box: pendingView,
  },
};
const moneyFormat = new Intl.NumberFormat("zh-CN", {
  style: "currency",
  currency: "CNY",
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
});

let currentView = "overview";
let busy = false;

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

function errorText(error) {
  return error?.message || error?.cause?.message || String(error);
}

function showStatus(text, kind = "ok") {
  status.textContent = text;
  status.className = `status ${kind}`;
  status.hidden = false;
}

function setPendingCount(count) {
  pendingCount.textContent = String(count);
  pendingCount.hidden = count === 0;
}

function addLine(element, label, text) {
  const item = document.createElement("li");
  const tag = document.createElement("span");
  tag.className = "tag";
  tag.textContent = label;
  const body = document.createElement("span");
  body.textContent = text;
  item.append(tag, body);
  element.append(item);
}

function renderDiff(element, diff) {
  for (const item of diff.added) {
    addLine(element, "新增", `${item.name} ${item.quantity || "—"} 份 · ${money(item.market_value_cents)}`);
  }
  for (const item of diff.removed) {
    addLine(element, "消失", `${item.name}（原 ${money(item.market_value_cents)}）`);
  }
  for (const item of diff.quantity_changed) {
    addLine(element, "份额", `${item.name} ${item.old_quantity || "—"} → ${item.new_quantity || "—"}`);
  }
  for (const item of diff.value_changed) {
    addLine(element, "市值", `${item.name} ${money(item.old_value_cents)} → ${money(item.new_value_cents)}（${signedMoney(item.delta_value_cents)}）`);
  }
  for (const item of diff.cleared) {
    addLine(element, "清仓", `${item.name} ${item.old_quantity || "—"} → ${item.new_quantity || "0"}`);
  }
  addLine(
    element,
    "总额",
    `${money(diff.total_before_cents)} → ${money(diff.total_after_cents)}（${signedMoney(diff.total_delta_cents)}）`,
  );
}

function setButtonsDisabled(disabled) {
  for (const button of document.querySelectorAll("button")) {
    button.disabled = disabled;
  }
}

function closePanels() {
  for (const card of list.children) {
    card.querySelector(".actions").hidden = false;
    card.querySelector(".confirm-panel").hidden = true;
    card.querySelector(".reason").value = "";
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
    const action = isReject
      ? async () => {
          await call("PendingService.Reject", review.snapshot_id, reason);
          showStatus("已丢弃");
        }
      : async () => {
          await call("PendingService.Approve", review.snapshot_id);
          showStatus("已确认生效");
        };
    run(async () => {
      await action();
      await reloadPending();
    });
  };

  if (isReject) {
    panel.querySelector(".reason").focus();
  } else {
    submit.focus();
  }
}

function renderPending(reviews) {
  list.replaceChildren();
  empty.hidden = reviews.length !== 0;
  setPendingCount(reviews.length);
  for (const review of reviews) {
    const card = cardTemplate.content.firstElementChild.cloneNode(true);
    card.querySelector(".account").textContent = review.account_name;
    card.querySelector(".meta").textContent =
      `${review.as_of} · 来源：${sourceLabels[review.source] || review.source}`;
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

function holdingRow(name, code, quantity, value) {
  const row = document.createElement("tr");
  for (const text of [name, code || "—", quantity || "—", money(value)]) {
    const cell = document.createElement("td");
    cell.textContent = text;
    row.append(cell);
  }
  return row;
}

function renderAccounts(accounts) {
  accountsBox.replaceChildren();
  if (accounts.length === 0) {
    const note = document.createElement("p");
    note.className = "muted";
    note.textContent = "还没有账户";
    accountsBox.append(note);
    return;
  }
  for (const account of accounts) {
    const details = document.createElement("details");
    details.className = "account";
    const summary = document.createElement("summary");

    const name = document.createElement("div");
    name.className = "account-name";
    const strong = document.createElement("strong");
    strong.textContent = account.name;
    const type = document.createElement("span");
    type.className = "type";
    type.textContent = typeLabels[account.type] || account.type;
    name.append(strong, type);

    const value = document.createElement("div");
    value.className = "account-value";
    value.textContent = money(account.value_cents);

    const share = document.createElement("div");
    share.className = "account-share";
    share.textContent = `占净资产 ${account.share_percent.toFixed(1)}%`;

    const date = document.createElement("div");
    date.className = "account-date";
    date.textContent = account.as_of ? `最新快照 ${account.as_of}` : "尚无已确认快照";

    summary.append(name, value, share, date);
    details.append(summary);

    const hasItems = account.items.length > 0 || account.cash_cents != null;
    if (!hasItems) {
      const note = document.createElement("p");
      note.className = "muted holdings-empty";
      note.textContent = "还没有持仓明细";
      details.append(note);
    } else {
      const table = document.createElement("table");
      table.className = "holdings";
      const head = document.createElement("thead");
      const headRow = document.createElement("tr");
      for (const label of ["名称", "代码", "数量", "市值"]) {
        const th = document.createElement("th");
        th.textContent = label;
        headRow.append(th);
      }
      head.append(headRow);
      const body = document.createElement("tbody");
      for (const item of account.items) {
        body.append(holdingRow(item.name, item.code, item.quantity, item.market_value_cents));
      }
      if (account.cash_cents != null) {
        body.append(holdingRow("现金", "", "", account.cash_cents));
      }
      table.append(head, body);
      details.append(table);
    }
    accountsBox.append(details);
  }
}

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
  if (points.length === 0) {
    const note = document.createElement("p");
    note.className = "muted";
    note.textContent = "还没有已确认的快照";
    chartBox.append(note);
    return;
  }

  const width = 780;
  const height = 220;
  const pad = { top: 18, right: 18, bottom: 30, left: 112 };
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

  const svg = svgEl("svg", {
    viewBox: `0 0 ${width} ${height}`,
    class: "chart-svg",
    role: "img",
    "aria-label": "净资产历史曲线",
  });

  for (let i = 0; i < 3; i += 1) {
    const value = max - ((max - min) * i) / 2;
    svg.append(svgEl("line", {
      x1: pad.left,
      x2: width - pad.right,
      y1: y(value),
      y2: y(value),
      class: "grid-line",
    }));
    const label = svgEl("text", {
      x: pad.left - 10,
      y: y(value) + 4,
      class: "axis-label",
      "text-anchor": "end",
    });
    label.textContent = money(Math.round(value));
    svg.append(label);
  }

  const coords = points.map((point, index) => [x(index), y(point.value_cents)]);
  if (points.length > 1) {
    const baseline = pad.top + innerHeight;
    const area = [
      `M ${coords[0][0]},${baseline}`,
      ...coords.map(([cx, cy]) => `L ${cx},${cy}`),
      `L ${coords[coords.length - 1][0]},${baseline}`,
      "Z",
    ].join(" ");
    svg.append(svgEl("path", { d: area, class: "chart-area" }));
    svg.append(svgEl("polyline", {
      points: coords.map(([cx, cy]) => `${cx},${cy}`).join(" "),
      class: "chart-line",
    }));
  }

  points.forEach((point, index) => {
    const [cx, cy] = coords[index];
    const dot = svgEl("circle", { cx, cy, r: 3.5, class: "chart-dot" });
    const tip = svgEl("title", {});
    tip.textContent = `${point.as_of} · ${money(point.value_cents)}`;
    dot.append(tip);
    svg.append(dot);
  });

  const labelIndexes = points.length === 1 ? [0] : [0, points.length - 1];
  for (const index of labelIndexes) {
    const label = svgEl("text", {
      x: x(index),
      y: height - 8,
      class: "axis-label",
      "text-anchor": points.length === 1 ? "middle" : index === 0 ? "start" : "end",
    });
    label.textContent = points[index].as_of;
    svg.append(label);
  }

  chartBox.append(svg);
}

function renderOverview(overview) {
  setPendingCount(overview.pending_count);
  overviewEmpty.hidden = !overview.empty;
  overviewBody.hidden = overview.empty;
  if (overview.empty) return;

  netWorth.textContent = money(overview.net_cents);
  netAsOf.textContent = overview.as_of ? `截至 ${overview.as_of}` : "";
  assets.textContent = money(overview.assets_cents);
  liabilities.textContent = money(overview.liabilities_cents);

  if (overview.change_cents == null) {
    netChange.hidden = true;
  } else {
    const percent = overview.change_percent == null ? "" : `（${signedPercent(overview.change_percent)}）`;
    netChange.textContent = `${signedMoney(overview.change_cents)}${percent} · 较上一有数据日期`;
    netChange.className = `change ${overview.change_cents >= 0 ? "up" : "down"}`;
    netChange.hidden = false;
  }

  renderAccounts(overview.accounts);
  renderChart(overview.history);
}

async function reloadOverview() {
  renderOverview(await call("OverviewService.Get"));
}

async function loadCurrent() {
  if (busy) return;
  busy = true;
  setButtonsDisabled(true);
  status.hidden = true;
  try {
    if (currentView === "overview") {
      await reloadOverview();
    } else {
      await reloadPending();
    }
  } catch (error) {
    showStatus(errorText(error), "error");
  } finally {
    busy = false;
    setButtonsDisabled(false);
  }
}

function showView(view) {
  if (!views[view] || view === currentView) return;
  currentView = view;
  for (const tab of tabs) {
    const active = tab.dataset.view === view;
    tab.classList.toggle("active", active);
    tab.setAttribute("aria-selected", String(active));
  }
  for (const [name, config] of Object.entries(views)) {
    config.box.hidden = name !== view;
  }
  title.textContent = views[view].title;
  subtitle.textContent = views[view].subtitle;
  status.hidden = true;
  loadCurrent();
}

async function run(action) {
  if (busy) return;
  busy = true;
  setButtonsDisabled(true);
  status.hidden = true;
  try {
    await action();
  } catch (error) {
    showStatus(errorText(error), "error");
  } finally {
    busy = false;
    setButtonsDisabled(false);
  }
}

for (const tab of tabs) {
  tab.addEventListener("click", () => showView(tab.dataset.view));
}
refresh.addEventListener("click", loadCurrent);
loadCurrent();
