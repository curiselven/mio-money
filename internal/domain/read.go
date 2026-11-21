package domain

import (
	"context"
	"sort"
)

type AccountSummary struct {
	Account
	LastSnapshotAsOf string `json:"last_snapshot_as_of,omitempty"`
}

func (s *Service) ListAccounts(ctx context.Context) ([]AccountSummary, error) {
	accounts, err := s.repo.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	dates, err := s.repo.LastConfirmedDates(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AccountSummary, len(accounts))
	for i, a := range accounts {
		out[i] = AccountSummary{Account: a, LastSnapshotAsOf: dates[a.ID]}
	}
	return out, nil
}

type Holdings struct {
	AccountID  string `json:"account_id"`
	Name       string `json:"name"`
	AsOf       string `json:"as_of"`
	CashCents  *int64 `json:"cash_cents"`
	TotalCents int64  `json:"total_cents"`
	Items      []Item `json:"items"`
}

func (s *Service) Holdings(ctx context.Context, accountID string) ([]Holdings, error) {
	accounts, err := s.repo.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[string]Account{}
	for _, a := range accounts {
		byID[a.ID] = a
	}
	var snaps []*Snapshot
	if accountID != "" {
		if _, err := s.repo.GetAccount(ctx, accountID); err != nil {
			return nil, err
		}
		sn, err := s.repo.LastConfirmedBefore(ctx, accountID, "")
		if err != nil {
			return nil, err
		}
		if sn != nil {
			snaps = append(snaps, sn)
		}
	} else {
		snaps, err = s.repo.LatestConfirmedPerAccount(ctx, "")
		if err != nil {
			return nil, err
		}
	}
	out := make([]Holdings, 0, len(snaps))
	for _, sn := range snaps {
		a := byID[sn.AccountID]
		out = append(out, Holdings{
			AccountID:  sn.AccountID,
			Name:       a.Name,
			AsOf:       sn.AsOf,
			CashCents:  sn.CashCents,
			TotalCents: sn.TotalCents(),
			Items:      sn.Items,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

type AccountValue struct {
	AccountID  string      `json:"account_id"`
	Name       string      `json:"name"`
	Type       AccountType `json:"type"`
	ValueCents int64       `json:"value_cents"`
}

type TypeTotal struct {
	Type             AccountType `json:"type"`
	AssetsCents      int64       `json:"assets_cents"`
	LiabilitiesCents int64       `json:"liabilities_cents"`
	NetCents         int64       `json:"net_cents"`
}

type NetWorth struct {
	AsOf             string         `json:"as_of,omitempty"`
	AssetsCents      int64          `json:"assets_cents"`
	LiabilitiesCents int64          `json:"liabilities_cents"`
	NetCents         int64          `json:"net_cents"`
	ByType           []TypeTotal    `json:"by_type"`
	ByAccount        []AccountValue `json:"by_account"`
}

func (s *Service) NetWorth(ctx context.Context, asOf string) (*NetWorth, error) {
	if asOf != "" {
		if err := validDate(asOf); err != nil {
			return nil, err
		}
	}
	return s.netWorthAt(ctx, asOf)
}

func (s *Service) netWorthAt(ctx context.Context, asOf string) (*NetWorth, error) {
	accounts, err := s.repo.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[string]Account{}
	for _, a := range accounts {
		byID[a.ID] = a
	}
	snaps, err := s.repo.LatestConfirmedPerAccount(ctx, asOf)
	if err != nil {
		return nil, err
	}
	out := &NetWorth{AsOf: asOf, ByType: []TypeTotal{}, ByAccount: []AccountValue{}}
	typeTotals := map[AccountType]*TypeTotal{}
	for _, sn := range snaps {
		a := byID[sn.AccountID]
		if a.Archived {
			continue
		}
		value := sn.TotalCents()
		out.ByAccount = append(out.ByAccount, AccountValue{
			AccountID: a.ID, Name: a.Name, Type: a.Type, ValueCents: value,
		})
		total := typeTotals[a.Type]
		if total == nil {
			total = &TypeTotal{Type: a.Type}
			typeTotals[a.Type] = total
		}
		if a.Type == AccountLiability {
			out.LiabilitiesCents += -value
			total.LiabilitiesCents += -value
		} else {
			out.AssetsCents += value
			total.AssetsCents += value
		}
	}
	out.NetCents = out.AssetsCents - out.LiabilitiesCents
	order := []AccountType{AccountCash, AccountFund, AccountStock, AccountMixed, AccountLiability, AccountOther}
	for _, t := range order {
		if total := typeTotals[t]; total != nil {
			total.NetCents = total.AssetsCents - total.LiabilitiesCents
			out.ByType = append(out.ByType, *total)
		}
	}
	sort.Slice(out.ByAccount, func(i, j int) bool { return out.ByAccount[i].Name < out.ByAccount[j].Name })
	return out, nil
}

type HistoryPoint struct {
	AsOf       string `json:"as_of"`
	ValueCents int64  `json:"value_cents"`
}

type History struct {
	AccountID string         `json:"account_id,omitempty"`
	Points    []HistoryPoint `json:"points"`
}

func (s *Service) History(ctx context.Context, accountID, from, to string) (*History, error) {
	if from != "" {
		if err := validDate(from); err != nil {
			return nil, err
		}
	}
	if to != "" {
		if err := validDate(to); err != nil {
			return nil, err
		}
	}
	snaps, err := s.repo.ListConfirmed(ctx, accountID, from, to)
	if err != nil {
		return nil, err
	}
	out := &History{AccountID: accountID, Points: []HistoryPoint{}}
	if accountID != "" {
		values := map[string]int64{}
		for _, sn := range snaps {
			values[sn.AsOf] = sn.TotalCents()
		}
		dates := make([]string, 0, len(values))
		for d := range values {
			dates = append(dates, d)
		}
		sort.Strings(dates)
		for _, d := range dates {
			out.Points = append(out.Points, HistoryPoint{AsOf: d, ValueCents: values[d]})
		}
		return out, nil
	}
	dates := map[string]bool{}
	for _, sn := range snaps {
		dates[sn.AsOf] = true
	}
	sorted := make([]string, 0, len(dates))
	for d := range dates {
		sorted = append(sorted, d)
	}
	sort.Strings(sorted)
	for _, d := range sorted {
		nw, err := s.netWorthAt(ctx, d)
		if err != nil {
			return nil, err
		}
		out.Points = append(out.Points, HistoryPoint{AsOf: d, ValueCents: nw.NetCents})
	}
	return out, nil
}

func (s *Service) Pending(ctx context.Context) ([]*Snapshot, error) {
	return s.repo.ListPending(ctx)
}

type PendingReview struct {
	SnapshotID  string    `json:"snapshot_id"`
	AccountID   string    `json:"account_id"`
	AccountName string    `json:"account_name"`
	AsOf        string    `json:"as_of"`
	Source      Source    `json:"source"`
	Diff        Diff      `json:"diff"`
	Anomalies   []Anomaly `json:"anomalies"`
}

// PendingReview lists pending snapshots that currently violate an anomaly rule.
func (s *Service) PendingReview(ctx context.Context) ([]PendingReview, error) {
	accounts, err := s.repo.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[string]Account{}
	for _, account := range accounts {
		byID[account.ID] = account
	}
	pending, err := s.repo.ListPending(ctx)
	if err != nil {
		return nil, err
	}
	out := []PendingReview{}
	for _, snapshot := range pending {
		account := byID[snapshot.AccountID]
		prev, err := s.repo.LastConfirmedBefore(ctx, snapshot.AccountID, snapshot.AsOf)
		if err != nil {
			return nil, err
		}
		anomalies := StructuredAnomalies(prev, snapshot, &account, s.threshold)
		if len(anomalies) == 0 {
			continue
		}
		out = append(out, PendingReview{
			SnapshotID:  snapshot.ID,
			AccountID:   snapshot.AccountID,
			AccountName: account.Name,
			AsOf:        snapshot.AsOf,
			Source:      snapshot.Source,
			Diff:        ComputeDiff(prev, snapshot),
			Anomalies:   anomalies,
		})
	}
	return out, nil
}
