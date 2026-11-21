package main

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/curiselven/mio-money/internal/domain"
	"github.com/curiselven/mio-money/internal/importer/efund"
	"github.com/curiselven/mio-money/internal/store"
)

func TestImportPreviewRejectsOldSchemaAndInvalidTarget(t *testing.T) {
	book := writeEfundPreviewBook(t)

	t.Run("old schema", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "old.db")
		db, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range []string{
			`CREATE TABLE schema_version (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`,
			`CREATE TABLE accounts (id TEXT PRIMARY KEY, name TEXT NOT NULL)`,
			`INSERT INTO schema_version(version, applied_at) VALUES (7, '2026-01-01T00:00:00Z')`,
		} {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatal(err)
			}
		}
		db.Close()

		var out strings.Builder
		err = runImport([]string{"efund", "--preview", book, "--db", dbPath}, &out)
		if err == nil || !strings.Contains(err.Error(), "桌面") {
			t.Fatalf("old schema error = %v", err)
		}
	})

	t.Run("archived target", func(t *testing.T) {
		dir := t.TempDir()
		dbPath := filepath.Join(dir, "mio.db")
		st, err := store.Open(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		svc := domain.NewService(st, 0.2)
		archived, err := svc.UpsertAccount(context.Background(), domain.UpsertAccountInput{
			Name: "示例归档账户", Type: domain.AccountFund, Archived: boolPointer(true),
		})
		if err != nil {
			t.Fatal(err)
		}
		st.Close()

		mapPath := filepath.Join(dir, "efund-map.json")
		mapping := &efund.Mapping{Mappings: []efund.MappingEntry{{
			SalesOrg: "示例基金销售有限公司", TradeAccount: "T0000001",
			AccountID: archived.ID, Account: archived.Name,
		}}}
		if err := efund.WriteMapping(mapPath, mapping); err != nil {
			t.Fatal(err)
		}
		var out strings.Builder
		if err := runImport([]string{"efund", "--preview", book, "--map", mapPath, "--db", dbPath}, &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "映射无效") || !strings.Contains(out.String(), "已归档") {
			t.Fatalf("preview did not reject archived target:\n%s", out.String())
		}
	})
}

func boolPointer(value bool) *bool { return &value }

func writeEfundPreviewBook(t *testing.T) string {
	t.Helper()
	header := []string{
		"序号", "基金代码", "基金名称", "份额类别", "基金管理人", "基金账户",
		"销售机构", "交易账户", "持有份额", "份额日期", "基金净值", "净值日期",
		"资产情况（结算币种）", "结算币种", "分红方式",
	}
	holding := []string{
		"1.0", "000216", "示例成长混合", "A", "示例基金管理有限公司", "F000001",
		"示例基金销售有限公司", "T0000001", "100", "2026/11/17", "1.0", "2026/11/17",
		"100.00", "人民币", "现金分红",
	}
	footer := []string{"总记录数： 1 笔", "数据来源：基金E账户App", "打印时间：2026年11月20日 22:21:38"}

	var sheet strings.Builder
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for rowIndex, row := range [][]string{header, holding, footer} {
		fmt.Fprintf(&sheet, `<row r="%d">`, rowIndex+1)
		for column, value := range row {
			ref := previewColumn(column) + fmt.Sprint(rowIndex+1)
			if rowIndex == 1 && column == 0 {
				fmt.Fprintf(&sheet, `<c r="%s" t="n"><v>%s</v></c>`, ref, value)
				continue
			}
			fmt.Fprintf(&sheet, `<c r="%s" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`,
				ref, previewEscape(value))
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)

	var buffer bytes.Buffer
	zw := zip.NewWriter(&buffer)
	files := map[string]string{
		"xl/workbook.xml":            `<workbook><sheets><sheet name="持有信息" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   sheet.String(),
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
	path := filepath.Join(t.TempDir(), "fixture.xlsx")
	if err := os.WriteFile(path, buffer.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func previewColumn(index int) string {
	index++
	var out []byte
	for index > 0 {
		index--
		out = append([]byte{byte('A' + index%26)}, out...)
		index /= 26
	}
	return string(out)
}

func previewEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return replacer.Replace(value)
}
