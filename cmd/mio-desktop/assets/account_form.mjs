const manageableTypes = new Set(["fund", "mixed", "cash", "stock", "other"]);

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
