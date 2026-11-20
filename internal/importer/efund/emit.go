package efund

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type Hint struct {
	FundCount       int      `json:"fund_count"`
	MarketValueYuan string   `json:"market_value_yuan"`
	TopFunds        []string `json:"top_funds"`
}

// Skeleton generates one empty mapping entry per sales_org + trade_account,
// ordered by market value descending.
func Skeleton(parsed *Result) *Mapping {
	groups := groupHoldings(parsed.Holdings, nil)
	totals := map[string]int64{}
	entries := make([]MappingEntry, 0, len(groups))
	for _, g := range groups {
		total, hint := hintFor(g.holdings)
		key := mappingKey(g.salesOrg, g.tradeAccount)
		totals[key] = total
		entries = append(entries, MappingEntry{
			SalesOrg:     g.salesOrg,
			TradeAccount: g.tradeAccount,
			Hint:         &hint,
		})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		left := totals[mappingKey(entries[i].SalesOrg, entries[i].TradeAccount)]
		right := totals[mappingKey(entries[j].SalesOrg, entries[j].TradeAccount)]
		if left != right {
			return left > right
		}
		if entries[i].SalesOrg != entries[j].SalesOrg {
			return entries[i].SalesOrg < entries[j].SalesOrg
		}
		return entries[i].TradeAccount < entries[j].TradeAccount
	})
	return &Mapping{Mappings: entries}
}

// MergeMapping keeps existing account choices, refreshes hints, and appends
// entries that are absent from the current export.
func MergeMapping(existing, generated *Mapping) (Mapping, int) {
	if existing == nil {
		existing = &Mapping{}
	}
	if generated == nil {
		generated = &Mapping{}
	}
	old := make(map[string]MappingEntry, len(existing.Mappings))
	for _, entry := range existing.Mappings {
		old[mappingKey(entry.SalesOrg, entry.TradeAccount)] = entry
	}
	used := map[string]bool{}
	out := make([]MappingEntry, 0, len(generated.Mappings)+len(existing.Mappings))
	added := 0
	for _, entry := range generated.Mappings {
		key := mappingKey(entry.SalesOrg, entry.TradeAccount)
		used[key] = true
		if previous, ok := old[key]; ok {
			previous.Hint = entry.Hint
			out = append(out, previous)
			continue
		}
		added++
		out = append(out, entry)
	}
	for _, entry := range existing.Mappings {
		if !used[mappingKey(entry.SalesOrg, entry.TradeAccount)] {
			out = append(out, entry)
		}
	}
	return Mapping{Mappings: out}, added
}

func WriteMapping(path string, mapping *Mapping) error {
	if mapping == nil {
		mapping = &Mapping{}
	}
	data, err := json.MarshalIndent(mapping, "", "  ")
	if err != nil {
		return fmt.Errorf("encode mapping: %w", err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create mapping directory: %w", err)
	}
	file, err := os.CreateTemp(dir, ".efund-map-*.tmp")
	if err != nil {
		return fmt.Errorf("create mapping temp file: %w", err)
	}
	temp := file.Name()
	defer os.Remove(temp)
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return fmt.Errorf("set mapping permissions: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write mapping: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close mapping: %w", err)
	}
	if err := os.Rename(temp, path); err != nil {
		return fmt.Errorf("replace mapping: %w", err)
	}
	return nil
}

func hintFor(holdings []Holding) (int64, Hint) {
	codes := map[string]bool{}
	fundTotals := map[string]int64{}
	order := []string{}
	funds := make([]struct {
		name  string
		cents int64
	}, 0, len(holdings))
	var total int64
	for _, h := range holdings {
		codes[h.FundCode] = true
		total += h.MarketValueCents
		if _, ok := fundTotals[h.FundName]; !ok {
			order = append(order, h.FundName)
		}
		fundTotals[h.FundName] += h.MarketValueCents
	}
	for _, name := range order {
		funds = append(funds, struct {
			name  string
			cents int64
		}{name: name, cents: fundTotals[name]})
	}
	sort.SliceStable(funds, func(i, j int) bool {
		if funds[i].cents != funds[j].cents {
			return funds[i].cents > funds[j].cents
		}
		return funds[i].name < funds[j].name
	})
	top := make([]string, 0, 3)
	for _, fund := range funds {
		if len(top) == 3 {
			break
		}
		top = append(top, fund.name)
	}
	return total, Hint{
		FundCount:       len(codes),
		MarketValueYuan: yuanString(total),
		TopFunds:        top,
	}
}
