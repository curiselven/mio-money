package efund

import (
	"archive/zip"
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

func zipWithSheet(t *testing.T, sheet string) []byte {
	t.Helper()
	file := filepath.Join(t.TempDir(), "sheet.xlsx")
	out, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(out)
	files := map[string]string{
		"xl/workbook.xml":            `<workbook><sheets><sheet name="持有信息" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   sheet,
	}
	for name, body := range files {
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
