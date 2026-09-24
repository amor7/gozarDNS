package store

import (
	"database/sql"
	"errors"
	"time"
)

type Payment struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"user_id"`
	Username    string `json:"username"`
	PlanID      *int64 `json:"plan_id"`
	PlanName    string `json:"plan_name"`
	Amount      int64  `json:"amount"`
	Note        string `json:"note"`
	HasReceipt  bool   `json:"has_receipt"`
	ReceiptType string `json:"receipt_type"`
	Status      string `json:"status"`
	AdminNote   string `json:"admin_note"`
	CreatedAt   int64  `json:"created_at"`
	DecidedAt   *int64 `json:"decided_at"`
	DecidedBy   string `json:"decided_by"`
	Method      string `json:"method"` // "receipt" or "idpay"
	TrackID     string `json:"track_id"`
}

const (
	PayPending  = "pending"
	PayApproved = "approved"
	PayRejected = "rejected"
)

const payCols = `pm.id, pm.user_id, u.username, pm.plan_id, COALESCE(p.name, ''), pm.amount, pm.note,
	pm.receipt IS NOT NULL, pm.receipt_type, pm.status, pm.admin_note, pm.created_at, pm.decided_at, pm.decided_by,
	pm.method, pm.track_id`

const payFrom = ` FROM payments pm JOIN users u ON u.id = pm.user_id LEFT JOIN plans p ON p.id = pm.plan_id `

func scanPayment(row interface{ Scan(...any) error }) (*Payment, error) {
	p := &Payment{}
	var plan, decided sql.NullInt64
	var has int
	err := row.Scan(&p.ID, &p.UserID, &p.Username, &plan, &p.PlanName, &p.Amount, &p.Note, &has,
		&p.ReceiptType, &p.Status, &p.AdminNote, &p.CreatedAt, &decided, &p.DecidedBy, &p.Method, &p.TrackID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	p.PlanID, p.DecidedAt, p.HasReceipt = ptr(plan), ptr(decided), has == 1
	return p, err
}

func (s *Store) Payment(id int64) (*Payment, error) {
	return scanPayment(s.db.QueryRow("SELECT "+payCols+payFrom+"WHERE pm.id = ?", id))
}

// Payments lists by status ("" for all) or for one user (userID > 0).
func (s *Store) Payments(status string, userID int64, limit int) ([]*Payment, error) {
	q := "SELECT " + payCols + payFrom + "WHERE 1=1"
	args := []any{}
	if status != "" {
		q += " AND pm.status = ?"
		args = append(args, status)
	} else {
		q += " AND pm.status != 'started'"
	}
	if userID > 0 {
		q += " AND pm.user_id = ?"
		args = append(args, userID)
	}
	q += " ORDER BY pm.id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Payment{}
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) PendingPayments() int {
	var n int
	_ = s.db.QueryRow("SELECT count(*) FROM payments WHERE status = 'pending'").Scan(&n)
	return n
}

var ErrPendingExists = errors.New("a receipt is already waiting")

func (s *Store) CreatePayment(userID int64, planID *int64, amount int64, note string, receipt []byte, ctype string) (int64, error) {
	var n int
	_ = s.db.QueryRow("SELECT count(*) FROM payments WHERE user_id = ? AND status = 'pending'", userID).Scan(&n)
	if n >= 3 {
		return 0, ErrPendingExists
	}
	res, err := s.db.Exec(`INSERT INTO payments(user_id, plan_id, amount, note, receipt, receipt_type, created_at)
		VALUES(?,?,?,?,?,?,?)`, userID, nullInt(planID), amount, note, receipt, ctype, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) Receipt(id int64) ([]byte, string, error) {
	var b []byte
	var t string
	err := s.db.QueryRow("SELECT receipt, receipt_type FROM payments WHERE id = ?", id).Scan(&b, &t)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && b == nil) {
		return nil, "", ErrNotFound
	}
	return b, t, err
}

// DecidePayment records the operator's decision. The image is kept for a
// while after approval so it can be looked at again; Prune drops old ones.
func (s *Store) DecidePayment(id int64, status, note, by string) error {
	res, err := s.db.Exec(`UPDATE payments SET status = ?, admin_note = ?, decided_at = ?, decided_by = ?
		WHERE id = ? AND status = 'pending'`, status, note, time.Now().Unix(), by, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// PruneReceipts drops receipt images decided more than 60 days ago; the row
// stays as the record.
func (s *Store) PruneReceipts() {
	_, _ = s.db.Exec("UPDATE payments SET receipt = NULL WHERE status != 'pending' AND decided_at < ?",
		time.Now().AddDate(0, 0, -60).Unix())
}

func (s *Store) RevenueSince(since int64) int64 {
	var n sql.NullInt64
	_ = s.db.QueryRow("SELECT sum(amount) FROM payments WHERE status = 'approved' AND decided_at >= ?", since).Scan(&n)
	return n.Int64
}
