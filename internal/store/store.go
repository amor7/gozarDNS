// Package store is the hub's database: customers, plans, gates, payments and
// the numbers that describe how all of it is being used.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("not found")

type Store struct {
	db   *sql.DB
	path string

	// Bumped on any change that alters what gates are told, so the config
	// revision can be recomputed only when it could have moved.
	mu  sync.Mutex
	gen int64
}

// Open opens (creating if needed) the database at path and brings its schema
// up to date. A restore staged by Restore is swapped in first.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	if err := swapInRestore(path); err != nil {
		return nil, err
	}
	dsn := "file:" + filepath.ToSlash(path) +
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// One connection: sqlite serialises writers anyway, and a single
	// connection turns lock contention into ordinary queueing.
	db.SetMaxOpenConns(1)
	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating %s: %w", path, err)
	}
	_ = os.Chmod(path, 0o600)
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Path() string { return s.path }

func (s *Store) touch() {
	s.mu.Lock()
	s.gen++
	s.mu.Unlock()
}

// Generation changes whenever gate-visible state may have changed.
func (s *Store) Generation() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen
}

var migrations = []string{
	// 1: everything
	`
CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);

CREATE TABLE admins (
	id            INTEGER PRIMARY KEY,
	username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
	pass_hash     TEXT NOT NULL,
	totp_secret   TEXT NOT NULL DEFAULT '',
	totp_enabled  INTEGER NOT NULL DEFAULT 0,
	created_at    INTEGER NOT NULL,
	last_login_at INTEGER
);

CREATE TABLE sessions (
	token_hash TEXT PRIMARY KEY,
	kind       TEXT NOT NULL,           -- 'admin' or 'user'
	subject    INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	expires_at INTEGER NOT NULL,
	ip         TEXT NOT NULL DEFAULT '',
	agent      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX sessions_subject ON sessions(kind, subject);

CREATE TABLE plans (
	id            INTEGER PRIMARY KEY,
	name          TEXT NOT NULL,
	description   TEXT NOT NULL DEFAULT '',
	price         INTEGER NOT NULL DEFAULT 0,
	quota_bytes   INTEGER NOT NULL DEFAULT 0,  -- 0: unlimited
	duration_days INTEGER NOT NULL DEFAULT 30, -- 0: never ends
	renewal       TEXT NOT NULL DEFAULT 'expire', -- 'expire' or 'reset'
	speed_kbps    INTEGER NOT NULL DEFAULT 0,
	max_devices   INTEGER NOT NULL DEFAULT 2,
	groups_json   TEXT NOT NULL DEFAULT '[]',
	excluded_json TEXT NOT NULL DEFAULT '[]',
	public        INTEGER NOT NULL DEFAULT 1,
	is_default    INTEGER NOT NULL DEFAULT 0,
	color         TEXT NOT NULL DEFAULT '',
	sort          INTEGER NOT NULL DEFAULT 0,
	created_at    INTEGER NOT NULL
);

CREATE TABLE users (
	id           INTEGER PRIMARY KEY,
	username     TEXT NOT NULL UNIQUE COLLATE NOCASE,
	display_name TEXT NOT NULL DEFAULT '',
	phone        TEXT NOT NULL DEFAULT '',
	pass_hash    TEXT NOT NULL DEFAULT '',
	plan_id      INTEGER REFERENCES plans(id) ON DELETE SET NULL,
	enabled      INTEGER NOT NULL DEFAULT 1,
	quota_bytes  INTEGER NOT NULL DEFAULT 0,
	used_bytes   INTEGER NOT NULL DEFAULT 0,
	period_start INTEGER,
	expires_at   INTEGER,
	speed_kbps   INTEGER NOT NULL DEFAULT 0,
	max_devices  INTEGER NOT NULL DEFAULT 2,
	note         TEXT NOT NULL DEFAULT '',
	created_at   INTEGER NOT NULL,
	last_seen_at INTEGER
);

CREATE TABLE devices (
	id           INTEGER PRIMARY KEY,
	user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	ip           TEXT NOT NULL UNIQUE,
	label        TEXT NOT NULL DEFAULT '',
	added_at     INTEGER NOT NULL,
	last_seen_at INTEGER
);
CREATE INDEX devices_user ON devices(user_id);

CREATE TABLE usage_daily (
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	day     TEXT NOT NULL,
	bytes   INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (user_id, day)
);

CREATE TABLE traffic (
	ts      INTEGER NOT NULL,  -- start of a 5-minute bucket
	gate_id TEXT NOT NULL,
	up      INTEGER NOT NULL DEFAULT 0,
	down    INTEGER NOT NULL DEFAULT 0,
	PRIMARY KEY (ts, gate_id)
);

CREATE TABLE payments (
	id           INTEGER PRIMARY KEY,
	user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	plan_id      INTEGER REFERENCES plans(id) ON DELETE SET NULL,
	amount       INTEGER NOT NULL DEFAULT 0,
	note         TEXT NOT NULL DEFAULT '',
	receipt      BLOB,
	receipt_type TEXT NOT NULL DEFAULT '',
	status       TEXT NOT NULL DEFAULT 'pending',
	admin_note   TEXT NOT NULL DEFAULT '',
	created_at   INTEGER NOT NULL,
	decided_at   INTEGER,
	decided_by   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX payments_status ON payments(status, created_at);

CREATE TABLE gates (
	id           TEXT PRIMARY KEY,
	name         TEXT NOT NULL,
	secret_hash  TEXT NOT NULL,
	public_ip    TEXT NOT NULL DEFAULT '',
	seen_ip      TEXT NOT NULL DEFAULT '',
	version      TEXT NOT NULL DEFAULT '',
	enforce      TEXT NOT NULL DEFAULT 'auto',
	last_seen_at INTEGER,
	stats_json   TEXT NOT NULL DEFAULT '{}',
	created_at   INTEGER NOT NULL
);

CREATE TABLE sync_batches (
	gate_id TEXT NOT NULL,
	batch   TEXT NOT NULL,
	at      INTEGER NOT NULL,
	PRIMARY KEY (gate_id, batch)
);

CREATE TABLE metrics (
	node      TEXT NOT NULL,
	ts        INTEGER NOT NULL,
	cpu       REAL NOT NULL DEFAULT 0,
	load1     REAL NOT NULL DEFAULT 0,
	mem_used  INTEGER NOT NULL DEFAULT 0,
	mem_total INTEGER NOT NULL DEFAULT 0,
	disk_used INTEGER NOT NULL DEFAULT 0,
	disk_total INTEGER NOT NULL DEFAULT 0,
	rx_bps    INTEGER NOT NULL DEFAULT 0,
	tx_bps    INTEGER NOT NULL DEFAULT 0,
	conns     INTEGER NOT NULL DEFAULT 0,
	uptime    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX metrics_node_ts ON metrics(node, ts);

CREATE TABLE custom_domains (
	domain   TEXT PRIMARY KEY,
	action   TEXT NOT NULL DEFAULT 'route', -- 'route' or 'direct'
	note     TEXT NOT NULL DEFAULT '',
	added_at INTEGER NOT NULL
);

CREATE TABLE activity (
	id     INTEGER PRIMARY KEY,
	at     INTEGER NOT NULL,
	actor  TEXT NOT NULL,
	kind   TEXT NOT NULL,
	detail TEXT NOT NULL DEFAULT ''
);
CREATE INDEX activity_at ON activity(at);
`,
}

func (s *Store) migrate() error {
	var v int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	for i := v; i < len(migrations); i++ {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("step %d: %w", i+1, err)
		}
		if _, err := tx.Exec("PRAGMA user_version = " + strconv.Itoa(i+1)); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// SchemaVersion is the migration count this binary knows.
func SchemaVersion() int { return len(migrations) }

// ---------------------------------------------------------------- settings

// Setting keys. Values are strings; typed accessors live on the callers.
const (
	SetBrand         = "brand"
	SetSupport       = "support"        // how customers reach the operator
	SetPaymentInfo   = "payment_info"   // card number, account holder, etc.
	SetSignupOpen    = "signup_open"    // "1" or "0"
	SetSignupPlan    = "signup_plan"    // plan id a new account starts on, "0" for none
	SetNotice        = "notice"         // announcement at the top of every customer page
	SetCurrency      = "currency"       // label shown next to prices
	SetDisabledRules = "disabled_rules" // JSON list of built-in rule ids switched off
	SetDNSAddress    = "dns_address"    // override for the DNS address customers are shown
)

var settingDefaults = map[string]string{
	SetBrand:         "Gozar",
	SetSignupOpen:    "1",
	SetSignupPlan:    "0",
	SetCurrency:      "تومان",
	SetDisabledRules: "[]",
}

func (s *Store) Setting(key string) string {
	var v string
	if err := s.db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&v); err != nil {
		return settingDefaults[key]
	}
	return v
}

func (s *Store) Settings() map[string]string {
	out := map[string]string{}
	for k, v := range settingDefaults {
		out[k] = v
	}
	rows, err := s.db.Query("SELECT key, value FROM settings")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var k, v string
		if rows.Scan(&k, &v) == nil {
			out[k] = v
		}
	}
	return out
}

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec("INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, value)
	s.touch()
	return err
}

// ---------------------------------------------------------------- activity

type Activity struct {
	ID     int64  `json:"id"`
	At     int64  `json:"at"`
	Actor  string `json:"actor"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

func (s *Store) Log(actor, kind, detail string) {
	_, _ = s.db.Exec("INSERT INTO activity(at, actor, kind, detail) VALUES(?, ?, ?, ?)", time.Now().Unix(), actor, kind, detail)
}

func (s *Store) Activity(limit int, before int64) ([]Activity, error) {
	if before <= 0 {
		before = 1 << 62
	}
	rows, err := s.db.Query("SELECT id, at, actor, kind, detail FROM activity WHERE id < ? ORDER BY id DESC LIMIT ?", before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Activity{}
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.At, &a.Actor, &a.Kind, &a.Detail); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- housekeeping

// Prune drops what is only worth keeping for a while.
func (s *Store) Prune() {
	now := time.Now().Unix()
	_, _ = s.db.Exec("DELETE FROM metrics WHERE ts < ?", now-7*86400)
	_, _ = s.db.Exec("DELETE FROM traffic WHERE ts < ?", now-90*86400)
	_, _ = s.db.Exec("DELETE FROM sync_batches WHERE at < ?", now-86400)
	_, _ = s.db.Exec("DELETE FROM sessions WHERE expires_at < ?", now)
	_, _ = s.db.Exec("DELETE FROM activity WHERE at < ?", now-180*86400)
	_, _ = s.db.Exec("DELETE FROM usage_daily WHERE day < ?", time.Now().AddDate(0, 0, -400).Format("2006-01-02"))
}

// ---------------------------------------------------------------- backup

// Backup writes a consistent copy of the whole database to dst. VACUUM INTO
// rather than a file copy: a live WAL database keeps recent rows in a
// separate log that a raw copy can miss.
func (s *Store) Backup(ctx context.Context, dst string) error {
	_ = os.Remove(dst)
	_, err := s.db.ExecContext(ctx, "VACUUM INTO ?", dst)
	return err
}

const restoreSuffix = ".restore"

// StageRestore checks src is a Gozar database and puts it where Open will
// pick it up on the next start. The caller restarts the process.
func (s *Store) StageRestore(src string) error {
	if err := CheckBackup(src); err != nil {
		return err
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path+restoreSuffix, b, 0o600)
}

// CheckBackup opens src read-only and confirms it looks like ours.
func CheckBackup(src string) error {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(src)+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var v int
	if err := db.QueryRow("PRAGMA user_version").Scan(&v); err != nil {
		return errors.New("this file is not a database")
	}
	if v < 1 || v > len(migrations) {
		return fmt.Errorf("this backup has schema version %d, which this version of Gozar cannot use", v)
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name IN ('users','plans','gates')").Scan(&n); err != nil || n != 3 {
		return errors.New("this database is not a Gozar backup")
	}
	var ok string
	if err := db.QueryRow("PRAGMA quick_check").Scan(&ok); err != nil || ok != "ok" {
		return errors.New("this backup is damaged")
	}
	return nil
}

func swapInRestore(path string) error {
	staged := path + restoreSuffix
	if _, err := os.Stat(staged); err != nil {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		keep := path + ".before-restore-" + time.Now().Format("20060102-150405")
		if err := os.Rename(path, keep); err != nil {
			return err
		}
	}
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
	return os.Rename(staged, path)
}

// ---------------------------------------------------------------- helpers

func nullInt(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func ptr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
