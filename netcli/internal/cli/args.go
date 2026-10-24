package cli

import (
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"github.com/netcli/netcli/internal/config"
)

func isIPv4(s string) bool {
	a, err := netip.ParseAddr(s)
	return err == nil && a.Is4()
}

func isIPv6(s string) bool {
	a, err := netip.ParseAddr(s)
	return err == nil && a.Is6() && !a.Is4In6()
}

func isPrefix4(s string) bool {
	p, err := netip.ParsePrefix(s)
	return err == nil && p.Addr().Is4()
}

func isPrefix6(s string) bool {
	p, err := netip.ParsePrefix(s)
	return err == nil && p.Addr().Is6()
}

func isMask(s string) bool {
	_, err := config.MaskToBits(s)
	return err == nil
}

func numRange(lo, hi int) func(string) bool {
	return func(s string) bool {
		n, err := strconv.Atoi(s)
		return err == nil && n >= lo && n <= hi
	}
}

var (
	tWord = &ArgType{Name: "WORD", Help: "Name", Match: func(s string) bool { return s != "" }}
	tLine = &ArgType{Name: "LINE", Help: "Text", Rest: true}

	tIPv4    = &ArgType{Name: "A.B.C.D", Help: "IPv4 address", Match: isIPv4}
	tMask    = &ArgType{Name: "A.B.C.D", Help: "Subnet mask", Match: isMask}
	tPrefix4 = &ArgType{Name: "A.B.C.D/nn", Help: "IPv4 prefix", Match: isPrefix4}
	tIPv6    = &ArgType{Name: "X:X:X:X::X", Help: "IPv6 address", Match: isIPv6}
	tPrefix6 = &ArgType{Name: "X:X:X:X::X/<0-128>", Help: "IPv6 prefix", Match: isPrefix6}
	tHost    = &ArgType{Name: "WORD", Help: "Host name or IP address", Match: func(s string) bool { return s != "" && !strings.HasPrefix(s, "-") }}

	tIfName = &ArgType{
		Name:     "IFNAME",
		Help:     "Interface name (wm0, vlan10, bridge0, ath0, wlan0 ...)",
		Match:    config.ValidInterfaceName,
		Complete: completeInterfaces,
	}
	tACLName = &ArgType{
		Name:  "WORD",
		Help:  "Access list name",
		Match: config.ValidName,
		Complete: func(s *Session, _ string) []string {
			c := s.running()
			var n []string
			for k := range c.AccessLists {
				n = append(n, k)
			}
			return n
		},
	}
	tPoolName = &ArgType{
		Name:  "WORD",
		Help:  "Pool name",
		Match: config.ValidName,
		Complete: func(s *Session, _ string) []string {
			c := s.running()
			var n []string
			for k := range c.DHCPPools {
				n = append(n, k)
			}
			return n
		},
	}
	tUserName = &ArgType{Name: "WORD", Help: "User name", Match: config.ValidName}
	tSecret   = &ArgType{Name: "LINE", Help: "The secret (password)", Match: func(s string) bool { return s != "" }}
	tMAC      = &ArgType{Name: "H:H:H:H:H:H", Help: "MAC address", Match: func(s string) bool {
		hw, err := net.ParseMAC(s)
		return err == nil && len(hw) == 6
	}}
)

func tNum(lo, hi int, help string) *ArgType {
	return &ArgType{Name: "<" + strconv.Itoa(lo) + "-" + strconv.Itoa(hi) + ">", Help: help, Match: numRange(lo, hi)}
}

func completeInterfaces(s *Session, _ string) []string {
	seen := map[string]bool{}
	if ifs, err := net.Interfaces(); err == nil {
		for _, i := range ifs {
			if config.ValidInterfaceName(i.Name) {
				seen[i.Name] = true
			}
		}
	}
	for n := range s.running().Interfaces {
		seen[n] = true
	}
	var out []string
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

// prefixFromArgs builds "addr/len" from either an A.B.C.D/nn argument or an
// address plus dotted mask.
func prefixFromArgs(a *Args, prefixKey, addrKey, maskKey string) string {
	if p := a.Get(prefixKey); p != "" {
		return p
	}
	bits, _ := config.MaskToBits(a.Get(maskKey))
	return a.Get(addrKey) + "/" + strconv.Itoa(bits)
}
