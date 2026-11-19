if (window.location.protocol === "wails:" || window.location.hostname === "wails.localhost") {
  await import("/wails/runtime.js");
}

const list = document.getElementById("list");
const empty = document.getElementById("empty");
const status = document.getElementById("status");
const refresh = document.getElementById("refresh");
const cardTemplate = document.getElementById("card");
const sourceLabels = { mcp: "Agent 提交", screenshot: "截图", statement: "对账单", manual: "手动" };
let busy = false;

async function call(method, ...args) {
  if (!globalThis.wails?.Call?.ByName) {
    throw new Error("Wails 运行时未就绪");
  }
  return globalThis.wails.Call.ByName(`main.PendingService.${method}`, ...args);
}

function money(cents) {
  const sign = cents < 0 ? "-" : "";
  return `${sign}¥${(Math.abs(cents) / 100).toFixed(2)}`;
}

function signedMoney(cents) {
  if (cents === 0) return "¥0.00";
  return `${cents > 0 ? "+" : "-"}¥${(Math.abs(cents) / 100).toFixed(2)}`;
}

function errorText(error) {
  return error?.message || error?.cause?.message || String(error);
}

function showStatus(text, kind = "ok") {
  status.textContent = text;
  status.className = `status ${kind}`;
  status.hidden = false;
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

  const handler = () => {
    const reason = panel.querySelector(".reason").value.trim();
    const action = isReject
      ? async () => {
          await call("Reject", review.snapshot_id, reason);
          showStatus("已丢弃");
        }
      : async () => {
          await call("Approve", review.snapshot_id);
          showStatus("已确认生效");
        };
    run(async () => {
      await action();
      await reload();
    });
  };
  submit.onclick = handler;

  if (isReject) {
    panel.querySelector(".reason").focus();
  } else {
    submit.focus();
  }
}

function render(reviews) {
  list.replaceChildren();
  empty.hidden = reviews.length !== 0;
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
    const approveButton = card.querySelector(".approve");
    const rejectButton = card.querySelector(".reject");
    approveButton.addEventListener("click", () => openPanel(card, review, "approve"));
    rejectButton.addEventListener("click", () => openPanel(card, review, "reject"));
    card.querySelector(".confirm-cancel").addEventListener("click", () => closePanels());
    list.append(card);
  }
}

async function reload() {
  render(await call("List"));
}

async function load() {
  if (busy) return;
  busy = true;
  setButtonsDisabled(true);
  try {
    await reload();
  } catch (error) {
    showStatus(errorText(error), "error");
  } finally {
    busy = false;
    setButtonsDisabled(false);
  }
}

refresh.addEventListener("click", load);
load();
