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
	"time"

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

func TestEfundClearSession(t *testing.T) {
	h := newEfundHarness(t)
	h.service.session = &efundSession{token: "token-a"}
	if err := h.service.ClearSession("token-b"); err != nil || h.service.session == nil {
		t.Fatalf("wrong token should keep session: %v", err)
	}
	if err := h.service.ClearSession("token-a"); err != nil || h.service.session != nil {
		t.Fatalf("matching token should clear session: %v", err)
	}
	if err := h.service.ClearSession("token-a"); err != nil {
		t.Fatalf("repeated clear should be idempotent: %v", err)
	}
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
		{SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map", AccountID: existing.ID, Direct: true},
		{SalesOrg: efundSalesB, TradeAccount: "T0000002", Mode: "map", AccountName: "新基金组合", Direct: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.CanImport {
		t.Fatalf("preview block = %s", preview.BlockReason)
	}
	createdAtSave := false
	for _, account := range mustAccounts(t, h.st) {
		if account.Name == "新基金组合" {
			createdAtSave = true
		}
	}
	if !createdAtSave {
		t.Fatal("new account was not created when mapping was saved")
	}

	result, err := h.service.Import(preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 2 || result.Results[0].Status != "applied" || result.Results[1].Status != "applied" {
		t.Fatalf("import result = %+v", result)
	}
	if result.Summary != "全部成功" || result.Applied != 2 {
		t.Fatalf("summary = %+v", result)
	}

	var created *domain.Account
	for _, account := range mustAccounts(t, h.st) {
		if account.Name == "新基金组合" {
			copy := account
			created = &copy
		}
	}
	if created == nil || created.Type != domain.AccountFund || created.Platform != "" || created.Currency != "CNY" {
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
		if item.Status != "skipped" || !strings.Contains(item.Message, "之前已导入") {
			t.Fatalf("repeat import = %+v", again)
		}
	}
	if again.Summary != "全部之前已导入" || again.Skipped != 2 {
		t.Fatalf("repeat summary = %+v", again)
	}
	if err := h.service.UndoSnapshot(result.Results[0].SnapshotID); err != nil {
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
		{SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map", AccountID: account.ID, Direct: true},
		{SalesOrg: efundSalesA, TradeAccount: "T0000002", Mode: "map", AccountID: account.ID, Direct: true},
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
		{SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map", AccountID: account.ID, Direct: true},
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

func TestEfundDesktopPortfolioTargetAndCashCarry(t *testing.T) {
	h := newEfundHarness(t)
	top := h.account(t, "示例账户")
	portfolio, err := h.svc.UpsertAccount(context.Background(), domain.UpsertAccountInput{
		Name: "示例组合", ParentID: top.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	cash := int64(12_300)
	previous, err := h.svc.Submit(context.Background(), domain.SubmitInput{
		AccountID: portfolio.ID, AsOf: "2026-11-01", Source: domain.SourceMCP, CashCents: &cash,
		Items: []domain.Item{{Name: "示例成长混合", Code: "000216", Quantity: "100", MarketValueCents: 10_000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Confirm(context.Background(), previous.SnapshotID); err != nil {
		t.Fatal(err)
	}
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundFooter(1),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "portfolio.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	preview, err = h.service.Remap(preview.Token, []EfundSelection{{
		SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map",
		AccountID: top.ID, PortfolioID: portfolio.ID,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Targets) != 1 || preview.Targets[0].AccountID != portfolio.ID ||
		preview.Targets[0].CashCents == nil || *preview.Targets[0].CashCents != cash {
		t.Fatalf("target = %+v", preview.Targets)
	}
	result, err := h.service.Import(preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	if result.Results[0].Status != "applied" {
		t.Fatalf("result = %+v", result)
	}
	snapshot, err := h.st.GetSnapshot(context.Background(), result.Results[0].SnapshotID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CashCents == nil || *snapshot.CashCents != cash || !strings.Contains(snapshot.SourceNote, "现金沿用上次") {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestEfundDesktopCreatesPortfolioWhenSavingMapping(t *testing.T) {
	h := newEfundHarness(t)
	top := h.account(t, "示例账户")
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundFooter(1),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "new-portfolio.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	preview, err = h.service.Remap(preview.Token, []EfundSelection{{
		SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map",
		AccountID: top.ID, PortfolioName: "新组合",
	}})
	if err != nil {
		t.Fatal(err)
	}
	var portfolio *domain.Account
	for _, account := range mustAccounts(t, h.st) {
		if account.Name == "新组合" {
			copy := account
			portfolio = &copy
		}
	}
	if portfolio == nil || portfolio.ParentID != top.ID || portfolio.Type != domain.AccountFund {
		t.Fatalf("portfolio = %+v", portfolio)
	}
	if len(preview.Targets) != 1 || preview.Targets[0].AccountID != portfolio.ID {
		t.Fatalf("preview = %+v", preview.Targets)
	}
}

func TestEfundDesktopAccountOptionsShowSnapshots(t *testing.T) {
	h := newEfundHarness(t)
	withSnapshot := h.account(t, "有快照账户")
	h.account(t, "空账户")
	result, err := h.svc.Submit(context.Background(), domain.SubmitInput{
		AccountID: withSnapshot.ID, AsOf: "2026-11-01", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "示例成长混合", Code: "000216", Quantity: "1", MarketValueCents: 100}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Confirm(context.Background(), result.SnapshotID); err != nil {
		t.Fatal(err)
	}
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundFooter(1),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "snapshots.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	flags := map[string]bool{}
	for _, option := range preview.Accounts {
		flags[option.ID] = option.HasSnapshots
	}
	if !flags[withSnapshot.ID] {
		t.Fatalf("account with snapshots not marked: %+v", preview.Accounts)
	}
	for _, option := range preview.Accounts {
		if option.Name == "空账户" && option.HasSnapshots {
			t.Fatalf("empty account marked as having snapshots: %+v", option)
		}
	}
}

func TestEfundDesktopFailedMappingArchivesNewAccount(t *testing.T) {
	h := newEfundHarness(t)
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundFooter(1),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "cleanup.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.Remap(preview.Token, []EfundSelection{{
		SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map",
		AccountName: "将被清理",
	}}); err == nil {
		t.Fatal("incomplete mapping was accepted")
	}
	for _, account := range mustAccounts(t, h.st) {
		if account.Name == "将被清理" && !account.Archived {
			t.Fatalf("failed mapping left orphan account: %+v", account)
		}
	}
}

func TestEfundDesktopFailedRemapRollsBackAllCreatedAccounts(t *testing.T) {
	h := newEfundHarness(t)
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundHolding(2, "000217", "示例价值混合", efundSalesB, "T0000002", "10", "2.0", "2026/11/17", "20.00", "2026/11/17"),
		efundFooter(2),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "rollback.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.service.Remap(preview.Token, []EfundSelection{
		{SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map", AccountName: "多选账户", PortfolioName: "多选组合"},
		{SalesOrg: efundSalesB, TradeAccount: "T0000002", Mode: "map", AccountName: "后失败账户"},
	}); err == nil {
		t.Fatal("incomplete second selection was accepted")
	}
	want := map[string]bool{"多选账户": false, "多选组合": false, "后失败账户": false}
	for _, account := range mustAccounts(t, h.st) {
		if _, ok := want[account.Name]; ok {
			want[account.Name] = account.Archived
		}
	}
	for name, archived := range want {
		if !archived {
			t.Fatalf("created account %q was not rolled back", name)
		}
	}
	if _, err := os.Stat(efund.DefaultMappingPath(h.dbPath)); !os.IsNotExist(err) {
		t.Fatalf("failed remap wrote mapping: %v", err)
	}
	if h.service.session == nil || len(h.service.session.mapping.Mappings) != 0 {
		t.Fatalf("failed remap changed session mapping: %+v", h.service.session)
	}
}

func TestEfundDesktopRemapReusesNewTargets(t *testing.T) {
	h := newEfundHarness(t)
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundHolding(2, "000217", "示例价值混合", efundSalesB, "T0000002", "10", "2.0", "2026/11/17", "20.00", "2026/11/17"),
		efundFooter(2),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "reuse.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	preview, err = h.service.Remap(preview.Token, []EfundSelection{
		{SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map", AccountName: "批量账户", PortfolioName: "批量组合"},
		{SalesOrg: efundSalesB, TradeAccount: "T0000002", Mode: "map", AccountName: "批量账户", PortfolioName: "批量组合"},
	})
	if err != nil {
		t.Fatal(err)
	}
	accounts := mustAccounts(t, h.st)
	tops, portfolios := 0, 0
	for _, account := range accounts {
		if account.Archived {
			continue
		}
		switch account.Name {
		case "批量账户":
			tops++
		case "批量组合":
			portfolios++
		}
	}
	if tops != 1 || portfolios != 1 {
		t.Fatalf("reused targets: tops=%d portfolios=%d accounts=%+v", tops, portfolios, accounts)
	}
	if len(preview.Targets) != 1 || preview.Targets[0].GroupCount != 2 {
		t.Fatalf("targets = %+v", preview.Targets)
	}
}

func TestEfundDesktopRemapUnknownKeyCreatesNothing(t *testing.T) {
	h := newEfundHarness(t)
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundFooter(1),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "unknown.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	_, err = h.service.Remap(preview.Token, []EfundSelection{
		{SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map", AccountName: "不应创建"},
		{SalesOrg: efundSalesA, TradeAccount: "T9999999", Mode: "map", AccountName: "也不应创建"},
	})
	if err == nil {
		t.Fatal("unknown trade account was accepted")
	}
	for _, account := range mustAccounts(t, h.st) {
		if account.Name == "不应创建" && !account.Archived {
			t.Fatalf("unknown key left an active account: %+v", account)
		}
	}
	if _, err := os.Stat(efund.DefaultMappingPath(h.dbPath)); !os.IsNotExist(err) {
		t.Fatalf("failed remap wrote mapping: %v", err)
	}
}

func TestEfundDesktopMappingWriteRetriesAfterFailure(t *testing.T) {
	h := newEfundHarness(t)
	account := h.account(t, "示例账户")
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundFooter(1),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "retry.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.service.mappingPath = filepath.Join(blocker, "efund-map.json")
	selection := []EfundSelection{{
		SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map", AccountID: account.ID, Direct: true,
	}}
	preview, err = h.service.Remap(preview.Token, selection)
	if err != nil {
		t.Fatal(err)
	}
	if preview.MappingError == "" {
		t.Fatalf("mapping error missing: %+v", preview)
	}
	refreshed, err := h.service.Refresh(preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.MappingError == "" {
		t.Fatalf("refresh lost pending mapping error: %+v", refreshed)
	}

	mappingPath := filepath.Join(t.TempDir(), "efund-map.json")
	h.service.mappingPath = mappingPath
	preview, err = h.service.Remap(preview.Token, selection)
	if err != nil {
		t.Fatal(err)
	}
	if preview.MappingError != "" {
		t.Fatalf("mapping error not cleared after retry: %+v", preview)
	}
	mapping, err := efund.LoadMapping(mappingPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(mapping.Mappings) != 1 || mapping.Mappings[0].AccountID != account.ID {
		t.Fatalf("mapping was not retried: %+v", mapping.Mappings)
	}
}

func TestEfundDesktopConflictsWithAnySource(t *testing.T) {
	h := newEfundHarness(t)
	account := h.account(t, "冲突账户")
	prior, err := h.svc.Submit(context.Background(), domain.SubmitInput{
		AccountID: account.ID, AsOf: "2026-11-17", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "示例成长混合", Code: "000216", Quantity: "100", MarketValueCents: 10_000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Confirm(context.Background(), prior.SnapshotID); err != nil {
		t.Fatal(err)
	}
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundFooter(1),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "conflict.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	preview, err = h.service.Remap(preview.Token, []EfundSelection{{
		SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map", AccountID: account.ID, Direct: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Targets[0].AlreadyImported || !strings.Contains(preview.Targets[0].ImportedStatus, "mcp") {
		t.Fatalf("target = %+v", preview.Targets[0])
	}
	if preview.Targets[0].ExistingSnapshotID != prior.SnapshotID ||
		preview.Targets[0].ExistingSnapshotSource != domain.SourceMCP {
		t.Fatalf("target snapshot = %+v", preview.Targets[0])
	}
	result, err := h.service.Import(preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	if result.Results[0].Status != "skipped" || !strings.Contains(result.Results[0].Message, "mcp") {
		t.Fatalf("result = %+v", result)
	}
	if result.Results[0].ExistingSnapshotID != prior.SnapshotID ||
		result.Results[0].ExistingSnapshotSource != domain.SourceMCP {
		t.Fatalf("result snapshot = %+v", result.Results[0])
	}
}

func TestEfundDesktopConflictsWithPendingOtherSource(t *testing.T) {
	h := newEfundHarness(t)
	account := h.account(t, "待确认账户")
	if _, err := h.svc.Submit(context.Background(), domain.SubmitInput{
		AccountID: account.ID, AsOf: "2026-11-17", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "示例成长混合", Code: "000216", Quantity: "100", MarketValueCents: 10_000}},
	}); err != nil {
		t.Fatal(err)
	}
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundFooter(1),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "pending.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	preview, err = h.service.Remap(preview.Token, []EfundSelection{{
		SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map", AccountID: account.ID, Direct: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !preview.Targets[0].AlreadyImported || !strings.Contains(preview.Targets[0].ImportedStatus, "待确认") {
		t.Fatalf("target = %+v", preview.Targets[0])
	}
	result, err := h.service.Import(preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	if result.Results[0].Status != "skipped" {
		t.Fatalf("result = %+v", result)
	}
	if result.Results[0].ExistingSnapshotSource != domain.SourceMCP ||
		result.Results[0].ExistingSnapshotID == "" {
		t.Fatalf("result snapshot = %+v", result.Results[0])
	}
}

func TestEfundDesktopUndoPreviouslyImportedEfundSnapshot(t *testing.T) {
	h := newEfundHarness(t)
	account := h.account(t, "已导入账户")
	prior, err := h.svc.Submit(context.Background(), domain.SubmitInput{
		AccountID: account.ID, AsOf: "2026-11-17", Source: domain.SourceEfundImport,
		Items: []domain.Item{{Name: "示例成长混合", Code: "000216", Quantity: "100", MarketValueCents: 10_000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Confirm(context.Background(), prior.SnapshotID); err != nil {
		t.Fatal(err)
	}
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundFooter(1),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "previous-import.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	preview, err = h.service.Remap(preview.Token, []EfundSelection{{
		SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map", AccountID: account.ID, Direct: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	target := preview.Targets[0]
	if !target.AlreadyImported || target.ExistingSnapshotID != prior.SnapshotID ||
		target.ExistingSnapshotSource != domain.SourceEfundImport {
		t.Fatalf("target = %+v", target)
	}
	result, err := h.service.Import(preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	if result.Results[0].Status != "skipped" || result.Results[0].ExistingSnapshotID != prior.SnapshotID ||
		result.Results[0].ExistingSnapshotSource != domain.SourceEfundImport {
		t.Fatalf("result = %+v", result.Results[0])
	}
	if err := h.service.UndoSnapshot(prior.SnapshotID); err != nil {
		t.Fatal(err)
	}
	preview, err = h.service.Refresh(preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Targets[0].AlreadyImported || preview.Targets[0].ExistingSnapshotID != "" {
		t.Fatalf("after undo target = %+v", preview.Targets[0])
	}
	result, err = h.service.Import(preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	if result.Results[0].Status != "applied" {
		t.Fatalf("reimport = %+v", result.Results[0])
	}
}

func TestEfundDesktopPerTargetImport(t *testing.T) {
	h := newEfundHarness(t)
	first := h.account(t, "账户一")
	second := h.account(t, "账户二")
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundHolding(2, "000217", "示例价值混合", efundSalesB, "T0000002", "10", "2.0", "2026/11/17", "20.00", "2026/11/17"),
		efundFooter(2),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "targets.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	preview, err = h.service.Remap(preview.Token, []EfundSelection{
		{SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map", AccountID: first.ID, Direct: true},
		{SalesOrg: efundSalesB, TradeAccount: "T0000002", Mode: "map", AccountID: second.ID, Direct: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Targets) != 2 {
		t.Fatalf("targets = %+v", preview.Targets)
	}
	key := preview.Targets[0].Key
	result, err := h.service.ImportTarget(preview.Token, key)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 1 || result.Results[0].Status != "applied" {
		t.Fatalf("result = %+v", result)
	}
	if snaps, err := h.st.ListConfirmed(context.Background(), second.ID, "", ""); err != nil || len(snaps) != 0 {
		t.Fatalf("second account snapshots = %+v, err=%v", snaps, err)
	}
	again, err := h.service.ImportTarget(preview.Token, key)
	if err != nil {
		t.Fatal(err)
	}
	if again.Results[0].Status != "skipped" {
		t.Fatalf("duplicate target = %+v", again)
	}
	if again.Summary != "全部之前已导入" {
		t.Fatalf("duplicate summary = %+v", again)
	}
}

func TestEfundDesktopRejectsNonFundTarget(t *testing.T) {
	h := newEfundHarness(t)
	stock, err := h.svc.UpsertAccount(context.Background(), domain.UpsertAccountInput{
		Name: "股票账户", Type: domain.AccountStock,
	})
	if err != nil {
		t.Fatal(err)
	}
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundFooter(1),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "stock.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range preview.Accounts {
		if account.ID == stock.ID {
			t.Fatal("non-fund account appeared in import options")
		}
	}
	if _, err := h.service.Remap(preview.Token, []EfundSelection{{
		SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map", AccountID: stock.ID, Direct: true,
	}}); err == nil || !strings.Contains(err.Error(), "fund") {
		t.Fatalf("error = %v", err)
	}
}

func TestEfundDesktopBlocksNonCNYAndShareDateMismatch(t *testing.T) {
	t.Run("non-CNY", func(t *testing.T) {
		h := newEfundHarness(t)
		account := h.account(t, "示例账户")
		rows := [][]efundCell{
			efundHeader(),
			efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
			efundFooter(1),
		}
		rows[1][13] = efundStr("美元")
		preview, err := h.service.Preview(efundBase64(efundBook(t, rows)), "", "usd.xlsx")
		if err != nil {
			t.Fatal(err)
		}
		preview, err = h.service.Remap(preview.Token, []EfundSelection{{
			SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map", AccountID: account.ID, Direct: true,
		}})
		if err != nil {
			t.Fatal(err)
		}
		if !preview.BlockedByData {
			t.Fatalf("preview = %+v", preview)
		}
		if _, err := h.service.ImportTarget(preview.Token, preview.Targets[0].Key); err == nil {
			t.Fatal("blocked target imported")
		}
	})
	t.Run("share-date-mismatch", func(t *testing.T) {
		h := newEfundHarness(t)
		account := h.account(t, "示例账户")
		data := efundBook(t, [][]efundCell{
			efundHeader(),
			efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
			efundHolding(2, "000217", "示例价值混合", efundSalesA, "T0000001", "10", "2.0", "2026/11/18", "20.00", "2026/11/18"),
			efundFooter(2),
		})
		preview, err := h.service.Preview(efundBase64(data), "", "dates.xlsx")
		if err != nil {
			t.Fatal(err)
		}
		preview, err = h.service.Remap(preview.Token, []EfundSelection{{
			SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map", AccountID: account.ID, Direct: true,
		}})
		if err != nil {
			t.Fatal(err)
		}
		if !preview.BlockedByData {
			t.Fatalf("preview = %+v", preview)
		}
		if _, err := h.service.ImportTarget(preview.Token, preview.Targets[0].Key); err == nil {
			t.Fatal("blocked target imported")
		}
	})
}

func TestEfundDesktopMappingWriteFailureStillImports(t *testing.T) {
	h := newEfundHarness(t)
	account := h.account(t, "示例账户")
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundFooter(1),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "mapping.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.service.mappingPath = filepath.Join(blocker, "efund-map.json")
	preview, err = h.service.Remap(preview.Token, []EfundSelection{{
		SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map", AccountID: account.ID, Direct: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if preview.MappingError == "" {
		t.Fatalf("mapping error missing: %+v", preview)
	}
	result, err := h.service.Import(preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	if result.Results[0].Status != "applied" {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Warnings) == 0 || !strings.Contains(result.Warnings[0], "映射未保存") {
		t.Fatalf("warnings = %+v", result.Warnings)
	}
}

func TestEfundDesktopUndoRejectsNonLatest(t *testing.T) {
	h := newEfundHarness(t)
	account := h.account(t, "示例账户")
	data := efundBook(t, [][]efundCell{
		efundHeader(),
		efundHolding(1, "000216", "示例成长混合", efundSalesA, "T0000001", "100", "1.0", "2026/11/17", "100.00", "2026/11/17"),
		efundFooter(1),
	})
	preview, err := h.service.Preview(efundBase64(data), "", "undo.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	preview, err = h.service.Remap(preview.Token, []EfundSelection{{
		SalesOrg: efundSalesA, TradeAccount: "T0000001", Mode: "map", AccountID: account.ID, Direct: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := h.service.Import(preview.Token)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	later, err := h.svc.Submit(context.Background(), domain.SubmitInput{
		AccountID: account.ID, AsOf: "2026-11-18", Source: domain.SourceMCP,
		Items: []domain.Item{{Name: "示例成长混合", Code: "000216", Quantity: "100", MarketValueCents: 10_000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Confirm(context.Background(), later.SnapshotID); err != nil {
		t.Fatal(err)
	}
	if err := h.service.UndoSnapshot(result.Results[0].SnapshotID); err == nil ||
		!strings.Contains(err.Error(), "最新") {
		t.Fatalf("undo error = %v", err)
	}
	last, err := h.st.LastConfirmed(context.Background(), account.ID)
	if err != nil || last == nil || last.ID != later.SnapshotID {
		t.Fatalf("last confirmed = %+v, err=%v", last, err)
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
