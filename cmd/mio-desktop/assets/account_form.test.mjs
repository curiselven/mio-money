import assert from "node:assert/strict";
import { accountEditValues, accountFormVisibility, accountOwnerNote, accountPortfolioView } from "./account_form.mjs";

assert.deepEqual(accountFormVisibility(null), { create: false, portfolio: false, edit: false });
for (const name of ["create", "portfolio", "edit"]) {
  const visible = accountFormVisibility(name);
  assert.deepEqual(Object.keys(visible).filter((key) => visible[key]), [name]);
}

for (const type of ["fund", "mixed", "cash", "stock", "other"]) {
  const values = accountEditValues({
    name: "示例账户",
    type,
    currency: "CNY",
    owner: "示例成员A",
    platform: "旧平台",
  });
  assert.equal(values.type, type);
  assert.equal(values.name, "示例账户");
  assert.equal(values.owner, "示例成员A");
  assert.equal("platform" in values, false);
}

assert.throws(() => accountEditValues({ type: "liability" }), /不支持的账户类型/);
assert.throws(() => accountEditValues(null), /账户不存在/);

const portfolio = accountEditValues({ name: "示例组合", type: "fund", parent_id: "acc_parent" });
assert.equal(portfolio.isPortfolio, true);

const accounts = [
  { id: "acc_parent", owner: "示例成员A" },
  { id: "acc_empty", owner: " " },
];
assert.equal(accountOwnerNote({ parent_id: "acc_parent" }, accounts), "归属：跟随账户（示例成员A）");
assert.equal(accountOwnerNote({ parent_id: "acc_empty" }, accounts), "归属：跟随账户（未归属）");
assert.equal(accountOwnerNote({ parent_id: "acc_missing" }, accounts), "归属：跟随账户（未归属）");
assert.equal(accountOwnerNote({ owner: "示例成员A" }, accounts), "");

const portfolios = [
  { id: "p1", parent_id: "acc_a" },
  { id: "p2", parent_id: "acc_a" },
  { id: "p3", parent_id: "acc_b" },
];

assert.deepEqual(accountPortfolioView({ id: "acc_cash", child_count: 0 }, portfolios), {
  items: [],
  count: 0,
  showCount: false,
});
assert.deepEqual(accountPortfolioView({ id: "acc_a", child_count: 2 }, portfolios), {
  items: portfolios.slice(0, 2),
  count: 2,
  showCount: true,
});
assert.deepEqual(accountPortfolioView({ id: "acc_b" }, portfolios), {
  items: [portfolios[2]],
  count: 1,
  showCount: true,
});

console.log("account_form: ok");
