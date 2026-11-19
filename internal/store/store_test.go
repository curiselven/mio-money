package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/curiselven/mio-money/internal/domain"
	"github.com/curiselven/mio-money/internal/store"
)

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("file:store_roundtrip?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	now := time.Now().UTC()
	account := &domain.Account{
		ID: "acc_test", Name: "示例券商", Platform: "broker", Type: domain.AccountStock,
		Currency: "CNY", Owner: "我", CreatedAt: now, UpdatedAt: now,
	}
	if err := st.UpsertAccount(ctx, account); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	gotAccount, err := st.GetAccount(ctx, "acc_test")
	if err != nil || gotAccount.Name != "示例券商" || gotAccount.Type != domain.AccountStock {
		t.Fatalf("account = %+v, err=%v", gotAccount, err)
	}

	cost := int64(90_000)
	cash := int64(12_345)
	snapshot := &domain.Snapshot{
		ID: "snp_test", AccountID: "acc_test", AsOf: "2026-11-01", Status: domain.StatusPending,
		Source: domain.SourceScreenshot, SourceNote: "示例截图", Currency: "CNY", CashCents: &cash,
		Items: []domain.Item{
			{Name: "示例ETF", Code: "510300", Quantity: "100.5", MarketValueCents: 100_000, CostCents: &cost, Note: "测试"},
			{Name: "示例现金", MarketValueCents: 1},
		},
		CreatedAt: now,
	}
	if err := st.InsertSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}

	got, err := st.GetSnapshot(ctx, "snp_test")
	if err != nil {
		t.Fatalf("get snapshot: %v", err)
	}
	if got.CashCents == nil || *got.CashCents != cash || len(got.Items) != 2 {
		t.Fatalf("snapshot = %+v", got)
	}
	if got.Items[0].Quantity != "100.5" || got.Items[0].CostCents == nil || *got.Items[0].CostCents != cost {
		t.Fatalf("item = %+v", got.Items[0])
	}

	confirmedAt := now.Add(time.Minute)
	if err := st.UpdateSnapshotStatus(ctx, "snp_test", domain.StatusConfirmed, &confirmedAt); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	latest, err := st.LastConfirmedBefore(ctx, "acc_test", "2026-11-01")
	if err != nil || latest == nil || latest.ID != "snp_test" {
		t.Fatalf("last confirmed = %+v, err=%v", latest, err)
	}
	dates, err := st.LastConfirmedDates(ctx)
	if err != nil || dates["acc_test"] != "2026-11-01" {
		t.Fatalf("dates = %v, err=%v", dates, err)
	}

	if err := st.AddAudit(ctx, domain.AuditEntry{
		Action: "confirm", AccountID: "acc_test", SnapshotID: "snp_test", CreatedAt: now,
	}); err != nil {
		t.Fatalf("audit: %v", err)
	}

	if version, err := st.SchemaVersion(ctx); err != nil || version != 4 {
		t.Fatalf("schema version = %d, err=%v", version, err)
	}
}
