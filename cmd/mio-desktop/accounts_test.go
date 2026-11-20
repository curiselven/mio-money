package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/curiselven/mio-money/internal/domain"
	"github.com/curiselven/mio-money/internal/store"
)

func boolp(value bool) *bool { return &value }

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
		Currency: "CNY", Owner: "我",
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
			Name: "示例" + string(accountType), Type: accountType, Currency: "CNY", Owner: "我",
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
