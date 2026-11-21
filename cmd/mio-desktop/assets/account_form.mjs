const manageableTypes = new Set(["fund", "mixed", "cash", "stock", "other"]);

export function accountFormVisibility(open) {
  return {
    create: open === "create",
    portfolio: open === "portfolio",
    edit: open === "edit",
  };
}

export function accountEditValues(item) {
  if (!item) throw new Error("账户不存在");
  if (!manageableTypes.has(item.type)) {
    throw new Error(`不支持的账户类型：${item.type || "空"}`);
  }
  return {
    name: item.name || "",
    type: item.type,
    currency: item.currency || "CNY",
    owner: item.owner || "",
    isPortfolio: Boolean(item.parent_id),
  };
}

export function accountOwnerNote(item, accounts = []) {
  if (!item?.parent_id) return "";
  const parent = accounts.find((account) => account.id === item.parent_id);
  const owner = parent?.owner?.trim();
  return `归属：跟随账户（${owner || "未归属"}）`;
}

export function accountPortfolioView(account, portfolios) {
  const items = portfolios.filter((portfolio) => portfolio.parent_id === account.id);
  const count = account.child_count ?? items.length;
  return { items, count, showCount: count > 0 };
}
