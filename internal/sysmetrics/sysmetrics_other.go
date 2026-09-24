//go:build !linux

package sysmetrics

import "time"

// Development builds on other systems report uptime only.
var started = time.Now()

func (s *Sampler) take() Sample {
	return Sample{Uptime: uint64(time.Since(started).Seconds())}
}
