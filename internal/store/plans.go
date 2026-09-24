package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Plan is both what is sold and what is routed: the allowance, the period,
// the speed, and which service groups go through the hub.
type Plan struct {
	ID           int64    `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Price        int64    `json:"price"`
	QuotaBytes   int64    `json:"quota_bytes"`
	DurationDays int      `json:"duration_days"`
	Renewal      string   `json:"renewal"`
	SpeedKbps    int      `json:"speed_kbps"`
	MaxDevices   int      `json:"max_devices"`
	Groups       []string `json:"groups"`
	Excluded     []string `json:"excluded"`
	Public       bool     `json:"public"`
	IsDefault    bool     `json:"is_default"`
	Color        string   `json:"color"`
	Sort         int      `json:"sort"`
	CreatedAt    int64    `json:"created_at"`
	Users        int      `json:"users"`
}

const planCols = `id, name, description, price, quota_bytes, duration_days, renewal, speed_kbps, max_devices,
	groups_json, excluded_json, public, is_default, color, sort, created_at,
	(SELECT count(*) FROM users u WHERE u.plan_id = plans.id)`

func scanPlan(row interface{ Scan(...any) error }) (*Plan, error) {
	p := &Plan{}
	var groups, excluded string
	var public, def int
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.QuotaBytes, &p.DurationDays, &p.Renewal,
		&p.SpeedKbps, &p.MaxDevices, &groups, &excluded, &public, &def, &p.Color, &p.Sort, &p.CreatedAt, &p.Users)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(groups), &p.Groups)
	_ = json.Unmarshal([]byte(excluded), &p.Excluded)
	if p.Groups == nil {
		p.Groups = []string{}
	}
	if p.Excluded == nil {
		p.Excluded = []string{}
	}
	p.Public, p.IsDefault = public == 1, def == 1
	return p, nil
}

func (s *Store) Plan(id int64) (*Plan, error) {
	return scanPlan(s.db.QueryRow("SELECT "+planCols+" FROM plans WHERE id = ?", id))
}

func (s *Store) Plans() ([]*Plan, error) {
	rows, err := s.db.Query("SELECT " + planCols + " FROM plans ORDER BY sort, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Plan{}
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DefaultPlan is the routing used for addresses nobody has registered yet,
// while a gate is still open to everyone.
func (s *Store) DefaultPlan() (*Plan, error) {
	p, err := scanPlan(s.db.QueryRow("SELECT " + planCols + " FROM plans WHERE is_default = 1 LIMIT 1"))
	if errors.Is(err, ErrNotFound) {
		return scanPlan(s.db.QueryRow("SELECT " + planCols + " FROM plans ORDER BY id LIMIT 1"))
	}
	return p, err
}

func (p *Plan) normalise() {
	p.Name = strings.TrimSpace(p.Name)
	if p.Renewal != "reset" {
		p.Renewal = "expire"
	}
	p.MaxDevices = min(max(p.MaxDevices, 1), 50)
	p.QuotaBytes = max(p.QuotaBytes, 0)
	p.DurationDays = max(p.DurationDays, 0)
	p.SpeedKbps = max(p.SpeedKbps, 0)
	p.Price = max(p.Price, 0)
	if p.Groups == nil {
		p.Groups = []string{}
	}
	if p.Excluded == nil {
		p.Excluded = []string{}
	}
}

func (s *Store) SavePlan(p *Plan) error {
	p.normalise()
	if p.Name == "" {
		return errors.New("a plan needs a name")
	}
	groups, _ := json.Marshal(p.Groups)
	excluded, _ := json.Marshal(p.Excluded)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if p.IsDefault {
		if _, err := tx.Exec("UPDATE plans SET is_default = 0"); err != nil {
			return err
		}
	}
	if p.ID == 0 {
		p.CreatedAt = time.Now().Unix()
		res, err := tx.Exec(`INSERT INTO plans(name, description, price, quota_bytes, duration_days, renewal, speed_kbps,
			max_devices, groups_json, excluded_json, public, is_default, color, sort, created_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			p.Name, p.Description, p.Price, p.QuotaBytes, p.DurationDays, p.Renewal, p.SpeedKbps, p.MaxDevices,
			string(groups), string(excluded), boolInt(p.Public), boolInt(p.IsDefault), p.Color, p.Sort, p.CreatedAt)
		if err != nil {
			return err
		}
		p.ID, _ = res.LastInsertId()
	} else {
		res, err := tx.Exec(`UPDATE plans SET name=?, description=?, price=?, quota_bytes=?, duration_days=?, renewal=?,
			speed_kbps=?, max_devices=?, groups_json=?, excluded_json=?, public=?, is_default=?, color=?, sort=? WHERE id=?`,
			p.Name, p.Description, p.Price, p.QuotaBytes, p.DurationDays, p.Renewal, p.SpeedKbps, p.MaxDevices,
			string(groups), string(excluded), boolInt(p.Public), boolInt(p.IsDefault), p.Color, p.Sort, p.ID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.touch()
	return nil
}

var ErrInUse = errors.New("in use")

func (s *Store) DeletePlan(id int64) error {
	var n int
	_ = s.db.QueryRow("SELECT count(*) FROM users WHERE plan_id = ?", id).Scan(&n)
	if n > 0 {
		return ErrInUse
	}
	res, err := s.db.Exec("DELETE FROM plans WHERE id = ?", id)
	if err != nil {
		return err
	}
	if k, _ := res.RowsAffected(); k == 0 {
		return ErrNotFound
	}
	s.touch()
	return nil
}

// ---------------------------------------------------------------- custom domains

type CustomDomain struct {
	Domain  string `json:"domain"`
	Action  string `json:"action"`
	Note    string `json:"note"`
	AddedAt int64  `json:"added_at"`
}

func (s *Store) CustomDomains() ([]CustomDomain, error) {
	rows, err := s.db.Query("SELECT domain, action, note, added_at FROM custom_domains ORDER BY domain")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CustomDomain{}
	for rows.Next() {
		var d CustomDomain
		if rows.Scan(&d.Domain, &d.Action, &d.Note, &d.AddedAt) == nil {
			out = append(out, d)
		}
	}
	return out, rows.Err()
}

func (s *Store) AddCustomDomain(d CustomDomain) error {
	if d.Action != "direct" {
		d.Action = "route"
	}
	_, err := s.db.Exec(`INSERT INTO custom_domains(domain, action, note, added_at) VALUES(?,?,?,?)
		ON CONFLICT(domain) DO UPDATE SET action = excluded.action, note = excluded.note`,
		d.Domain, d.Action, d.Note, time.Now().Unix())
	s.touch()
	return err
}

func (s *Store) DeleteCustomDomain(domain string) error {
	res, err := s.db.Exec("DELETE FROM custom_domains WHERE domain = ?", domain)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	s.touch()
	return nil
}
