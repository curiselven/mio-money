const doneStates = new Set(["applied", "ignored", "pending", "skipped"]);

export function importIsPending(status) {
  return typeof status === "string" && status.includes("待确认");
}

export function portfolioChoices(top) {
  if (!top) return { allowDirect: true, allowNew: true, note: "" };
  return {
    allowDirect: !top.has_children,
    allowNew: !top.has_snapshots,
    note: top.has_snapshots ? "该账户已有快照，不能再新建组合" : "",
  };
}

export function targetForGroup(group, targets) {
  if (!group?.account_id) return null;
  return (targets || []).find((target) => target.account_id === group.account_id) || null;
}

export function targetState(target, outcome) {
  if (outcome?.status === "undone") return "open";
  if (outcome) {
    return outcome.status === "skipped" && importIsPending(target?.imported_status) ? "pending" : outcome.status;
  }
  if (target?.already_imported) return importIsPending(target.imported_status) ? "pending" : "skipped";
  return "open";
}

export function groupState(group, targets, outcomes) {
  if (group?.mode === "ignore") return { state: "ignored", target: null, outcome: null };
  const target = targetForGroup(group, targets);
  const outcome = target?.account_id ? outcomes.get(target.account_id) || null : null;
  return { state: targetState(target, outcome), target, outcome };
}

export function mergeResults(outcomes, results) {
  for (const result of results || []) {
    const key = result.account_id || `name:${result.account_name || ""}`;
    if (key === "name:") continue;
    const previous = outcomes.get(key);
    if (previous && (previous.status === "applied" || previous.status === "pending") && result.status === "skipped") continue;
    outcomes.set(key, { ...result });
  }
  return outcomes;
}

export function priorImportUndo(entry) {
  const source = entry?.outcome?.existing_snapshot_source || entry?.target?.existing_snapshot_source || "";
  const snapshotID = entry?.outcome?.existing_snapshot_id || entry?.target?.existing_snapshot_id || "";
  const pending = importIsPending(entry?.target?.imported_status);
  return { source, snapshotID, pending, canUndo: source === "efund_import" && Boolean(snapshotID) && !pending };
}

export function importSummary(groups, targets, outcomes) {
  const rows = (groups || []).map((group) => ({ group, ...groupState(group, targets, outcomes) }));
  const counts = { applied: 0, appliedCents: 0, pending: 0, ignored: 0, skipped: 0, failed: 0 };
  for (const row of rows) {
    if (row.state === "applied") {
      counts.applied += 1;
      counts.appliedCents += row.group.market_value_cents || 0;
    } else if (row.state === "pending") counts.pending += 1;
    else if (row.state === "ignored") counts.ignored += 1;
    else if (row.state === "skipped") counts.skipped += 1;
    else if (row.state === "failed") counts.failed += 1;
  }
  const done = rows.length > 0 && rows.every((row) => doneStates.has(row.state));
  return { rows, counts, done };
}
