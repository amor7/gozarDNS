package store

import (
	"database/sql"
	"errors"
	"time"
)

// Online payments. A checkout starts as "started" and becomes "approved" only
// once the gateway has verified the money arrived; one abandoned halfway
// becomes "failed". "started" rows never show up in the receipt queue.
const (
	PayStarted = "started"
	PayFailed  = "failed"
)

type OnlineOrder struct {
	ID        int64
	UserID    int64
	PlanID    *int64
	Amount    int64 // in the plan's price unit
	Status    string
	Ref       string // the gateway's own id for this payment
	CreatedAt int64
}

func (s *Store) StartOnlinePayment(userID int64, planID int64, amount int64) (int64, error) {
	res, err := s.db.Exec(`INSERT INTO payments(user_id, plan_id, amount, status, method, created_at)
		VALUES(?,?,?,?,?,?)`, userID, planID, amount, PayStarted, "idpay", time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) OnlineOrder(id int64) (*OnlineOrder, error) {
	o := &OnlineOrder{}
	var plan sql.NullInt64
	err := s.db.QueryRow(`SELECT id, user_id, plan_id, amount, status, gateway_ref, created_at FROM payments
		WHERE id = ? AND method = 'idpay'`, id).Scan(&o.ID, &o.UserID, &plan, &o.Amount, &o.Status, &o.Ref, &o.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	o.PlanID = ptr(plan)
	return o, err
}

func (s *Store) SetOnlineRef(id int64, ref string) error {
	_, err := s.db.Exec("UPDATE payments SET gateway_ref = ? WHERE id = ? AND status = ?", ref, id, PayStarted)
	return err
}

// FinishOnline records the outcome. It returns false when the order was
// already finished, so a callback that arrives twice is only acted on once.
func (s *Store) FinishOnline(id int64, ok bool, trackID, note string) (bool, error) {
	status := PayFailed
	if ok {
		status = PayApproved
	}
	res, err := s.db.Exec(`UPDATE payments SET status = ?, track_id = ?, admin_note = ?, decided_at = ?, decided_by = 'idpay'
		WHERE id = ? AND method = 'idpay' AND status = ?`, status, trackID, note, time.Now().Unix(), id, PayStarted)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// PruneOnline drops checkouts nobody finished within two days.
func (s *Store) PruneOnline() {
	_, _ = s.db.Exec("DELETE FROM payments WHERE status = ? AND created_at < ?", PayStarted, time.Now().Add(-48*time.Hour).Unix())
}
