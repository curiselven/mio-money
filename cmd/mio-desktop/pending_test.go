package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/curiselven/mio-money/internal/domain"
	"github.com/curiselven/mio-money/internal/store"
)

func TestPendingService(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "mio.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	svc := domain.NewService(st, 0.2)
	pending := NewPendingService(svc)
	account, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "测试账户", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: account.ID, AsOf: "2026-11-01", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "测试基金", Code: "000001", Quantity: "10", MarketValueCents: 100_000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(ctx, first.SnapshotID); err != nil {
		t.Fatal(err)
	}
	anomalous, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: account.ID, AsOf: "2026-11-02", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "测试基金", Code: "000001", Quantity: "10", MarketValueCents: 150_000}},
	})
	if err != nil {
		t.Fatal(err)
	}

	reviews, err := pending.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 1 || reviews[0].SnapshotID != anomalous.SnapshotID || reviews[0].AccountName != "测试账户" {
		t.Fatalf("reviews = %+v", reviews)
	}
	if err := pending.Approve(anomalous.SnapshotID); err != nil {
		t.Fatal(err)
	}
	if reviews, err := pending.List(); err != nil || len(reviews) != 0 {
		t.Fatalf("reviews after approve = %+v, err=%v", reviews, err)
	}

	second, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: account.ID, AsOf: "2026-11-03", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "测试基金", Code: "000001", Quantity: "10", MarketValueCents: 300_000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := pending.Reject(second.SnapshotID, "测试原因"); err != nil {
		t.Fatal(err)
	}
	if reviews, err := pending.List(); err != nil || len(reviews) != 0 {
		t.Fatalf("reviews after reject = %+v, err=%v", reviews, err)
	}
	audits, err := st.ListAudit(ctx, second.SnapshotID)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) == 0 || audits[len(audits)-1].Action != "reject" ||
		!strings.Contains(audits[len(audits)-1].Detail, "测试原因") {
		t.Fatalf("audits = %+v", audits)
	}
}
