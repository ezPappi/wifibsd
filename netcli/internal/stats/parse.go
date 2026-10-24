package stats

import (
	"strconv"
	"strings"
)

// Parsers for NetBSD command output. They live in a file without build
// constraints so they can be unit tested on any platform.

func linkLine(line string) (name string, rest []string, ok bool) {
	f := strings.Fields(line)
	if len(f) < 4 {
		return "", nil, false
	}
	for i, x := range f {
		if strings.HasPrefix(x, "<Link") {
			name = strings.TrimSuffix(f[0], "*")
			return name, f[i+1:], true
		}
	}
	return "", nil, false
}

// parseNetstatBytes parses "netstat -ibn": the last two columns of the
// <Link> lines are Ibytes and Obytes.
func parseNetstatBytes(out string, m map[string]IfCounters) {
	for _, line := range strings.Split(out, "\n") {
		name, rest, ok := linkLine(line)
		if !ok || len(rest) < 2 {
			continue
		}
		c := m[name]
		c.RxBytes = atou(rest[len(rest)-2])
		c.TxBytes = atou(rest[len(rest)-1])
		m[name] = c
	}
}

// parseNetstatPackets parses "netstat -in": the last five columns are
// Ipkts Ierrs Opkts Oerrs Colls.
func parseNetstatPackets(out string, m map[string]IfCounters) {
	for _, line := range strings.Split(out, "\n") {
		name, rest, ok := linkLine(line)
		if !ok || len(rest) < 5 {
			continue
		}
		n := len(rest)
		c := m[name]
		c.RxPkts = atou(rest[n-5])
		c.RxErrs = atou(rest[n-4])
		c.TxPkts = atou(rest[n-3])
		c.TxErrs = atou(rest[n-2])
		m[name] = c
	}
}

// parseCPTime parses "user = 1, nice = 0, sys = 2, intr = 0, idle = 9"
// (or plain numbers). Idle is the last counter.
func parseCPTime(out string) (total, idle uint64) {
	var nums []uint64
	for _, f := range strings.FieldsFunc(out, func(r rune) bool { return r < '0' || r > '9' }) {
		nums = append(nums, atou(f))
	}
	for _, n := range nums {
		total += n
	}
	if len(nums) > 0 {
		idle = nums[len(nums)-1]
	}
	return
}

func atou(s string) uint64 {
	n, _ := strconv.ParseUint(s, 10, 64)
	return n
}
