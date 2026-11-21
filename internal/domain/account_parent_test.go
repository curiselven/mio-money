package domain_test

import (
	"context"
	"strings"
	"testing"

	"github.com/curiselven/mio-money/internal/domain"
)

func boolp(value bool) *bool { return &value }

func TestAccountPortfolioRules(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestServiceAndStore(t)
	parent, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例账户", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例组合", ParentID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentID != parent.ID || child.Type != domain.AccountFund || child.Currency != "CNY" {
		t.Fatalf("child = %+v", child)
	}
	if _, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: parent.ID, AsOf: "2026-11-17", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "示例基金", MarketValueCents: 100}},
	}); err == nil {
		t.Fatal("parent with a portfolio accepted a snapshot")
	}
	if _, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: child.ID, AsOf: "2026-11-17", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "示例基金", Code: "000001", Quantity: "1", MarketValueCents: 100}},
	}); err != nil {
		t.Fatalf("portfolio snapshot: %v", err)
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "三级组合", ParentID: child.ID}); err == nil {
		t.Fatal("third level account was accepted")
	}
}

func TestAccountWithSnapshotCannotBecomeParent(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestServiceAndStore(t)
	account, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "已有快照", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: account.ID, AsOf: "2026-11-17", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "示例基金", Code: "000001", Quantity: "1", MarketValueCents: 100}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(ctx, result.SnapshotID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例组合", ParentID: account.ID}); err == nil {
		t.Fatal("account with snapshots became a parent")
	}
}

func TestArchiveOnlyEmptyAccounts(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestServiceAndStore(t)
	empty, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "空账户", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{ID: empty.ID, Archived: boolp(true)}); err != nil {
		t.Fatalf("archive empty: %v", err)
	}
	parent, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "有组合", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "组合", ParentID: parent.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{ID: parent.ID, Archived: boolp(true)}); err == nil {
		t.Fatal("parent with portfolio was archived")
	}
	withSnapshot, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "有快照", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: withSnapshot.ID, AsOf: "2026-11-17", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "示例基金", MarketValueCents: 100}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(ctx, result.SnapshotID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{ID: withSnapshot.ID, Archived: boolp(true)}); err == nil {
		t.Fatal("account with snapshot was archived")
	}
}

func TestPortfolioTypeAndCurrencyFollowParent(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestServiceAndStore(t)
	parent, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{
		Name: "示例账户", Type: domain.AccountStock, Currency: "USD",
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{
		Name: "示例组合", ParentID: parent.ID, Type: domain.AccountFund, Currency: "CNY",
	})
	if err != nil {
		t.Fatal(err)
	}
	if child.Type != domain.AccountStock || child.Currency != "USD" {
		t.Fatalf("create inherited %s/%s, want stock/USD", child.Type, child.Currency)
	}

	updated, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{
		ID: child.ID, Type: domain.AccountCash, Currency: "EUR",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Type != domain.AccountStock || updated.Currency != "USD" {
		t.Fatalf("portfolio update changed type/currency: %s/%s", updated.Type, updated.Currency)
	}

	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{
		ID: parent.ID, Type: domain.AccountMixed, Currency: "EUR",
	}); err != nil {
		t.Fatal(err)
	}
	stored, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{ID: child.ID})
	if err != nil {
		t.Fatal(err)
	}
	if stored.Type != domain.AccountMixed || stored.Currency != "EUR" {
		t.Fatalf("parent change did not cascade: %s/%s", stored.Type, stored.Currency)
	}

	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{
		ID: parent.ID, Type: domain.AccountLiability,
	}); err == nil {
		t.Fatal("parent with portfolios became a liability")
	}
}

func TestParentCurrencyChangeRejectedBySnapshotCurrency(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestServiceAndStore(t)
	parent, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例账户", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例组合", ParentID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: child.ID, AsOf: "2026-11-17", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "示例基金", MarketValueCents: 100}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(ctx, result.SnapshotID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{ID: parent.ID, Currency: "USD"}); err == nil {
		t.Fatal("parent currency changed despite a CNY snapshot on its portfolio")
	}
	storedParent, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{ID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	storedChild, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{ID: child.ID})
	if err != nil {
		t.Fatal(err)
	}
	if storedParent.Currency != "CNY" || storedChild.Currency != "CNY" {
		t.Fatalf("currency left inconsistent: parent=%s child=%s", storedParent.Currency, storedChild.Currency)
	}
}

func TestParentCurrencyChangeRejectedByPendingSnapshot(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestServiceAndStore(t)
	parent, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例账户", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例组合", ParentID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: child.ID, AsOf: "2026-11-17", Source: domain.SourceMCP, Currency: "USD",
		Items: []domain.Item{{Name: "示例基金", MarketValueCents: 100}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{ID: parent.ID, Currency: "EUR"}); err == nil ||
		!strings.Contains(err.Error(), "币种") {
		t.Fatalf("parent currency change with a pending USD snapshot error = %v", err)
	}
}

func TestArchivedParentRejectsPortfolioWrites(t *testing.T) {
	ctx := context.Background()
	svc, st := newTestServiceAndStore(t)
	parent, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例账户", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例组合", ParentID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{ID: child.ID, Archived: boolp(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{ID: parent.ID, Archived: boolp(true)}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{ID: child.ID, Archived: boolp(false)}); err == nil ||
		!strings.Contains(err.Error(), "上级账户已归档") {
		t.Fatalf("restore under archived parent error = %v", err)
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{ID: child.ID, Name: "改名组合"}); err == nil ||
		!strings.Contains(err.Error(), "上级账户已归档") {
		t.Fatalf("rename under archived parent error = %v", err)
	}

	active, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "另一账户", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	pendingChild, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "待确认组合", ParentID: active.ID})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: pendingChild.ID, AsOf: "2026-11-17", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "示例基金", MarketValueCents: 100}},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := st.GetAccount(ctx, active.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored.Archived = true
	if err := st.UpsertAccount(ctx, stored); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: pendingChild.ID, AsOf: "2026-11-18", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "示例基金", MarketValueCents: 100}},
	}); err == nil || !strings.Contains(err.Error(), "上级账户已归档") {
		t.Fatalf("submit under archived parent error = %v", err)
	}
	if _, err := svc.Confirm(ctx, result.SnapshotID); err == nil || !strings.Contains(err.Error(), "上级账户已归档") {
		t.Fatalf("confirm under archived parent error = %v", err)
	}
	if _, err := svc.ApproveAnomalous(ctx, result.SnapshotID); err == nil || !strings.Contains(err.Error(), "上级账户已归档") {
		t.Fatalf("approve under archived parent error = %v", err)
	}
}

func TestAccountNamesAreTrimmed(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestServiceAndStore(t)
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "   ", Type: domain.AccountFund}); err == nil {
		t.Fatal("blank account name was accepted")
	}
	account, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: " 示例账户 ", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	if account.Name != "示例账户" {
		t.Fatalf("created name = %q", account.Name)
	}
	child, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: " 示例组合 ", ParentID: account.ID})
	if err != nil {
		t.Fatal(err)
	}
	if child.Name != "示例组合" {
		t.Fatalf("portfolio name = %q", child.Name)
	}
	renamed, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{ID: child.ID, Name: " 改名组合 "})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != "改名组合" {
		t.Fatalf("renamed = %q", renamed.Name)
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{ID: child.ID, Name: "  "}); err == nil {
		t.Fatal("blank rename was accepted")
	}
	stored, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{ID: child.ID})
	if err != nil {
		t.Fatal(err)
	}
	if stored.Name != "改名组合" {
		t.Fatalf("name after rejected rename = %q", stored.Name)
	}
}

func TestSiblingNamesUniqueOnCreateAndRename(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestServiceAndStore(t)
	first, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例账户", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: " 示例账户 ", Type: domain.AccountFund}); err == nil {
		t.Fatal("duplicate top-level name was created")
	}
	second, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "另一个账户", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{ID: second.ID, Name: "示例账户"}); err == nil {
		t.Fatal("top-level account was renamed to a duplicate")
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例账户", Type: domain.AccountFund, Archived: boolp(true)}); err != nil {
		t.Fatalf("archived duplicate should not block: %v", err)
	}

	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例组合", ParentID: first.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: " 示例组合 ", ParentID: first.ID}); err == nil {
		t.Fatal("duplicate portfolio name was created")
	}
	renameMe, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "待改名组合", ParentID: first.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{ID: renameMe.ID, Name: "示例组合"}); err == nil {
		t.Fatal("portfolio was renamed to a duplicate")
	}
	other, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "第三个账户", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例组合", ParentID: other.ID}); err != nil {
		t.Fatalf("same name under a different parent should be allowed: %v", err)
	}
}

func TestArchivedAccountRejectsSubmitAndConfirm(t *testing.T) {
	ctx := context.Background()
	svc, st := newTestServiceAndStore(t)
	archived, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{
		Name: "归档账户", Type: domain.AccountFund, Archived: boolp(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: archived.ID, AsOf: "2026-11-17", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "示例基金", MarketValueCents: 100}},
	}); err == nil || !strings.Contains(err.Error(), "已归档") {
		t.Fatalf("submit to archived account error = %v", err)
	}

	active, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例账户", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: active.ID, AsOf: "2026-11-17", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "示例基金", MarketValueCents: 100}},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := st.GetAccount(ctx, active.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored.Archived = true
	if err := st.UpsertAccount(ctx, stored); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(ctx, result.SnapshotID); err == nil || !strings.Contains(err.Error(), "已归档") {
		t.Fatalf("confirm archived account error = %v", err)
	}
	if _, err := svc.ApproveAnomalous(ctx, result.SnapshotID); err == nil || !strings.Contains(err.Error(), "已归档") {
		t.Fatalf("approve anomalous archived account error = %v", err)
	}
}
