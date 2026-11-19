package mcp_test

import (
	"context"
	"encoding/json"
	"strings"
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
		"set_loan_terms", "get_loan", "set_loan_history", "get_loan_history",
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
	for _, name := range tools.Tools {
		lower := strings.ToLower(name.Name)
		if strings.Contains(lower, "approve") || strings.Contains(lower, "override") || strings.Contains(lower, "force") {
			t.Fatalf("desktop override tool must not be exposed over MCP: %s", name.Name)
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

	var liability domain.Account
	call(t, session, "upsert_account", map[string]any{"name": "示例房贷", "type": "liability"}, &liability)
	var terms domain.LoanTerms
	call(t, session, "set_loan_terms", map[string]any{
		"account_id":             liability.ID,
		"anchor_date":            "2026-01-01",
		"anchor_principal_cents": 100_000_000,
		"annual_rate":            "3.10",
		"method":                 "equal_installment",
		"payment_day":            20,
		"remaining_periods":      240,
	}, &terms)
	if terms.AccountID != liability.ID || terms.Method != domain.LoanEqualInstallment {
		t.Fatalf("terms = %+v", terms)
	}
	var loan domain.LoanView
	call(t, session, "get_loan", map[string]any{"account_id": liability.ID, "schedule": true}, &loan)
	if loan.BalanceCents <= 0 || loan.RemainingPeriods <= 0 || len(loan.Schedule) != 240 || loan.Next == nil {
		t.Fatalf("loan = %+v", loan)
	}

	var history domain.LoanHistory
	call(t, session, "set_loan_history", map[string]any{
		"account_id":               liability.ID,
		"original_principal_cents": 100_000_000,
		"first_payment_date":       "2026-01-20",
		"total_periods":            12,
		"method":                   "equal_installment",
		"initial_annual_rate":      "4.90",
		"rate_changes":             []map[string]any{{"date": "2026-06-25", "annual_rate": "5.00"}},
	}, &history)
	if history.TotalPeriods != 12 || len(history.RateChanges) != 1 {
		t.Fatalf("history = %+v", history)
	}
	var historyView domain.LoanHistoryView
	call(t, session, "get_loan_history", map[string]any{"account_id": liability.ID, "schedule": true}, &historyView)
	if len(historyView.Periods) != 12 || historyView.PaidPeriods == 0 {
		t.Fatalf("history view = %+v", historyView.LoanCalculation)
	}
	var sliced domain.LoanHistoryView
	call(t, session, "get_loan_history", map[string]any{"account_id": liability.ID, "from": 2, "to": 3}, &sliced)
	if len(sliced.Periods) != 2 || sliced.Periods[0].Number != 2 {
		t.Fatalf("sliced = %+v", sliced.Periods)
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
