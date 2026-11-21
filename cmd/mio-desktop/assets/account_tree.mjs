export function accountRows(accounts, expanded) {
  const ids = new Set(accounts.map((account) => account.account_id));
  const children = new Map();
  for (const account of accounts) {
    if (!account.parent_id) continue;
    if (!children.has(account.parent_id)) children.set(account.parent_id, []);
    children.get(account.parent_id).push(account);
  }
  const rows = new Map();
  for (const account of accounts) {
    const kids = children.get(account.account_id) || [];
    const parentId = account.parent_id || "";
    const root = !parentId || !ids.has(parentId);
    const hasChildren = Boolean(account.has_children || kids.length);
    const hasDetails = Boolean((account.items || []).length) || account.cash_cents != null || Boolean(account.loan);
    const asOf = [account.as_of, ...kids.map((child) => child.as_of)]
      .filter(Boolean)
      .reduce((latest, date) => (date > latest ? date : latest), "");
    rows.set(account.account_id, {
      parentId,
      root,
      totalCents: account.value_cents + kids.reduce((sum, child) => sum + child.value_cents, 0),
      totalShare: account.share_percent + kids.reduce((sum, child) => sum + child.share_percent, 0),
      totalAsOf: asOf,
      hasChildren,
      expandable: hasChildren || hasDetails,
      visible: root || expanded.has(parentId),
      detailVisible: expanded.has(account.account_id) && (root || expanded.has(parentId)),
    });
  }
  return rows;
}

export function allocationTotals(accounts) {
  const rows = accountRows(accounts, new Set());
  const totals = new Map();
  for (const account of accounts) {
    const info = rows.get(account.account_id);
    if (!info.root || account.type === "liability" || info.totalCents <= 0) continue;
    totals.set(account.type, (totals.get(account.type) || 0) + info.totalCents);
  }
  return totals;
}
