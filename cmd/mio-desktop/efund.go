package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/curiselven/mio-money/internal/domain"
	"github.com/curiselven/mio-money/internal/importer/efund"
	"github.com/curiselven/mio-money/internal/store"
)

const (
	efundModeExisting = "existing"
	efundModeNew      = "new"
	efundModeIgnore   = "ignore"
	efundModeUnmapped = "unmapped"
)

type EfundService struct {
	svc         *domain.Service
	st          *store.Store
	mappingPath string
	mu          sync.Mutex
	session     *efundSession
}

type efundSession struct {
	token      string
	parsed     *efund.Result
	mapping    *efund.Mapping
	mappingErr string
}

type EfundAccountOption struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Platform    string             `json:"platform,omitempty"`
	Type        domain.AccountType `json:"type"`
	Currency    string             `json:"currency"`
	ParentID    string             `json:"parent_id,omitempty"`
	ParentName  string             `json:"parent_name,omitempty"`
	HasChildren bool               `json:"has_children"`
}

type EfundGroupPreview struct {
	SalesOrg         string          `json:"sales_org"`
	TradeAccount     string          `json:"trade_account"`
	Name             string          `json:"name"`
	Mode             string          `json:"mode"`
	AccountID        string          `json:"account_id,omitempty"`
	AccountName      string          `json:"account_name,omitempty"`
	TopAccountID     string          `json:"top_account_id,omitempty"`
	TopAccountName   string          `json:"top_account_name,omitempty"`
	PortfolioID      string          `json:"portfolio_id,omitempty"`
	PortfolioName    string          `json:"portfolio_name,omitempty"`
	Direct           bool            `json:"direct"`
	Ignored          bool            `json:"ignored"`
	FundCount        int             `json:"fund_count"`
	MarketValueCents int64           `json:"market_value_cents"`
	TopFunds         []string        `json:"top_funds"`
	Holdings         []efund.Holding `json:"holdings"`
	Issue            string          `json:"issue,omitempty"`
}

type EfundTargetPreview struct {
	Key                    string           `json:"key"`
	AccountID              string           `json:"account_id,omitempty"`
	AccountName            string           `json:"account_name"`
	GroupCount             int              `json:"group_count"`
	FundCount              int              `json:"fund_count"`
	MarketValueCents       int64            `json:"market_value_cents"`
	PreviousAsOf           string           `json:"previous_as_of,omitempty"`
	Diff                   domain.Diff      `json:"diff"`
	Anomalies              []domain.Anomaly `json:"anomalies"`
	Holdings               []domain.Item    `json:"holdings"`
	AlreadyImported        bool             `json:"already_imported"`
	ImportedStatus         string           `json:"imported_status,omitempty"`
	ExistingSnapshotID     string           `json:"existing_snapshot_id,omitempty"`
	ExistingSnapshotSource domain.Source    `json:"existing_snapshot_source,omitempty"`
	CashCents              *int64           `json:"cash_cents,omitempty"`
	mappingKeys            []string
	tradeAccounts          []string
}

type EfundPreview struct {
	Token                 string               `json:"token"`
	FileName              string               `json:"file_name"`
	Meta                  efund.Meta           `json:"meta"`
	Warnings              []efund.Warning      `json:"warnings"`
	Groups                []EfundGroupPreview  `json:"groups"`
	Targets               []EfundTargetPreview `json:"targets"`
	Accounts              []EfundAccountOption `json:"accounts"`
	CanImport             bool                 `json:"can_import"`
	BlockReason           string               `json:"block_reason,omitempty"`
	BlockedByData         bool                 `json:"blocked_by_data"`
	TotalMarketValueCents int64                `json:"total_market_value_cents"`
	FileTotalCents        int64                `json:"file_total_cents"`
	IgnoredTotalCents     int64                `json:"ignored_total_cents"`
	UnmappedTotalCents    int64                `json:"unmapped_total_cents"`
	MappingError          string               `json:"mapping_error,omitempty"`
	TotalHoldingCount     int                  `json:"total_holding_count"`
}

type EfundSelection struct {
	SalesOrg      string `json:"sales_org"`
	TradeAccount  string `json:"trade_account"`
	Mode          string `json:"mode"`
	AccountID     string `json:"account_id,omitempty"`
	AccountName   string `json:"account_name,omitempty"`
	PortfolioID   string `json:"portfolio_id,omitempty"`
	PortfolioName string `json:"portfolio_name,omitempty"`
	Direct        bool   `json:"direct"`
}

type EfundImportAccountResult struct {
	AccountID              string        `json:"account_id,omitempty"`
	AccountName            string        `json:"account_name"`
	Status                 string        `json:"status"`
	SnapshotID             string        `json:"snapshot_id,omitempty"`
	Anomalies              []string      `json:"anomalies,omitempty"`
	Message                string        `json:"message,omitempty"`
	ExistingSnapshotID     string        `json:"existing_snapshot_id,omitempty"`
	ExistingSnapshotSource domain.Source `json:"existing_snapshot_source,omitempty"`
	MarketValueCents       int64         `json:"market_value_cents"`
}

type EfundImportResult struct {
	Summary  string                     `json:"summary"`
	Applied  int                        `json:"applied"`
	Pending  int                        `json:"pending"`
	Skipped  int                        `json:"skipped"`
	Failed   int                        `json:"failed"`
	Results  []EfundImportAccountResult `json:"results"`
	Warnings []string                   `json:"warnings,omitempty"`
}

type efundGroup struct {
	salesOrg     string
	tradeAccount string
	holdings     []efund.Holding
}

type efundTarget struct {
	accountID     string
	accountName   string
	groups        []EfundGroupPreview
	holdings      []efund.Holding
	mappingKeys   []string
	tradeAccounts []string
	cashCents     *int64
}

func NewEfundService(svc *domain.Service, st *store.Store, dbPath string) *EfundService {
	return &EfundService{
		svc:         svc,
		st:          st,
		mappingPath: efund.DefaultMappingPath(dbPath),
	}
}

func (s *EfundService) Preview(dataBase64, password, fileName string) (*EfundPreview, error) {
	data, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		return nil, errors.New("文件数据读取失败")
	}
	parsed, err := efund.ParseBytes(fileName, data, func() (string, error) {
		if password == "" {
			return "", errors.New("zip 已加密，请输入密码")
		}
		return password, nil
	})
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	mapping, err := s.loadMapping()
	if err != nil {
		return nil, err
	}
	accounts, err := s.st.ListAccounts(context.Background())
	if err != nil {
		return nil, err
	}
	preview, err := s.buildPreview(parsed, mapping, accounts)
	if err != nil {
		return nil, err
	}
	token := newEfundToken()
	s.session = &efundSession{token: token, parsed: parsed, mapping: mapping}
	preview.Token = token
	return preview, nil
}

func (s *EfundService) Remap(token string, selections []EfundSelection) (*EfundPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.sessionFor(token)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	accounts, err := s.st.ListAccounts(context.Background())
	if err != nil {
		return nil, err
	}
	mapping := cloneMapping(session.mapping)
	valid := map[string]bool{}
	for _, h := range session.parsed.Holdings {
		valid[efund.MappingKey(h.SalesOrg, h.TradeAccount)] = true
	}
	for _, selection := range selections {
		key := efund.MappingKey(selection.SalesOrg, selection.TradeAccount)
		if !valid[key] {
			return nil, fmt.Errorf("未知的交易账户：%s / %s", selection.SalesOrg, selection.TradeAccount)
		}
		accounts, err = applyEfundSelection(ctx, s.svc, mapping, accounts, selection)
		if err != nil {
			return nil, err
		}
	}
	if !mappingEqual(mapping, session.mapping) {
		session.mapping = mapping
		if err := efund.WriteMapping(s.mappingPath, mapping); err != nil {
			session.mappingErr = err.Error()
		} else {
			session.mappingErr = ""
		}
	}
	preview, err := s.buildPreview(session.parsed, mapping, accounts)
	if err != nil {
		return nil, err
	}
	s.applyMappingWarning(preview)
	preview.Token = token
	return preview, nil
}

func (s *EfundService) Refresh(token string) (*EfundPreview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.sessionFor(token)
	if err != nil {
		return nil, err
	}
	mapping := session.mapping
	if mapping == nil {
		mapping, err = s.loadMapping()
		if err != nil {
			return nil, err
		}
		session.mapping = mapping
	}
	accounts, err := s.st.ListAccounts(context.Background())
	if err != nil {
		return nil, err
	}
	preview, err := s.buildPreview(session.parsed, mapping, accounts)
	if err != nil {
		return nil, err
	}
	s.applyMappingWarning(preview)
	preview.Token = token
	return preview, nil
}

func (s *EfundService) Import(token string) (*EfundImportResult, error) {
	return s.importTargets(token, "")
}

func (s *EfundService) ImportTarget(token, targetKey string) (*EfundImportResult, error) {
	return s.importTargets(token, targetKey)
}

func (s *EfundService) importTargets(token, onlyKey string) (*EfundImportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, err := s.sessionFor(token)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	accounts, err := s.st.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	mapping := session.mapping
	if mapping == nil {
		mapping, err = s.loadMapping()
		if err != nil {
			return nil, err
		}
		session.mapping = mapping
	}
	preview, err := s.buildPreview(session.parsed, mapping, accounts)
	if err != nil {
		return nil, err
	}
	if preview.BlockedByData {
		return nil, errors.New(preview.BlockReason)
	}
	if onlyKey == "" && !preview.CanImport {
		return nil, errors.New(preview.BlockReason)
	}

	result := &EfundImportResult{Results: []EfundImportAccountResult{}}
	for _, target := range preview.Targets {
		if onlyKey != "" && target.Key != onlyKey {
			continue
		}
		if target.AlreadyImported {
			result.Results = append(result.Results, EfundImportAccountResult{
				AccountID: target.AccountID, AccountName: target.AccountName,
				Status:                 "skipped",
				Message:                "之前已导入：该账户在 " + session.parsed.Meta.ShareDate + " 已有 " + target.ImportedStatus + " 快照",
				ExistingSnapshotID:     target.ExistingSnapshotID,
				ExistingSnapshotSource: target.ExistingSnapshotSource,
				MarketValueCents:       target.MarketValueCents,
			})
			continue
		}
		accountID := target.AccountID
		existing, err := s.conflictSnapshot(ctx, accountID, session.parsed.Meta.ShareDate)
		if err != nil {
			result.Results = append(result.Results, EfundImportAccountResult{
				AccountID: accountID, AccountName: target.AccountName,
				Status: "failed", Message: err.Error(), MarketValueCents: target.MarketValueCents,
			})
			continue
		}
		if existing != nil {
			existingID, existingSource, status := efundSnapshotFields(existing)
			result.Results = append(result.Results, EfundImportAccountResult{
				AccountID: accountID, AccountName: target.AccountName,
				Status: "skipped", Message: "之前已导入：该账户在 " + session.parsed.Meta.ShareDate + " 已有 " + status + " 快照",
				ExistingSnapshotID: existingID, ExistingSnapshotSource: existingSource,
				MarketValueCents: target.MarketValueCents,
			})
			continue
		}

		note := efundSourceNote(session.parsed.Meta, target.tradeAccounts)
		if target.CashCents != nil {
			note += "，现金沿用上次"
		}
		submitted, err := s.svc.Submit(ctx, domain.SubmitInput{
			AccountID:   accountID,
			AsOf:        session.parsed.Meta.ShareDate,
			Source:      domain.SourceEfundImport,
			SourceNote:  note,
			Currency:    "CNY",
			CashCents:   target.CashCents,
			Items:       target.Holdings,
			AuditDetail: efundAuditDetail(session.parsed.Meta),
		})
		if err != nil {
			result.Results = append(result.Results, EfundImportAccountResult{
				AccountID: accountID, AccountName: target.AccountName,
				Status: "failed", Message: err.Error(), MarketValueCents: target.MarketValueCents,
			})
			continue
		}
		_, confirmErr := s.svc.Confirm(ctx, submitted.SnapshotID)
		item := EfundImportAccountResult{
			AccountID: accountID, AccountName: target.AccountName,
			SnapshotID: submitted.SnapshotID, Anomalies: submitted.Anomalies,
			MarketValueCents: target.MarketValueCents,
		}
		switch {
		case confirmErr == nil:
			item.Status = "applied"
			item.Message = "已生效"
		case errors.Is(confirmErr, domain.ErrAnomaly):
			item.Status = "pending"
			item.Message = "命中异常规则，已进入待确认"
		default:
			item.Status = "failed"
			item.Message = confirmErr.Error()
		}
		result.Results = append(result.Results, item)
	}
	if len(result.Results) == 0 {
		return nil, errors.New("没有找到要导入的目标")
	}
	if session.mappingErr != "" {
		result.Warnings = append(result.Warnings, "映射未保存："+session.mappingErr)
	}
	summarizeImportResult(result)
	return result, nil
}

func (s *EfundService) UndoSnapshot(snapshotID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.svc.UndoSnapshot(context.Background(), snapshotID)
	return err
}

// ClearSession drops the in-memory preview; it does not touch the database or mapping file.
func (s *EfundService) ClearSession(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.session != nil && token != "" && s.session.token == token {
		s.session = nil
	}
	return nil
}

func (s *EfundService) applyMappingWarning(preview *EfundPreview) {
	if s.session == nil || s.session.mappingErr == "" {
		return
	}
	preview.MappingError = s.session.mappingErr
	preview.Warnings = append(preview.Warnings, efund.Warning{
		Code: "mapping_save_failed", Severity: efund.SeverityWarning,
		Message: "映射未保存：" + s.session.mappingErr,
	})
}

func (s *EfundService) sessionFor(token string) (*efundSession, error) {
	if s.session == nil || token == "" || s.session.token != token {
		return nil, errors.New("预览已过期，请重新解析文件")
	}
	return s.session, nil
}

func (s *EfundService) loadMapping() (*efund.Mapping, error) {
	mapping, err := efund.LoadMapping(s.mappingPath)
	if errors.Is(err, os.ErrNotExist) {
		return &efund.Mapping{Mappings: []efund.MappingEntry{}}, nil
	}
	return mapping, err
}

func (s *EfundService) conflictSnapshot(ctx context.Context, accountID, asOf string) (*domain.Snapshot, error) {
	confirmed, err := s.st.ListConfirmed(ctx, accountID, asOf, asOf)
	if err != nil {
		return nil, err
	}
	if len(confirmed) > 0 {
		return confirmed[0], nil
	}
	pending, err := s.st.ListPending(ctx)
	if err != nil {
		return nil, err
	}
	for _, snapshot := range pending {
		if snapshot.AccountID == accountID && snapshot.AsOf == asOf {
			return snapshot, nil
		}
	}
	return nil, nil
}

func efundSnapshotFields(snapshot *domain.Snapshot) (string, domain.Source, string) {
	if snapshot == nil {
		return "", "", ""
	}
	status := string(snapshot.Status)
	switch snapshot.Status {
	case domain.StatusConfirmed:
		status = "已确认"
	case domain.StatusPending:
		status = "待确认"
	case domain.StatusRejected:
		status = "已丢弃"
	case domain.StatusUndone:
		status = "已撤销"
	}
	return snapshot.ID, snapshot.Source, string(snapshot.Source) + " / " + status
}

func findMappingEntry(mapping *efund.Mapping, key string) *efund.MappingEntry {
	for i := range mapping.Mappings {
		if efund.MappingKey(mapping.Mappings[i].SalesOrg, mapping.Mappings[i].TradeAccount) == key {
			return &mapping.Mappings[i]
		}
	}
	return nil
}

func groupEfundHoldings(holdings []efund.Holding) []efundGroup {
	order := []string{}
	byKey := map[string]*efundGroup{}
	for _, holding := range holdings {
		key := efund.MappingKey(holding.SalesOrg, holding.TradeAccount)
		group := byKey[key]
		if group == nil {
			group = &efundGroup{salesOrg: holding.SalesOrg, tradeAccount: holding.TradeAccount}
			byKey[key] = group
			order = append(order, key)
		}
		group.holdings = append(group.holdings, holding)
	}
	out := make([]efundGroup, 0, len(order))
	for _, key := range order {
		out = append(out, *byKey[key])
	}
	return out
}

func addEfundGroupToTarget(target *efundTarget, group efundGroup, key string) {
	target.groups = append(target.groups, EfundGroupPreview{
		SalesOrg: group.salesOrg, TradeAccount: group.tradeAccount,
	})
	target.holdings = append(target.holdings, group.holdings...)
	target.mappingKeys = append(target.mappingKeys, key)
	found := false
	for _, trade := range target.tradeAccounts {
		if trade == group.tradeAccount {
			found = true
			break
		}
	}
	if !found {
		target.tradeAccounts = append(target.tradeAccounts, group.tradeAccount)
	}
}

func aggregateEfundItems(holdings []efund.Holding) ([]domain.Item, error) {
	type aggregate struct {
		name      string
		quantity  *big.Rat
		value     int64
		notes     []string
		noteIndex map[string]bool
	}
	order := []string{}
	byCode := map[string]*aggregate{}
	for _, holding := range holdings {
		quantity, ok := new(big.Rat).SetString(holding.Quantity)
		if !ok {
			return nil, fmt.Errorf("基金 %s 的份额无效", holding.FundCode)
		}
		item := byCode[holding.FundCode]
		if item == nil {
			item = &aggregate{name: holding.FundName, quantity: new(big.Rat), noteIndex: map[string]bool{}}
			byCode[holding.FundCode] = item
			order = append(order, holding.FundCode)
		}
		item.quantity.Add(item.quantity, quantity)
		item.value += holding.MarketValueCents
		note := fmt.Sprintf("净值 %s（%s）", holding.NAV, holding.NAVDate)
		if !item.noteIndex[note] {
			item.noteIndex[note] = true
			item.notes = append(item.notes, note)
		}
	}
	items := make([]domain.Item, 0, len(order))
	for _, code := range order {
		item := byCode[code]
		items = append(items, domain.Item{
			Name: item.name, Code: code, Quantity: decimalString(item.quantity),
			MarketValueCents: item.value, Note: strings.Join(item.notes, "；"),
		})
	}
	return items, nil
}

func decimalString(value *big.Rat) string {
	den := new(big.Int).Set(value.Denom())
	two, five := big.NewInt(2), big.NewInt(5)
	rem := new(big.Int)
	twos, fives := 0, 0
	for {
		rem.Mod(den, two)
		if rem.Sign() != 0 {
			break
		}
		den.Quo(den, two)
		twos++
	}
	for {
		rem.Mod(den, five)
		if rem.Sign() != 0 {
			break
		}
		den.Quo(den, five)
		fives++
	}
	if den.Cmp(big.NewInt(1)) != 0 {
		return value.FloatString(10)
	}
	scale := twos
	if fives > scale {
		scale = fives
	}
	if scale == 0 {
		return value.Num().String()
	}
	scaled := new(big.Int).Mul(value.Num(), new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil))
	scaled.Quo(scaled, value.Denom())
	negative := scaled.Sign() < 0
	if negative {
		scaled.Abs(scaled)
	}
	digits := scaled.String()
	for len(digits) <= scale {
		digits = "0" + digits
	}
	out := digits[:len(digits)-scale] + "." + digits[len(digits)-scale:]
	out = strings.TrimRight(out, "0")
	out = strings.TrimRight(out, ".")
	if out == "" {
		out = "0"
	}
	if negative {
		out = "-" + out
	}
	return out
}

func countEfundFunds(holdings []efund.Holding) int {
	codes := map[string]bool{}
	for _, holding := range holdings {
		codes[holding.FundCode] = true
	}
	return len(codes)
}

func sumEfundValue(holdings []efund.Holding) int64 {
	var total int64
	for _, holding := range holdings {
		total += holding.MarketValueCents
	}
	return total
}

func sumItems(items []domain.Item) int64 {
	var total int64
	for _, item := range items {
		total += item.MarketValueCents
	}
	return total
}

func topEfundFunds(holdings []efund.Holding) []string {
	type fund struct {
		name  string
		value int64
	}
	totals := map[string]int64{}
	order := []string{}
	for _, holding := range holdings {
		if _, ok := totals[holding.FundName]; !ok {
			order = append(order, holding.FundName)
		}
		totals[holding.FundName] += holding.MarketValueCents
	}
	funds := make([]fund, 0, len(order))
	for _, name := range order {
		funds = append(funds, fund{name: name, value: totals[name]})
	}
	sort.SliceStable(funds, func(i, j int) bool {
		if funds[i].value != funds[j].value {
			return funds[i].value > funds[j].value
		}
		return funds[i].name < funds[j].name
	})
	out := make([]string, 0, 3)
	for _, f := range funds {
		if len(out) == 3 {
			break
		}
		out = append(out, f.name)
	}
	return out
}

func efundSourceNote(meta efund.Meta, tradeAccounts []string) string {
	note := "基金E账户导出 份额日期 " + meta.ShareDate
	if meta.PrintTime != "" {
		note += "，打印时间 " + meta.PrintTime
	}
	if len(tradeAccounts) > 0 {
		note += "，交易账户 " + strings.Join(tradeAccounts, "、")
	}
	return note
}

func efundAuditDetail(meta efund.Meta) string {
	printTime := meta.PrintTime
	if printTime == "" {
		printTime = "未知"
	}
	return "来源 efund_import；份额日期 " + meta.ShareDate + "；文件打印时间 " + printTime
}

func newEfundToken() string {
	data := make([]byte, 16)
	if _, err := rand.Read(data); err != nil {
		panic(err)
	}
	return hex.EncodeToString(data)
}
