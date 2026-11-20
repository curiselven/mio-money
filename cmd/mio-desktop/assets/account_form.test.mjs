import assert from "node:assert/strict";
import { accountEditValues } from "./account_form.mjs";

for (const type of ["fund", "mixed", "cash", "stock", "other"]) {
  const values = accountEditValues({
    name: "示例账户",
    type,
    currency: "CNY",
    owner: "我",
    platform: "旧平台",
  });
  assert.equal(values.type, type);
  assert.equal(values.name, "示例账户");
  assert.equal(values.owner, "我");
  assert.equal("platform" in values, false);
}

assert.throws(() => accountEditValues({ type: "liability" }), /不支持的账户类型/);
assert.throws(() => accountEditValues(null), /账户不存在/);

const portfolio = accountEditValues({ name: "示例组合", type: "fund", parent_id: "acc_parent" });
assert.equal(portfolio.isPortfolio, true);

console.log("account_form: ok");
