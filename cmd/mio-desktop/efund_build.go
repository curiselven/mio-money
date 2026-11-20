package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/curiselven/mio-money/internal/domain"
	"github.com/curiselven/mio-money/internal/importer/efund"
)

type efundLeaf struct {
	leaf   domain.Account
	top    domain.Account
	direct bool
	issue  string
}

func (s *EfundService) buildPreview(parsed *efund.Result, mapping *efund.Mapping, accounts []domain.Account) (*EfundPreview, error) {
	ctx := context.Background()
	byID := map[string]domain.Account{}
	children := map[string][]domain.Account{}
	byName := map[string][]domain.Account{}
	options := []EfundAccountOption{}
	parentName := map[string]string{}
	for _, account := range accounts {
		byID[account.ID] = account
		if account.ParentID != "" && !account.Archived {
			children[account.ParentID] = append(children[account.ParentID], account)
		}
	}
	for _, account := range accounts {
		if account.ParentID != "" {
			parentName[account.ID] = byID[account.ParentID].Name
		}
		if account.Archived || !importableAccountType(account.Type) {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(account.Name))
		byName[key] = append(byName[key], account)
		options = append(options, EfundAccountOption{
			ID: account.ID, Name: account.Name, Platform: account.Platform,
			Type: account.Type, Currency: account.Currency,
			ParentID: account.ParentID, ParentName: parentName[account.ID],
			HasChildren: len(children[account.ID]) > 0,
		})
	}
	sort.SliceStable(options, func(i, j int) bool {
		if options[i].ParentID == "" && options[j].ParentID != "" {
			return true
		}
		if options[i].ParentID != "" && options[j].ParentID == "" {
			return false
		}
		if options[i].Name == options[j].Name {
			return options[i].ID < options[j].ID
		}
		return options[i].Name < options[j].Name
	})

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
	targets := map[string]*efundTarget{}
	targetOrder := []string{}
	unresolved := 0

	for _, group := range groupEfundHoldings(parsed.Holdings) {
		key := efund.MappingKey(group.salesOrg, group.tradeAccount)
		entry := entries[key]
		value := sumEfundValue(group.holdings)
		preview.FileTotalCents += value
		view := EfundGroupPreview{
			SalesOrg: group.salesOrg, TradeAccount: group.tradeAccount,
			Mode: efundModeUnmapped, FundCount: countEfundFunds(group.holdings),
			MarketValueCents: value, TopFunds: topEfundFunds(group.holdings),
			Holdings: group.holdings,
			Name:     group.salesOrg + "·" + group.tradeAccount + "·未映射",
		}
		switch {
		case entry.Ignore:
			view.Mode = efundModeIgnore
			view.Ignored = true
			view.Name = group.salesOrg + "·" + group.tradeAccount + "·已忽略"
			preview.IgnoredTotalCents += value
		case entry.AccountID == "" && entry.Account == "":
			unresolved++
			preview.UnmappedTotalCents += value
		default:
			resolved := resolveEfundLeaf(entry, byID, children, byName)
			if resolved.issue != "" {
				view.Issue = resolved.issue
				unresolved++
				preview.UnmappedTotalCents += value
				preview.Warnings = append(preview.Warnings, efund.Warning{
					Code: "mapping_target_invalid", Severity: efund.SeverityWarning,
					Message: group.salesOrg + " / " + group.tradeAccount + "：" + resolved.issue,
				})
				break
			}
			view.Mode = efundModeExisting
			view.AccountID = resolved.leaf.ID
			view.AccountName = resolved.leaf.Name
			view.TopAccountID = resolved.top.ID
			view.TopAccountName = resolved.top.Name
			view.PortfolioID = ""
			view.PortfolioName = ""
			view.Direct = resolved.direct
			if resolved.direct {
				view.Name = resolved.top.Name
			} else {
				view.PortfolioID = resolved.leaf.ID
				view.PortfolioName = resolved.leaf.Name
				view.Name = resolved.top.Name + " · " + resolved.leaf.Name
			}
			targetKey := "id:" + resolved.leaf.ID
			target := targets[targetKey]
			if target == nil {
				target = &efundTarget{accountID: resolved.leaf.ID, accountName: view.Name}
				targets[targetKey] = target
				targetOrder = append(targetOrder, targetKey)
			}
			addEfundGroupToTarget(target, group, key)
		}
		preview.Groups = append(preview.Groups, view)
	}

	for _, key := range targetOrder {
		target := targets[key]
		items, err := aggregateEfundItems(target.holdings)
		if err != nil {
			return nil, err
		}
		previous, err := s.st.LastConfirmedBefore(ctx, target.accountID, parsed.Meta.ShareDate)
		if err != nil {
			return nil, err
		}
		account := byID[target.accountID]
		cash := carriedCash(previous)
		next := &domain.Snapshot{
			AccountID: target.accountID, AsOf: parsed.Meta.ShareDate,
			Source: domain.SourceEfundImport, Currency: "CNY", CashCents: cash, Items: items,
		}
		existing, err := s.conflictSnapshot(ctx, target.accountID, parsed.Meta.ShareDate)
		if err != nil {
			return nil, err
		}
		existingID, existingSource, status := efundSnapshotFields(existing)
		targetView := EfundTargetPreview{
			Key: "id:" + target.accountID, AccountID: target.accountID,
			AccountName: target.accountName, GroupCount: len(target.groups),
			FundCount: len(items), MarketValueCents: sumItems(items),
			PreviousAsOf: previousAsOf(previous), Diff: domain.ComputeDiff(previous, next),
			Anomalies: domain.StructuredAnomalies(previous, next, &account, s.svc.Threshold()),
			Holdings:  items, AlreadyImported: existing != nil, ImportedStatus: status,
			ExistingSnapshotID: existingID, ExistingSnapshotSource: existingSource,
			CashCents: cash, mappingKeys: target.mappingKeys, tradeAccounts: target.tradeAccounts,
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

func resolveEfundLeaf(entry efund.MappingEntry, byID map[string]domain.Account, children map[string][]domain.Account, byName map[string][]domain.Account) efundLeaf {
	if entry.AccountID != "" {
		return resolveEfundLeafID(entry.AccountID, byID, children)
	}
	matches := byName[strings.ToLower(strings.TrimSpace(entry.Account))]
	importable := make([]domain.Account, 0, len(matches))
	for _, account := range matches {
		if len(children[account.ID]) == 0 {
			importable = append(importable, account)
		}
	}
	switch len(importable) {
	case 0:
		return efundLeaf{issue: "旧映射找不到可导入的账户或组合，请重新选择"}
	case 1:
		return resolveEfundLeafID(importable[0].ID, byID, children)
	default:
		return efundLeaf{issue: "存在多个同名账户，请按账户 ID 重新选择"}
	}
}

func resolveEfundLeafID(id string, byID map[string]domain.Account, children map[string][]domain.Account) efundLeaf {
	leaf, ok := byID[id]
	if !ok || leaf.Archived {
		return efundLeaf{issue: "目标账户不存在或已归档"}
	}
	if !importableAccountType(leaf.Type) {
		return efundLeaf{issue: "目标账户类型必须是 fund 或 mixed"}
	}
	if len(children[leaf.ID]) > 0 {
		return efundLeaf{issue: "该账户下有组合，不能直接导入；请选择具体组合"}
	}
	if leaf.ParentID == "" {
		return efundLeaf{leaf: leaf, top: leaf, direct: true}
	}
	top, ok := byID[leaf.ParentID]
	if !ok || top.Archived {
		return efundLeaf{issue: "组合的上级账户不存在或已归档"}
	}
	return efundLeaf{leaf: leaf, top: top}
}

func applyEfundSelection(ctx context.Context, svc *domain.Service, mapping *efund.Mapping, accounts []domain.Account, selection EfundSelection) ([]domain.Account, error) {
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
		entry.Account, entry.AccountID, entry.ParentAccount = "", "", ""
		entry.Ignore = true
		return accounts, nil
	case efundModeUnmapped, "":
		entry.Account, entry.AccountID, entry.ParentAccount = "", "", ""
		entry.Ignore = false
		return accounts, nil
	case "map", efundModeExisting, efundModeNew:
	default:
		return accounts, fmt.Errorf("未知映射模式：%s", selection.Mode)
	}

	created := []domain.Account{}
	fail := func(err error) ([]domain.Account, error) {
		for i := len(created) - 1; i >= 0; i-- {
			archived := true
			_, _ = svc.UpsertAccount(ctx, domain.UpsertAccountInput{ID: created[i].ID, Archived: &archived})
		}
		return accounts, err
	}
	var top domain.Account
	if selection.AccountID != "" {
		found := false
		for _, account := range accounts {
			if account.ID == selection.AccountID && !account.Archived {
				top, found = account, true
				break
			}
		}
		if !found || top.ParentID != "" {
			return fail(errors.New("请选择顶层账户"))
		}
		if !importableAccountType(top.Type) {
			return fail(errors.New("目标账户类型必须是 fund 或 mixed"))
		}
	} else {
		name := strings.TrimSpace(selection.AccountName)
		if name == "" {
			return fail(errors.New("请输入新账户名称"))
		}
		for _, account := range accounts {
			if !account.Archived && account.ParentID == "" && strings.EqualFold(strings.TrimSpace(account.Name), name) {
				return fail(fmt.Errorf("账户名「%s」已存在，请选择已有账户", name))
			}
		}
		account, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{
			Name: name, Type: domain.AccountFund, Currency: "CNY",
		})
		if err != nil {
			return fail(err)
		}
		top = *account
		created = append(created, *account)
		accounts = append(accounts, *account)
	}

	var leaf domain.Account
	switch {
	case selection.PortfolioID != "":
		found := false
		for _, account := range accounts {
			if account.ID == selection.PortfolioID && account.ParentID == top.ID && !account.Archived {
				leaf, found = account, true
				break
			}
		}
		if !found || !importableAccountType(leaf.Type) {
			return fail(errors.New("请选择该账户下可导入的组合"))
		}
	case selection.Direct:
		for _, account := range accounts {
			if account.ParentID == top.ID && !account.Archived {
				return fail(errors.New("该账户已有组合，不能直接记在账户上"))
			}
		}
		leaf = top
	default:
		name := strings.TrimSpace(selection.PortfolioName)
		if name == "" {
			return fail(errors.New("请输入组合名称"))
		}
		for _, account := range accounts {
			if account.ParentID == top.ID && !account.Archived && strings.EqualFold(strings.TrimSpace(account.Name), name) {
				return fail(fmt.Errorf("组合「%s」已存在，请选择已有组合", name))
			}
		}
		child, err := svc.UpsertAccount(ctx, domain.UpsertAccountInput{
			Name: name, ParentID: top.ID,
		})
		if err != nil {
			return fail(err)
		}
		leaf = *child
		created = append(created, *child)
		accounts = append(accounts, *child)
	}
	entry.AccountID = leaf.ID
	entry.Account = leaf.Name
	entry.ParentAccount = top.Name
	entry.Ignore = false
	return accounts, nil
}

func importableAccountType(t domain.AccountType) bool {
	return t == domain.AccountFund || t == domain.AccountMixed
}

func carriedCash(previous *domain.Snapshot) *int64 {
	if previous == nil || previous.CashCents == nil || *previous.CashCents == 0 {
		return nil
	}
	cash := *previous.CashCents
	return &cash
}

func previousAsOf(previous *domain.Snapshot) string {
	if previous == nil {
		return ""
	}
	return previous.AsOf
}

func cloneMapping(mapping *efund.Mapping) *efund.Mapping {
	if mapping == nil {
		return &efund.Mapping{Mappings: []efund.MappingEntry{}}
	}
	out := &efund.Mapping{Mappings: make([]efund.MappingEntry, len(mapping.Mappings))}
	copy(out.Mappings, mapping.Mappings)
	for i := range out.Mappings {
		if mapping.Mappings[i].Hint != nil {
			hint := *mapping.Mappings[i].Hint
			out.Mappings[i].Hint = &hint
		}
	}
	return out
}

func mappingEqual(left, right *efund.Mapping) bool {
	if left == nil || right == nil {
		return left == right
	}
	if len(left.Mappings) != len(right.Mappings) {
		return false
	}
	index := make(map[string]efund.MappingEntry, len(right.Mappings))
	for _, entry := range right.Mappings {
		index[efund.MappingKey(entry.SalesOrg, entry.TradeAccount)] = entry
	}
	for _, entry := range left.Mappings {
		other, ok := index[efund.MappingKey(entry.SalesOrg, entry.TradeAccount)]
		if !ok || entry.Account != other.Account || entry.AccountID != other.AccountID ||
			entry.ParentAccount != other.ParentAccount || entry.Ignore != other.Ignore {
			return false
		}
	}
	return true
}

func summarizeImportResult(result *EfundImportResult) {
	for _, item := range result.Results {
		switch item.Status {
		case "applied":
			result.Applied++
		case "pending":
			result.Pending++
		case "skipped":
			result.Skipped++
		default:
			result.Failed++
		}
	}
	total := len(result.Results)
	switch {
	case result.Failed == total && total > 0:
		result.Summary = "全部失败"
	case result.Failed > 0:
		result.Summary = "部分失败"
	case result.Skipped == total && total > 0:
		result.Summary = "全部之前已导入"
	case result.Skipped > 0:
		result.Summary = "部分成功，部分之前已导入"
	case result.Pending > 0:
		result.Summary = fmt.Sprintf("全部成功，其中 %d 个待确认", result.Pending)
	default:
		result.Summary = "全部成功"
	}
}
