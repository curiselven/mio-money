package efund

import (
	"strings"

	"github.com/curiselven/mio-money/internal/domain"
)

// Target is the account that a mapping entry will write to.
type Target struct {
	Leaf   domain.Account
	Top    domain.Account
	Direct bool
	Issue  string
}

func ImportableAccountType(t domain.AccountType) bool {
	return t == domain.AccountFund || t == domain.AccountMixed
}

// ResolveTarget applies the same target rules to CLI previews and desktop imports.
func ResolveTarget(entry MappingEntry, accounts []domain.Account) Target {
	byID := map[string]domain.Account{}
	children := map[string][]domain.Account{}
	importableByName := map[string][]domain.Account{}
	for _, account := range accounts {
		byID[account.ID] = account
		if account.Archived {
			continue
		}
		if account.ParentID != "" {
			children[account.ParentID] = append(children[account.ParentID], account)
		}
		if ImportableAccountType(account.Type) {
			key := strings.ToLower(strings.TrimSpace(account.Name))
			importableByName[key] = append(importableByName[key], account)
		}
	}
	if entry.AccountID != "" {
		return resolveTargetID(entry.AccountID, byID, children)
	}
	matches := importableByName[strings.ToLower(strings.TrimSpace(entry.Account))]
	importable := make([]domain.Account, 0, len(matches))
	for _, account := range matches {
		if len(children[account.ID]) == 0 {
			importable = append(importable, account)
		}
	}
	switch len(importable) {
	case 0:
		return Target{Issue: "旧映射找不到可导入的账户或组合，请重新选择"}
	case 1:
		return resolveTargetID(importable[0].ID, byID, children)
	default:
		return Target{Issue: "存在多个同名账户，请按账户 ID 重新选择"}
	}
}

func resolveTargetID(id string, byID map[string]domain.Account, children map[string][]domain.Account) Target {
	leaf, ok := byID[id]
	if !ok || leaf.Archived {
		return Target{Issue: "目标账户不存在或已归档"}
	}
	if !ImportableAccountType(leaf.Type) {
		return Target{Issue: "目标账户类型必须是 fund 或 mixed"}
	}
	if len(children[leaf.ID]) > 0 {
		return Target{Issue: "该账户下有组合，不能直接导入；请选择具体组合"}
	}
	if leaf.ParentID == "" {
		return Target{Leaf: leaf, Top: leaf, Direct: true}
	}
	top, ok := byID[leaf.ParentID]
	if !ok || top.Archived {
		return Target{Issue: "组合的上级账户不存在或已归档"}
	}
	return Target{Leaf: leaf, Top: top}
}
