package domain_test

import (
	"context"
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
