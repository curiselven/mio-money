package efund

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkeletonOrderingAndHints(t *testing.T) {
	parsed, err := Parse(mappingBook(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	skeleton := Skeleton(parsed)
	if len(skeleton.Mappings) != 3 {
		t.Fatalf("mappings = %+v", skeleton.Mappings)
	}
	first := skeleton.Mappings[0]
	if first.TradeAccount != "T0000001" || first.Account != "" || first.AccountID != "" {
		t.Fatalf("first mapping = %+v", first)
	}
	if first.Hint == nil || first.Hint.FundCount != 3 || first.Hint.MarketValueYuan != "1210.00" {
		t.Fatalf("first hint = %+v", first.Hint)
	}
	wantTop := []string{"示例成长混合", "示例价值混合", "示例新基金"}
	if len(first.Hint.TopFunds) != len(wantTop) {
		t.Fatalf("top funds = %+v", first.Hint.TopFunds)
	}
	for i, name := range wantTop {
		if first.Hint.TopFunds[i] != name {
			t.Fatalf("top funds = %+v", first.Hint.TopFunds)
		}
	}
	if skeleton.Mappings[1].TradeAccount != "T0000002" || skeleton.Mappings[2].TradeAccount != "T0000009" {
		t.Fatalf("order = %+v", skeleton.Mappings)
	}
}

func TestMergeMappingPreservesChoicesAndWrites0600(t *testing.T) {
	parsed, err := Parse(mappingBook(t))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	generated := Skeleton(parsed)
	existing := &Mapping{Mappings: []MappingEntry{
		{
			SalesOrg: salesA, TradeAccount: "T0000001", Account: "长赢计划",
			AccountID: "acc_changying", Hint: &Hint{FundCount: 99},
		},
		{SalesOrg: "旧销售", TradeAccount: "T0000999", Account: "旧账户"},
	}}
	merged, added := MergeMapping(existing, generated)
	if added != 2 || len(merged.Mappings) != 4 {
		t.Fatalf("added=%d mappings=%+v", added, merged.Mappings)
	}
	first := merged.Mappings[0]
	if first.Account != "长赢计划" || first.AccountID != "acc_changying" || first.Hint.FundCount != 3 {
		t.Fatalf("merged first = %+v", first)
	}
	if merged.Mappings[3].Account != "旧账户" {
		t.Fatalf("stale mapping not preserved: %+v", merged.Mappings)
	}

	file := filepath.Join(t.TempDir(), "efund-map.json")
	if err := WriteMapping(file, &merged); err != nil {
		t.Fatalf("write mapping: %v", err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatalf("stat mapping: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mapping mode = %o", info.Mode().Perm())
	}
	loaded, err := LoadMapping(file)
	if err != nil {
		t.Fatalf("load mapping: %v", err)
	}
	if loaded.Mappings[0].AccountID != "acc_changying" || loaded.Mappings[2].Account != "" {
		t.Fatalf("loaded = %+v", loaded.Mappings)
	}
}
