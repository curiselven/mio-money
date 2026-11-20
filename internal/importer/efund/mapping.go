package efund

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type MappingEntry struct {
	SalesOrg     string `json:"sales_org"`
	TradeAccount string `json:"trade_account"`
	Account      string `json:"account"`
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
		if entry.SalesOrg == "" || entry.TradeAccount == "" || entry.Account == "" {
			return nil, fmt.Errorf("mapping[%d]: sales_org, trade_account and account are required", i)
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
