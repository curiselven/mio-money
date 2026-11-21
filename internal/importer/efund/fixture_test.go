package efund

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type fcell struct {
	text    string
	numeric bool
	shared  bool
	skip    bool
}

func s(v string) fcell      { return fcell{text: v} }
func shared(v string) fcell { return fcell{text: v, shared: true} }
func num(v string) fcell    { return fcell{text: v, numeric: true} }
func gap() fcell            { return fcell{skip: true} }

func writeBook(t *testing.T, rows [][]fcell) string {
	t.Helper()
	sharedIndex := map[string]int{}
	var sharedList []string
	for _, row := range rows {
		for _, c := range row {
			if !c.shared {
				continue
			}
			if _, ok := sharedIndex[c.text]; !ok {
				sharedIndex[c.text] = len(sharedList)
				sharedList = append(sharedList, c.text)
			}
		}
	}

	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for i, row := range rows {
		if len(row) == 0 {
			fmt.Fprintf(&sheet, `<row r="%d" ht="5" customHeight="1"/>`, i+1)
			continue
		}
		fmt.Fprintf(&sheet, `<row r="%d">`, i+1)
		for j, c := range row {
			if c.skip {
				continue
			}
			ref := columnName(j) + strconv.Itoa(i+1)
			switch {
			case c.numeric:
				fmt.Fprintf(&sheet, `<c r="%s" t="n" s="15"><v>%s</v></c>`, ref, escapeXML(c.text))
			case c.shared:
				fmt.Fprintf(&sheet, `<c r="%s" t="s"><v>%d</v></c>`, ref, sharedIndex[c.text])
			default:
				fmt.Fprintf(&sheet, `<c r="%s" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`,
					ref, escapeXML(c.text))
			}
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)

	var sst strings.Builder
	sst.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	fmt.Fprintf(&sst, `<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="%d" uniqueCount="%d">`,
		len(sharedList), len(sharedList))
	for _, value := range sharedList {
		fmt.Fprintf(&sst, `<si><t xml:space="preserve">%s</t></si>`, escapeXML(value))
	}
	sst.WriteString(`</sst>`)

	files := map[string]string{
		"xl/workbook.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ` +
			`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
			`<sheets><sheet name="持有信息" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" ` +
			`Target="worksheets/holdings.xml"/></Relationships>`,
		"xl/sharedStrings.xml":       sst.String(),
		"xl/worksheets/holdings.xml": sheet.String(),
	}
	file := filepath.Join(t.TempDir(), "efund-fixture.xlsx")
	out, err := os.Create(file)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}
	zw := zip.NewWriter(out)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create: %v", err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatalf("zip write: %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	if err := out.Close(); err != nil {
		t.Fatalf("close fixture: %v", err)
	}
	return file
}

func columnName(index int) string {
	index++
	var out []byte
	for index > 0 {
		index--
		out = append([]byte{byte('A' + index%26)}, out...)
		index /= 26
	}
	return string(out)
}

func escapeXML(value string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}

func headerRow() []fcell {
	return []fcell{
		s("序号"), s("基金代码"), s("基金名称"), s("份额类别"), s("基金管理人"),
		s("基金账户"), s("销售机构"), s("交易账户"), s("持有份额"), s("份额日期"),
		s("基金净值"), s("净值日期"), s("资产情况\n（结算币种）"), s("结算币种"), s("分红方式"),
	}
}

func personalRows() [][]fcell {
	return [][]fcell{
		{shared("基金E账户App投资者公募基金持有信息")},
		{s("姓名"), s("证件类型"), s("证件号码")},
		{s("示例姓名"), s("居民身份证"), s("000000********0000")},
		{},
	}
}

func footerRow(total int) []fcell {
	return []fcell{
		s(fmt.Sprintf("总记录数： %d 笔", total)),
		s("数据来源：基金E账户App"),
		s("打印时间：2026年11月20日 22:21:38"),
	}
}

func holdingRow(seq int, code, name, salesOrg, tradeAccount, quantity, date, nav, navDate, amount string) []fcell {
	return []fcell{
		num(fmt.Sprintf("%d.0", seq)), s(code), s(name), s("A"), s("示例基金管理有限公司"),
		s("F000001"), s(salesOrg), s(tradeAccount), s(quantity), s(date),
		s(nav), s(navDate), s(amount), s("人民币"), s("现金分红"),
	}
}
