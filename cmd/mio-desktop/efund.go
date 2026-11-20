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
	token  string
	parsed *efund.Result
}

type EfundAccountOption struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Platform string             `json:"platform,omitempty"`
	Type     domain.AccountType `json:"type"`
	Currency string             `json:"currency"`
}

type EfundGroupPreview struct {
	SalesOrg         string          `json:"sales_org"`
	TradeAccount     string          `json:"trade_account"`
	Name             string          `json:"name"`
	Mode             string          `json:"mode"`
	AccountID        string          `json:"account_id,omitempty"`
	AccountName      string          `json:"account_name,omitempty"`
	Ignored          bool            `json:"ignored"`
	FundCount        int             `json:"fund_count"`
	MarketValueCents int64           `json:"market_value_cents"`
	TopFunds         []string        `json:"top_funds"`
	Holdings         []efund.Holding `json:"holdings"`
	Issue            string          `json:"issue,omitempty"`
}

type EfundTargetPreview struct {
	AccountID        string           `json:"account_id,omitempty"`
	AccountName      string           `json:"account_name"`
	NewAccount       bool             `json:"new_account"`
	Platform         string           `json:"platform,omitempty"`
	GroupCount       int              `json:"group_count"`
	FundCount        int              `json:"fund_count"`
	MarketValueCents int64            `json:"market_value_cents"`
	PreviousAsOf     string           `json:"previous_as_of,omitempty"`
	Diff             domain.Diff      `json:"diff"`
	Anomalies        []domain.Anomaly `json:"anomalies"`
	Holdings         []domain.Item    `json:"holdings"`
	AlreadyImported  bool             `json:"already_imported"`
	ImportedStatus   string           `json:"imported_status,omitempty"`
	mappingKeys      []string
	tradeAccounts    []string
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
	TotalHoldingCount     int                  `json:"total_holding_count"`
}

type EfundSelection struct {
	SalesOrg     string `json:"sales_org"`
	TradeAccount string `json:"trade_account"`
	Mode         string `json:"mode"`
	AccountID    string `json:"account_id,omitempty"`
	AccountName  string `json:"account_name,omitempty"`
}

type EfundImportAccountResult struct {
	AccountID        string   `json:"account_id,omitempty"`
	AccountName      string   `json:"account_name"`
	NewAccount       bool     `json:"new_account"`
	Status           string   `json:"status"`
	SnapshotID       string   `json:"snapshot_id,omitempty"`
	Anomalies        []string `json:"anomalies,omitempty"`
	Message          string   `json:"message,omitempty"`
	MarketValueCents int64    `json:"market_value_cents"`
}

type EfundImportResult struct {
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
	newAccount    bool
	platform      string
	groups        []EfundGroupPreview
	holdings      []efund.Holding
	mappingKeys   []string
	tradeAccounts []string
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
	s.session = &efundSession{token: token, parsed: parsed}
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
	accounts, err := s.st.ListAccounts(context.Background())
	if err != nil {
		return nil, err
	}
	mapping, err := s.loadMapping()
	if err != nil {
		return nil, err
	}
	valid := map[string]bool{}
	for _, h := range session.parsed.Holdings {
		valid[efund.MappingKey(h.SalesOrg, h.TradeAccount)] = true
	}
	for _, selection := range selections {
		key := efund.MappingKey(selection.SalesOrg, selection.TradeAccount)
		if !valid[key] {
			return nil, fmt.Errorf("未知的交易账户：%s / %s", selection.SalesOrg, selection.TradeAccount)
		}
		if err := applyEfundSelection(mapping, accounts, selection); err != nil {
			return nil, err
		}
	}
	if err := efund.WriteMapping(s.mappingPath, mapping); err != nil {
		return nil, err
	}
	preview, err := s.buildPreview(session.parsed, mapping, accounts)
	if err != nil {
		return nil, err
	}
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
	mapping, err := s.loadMapping()
	if err != nil {
		return nil, err
	}
	accounts, err := s.st.ListAccounts(context.Background())
	if err != nil {
		return nil, err
	}
	preview, err := s.buildPreview(session.parsed, mapping, accounts)
	if err != nil {
		return nil, err
	}
	preview.Token = token
	return preview, nil
}

func (s *EfundService) Import(token string) (*EfundImportResult, error) {
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
	mapping, err := s.loadMapping()
	if err != nil {
		return nil, err
	}
	preview, err := s.buildPreview(session.parsed, mapping, accounts)
	if err != nil {
		return nil, err
	}
	if !preview.CanImport {
		return nil, errors.New(preview.BlockReason)
	}

	result := &EfundImportResult{Results: []EfundImportAccountResult{}}
	mappingChanged := false
	for _, target := range preview.Targets {
		if target.AlreadyImported {
			result.Results = append(result.Results, EfundImportAccountResult{
				AccountID: target.AccountID, AccountName: target.AccountName,
				NewAccount: target.NewAccount, Status: "skipped",
				Message:          "已跳过：该账户在 " + session.parsed.Meta.ShareDate + " 已有 efund_import 快照（" + target.ImportedStatus + "）",
				MarketValueCents: target.MarketValueCents,
			})
			continue
		}
		accountID := target.AccountID
		if target.NewAccount {
			account, err := s.svc.UpsertAccount(ctx, domain.UpsertAccountInput{
				Name: target.AccountName, Platform: target.Platform,
				Type: domain.AccountFund, Currency: "CNY",
			})
			if err != nil {
				result.Results = append(result.Results, EfundImportAccountResult{
					AccountName: target.AccountName, NewAccount: true, Status: "failed",
					Message: err.Error(), MarketValueCents: target.MarketValueCents,
				})
				continue
			}
			accountID = account.ID
			for _, key := range target.mappingKeys {
				if entry := findMappingEntry(mapping, key); entry != nil {
					entry.AccountID = account.ID
					entry.Account = account.Name
					entry.Ignore = false
				}
			}
			mappingChanged = true
		}

		status, imported, err := s.importedStatus(ctx, accountID, session.parsed.Meta.ShareDate)
		if err != nil {
			result.Results = append(result.Results, EfundImportAccountResult{
				AccountID: accountID, AccountName: target.AccountName, NewAccount: target.NewAccount,
				Status: "failed", Message: err.Error(), MarketValueCents: target.MarketValueCents,
			})
			continue
		}
		if imported {
			result.Results = append(result.Results, EfundImportAccountResult{
				AccountID: accountID, AccountName: target.AccountName, NewAccount: target.NewAccount,
				Status: "skipped", Message: "已跳过：该账户在 " + session.parsed.Meta.ShareDate + " 已有 efund_import 快照（" + status + "）",
				MarketValueCents: target.MarketValueCents,
			})
			continue
		}

		submitted, err := s.svc.Submit(ctx, domain.SubmitInput{
			AccountID:   accountID,
			AsOf:        session.parsed.Meta.ShareDate,
			Source:      domain.SourceEfundImport,
			SourceNote:  efundSourceNote(session.parsed.Meta, target.tradeAccounts),
			Currency:    "CNY",
			Items:       target.Holdings,
			AuditDetail: efundAuditDetail(session.parsed.Meta),
		})
		if err != nil {
			result.Results = append(result.Results, EfundImportAccountResult{
				AccountID: accountID, AccountName: target.AccountName, NewAccount: target.NewAccount,
				Status: "failed", Message: err.Error(), MarketValueCents: target.MarketValueCents,
			})
			continue
		}
		_, confirmErr := s.svc.Confirm(ctx, submitted.SnapshotID)
		item := EfundImportAccountResult{
			AccountID: accountID, AccountName: target.AccountName, NewAccount: target.NewAccount,
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
	if mappingChanged {
		if err := efund.WriteMapping(s.mappingPath, mapping); err != nil {
			result.Warnings = append(result.Warnings, "映射文件写回失败："+err.Error())
		}
	}
	return result, nil
}

func (s *EfundService) Undo(accountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.svc.Undo(context.Background(), accountID)
	return err
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

func (s *EfundService) buildPreview(parsed *efund.Result, mapping *efund.Mapping, accounts []domain.Account) (*EfundPreview, error) {
	ctx := context.Background()
	byID := map[string]domain.Account{}
	byName := map[string][]domain.Account{}
	options := []EfundAccountOption{}
	for _, account := range accounts {
		byID[account.ID] = account
		if account.Archived {
			continue
		}
		byName[strings.ToLower(strings.TrimSpace(account.Name))] = append(
			byName[strings.ToLower(strings.TrimSpace(account.Name))], account)
		options = append(options, EfundAccountOption{
			ID: account.ID, Name: account.Name, Platform: account.Platform,
			Type: account.Type, Currency: account.Currency,
		})
	}
	sort.Slice(options, func(i, j int) bool { return options[i].Name < options[j].Name })

	entries := map[string]efund.MappingEntry{}
	for _, entry := range mapping.Mappings {
		entries[efund.MappingKey(entry.SalesOrg, entry.TradeAccount)] = entry
	}

	preview := &EfundPreview{
		FileName: parsed.File,
		Meta:     parsed.Meta,
		Warnings: append([]efund.Warning{}, parsed.Warnings...),
		Groups:   []EfundGroupPreview{},
		Targets:  []EfundTargetPreview{},
		Accounts: options,
	}
	groups := groupEfundHoldings(parsed.Holdings)
	targets := map[string]*efundTarget{}
	targetOrder := []string{}
	unresolved := 0

	for _, group := range groups {
		key := efund.MappingKey(group.salesOrg, group.tradeAccount)
		entry := entries[key]
		view := EfundGroupPreview{
			SalesOrg:     group.salesOrg,
			TradeAccount: group.tradeAccount,
			Mode:         efundModeUnmapped,
			Name:         group.salesOrg + "·" + group.tradeAccount + "·未映射",
			FundCount:    countEfundFunds(group.holdings),
			Holdings:     group.holdings,
			TopFunds:     topEfundFunds(group.holdings),
		}
		view.MarketValueCents = sumEfundValue(group.holdings)

		switch {
		case entry.Ignore:
			view.Mode = efundModeIgnore
			view.Ignored = true
			view.Name = group.salesOrg + "·" + group.tradeAccount + "·已忽略"
		case entry.AccountID != "":
			if account, ok := byID[entry.AccountID]; ok && !account.Archived {
				view.Mode = efundModeExisting
				view.AccountID = account.ID
				view.AccountName = account.Name
				view.Name = account.Name
			} else {
				view.Issue = "映射账户 ID 不存在或已归档，请重新选择"
				preview.Warnings = append(preview.Warnings, efund.Warning{
					Code: "mapping_account_missing", Severity: efund.SeverityWarning,
					Message: group.salesOrg + " / " + group.tradeAccount + "：" + view.Issue,
				})
			}
		case entry.Account != "":
			matches := byName[strings.ToLower(entry.Account)]
			switch len(matches) {
			case 1:
				view.Mode = efundModeExisting
				view.AccountID = matches[0].ID
				view.AccountName = matches[0].Name
				view.Name = matches[0].Name
			case 0:
				view.Mode = efundModeNew
				view.AccountName = entry.Account
				view.Name = entry.Account
			default:
				view.Issue = "库中有多个同名账户，请选择具体账户"
				preview.Warnings = append(preview.Warnings, efund.Warning{
					Code: "mapping_account_ambiguous", Severity: efund.SeverityWarning,
					Message: group.salesOrg + " / " + group.tradeAccount + "：" + view.Issue,
				})
			}
		}
		preview.Groups = append(preview.Groups, view)

		switch view.Mode {
		case efundModeExisting:
			targetKey := "id:" + view.AccountID
			target := targets[targetKey]
			if target == nil {
				target = &efundTarget{accountID: view.AccountID, accountName: view.AccountName}
				targets[targetKey] = target
				targetOrder = append(targetOrder, targetKey)
			}
			addEfundGroupToTarget(target, group, key)
		case efundModeNew:
			targetKey := "new:" + strings.ToLower(strings.TrimSpace(view.AccountName))
			target := targets[targetKey]
			if target == nil {
				target = &efundTarget{
					accountName: view.AccountName,
					newAccount:  true, platform: group.salesOrg,
				}
				targets[targetKey] = target
				targetOrder = append(targetOrder, targetKey)
			}
			addEfundGroupToTarget(target, group, key)
		case efundModeUnmapped:
			unresolved++
		}
	}

	for _, key := range targetOrder {
		target := targets[key]
		items, err := aggregateEfundItems(target.holdings)
		if err != nil {
			return nil, err
		}
		var previous *domain.Snapshot
		if target.accountID != "" {
			previous, err = s.st.LastConfirmedBefore(ctx, target.accountID, parsed.Meta.ShareDate)
			if err != nil {
				return nil, err
			}
		}
		account := domain.Account{
			ID: target.accountID, Name: target.accountName, Type: domain.AccountFund,
			Currency: "CNY", Platform: target.platform,
		}
		if existing, ok := byID[target.accountID]; ok {
			account = existing
		}
		next := &domain.Snapshot{
			AccountID: target.accountID, AsOf: parsed.Meta.ShareDate,
			Source: domain.SourceEfundImport, Currency: "CNY", Items: items,
		}
		diff := domain.ComputeDiff(previous, next)
		anomalies := domain.StructuredAnomalies(previous, next, &account, s.svc.Threshold())
		targetView := EfundTargetPreview{
			AccountID: target.accountID, AccountName: target.accountName,
			NewAccount: target.newAccount, Platform: target.platform,
			GroupCount: len(target.groups), FundCount: len(items),
			MarketValueCents: sumItems(items), Diff: diff, Anomalies: anomalies,
			Holdings: items, mappingKeys: target.mappingKeys,
			tradeAccounts: target.tradeAccounts,
		}
		if previous != nil {
			targetView.PreviousAsOf = previous.AsOf
		}
		if target.accountID != "" {
			status, imported, err := s.importedStatus(ctx, target.accountID, parsed.Meta.ShareDate)
			if err != nil {
				return nil, err
			}
			targetView.AlreadyImported = imported
			targetView.ImportedStatus = status
		}
		preview.Targets = append(preview.Targets, targetView)
		preview.TotalMarketValueCents += targetView.MarketValueCents
		preview.TotalHoldingCount += len(items)
	}

	var blocks []string
	for _, warning := range preview.Warnings {
		if warning.Severity == efund.SeverityBlocksWrite {
			blocks = append(blocks, warning.Message)
		}
	}
	switch {
	case len(blocks) > 0:
		preview.BlockedByData = true
		preview.BlockReason = "文件校验未通过，禁止写入：" + strings.Join(blocks, "；")
	case unresolved > 0:
		preview.BlockReason = fmt.Sprintf("还有 %d 个交易账户未决定映射", unresolved)
	case len(preview.Targets) == 0:
		preview.BlockReason = "没有要导入的组合"
	case parsed.Meta.ShareDate == "":
		preview.BlockReason = "文件没有可用的份额日期"
	default:
		preview.CanImport = true
	}
	return preview, nil
}

func (s *EfundService) importedStatus(ctx context.Context, accountID, asOf string) (string, bool, error) {
	confirmed, err := s.st.ListConfirmed(ctx, accountID, asOf, asOf)
	if err != nil {
		return "", false, err
	}
	for _, snapshot := range confirmed {
		if snapshot.Source == domain.SourceEfundImport {
			return "已确认", true, nil
		}
	}
	pending, err := s.st.ListPending(ctx)
	if err != nil {
		return "", false, err
	}
	for _, snapshot := range pending {
		if snapshot.AccountID == accountID && snapshot.AsOf == asOf && snapshot.Source == domain.SourceEfundImport {
			return "待确认", true, nil
		}
	}
	return "", false, nil
}

func applyEfundSelection(mapping *efund.Mapping, accounts []domain.Account, selection EfundSelection) error {
	key := efund.MappingKey(selection.SalesOrg, selection.TradeAccount)
	entry := findMappingEntry(mapping, key)
	if entry == nil {
		mapping.Mappings = append(mapping.Mappings, efund.MappingEntry{
			SalesOrg: selection.SalesOrg, TradeAccount: selection.TradeAccount,
		})
		entry = &mapping.Mappings[len(mapping.Mappings)-1]
	}
	switch selection.Mode {
	case efundModeIgnore:
		entry.Account = ""
		entry.AccountID = ""
		entry.Ignore = true
	case efundModeExisting:
		var account *domain.Account
		for i := range accounts {
			if accounts[i].ID == selection.AccountID && !accounts[i].Archived {
				account = &accounts[i]
				break
			}
		}
		if account == nil {
			return errors.New("选择的账户不存在或已归档")
		}
		entry.Account = account.Name
		entry.AccountID = account.ID
		entry.Ignore = false
	case efundModeNew:
		name := strings.TrimSpace(selection.AccountName)
		if name == "" {
			return errors.New("请输入新账户名称")
		}
		for _, account := range accounts {
			if !account.Archived && strings.EqualFold(strings.TrimSpace(account.Name), name) {
				return fmt.Errorf("账户名「%s」已存在，请选择已有账户", name)
			}
		}
		entry.Account = name
		entry.AccountID = ""
		entry.Ignore = false
	case efundModeUnmapped, "":
		entry.Account = ""
		entry.AccountID = ""
		entry.Ignore = false
	default:
		return fmt.Errorf("未知映射模式：%s", selection.Mode)
	}
	return nil
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
