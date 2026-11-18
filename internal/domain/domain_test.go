package domain_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/curiselven/mio-money/internal/domain"
	"github.com/curiselven/mio-money/internal/store"
)

func newTestService(t *testing.T) *domain.Service {
	t.Helper()
	name := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	st, err := store.Open(fmt.Sprintf("file:domain_%s?mode=memory&cache=shared", name))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return domain.NewService(st, 0.2)
}

func int64p(v int64) *int64 { return &v }

func TestComputeDiff(t *testing.T) {
	prev := &domain.Snapshot{
		CashCents: int64p(10_000),
		Items: []domain.Item{
			{Name: "沪深300", Code: "000300", Quantity: "100", MarketValueCents: 100_000},
			{Name: "货币基金", Quantity: "10", MarketValueCents: 20_000},
			{Name: "旧持仓", Code: "OLD", Quantity: "5", MarketValueCents: 30_000},
		},
	}
	next := &domain.Snapshot{
		CashCents: int64p(15_000),
		Items: []domain.Item{
			{Name: "沪深300", Code: "000300", Quantity: "100", MarketValueCents: 120_000},
			{Name: "货币基金", Quantity: "12.5", MarketValueCents: 25_000},
			{Name: "新持仓", Code: "NEW", Quantity: "1", MarketValueCents: 5_000},
		},
	}
	d := domain.ComputeDiff(prev, next)

	if len(d.ValueChanged) != 1 || d.ValueChanged[0].Code != "000300" || d.ValueChanged[0].DeltaValueCents != 20_000 {
		t.Fatalf("value changed = %+v", d.ValueChanged)
	}
	if len(d.QuantityChanged) != 1 || d.QuantityChanged[0].Name != "货币基金" {
		t.Fatalf("quantity changed = %+v", d.QuantityChanged)
	}
	if len(d.Added) != 1 || d.Added[0].Code != "NEW" {
		t.Fatalf("added = %+v", d.Added)
	}
	if len(d.Removed) != 1 || d.Removed[0].Name != "旧持仓" {
		t.Fatalf("removed = %+v", d.Removed)
	}
	if d.TotalBeforeCents != 160_000 || d.TotalAfterCents != 165_000 || d.TotalDeltaCents != 5_000 {
		t.Fatalf("totals = %+v", d)
	}
}

func TestDiffMatchesNameWhenCodeMissing(t *testing.T) {
	prev := &domain.Snapshot{Items: []domain.Item{{Name: "黄金ETF", Code: "518880", Quantity: "10", MarketValueCents: 10_000}}}
	next := &domain.Snapshot{Items: []domain.Item{{Name: "黄金ETF", Quantity: "10", MarketValueCents: 11_000}}}
	d := domain.ComputeDiff(prev, next)
	if len(d.Added) != 0 || len(d.Removed) != 0 || len(d.ValueChanged) != 1 {
		t.Fatalf("diff = %+v", d)
	}
}

func TestAnomalyRules(t *testing.T) {
	account := &domain.Account{Currency: "CNY"}
	base := &domain.Snapshot{
		Currency:  "CNY",
		CashCents: int64p(1_000),
		Items:     []domain.Item{{Name: "A", Code: "A", Quantity: "1", MarketValueCents: 100_000}},
	}

	cases := []struct {
		name string
		next *domain.Snapshot
		want string
	}{
		{
			name: "total change over threshold",
			next: &domain.Snapshot{CashCents: int64p(1_000), Currency: "CNY",
				Items: []domain.Item{{Name: "A", Code: "A", Quantity: "1", MarketValueCents: 130_000}}},
			want: "总市值",
		},
		{
			name: "position disappears",
			next: &domain.Snapshot{CashCents: int64p(1_000), Currency: "CNY", Items: []domain.Item{}},
			want: "整条消失",
		},
		{
			name: "negative market value",
			next: &domain.Snapshot{Currency: "CNY", Items: []domain.Item{
				{Name: "A", Code: "A", Quantity: "1", MarketValueCents: 100_000},
				{Name: "B", Code: "B", Quantity: "1", MarketValueCents: -1},
			}},
			want: "市值为负数",
		},
		{
			name: "negative quantity",
			next: &domain.Snapshot{Currency: "CNY", Items: []domain.Item{
				{Name: "A", Code: "A", Quantity: "-1", MarketValueCents: 100_000},
			}},
			want: "份额为负数",
		},
		{
			name: "currency mismatch",
			next: &domain.Snapshot{Currency: "USD", Items: []domain.Item{
				{Name: "A", Code: "A", Quantity: "1", MarketValueCents: 100_000},
			}},
			want: "币种",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := domain.Anomalies(base, tc.next, account, 0.2)
			joined := strings.Join(got, "|")
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("anomalies = %v, want %q", got, tc.want)
			}
		})
	}

	t.Run("within threshold has no anomaly", func(t *testing.T) {
		next := &domain.Snapshot{Currency: "CNY", CashCents: int64p(1_000),
			Items: []domain.Item{{Name: "A", Code: "A", Quantity: "1", MarketValueCents: 120_000}}}
		if got := domain.Anomalies(base, next, account, 0.2); len(got) != 0 {
			t.Fatalf("anomalies = %v", got)
		}
	})

	t.Run("first snapshot has no anomaly", func(t *testing.T) {
		if got := domain.Anomalies(nil, base, account, 0.2); len(got) != 0 {
			t.Fatalf("anomalies = %v", got)
		}
	})

	t.Run("explicit zero quantity is a clearance, not an anomaly", func(t *testing.T) {
		next := &domain.Snapshot{Currency: "CNY", CashCents: int64p(101_000),
			Items: []domain.Item{{Name: "A", Code: "A", Quantity: "0", MarketValueCents: 0}}}
		if got := domain.Anomalies(base, next, account, 0.2); len(got) != 0 {
			t.Fatalf("anomalies = %v", got)
		}
	})

	t.Run("vanished zero quantity item is not an anomaly", func(t *testing.T) {
		prev := &domain.Snapshot{Currency: "CNY", CashCents: int64p(1_000),
			Items: []domain.Item{{Name: "A", Code: "A", Quantity: "0", MarketValueCents: 0}}}
		next := &domain.Snapshot{Currency: "CNY", CashCents: int64p(1_000)}
		if got := domain.Anomalies(prev, next, account, 0.2); len(got) != 0 {
			t.Fatalf("anomalies = %v", got)
		}
	})
}

func TestSubmitConfirmUndo(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	account, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例账户", Type: domain.AccountFund})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	first, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: account.ID,
		AsOf:      "2026-11-01",
		Source:    domain.SourceMCP,
		CashCents: int64p(100_000),
		Items:     []domain.Item{{Name: "示例基金", Code: "000001", Quantity: "100", MarketValueCents: 200_000}},
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if first.Status != domain.StatusPending || first.Diff.TotalAfterCents != 300_000 {
		t.Fatalf("submit result = %+v", first)
	}

	holdings, err := svc.Holdings(ctx, account.ID)
	if err != nil {
		t.Fatalf("holdings: %v", err)
	}
	if len(holdings) != 0 {
		t.Fatalf("pending snapshot must not be visible, got %+v", holdings)
	}

	if _, err := svc.Confirm(ctx, first.SnapshotID); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	holdings, err = svc.Holdings(ctx, account.ID)
	if err != nil || len(holdings) != 1 || holdings[0].TotalCents != 300_000 {
		t.Fatalf("holdings after confirm = %+v, err=%v", holdings, err)
	}

	second, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: account.ID,
		AsOf:      "2026-11-08",
		Source:    domain.SourceMCP,
		CashCents: int64p(100_000),
		Items:     []domain.Item{{Name: "示例基金", Code: "000001", Quantity: "110", MarketValueCents: 220_000}},
	})
	if err != nil {
		t.Fatalf("submit second: %v", err)
	}
	if _, err := svc.Confirm(ctx, second.SnapshotID); err != nil {
		t.Fatalf("confirm second: %v", err)
	}

	nw, err := svc.NetWorth(ctx, "2026-11-08")
	if err != nil {
		t.Fatalf("net worth: %v", err)
	}
	if nw.NetCents != 320_000 {
		t.Fatalf("net worth = %+v", nw)
	}

	undone, err := svc.Undo(ctx, account.ID)
	if err != nil {
		t.Fatalf("undo: %v", err)
	}
	if undone.ID != second.SnapshotID || undone.Status != domain.StatusUndone {
		t.Fatalf("undone = %+v", undone)
	}
	holdings, err = svc.Holdings(ctx, account.ID)
	if err != nil || len(holdings) != 1 || holdings[0].TotalCents != 300_000 || holdings[0].AsOf != "2026-11-01" {
		t.Fatalf("holdings after undo = %+v, err=%v", holdings, err)
	}
	if _, err := svc.Undo(ctx, account.ID); err != nil {
		t.Fatalf("second undo: %v", err)
	}
	if _, err := svc.Undo(ctx, account.ID); err == nil {
		t.Fatalf("undo with nothing left should fail")
	}
}

func TestConfirmRefusesAnomaly(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	account, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例账户", Type: domain.AccountStock})
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: account.ID, AsOf: "2026-11-01", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "示例股票", Code: "600000", MarketValueCents: 100_000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(ctx, first.SnapshotID); err != nil {
		t.Fatal(err)
	}
	second, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: account.ID, AsOf: "2026-11-02", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "示例股票", Code: "600000", MarketValueCents: 150_000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(ctx, second.SnapshotID); !errors.Is(err, domain.ErrAnomaly) {
		t.Fatalf("confirm error = %v, want ErrAnomaly", err)
	}
	pending, err := svc.Pending(ctx)
	if err != nil || len(pending) != 1 || pending[0].ID != second.SnapshotID {
		t.Fatalf("pending = %+v, err=%v", pending, err)
	}
	if _, err := svc.Reject(ctx, second.SnapshotID, "测试丢弃"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if pending, _ := svc.Pending(ctx); len(pending) != 0 {
		t.Fatalf("pending after reject = %+v", pending)
	}
}

func TestZeroQuantityClearsPosition(t *testing.T) {
	ctx := context.Background()
	svc := newTestService(t)
	account, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{Name: "示例账户", Type: domain.AccountFund})
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: account.ID, AsOf: "2026-11-01", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "示例基金", Code: "000001", Quantity: "100", MarketValueCents: 100_000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(ctx, first.SnapshotID); err != nil {
		t.Fatal(err)
	}

	// Sold everything: shares explicitly go to 0, proceeds land in cash.
	second, err := svc.Submit(ctx, domain.SubmitInput{
		AccountID: account.ID, AsOf: "2026-11-02", Source: domain.SourceMCP,
		CashCents: int64p(100_000),
		Items:     []domain.Item{{Name: "示例基金", Code: "000001", Quantity: "0", MarketValueCents: 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Anomalies) != 0 {
		t.Fatalf("anomalies = %v", second.Anomalies)
	}
	if len(second.Diff.Cleared) != 1 || second.Diff.Cleared[0].OldQuantity != "100" || second.Diff.Cleared[0].NewQuantity != "0" {
		t.Fatalf("cleared = %+v", second.Diff.Cleared)
	}
	if _, err := svc.Confirm(ctx, second.SnapshotID); err != nil {
		t.Fatalf("confirm cleared snapshot: %v", err)
	}
	holdings, err := svc.Holdings(ctx, account.ID)
	if err != nil || len(holdings) != 1 || holdings[0].TotalCents != 100_000 {
		t.Fatalf("holdings = %+v, err=%v", holdings, err)
	}
	if len(holdings[0].Items) != 1 || holdings[0].Items[0].Quantity != "0" {
		t.Fatalf("cleared item not kept in snapshot: %+v", holdings[0].Items)
	}
}
