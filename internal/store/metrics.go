package store

import (
	"time"

	"github.com/amor7/gozar/internal/sysmetrics"
)

func (s *Store) RecordMetrics(node string, m sysmetrics.Sample) {
	_, _ = s.db.Exec(`INSERT INTO metrics(node, ts, cpu, load1, mem_used, mem_total, disk_used, disk_total, rx_bps, tx_bps, conns, uptime)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, node, time.Now().Unix(), m.CPU, m.Load1, m.MemUsed, m.MemTotal,
		m.DiskUsed, m.DiskTotal, m.RxBps, m.TxBps, m.Conns, m.Uptime)
}

type MetricPoint struct {
	TS    int64   `json:"ts"`
	CPU   float64 `json:"cpu"`
	Mem   float64 `json:"mem"` // percent
	RxBps int64   `json:"rx"`
	TxBps int64   `json:"tx"`
	Conns int64   `json:"conns"`
}

// Metrics returns averaged points for node over the last span, in about
// `points` buckets.
func (s *Store) Metrics(node string, span time.Duration, points int) ([]MetricPoint, error) {
	from := time.Now().Add(-span).Unix()
	step := max(int64(span.Seconds())/int64(points), 30)
	rows, err := s.db.Query(`SELECT (ts / ?) * ?, avg(cpu), avg(CASE WHEN mem_total > 0 THEN 100.0 * mem_used / mem_total ELSE 0 END),
		avg(rx_bps), avg(tx_bps), max(conns) FROM metrics WHERE node = ? AND ts >= ? GROUP BY ts / ? ORDER BY 1`,
		step, step, node, from, step)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MetricPoint{}
	for rows.Next() {
		var p MetricPoint
		var rx, tx float64
		if err := rows.Scan(&p.TS, &p.CPU, &p.Mem, &rx, &tx, &p.Conns); err != nil {
			return nil, err
		}
		p.RxBps, p.TxBps = int64(rx), int64(tx)
		p.CPU = float64(int(p.CPU*10)) / 10
		p.Mem = float64(int(p.Mem*10)) / 10
		out = append(out, p)
	}
	return out, rows.Err()
}

type TrafficPoint struct {
	TS   int64 `json:"ts"`
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

// Traffic sums customer traffic across all gates in buckets of step.
func (s *Store) Traffic(span, step time.Duration) ([]TrafficPoint, error) {
	st := int64(step.Seconds())
	now := time.Now().Unix()
	from := (now - int64(span.Seconds())) / st * st
	rows, err := s.db.Query("SELECT (ts / ?) * ?, sum(up), sum(down) FROM traffic WHERE ts >= ? GROUP BY ts / ? ORDER BY 1", st, st, from, st)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	got := map[int64]TrafficPoint{}
	for rows.Next() {
		var p TrafficPoint
		if rows.Scan(&p.TS, &p.Up, &p.Down) == nil {
			got[p.TS] = p
		}
	}
	out := []TrafficPoint{}
	for t := from; t <= now; t += st {
		p := got[t]
		p.TS = t
		out = append(out, p)
	}
	return out, nil
}

// Totals is the headline numbers for the dashboard.
type Totals struct {
	Users        int   `json:"users"`
	Active       int   `json:"active"`
	Devices      int   `json:"devices"`
	OnlineNow    int   `json:"online_now"` // devices seen in the last 5 minutes
	TrafficToday int64 `json:"traffic_today"`
	TrafficMonth int64 `json:"traffic_month"`
	Pending      int   `json:"pending_payments"`
	RevenueMonth int64 `json:"revenue_month"`
	ExpiringSoon int   `json:"expiring_soon"` // active, ending within 3 days
}

func (s *Store) Totals() Totals {
	var t Totals
	now := time.Now()
	users, _, _ := s.Users(UserFilter{})
	t.Users = len(users)
	for _, u := range users {
		if u.State == StateActive {
			t.Active++
			if u.ExpiresAt != nil && *u.ExpiresAt-now.Unix() < 3*86400 {
				t.ExpiringSoon++
			}
		}
	}
	t.Devices = s.DeviceCount()
	_ = s.db.QueryRow("SELECT count(*) FROM devices WHERE last_seen_at >= ?", now.Add(-5*time.Minute).Unix()).Scan(&t.OnlineNow)
	_ = s.db.QueryRow("SELECT COALESCE(sum(bytes),0) FROM usage_daily WHERE day = ?", now.Format("2006-01-02")).Scan(&t.TrafficToday)
	_ = s.db.QueryRow("SELECT COALESCE(sum(bytes),0) FROM usage_daily WHERE day >= ?", now.AddDate(0, 0, -29).Format("2006-01-02")).Scan(&t.TrafficMonth)
	t.Pending = s.PendingPayments()
	t.RevenueMonth = s.RevenueSince(now.AddDate(0, 0, -30).Unix())
	return t
}
