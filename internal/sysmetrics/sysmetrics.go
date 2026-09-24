// Package sysmetrics samples the health of the machine it runs on.
package sysmetrics

import (
	"sync"
	"time"
)

// Sample is one reading. Rates are averaged since the previous reading.
type Sample struct {
	CPU       float64 `json:"cpu"` // percent, all cores
	Load1     float64 `json:"load1"`
	MemUsed   uint64  `json:"mem_used"`
	MemTotal  uint64  `json:"mem_total"`
	DiskUsed  uint64  `json:"disk_used"`
	DiskTotal uint64  `json:"disk_total"`
	RxBps     uint64  `json:"rx_bps"` // bytes per second
	TxBps     uint64  `json:"tx_bps"`
	Uptime    uint64  `json:"uptime"` // seconds
	Conns     int64   `json:"conns"`  // proxied connections open right now
}

// Sampler keeps the previous counters so each Sample can report rates.
type Sampler struct {
	mu       sync.Mutex
	at       time.Time
	cpuIdle  uint64
	cpuTotal uint64
	rx, tx   uint64
	path     string
}

// New samples disk usage of the filesystem holding path.
func New(path string) *Sampler {
	s := &Sampler{path: path}
	s.Take() // prime the counters
	return s
}

func (s *Sampler) Take() Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.take()
}
