//go:build netbsd

package stats

import (
	"os/exec"
	"strings"
	"sync"
	"time"
)

// On NetBSD the counters come from netstat(1), sysctl(8) and vmstat(1).
// This avoids cgo and keeps the binary statically linked.

var memCache struct {
	sync.Mutex
	at          time.Time
	total, free uint64
}

// Read takes one sample.
func Read() (*Sample, error) {
	s := &Sample{Time: time.Now(), Ifaces: map[string]IfCounters{}}

	out, err := exec.Command("netstat", "-i", "-b", "-n").Output()
	if err != nil {
		return nil, err
	}
	parseNetstatBytes(string(out), s.Ifaces)
	if out, err := exec.Command("netstat", "-i", "-n").Output(); err == nil {
		parseNetstatPackets(string(out), s.Ifaces)
	}

	if out, err := exec.Command("sysctl", "-n", "kern.cp_time").Output(); err == nil {
		s.CPUTotal, s.CPUIdle = parseCPTime(string(out))
	}

	memCache.Lock()
	if time.Since(memCache.at) > 10*time.Second {
		memCache.total, memCache.free = readMem()
		memCache.at = time.Now()
	}
	s.MemTotal, s.MemFree = memCache.total, memCache.free
	memCache.Unlock()
	return s, nil
}

func readMem() (total, free uint64) {
	if out, err := exec.Command("sysctl", "-n", "hw.physmem64").Output(); err == nil {
		total = atou(strings.TrimSpace(string(out)))
	}
	out, err := exec.Command("vmstat", "-s").Output()
	if err != nil {
		return total, 0
	}
	var pageSize, pagesFree uint64
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		desc := strings.Join(f[1:], " ")
		switch desc {
		case "bytes per page":
			pageSize = atou(f[0])
		case "pages free":
			pagesFree = atou(f[0])
		}
	}
	return total, pageSize * pagesFree
}
