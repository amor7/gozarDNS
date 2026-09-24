package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Account states, derived rather than stored so they can never go stale.
const (
	StateActive    = "active"
	StateDisabled  = "disabled"
	StateNoPlan    = "no_plan"
	StateExpired   = "expired"
	StateOverQuota = "over_quota"
)

type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Phone       string `json:"phone"`
	PlanID      *int64 `json:"plan_id"`
	PlanName    string `json:"plan_name"`
	Enabled     bool   `json:"enabled"`
	QuotaBytes  int64  `json:"quota_bytes"`
	UsedBytes   int64  `json:"used_bytes"`
	PeriodStart *int64 `json:"period_start"`
	ExpiresAt   *int64 `json:"expires_at"`
	SpeedKbps   int    `json:"speed_kbps"`
	MaxDevices  int    `json:"max_devices"`
	Note        string `json:"note"`
	CreatedAt   int64  `json:"created_at"`
	LastSeenAt  *int64 `json:"last_seen_at"`
	Devices     int    `json:"devices"`
	State       string `json:"state"`
	HasPassword bool   `json:"has_password"`

	passHash string
}

func (u *User) PassHash() string { return u.passHash }

// computeState decides whether the account may be used right now.
func (u *User) computeState(now int64) {
	switch {
	case !u.Enabled:
		u.State = StateDisabled
	case u.PlanID == nil:
		u.State = StateNoPlan
	case u.ExpiresAt != nil && *u.ExpiresAt <= now:
		u.State = StateExpired
	case u.QuotaBytes > 0 && u.UsedBytes >= u.QuotaBytes:
		u.State = StateOverQuota
	default:
		u.State = StateActive
	}
}

const userCols = `u.id, u.username, u.display_name, u.phone, u.pass_hash, u.plan_id, COALESCE(p.name, ''),
	u.enabled, u.quota_bytes, u.used_bytes, u.period_start, u.expires_at, u.speed_kbps, u.max_devices,
	u.note, u.created_at, u.last_seen_at, (SELECT count(*) FROM devices d WHERE d.user_id = u.id)`

const userFrom = ` FROM users u LEFT JOIN plans p ON p.id = u.plan_id `

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	var plan, start, exp, seen sql.NullInt64
	var enabled int
	err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Phone, &u.passHash, &plan, &u.PlanName,
		&enabled, &u.QuotaBytes, &u.UsedBytes, &start, &exp, &u.SpeedKbps, &u.MaxDevices,
		&u.Note, &u.CreatedAt, &seen, &u.Devices)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.PlanID, u.PeriodStart, u.ExpiresAt, u.LastSeenAt = ptr(plan), ptr(start), ptr(exp), ptr(seen)
	u.Enabled = enabled == 1
	u.HasPassword = u.passHash != ""
	u.computeState(time.Now().Unix())
	return u, nil
}

func (s *Store) User(id int64) (*User, error) {
	return scanUser(s.db.QueryRow("SELECT "+userCols+userFrom+"WHERE u.id = ?", id))
}

func (s *Store) UserByName(name string) (*User, error) {
	return scanUser(s.db.QueryRow("SELECT "+userCols+userFrom+"WHERE u.username = ?", strings.TrimSpace(name)))
}

type UserFilter struct {
	Query  string
	State  string
	PlanID int64
	Sort   string // "recent", "usage", "expiry", "name"
	Limit  int
	Offset int
}

// Users lists accounts. State is derived, so it is filtered after the query.
func (s *Store) Users(f UserFilter) ([]*User, int, error) {
	where := []string{"1=1"}
	args := []any{}
	if q := strings.TrimSpace(f.Query); q != "" {
		where = append(where, `(u.username LIKE ? OR u.display_name LIKE ? OR u.phone LIKE ? OR u.note LIKE ?
			OR EXISTS (SELECT 1 FROM devices d WHERE d.user_id = u.id AND d.ip LIKE ?))`)
		like := "%" + q + "%"
		args = append(args, like, like, like, like, like)
	}
	if f.PlanID > 0 {
		where = append(where, "u.plan_id = ?")
		args = append(args, f.PlanID)
	}
	order := "u.id DESC"
	switch f.Sort {
	case "usage":
		order = "u.used_bytes DESC"
	case "expiry":
		order = "u.expires_at IS NULL, u.expires_at ASC"
	case "name":
		order = "u.username COLLATE NOCASE ASC"
	case "seen":
		order = "u.last_seen_at IS NULL, u.last_seen_at DESC"
	}
	rows, err := s.db.Query("SELECT "+userCols+userFrom+"WHERE "+strings.Join(where, " AND ")+" ORDER BY "+order, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var all []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		if f.State != "" && u.State != f.State {
			continue
		}
		all = append(all, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	total := len(all)
	if f.Offset > 0 {
		if f.Offset >= len(all) {
			all = nil
		} else {
			all = all[f.Offset:]
		}
	}
	if f.Limit > 0 && len(all) > f.Limit {
		all = all[:f.Limit]
	}
	if all == nil {
		all = []*User{}
	}
	return all, total, nil
}

type NewUser struct {
	Username    string
	DisplayName string
	Phone       string
	Password    string // already hashed by the caller, or empty
	PlanID      int64  // 0: no plan yet
	Note        string
}

var ErrTaken = errors.New("taken")

func (s *Store) CreateUser(n NewUser) (*User, error) {
	now := time.Now().Unix()
	res, err := s.db.Exec(`INSERT INTO users(username, display_name, phone, pass_hash, note, created_at, max_devices)
		VALUES(?, ?, ?, ?, ?, ?, 2)`, n.Username, n.DisplayName, n.Phone, n.Password, n.Note, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrTaken
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	if n.PlanID > 0 {
		if err := s.ApplyPlan(id, n.PlanID); err != nil {
			return nil, err
		}
	}
	s.touch()
	return s.User(id)
}

// UserPatch changes only the fields that are set.
type UserPatch struct {
	DisplayName *string `json:"display_name"`
	Phone       *string `json:"phone"`
	Enabled     *bool   `json:"enabled"`
	QuotaBytes  *int64  `json:"quota_bytes"`
	UsedBytes   *int64  `json:"used_bytes"`
	ExpiresAt   *int64  `json:"expires_at"` // 0 clears it
	SpeedKbps   *int    `json:"speed_kbps"`
	MaxDevices  *int    `json:"max_devices"`
	Note        *string `json:"note"`
	PlanID      *int64  `json:"plan_id"` // switches routing only; ApplyPlan also resets the allowance
}

func (s *Store) UpdateUser(id int64, p UserPatch) error {
	sets := []string{}
	args := []any{}
	add := func(col string, v any) {
		sets = append(sets, col+" = ?")
		args = append(args, v)
	}
	if p.DisplayName != nil {
		add("display_name", strings.TrimSpace(*p.DisplayName))
	}
	if p.Phone != nil {
		add("phone", strings.TrimSpace(*p.Phone))
	}
	if p.Enabled != nil {
		add("enabled", boolInt(*p.Enabled))
	}
	if p.QuotaBytes != nil {
		add("quota_bytes", max(*p.QuotaBytes, 0))
	}
	if p.UsedBytes != nil {
		add("used_bytes", max(*p.UsedBytes, 0))
	}
	if p.ExpiresAt != nil {
		if *p.ExpiresAt <= 0 {
			add("expires_at", nil)
		} else {
			add("expires_at", *p.ExpiresAt)
		}
	}
	if p.SpeedKbps != nil {
		add("speed_kbps", max(*p.SpeedKbps, 0))
	}
	if p.MaxDevices != nil {
		add("max_devices", min(max(*p.MaxDevices, 1), 50))
	}
	if p.Note != nil {
		add("note", *p.Note)
	}
	if p.PlanID != nil {
		if *p.PlanID <= 0 {
			add("plan_id", nil)
		} else {
			add("plan_id", *p.PlanID)
		}
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, id)
	res, err := s.db.Exec("UPDATE users SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.touch()
	return nil
}

func (s *Store) SetUserPassword(id int64, hash string) error {
	_, err := s.db.Exec("UPDATE users SET pass_hash = ? WHERE id = ?", hash, id)
	if err == nil {
		_, err = s.db.Exec("DELETE FROM sessions WHERE kind = 'user' AND subject = ?", id)
	}
	return err
}

func (s *Store) DeleteUser(id int64) error {
	res, err := s.db.Exec("DELETE FROM users WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, _ = s.db.Exec("DELETE FROM sessions WHERE kind = 'user' AND subject = ?", id)
	s.touch()
	return nil
}

// ApplyPlan puts a user on a plan starting now: fresh allowance, fresh
// period, the plan's speed and device limit.
func (s *Store) ApplyPlan(userID, planID int64) error {
	p, err := s.Plan(planID)
	if err != nil {
		return err
	}
	now := time.Now()
	var exp any
	if p.DurationDays > 0 {
		exp = now.AddDate(0, 0, p.DurationDays).Unix()
	}
	res, err := s.db.Exec(`UPDATE users SET plan_id = ?, quota_bytes = ?, used_bytes = 0, period_start = ?,
		expires_at = ?, speed_kbps = ?, max_devices = ? WHERE id = ?`,
		p.ID, p.QuotaBytes, now.Unix(), exp, p.SpeedKbps, p.MaxDevices, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.touch()
	return nil
}

// ExtendPlan renews on top of what is left: more days after the current end
// (or from now if it already ended) and the plan's allowance added.
func (s *Store) ExtendPlan(userID, planID int64) error {
	u, err := s.User(userID)
	if err != nil {
		return err
	}
	if u.PlanID == nil || *u.PlanID != planID || u.State == StateExpired {
		return s.ApplyPlan(userID, planID)
	}
	p, err := s.Plan(planID)
	if err != nil {
		return err
	}
	var exp any
	if p.DurationDays > 0 {
		from := time.Now()
		if u.ExpiresAt != nil && *u.ExpiresAt > from.Unix() {
			from = time.Unix(*u.ExpiresAt, 0)
		}
		exp = from.AddDate(0, 0, p.DurationDays).Unix()
	}
	var quota int64
	switch {
	case p.QuotaBytes == 0:
		quota = 0
	case u.QuotaBytes == 0:
		quota = u.UsedBytes + p.QuotaBytes
	default:
		quota = max(u.QuotaBytes, u.UsedBytes) + p.QuotaBytes
	}
	_, err = s.db.Exec(`UPDATE users SET quota_bytes = ?, expires_at = ?, speed_kbps = ?, max_devices = ? WHERE id = ?`,
		quota, exp, p.SpeedKbps, p.MaxDevices, userID)
	s.touch()
	return err
}

func (s *Store) ResetUsage(id int64) error {
	_, err := s.db.Exec("UPDATE users SET used_bytes = 0, period_start = ? WHERE id = ?", time.Now().Unix(), id)
	s.touch()
	return err
}

// RollPeriods starts a fresh period for users on a "reset" plan whose period
// has ended: usage back to zero, end date moved forward. Plans that "expire"
// are left expired until someone renews them.
func (s *Store) RollPeriods() (int, error) {
	now := time.Now()
	rows, err := s.db.Query(`SELECT u.id, u.expires_at, p.duration_days, p.quota_bytes FROM users u
		JOIN plans p ON p.id = u.plan_id
		WHERE p.renewal = 'reset' AND p.duration_days > 0 AND u.expires_at IS NOT NULL AND u.expires_at <= ?`, now.Unix())
	if err != nil {
		return 0, err
	}
	type roll struct {
		id, exp, quota int64
		days           int
	}
	var todo []roll
	for rows.Next() {
		var r roll
		if err := rows.Scan(&r.id, &r.exp, &r.days, &r.quota); err == nil {
			todo = append(todo, r)
		}
	}
	rows.Close()
	for _, r := range todo {
		next := time.Unix(r.exp, 0)
		for !next.After(now) {
			next = next.AddDate(0, 0, r.days)
		}
		start := next.AddDate(0, 0, -r.days)
		_, _ = s.db.Exec("UPDATE users SET used_bytes = 0, period_start = ?, expires_at = ? WHERE id = ?",
			start.Unix(), next.Unix(), r.id)
	}
	if len(todo) > 0 {
		s.touch()
	}
	return len(todo), nil
}

// ---------------------------------------------------------------- devices

type Device struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	IP         string `json:"ip"`
	Label      string `json:"label"`
	AddedAt    int64  `json:"added_at"`
	LastSeenAt *int64 `json:"last_seen_at"`
}

func (s *Store) Devices(userID int64) ([]Device, error) {
	rows, err := s.db.Query("SELECT id, user_id, ip, label, added_at, last_seen_at FROM devices WHERE user_id = ? ORDER BY added_at", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		var d Device
		var seen sql.NullInt64
		if err := rows.Scan(&d.ID, &d.UserID, &d.IP, &d.Label, &d.AddedAt, &seen); err != nil {
			return nil, err
		}
		d.LastSeenAt = ptr(seen)
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) DeviceByIP(ip string) (*Device, error) {
	var d Device
	var seen sql.NullInt64
	err := s.db.QueryRow("SELECT id, user_id, ip, label, added_at, last_seen_at FROM devices WHERE ip = ?", ip).
		Scan(&d.ID, &d.UserID, &d.IP, &d.Label, &d.AddedAt, &seen)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	d.LastSeenAt = ptr(seen)
	return &d, err
}

var (
	ErrIPOwned     = errors.New("address belongs to another account")
	ErrTooMany     = errors.New("device limit reached")
	ErrAlreadyMine = errors.New("already registered")
)

// ClaimIP registers ip for the user. When the user is at their device limit,
// replace names which of their devices gives way; otherwise ErrTooMany.
func (s *Store) ClaimIP(userID int64, ip, label string, replace int64) (*Device, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var owner int64
	err = tx.QueryRow("SELECT user_id FROM devices WHERE ip = ?", ip).Scan(&owner)
	if err == nil {
		if owner == userID {
			return nil, ErrAlreadyMine
		}
		return nil, ErrIPOwned
	}
	var limit, have int
	if err := tx.QueryRow("SELECT max_devices, (SELECT count(*) FROM devices WHERE user_id = ?) FROM users WHERE id = ?", userID, userID).Scan(&limit, &have); err != nil {
		return nil, ErrNotFound
	}
	if have >= limit {
		if replace <= 0 {
			return nil, ErrTooMany
		}
		res, err := tx.Exec("DELETE FROM devices WHERE id = ? AND user_id = ?", replace, userID)
		if err != nil {
			return nil, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, ErrNotFound
		}
	}
	now := time.Now().Unix()
	res, err := tx.Exec("INSERT INTO devices(user_id, ip, label, added_at, last_seen_at) VALUES(?, ?, ?, ?, ?)", userID, ip, label, now, now)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	s.touch()
	return &Device{ID: id, UserID: userID, IP: ip, Label: label, AddedAt: now, LastSeenAt: &now}, nil
}

// AddDeviceAdmin registers an address by hand, over the device limit if need be.
func (s *Store) AddDeviceAdmin(userID int64, ip, label string) error {
	var owner int64
	if err := s.db.QueryRow("SELECT user_id FROM devices WHERE ip = ?", ip).Scan(&owner); err == nil {
		if owner == userID {
			return ErrAlreadyMine
		}
		return ErrIPOwned
	}
	_, err := s.db.Exec("INSERT INTO devices(user_id, ip, label, added_at) VALUES(?, ?, ?, ?)", userID, ip, label, time.Now().Unix())
	s.touch()
	return err
}

func (s *Store) RenameDevice(userID, id int64, label string) error {
	_, err := s.db.Exec("UPDATE devices SET label = ? WHERE id = ? AND user_id = ?", label, id, userID)
	return err
}

func (s *Store) DeleteDevice(userID, id int64) error {
	res, err := s.db.Exec("DELETE FROM devices WHERE id = ? AND user_id = ?", id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.touch()
	return nil
}

func (s *Store) DeviceCount() int {
	var n int
	_ = s.db.QueryRow("SELECT count(*) FROM devices").Scan(&n)
	return n
}

// ---------------------------------------------------------------- usage

type DayUsage struct {
	Day   string `json:"day"`
	Bytes int64  `json:"bytes"`
}

// DailyUsage returns the last n days for one user (or everyone when userID
// is 0), oldest first, with empty days filled in.
func (s *Store) DailyUsage(userID int64, n int) ([]DayUsage, error) {
	from := time.Now().AddDate(0, 0, -(n - 1)).Format("2006-01-02")
	q := "SELECT day, sum(bytes) FROM usage_daily WHERE day >= ?"
	args := []any{from}
	if userID > 0 {
		q += " AND user_id = ?"
		args = append(args, userID)
	}
	rows, err := s.db.Query(q+" GROUP BY day", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	got := map[string]int64{}
	for rows.Next() {
		var d string
		var b int64
		if rows.Scan(&d, &b) == nil {
			got[d] = b
		}
	}
	out := make([]DayUsage, 0, n)
	for i := n - 1; i >= 0; i-- {
		d := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		out = append(out, DayUsage{Day: d, Bytes: got[d]})
	}
	return out, nil
}

type TopUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Bytes    int64  `json:"bytes"`
}

func (s *Store) TopUsers(days, n int) ([]TopUser, error) {
	from := time.Now().AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	rows, err := s.db.Query(`SELECT u.id, u.username, sum(d.bytes) b FROM usage_daily d JOIN users u ON u.id = d.user_id
		WHERE d.day >= ? GROUP BY u.id ORDER BY b DESC LIMIT ?`, from, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TopUser{}
	for rows.Next() {
		var t TopUser
		if rows.Scan(&t.ID, &t.Username, &t.Bytes) == nil {
			out = append(out, t)
		}
	}
	return out, nil
}
