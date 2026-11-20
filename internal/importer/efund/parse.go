package efund

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

var (
	decimalPattern = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
	intPattern     = regexp.MustCompile(`^\d+$`)
	seqPattern     = regexp.MustCompile(`^-?\d+(\.\d+)?([eE][+-]?\d+)?$`)
	totalPattern   = regexp.MustCompile(`总记录数\s*[:：]\s*(\d+)`)
	printPattern   = regexp.MustCompile(`打印时间\s*[:：]\s*(.+)`)
)

var requiredHeaders = []string{
	"序号", "基金代码", "基金名称", "份额类别", "基金管理人", "基金账户",
	"销售机构", "交易账户", "持有份额", "份额日期", "基金净值", "净值日期",
	"结算币种", "分红方式",
}

func parseXLSX(file string, data []byte) (*Result, error) {
	rows, err := readSheet(data)
	if err != nil {
		return nil, err
	}
	headerAt, headers, err := findHeader(rows)
	if err != nil {
		return nil, err
	}
	assetCol, ok := headers["资产情况（结算币种）"]
	if !ok {
		assetCol, ok = headers["资产情况(结算币种)"]
	}
	if !ok {
		return nil, errors.New("未找到表头「资产情况（结算币种）」")
	}

	res := &Result{
		File:     file,
		Holdings: []Holding{},
		Warnings: []Warning{},
	}
	navMismatch := map[string]bool{}
	shareDates := map[string]bool{}
	tailAt := len(rows)

	for i := headerAt + 1; i < len(rows); i++ {
		row := rows[i]
		seqText := strings.TrimSpace(cellAt(row.cells, headers["序号"]))
		seq, ok := parseSeq(seqText)
		if !ok {
			tailAt = i
			break
		}
		h, err := parseHolding(row.number, row.cells, headers, assetCol)
		if err != nil {
			return nil, err
		}
		h.Seq = seq
		res.Holdings = append(res.Holdings, h)
		shareDates[h.ShareDate] = true
		if h.NAVDate != h.ShareDate {
			navMismatch[h.FundCode] = true
		}
		if h.CurrencyCode != "CNY" {
			res.Warnings = append(res.Warnings, Warning{
				Code: "currency", Severity: SeverityWarning, Row: row.number,
				Message: fmt.Sprintf("第 %d 行结算币种「%s」不是人民币，暂不换算", row.number, h.Currency),
			})
		}
		expected, err := expectedCents(h.Quantity, h.NAV)
		if err != nil {
			return nil, fmt.Errorf("第 %d 行: %w", row.number, err)
		}
		if delta := expected - h.MarketValueCents; abs64(delta) > 1 {
			res.Warnings = append(res.Warnings, Warning{
				Code: "value_mismatch", Severity: SeverityWarning, Row: row.number,
				Message: fmt.Sprintf("第 %d 行「%s」份额×净值约 %s，与金额 %s 相差 %s",
					row.number, h.FundName, yuanString(expected), yuanString(h.MarketValueCents), yuanString(delta)),
			})
		}
	}

	res.Meta.ActualRows = len(res.Holdings)
	if len(shareDates) > 1 {
		res.Warnings = append(res.Warnings, Warning{
			Code:     "share_date_mismatch",
			Severity: SeverityWarning,
			Message:  "数据行的份额日期不一致，预览按最新日期归组",
		})
	}
	for date := range shareDates {
		if date > res.Meta.ShareDate {
			res.Meta.ShareDate = date
		}
	}
	if res.Meta.ShareDate == "" && len(res.Holdings) > 0 {
		return nil, errors.New("未找到可用的份额日期")
	}

	total, printTime := readFooter(rows[tailAt:])
	if total >= 0 {
		res.Meta.TotalRecords = &total
		res.Meta.RecordsMatch = total == len(res.Holdings)
		if !res.Meta.RecordsMatch {
			res.Warnings = append(res.Warnings, Warning{
				Code:     "record_count_mismatch",
				Severity: SeverityBlocksWrite,
				Message:  fmt.Sprintf("文件写总记录数 %d，实际解析 %d 行", total, len(res.Holdings)),
			})
		}
	} else {
		res.Warnings = append(res.Warnings, Warning{
			Code:     "record_count_missing",
			Severity: SeverityBlocksWrite,
			Message:  "尾部未找到「总记录数」，已按实际解析行数预览",
		})
	}
	res.Meta.PrintTime = printTime
	res.Meta.NAVDateMismatchFunds = len(navMismatch)
	if len(navMismatch) > 0 {
		res.Warnings = append(res.Warnings, Warning{
			Code:     "nav_date_mismatch",
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("%d 只基金的净值日期与份额日期不一致（QDII 常见），已按份额日期归组", len(navMismatch)),
		})
	}
	return res, nil
}

func findHeader(rows []sheetRow) (int, map[string]int, error) {
	for i, row := range rows {
		headers := headerMap(row.cells)
		if _, ok := headers["序号"]; !ok {
			continue
		}
		if _, ok := headers["基金代码"]; !ok {
			continue
		}
		var missing []string
		for _, name := range requiredHeaders {
			if _, ok := headers[name]; !ok {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			return 0, nil, fmt.Errorf("表头缺少列：%s", strings.Join(missing, "、"))
		}
		return i, headers, nil
	}
	return 0, nil, errors.New("未找到表头（需要「序号」「基金代码」列）")
}

func headerMap(cells []string) map[string]int {
	out := make(map[string]int, len(cells))
	for i, value := range cells {
		name := normalizeHeader(value)
		if name == "" {
			continue
		}
		if _, exists := out[name]; !exists {
			out[name] = i
		}
	}
	return out
}

func normalizeHeader(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, value)
}

func parseHolding(rowNumber int, cells []string, headers map[string]int, assetCol int) (Holding, error) {
	h := Holding{
		FundCode:       strings.TrimSpace(cellAt(cells, headers["基金代码"])),
		FundName:       strings.TrimSpace(cellAt(cells, headers["基金名称"])),
		ShareClass:     strings.TrimSpace(cellAt(cells, headers["份额类别"])),
		Manager:        strings.TrimSpace(cellAt(cells, headers["基金管理人"])),
		FundAccount:    strings.TrimSpace(cellAt(cells, headers["基金账户"])),
		SalesOrg:       strings.TrimSpace(cellAt(cells, headers["销售机构"])),
		TradeAccount:   strings.TrimSpace(cellAt(cells, headers["交易账户"])),
		Quantity:       strings.TrimSpace(cellAt(cells, headers["持有份额"])),
		NAV:            strings.TrimSpace(cellAt(cells, headers["基金净值"])),
		Currency:       strings.TrimSpace(cellAt(cells, headers["结算币种"])),
		DividendMethod: strings.TrimSpace(cellAt(cells, headers["分红方式"])),
	}
	if h.FundCode == "" || h.FundName == "" {
		return Holding{}, fmt.Errorf("第 %d 行缺少基金代码或基金名称", rowNumber)
	}
	if !decimalPattern.MatchString(h.Quantity) {
		return Holding{}, fmt.Errorf("第 %d 行份额 %q 不是普通十进制", rowNumber, h.Quantity)
	}
	if !decimalPattern.MatchString(h.NAV) {
		return Holding{}, fmt.Errorf("第 %d 行净值 %q 不是普通十进制", rowNumber, h.NAV)
	}
	amount, err := parseCents(cellAt(cells, assetCol))
	if err != nil {
		return Holding{}, fmt.Errorf("第 %d 行金额: %w", rowNumber, err)
	}
	h.MarketValueCents = amount
	if h.ShareDate, err = parseDate(cellAt(cells, headers["份额日期"])); err != nil {
		return Holding{}, fmt.Errorf("第 %d 行份额日期: %w", rowNumber, err)
	}
	if h.NAVDate, err = parseDate(cellAt(cells, headers["净值日期"])); err != nil {
		return Holding{}, fmt.Errorf("第 %d 行净值日期: %w", rowNumber, err)
	}
	h.CurrencyCode = currencyCode(h.Currency)
	return h, nil
}

func readFooter(rows []sheetRow) (int, string) {
	total := -1
	printTime := ""
	for _, row := range rows {
		for _, cell := range row.cells {
			if total < 0 {
				if m := totalPattern.FindStringSubmatch(cell); m != nil {
					if n, err := strconv.Atoi(m[1]); err == nil {
						total = n
					}
				}
			}
			if printTime == "" {
				if m := printPattern.FindStringSubmatch(cell); m != nil {
					printTime = strings.TrimSpace(m[1])
				}
			}
		}
	}
	return total, printTime
}

func cellAt(cells []string, index int) string {
	if index < 0 || index >= len(cells) {
		return ""
	}
	return cells[index]
}

// parseSeq accepts Excel numeric cells such as 1, 1.0 and 1E0, but not
// fractions or arbitrary strings. The sequence is not money, so this only
// decides where the data block ends.
func parseSeq(value string) (int, bool) {
	s := strings.TrimSpace(value)
	if !seqPattern.MatchString(s) {
		return 0, false
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok || !r.IsInt() || r.Sign() < 0 || !r.Num().IsInt64() {
		return 0, false
	}
	n := r.Num().Int64()
	if n > int64(^uint(0)>>1) {
		return 0, false
	}
	return int(n), true
}

func parseDate(value string) (string, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006/1/2", "2006-1-2"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t.Format("2006-01-02"), nil
		}
	}
	return "", fmt.Errorf("%q 不是 YYYY/MM/DD 日期", value)
}

// parseCents parses a decimal amount without floating point. Extra decimals
// are rounded half away from zero to the nearest cent.
func parseCents(value string) (int64, error) {
	s := strings.ReplaceAll(strings.TrimSpace(value), ",", "")
	if s == "" {
		return 0, errors.New("金额为空")
	}
	negative := false
	switch s[0] {
	case '-':
		negative = true
		s = s[1:]
	case '+':
		s = s[1:]
	}
	intPart, fracPart, _ := strings.Cut(s, ".")
	if intPart == "" || !intPattern.MatchString(intPart) || (fracPart != "" && !intPattern.MatchString(fracPart)) {
		return 0, fmt.Errorf("%q 不是有效金额", value)
	}
	frac := fracPart
	if len(frac) > 2 {
		roundUp := frac[2] >= '5'
		frac = frac[:2]
		cents, err := strconv.ParseInt(intPart+frac, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%q 超出范围", value)
		}
		if roundUp {
			cents++
		}
		if negative {
			cents = -cents
		}
		return cents, nil
	}
	for len(frac) < 2 {
		frac += "0"
	}
	cents, err := strconv.ParseInt(intPart+frac, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%q 超出范围", value)
	}
	if negative {
		cents = -cents
	}
	return cents, nil
}

func expectedCents(quantity, nav string) (int64, error) {
	q, ok := new(big.Rat).SetString(quantity)
	if !ok {
		return 0, fmt.Errorf("份额 %q 无效", quantity)
	}
	n, ok := new(big.Rat).SetString(nav)
	if !ok {
		return 0, fmt.Errorf("净值 %q 无效", nav)
	}
	return parseCents(new(big.Rat).Mul(q, n).FloatString(2))
}

func currencyCode(value string) string {
	switch strings.TrimSpace(value) {
	case "人民币", "CNY":
		return "CNY"
	case "美元", "USD":
		return "USD"
	case "港币", "港元", "HKD":
		return "HKD"
	case "欧元", "EUR":
		return "EUR"
	default:
		return strings.TrimSpace(value)
	}
}

func yuanString(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
