package main

import (
	"context"
	"sort"

	"github.com/curiselven/mio-money/internal/domain"
)

// OverviewService exposes a read-only snapshot of the whole portfolio to the
// desktop UI. Every confirmation change is visible on the next Get call.
type OverviewService struct {
	svc *domain.Service
}

func NewOverviewService(svc *domain.Service) *OverviewService {
	return &OverviewService{svc: svc}
}

type OverviewAccount struct {
	AccountID    string               `json:"account_id"`
	Name         string               `json:"name"`
	Type         domain.AccountType   `json:"type"`
	AsOf         string               `json:"as_of,omitempty"`
	ValueCents   int64                `json:"value_cents"`
	SharePercent float64              `json:"share_percent"`
	CashCents    *int64               `json:"cash_cents,omitempty"`
	Items        []domain.Item        `json:"items"`
	Loan         *domain.LoanBriefing `json:"loan,omitempty"`
	ParentID     string               `json:"parent_id,omitempty"`
	Depth        int                  `json:"depth"`
	HasChildren  bool                 `json:"has_children"`
}

type Overview struct {
	AsOf             string                `json:"as_of,omitempty"`
	NetCents         int64                 `json:"net_cents"`
	AssetsCents      int64                 `json:"assets_cents"`
	LiabilitiesCents int64                 `json:"liabilities_cents"`
	ChangeCents      *int64                `json:"change_cents,omitempty"`
	ChangePercent    *float64              `json:"change_percent,omitempty"`
	Accounts         []OverviewAccount     `json:"accounts"`
	History          []domain.HistoryPoint `json:"history"`
	PendingCount     int                   `json:"pending_count"`
	Empty            bool                  `json:"empty"`
}

func (s *OverviewService) Get() (*Overview, error) {
	ctx := context.Background()
	accounts, err := s.svc.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	netWorth, err := s.svc.NetWorth(ctx, "")
	if err != nil {
		return nil, err
	}
	history, err := s.svc.History(ctx, "", "", "")
	if err != nil {
		return nil, err
	}
	holdings, err := s.svc.Holdings(ctx, "")
	if err != nil {
		return nil, err
	}
	pending, err := s.svc.PendingReview(ctx)
	if err != nil {
		return nil, err
	}
	loans, err := s.svc.LoanBriefings(ctx)
	if err != nil {
		return nil, err
	}

	byAccount := map[string]domain.Holdings{}
	for _, h := range holdings {
		byAccount[h.AccountID] = h
	}

	out := &Overview{
		NetCents:         netWorth.NetCents,
		AssetsCents:      netWorth.AssetsCents,
		LiabilitiesCents: netWorth.LiabilitiesCents,
		Accounts:         make([]OverviewAccount, 0, len(accounts)),
		History:          history.Points,
		PendingCount:     len(pending),
		Empty:            len(accounts) == 0 && len(history.Points) == 0,
	}
	if len(history.Points) > 0 {
		out.AsOf = history.Points[len(history.Points)-1].AsOf
	}
	if len(history.Points) >= 2 {
		current := history.Points[len(history.Points)-1].ValueCents
		previous := history.Points[len(history.Points)-2].ValueCents
		change := current - previous
		out.ChangeCents = &change
		if previous != 0 {
			percent := float64(change) / float64(previous) * 100
			out.ChangePercent = &percent
		}
	}

	for _, account := range accounts {
		h := byAccount[account.ID]
		value := h.TotalCents
		share := 0.0
		if out.NetCents != 0 {
			share = float64(value) / float64(out.NetCents) * 100
		}
		items := h.Items
		if items == nil {
			items = []domain.Item{}
		}
		out.Accounts = append(out.Accounts, OverviewAccount{
			AccountID:    account.ID,
			Name:         account.Name,
			Type:         account.Type,
			AsOf:         account.LastSnapshotAsOf,
			ValueCents:   value,
			SharePercent: share,
			CashCents:    h.CashCents,
			Items:        items,
			ParentID:     account.ParentID,
		})
		if briefing, ok := loans[account.ID]; ok {
			out.Accounts[len(out.Accounts)-1].Loan = &briefing
		}
	}
	byID := map[string]int{}
	for i, account := range out.Accounts {
		byID[account.AccountID] = i
	}
	children := map[string][]OverviewAccount{}
	tops := []OverviewAccount{}
	for _, account := range out.Accounts {
		if account.ParentID != "" {
			if _, ok := byID[account.ParentID]; ok {
				children[account.ParentID] = append(children[account.ParentID], account)
				continue
			}
		}
		tops = append(tops, account)
	}
	for parent := range children {
		list := children[parent]
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].ValueCents == list[j].ValueCents {
				return list[i].Name < list[j].Name
			}
			return list[i].ValueCents > list[j].ValueCents
		})
		children[parent] = list
	}
	sort.SliceStable(tops, func(i, j int) bool {
		left := tops[i].ValueCents + sumOverviewChildren(children[tops[i].AccountID])
		right := tops[j].ValueCents + sumOverviewChildren(children[tops[j].AccountID])
		if left == right {
			return tops[i].Name < tops[j].Name
		}
		return left > right
	})
	ordered := make([]OverviewAccount, 0, len(out.Accounts))
	for _, top := range tops {
		top.Depth = 0
		top.HasChildren = len(children[top.AccountID]) > 0
		ordered = append(ordered, top)
		for _, child := range children[top.AccountID] {
			child.Depth = 1
			ordered = append(ordered, child)
		}
	}
	out.Accounts = ordered
	return out, nil
}

func sumOverviewChildren(accounts []OverviewAccount) int64 {
	var total int64
	for _, account := range accounts {
		total += account.ValueCents
	}
	return total
}
