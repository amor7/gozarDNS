package sysmetrics

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func (s *Sampler) take() Sample {
	var out Sample
	now := time.Now()

	idle, total := readCPU()
	if s.cpuTotal > 0 && total > s.cpuTotal {
		dt := float64(total - s.cpuTotal)
		di := float64(idle - s.cpuIdle)
		out.CPU = round1(100 * (1 - di/dt))
	}
	s.cpuIdle, s.cpuTotal = idle, total

	mem := readKV("/proc/meminfo")
	out.MemTotal = mem["MemTotal"] * 1024
	out.MemUsed = (mem["MemTotal"] - mem["MemAvailable"]) * 1024

	var st syscall.Statfs_t
	if syscall.Statfs(s.path, &st) == nil {
		out.DiskTotal = st.Blocks * uint64(st.Bsize)
		out.DiskUsed = (st.Blocks - st.Bfree) * uint64(st.Bsize)
	}

	rx, tx := readNet()
	if !s.at.IsZero() {
		secs := now.Sub(s.at).Seconds()
		if secs > 0 && rx >= s.rx && tx >= s.tx {
			out.RxBps = uint64(float64(rx-s.rx) / secs)
			out.TxBps = uint64(float64(tx-s.tx) / secs)
		}
	}
	s.rx, s.tx, s.at = rx, tx, now

	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		if f := strings.Fields(string(b)); len(f) > 0 {
			v, _ := strconv.ParseFloat(f[0], 64)
			out.Uptime = uint64(v)
		}
	}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		if f := strings.Fields(string(b)); len(f) > 0 {
			out.Load1, _ = strconv.ParseFloat(f[0], 64)
		}
	}
	return out
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }

func readCPU() (idle, total uint64) {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	if !sc.Scan() {
		return
	}
	fields := strings.Fields(sc.Text())
	if len(fields) < 5 || fields[0] != "cpu" {
		return
	}
	for i, v := range fields[1:] {
		n, _ := strconv.ParseUint(v, 10, 64)
		total += n
		if i == 3 || i == 4 { // idle, iowait
			idle += n
		}
	}
	return
}

func readKV(path string) map[string]uint64 {
	out := map[string]uint64{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(v)
		if len(fields) > 0 {
			out[k], _ = strconv.ParseUint(fields[0], 10, 64)
		}
	}
	return out
}

// readNet sums every interface except loopback and the usual virtual ones.
func readNet() (rx, tx uint64) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "lo" || strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "br-") {
			continue
		}
		fields := strings.Fields(rest)
		if len(fields) < 9 {
			continue
		}
		r, _ := strconv.ParseUint(fields[0], 10, 64)
		t, _ := strconv.ParseUint(fields[8], 10, 64)
		rx += r
		tx += t
	}
	return
}
