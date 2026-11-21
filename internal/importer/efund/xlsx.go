package efund

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

type sheetRow struct {
	number int
	cells  []string
}

type xlsxCell struct {
	Ref    string      `xml:"r,attr"`
	Type   string      `xml:"t,attr"`
	Value  string      `xml:"v"`
	Inline *xlsxInline `xml:"is"`
}

type xlsxInline struct {
	Text string    `xml:"t"`
	Runs []xlsxRun `xml:"r"`
}

type xlsxRun struct {
	Text string `xml:"t"`
}

const (
	maxSheetRows = 100000
	maxColumns   = 16384
)

func readSheet(data []byte) ([]sheetRow, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("打开 xlsx 失败（需要已解密的 .xlsx）: %w", err)
	}
	if len(zr.File) > maxZipEntries {
		return nil, fmt.Errorf("xlsx 条目过多：%d（上限 %d）", len(zr.File), maxZipEntries)
	}

	sheetPath := workbookSheetPath(zr)
	shared, err := readSharedStrings(zr)
	if err != nil {
		return nil, err
	}
	sheetData, err := readZipFile(zr, sheetPath)
	if err != nil {
		return nil, fmt.Errorf("read sheet %s: %w", sheetPath, err)
	}
	var ws struct {
		Rows []struct {
			Number int        `xml:"r,attr"`
			Cells  []xlsxCell `xml:"c"`
		} `xml:"sheetData>row"`
	}
	if err := xml.Unmarshal(sheetData, &ws); err != nil {
		return nil, fmt.Errorf("parse sheet: %w", err)
	}
	if len(ws.Rows) > maxSheetRows {
		return nil, fmt.Errorf("sheet 行数过多：%d（上限 %d）", len(ws.Rows), maxSheetRows)
	}

	rows := make([]sheetRow, 0, len(ws.Rows))
	lastNumber := 0
	for _, raw := range ws.Rows {
		number := raw.Number
		if number <= lastNumber {
			number = lastNumber + 1
		}
		if number > maxSheetRows {
			return nil, fmt.Errorf("sheet 行号超过 %d", maxSheetRows)
		}
		lastNumber = number
		var cells []string
		lastCol := -1
		for _, c := range raw.Cells {
			col, err := cellColumn(c.Ref)
			if err != nil {
				return nil, err
			}
			if c.Ref == "" {
				col = lastCol + 1
			}
			if col > lastCol {
				cells = append(cells, make([]string, col-lastCol)...)
				lastCol = col
			}
			value, err := cellText(c, shared)
			if err != nil {
				return nil, err
			}
			cells[col] = value
		}
		rows = append(rows, sheetRow{number: number, cells: cells})
	}
	return rows, nil
}

func cellText(c xlsxCell, shared []string) (string, error) {
	switch c.Type {
	case "s":
		idx, err := strconv.Atoi(strings.TrimSpace(c.Value))
		if err != nil || idx < 0 || idx >= len(shared) {
			return "", fmt.Errorf("xlsx: bad shared string index %q", c.Value)
		}
		return shared[idx], nil
	case "inlineStr":
		if c.Inline == nil {
			return "", nil
		}
		var b strings.Builder
		b.WriteString(c.Inline.Text)
		for _, run := range c.Inline.Runs {
			b.WriteString(run.Text)
		}
		return b.String(), nil
	default:
		return c.Value, nil
	}
}

func readSharedStrings(zr *zip.Reader) ([]string, error) {
	data, err := readZipFile(zr, "xl/sharedStrings.xml")
	if err != nil {
		return nil, nil
	}
	var sst struct {
		Items []struct {
			Text string    `xml:"t"`
			Runs []xlsxRun `xml:"r"`
		} `xml:"si"`
	}
	if err := xml.Unmarshal(data, &sst); err != nil {
		return nil, fmt.Errorf("parse sharedStrings: %w", err)
	}
	out := make([]string, len(sst.Items))
	for i, item := range sst.Items {
		var b strings.Builder
		b.WriteString(item.Text)
		for _, run := range item.Runs {
			b.WriteString(run.Text)
		}
		out[i] = b.String()
	}
	return out, nil
}

func workbookSheetPath(zr *zip.Reader) string {
	const fallback = "xl/worksheets/sheet1.xml"
	data, err := readZipFile(zr, "xl/workbook.xml")
	if err != nil {
		return fallback
	}
	var wb struct {
		Sheets []struct {
			RID string `xml:"id,attr"`
		} `xml:"sheets>sheet"`
	}
	if xml.Unmarshal(data, &wb) != nil || len(wb.Sheets) == 0 || wb.Sheets[0].RID == "" {
		return fallback
	}
	relData, err := readZipFile(zr, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return fallback
	}
	var rels struct {
		Items []struct {
			ID     string `xml:"Id,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if xml.Unmarshal(relData, &rels) != nil {
		return fallback
	}
	for _, rel := range rels.Items {
		if rel.ID != wb.Sheets[0].RID || rel.Target == "" {
			continue
		}
		target := strings.TrimPrefix(rel.Target, "/")
		if strings.HasPrefix(target, "xl/") {
			return target
		}
		return path.Join("xl", target)
	}
	return fallback
}

func readZipFile(zr *zip.Reader, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		data, err := io.ReadAll(io.LimitReader(rc, maxXLSXBytes+1))
		if err != nil {
			return nil, err
		}
		if len(data) > maxXLSXBytes {
			return nil, fmt.Errorf("xlsx 文件 %s 解压后超过 %d MB", name, maxXLSXBytes>>20)
		}
		return data, nil
	}
	return nil, fmt.Errorf("xlsx: %s not found", name)
}

func cellColumn(ref string) (int, error) {
	if ref == "" {
		return 0, nil
	}
	col := 0
	letters := 0
	for _, r := range ref {
		if r >= 'A' && r <= 'Z' {
			col = col*26 + int(r-'A'+1)
			letters++
			continue
		}
		break
	}
	if letters == 0 {
		return 0, fmt.Errorf("xlsx: bad cell reference %q", ref)
	}
	if col > maxColumns {
		return 0, fmt.Errorf("xlsx: 单元格列号超过 XFD: %q", ref)
	}
	return col - 1, nil
}
