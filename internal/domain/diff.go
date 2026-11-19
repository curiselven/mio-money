package domain

import (
	"fmt"
	"math/big"
)

type ItemLine struct {
	Name             string `json:"name"`
	Code             string `json:"code,omitempty"`
	Quantity         string `json:"quantity,omitempty"`
	MarketValueCents int64  `json:"market_value_cents"`
}

type ItemChange struct {
	Name            string `json:"name"`
	Code            string `json:"code,omitempty"`
	OldQuantity     string `json:"old_quantity,omitempty"`
	NewQuantity     string `json:"new_quantity,omitempty"`
	OldValueCents   int64  `json:"old_value_cents"`
	NewValueCents   int64  `json:"new_value_cents"`
	DeltaValueCents int64  `json:"delta_value_cents"`
}

type Diff struct {
	Added            []ItemLine   `json:"added"`
	Removed          []ItemLine   `json:"removed"`
	QuantityChanged  []ItemChange `json:"quantity_changed"`
	ValueChanged     []ItemChange `json:"value_changed"`
	Cleared          []ItemChange `json:"cleared"`
	CashBeforeCents  *int64       `json:"cash_before_cents"`
	CashAfterCents   *int64       `json:"cash_after_cents"`
	TotalBeforeCents int64        `json:"total_before_cents"`
	TotalAfterCents  int64        `json:"total_after_cents"`
	TotalDeltaCents  int64        `json:"total_delta_cents"`
}

// ComputeDiff aligns next's items with prev's items: code first, then name.
func ComputeDiff(prev, next *Snapshot) Diff {
	d := Diff{
		Added:           []ItemLine{},
		Removed:         []ItemLine{},
		QuantityChanged: []ItemChange{},
		ValueChanged:    []ItemChange{},
		Cleared:         []ItemChange{},
	}
	if prev != nil {
		d.CashBeforeCents = prev.CashCents
		d.TotalBeforeCents = prev.TotalCents()
	}
	d.CashAfterCents = next.CashCents
	d.TotalAfterCents = next.TotalCents()
	d.TotalDeltaCents = d.TotalAfterCents - d.TotalBeforeCents

	oldItems := []Item(nil)
	if prev != nil {
		oldItems = prev.Items
	}
	match, used := align(oldItems, next.Items)

	for i, n := range next.Items {
		if match[i] < 0 {
			d.Added = append(d.Added, ItemLine{Name: n.Name, Code: n.Code, Quantity: n.Quantity, MarketValueCents: n.MarketValueCents})
			continue
		}
		o := oldItems[match[i]]
		sameQuantity := quantityEqual(o.Quantity, n.Quantity)
		change := ItemChange{
			Name:            n.Name,
			Code:            n.Code,
			OldQuantity:     o.Quantity,
			NewQuantity:     n.Quantity,
			OldValueCents:   o.MarketValueCents,
			NewValueCents:   n.MarketValueCents,
			DeltaValueCents: n.MarketValueCents - o.MarketValueCents,
		}
		switch {
		case quantityIsZero(n.Quantity) && !quantityIsZero(o.Quantity):
			d.Cleared = append(d.Cleared, change)
		case !sameQuantity:
			d.QuantityChanged = append(d.QuantityChanged, change)
		case o.MarketValueCents != n.MarketValueCents:
			d.ValueChanged = append(d.ValueChanged, change)
		}
	}
	for j, o := range oldItems {
		if !used[j] {
			d.Removed = append(d.Removed, ItemLine{Name: o.Name, Code: o.Code, Quantity: o.Quantity, MarketValueCents: o.MarketValueCents})
		}
	}
	return d
}

// align maps each new item to an old item index (-1 when new). Code wins; an
// item without a code matches by name.
func align(oldItems, newItems []Item) ([]int, []bool) {
	match := make([]int, len(newItems))
	for i := range match {
		match[i] = -1
	}
	used := make([]bool, len(oldItems))
	for i, n := range newItems {
		if n.Code == "" {
			continue
		}
		for j, o := range oldItems {
			if !used[j] && o.Code == n.Code {
				match[i], used[j] = j, true
				break
			}
		}
	}
	for i, n := range newItems {
		if match[i] >= 0 {
			continue
		}
		for j, o := range oldItems {
			if !used[j] && o.Name == n.Name {
				match[i], used[j] = j, true
				break
			}
		}
	}
	return match, used
}

func quantityEqual(a, b string) bool {
	if a == b {
		return true
	}
	ra, okA := new(big.Rat).SetString(a)
	rb, okB := new(big.Rat).SetString(b)
	if !okA || !okB {
		return false
	}
	return ra.Cmp(rb) == 0
}

// quantityIsZero reports whether q is an explicit decimal zero ("0", "0.00").
// An empty quantity means unknown, not zero.
func quantityIsZero(q string) bool {
	if q == "" {
		return false
	}
	r, ok := new(big.Rat).SetString(q)
	return ok && r.Sign() == 0
}

func percentChange(before, after int64) float64 {
	if before == 0 {
		if after == 0 {
			return 0
		}
		return 100
	}
	diff := float64(after - before)
	if diff < 0 {
		diff = -diff
	}
	base := float64(before)
	if base < 0 {
		base = -base
	}
	return diff / base * 100
}

func formatPercent(v float64) string {
	return fmt.Sprintf("%.1f%%", v)
}
