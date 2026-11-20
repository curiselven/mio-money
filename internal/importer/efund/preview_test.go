package efund

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/curiselven/mio-money/internal/domain"
	"github.com/curiselven/mio-money/internal/store"
)

type fakeRepo struct {
	accounts []domain.Account
	prev     map[string]*domain.Snapshot
}

func (f fakeRepo) ListAccounts(context.Context) ([]domain.Account, error) {
	return f.accounts, nil
}

func (f fakeRepo) LastConfirmedBefore(_ context.Context, accountID, _ string) (*domain.Snapshot, error) {
	return f.prev[accountID], nil
}

func mappingBook(t *testing.T) string {
	t.Helper()
	rows := append(personalRows(), headerRow())
	rows = append(rows,
		holdingRow(1, "000216", "示例成长混合", salesA, "T0000001", "1000", "2026/11/17", "1.0", "2026/11/17", "1000.00"),
		holdingRow(2, "000217", "示例价值混合", salesA, "T0000001", "100", "2026/11/17", "2.0", "2026/11/17", "200.00"),
		holdingRow(3, "000218", "示例新基金", salesA, "T0000001", "10", "2026/11/17", "1.0", "2026/11/17", "10.00"),
		holdingRow(4, "000300", "示例二号组合", salesA, "T0000002", "5", "2026/11/17", "2.0", "2026/11/17", "10.00"),
		holdingRow(5, "000400", "示例未映射基金", salesB, "T0000009", "1", "2026/11/17", "3.0", "2026/11/17", "3.00"),
		footerRow(5),
	)
	return writeBook(t, rows)
}

func TestPreviewMappingAndDiff(t *testing.T) {
	parsed, err := Parse(mappingBook(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	repo := fakeRepo{
		accounts: []domain.Account{
			{ID: "acc_changying", Name: "长赢计划", Type: domain.AccountFund, Currency: "CNY"},
		},
		prev: map[string]*domain.Snapshot{
			"acc_changying": {
				ID: "snp_prev", AccountID: "acc_changying", AsOf: "2026-11-01", Currency: "CNY",
				Items: []domain.Item{
					{Name: "示例成长混合", Code: "000216", Quantity: "900", MarketValueCents: 90_000},
					{Name: "示例价值混合", Code: "000217", Quantity: "100", MarketValueCents: 15_000},
					{Name: "已清仓基金", Code: "000111", Quantity: "5", MarketValueCents: 500},
				},
			},
		},
	}
	mapping := &Mapping{Mappings: []MappingEntry{
		{SalesOrg: salesA, TradeAccount: "T0000001", Account: "长赢计划"},
		{SalesOrg: salesA, TradeAccount: "T0000002", Account: "新组合"},
	}}
	preview, err := Build(t.Context(), repo, parsed, mapping)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(preview.Groups) != 3 {
		t.Fatalf("groups = %+v", preview.Groups)
	}
	matched := preview.Groups[0]
	if matched.Status != "matched" || matched.AccountID != "acc_changying" || matched.PreviousAsOf != "2026-11-01" {
		t.Fatalf("matched group = %+v", matched)
	}
	diff := matched.Diff
	if diff == nil || len(diff.Added) != 1 || diff.Added[0].Code != "000218" {
		t.Fatalf("added = %+v", diff)
	}
	if len(diff.Removed) != 1 || diff.Removed[0].Code != "000111" {
		t.Fatalf("removed = %+v", diff)
	}
	if len(diff.QuantityChanged) != 1 || diff.QuantityChanged[0].Code != "000216" {
		t.Fatalf("quantity changed = %+v", diff)
	}
	if len(diff.ValueChanged) != 1 || diff.ValueChanged[0].Code != "000217" {
		t.Fatalf("value changed = %+v", diff)
	}
	if preview.Groups[1].Status != "new" || !preview.Groups[1].NewAccount {
		t.Fatalf("new group = %+v", preview.Groups[1])
	}
	if !preview.Groups[2].Unmapped || preview.Groups[2].Name != salesB+"·未分组" {
		t.Fatalf("unmapped group = %+v", preview.Groups[2])
	}
	if len(preview.Unmapped) != 1 || preview.Unmapped[0].TradeAccount != "T0000009" {
		t.Fatalf("unmapped = %+v", preview.Unmapped)
	}
}

func TestPreviewDoesNotWriteDatabase(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "mio.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	now := time.Now().UTC()
	account := &domain.Account{
		ID: "acc_changying", Name: "长赢计划", Type: domain.AccountFund,
		Currency: "CNY", CreatedAt: now, UpdatedAt: now,
	}
	if err := st.UpsertAccount(t.Context(), account); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	snapshot := &domain.Snapshot{
		ID: "snp_prev", AccountID: account.ID, AsOf: "2026-11-01", Status: domain.StatusConfirmed,
		Source: domain.SourceStatement, Currency: "CNY", CreatedAt: now, ConfirmedAt: &now,
		Items: []domain.Item{{Name: "示例成长混合", Code: "000216", Quantity: "900", MarketValueCents: 90_000}},
	}
	if err := st.InsertSnapshot(t.Context(), snapshot); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	before := readDBFiles(t, dbPath)

	parsed, err := Parse(mappingBook(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ro, err := store.OpenReadOnly(dbPath)
	if err != nil || ro == nil {
		t.Fatalf("open read-only = %v, err=%v", ro, err)
	}
	preview, err := Build(t.Context(), ro, parsed, &Mapping{Mappings: []MappingEntry{
		{SalesOrg: salesA, TradeAccount: "T0000001", Account: "长赢计划"},
	}})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := ro.Close(); err != nil {
		t.Fatalf("close read-only: %v", err)
	}
	if preview.Groups[0].Diff == nil || preview.Groups[0].PreviousAsOf != "2026-11-01" {
		t.Fatalf("preview diff = %+v", preview.Groups[0])
	}
	after := readDBFiles(t, dbPath)
	if len(before) != len(after) {
		t.Fatalf("database data files changed: before %v, after %v", keys(before), keys(after))
	}
	for name, data := range before {
		if string(data) != string(after[name]) {
			t.Fatalf("preview changed %s", name)
		}
	}
}

func TestPreviewWithoutDatabase(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.db")
	st, err := store.OpenReadOnly(missing)
	if err != nil || st != nil {
		t.Fatalf("open read-only missing = %v, err=%v", st, err)
	}
	parsed, err := Parse(sampleBook(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	preview, err := Build(t.Context(), nil, parsed, &Mapping{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(preview.Groups) != 2 {
		t.Fatalf("groups = %+v", preview.Groups)
	}
}

func TestRenderedOutputHidesPersonalRows(t *testing.T) {
	parsed, err := Parse(sampleBook(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	preview, err := Build(t.Context(), nil, parsed, &Mapping{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	var text strings.Builder
	Render(&text, preview)
	for _, secret := range []string{"示例姓名", "000000********0000", "证件号码", "住址"} {
		if strings.Contains(text.String(), secret) {
			t.Fatalf("personal information leaked to text: %s", secret)
		}
	}
}

func readDBFiles(t *testing.T, dbPath string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for _, name := range []string{dbPath, dbPath + "-wal"} {
		data, err := os.ReadFile(name)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("read %s: %v", name, err)
		}
		out[filepath.Base(name)] = data
	}
	return out
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}
