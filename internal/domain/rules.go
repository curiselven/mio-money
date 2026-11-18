package domain

import (
	"fmt"
	"math/big"
	"strings"
)

// Anomalies returns every rule violation that blocks confirm_snapshot.
// threshold is a fraction, e.g. 0.2 for 20%.
func Anomalies(prev, next *Snapshot, account *Account, threshold float64) []string {
	out := []string{}
	if prev != nil {
		if change := percentChange(prev.TotalCents(), next.TotalCents()); change > threshold*100 {
			out = append(out, fmt.Sprintf("账户总市值相对上次变化 %s，超过阈值 %s", formatPercent(change), formatPercent(threshold*100)))
		}
		_, matched := align(prev.Items, next.Items)
		for j, o := range prev.Items {
			if !matched[j] && !quantityIsZero(o.Quantity) {
				out = append(out, fmt.Sprintf("上次持有的「%s」本次整条消失", o.Name))
			}
		}
	}
	for _, it := range next.Items {
		if it.MarketValueCents < 0 {
			out = append(out, fmt.Sprintf("「%s」的市值为负数", it.Name))
		}
		if it.Quantity != "" {
			if q, ok := new(big.Rat).SetString(it.Quantity); ok && q.Sign() < 0 {
				out = append(out, fmt.Sprintf("「%s」的份额为负数", it.Name))
			}
		}
	}
	if account != nil {
		currency := next.Currency
		if currency == "" {
			currency = account.Currency
		}
		if !strings.EqualFold(currency, account.Currency) {
			out = append(out, fmt.Sprintf("快照币种 %s 与账户币种 %s 不一致", currency, account.Currency))
		}
	}
	return out
}
