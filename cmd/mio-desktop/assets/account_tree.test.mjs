import assert from "node:assert/strict";
import { accountRows } from "./account_tree.mjs";

const accounts = [
  {
    account_id: "acc_top",
    name: "示例账户",
    value_cents: 100000,
    share_percent: 10,
    items: [],
  },
  {
    account_id: "acc_child",
    parent_id: "acc_top",
    name: "示例组合",
    value_cents: 250000,
    share_percent: 25,
    items: [{ name: "示例基金" }],
  },
];

let rows = accountRows(accounts, new Set());
assert.equal(rows.get("acc_top").totalCents, 350000);
assert.equal(rows.get("acc_top").totalShare, 35);
assert.equal(rows.get("acc_top").visible, true);
assert.equal(rows.get("acc_top").detailVisible, false);
assert.equal(rows.get("acc_child").visible, false);
assert.equal(rows.get("acc_child").detailVisible, false);

rows = accountRows(accounts, new Set(["acc_top"]));
assert.equal(rows.get("acc_child").visible, true);
assert.equal(rows.get("acc_child").detailVisible, false);
assert.equal(rows.get("acc_child").expandable, true);

rows = accountRows(accounts, new Set(["acc_top", "acc_child"]));
assert.equal(rows.get("acc_child").detailVisible, true);

rows = accountRows(accounts, new Set(["acc_child"]));
assert.equal(rows.get("acc_child").visible, false);
assert.equal(rows.get("acc_child").detailVisible, false);

const cashOnly = accountRows([{ account_id: "acc_cash", value_cents: 0, share_percent: 0, cash_cents: 500 }], new Set());
assert.equal(cashOnly.get("acc_cash").expandable, true);
assert.equal(cashOnly.get("acc_cash").hasChildren, false);

console.log("account_tree: ok");
