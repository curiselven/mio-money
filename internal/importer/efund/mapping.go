package efund

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type MappingEntry struct {
	SalesOrg      string `json:"sales_org"`
	TradeAccount  string `json:"trade_account"`
	Account       string `json:"account"`
	AccountID     string `json:"account_id"`
	ParentAccount string `json:"parent_account,omitempty"`
	Ignore        bool   `json:"ignore,omitempty"`
	Hint          *Hint  `json:"hint,omitempty"`
}

type Mapping struct {
	Mappings []MappingEntry `json:"mappings"`
}

// DefaultMappingPath keeps the private mapping next to the database.
func DefaultMappingPath(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "efund-map.json")
}

func LoadMapping(file string) (*Mapping, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read mapping: %w", err)
	}
	var m Mapping
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse mapping: %w", err)
	}
	seen := map[string]bool{}
	for i := range m.Mappings {
		entry := &m.Mappings[i]
		entry.SalesOrg = strings.TrimSpace(entry.SalesOrg)
		entry.TradeAccount = strings.TrimSpace(entry.TradeAccount)
		entry.Account = strings.TrimSpace(entry.Account)
		entry.AccountID = strings.TrimSpace(entry.AccountID)
		entry.ParentAccount = strings.TrimSpace(entry.ParentAccount)
		if entry.SalesOrg == "" || entry.TradeAccount == "" {
			return nil, fmt.Errorf("mapping[%d]: sales_org and trade_account are required", i)
		}
		key := mappingKey(entry.SalesOrg, entry.TradeAccount)
		if seen[key] {
			return nil, fmt.Errorf("mapping[%d]: duplicate sales_org + trade_account", i)
		}
		seen[key] = true
	}
	return &m, nil
}

func mappingKey(salesOrg, tradeAccount string) string {
	return strings.TrimSpace(salesOrg) + "\x00" + strings.TrimSpace(tradeAccount)
}

func MappingKey(salesOrg, tradeAccount string) string {
	return mappingKey(salesOrg, tradeAccount)
}
