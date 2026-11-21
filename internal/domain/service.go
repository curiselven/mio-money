package domain

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var ErrAnomaly = errors.New("anomaly")

var quantityPattern = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

type Repo interface {
	GetAccount(ctx context.Context, id string) (*Account, error)
	UpsertAccount(ctx context.Context, a *Account) error
	ListAccounts(ctx context.Context) ([]Account, error)
	LastConfirmedDates(ctx context.Context) (map[string]string, error)

	InsertSnapshot(ctx context.Context, s *Snapshot) error
	GetSnapshot(ctx context.Context, id string) (*Snapshot, error)
	UpdateSnapshotStatus(ctx context.Context, id string, status SnapshotStatus, confirmedAt *time.Time) error
	LastConfirmedBefore(ctx context.Context, accountID, date string) (*Snapshot, error)
	LastConfirmed(ctx context.Context, accountID string) (*Snapshot, error)
	LatestConfirmedPerAccount(ctx context.Context, asOf string) ([]*Snapshot, error)
	ListConfirmed(ctx context.Context, accountID, from, to string) ([]*Snapshot, error)
	ListPending(ctx context.Context) ([]*Snapshot, error)

	AddAudit(ctx context.Context, e AuditEntry) error

	GetLoan(ctx context.Context, accountID string) (*LoanTerms, error)
	ListLoans(ctx context.Context) ([]LoanTerms, error)
	UpsertLoan(ctx context.Context, t *LoanTerms) error
	DeleteLoan(ctx context.Context, accountID string) error
	GetLoanHistory(ctx context.Context, accountID string) (*LoanHistory, error)
	ListLoanHistories(ctx context.Context) ([]LoanHistory, error)
	UpsertLoanHistory(ctx context.Context, h *LoanHistory) error
	DeleteLoanHistory(ctx context.Context, accountID string) error
	HasSnapshotAt(ctx context.Context, accountID, asOf string) (bool, error)
	HasChildren(ctx context.Context, accountID string) (bool, error)
	HasAnySnapshot(ctx context.Context, accountID string) (bool, error)
}

type Service struct {
	repo      Repo
	threshold float64 // fraction, e.g. 0.2
	now       func() time.Time
}

func NewService(repo Repo, threshold float64) *Service {
	return &Service{repo: repo, threshold: threshold, now: time.Now}
}

func (s *Service) Threshold() float64 { return s.threshold }

func validDate(v string) error {
	if _, err := time.Parse("2006-01-02", v); err != nil {
		return fmt.Errorf("%q is not a valid date (YYYY-MM-DD)", v)
	}
	return nil
}

type SubmitInput struct {
	AccountID   string
	AsOf        string
	Source      Source
	SourceNote  string
	Currency    string
	CashCents   *int64
	Items       []Item
	AuditDetail string
}

type SubmitResult struct {
	SnapshotID string         `json:"snapshot_id"`
	Status     SnapshotStatus `json:"status"`
	Diff       Diff           `json:"diff"`
	Anomalies  []string       `json:"anomalies"`
}

// Submit stores a pending snapshot plus its preview. Pending data never
// affects holdings or net worth.
func (s *Service) Submit(ctx context.Context, in SubmitInput) (*SubmitResult, error) {
	if in.AsOf == "" {
		return nil, errors.New("as_of is required (YYYY-MM-DD)")
	}
	if err := validDate(in.AsOf); err != nil {
		return nil, fmt.Errorf("as_of: %w", err)
	}
	source := in.Source
	if source == "" {
		source = SourceMCP
	}
	if source == SourceLoan || !source.Valid() {
		return nil, fmt.Errorf("source %q must be one of screenshot, mcp, statement, manual, efund_import", source)
	}
	account, err := s.repo.GetAccount(ctx, in.AccountID)
	if err != nil {
		return nil, err
	}
	if account.Archived {
		return nil, errors.New("账户已归档，请先恢复后再写入")
	}
	if children, err := s.repo.HasChildren(ctx, account.ID); err != nil {
		return nil, err
	} else if children {
		return nil, errors.New("该账户下有组合，快照必须写在具体组合上")
	}
	currency := strings.ToUpper(in.Currency)
	if currency == "" {
		currency = account.Currency
	}
	items := make([]Item, len(in.Items))
	for i, it := range in.Items {
		if strings.TrimSpace(it.Name) == "" {
			return nil, fmt.Errorf("items[%d].name is required", i)
		}
		if it.Quantity != "" && !quantityPattern.MatchString(it.Quantity) {
			return nil, fmt.Errorf("items[%d].quantity %q must be a plain decimal string", i, it.Quantity)
		}
		items[i] = it
	}

	prev, err := s.repo.LastConfirmedBefore(ctx, account.ID, in.AsOf)
	if err != nil {
		return nil, err
	}
	snapshot := &Snapshot{
		ID:         newID("snp"),
		AccountID:  account.ID,
		AsOf:       in.AsOf,
		Status:     StatusPending,
		Source:     source,
		SourceNote: in.SourceNote,
		Currency:   currency,
		CashCents:  in.CashCents,
		Items:      items,
		CreatedAt:  s.now().UTC(),
	}
	if err := s.repo.InsertSnapshot(ctx, snapshot); err != nil {
		return nil, err
	}
	if err := s.audit(ctx, "submit", snapshot, in.AuditDetail); err != nil {
		return nil, err
	}
	return &SubmitResult{
		SnapshotID: snapshot.ID,
		Status:     StatusPending,
		Diff:       ComputeDiff(prev, snapshot),
		Anomalies:  Anomalies(prev, snapshot, account, s.threshold),
	}, nil
}

// Confirm makes a pending snapshot live. Any anomaly blocks it.
func (s *Service) Confirm(ctx context.Context, snapshotID string) (*Snapshot, error) {
	return s.confirm(ctx, snapshotID, false)
}

// ApproveAnomalous makes a pending snapshot live from the desktop UI, bypassing
// anomaly rules but keeping every other Confirm check.
func (s *Service) ApproveAnomalous(ctx context.Context, snapshotID string) (*Snapshot, error) {
	return s.confirm(ctx, snapshotID, true)
}

func (s *Service) confirm(ctx context.Context, snapshotID string, allowAnomalies bool) (*Snapshot, error) {
	snapshot, err := s.repo.GetSnapshot(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	if snapshot.Status != StatusPending {
		return nil, fmt.Errorf("snapshot %s is %s, only pending snapshots can be confirmed", snapshotID, snapshot.Status)
	}
	account, err := s.repo.GetAccount(ctx, snapshot.AccountID)
	if err != nil {
		return nil, err
	}
	if account.Archived {
		return nil, errors.New("账户已归档，请先恢复后再确认")
	}
	prev, err := s.repo.LastConfirmedBefore(ctx, snapshot.AccountID, snapshot.AsOf)
	if err != nil {
		return nil, err
	}
	anomalies := StructuredAnomalies(prev, snapshot, account, s.threshold)
	if len(anomalies) > 0 && !allowAnomalies {
		return nil, fmt.Errorf("%w: %s", ErrAnomaly, strings.Join(anomalyMessages(anomalies), "；"))
	}
	now := s.now().UTC()
	if err := s.repo.UpdateSnapshotStatus(ctx, snapshot.ID, StatusConfirmed, &now); err != nil {
		return nil, err
	}
	snapshot.Status = StatusConfirmed
	snapshot.ConfirmedAt = &now
	action := "confirm"
	detail := ""
	if allowAnomalies && len(anomalies) > 0 {
		action = "confirm_override"
		detail = "桌面端放行：" + strings.Join(anomalyMessages(anomalies), "；")
	}
	if err := s.audit(ctx, action, snapshot, detail); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (s *Service) Reject(ctx context.Context, snapshotID, reason string) (*Snapshot, error) {
	snapshot, err := s.repo.GetSnapshot(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	if snapshot.Status != StatusPending {
		return nil, fmt.Errorf("snapshot %s is %s, only pending snapshots can be rejected", snapshotID, snapshot.Status)
	}
	if err := s.repo.UpdateSnapshotStatus(ctx, snapshot.ID, StatusRejected, nil); err != nil {
		return nil, err
	}
	snapshot.Status = StatusRejected
	if err := s.audit(ctx, "reject", snapshot, reason); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// Undo reverts the account's most recently confirmed snapshot.
func (s *Service) Undo(ctx context.Context, accountID string) (*Snapshot, error) {
	snapshot, err := s.repo.LastConfirmed(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return nil, fmt.Errorf("account %s has no confirmed snapshot to undo", accountID)
	}
	if err := s.repo.UpdateSnapshotStatus(ctx, snapshot.ID, StatusUndone, snapshot.ConfirmedAt); err != nil {
		return nil, err
	}
	snapshot.Status = StatusUndone
	if err := s.audit(ctx, "undo", snapshot, ""); err != nil {
		return nil, err
	}
	return snapshot, nil
}

// UndoSnapshot undoes one confirmed efund_import snapshot only when it is the
// account's current confirmed snapshot.
func (s *Service) UndoSnapshot(ctx context.Context, snapshotID string) (*Snapshot, error) {
	snapshot, err := s.repo.GetSnapshot(ctx, snapshotID)
	if err != nil {
		return nil, err
	}
	if snapshot.Source != SourceEfundImport {
		return nil, errors.New("只能撤销基金E账户导入生成的快照")
	}
	if snapshot.Status != StatusConfirmed {
		return nil, fmt.Errorf("快照当前状态是 %s，不能撤销", snapshot.Status)
	}
	latest, err := s.repo.LastConfirmed(ctx, snapshot.AccountID)
	if err != nil {
		return nil, err
	}
	if latest == nil || latest.ID != snapshotID {
		return nil, errors.New("该快照不是账户当前最新的已确认快照，不能撤销")
	}
	if err := s.repo.UpdateSnapshotStatus(ctx, snapshot.ID, StatusUndone, snapshot.ConfirmedAt); err != nil {
		return nil, err
	}
	snapshot.Status = StatusUndone
	if err := s.audit(ctx, "undo", snapshot, "按快照撤销基金E账户导入"); err != nil {
		return nil, err
	}
	return snapshot, nil
}

type UpsertAccountInput struct {
	ID       string      `json:"id,omitempty"`
	Name     string      `json:"name,omitempty"`
	Platform string      `json:"platform,omitempty"`
	Type     AccountType `json:"type,omitempty"`
	Currency string      `json:"currency,omitempty"`
	Owner    *string     `json:"owner,omitempty"`
	ParentID string      `json:"parent_id,omitempty"`
	Archived *bool       `json:"archived,omitempty"`
}

func (s *Service) UpsertAccount(ctx context.Context, in UpsertAccountInput) (*Account, error) {
	now := s.now().UTC()
	if in.ID == "" {
		if strings.TrimSpace(in.Name) == "" {
			return nil, errors.New("name is required when creating an account")
		}
		if in.ParentID != "" {
			parent, err := s.repo.GetAccount(ctx, in.ParentID)
			if err != nil {
				return nil, err
			}
			if parent.ParentID != "" {
				return nil, errors.New("只允许两级账户：组合下不能再建组合")
			}
			if parent.Archived {
				return nil, errors.New("上级账户已归档")
			}
			if parent.Type == AccountLiability {
				return nil, errors.New("负债账户不能作为上级账户")
			}
			has, err := s.repo.HasAnySnapshot(ctx, parent.ID)
			if err != nil {
				return nil, err
			}
			if has {
				return nil, errors.New("已有快照的账户不能作为上级账户")
			}
			in.Type = parent.Type
			in.Currency = parent.Currency
			owner := parent.Owner
			in.Owner = &owner
			if in.Platform == "" {
				in.Platform = parent.Platform
			}
		}
		if !in.Type.Valid() {
			return nil, fmt.Errorf("type %q must be one of cash, fund, stock, mixed, liability, other", in.Type)
		}
		currency := strings.ToUpper(in.Currency)
		if currency == "" {
			currency = "CNY"
		}
		if in.Archived == nil || !*in.Archived {
			if err := s.ensureUniqueName(ctx, in.ParentID, in.Name, ""); err != nil {
				return nil, err
			}
		}
		owner := ""
		if in.Owner != nil {
			owner = *in.Owner
		}
		account := &Account{
			ID:        newID("acc"),
			Name:      in.Name,
			Platform:  in.Platform,
			Type:      in.Type,
			Currency:  currency,
			Owner:     owner,
			ParentID:  in.ParentID,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if in.Archived != nil {
			account.Archived = *in.Archived
		}
		if err := s.repo.UpsertAccount(ctx, account); err != nil {
			return nil, err
		}
		return account, nil
	}

	account, err := s.repo.GetAccount(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	wasArchived := account.Archived
	wasName := account.Name
	cascade := false
	if in.Name != "" {
		account.Name = in.Name
	}
	if in.Platform != "" {
		account.Platform = in.Platform
	}
	if account.ParentID != "" {
		// A portfolio always mirrors its parent; individual type/currency
		// changes are ignored, like owner changes.
		parent, err := s.repo.GetAccount(ctx, account.ParentID)
		if err != nil {
			return nil, err
		}
		account.Type = parent.Type
		account.Currency = parent.Currency
	} else {
		newType := account.Type
		if in.Type != "" {
			if !in.Type.Valid() {
				return nil, fmt.Errorf("type %q must be one of cash, fund, stock, mixed, liability, other", in.Type)
			}
			newType = in.Type
		}
		newCurrency := account.Currency
		if in.Currency != "" {
			newCurrency = strings.ToUpper(in.Currency)
		}
		typeChanged := newType != account.Type
		currencyChanged := newCurrency != account.Currency
		if typeChanged || currencyChanged {
			hasChildren, err := s.repo.HasChildren(ctx, account.ID)
			if err != nil {
				return nil, err
			}
			if hasChildren {
				if typeChanged && newType == AccountLiability {
					return nil, errors.New("负债账户不能作为上级账户")
				}
				if currencyChanged {
					if _, err := s.conflictingChildSnapshotCurrency(ctx, account.ID, newCurrency); err != nil {
						return nil, err
					}
				}
			}
			account.Type = newType
			account.Currency = newCurrency
			cascade = typeChanged || currencyChanged
		}
	}
	ownerChanged := in.Owner != nil && account.ParentID == ""
	if ownerChanged {
		account.Owner = *in.Owner
	}
	if in.ParentID != "" && in.ParentID != account.ParentID {
		return nil, errors.New("不支持修改上级账户")
	}
	if in.Archived != nil {
		if *in.Archived && !account.Archived {
			hasChildren, err := s.repo.HasChildren(ctx, account.ID)
			if err != nil {
				return nil, err
			}
			if hasChildren {
				return nil, errors.New("有组合的账户不能归档")
			}
			hasSnapshots, err := s.repo.HasAnySnapshot(ctx, account.ID)
			if err != nil {
				return nil, err
			}
			if hasSnapshots {
				return nil, errors.New("有快照的账户不能归档")
			}
		}
		account.Archived = *in.Archived
	}
	nameChanged := in.Name != "" && strings.TrimSpace(in.Name) != strings.TrimSpace(wasName)
	if !account.Archived && (nameChanged || (wasArchived && in.Archived != nil && !*in.Archived)) {
		if err := s.ensureUniqueName(ctx, account.ParentID, account.Name, account.ID); err != nil {
			return nil, err
		}
	}
	account.UpdatedAt = now
	if err := s.repo.UpsertAccount(ctx, account); err != nil {
		return nil, err
	}
	if ownerChanged || cascade {
		accounts, err := s.repo.ListAccounts(ctx)
		if err != nil {
			return nil, err
		}
		for i := range accounts {
			if accounts[i].ParentID != account.ID {
				continue
			}
			if ownerChanged {
				accounts[i].Owner = account.Owner
			}
			if cascade {
				accounts[i].Type = account.Type
				accounts[i].Currency = account.Currency
			}
			accounts[i].UpdatedAt = now
			if err := s.repo.UpsertAccount(ctx, &accounts[i]); err != nil {
				return nil, err
			}
		}
	}
	return account, nil
}

func (s *Service) ensureUniqueName(ctx context.Context, parentID, name, excludeID string) error {
	accounts, err := s.repo.ListAccounts(ctx)
	if err != nil {
		return err
	}
	trimmed := strings.TrimSpace(name)
	for _, account := range accounts {
		if account.ID == excludeID || account.Archived || account.ParentID != parentID {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(account.Name), trimmed) {
			if parentID == "" {
				return errors.New("同名账户已存在")
			}
			return errors.New("该账户下已有同名组合")
		}
	}
	return nil
}

func (s *Service) conflictingChildSnapshotCurrency(ctx context.Context, parentID, currency string) (*Snapshot, error) {
	accounts, err := s.repo.ListAccounts(ctx)
	if err != nil {
		return nil, err
	}
	pending, err := s.repo.ListPending(ctx)
	if err != nil {
		return nil, err
	}
	for _, account := range accounts {
		if account.ParentID != parentID {
			continue
		}
		confirmed, err := s.repo.ListConfirmed(ctx, account.ID, "", "")
		if err != nil {
			return nil, err
		}
		for _, snapshot := range append(confirmed, pending...) {
			if snapshot.AccountID == account.ID && snapshot.Currency != currency {
				return snapshot, fmt.Errorf("组合「%s」已有 %s 币种的快照，不能把上级账户币种改为 %s", account.Name, snapshot.Currency, currency)
			}
		}
	}
	return nil, nil
}

func (s *Service) audit(ctx context.Context, action string, snapshot *Snapshot, detail string) error {
	return s.repo.AddAudit(ctx, AuditEntry{
		Action:     action,
		AccountID:  snapshot.AccountID,
		SnapshotID: snapshot.ID,
		Detail:     detail,
		CreatedAt:  s.now().UTC(),
	})
}

func newID(prefix string) string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return prefix + "_" + hex.EncodeToString(b)
}
