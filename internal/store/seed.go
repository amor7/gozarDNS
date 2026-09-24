package store

import (
	"github.com/amor7/gozarDNS/internal/sysmetrics"
)

// Seeding helpers: they write history directly, for the development stack
// and for tests. Nothing in the running service calls them.

func (s *Store) SeedUsage(userID int64, day string, n int64) error {
	_, err := s.db.Exec(`INSERT INTO usage_daily(user_id, day, bytes) VALUES(?,?,?)
		ON CONFLICT(user_id, day) DO UPDATE SET bytes = bytes + excluded.bytes`, userID, day, n)
	return err
}

func (s *Store) SeedTraffic(ts int64, gateID string, up, down int64) error {
	_, err := s.db.Exec(`INSERT INTO traffic(ts, gate_id, up, down) VALUES(?,?,?,?)
		ON CONFLICT(ts, gate_id) DO UPDATE SET up = up + excluded.up, down = down + excluded.down`, ts/300*300, gateID, up, down)
	return err
}

func (s *Store) SeedMetrics(node string, ts int64, m sysmetrics.Sample) error {
	_, err := s.db.Exec(`INSERT INTO metrics(node, ts, cpu, load1, mem_used, mem_total, disk_used, disk_total, rx_bps, tx_bps, conns, uptime)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, node, ts, m.CPU, m.Load1, m.MemUsed, m.MemTotal, m.DiskUsed, m.DiskTotal, m.RxBps, m.TxBps, m.Conns, m.Uptime)
	return err
}

func (s *Store) SeedGateSeen(id, publicIP string, at int64, statusJSON, version string) error {
	_, err := s.db.Exec("UPDATE gates SET public_ip = ?, seen_ip = ?, last_seen_at = ?, stats_json = ?, version = ? WHERE id = ?", publicIP, publicIP, at, statusJSON, version, id)
	return err
}

func (s *Store) SeedDeviceSeen(ip string, at int64) error {
	_, err := s.db.Exec("UPDATE devices SET last_seen_at = ? WHERE ip = ?", at, ip)
	return err
}

func (s *Store) SeedCreatedAt(userID, at int64) error {
	_, err := s.db.Exec("UPDATE users SET created_at = ? WHERE id = ?", at, userID)
	return err
}

func (s *Store) SeedPaymentTime(id, at int64) error {
	_, err := s.db.Exec("UPDATE payments SET created_at = ? WHERE id = ?", at, id)
	return err
}
