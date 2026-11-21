package efund

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCellColumnLimit(t *testing.T) {
	if col, err := cellColumn("XFD1"); err != nil || col != maxColumns-1 {
		t.Fatalf("XFD = %d, err=%v", col, err)
	}
	if _, err := cellColumn("XFE1"); err == nil {
		t.Fatal("XFE column was accepted")
	}
}

func TestOuterZipEntryLimit(t *testing.T) {
	file := filepath.Join(t.TempDir(), "many.zip")
	out, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	for i := 0; i <= maxZipEntries; i++ {
		if _, err := zw.Create("file.txt"); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(file); err == nil || !strings.Contains(err.Error(), "条目过多") {
		t.Fatalf("error = %v", err)
	}
}

func TestOuterZipEntrySizeLimit(t *testing.T) {
	file := filepath.Join(t.TempDir(), "large.zip")
	out, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	entry, err := zw.Create("large.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(make([]byte, maxXLSXBytes+1)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(file); err == nil || !strings.Contains(err.Error(), "超过") {
		t.Fatalf("error = %v", err)
	}
}

func TestSheetRowLimit(t *testing.T) {
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for i := 0; i <= maxSheetRows; i++ {
		sheet.WriteString(`<row/>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	if _, err := readSheet(zipWithSheet(t, sheet.String())); err == nil || !strings.Contains(err.Error(), "行数过多") {
		t.Fatalf("error = %v", err)
	}
}

func TestRawFileSizeLimit(t *testing.T) {
	file := filepath.Join(t.TempDir(), "large.xlsx")
	out, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := out.Truncate(MaxInputBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(file); err == nil || !strings.Contains(err.Error(), "上限 32 MB") {
		t.Fatalf("error = %v", err)
	}
}

func TestSparseWideColumn(t *testing.T) {
	sheet := `<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>` +
		`<row r="1"><c r="XFD1" t="inlineStr"><is><t>宽列</t></is></c></row>` +
		`</sheetData></worksheet>`
	rows, err := readSheet(zipWithSheet(t, sheet))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0].cells) != 1 || rows[0].cells[maxColumns-1] != "宽列" {
		t.Fatalf("sparse row = %+v", rows)
	}
}

func TestSheetCellLimit(t *testing.T) {
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData><row>`)
	sheet.WriteString(strings.Repeat("<c/>", maxSheetCells+1))
	sheet.WriteString(`</row></sheetData></worksheet>`)
	if _, err := readSheet(zipWithSheet(t, sheet.String())); err == nil || !strings.Contains(err.Error(), "单元格过多") {
		t.Fatalf("error = %v", err)
	}
}

func TestSheetEmptyCellLimitAcrossRows(t *testing.T) {
	perRow := 500
	rows := maxSheetCells/perRow + 2
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for i := 1; i <= rows; i++ {
		fmt.Fprintf(&sheet, `<row r="%d">`, i)
		sheet.WriteString(strings.Repeat("<c/>", perRow))
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	if _, err := readSheet(zipWithSheet(t, sheet.String())); err == nil || !strings.Contains(err.Error(), "单元格过多") {
		t.Fatalf("error = %v", err)
	}
}

func TestCellTextLimit(t *testing.T) {
	text := strings.Repeat("a", maxCellTextBytes+1)
	sheet := `<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>` +
		`<row r="1"><c r="A1" t="inlineStr"><is><t>` + text + `</t></is></c></row>` +
		`</sheetData></worksheet>`
	if _, err := readSheet(zipWithSheet(t, sheet)); err == nil || !strings.Contains(err.Error(), "文本过长") {
		t.Fatalf("error = %v", err)
	}
}

func TestRowNumberRejectsScientificAndLarge(t *testing.T) {
	for _, tc := range []struct {
		name string
		row  string
		want string
	}{
		{"scientific", `<row r="1E+03"/>`, "科学计数"},
		{"too-large", `<row r="100001"/>`, "行号超过"},
		{"scientific-cell", `<row r="1"><c r="A1E+03"/></row>`, "科学计数"},
		{"large-cell", `<row r="1"><c r="A100001"/></row>`, "单元格行号超过"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sheet := `<?xml version="1.0" encoding="UTF-8"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>` +
				tc.row + `</sheetData></worksheet>`
			if _, err := readSheet(zipWithSheet(t, sheet)); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func zipWithSheet(t *testing.T, sheet string) []byte {
	t.Helper()
	return zipWithParts(t, map[string]string{
		"xl/workbook.xml":            `<workbook><sheets><sheet name="持有信息" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   sheet,
	})
}

func TestSharedStringsLimit(t *testing.T) {
	parts := map[string]string{
		"xl/workbook.xml":            `<workbook><sheets><sheet name="持有信息" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   `<worksheet><sheetData/></worksheet>`,
		"xl/sharedStrings.xml": `<sst>` +
			strings.Repeat(`<si><t>示例文本</t></si>`, maxSharedStrings+1) +
			`</sst>`,
	}
	if _, err := readSheet(zipWithParts(t, parts)); err == nil || !strings.Contains(err.Error(), "sharedStrings 条目过多") {
		t.Fatalf("error = %v", err)
	}
}

func zipWithParts(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	file := filepath.Join(t.TempDir(), "sheet.xlsx")
	out, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	for name, body := range parts {
		writer, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
