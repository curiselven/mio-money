# Changelog

## Unreleased

### 基金E账户导入

- 新增基金E账户 xlsx 解析、CLI 预览与 `--emit-map` 映射骨架。
- 支持外层 WinZip AES zip、密码环境变量/终端/桌面输入，内存解压并限制 zip/xlsx 资源。
- 映射支持 `account_id`、`ignore`、两级 `parent_account`；CLI 预览区分已映射、已忽略、未映射。
- 桌面端新增「导入 · 基金E账户」页面：分组明细、页内映射、差异/异常预览、按目标/全部导入。
- 新增 `source=efund_import` 与迁移 0006；写入复用 pending/confirm 流程，记录审计来源、份额日期和打印时间。
- 同一目标同一份额日期已有任意来源 confirmed/pending 快照时跳过；支持按 snapshot 精确撤销。
- 目标账户仅允许 fund/mixed；非人民币、份额日期不一致、记录数不符会阻止写入；现金可从上次快照沿用。
- 映射写失败不阻断导入；导入结果按全部成功/部分失败/全部跳过聚合，并单独显示刷新失败。

### 账户与组合

- 迁移 0007 增加 `accounts.parent_id`，支持「账户 → 组合」两级结构。
- 新增账户管理页面：新建/改名账户和组合，归属成员，归档空对象。
- 有快照的账户不能作为父账户；有组合的父账户不能直接提交快照。
- 总览按账户缩进显示组合。

### 安装与文档

- 新增 `scripts/install-mac.sh`、`packaging/macos/Info.plist`：构建 Mio.app/CLI、备份数据库、归档旧应用、ad-hoc 签名。
- 更新 README 安装说明、基金E账户导入文档和迁移回退说明。
