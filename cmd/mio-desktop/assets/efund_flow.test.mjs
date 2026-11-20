import assert from "node:assert/strict";
import { groupState, importSummary, mergeResults, targetState } from "./efund_flow.mjs";

const targets = [
  { key: "id:acc_a", account_id: "acc_a", account_name: "示例账户 A", already_imported: false },
  { key: "id:acc_b", account_id: "acc_b", account_name: "示例账户 B", already_imported: true, imported_status: "efund_import / 待确认" },
];
const groups = [
  { account_id: "acc_a", mode: "existing", market_value_cents: 1000 },
  { account_id: "acc_b", mode: "existing", market_value_cents: 2000 },
  { account_id: "", mode: "ignore", ignored: true, market_value_cents: 3000 },
];

let outcomes = new Map();
assert.equal(groupState(groups[0], targets, outcomes).state, "open");
assert.equal(groupState(groups[1], targets, outcomes).state, "pending");
assert.equal(groupState(groups[2], targets, outcomes).state, "ignored");

mergeResults(outcomes, [
  { account_id: "acc_a", status: "applied", snapshot_id: "snap_a", market_value_cents: 1000 },
]);
assert.equal(groupState(groups[0], targets, outcomes).state, "applied");
let summary = importSummary(groups, targets, outcomes);
assert.deepEqual(summary.counts, { applied: 1, appliedCents: 1000, pending: 1, ignored: 1, skipped: 0, failed: 0 });
assert.equal(summary.done, true);

mergeResults(outcomes, [
  { account_id: "acc_a", status: "skipped", message: "已跳过" },
]);
assert.equal(outcomes.get("acc_a").status, "applied");

outcomes.get("acc_a").status = "undone";
summary = importSummary(groups, targets, outcomes);
assert.equal(groupState(groups[0], targets, outcomes).state, "open");
assert.equal(summary.done, false);

outcomes = new Map();
mergeResults(outcomes, [{ account_id: "acc_a", status: "failed", message: "示例失败" }]);
assert.equal(groupState(groups[0], targets, outcomes).state, "failed");
assert.equal(importSummary(groups, targets, outcomes).done, false);

assert.equal(targetState({ already_imported: true, imported_status: "manual / 已确认" }, null), "skipped");
assert.equal(targetState({ already_imported: true, imported_status: "efund_import / 待确认" }, null), "pending");
assert.equal(targetState({ already_imported: true }, { status: "undone" }), "open");

assert.equal(importSummary([groups[2]], targets, new Map()).done, true);
assert.equal(importSummary([], targets, new Map()).done, false);

console.log("efund_flow: ok");
