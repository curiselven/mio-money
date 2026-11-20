package efund

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExampleMappingIsValid(t *testing.T) {
	m, err := LoadMapping(filepath.Join("..", "..", "..", "examples", "efund-map.example.json"))
	if err != nil {
		t.Fatalf("load example: %v", err)
	}
	if len(m.Mappings) != 3 || m.Mappings[0].Account == "" {
		t.Fatalf("example mapping = %+v", m)
	}
	if got := DefaultMappingPath("/data/MioMoney/mio.db"); got != "/data/MioMoney/efund-map.json" {
		t.Fatalf("default mapping path = %s", got)
	}
}

func TestLoadMappingRejectsDuplicateKey(t *testing.T) {
	file := filepath.Join(t.TempDir(), "map.json")
	body := `{"mappings":[
		{"sales_org":"示例销售","trade_account":"T1","account":"A"},
		{"sales_org":"示例销售","trade_account":"T1","account":"B"}
	]}`
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatalf("write mapping: %v", err)
	}
	if _, err := LoadMapping(file); err == nil {
		t.Fatal("duplicate mapping key was accepted")
	}
}
