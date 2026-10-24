//go:build linux

package stats

import (
	"os"
	"strings"
	"time"
)

// The Linux sampler exists so netcli can be developed and demonstrated on
// a Linux workstation; the router target is NetBSD.

// Read takes one sample.
func Read() (*Sample, error) {
	s := &Sample{Time: time.Now(), Ifaces: map[string]IfCounters{}}
	data, err := os.ReadFile("/proc/net/dev")
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		f := strings.Fields(rest)
		if len(f) < 16 {
			continue
		}
		s.Ifaces[strings.TrimSpace(name)] = IfCounters{
			RxBytes: atou(f[0]), RxPkts: atou(f[1]), RxErrs: atou(f[2]),
			TxBytes: atou(f[8]), TxPkts: atou(f[9]), TxErrs: atou(f[10]),
		}
	}
	if data, err := os.ReadFile("/proc/stat"); err == nil {
		line, _, _ := strings.Cut(string(data), "\n")
		f := strings.Fields(line)
		for i, x := range f[1:] {
			n := atou(x)
			s.CPUTotal += n
			if i == 3 || i == 4 { // idle, iowait
				s.CPUIdle += n
			}
		}
	}
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(line)
			if len(f) < 2 {
				continue
			}
			switch f[0] {
			case "MemTotal:":
				s.MemTotal = atou(f[1]) * 1024
			case "MemAvailable:":
				s.MemFree = atou(f[1]) * 1024
			}
		}
	}
	return s, nil
}
