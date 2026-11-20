export function accountRows(accounts, expanded) {
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
    const hasChildren = Boolean(account.has_children || kids.length);
    const hasDetails = Boolean((account.items || []).length) || account.cash_cents != null || Boolean(account.loan);
    rows.set(account.account_id, {
      parentId,
      totalCents: account.value_cents + kids.reduce((sum, child) => sum + child.value_cents, 0),
      totalShare: account.share_percent + kids.reduce((sum, child) => sum + child.share_percent, 0),
      hasChildren,
      expandable: hasChildren || hasDetails,
      visible: !parentId || expanded.has(parentId),
      detailVisible: expanded.has(account.account_id) && (!parentId || expanded.has(parentId)),
    });
  }
  return rows;
}
