package mcp_test

import (
	"context"
	"encoding/json"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/curiselven/mio-money/internal/domain"
	mioserver "github.com/curiselven/mio-money/internal/mcp"
	"github.com/curiselven/mio-money/internal/store"
)

func TestToolsRoundTrip(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open("file:mcp_smoke?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	server := mioserver.NewServer(domain.NewService(st, 0.2))
	serverTransport, clientTransport := sdk.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatal(err)
	}
	session, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"submit_snapshot", "confirm_snapshot", "reject_snapshot", "undo_snapshot", "upsert_account",
		"list_accounts", "get_holdings", "get_net_worth", "get_history", "list_pending",
	}
	if len(tools.Tools) != len(want) {
		t.Fatalf("got %d tools, want %d", len(tools.Tools), len(want))
	}
	names := map[string]bool{}
	for _, tool := range tools.Tools {
		names[tool.Name] = true
	}
	for _, name := range want {
		if !names[name] {
			t.Fatalf("missing tool %s", name)
		}
	}

	var account domain.Account
	call(t, session, "upsert_account", map[string]any{"name": "示例账户", "type": "fund"}, &account)
	if account.ID == "" {
		t.Fatal("account id is empty")
	}

	var submitted domain.SubmitResult
	call(t, session, "submit_snapshot", map[string]any{
		"account_id": account.ID,
		"as_of":      "2026-11-01",
		"source":     "mcp",
		"cash_cents": 100_000,
		"items": []map[string]any{
			{"name": "示例基金", "code": "000001", "quantity": "100", "market_value_cents": 200_000},
		},
	}, &submitted)
	if submitted.Status != domain.StatusPending {
		t.Fatalf("submit = %+v", submitted)
	}

	var confirmed domain.Snapshot
	call(t, session, "confirm_snapshot", map[string]any{"snapshot_id": submitted.SnapshotID}, &confirmed)
	if confirmed.Status != domain.StatusConfirmed {
		t.Fatalf("confirm = %+v", confirmed)
	}

	var nw domain.NetWorth
	call(t, session, "get_net_worth", map[string]any{"as_of": "2026-11-01"}, &nw)
	if nw.NetCents != 300_000 {
		t.Fatalf("net worth = %+v", nw)
	}
}

func call(t *testing.T, session *sdk.ClientSession, name string, args map[string]any, out any) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &sdk.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("%s: %v (raw %s)", name, err, raw)
	}
}
