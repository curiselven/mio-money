package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/curiselven/mio-money/internal/domain"
	"github.com/curiselven/mio-money/internal/store"
)

func boolp(value bool) *bool { return &value }

func ownerp(value string) *string { return &value }

func TestAccountServiceTreeRenameAndArchive(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "mio.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := domain.NewService(st, 0.2)
	accounts := NewAccountService(svc, st)

	top, err := accounts.SaveAccount(AccountFormInput{
		Name: "示例账户", Platform: "示例平台", Type: domain.AccountFund,
		Currency: "CNY", Owner: ownerp("我"),
	})
	if err != nil {
		t.Fatal(err)
	}
	portfolio, err := accounts.SavePortfolio(PortfolioFormInput{ParentID: top.ID, Name: "示例组合"})
	if err != nil {
		t.Fatal(err)
	}
	tree, err := accounts.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Accounts) != 1 || len(tree.Portfolios) != 1 || len(tree.Owners) != 1 || tree.Owners[0] != "我" {
		t.Fatalf("tree = %+v", tree)
	}
	if tree.Portfolios[0].Owner != "我" || tree.Portfolios[0].Currency != "CNY" || tree.Portfolios[0].Type != domain.AccountFund {
		t.Fatalf("portfolio inheritance = %+v", tree.Portfolios[0])
	}

	if _, err := accounts.SaveAccount(AccountFormInput{ID: top.ID, Name: "改名账户"}); err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.SavePortfolio(PortfolioFormInput{ID: portfolio.ID, Name: "改名组合"}); err != nil {
		t.Fatal(err)
	}
	tree, err = accounts.List()
	if err != nil || tree.Accounts[0].Name != "改名账户" || tree.Portfolios[0].Name != "改名组合" {
		t.Fatalf("rename = %+v, err=%v", tree, err)
	}
	if tree.Accounts[0].Platform != "示例平台" {
		t.Fatalf("rename cleared platform: %+v", tree.Accounts[0])
	}
	storedPortfolio, err := st.GetAccount(context.Background(), portfolio.ID)
	if err != nil || tree.Accounts[0].Owner != "我" || storedPortfolio.Owner != "我" {
		t.Fatalf("rename cleared owner: tree=%+v portfolio=%+v err=%v", tree.Accounts[0], storedPortfolio, err)
	}

	empty, err := accounts.SaveAccount(AccountFormInput{Name: "空账户", Type: domain.AccountCash})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.SaveAccount(AccountFormInput{ID: empty.ID, Archived: boolp(true)}); err != nil {
		t.Fatalf("archive empty: %v", err)
	}
	if _, err := accounts.SaveAccount(AccountFormInput{ID: top.ID, Archived: boolp(true)}); err == nil {
		t.Fatal("account with portfolio was archived")
	}
	if _, err := accounts.SaveAccount(AccountFormInput{Name: "负债", Type: domain.AccountLiability}); err == nil {
		t.Fatal("liability account was created from account manager")
	}

	result, err := svc.Submit(context.Background(), domain.SubmitInput{
		AccountID: portfolio.ID, AsOf: "2026-11-17", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "示例基金", Code: "000001", Quantity: "1", MarketValueCents: 100}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Confirm(context.Background(), result.SnapshotID); err != nil {
		t.Fatal(err)
	}
	if _, err := accounts.SavePortfolio(PortfolioFormInput{ID: portfolio.ID, Archived: boolp(true)}); err == nil {
		t.Fatal("portfolio with snapshot was archived")
	}
}

func TestPortfolioOwnerFollowsParent(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "mio.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := domain.NewService(st, 0.2)
	accounts := NewAccountService(svc, st)

	top, err := accounts.SaveAccount(AccountFormInput{Name: "示例账户", Type: domain.AccountFund, Owner: ownerp("林澈")})
	if err != nil {
		t.Fatal(err)
	}
	portfolio, err := accounts.SavePortfolio(PortfolioFormInput{ParentID: top.ID, Name: "示例组合"})
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{
		Name: "示例组合二", ParentID: top.ID, Owner: ownerp("别人"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if portfolio.Owner != "林澈" || explicit.Owner != "林澈" {
		t.Fatalf("portfolio create owner = %q / %q", portfolio.Owner, explicit.Owner)
	}

	stored, err := st.GetAccount(ctx, portfolio.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored.Owner = ""
	if err := st.UpsertAccount(ctx, stored); err != nil {
		t.Fatal(err)
	}
	summaries, err := svc.ListAccounts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range summaries {
		if account.ID == portfolio.ID && account.Owner != "林澈" {
			t.Fatalf("read effective owner = %q", account.Owner)
		}
	}
	tree, err := accounts.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range tree.Portfolios {
		if item.Owner != "林澈" {
			t.Fatalf("tree portfolio owner = %+v", item)
		}
	}

	if _, err := accounts.SaveAccount(AccountFormInput{ID: top.ID, Name: "改名账户"}); err != nil {
		t.Fatal(err)
	}
	storedTop, err := st.GetAccount(ctx, top.ID)
	if err != nil || storedTop.Name != "改名账户" || storedTop.Owner != "林澈" {
		t.Fatalf("rename top = %+v, err=%v", storedTop, err)
	}

	if _, err := accounts.SaveAccount(AccountFormInput{ID: top.ID, Owner: ownerp("周岚")}); err != nil {
		t.Fatal(err)
	}
	for _, account := range mustListAccounts(t, st) {
		if account.ParentID == top.ID && account.Owner != "周岚" {
			t.Fatalf("cascade owner = %+v", account)
		}
	}

	if _, err := accounts.SaveAccount(AccountFormInput{ID: top.ID, Owner: ownerp("")}); err != nil {
		t.Fatal(err)
	}
	for _, account := range mustListAccounts(t, st) {
		if account.ParentID == top.ID && account.Owner != "" {
			t.Fatalf("clear owner = %+v", account)
		}
	}
	tree, err = accounts.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range tree.Portfolios {
		if item.Owner != "" {
			t.Fatalf("tree cleared owner = %+v", item)
		}
	}
}

func mustListAccounts(t *testing.T, st *store.Store) []domain.Account {
	t.Helper()
	accounts, err := st.ListAccounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return accounts
}

func TestAccountServiceEditsAllManagedTypes(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "mio.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := domain.NewService(st, 0.2)
	accounts := NewAccountService(svc, st)
	for _, accountType := range []domain.AccountType{
		domain.AccountFund, domain.AccountMixed, domain.AccountCash, domain.AccountStock, domain.AccountOther,
	} {
		account, err := accounts.SaveAccount(AccountFormInput{
			Name: "示例" + string(accountType), Type: accountType, Currency: "CNY", Owner: ownerp("我"),
		})
		if err != nil {
			t.Fatalf("%s create: %v", accountType, err)
		}
		updated, err := accounts.SaveAccount(AccountFormInput{ID: account.ID, Name: "改名" + string(accountType)})
		if err != nil || updated.Name != "改名"+string(accountType) || updated.Type != accountType {
			t.Fatalf("%s edit = %+v, err=%v", accountType, updated, err)
		}
	}
	liability, err := svc.UpsertAccount(context.Background(), domain.UpsertAccountInput{
		Name: "示例房贷", Type: domain.AccountLiability,
	})
	if err != nil {
		t.Fatal(err)
	}
	tree, err := accounts.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range tree.Accounts {
		if item.ID == liability.ID {
			t.Fatal("liability account appeared in account management")
		}
	}
}

func TestAccountServiceListSummarizesLatestSnapshots(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "mio.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := domain.NewService(st, 0.2)
	accounts := NewAccountService(svc, st)

	top, err := accounts.SaveAccount(AccountFormInput{Name: "示例账户", Type: domain.AccountFund, Owner: ownerp("我")})
	if err != nil {
		t.Fatal(err)
	}
	first, err := accounts.SavePortfolio(PortfolioFormInput{ParentID: top.ID, Name: "示例组合一"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := accounts.SavePortfolio(PortfolioFormInput{ParentID: top.ID, Name: "示例组合二"})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := accounts.SaveAccount(AccountFormInput{Name: "空账户", Type: domain.AccountCash})
	if err != nil {
		t.Fatal(err)
	}
	submitConfirmed(t, svc, first.ID, "2026-10-01", 120_000)
	submitConfirmed(t, svc, second.ID, "2026-10-18", 80_000)

	tree, err := accounts.List()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]AccountManageItem{}
	for _, item := range tree.Accounts {
		byID[item.ID] = item
	}
	for _, item := range tree.Portfolios {
		byID[item.ID] = item
	}
	parent := byID[top.ID]
	if parent.ChildCount != 2 || parent.LatestValueCents == nil || *parent.LatestValueCents != 200_000 ||
		parent.LatestAsOf != "2026-10-18" {
		t.Fatalf("parent summary = %+v", parent)
	}
	if firstItem := byID[first.ID]; firstItem.LatestValueCents == nil ||
		*firstItem.LatestValueCents != 120_000 || firstItem.LatestAsOf != "2026-10-01" {
		t.Fatalf("first portfolio summary = %+v", firstItem)
	}
	if secondItem := byID[second.ID]; secondItem.LatestValueCents == nil ||
		*secondItem.LatestValueCents != 80_000 || secondItem.LatestAsOf != "2026-10-18" {
		t.Fatalf("second portfolio summary = %+v", secondItem)
	}
	if emptyItem := byID[empty.ID]; emptyItem.LatestValueCents != nil || emptyItem.LatestAsOf != "" {
		t.Fatalf("empty account summary = %+v", emptyItem)
	}
}
