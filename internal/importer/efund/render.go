package efund

import (
	"fmt"
	"io"

	"github.com/curiselven/mio-money/internal/domain"
)

func Render(w io.Writer, p *Preview) {
	fmt.Fprintf(w, "文件：%s\n", p.File)
	if p.Meta.ShareDate != "" {
		fmt.Fprintf(w, "份额日期：%s\n", p.Meta.ShareDate)
	}
	if p.Meta.PrintTime != "" {
		fmt.Fprintf(w, "打印时间：%s\n", p.Meta.PrintTime)
	}
	switch {
	case p.Meta.TotalRecords == nil:
		fmt.Fprintf(w, "记录数：文件未给出总记录数，实际 %d 行\n", p.Meta.ActualRows)
	case p.Meta.RecordsMatch:
		fmt.Fprintf(w, "记录数：%d / %d ✓\n", *p.Meta.TotalRecords, p.Meta.ActualRows)
	default:
		fmt.Fprintf(w, "记录数：文件写 %d，实际 %d ✗\n", *p.Meta.TotalRecords, p.Meta.ActualRows)
	}
	if p.Meta.NAVDateMismatchFunds > 0 {
		fmt.Fprintf(w, "净值日期不一致：%d 只（QDII 常见）\n", p.Meta.NAVDateMismatchFunds)
	}

	fmt.Fprintln(w, "\n按组合汇总（未写入任何数据）")
	for _, g := range p.Groups {
		fmt.Fprintf(w, "\n- %s  %s/%s\n", g.Name, g.SalesOrg, g.TradeAccount)
		switch g.Status {
		case "matched":
			fmt.Fprintf(w, "  对应账户：%s（已关联）\n", g.AccountName)
		case "ignored":
			fmt.Fprintln(w, "  对应账户：已忽略")
		case "invalid":
			fmt.Fprintln(w, "  对应账户：映射无效")
		default:
			fmt.Fprintf(w, "  对应账户：未映射\n")
		}
		fmt.Fprintf(w, "  基金 %d 只，市值 %s\n", g.FundCount, moneyString(g.MarketValueCents, g.Currency))
		if g.Diff != nil {
			fmt.Fprintf(w, "  对比最近快照 %s：\n", g.PreviousAsOf)
			renderDiff(w, g.Diff)
		}
	}
	fmt.Fprintf(w, "\n文件总计：%s\n", moneyString(p.FileTotalCents, p.Total.Currency))
	fmt.Fprintf(w, "已映射小计：%s\n", moneyString(p.MappedTotalCents, p.Total.Currency))
	fmt.Fprintf(w, "已忽略：%s\n", moneyString(p.IgnoredTotalCents, p.Total.Currency))
	fmt.Fprintf(w, "未映射：%s\n", moneyString(p.UnmappedTotalCents, p.Total.Currency))
	fmt.Fprintf(w, "共 %d 只基金（%d 条持仓）\n", p.Total.FundCount, p.Total.HoldingCount)

	if len(p.Unmapped) > 0 {
		fmt.Fprintln(w, "\n⚠ 未映射的交易账户")
		for _, u := range p.Unmapped {
			fmt.Fprintf(w, "- %s：%d 只，市值 %s\n",
				u.Name, u.FundCount, moneyString(u.MarketValueCents, u.Currency))
		}
	}
	if len(p.Warnings) > 0 {
		fmt.Fprintln(w, "\n校验告警")
		for _, warning := range p.Warnings {
			fmt.Fprintf(w, "- [%s/%s] %s", warning.Code, warning.Severity, warning.Message)
			if warning.Severity == SeverityBlocksWrite {
				fmt.Fprint(w, "（写入时将被阻止）")
			}
			fmt.Fprintln(w)
		}
	}
}

func renderDiff(w io.Writer, diff *domain.Diff) {
	for _, item := range diff.Added {
		fmt.Fprintf(w, "    新增：%s（%s）\n", item.Name, moneyString(item.MarketValueCents, "CNY"))
	}
	for _, item := range diff.Removed {
		fmt.Fprintf(w, "    消失：%s（%s）\n", item.Name, moneyString(item.MarketValueCents, "CNY"))
	}
	for _, item := range diff.Cleared {
		fmt.Fprintf(w, "    清仓：%s（%s → %s）\n", item.Name, item.OldQuantity, item.NewQuantity)
	}
	for _, item := range diff.QuantityChanged {
		fmt.Fprintf(w, "    份额变化：%s %s → %s\n", item.Name, item.OldQuantity, item.NewQuantity)
	}
	for _, item := range diff.ValueChanged {
		fmt.Fprintf(w, "    市值变化：%s %s → %s\n", item.Name,
			moneyString(item.OldValueCents, "CNY"), moneyString(item.NewValueCents, "CNY"))
	}
	if len(diff.Added)+len(diff.Removed)+len(diff.Cleared)+len(diff.QuantityChanged)+len(diff.ValueChanged) == 0 {
		fmt.Fprintln(w, "    无持仓变化")
	}
}

func moneyString(cents int64, currency string) string {
	prefix := ""
	switch {
	case currency == "CNY":
		prefix = "¥"
	case currency != "":
		prefix = currency + " "
	}
	return prefix + yuanString(cents)
}
