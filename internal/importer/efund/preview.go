package efund

import (
	"context"
	"fmt"
	"sort"

	"github.com/curiselven/mio-money/internal/domain"
)

type group struct {
	salesOrg     string
	tradeAccount string
	mapping      MappingEntry
	holdings     []Holding
}

// Build joins parsed holdings to Mio accounts and computes read-only diffs.
// repo may be nil when no database exists yet.
func Build(ctx context.Context, repo ReadRepo, parsed *Result, mapping *Mapping) (*Preview, error) {
	if parsed == nil {
		return nil, fmt.Errorf("parse result is required")
	}
	accounts := []domain.Account{}
	if repo != nil {
		var err error
		accounts, err = repo.ListAccounts(ctx)
		if err != nil {
			return nil, err
		}
	}
	targets := map[string]MappingEntry{}
	if mapping != nil {
		for _, entry := range mapping.Mappings {
			targets[mappingKey(entry.SalesOrg, entry.TradeAccount)] = entry
		}
	}
	groups := groupHoldings(parsed.Holdings, targets)

	out := &Preview{
		File:     parsed.File,
		Meta:     parsed.Meta,
		Groups:   []GroupPreview{},
		Unmapped: []UnmappedGroup{},
		Warnings: append([]Warning{}, parsed.Warnings...),
	}
	allCodes := map[string]bool{}
	allCurrencies := map[string]bool{}

	for _, g := range groups {
		gp := GroupPreview{
			SalesOrg:     g.salesOrg,
			TradeAccount: g.tradeAccount,
			Currency:     "CNY",
			Holdings:     g.holdings,
			Status:       "unmapped",
		}
		target := g.mapping
		switch {
		case target.Ignore:
			gp.Name = g.salesOrg + "·" + g.tradeAccount + "·已忽略"
			gp.Ignored = true
			gp.Status = "ignored"
		case target.AccountID != "" || target.Account != "":
			resolved := ResolveTarget(target, accounts)
			if resolved.Issue != "" {
				gp.Name = displayTarget(target) + "·映射无效"
				gp.Target = displayTarget(target)
				gp.Status = "invalid"
				gp.Unmapped = true
				out.Warnings = append(out.Warnings, Warning{
					Code:     "mapping_target_invalid",
					Severity: SeverityWarning,
					Message:  g.salesOrg + " / " + g.tradeAccount + "：" + resolved.Issue,
				})
				break
			}
			gp.Name = resolved.Leaf.Name
			gp.Target = resolved.Leaf.Name
			gp.AccountID = resolved.Leaf.ID
			gp.AccountName = resolved.Leaf.Name
			gp.Status = "matched"
		default:
			gp.Name = g.salesOrg + "·" + g.tradeAccount + "·未分组"
			gp.Unmapped = true
		}

		codes := map[string]bool{}
		groupCurrencies := map[string]bool{}
		for _, h := range g.holdings {
			codes[h.FundCode] = true
			allCodes[h.FundCode] = true
			groupCurrencies[h.CurrencyCode] = true
			allCurrencies[h.CurrencyCode] = true
			gp.MarketValueCents += h.MarketValueCents
		}
		if len(groupCurrencies) == 1 {
			for currency := range groupCurrencies {
				gp.Currency = currency
			}
		} else {
			gp.Currency = ""
		}
		gp.FundCount = len(codes)

		if gp.AccountID != "" && parsed.Meta.ShareDate != "" {
			prev, err := repo.LastConfirmedBefore(ctx, gp.AccountID, parsed.Meta.ShareDate)
			if err != nil {
				return nil, err
			}
			if prev != nil {
				next := snapshotFor(gp.AccountID, parsed.Meta.ShareDate, g.holdings)
				diff := domain.ComputeDiff(prev, next)
				gp.Diff = &diff
				gp.PreviousAsOf = prev.AsOf
			}
		}

		out.Groups = append(out.Groups, gp)
		out.FileTotalCents += gp.MarketValueCents
		switch gp.Status {
		case "matched", "new":
			out.MappedTotalCents += gp.MarketValueCents
		case "ignored":
			out.IgnoredTotalCents += gp.MarketValueCents
		default:
			out.UnmappedTotalCents += gp.MarketValueCents
		}
		out.Total.HoldingCount += len(g.holdings)
		out.Total.MarketValueCents += gp.MarketValueCents
		if gp.Unmapped {
			out.Unmapped = append(out.Unmapped, UnmappedGroup{
				Name:             gp.Name,
				SalesOrg:         g.salesOrg,
				TradeAccount:     g.tradeAccount,
				FundCount:        gp.FundCount,
				MarketValueCents: gp.MarketValueCents,
				Currency:         gp.Currency,
			})
		}
	}
	out.Total.FundCount = len(allCodes)
	if len(allCurrencies) == 1 {
		for currency := range allCurrencies {
			out.Total.Currency = currency
		}
	}
	sort.Slice(out.Warnings, func(i, j int) bool {
		if out.Warnings[i].Row != out.Warnings[j].Row {
			return out.Warnings[i].Row < out.Warnings[j].Row
		}
		return out.Warnings[i].Code < out.Warnings[j].Code
	})
	return out, nil
}

func groupHoldings(holdings []Holding, targets map[string]MappingEntry) []group {
	order := []string{}
	byKey := map[string]*group{}
	for _, h := range holdings {
		key := mappingKey(h.SalesOrg, h.TradeAccount)
		g := byKey[key]
		if g == nil {
			g = &group{
				salesOrg:     h.SalesOrg,
				tradeAccount: h.TradeAccount,
				mapping:      targets[key],
			}
			byKey[key] = g
			order = append(order, key)
		}
		g.holdings = append(g.holdings, h)
	}
	out := make([]group, 0, len(order))
	for _, key := range order {
		out = append(out, *byKey[key])
	}
	return out
}

func displayTarget(entry MappingEntry) string {
	if entry.Account != "" {
		return entry.Account
	}
	return entry.AccountID
}

func snapshotFor(accountID, asOf string, holdings []Holding) *domain.Snapshot {
	items := make([]domain.Item, 0, len(holdings))
	for _, h := range holdings {
		items = append(items, domain.Item{
			Name:             h.FundName,
			Code:             h.FundCode,
			Quantity:         h.Quantity,
			MarketValueCents: h.MarketValueCents,
		})
	}
	return &domain.Snapshot{
		AccountID: accountID,
		AsOf:      asOf,
		Currency:  "CNY",
		Items:     items,
	}
}
