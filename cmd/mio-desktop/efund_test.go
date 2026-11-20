package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	yzip "github.com/yeka/zip"

	"github.com/curiselven/mio-money/internal/domain"
	"github.com/curiselven/mio-money/internal/importer/efund"
	"github.com/curiselven/mio-money/internal/store"
)

const (
	efundSalesA = "示例基金销售有限公司"
	efundSalesB = "另一示例基金销售有限公司"
)

type efundCell struct {
	text    string
	numeric bool
}

func efundStr(value string) efundCell { return efundCell{text: value} }
func efundNum(value string) efundCell { return efundCell{text: value, numeric: true} }

type efundHarness struct {
	svc     *domain.Service
	st      *store.Store
	service *EfundService
	dbPath  string
}

func newEfundHarness(t *testing.T) *efundHarness {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "mio.db")
	st, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := domain.NewService(st, 0.2)
	return &efundHarness{svc: svc, st: st, service: NewEfundService(svc, st, dbPath), dbPath: dbPath}
}

func (h *efundHarness) account(t *testing.T, name string) *domain.Account {
	t.Helper()
	account, err := h.svc.UpsertAccount(context.Background(), domain.UpsertAccountInput{
		Name: name, Type: domain.AccountFund, Currency: "CNY",
	})
	if err != nil {
		t.Fatal(err)
	}
	return account
}

func TestEfundDesktopImportNewAndExisting(t *testing.T) {
	h := newEfundHarness(t)
	existing := h.account(t, "已有账户")
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundHolding(2, "000217", "示例价值混合", efundSalesA, "T0000001", "10", "2.0", "2026/11/17", "20.00", "2026/11/17"),
		efundHolding(3, "000216", "示例成长混合", efundSalesB, "T0000002", "50", "1.0", "2026/11/17", "50.00", "2026/11/17"),
		efundFooter(3),
	})
	encoded := efundBase64(data)

	preview, err := h.service.Preview(encoded, "", "fixture.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if preview.CanImport || len(preview.Groups) != 2 || len(preview.Targets) != 0 {
		t.Fatalf("preview = %+v", preview)
	}
	preview, err = h.service.Remap(preview.Token, []EfundSelection{
		{SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: efundModeExisting, AccountID: existing.ID},
		{SalesOrg: efundSalesB, TradeAccount: "T0000002", Mode: efundModeNew, AccountName: "新基金组合"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.CanImport {
		t.Fatalf("preview block = %s", preview.BlockReason)
	}
	for _, account := range mustAccounts(t, h.st) {
		if account.Name == "新基金组合" {
			t.Fatal("new account was created before Import")
		}
	}

	result, err := h.service.Import(preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 2 || result.Results[0].Status != "applied" || result.Results[1].Status != "applied" {
		t.Fatalf("import result = %+v", result)
	}

	var created *domain.Account
	for _, account := range mustAccounts(t, h.st) {
		if account.Name == "新基金组合" {
			copy := account
			created = &copy
		}
	}
	if created == nil || created.Type != domain.AccountFund || created.Platform != efundSalesB || created.Currency != "CNY" {
		t.Fatalf("created account = %+v", created)
	}
	mapping, err := efund.LoadMapping(efund.DefaultMappingPath(h.dbPath))
	if err != nil {
		t.Fatal(err)
	}
	foundNewMapping := false
	for _, entry := range mapping.Mappings {
		if entry.SalesOrg == efundSalesB && entry.TradeAccount == "T0000002" {
			foundNewMapping = entry.AccountID == created.ID && entry.Account == created.Name
		}
	}
	if !foundNewMapping {
		t.Fatalf("new account mapping was not saved: %+v", mapping.Mappings)
	}
	existingSnapshots, err := h.st.ListConfirmed(context.Background(), existing.ID, "2026-11-17", "2026-11-17")
	if err != nil || len(existingSnapshots) != 1 {
		t.Fatalf("existing snapshots = %+v, err=%v", existingSnapshots, err)
	}
	snapshot := existingSnapshots[0]
	if snapshot.Source != domain.SourceEfundImport || snapshot.AsOf != "2026-11-17" || len(snapshot.Items) != 2 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	if !strings.Contains(snapshot.SourceNote, "交易账户 T0000001") || !strings.Contains(snapshot.SourceNote, "打印时间") {
		t.Fatalf("source note = %q", snapshot.SourceNote)
	}
	audits, err := h.st.ListAudit(context.Background(), snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundAudit := false
	for _, entry := range audits {
		if entry.Action == "submit" && strings.Contains(entry.Detail, "来源 efund_import") &&
			strings.Contains(entry.Detail, "2026-11-17") {
			foundAudit = true
		}
	}
	if !foundAudit {
		t.Fatalf("audits = %+v", audits)
	}

	again, err := h.service.Import(preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range again.Results {
		if item.Status != "skipped" || !strings.Contains(item.Message, "已跳过") {
			t.Fatalf("repeat import = %+v", again)
		}
	}
	if err := h.service.Undo(existing.ID); err != nil {
		t.Fatal(err)
	}
	if latest, err := h.st.LastConfirmed(context.Background(), existing.ID); err != nil || latest != nil {
		t.Fatalf("after undo = %+v, err=%v", latest, err)
	}
}

func TestEfundDesktopMergesGroupsAndFunds(t *testing.T) {
	h := newEfundHarness(t)
	account := h.account(t, "合并账户")
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100.5", "1.0", "2026/11/17", "100.50", "2026/11/17"),
		efundHolding(2, "000216", "示例成长混合", efundSalesA, "T0000002", "50.25", "1.0", "2026/11/17", "50.25", "2026/11/17"),
		efundFooter(2),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "merge.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	preview, err = h.service.Remap(preview.Token, []EfundSelection{
		{SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: efundModeExisting, AccountID: account.ID},
		{SalesOrg: efundSalesA, TradeAccount: "T0000002", Mode: efundModeExisting, AccountID: account.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Targets) != 1 || len(preview.Targets[0].Holdings) != 1 {
		t.Fatalf("targets = %+v", preview.Targets)
	}
	item := preview.Targets[0].Holdings[0]
	if item.Quantity != "150.75" || item.MarketValueCents != 15_075 {
		t.Fatalf("merged item = %+v", item)
	}
	if _, err := h.service.Import(preview.Token); err != nil {
		t.Fatal(err)
	}
	snapshots, err := h.st.ListConfirmed(context.Background(), account.ID, "2026-11-17", "2026-11-17")
	if err != nil || len(snapshots) != 1 || len(snapshots[0].Items) != 1 {
		t.Fatalf("snapshots = %+v, err=%v", snapshots, err)
	}
	if snapshots[0].Items[0].Quantity != "150.75" || snapshots[0].Items[0].MarketValueCents != 15_075 {
		t.Fatalf("snapshot item = %+v", snapshots[0].Items[0])
	}
}

func TestEfundDesktopIgnorePersistsAndBlocksEmptyImport(t *testing.T) {
	h := newEfundHarness(t)
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundFooter(1),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "ignore.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	preview, err = h.service.Remap(preview.Token, []EfundSelection{
		{SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: efundModeIgnore},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Groups[0].Ignored || len(preview.Targets) != 0 || preview.CanImport {
		t.Fatalf("ignored preview = %+v", preview)
	}
	if _, err := h.service.Import(preview.Token); err == nil {
		t.Fatal("empty ignored import was allowed")
	}
	mappingPath := efund.DefaultMappingPath(h.dbPath)
	mapping, err := efund.LoadMapping(mappingPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(mapping.Mappings) != 1 || !mapping.Mappings[0].Ignore {
		t.Fatalf("mapping = %+v", mapping)
	}
	if info, err := os.Stat(mappingPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mapping mode = %v, err=%v", info, err)
	}
	reopened, err := h.service.Preview(efundBase64(data), "", "ignore.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.Groups[0].Ignored {
		t.Fatalf("ignore was not loaded on reopen: %+v", reopened.Groups[0])
	}
}

func TestEfundDesktopBlocksWrite(t *testing.T) {
	h := newEfundHarness(t)
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundFooter(5),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "bad.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if preview.CanImport || !preview.BlockedByData || !strings.Contains(preview.BlockReason, "禁止写入") {
		t.Fatalf("preview = %+v", preview)
	}
	if _, err := h.service.Import(preview.Token); err == nil || !strings.Contains(err.Error(), "禁止写入") {
		t.Fatalf("import error = %v", err)
	}
	if accounts := mustAccounts(t, h.st); len(accounts) != 0 {
		t.Fatalf("accounts = %+v", accounts)
	}
}

func TestEfundDesktopAnomalyGoesPending(t *testing.T) {
	h := newEfundHarness(t)
	account := h.account(t, "异常账户")
	first, err := h.svc.Submit(context.Background(), domain.SubmitInput{
		AccountID: account.ID, AsOf: "2026-11-01", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "示例成长混合", Code: "000216", Quantity: "100", MarketValueCents: 10_000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Confirm(context.Background(), first.SnapshotID); err != nil {
		t.Fatal(err)
	}
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "2.0", "2026/11/17", "200.00", "2026/11/17"),
		efundFooter(1),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "anomaly.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	preview, err = h.service.Remap(preview.Token, []EfundSelection{
		{SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: efundModeExisting, AccountID: account.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Targets[0].Anomalies) == 0 {
		t.Fatalf("expected preview anomalies: %+v", preview.Targets[0])
	}
	result, err := h.service.Import(preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 || result.Results[0].Status != "pending" || len(result.Results[0].Anomalies) == 0 {
		t.Fatalf("result = %+v", result)
	}
	pending, err := h.st.ListPending(context.Background())
	if err != nil || len(pending) != 1 || pending[0].Source != domain.SourceEfundImport {
		t.Fatalf("pending = %+v, err=%v", pending, err)
	}
	last, err := h.st.LastConfirmed(context.Background(), account.ID)
	if err != nil || last == nil || last.AsOf != "2026-11-01" {
		t.Fatalf("last confirmed = %+v, err=%v", last, err)
	}
}

func TestEfundDesktopEncryptedZipPassword(t *testing.T) {
	h := newEfundHarness(t)
	xlsx := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundFooter(1),
	})
	archive := efundEncryptedZip(t, xlsx, "fictional-password")
	encoded := efundBase64(archive)
	if _, err := h.service.Preview(encoded, "wrong-password", "secret.zip"); err == nil ||
		!strings.Contains(err.Error(), "密码错误") || strings.Contains(err.Error(), "wrong-password") {
		t.Fatalf("wrong password error = %v", err)
	}
	preview, err := h.service.Preview(encoded, "fictional-password", "secret.zip")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Groups) != 1 {
		t.Fatalf("preview = %+v", preview)
	}
}

func mustAccounts(t *testing.T, st *store.Store) []domain.Account {
	t.Helper()
	accounts, err := st.ListAccounts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return accounts
}

func efundHeader() []efundCell {
	return []efundCell{
		efundStr("序号"), efundStr("基金代码"), efundStr("基金名称"), efundStr("份额类别"), efundStr("基金管理人"),
		efundStr("基金账户"), efundStr("销售机构"), efundStr("交易账户"), efundStr("持有份额"), efundStr("份额日期"),
		efundStr("基金净值"), efundStr("净值日期"), efundStr("资产情况\n（结算币种）"), efundStr("结算币种"), efundStr("分红方式"),
	}
}

func efundHolding(seq int, code, name, salesOrg, tradeAccount, quantity, nav, navDate, amount, shareDate string) []efundCell {
	return []efundCell{
		efundNum(fmt.Sprintf("%d.0", seq)), efundStr(code), efundStr(name), efundStr("A"), efundStr("示例基金管理有限公司"),
		efundStr("F000001"), efundStr(salesOrg), efundStr(tradeAccount), efundStr(quantity), efundStr(shareDate),
		efundStr(nav), efundStr(navDate), efundStr(amount), efundStr("人民币"), efundStr("现金分红"),
	}
}

func efundFooter(total int) []efundCell {
	return []efundCell{
		efundStr(fmt.Sprintf("总记录数： %d 笔", total)),
		efundStr("数据来源：基金E账户App"),
		efundStr("打印时间：2026年11月20日 22:21:38"),
	}
}

func efundBook(t *testing.T, rows [][]efundCell) []byte {
	t.Helper()
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for i, row := range rows {
		fmt.Fprintf(&sheet, `<row r="%d">`, i+1)
		for j, cell := range row {
			ref := efundColumn(j) + strconv.Itoa(i+1)
			if cell.numeric {
				fmt.Fprintf(&sheet, `<c r="%s" t="n" s="15"><v>%s</v></c>`, ref, cell.text)
			} else {
				fmt.Fprintf(&sheet, `<c r="%s" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`,
					ref, efundEscape(cell.text))
			}
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)

	files := map[string]string{
		"xl/workbook.xml":            `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="持有信息" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
		"xl/worksheets/sheet1.xml":   sheet.String(),
	}
	var buffer bytes.Buffer
	zw := zip.NewWriter(&buffer)
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
	return buffer.Bytes()
}

func efundColumn(index int) string {
	index++
	var out []byte
	for index > 0 {
		index--
		out = append([]byte{byte('A' + index%26)}, out...)
		index /= 26
	}
	return string(out)
}

func efundEscape(value string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(value))
	return b.String()
}

func efundBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func efundEncryptedZip(t *testing.T, data []byte, password string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	zw := yzip.NewWriter(&buffer)
	writer, err := zw.Encrypt("holdings.xlsx", password, yzip.AES256Encryption)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
