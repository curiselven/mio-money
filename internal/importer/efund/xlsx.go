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
	cells  map[int]string
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
	maxSheetRows     = 100000
	maxColumns       = 16384
	maxSheetCells    = 100000
	maxCellTextBytes = 4096
	maxSharedStrings = 100000
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
	return parseSheet(sheetData, shared)
}

func parseSheet(data []byte, shared []string) ([]sheetRow, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	rows := []sheetRow{}
	lastNumber := 0
	totalCells := 0
	var current *sheetRow
	lastCol := -1
	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse sheet: %w", err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			switch {
			case t.Name.Local == "row" && current == nil:
				if len(rows) >= maxSheetRows {
					return nil, fmt.Errorf("sheet 行数过多：%d（上限 %d）", len(rows)+1, maxSheetRows)
				}
				number := lastNumber + 1
				for _, attr := range t.Attr {
					if attr.Name.Local == "r" {
						number, err = parseRowNumber(attr.Value, number)
						if err != nil {
							return nil, err
						}
						break
					}
				}
				if number <= lastNumber {
					number = lastNumber + 1
				}
				if number > maxSheetRows {
					return nil, fmt.Errorf("sheet 行号超过 %d", maxSheetRows)
				}
				lastNumber = number
				current = &sheetRow{number: number, cells: map[int]string{}}
				lastCol = -1
			case t.Name.Local == "c" && current != nil:
				totalCells++
				if totalCells > maxSheetCells {
					return nil, fmt.Errorf("sheet 单元格过多：%d（上限 %d）", totalCells, maxSheetCells)
				}
				var cell xlsxCell
				if err := dec.DecodeElement(&cell, &t); err != nil {
					return nil, fmt.Errorf("parse sheet: %w", err)
				}
				col, err := cellColumn(cell.Ref)
				if err != nil {
					return nil, err
				}
				if cell.Ref == "" {
					col = lastCol + 1
				}
				lastCol = col
				value, err := cellText(cell, shared)
				if err != nil {
					return nil, err
				}
				current.cells[col] = value
			}
		case xml.EndElement:
			if t.Name.Local == "row" && current != nil {
				rows = append(rows, *current)
				current = nil
			}
		}
	}
	return rows, nil
}

func readSharedStrings(zr *zip.Reader) ([]string, error) {
	data, err := readZipFile(zr, "xl/sharedStrings.xml")
	if err != nil {
		return nil, nil
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	out := []string{}
	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse sharedStrings: %w", err)
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "si" {
			continue
		}
		if len(out) >= maxSharedStrings {
			return nil, fmt.Errorf("sharedStrings 条目过多：%d（上限 %d）", len(out)+1, maxSharedStrings)
		}
		var item struct {
			Text string    `xml:"t"`
			Runs []xlsxRun `xml:"r"`
		}
		if err := dec.DecodeElement(&item, &start); err != nil {
			return nil, fmt.Errorf("parse sharedStrings: %w", err)
		}
		var b strings.Builder
		b.WriteString(item.Text)
		for _, run := range item.Runs {
			b.WriteString(run.Text)
		}
		value, err := cellTextValue(b.String())
		if err != nil {
			return nil, fmt.Errorf("sharedStrings 第 %d 条：%w", len(out)+1, err)
		}
		out = append(out, value)
	}
	return out, nil
}

func parseRowNumber(value string, fallback int) (int, error) {
	if value == "" {
		return fallback, nil
	}
	if strings.ContainsAny(value, ".eE+-") {
		return 0, fmt.Errorf("xlsx: 行号不支持科学计数或小数：%q", value)
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("xlsx: 行号无效或过大：%q", value)
	}
	if number <= 0 {
		return 0, fmt.Errorf("xlsx: 行号必须为正整数：%q", value)
	}
	return number, nil
}

func cellText(c xlsxCell, shared []string) (string, error) {
	var value string
	switch c.Type {
	case "s":
		idx, err := strconv.Atoi(strings.TrimSpace(c.Value))
		if err != nil || idx < 0 || idx >= len(shared) {
			return "", fmt.Errorf("xlsx: bad shared string index %q", c.Value)
		}
		value = shared[idx]
	case "inlineStr":
		if c.Inline == nil {
			return "", nil
		}
		var b strings.Builder
		b.WriteString(c.Inline.Text)
		for _, run := range c.Inline.Runs {
			b.WriteString(run.Text)
		}
		value = b.String()
	default:
		value = c.Value
	}
	return cellTextValue(value)
}

func cellTextValue(value string) (string, error) {
	if len(value) > maxCellTextBytes {
		return "", fmt.Errorf("单元格文本过长：%d 字节（上限 %d）", len(value), maxCellTextBytes)
	}
	return value, nil
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
	if suffix := ref[letters:]; suffix != "" {
		for _, r := range suffix {
			if r < '0' || r > '9' {
				return 0, fmt.Errorf("xlsx: 单元格行号无效（不支持科学计数）：%q", ref)
			}
		}
		row, err := strconv.Atoi(suffix)
		if err != nil || row <= 0 {
			return 0, fmt.Errorf("xlsx: 单元格行号无效或过大：%q", ref)
		}
		if row > maxSheetRows {
			return 0, fmt.Errorf("xlsx: 单元格行号超过 %d：%q", maxSheetRows, ref)
		}
	}
	if col > maxColumns {
		return 0, fmt.Errorf("xlsx: 单元格列号超过 XFD: %q", ref)
	}
	return col - 1, nil
}
