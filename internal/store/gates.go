package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/amor7/gozarDNS/internal/auth"
	"github.com/amor7/gozarDNS/internal/proto"
	"github.com/amor7/gozarDNS/internal/sysmetrics"
)

type Gate struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	PublicIP   string          `json:"public_ip"`
	SeenIP     string          `json:"seen_ip"`
	Version    string          `json:"version"`
	Enforce    string          `json:"enforce"` // "auto", "on", "off"
	LastSeenAt *int64          `json:"last_seen_at"`
	Stats      json.RawMessage `json:"stats"`
	CreatedAt  int64           `json:"created_at"`
	Online     bool            `json:"online"`

	secretHash string
}

// GateStatus is what the last sync said about the gate.
type GateStatus struct {
	Host  sysmetrics.Sample `json:"host"`
	Stats proto.GateStats   `json:"stats"`
}

const gateCols = "id, name, secret_hash, public_ip, seen_ip, version, enforce, last_seen_at, stats_json, created_at"

func scanGate(row interface{ Scan(...any) error }) (*Gate, error) {
	g := &Gate{}
	var seen sql.NullInt64
	var stats string
	err := row.Scan(&g.ID, &g.Name, &g.secretHash, &g.PublicIP, &g.SeenIP, &g.Version, &g.Enforce, &seen, &stats, &g.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	g.LastSeenAt = ptr(seen)
	g.Stats = json.RawMessage(stats)
	g.Online = g.LastSeenAt != nil && time.Now().Unix()-*g.LastSeenAt < 120
	return g, nil
}

func (s *Store) Gates() ([]*Gate, error) {
	rows, err := s.db.Query("SELECT " + gateCols + " FROM gates ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Gate{}
	for rows.Next() {
		g, err := scanGate(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) Gate(id string) (*Gate, error) {
	return scanGate(s.db.QueryRow("SELECT "+gateCols+" FROM gates WHERE id = ?", id))
}

// CreateGate registers a gate and returns its secret, which is shown once.
func (s *Store) CreateGate(name string) (*Gate, string, error) {
	id := "g" + strings.ToLower(auth.Token(6))
	id = strings.NewReplacer("-", "x", "_", "y").Replace(id)
	secret := auth.Token(32)
	_, err := s.db.Exec("INSERT INTO gates(id, name, secret_hash, created_at) VALUES(?,?,?,?)",
		id, strings.TrimSpace(name), auth.HashToken(secret), time.Now().Unix())
	if err != nil {
		return nil, "", err
	}
	g, err := s.Gate(id)
	return g, secret, err
}

// RotateGateSecret invalidates the old credential and returns a new one.
func (s *Store) RotateGateSecret(id string) (string, error) {
	secret := auth.Token(32)
	res, err := s.db.Exec("UPDATE gates SET secret_hash = ? WHERE id = ?", auth.HashToken(secret), id)
	if err != nil {
		return "", err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return "", ErrNotFound
	}
	return secret, nil
}

// AuthGate checks a "<id>.<secret>" credential.
func (s *Store) AuthGate(cred string) (*Gate, bool) {
	id, secret, ok := strings.Cut(cred, ".")
	if !ok {
		return nil, false
	}
	g, err := s.Gate(id)
	if err != nil {
		auth.HashToken(secret)
		return nil, false
	}
	if !constantEq(g.secretHash, auth.HashToken(secret)) {
		return nil, false
	}
	return g, true
}

func constantEq(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := 0; i < len(a); i++ {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

func (s *Store) UpdateGate(id string, name *string, enforce *string) error {
	if name != nil {
		if _, err := s.db.Exec("UPDATE gates SET name = ? WHERE id = ?", strings.TrimSpace(*name), id); err != nil {
			return err
		}
	}
	if enforce != nil {
		switch *enforce {
		case "auto", "on", "off":
		default:
			return errors.New("enforce must be auto, on or off")
		}
		if _, err := s.db.Exec("UPDATE gates SET enforce = ? WHERE id = ?", *enforce, id); err != nil {
			return err
		}
		s.touch()
	}
	return nil
}

func (s *Store) DeleteGate(id string) error {
	res, err := s.db.Exec("DELETE FROM gates WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GateIPs lists every address a gate has reported or been seen from. The
// hub's egress answers these and nobody else.
func (s *Store) GateIPs() []string {
	rows, err := s.db.Query("SELECT public_ip, seen_ip FROM gates")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var a, b string
		if rows.Scan(&a, &b) == nil {
			for _, ip := range []string{a, b} {
				if ip != "" {
					out = append(out, ip)
				}
			}
		}
	}
	return out
}

// RecordSync folds one sync into the database: usage (once per batch), the
// gate's own status, and last-seen times for the addresses that were active.
// It returns false when the batch had already been applied.
func (s *Store) RecordSync(g *Gate, seenIP string, req *proto.SyncRequest) (bool, error) {
	now := time.Now()
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	status, _ := json.Marshal(GateStatus{Host: req.Host, Stats: req.Stats})
	if _, err := tx.Exec("UPDATE gates SET public_ip = ?, seen_ip = ?, version = ?, last_seen_at = ?, stats_json = ? WHERE id = ?",
		req.PublicIP, seenIP, req.Version, now.Unix(), string(status), g.ID); err != nil {
		return false, err
	}
	if ipChanged := g.SeenIP != seenIP || g.PublicIP != req.PublicIP; ipChanged {
		s.touch()
	}

	fresh := true
	if req.Batch != "" {
		res, err := tx.Exec("INSERT OR IGNORE INTO sync_batches(gate_id, batch, at) VALUES(?,?,?)", g.ID, req.Batch, now.Unix())
		if err != nil {
			return false, err
		}
		n, _ := res.RowsAffected()
		fresh = n == 1
	}
	if fresh {
		day := now.Format("2006-01-02")
		bucket := now.Unix() / 300 * 300
		var up, down int64
		for uid, u := range req.Usage {
			if u.Up < 0 || u.Down < 0 {
				continue
			}
			total := u.Up + u.Down
			up += u.Up
			down += u.Down
			if uid <= 0 || total == 0 {
				continue
			}
			if _, err := tx.Exec("UPDATE users SET used_bytes = used_bytes + ?, last_seen_at = ? WHERE id = ?", total, now.Unix(), uid); err != nil {
				return false, err
			}
			if _, err := tx.Exec(`INSERT INTO usage_daily(user_id, day, bytes) SELECT ?, ?, ? WHERE EXISTS (SELECT 1 FROM users WHERE id = ?)
				ON CONFLICT(user_id, day) DO UPDATE SET bytes = bytes + excluded.bytes`, uid, day, total, uid); err != nil {
				return false, err
			}
		}
		if up+down > 0 {
			if _, err := tx.Exec(`INSERT INTO traffic(ts, gate_id, up, down) VALUES(?,?,?,?)
				ON CONFLICT(ts, gate_id) DO UPDATE SET up = up + excluded.up, down = down + excluded.down`,
				bucket, g.ID, up, down); err != nil {
				return false, err
			}
		}
		for ip, at := range req.Seen {
			if _, err := tx.Exec("UPDATE devices SET last_seen_at = ? WHERE ip = ? AND (last_seen_at IS NULL OR last_seen_at < ?)", at, ip, at); err != nil {
				return false, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	if fresh && len(req.Usage) > 0 {
		s.touch() // someone may have just run out
	}
	return fresh, nil
}

// ---------------------------------------------------------------- sessions

type Session struct {
	Kind    string
	Subject int64
	Expires int64
}

func (s *Store) CreateSession(kind string, subject int64, ttl time.Duration, ip, agent string) (string, error) {
	tok := auth.Token(32)
	now := time.Now()
	if len(agent) > 200 {
		agent = agent[:200]
	}
	_, err := s.db.Exec("INSERT INTO sessions(token_hash, kind, subject, created_at, expires_at, ip, agent) VALUES(?,?,?,?,?,?,?)",
		auth.HashToken(tok), kind, subject, now.Unix(), now.Add(ttl).Unix(), ip, agent)
	return tok, err
}

func (s *Store) Session(kind, tok string) (*Session, bool) {
	if tok == "" {
		return nil, false
	}
	var ss Session
	err := s.db.QueryRow("SELECT kind, subject, expires_at FROM sessions WHERE token_hash = ? AND kind = ?",
		auth.HashToken(tok), kind).Scan(&ss.Kind, &ss.Subject, &ss.Expires)
	if err != nil || ss.Expires < time.Now().Unix() {
		return nil, false
	}
	return &ss, true
}

func (s *Store) DeleteSession(tok string) {
	_, _ = s.db.Exec("DELETE FROM sessions WHERE token_hash = ?", auth.HashToken(tok))
}

// ---------------------------------------------------------------- admins

type Admin struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	TOTPEnabled bool   `json:"totp_enabled"`
	CreatedAt   int64  `json:"created_at"`
	LastLoginAt *int64 `json:"last_login_at"`

	PassHash   string `json:"-"`
	TOTPSecret string `json:"-"`
}

const adminCols = "id, username, pass_hash, totp_secret, totp_enabled, created_at, last_login_at"

func scanAdmin(row interface{ Scan(...any) error }) (*Admin, error) {
	a := &Admin{}
	var en int
	var last sql.NullInt64
	err := row.Scan(&a.ID, &a.Username, &a.PassHash, &a.TOTPSecret, &en, &a.CreatedAt, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	a.TOTPEnabled, a.LastLoginAt = en == 1, ptr(last)
	return a, err
}

func (s *Store) Admin(id int64) (*Admin, error) {
	return scanAdmin(s.db.QueryRow("SELECT "+adminCols+" FROM admins WHERE id = ?", id))
}

func (s *Store) AdminByName(name string) (*Admin, error) {
	return scanAdmin(s.db.QueryRow("SELECT "+adminCols+" FROM admins WHERE username = ?", strings.TrimSpace(name)))
}

func (s *Store) Admins() ([]*Admin, error) {
	rows, err := s.db.Query("SELECT " + adminCols + " FROM admins ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Admin{}
	for rows.Next() {
		a, err := scanAdmin(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) CreateAdmin(name, hash string) (*Admin, error) {
	res, err := s.db.Exec("INSERT INTO admins(username, pass_hash, created_at) VALUES(?,?,?)", strings.TrimSpace(name), hash, time.Now().Unix())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrTaken
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.Admin(id)
}

func (s *Store) SetAdminPassword(id int64, hash string) error {
	_, err := s.db.Exec("UPDATE admins SET pass_hash = ? WHERE id = ?", hash, id)
	if err == nil {
		_, err = s.db.Exec("DELETE FROM sessions WHERE kind = 'admin' AND subject = ?", id)
	}
	return err
}

func (s *Store) SetAdminTOTP(id int64, secret string, enabled bool) error {
	_, err := s.db.Exec("UPDATE admins SET totp_secret = ?, totp_enabled = ? WHERE id = ?", secret, boolInt(enabled), id)
	return err
}

func (s *Store) TouchAdminLogin(id int64) {
	_, _ = s.db.Exec("UPDATE admins SET last_login_at = ? WHERE id = ?", time.Now().Unix(), id)
}

func (s *Store) DeleteAdmin(id int64) error {
	var n int
	_ = s.db.QueryRow("SELECT count(*) FROM admins").Scan(&n)
	if n <= 1 {
		return errors.New("the last admin cannot be removed")
	}
	_, err := s.db.Exec("DELETE FROM admins WHERE id = ?", id)
	if err == nil {
		_, _ = s.db.Exec("DELETE FROM sessions WHERE kind = 'admin' AND subject = ?", id)
	}
	return err
}
