package mcp

import (
	"context"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/curiselven/mio-money/internal/domain"
)

func NewServer(svc *domain.Service) *sdk.Server {
	server := sdk.NewServer(&sdk.Implementation{
		Name:    "mio-money",
		Version: "0.1.0",
	}, nil)

	sdk.AddTool(server, &sdk.Tool{
		Name:        "submit_snapshot",
		Description: "Store a pending snapshot of one account and return a diff preview. Nothing goes live until confirm_snapshot. Amounts are integer cents. quantity is a decimal string. Empty items means the position is fully closed.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in SubmitSnapshotInput) (*sdk.CallToolResult, domain.SubmitResult, error) {
		cash := in.CashCents
		res, err := svc.Submit(ctx, domain.SubmitInput{
			AccountID:  in.AccountID,
			AsOf:       in.AsOf,
			Source:     domain.Source(in.Source),
			SourceNote: in.SourceNote,
			Currency:   in.Currency,
			CashCents:  cash,
			Items:      in.Items,
		})
		if err != nil {
			return nil, domain.SubmitResult{}, err
		}
		return nil, *res, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "confirm_snapshot",
		Description: "Make a pending snapshot live. Refuses when an anomaly rule is hit (large total change, disappearing position, negative quantity/value, currency mismatch); the user must confirm on the desktop app.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in SnapshotIDInput) (*sdk.CallToolResult, domain.Snapshot, error) {
		snapshot, err := svc.Confirm(ctx, in.SnapshotID)
		if err != nil {
			return nil, domain.Snapshot{}, err
		}
		return nil, *snapshot, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "reject_snapshot",
		Description: "Discard a pending snapshot. The reason is stored in the audit log.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in RejectSnapshotInput) (*sdk.CallToolResult, domain.Snapshot, error) {
		snapshot, err := svc.Reject(ctx, in.SnapshotID, in.Reason)
		if err != nil {
			return nil, domain.Snapshot{}, err
		}
		return nil, *snapshot, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "undo_snapshot",
		Description: "Undo the most recently confirmed snapshot of an account. The snapshot becomes undone and the previous confirmed snapshot becomes current again.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in AccountIDInput) (*sdk.CallToolResult, domain.Snapshot, error) {
		snapshot, err := svc.Undo(ctx, in.AccountID)
		if err != nil {
			return nil, domain.Snapshot{}, err
		}
		return nil, *snapshot, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "upsert_account",
		Description: "Create an account (omit id) or update one by id. type is cash, fund, stock, mixed, liability or other. currency defaults to CNY. Liability balances are recorded as negative cash_cents in snapshots.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in domain.UpsertAccountInput) (*sdk.CallToolResult, domain.Account, error) {
		account, err := svc.UpsertAccount(ctx, in)
		if err != nil {
			return nil, domain.Account{}, err
		}
		return nil, *account, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "list_accounts",
		Description: "List all accounts with the date of each account's latest confirmed snapshot.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, _ EmptyInput) (*sdk.CallToolResult, ListAccountsOutput, error) {
		accounts, err := svc.ListAccounts(ctx)
		if err != nil {
			return nil, ListAccountsOutput{}, err
		}
		return nil, ListAccountsOutput{Accounts: accounts}, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "get_holdings",
		Description: "Current holdings from the latest confirmed snapshot, for one account or all accounts when account_id is omitted.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in OptionalAccountIDInput) (*sdk.CallToolResult, HoldingsOutput, error) {
		holdings, err := svc.Holdings(ctx, in.AccountID)
		if err != nil {
			return nil, HoldingsOutput{}, err
		}
		return nil, HoldingsOutput{Holdings: holdings}, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "get_net_worth",
		Description: "Net worth = assets - liabilities, grouped by account type and account. Uses each account's latest confirmed snapshot on or before as_of; omit as_of for the latest data. Liability values are reported as positive liabilities_cents.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in AsOfInput) (*sdk.CallToolResult, domain.NetWorth, error) {
		nw, err := svc.NetWorth(ctx, in.AsOf)
		if err != nil {
			return nil, domain.NetWorth{}, err
		}
		return nil, *nw, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "get_history",
		Description: "Time series of confirmed snapshot values. With account_id: that account's total per date. Without: net worth per date. from/to are optional YYYY-MM-DD bounds.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in HistoryInput) (*sdk.CallToolResult, domain.History, error) {
		history, err := svc.History(ctx, in.AccountID, in.From, in.To)
		if err != nil {
			return nil, domain.History{}, err
		}
		return nil, *history, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "list_pending",
		Description: "List snapshots waiting for confirmation.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, _ EmptyInput) (*sdk.CallToolResult, PendingOutput, error) {
		pending, err := svc.Pending(ctx)
		if err != nil {
			return nil, PendingOutput{}, err
		}
		return nil, PendingOutput{Snapshots: pending}, nil
	})

	return server
}
