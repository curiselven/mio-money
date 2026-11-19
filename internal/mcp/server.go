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

	sdk.AddTool(server, &sdk.Tool{
		Name:        "set_loan_terms",
		Description: "Set or reset the repayment terms of a liability account. Use the current remaining principal to model a rate change or prepayment. If anchor_date has no snapshot yet, a confirmed snapshot with cash_cents = -anchor_principal_cents (source loan_schedule) is written. Amounts are integer cents; annual_rate is a decimal string like \"3.10\".",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in SetLoanTermsInput) (*sdk.CallToolResult, domain.LoanTerms, error) {
		terms, err := svc.SetLoanTerms(ctx, domain.SetLoanTermsInput{
			AccountID:            in.AccountID,
			AnchorDate:           in.AnchorDate,
			AnchorPrincipalCents: in.AnchorPrincipalCents,
			AnnualRate:           in.AnnualRate,
			Method:               domain.LoanMethod(in.Method),
			PaymentDay:           in.PaymentDay,
			RemainingPeriods:     in.RemainingPeriods,
		})
		if err != nil {
			return nil, domain.LoanTerms{}, err
		}
		return nil, *terms, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "get_loan",
		Description: "Repayment terms plus the computed state as of today: remaining balance, remaining periods, next payment (date, payment, principal, interest) and remaining total interest. Pass schedule:true for every future period.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in GetLoanInput) (*sdk.CallToolResult, domain.LoanView, error) {
		view, err := svc.Loan(ctx, in.AccountID, in.Schedule)
		if err != nil {
			return nil, domain.LoanView{}, err
		}
		return nil, *view, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "set_loan_history",
		Description: "Replace the account's plan with a full-history plan replayed from period 1 like a bank: initial principal, first payment date, total periods, method, initial annual rate, rate changes and optional prepayments. A rate change applies from the first payment strictly after its effective date. This removes any anchor-mode terms for the account; existing snapshots are kept.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in SetLoanHistoryInput) (*sdk.CallToolResult, domain.LoanHistory, error) {
		changes := make([]domain.LoanRateChange, len(in.RateChanges))
		for i, c := range in.RateChanges {
			changes[i] = domain.LoanRateChange{Date: c.Date, AnnualRate: c.AnnualRate}
		}
		prepayments := make([]domain.LoanPrepayment, len(in.Prepayments))
		for i, p := range in.Prepayments {
			prepayments[i] = domain.LoanPrepayment{Date: p.Date, AmountCents: p.AmountCents, Mode: domain.PrepaymentMode(p.Mode)}
		}
		history, err := svc.SetLoanHistory(ctx, domain.SetLoanHistoryInput{
			AccountID:              in.AccountID,
			OriginalPrincipalCents: in.OriginalPrincipalCents,
			FirstPaymentDate:       in.FirstPaymentDate,
			TotalPeriods:           in.TotalPeriods,
			Method:                 domain.LoanMethod(in.Method),
			InitialAnnualRate:      in.InitialAnnualRate,
			RateChanges:            changes,
			Prepayments:            prepayments,
		})
		if err != nil {
			return nil, domain.LoanHistory{}, err
		}
		return nil, *history, nil
	})

	sdk.AddTool(server, &sdk.Tool{
		Name:        "get_loan_history",
		Description: "Full-history loan state as of today: paid periods, balance, next payment and remaining interest, plus the period detail. Pass from/to (1-based) for a slice or schedule:true for all periods.",
	}, func(ctx context.Context, _ *sdk.CallToolRequest, in GetLoanHistoryInput) (*sdk.CallToolResult, domain.LoanHistoryView, error) {
		view, err := svc.LoanHistory(ctx, in.AccountID)
		if err != nil {
			return nil, domain.LoanHistoryView{}, err
		}
		if in.From > 0 || in.To > 0 {
			from := in.From
			if from < 1 {
				from = 1
			}
			to := in.To
			if to <= 0 || to > len(view.Periods) {
				to = len(view.Periods)
			}
			if from <= to {
				view.Periods = view.Periods[from-1 : to]
			} else {
				view.Periods = nil
			}
		} else if !in.Schedule {
			view.Periods = nil
		}
		return nil, *view, nil
	})

	return server
}
