package efund

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	salesA = "示例基金销售有限公司"
	salesB = "另一示例基金销售有限公司"
)

func sampleBook(t *testing.T) string {
	t.Helper()
	rows := append(personalRows(), headerRow())
	rows = append(rows,
		holdingRow(1, "000216", "示例成长混合", salesA, "T0000001", "7886.73", "2026/11/17", "0.7609", "2026/11/17", "6001.01"),
		holdingRow(2, "000216", "示例成长混合", salesA, "T0000002", "1000", "2026/11/17", "0.7609", "2026/11/17", "760.90"),
		holdingRow(3, "000999", "示例全球精选", salesA, "T0000001", "100", "2026/11/17", "1.2345", "2026/11/16", "123.45"),
		footerRow(3),
	)
	return writeBook(t, rows)
}

func TestParseHoldingSheet(t *testing.T) {
	parsed, err := Parse(sampleBook(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parsed.Meta.ShareDate != "2026-11-17" || parsed.Meta.PrintTime != "2026年11月20日 22:21:38" {
		t.Fatalf("meta = %+v", parsed.Meta)
	}
	if parsed.Meta.TotalRecords == nil || *parsed.Meta.TotalRecords != 3 || !parsed.Meta.RecordsMatch {
		t.Fatalf("record count meta = %+v", parsed.Meta)
	}
	if parsed.Meta.ActualRows != 3 || len(parsed.Holdings) != 3 {
		t.Fatalf("holdings = %+v", parsed.Holdings)
	}
	first := parsed.Holdings[0]
	if first.FundCode != "000216" || first.Quantity != "7886.73" || first.MarketValueCents != 600_101 {
		t.Fatalf("first holding = %+v", first)
	}
	if parsed.Holdings[1].TradeAccount != "T0000002" {
		t.Fatalf("second holding = %+v", parsed.Holdings[1])
	}
	if parsed.Holdings[2].NAVDate != "2026-11-16" || parsed.Meta.NAVDateMismatchFunds != 1 {
		t.Fatalf("QDII row = %+v, meta = %+v", parsed.Holdings[2], parsed.Meta)
	}

	preview, err := Build(t.Context(), nil, parsed, &Mapping{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(preview.Groups) != 2 {
		t.Fatalf("groups = %+v", preview.Groups)
	}
	if preview.Groups[0].FundCount != 2 || preview.Groups[1].FundCount != 1 {
		t.Fatalf("same fund in two trade accounts must not merge: %+v", preview.Groups)
	}
	if preview.Total.MarketValueCents != 688_536 || preview.Total.FundCount != 2 || preview.Total.HoldingCount != 3 {
		t.Fatalf("total = %+v", preview.Total)
	}
	if len(preview.Unmapped) != 2 {
		t.Fatalf("unmapped = %+v", preview.Unmapped)
	}

	data, err := json.Marshal(preview)
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	for _, secret := range []string{"示例姓名", "000000********0000", "证件号码"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("personal information leaked: %s", secret)
		}
	}
}

func TestParseWarnings(t *testing.T) {
	rows := append(personalRows(), headerRow())
	rows = append(rows,
		holdingRow(1, "000100", "示例一号", salesA, "T0000001", "100", "2026/11/17", "1.0", "2026/11/17", "100.01"),
		holdingRow(2, "000101", "示例二号", salesA, "T0000001", "100", "2026/11/17", "1.0", "2026/11/17", "100.02"),
		holdingRow(3, "000102", "示例美元债", salesA, "T0000001", "100", "2026/11/17", "1.0", "2026/11/17", "100.00"),
	)
	rows[7][13] = s("美元")
	rows = append(rows, footerRow(5))

	parsed, err := Parse(writeBook(t, rows))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	codes := map[string]int{}
	severities := map[string]Severity{}
	for _, warning := range parsed.Warnings {
		codes[warning.Code]++
		severities[warning.Code] = warning.Severity
	}
	if codes["value_mismatch"] != 1 || codes["record_count_mismatch"] != 1 || codes["currency"] != 1 {
		t.Fatalf("warnings = %+v", parsed.Warnings)
	}
	if severities["value_mismatch"] != SeverityWarning ||
		severities["record_count_mismatch"] != SeverityBlocksWrite ||
		severities["currency"] != SeverityWarning {
		t.Fatalf("severities = %+v", severities)
	}
	data, err := json.Marshal(parsed)
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	if !strings.Contains(string(data), `"severity":"blocks_write"`) {
		t.Fatalf("blocks_write severity missing from JSON: %s", data)
	}
	for _, warning := range parsed.Warnings {
		if warning.Code == "value_mismatch" && warning.Row != 7 {
			t.Fatalf("value warning row = %d, want 7", warning.Row)
		}
	}

	var text strings.Builder
	preview, err := Build(t.Context(), nil, parsed, &Mapping{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	Render(&text, preview)
	if !strings.Contains(text.String(), "值") && !strings.Contains(text.String(), "相差") {
		t.Fatalf("value warning missing from render:\n%s", text.String())
	}
	if !strings.Contains(text.String(), "写入时将被阻止") {
		t.Fatalf("blocks_write note missing from render:\n%s", text.String())
	}
}

func TestParseNumericSequenceAndBlankRow(t *testing.T) {
	rows := append(personalRows(), headerRow())
	first := holdingRow(1, "000216", "示例成长混合", salesA, "T0000001", "100", "2026/11/17", "1.0", "2026/11/17", "100.00")
	first[3] = gap()
	first[13] = s("美元")
	rows = append(rows, first, footerRow(1))

	parsed, err := Parse(writeBook(t, rows))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Holdings) != 1 || parsed.Holdings[0].Seq != 1 {
		t.Fatalf("holdings = %+v, want one row with seq 1", parsed.Holdings)
	}
	if parsed.Holdings[0].SalesOrg != salesA || parsed.Holdings[0].TradeAccount != "T0000001" {
		t.Fatalf("missing cell shifted columns: %+v", parsed.Holdings[0])
	}
	var currencyRow int
	for _, warning := range parsed.Warnings {
		if warning.Code == "currency" {
			currencyRow = warning.Row
		}
	}
	if currencyRow != 6 {
		t.Fatalf("warning row = %d, want Excel row 6", currencyRow)
	}
}

func TestParseSeq(t *testing.T) {
	cases := []struct {
		value string
		want  int
		ok    bool
	}{
		{"1", 1, true},
		{"1.0", 1, true},
		{"12.0", 12, true},
		{"1E0", 1, true},
		{"1.5", 0, false},
		{"abc", 0, false},
		{"", 0, false},
	}
	for _, c := range cases {
		got, ok := parseSeq(c.value)
		if ok != c.ok || got != c.want {
			t.Fatalf("parseSeq(%q) = %d, %v; want %d, %v", c.value, got, ok, c.want, c.ok)
		}
	}
}
