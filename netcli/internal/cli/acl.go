package cli

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/netcli/netcli/internal/config"
)

var portNames = map[string]int{
	"ftp-data": 20, "ftp": 21, "ssh": 22, "telnet": 23, "smtp": 25, "domain": 53,
	"bootps": 67, "bootpc": 68, "tftp": 69, "www": 80, "http": 80, "pop3": 110,
	"ntp": 123, "imap": 143, "snmp": 161, "snmptrap": 162, "bgp": 179, "ldap": 389,
	"https": 443, "isakmp": 500, "syslog": 514, "submission": 587, "imaps": 993,
	"pop3s": 995, "openvpn": 1194, "non500-isakmp": 4500, "wireguard": 51820,
}

var icmpNames = map[string]int{
	"echo-reply": 0, "unreachable": 3, "redirect": 5, "echo": 8,
	"router-advertisement": 9, "router-solicitation": 10, "time-exceeded": 11,
	"parameter-problem": 12, "timestamp-request": 13, "timestamp-reply": 14,
}

var icmp6Names = map[string]int{
	"unreachable": 1, "packet-too-big": 2, "time-exceeded": 3, "parameter-problem": 4,
	"echo": 128, "echo-request": 128, "echo-reply": 129, "router-solicitation": 133,
	"router-advertisement": 134, "nd-ns": 135, "nd-na": 136,
}

// aclRuleHelp is shown for "permit ?" and "deny ?".
const aclRuleHelp = "PROTO SRC [PORTS] DST [PORTS] [ICMP-TYPE] [log] [stateless]\n" +
	"      PROTO: ip | tcp | udp | icmp | icmpv6 | <0-255>\n" +
	"      SRC/DST: any | host A.B.C.D | A.B.C.D WILDCARD | A.B.C.D/nn | X:X::X/nn\n" +
	"      PORTS: eq P | range P1 P2 | gt P | lt P   (P is a number or name like www)"

type aceParser struct {
	toks []string
	i    int
}

func (p *aceParser) next() (string, bool) {
	if p.i >= len(p.toks) {
		return "", false
	}
	t := p.toks[p.i]
	p.i++
	return t, true
}

func (p *aceParser) peek() string {
	if p.i >= len(p.toks) {
		return ""
	}
	return p.toks[p.i]
}

// parseACE parses "permit|deny ..." (without sequence number).
func parseACE(action, rule string) (config.ACE, error) {
	e := config.ACE{Action: action}
	p := &aceParser{toks: strings.Fields(strings.ToLower(rule))}

	proto, ok := p.next()
	if !ok {
		return e, fmt.Errorf("protocol expected")
	}
	switch proto {
	case "ip", "ipv6":
		e.Protocol = "ip"
	case "tcp", "udp", "icmp", "icmpv6":
		e.Protocol = proto
	case "ipv6-icmp":
		e.Protocol = "icmpv6"
	default:
		n, err := strconv.Atoi(proto)
		if err != nil || n < 0 || n > 255 {
			return e, fmt.Errorf("unknown protocol %q", proto)
		}
		switch n {
		case 1:
			e.Protocol = "icmp"
		case 6:
			e.Protocol = "tcp"
		case 17:
			e.Protocol = "udp"
		case 58:
			e.Protocol = "icmpv6"
		default:
			e.Protocol = proto
		}
	}
	ports := e.Protocol == "tcp" || e.Protocol == "udp"

	var err error
	if e.Source, err = p.addr(); err != nil {
		return e, fmt.Errorf("source: %w", err)
	}
	if ports {
		if e.SourcePort, err = p.ports(); err != nil {
			return e, err
		}
	}
	if e.Destination, err = p.addr(); err != nil {
		return e, fmt.Errorf("destination: %w", err)
	}
	if ports {
		if e.DestinationPort, err = p.ports(); err != nil {
			return e, err
		}
	}
	if e.Protocol == "icmp" || e.Protocol == "icmpv6" {
		if t := p.peek(); t != "" && t != "log" && t != "stateless" {
			p.next()
			names := icmpNames
			if e.Protocol == "icmpv6" {
				names = icmp6Names
			}
			n, ok := names[t]
			if !ok {
				n, err = strconv.Atoi(t)
				if err != nil || n < 0 || n > 255 {
					return e, fmt.Errorf("unknown icmp type %q", t)
				}
			}
			e.ICMPType = &n
		}
	}
	for {
		t, ok := p.next()
		if !ok {
			break
		}
		switch t {
		case "log":
			e.Log = true
		case "stateless":
			e.Stateless = true
		default:
			return e, fmt.Errorf("unexpected %q", t)
		}
	}
	v := e
	v.Seq = 1 // the sequence number is assigned by the caller
	return e, v.Validate()
}

// addr parses: any | host A | A WILDCARD | A/nn
func (p *aceParser) addr() (string, error) {
	t, ok := p.next()
	if !ok {
		return "", fmt.Errorf("address expected")
	}
	switch t {
	case "any", "any4", "any6":
		return "any", nil
	case "host":
		h, ok := p.next()
		if !ok {
			return "", fmt.Errorf("host address expected")
		}
		a, err := netip.ParseAddr(h)
		if err != nil {
			return "", fmt.Errorf("invalid address %q", h)
		}
		return netip.PrefixFrom(a, a.BitLen()).String(), nil
	}
	if pr, err := netip.ParsePrefix(t); err == nil {
		if pr != pr.Masked() {
			return "", fmt.Errorf("%s has host bits set (did you mean %s?)", t, pr.Masked())
		}
		return pr.String(), nil
	}
	a, err := netip.ParseAddr(t)
	if err != nil {
		return "", fmt.Errorf("invalid address %q", t)
	}
	if a.Is4() && isIPv4(p.peek()) {
		wc, _ := p.next()
		bits, err := config.WildcardToBits(wc)
		if err != nil {
			return "", err
		}
		pr := netip.PrefixFrom(a, bits)
		if pr != pr.Masked() {
			return "", fmt.Errorf("%s %s has bits outside the wildcard", t, wc)
		}
		return pr.String(), nil
	}
	return netip.PrefixFrom(a, a.BitLen()).String(), nil
}

func port(t string) (int, error) {
	if n, ok := portNames[t]; ok {
		return n, nil
	}
	n, err := strconv.Atoi(t)
	if err != nil || n < 0 || n > 65535 {
		return 0, fmt.Errorf("invalid port %q", t)
	}
	return n, nil
}

// ports parses an optional port qualifier.
func (p *aceParser) ports() (string, error) {
	op := p.peek()
	switch op {
	case "eq", "gt", "lt", "range":
		p.next()
	case "neq":
		return "", fmt.Errorf("neq is not supported by NPF; use two ranges")
	default:
		return "", nil
	}
	a, ok := p.next()
	if !ok {
		return "", fmt.Errorf("port expected after %s", op)
	}
	lo, err := port(a)
	if err != nil {
		return "", err
	}
	switch op {
	case "eq":
		return strconv.Itoa(lo), nil
	case "gt":
		if lo >= 65535 {
			return "", fmt.Errorf("gt %d matches nothing", lo)
		}
		return fmt.Sprintf("%d-65535", lo+1), nil
	case "lt":
		if lo <= 1 {
			return "", fmt.Errorf("lt %d matches nothing", lo)
		}
		return fmt.Sprintf("1-%d", lo-1), nil
	}
	b, ok := p.next()
	if !ok {
		return "", fmt.Errorf("end of range expected")
	}
	hi, err := port(b)
	if err != nil {
		return "", err
	}
	if hi < lo {
		return "", fmt.Errorf("range end %d is below start %d", hi, lo)
	}
	if hi == lo {
		return strconv.Itoa(lo), nil
	}
	return fmt.Sprintf("%d-%d", lo, hi), nil
}

// formatACE renders an entry in IOS syntax (without sequence number).
func formatACE(e config.ACE) string {
	parts := []string{e.Action, e.Protocol, fmtAddr(e.Source)}
	if e.SourcePort != "" {
		parts = append(parts, fmtPorts(e.SourcePort))
	}
	parts = append(parts, fmtAddr(e.Destination))
	if e.DestinationPort != "" {
		parts = append(parts, fmtPorts(e.DestinationPort))
	}
	if e.ICMPType != nil {
		parts = append(parts, icmpName(e.Protocol, *e.ICMPType))
	}
	if e.Log {
		parts = append(parts, "log")
	}
	if e.Stateless {
		parts = append(parts, "stateless")
	}
	return strings.Join(parts, " ")
}

func icmpName(proto string, t int) string {
	names := icmpNames
	if proto == "icmpv6" {
		names = icmp6Names
	}
	best := ""
	for n, v := range names {
		if v == t && (best == "" || len(n) < len(best) || (len(n) == len(best) && n < best)) {
			best = n
		}
	}
	if best != "" {
		return best
	}
	return strconv.Itoa(t)
}

func fmtAddr(a string) string {
	if a == "any" {
		return "any"
	}
	p, err := netip.ParsePrefix(a)
	if err != nil {
		return a
	}
	if p.IsSingleIP() {
		return "host " + p.Addr().String()
	}
	if p.Addr().Is4() {
		var m uint32
		if p.Bits() > 0 {
			m = ^uint32(0) << (32 - p.Bits())
		}
		w := ^m
		return fmt.Sprintf("%s %d.%d.%d.%d", p.Addr(), w>>24, (w>>16)&0xff, (w>>8)&0xff, w&0xff)
	}
	return p.String()
}

func fmtPorts(p string) string {
	lo, hi, err := config.ParsePortSpec(p)
	if err != nil {
		return p
	}
	if lo == hi {
		return "eq " + strconv.Itoa(lo)
	}
	return fmt.Sprintf("range %d %d", lo, hi)
}

// sameRule compares two entries ignoring the sequence number.
func sameRule(a, b config.ACE) bool {
	a.Seq, b.Seq = 0, 0
	return formatACE(a) == formatACE(b)
}
