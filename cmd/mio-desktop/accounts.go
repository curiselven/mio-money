package main

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/curiselven/mio-money/internal/domain"
	"github.com/curiselven/mio-money/internal/store"
)

type AccountService struct {
	svc *domain.Service
	st  *store.Store
}

type AccountManageItem struct {
	ID               string             `json:"id"`
	Name             string             `json:"name"`
	Platform         string             `json:"platform,omitempty"`
	Type             domain.AccountType `json:"type"`
	Currency         string             `json:"currency"`
	Owner            string             `json:"owner,omitempty"`
	ParentID         string             `json:"parent_id,omitempty"`
	Archived         bool               `json:"archived"`
	HasSnapshots     bool               `json:"has_snapshots"`
	HasChildren      bool               `json:"has_children"`
	ChildCount       int                `json:"child_count"`
	LatestValueCents *int64             `json:"latest_value_cents,omitempty"`
	LatestAsOf       string             `json:"latest_as_of,omitempty"`
}

type AccountTree struct {
	Accounts   []AccountManageItem `json:"accounts"`
	Portfolios []AccountManageItem `json:"portfolios"`
	Owners     []string            `json:"owners"`
}

type AccountFormInput struct {
	ID       string             `json:"id,omitempty"`
	Name     string             `json:"name"`
	Platform string             `json:"platform,omitempty"`
	Type     domain.AccountType `json:"type,omitempty"`
	Currency string             `json:"currency,omitempty"`
	Owner    string             `json:"owner,omitempty"`
	Archived *bool              `json:"archived,omitempty"`
}

type PortfolioFormInput struct {
	ID       string `json:"id,omitempty"`
	ParentID string `json:"parent_id,omitempty"`
	Name     string `json:"name"`
	Archived *bool  `json:"archived,omitempty"`
}

func NewAccountService(svc *domain.Service, st *store.Store) *AccountService {
	return &AccountService{svc: svc, st: st}
}

func (s *AccountService) List() (*AccountTree, error) {
	ctx := context.Background()
	accounts, err := s.svc.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	holdings, err := s.svc.Holdings(ctx, "")
	if err != nil {
		return nil, err
	}
	latest := make(map[string]domain.Holdings, len(holdings))
	for _, holding := range holdings {
		latest[holding.AccountID] = holding
	}
	childCounts := map[string]int{}
	for _, account := range accounts {
		if account.ParentID != "" {
			childCounts[account.ParentID]++
		}
	}
	owners := map[string]bool{}
	out := &AccountTree{Accounts: []AccountManageItem{}, Portfolios: []AccountManageItem{}, Owners: []string{}}
	for _, account := range accounts {
		if account.Owner != "" {
			owners[account.Owner] = true
		}
		if account.Type == domain.AccountLiability {
			continue
		}
		hasSnapshots, err := s.st.HasAnySnapshot(ctx, account.ID)
		if err != nil {
			return nil, err
		}
		hasChildren, err := s.st.HasChildren(ctx, account.ID)
		if err != nil {
			return nil, err
		}
		item := AccountManageItem{
			ID: account.ID, Name: account.Name, Platform: account.Platform,
			Type: account.Type, Currency: account.Currency, Owner: account.Owner,
			ParentID: account.ParentID, Archived: account.Archived,
			HasSnapshots: hasSnapshots, HasChildren: hasChildren,
			ChildCount: childCounts[account.ID],
		}
		if holding, ok := latest[account.ID]; ok {
			value := holding.TotalCents
			item.LatestValueCents = &value
			item.LatestAsOf = holding.AsOf
		}
		if account.ParentID == "" {
			out.Accounts = append(out.Accounts, item)
		} else {
			out.Portfolios = append(out.Portfolios, item)
		}
	}
	for i := range out.Accounts {
		var total int64
		hasValue := false
		asOf := ""
		if out.Accounts[i].LatestValueCents != nil {
			total = *out.Accounts[i].LatestValueCents
			hasValue = true
			asOf = out.Accounts[i].LatestAsOf
		}
		for _, portfolio := range out.Portfolios {
			if portfolio.ParentID != out.Accounts[i].ID || portfolio.LatestValueCents == nil {
				continue
			}
			total += *portfolio.LatestValueCents
			hasValue = true
			if portfolio.LatestAsOf > asOf {
				asOf = portfolio.LatestAsOf
			}
		}
		if hasValue {
			out.Accounts[i].LatestValueCents = &total
			out.Accounts[i].LatestAsOf = asOf
		}
	}
	for owner := range owners {
		out.Owners = append(out.Owners, owner)
	}
	sort.Slice(out.Owners, func(i, j int) bool { return out.Owners[i] < out.Owners[j] })
	sort.Slice(out.Accounts, func(i, j int) bool { return out.Accounts[i].Name < out.Accounts[j].Name })
	sort.Slice(out.Portfolios, func(i, j int) bool { return out.Portfolios[i].Name < out.Portfolios[j].Name })
	return out, nil
}

func (s *AccountService) SaveAccount(in AccountFormInput) (*domain.Account, error) {
	if in.Type == domain.AccountLiability {
		return nil, errors.New("负债账户请在贷款模块管理")
	}
	if in.Type != "" && in.Type != domain.AccountFund && in.Type != domain.AccountMixed &&
		in.Type != domain.AccountCash && in.Type != domain.AccountStock && in.Type != domain.AccountOther {
		return nil, errors.New("账户类型必须是 fund、mixed、cash、stock 或 other")
	}
	if in.ID == "" {
		accounts, err := s.st.ListAccounts(context.Background())
		if err != nil {
			return nil, err
		}
		for _, account := range accounts {
			if !account.Archived && account.ParentID == "" && strings.EqualFold(strings.TrimSpace(account.Name), strings.TrimSpace(in.Name)) {
				return nil, errors.New("同名账户已存在")
			}
		}
	}
	return s.svc.UpsertAccount(context.Background(), domain.UpsertAccountInput{
		ID: in.ID, Name: in.Name, Platform: in.Platform, Type: in.Type,
		Currency: in.Currency, Owner: in.Owner, Archived: in.Archived,
	})
}

func (s *AccountService) SavePortfolio(in PortfolioFormInput) (*domain.Account, error) {
	ctx := context.Background()
	if in.ID == "" {
		if in.ParentID == "" {
			return nil, errors.New("组合必须属于一个顶层账户")
		}
		accounts, err := s.st.ListAccounts(ctx)
		if err != nil {
			return nil, err
		}
		for _, account := range accounts {
			if !account.Archived && account.ParentID == in.ParentID &&
				strings.EqualFold(strings.TrimSpace(account.Name), strings.TrimSpace(in.Name)) {
				return nil, errors.New("该账户下已有同名组合")
			}
		}
		return s.svc.UpsertAccount(ctx, domain.UpsertAccountInput{
			Name: in.Name, ParentID: in.ParentID,
		})
	}
	account, err := s.st.GetAccount(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	if account.ParentID == "" {
		return nil, errors.New("该账户不是组合")
	}
	return s.svc.UpsertAccount(ctx, domain.UpsertAccountInput{
		ID: in.ID, Name: in.Name, Archived: in.Archived,
	})
}
