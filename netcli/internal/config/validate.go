package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	ifNameRe   = regexp.MustCompile(`^[a-z][a-z0-9]*[0-9]+$`)
	nameRe     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,63}$`)
	hostnameRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
	domainRe   = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)
	countryRe  = regexp.MustCompile(`^[A-Z]{2}$`)
)

// Interface kinds derived from the interface name.
const (
	KindPhysical = "physical"
	KindVLAN     = "vlan"
	KindBridge   = "bridge"
	KindWLAN     = "wlan" // cloned VAP (new wifi stack)
	KindLoopback = "loopback"
)

// InterfaceKind classifies an interface by name.
func InterfaceKind(name string) string {
	base := strings.TrimRight(name, "0123456789")
	switch base {
	case "vlan":
		return KindVLAN
	case "bridge":
		return KindBridge
	case "wlan":
		return KindWLAN
	case "lo":
		return KindLoopback
	}
	return KindPhysical
}

// IsCloned reports whether the interface must be created with
// "ifconfig <if> create".
func IsCloned(name string) bool {
	switch InterfaceKind(name) {
	case KindVLAN, KindBridge, KindWLAN:
		return true
	}
	return false
}

// ValidInterfaceName reports whether name looks like a NetBSD interface.
func ValidInterfaceName(name string) bool { return ifNameRe.MatchString(name) }

// ValidName reports whether name is usable for ACLs, pools and users.
func ValidName(name string) bool { return nameRe.MatchString(name) }

// SortInterfaceNames sorts names by driver then unit number (wm2 < wm10).
func SortInterfaceNames(names []string) {
	sort.Slice(names, func(i, j int) bool {
		bi, ui := splitIf(names[i])
		bj, uj := splitIf(names[j])
		ki, kj := kindOrder(names[i]), kindOrder(names[j])
		if ki != kj {
			return ki < kj
		}
		if bi != bj {
			return bi < bj
		}
		return ui < uj
	})
}

func kindOrder(name string) int {
	switch InterfaceKind(name) {
	case KindLoopback:
		return 0
	case KindPhysical:
		return 1
	case KindVLAN:
		return 2
	case KindWLAN:
		return 3
	case KindBridge:
		return 4
	}
	return 5
}

func splitIf(name string) (string, int) {
	base := strings.TrimRight(name, "0123456789")
	n, _ := strconv.Atoi(name[len(base):])
	return base, n
}

// ParsePrefix parses an address with prefix length ("192.0.2.1/24"). A bare
// address is accepted when allowBare is set and gets a host prefix.
func ParsePrefix(s string, allowBare bool) (netip.Prefix, error) {
	if p, err := netip.ParsePrefix(s); err == nil {
		return p, nil
	}
	if allowBare {
		if a, err := netip.ParseAddr(s); err == nil {
			return netip.PrefixFrom(a, a.BitLen()), nil
		}
	}
	return netip.Prefix{}, fmt.Errorf("invalid prefix %q", s)
}

// MaskToBits converts a dotted netmask ("255.255.255.0") to a prefix length.
func MaskToBits(mask string) (int, error) {
	ip := net.ParseIP(mask).To4()
	if ip == nil {
		return 0, fmt.Errorf("invalid netmask %q", mask)
	}
	ones, bits := net.IPMask(ip).Size()
	if bits == 0 {
		return 0, fmt.Errorf("non-contiguous netmask %q", mask)
	}
	return ones, nil
}

// WildcardToBits converts an IOS wildcard mask ("0.0.0.255") to a prefix
// length.
func WildcardToBits(wc string) (int, error) {
	ip := net.ParseIP(wc).To4()
	if ip == nil {
		return 0, fmt.Errorf("invalid wildcard %q", wc)
	}
	inv := net.IPv4(^ip[0], ^ip[1], ^ip[2], ^ip[3]).To4()
	ones, bits := net.IPMask(inv).Size()
	if bits == 0 {
		return 0, fmt.Errorf("non-contiguous wildcard %q", wc)
	}
	return ones, nil
}

// ParsePortSpec validates a port ("22") or range ("1024-65535").
func ParsePortSpec(s string) (lo, hi int, err error) {
	a, b, isRange := strings.Cut(s, "-")
	lo, err = strconv.Atoi(a)
	if err != nil || lo < 0 || lo > 65535 {
		return 0, 0, fmt.Errorf("invalid port %q", s)
	}
	hi = lo
	if isRange {
		hi, err = strconv.Atoi(b)
		if err != nil || hi < lo || hi > 65535 {
			return 0, 0, fmt.Errorf("invalid port range %q", s)
		}
	}
	return lo, hi, nil
}

// Validate checks the configuration for errors. All problems are reported.
func (c *Config) Validate() error {
	c.Normalize()
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if c.Hostname != "" && !hostnameRe.MatchString(c.Hostname) {
		add("hostname %q is invalid", c.Hostname)
	}
	if c.DomainName != "" && !domainRe.MatchString(c.DomainName) {
		add("domain-name %q is invalid", c.DomainName)
	}
	for _, ns := range c.NameServers {
		if _, err := netip.ParseAddr(ns); err != nil {
			add("name-server %q is not an IP address", ns)
		}
	}
	for u := range c.Users {
		if !ValidName(u) {
			add("user name %q is invalid", u)
		}
	}

	for _, name := range c.InterfaceNames() {
		c.validateInterface(name, c.Interfaces[name], add)
	}

	for i, r := range c.Routes {
		p, err := ParsePrefix(r.Prefix, false)
		if err != nil {
			add("route %d: %v", i+1, err)
			continue
		}
		if p != p.Masked() {
			add("route %s: prefix has host bits set (did you mean %s?)", r.Prefix, p.Masked())
		}
		if strings.EqualFold(r.NextHop, "null0") {
			continue
		}
		nh, err := netip.ParseAddr(r.NextHop)
		if err != nil {
			add("route %s: invalid next hop %q", r.Prefix, r.NextHop)
		} else if nh.Is4() != p.Addr().Is4() {
			add("route %s: next hop %s is a different address family", r.Prefix, r.NextHop)
		}
	}

	for name, acl := range c.AccessLists {
		if !ValidName(name) {
			add("access-list name %q is invalid", name)
		}
		seen := map[int]bool{}
		for _, e := range acl.Entries {
			if seen[e.Seq] {
				add("access-list %s: duplicate sequence %d", name, e.Seq)
			}
			seen[e.Seq] = true
			if err := e.Validate(); err != nil {
				add("access-list %s seq %d: %v", name, e.Seq, err)
			}
		}
	}

	for i, n := range c.NAT {
		if _, ok := c.Interfaces[n.Interface]; !ok {
			add("nat rule %d: interface %s is not configured", i+1, n.Interface)
		}
		switch n.Type {
		case NATOverload:
			if p, err := ParsePrefix(n.Source, false); err != nil {
				add("nat rule %d: %v", i+1, err)
			} else if p != p.Masked() {
				add("nat rule %d: source %s has host bits set", i+1, n.Source)
			}
		case NATStatic:
			if n.Protocol != "tcp" && n.Protocol != "udp" {
				add("nat rule %d: protocol must be tcp or udp", i+1)
			}
			if a, err := netip.ParseAddr(n.LocalAddr); err != nil || !a.Is4() {
				add("nat rule %d: invalid local address %q", i+1, n.LocalAddr)
			}
			if n.LocalPort < 1 || n.LocalPort > 65535 || n.PublicPort < 1 || n.PublicPort > 65535 {
				add("nat rule %d: ports must be 1-65535", i+1)
			}
		default:
			add("nat rule %d: unknown type %q", i+1, n.Type)
		}
	}

	for name, p := range c.DHCPPools {
		c.validatePool(name, p, add)
	}

	if c.Web.Port < 0 || c.Web.Port > 65535 {
		add("web port %d is invalid", c.Web.Port)
	}
	if c.Web.Bind != "" {
		if _, err := netip.ParseAddr(c.Web.Bind); err != nil {
			add("web bind address %q is invalid", c.Web.Bind)
		}
	}
	return errors.Join(errs...)
}

func (c *Config) validateInterface(name string, ifc *Interface, add func(string, ...any)) {
	if !ValidInterfaceName(name) {
		add("interface name %q is invalid", name)
		return
	}
	if ifc.MTU != 0 && (ifc.MTU < 576 || ifc.MTU > 9216) {
		add("interface %s: mtu %d out of range (576-9216)", name, ifc.MTU)
	}
	if strings.ContainsAny(ifc.Description, "\n\r") {
		add("interface %s: description must be a single line", name)
	}
	if ifc.DHCP && len(ifc.IPv4) > 0 {
		add("interface %s: both dhcp and static IPv4 addresses configured", name)
	}
	for _, a := range ifc.IPv4 {
		p, err := ParsePrefix(a, false)
		if err != nil || !p.Addr().Is4() {
			add("interface %s: invalid IPv4 address %q", name, a)
		}
	}
	for _, a := range ifc.IPv6 {
		p, err := ParsePrefix(a, false)
		if err != nil || !p.Addr().Is6() || p.Addr().Is4In6() {
			add("interface %s: invalid IPv6 address %q", name, a)
		}
	}
	for _, g := range []string{ifc.AccessGroupIn, ifc.AccessGroupOut} {
		if g == "" {
			continue
		}
		if _, ok := c.AccessLists[g]; !ok {
			add("interface %s: access-list %s does not exist", name, g)
		}
	}

	kind := InterfaceKind(name)
	if kind == KindVLAN {
		if ifc.VLAN == nil {
			add("interface %s: missing encapsulation dot1q", name)
		} else {
			if ifc.VLAN.ID < 1 || ifc.VLAN.ID > 4094 {
				add("interface %s: vlan id %d out of range", name, ifc.VLAN.ID)
			}
			if _, ok := c.Interfaces[ifc.VLAN.Parent]; !ok {
				add("interface %s: parent %s is not configured", name, ifc.VLAN.Parent)
			}
		}
	} else if ifc.VLAN != nil {
		add("interface %s: dot1q encapsulation is only valid on vlanN interfaces", name)
	}
	if kind == KindBridge {
		if ifc.Bridge != nil {
			for _, m := range ifc.Bridge.Members {
				if m == name {
					add("interface %s: bridge cannot be a member of itself", name)
				} else if _, ok := c.Interfaces[m]; !ok {
					add("interface %s: member %s is not configured", name, m)
				}
			}
		}
	} else if ifc.Bridge != nil {
		add("interface %s: bridge members are only valid on bridgeN interfaces", name)
	}
	if w := ifc.Wireless; w != nil {
		// Wireless settings are entered one command at a time, so only
		// values that are present are checked here; Ready() decides when
		// the interface is complete enough to be brought up.
		if kind != KindPhysical && kind != KindWLAN {
			add("interface %s: wireless settings are not valid here", name)
		}
		if kind == KindPhysical && w.Device != "" {
			add("interface %s: wireless device is only used with wlanN interfaces", name)
		}
		if w.Device != "" && !ValidInterfaceName(w.Device) {
			add("interface %s: invalid wireless device %q", name, w.Device)
		}
		switch w.Mode {
		case "", ModeStation, ModeHostAP:
		default:
			add("interface %s: wireless mode must be station or hostap", name)
		}
		if len(w.SSID) > 32 || strings.ContainsAny(w.SSID, "\n\r\x00") {
			add("interface %s: ssid must be at most 32 bytes on one line", name)
		}
		switch w.Security {
		case "", SecurityOpen, SecurityWPA2PSK:
		default:
			add("interface %s: unknown wireless security %q", name, w.Security)
		}
		if w.PSK != "" && (len(w.PSK) < 8 || len(w.PSK) > 63 || strings.ContainsAny(w.PSK, "\n\r\"")) {
			add("interface %s: psk must be 8-63 characters without quotes", name)
		}
		if w.Channel < 0 || w.Channel > 196 {
			add("interface %s: invalid channel %d", name, w.Channel)
		}
		switch w.HWMode {
		case "", "a", "b", "g":
		default:
			add("interface %s: hw-mode must be a, b or g", name)
		}
		if w.Country != "" && !countryRe.MatchString(w.Country) {
			add("interface %s: country must be a two letter code like NO", name)
		}
	}
}

// Ready reports whether the wireless settings are complete enough to start
// wpa_supplicant or hostapd. kind is the interface kind.
func (w *Wireless) Ready(kind string) bool {
	if w == nil || w.SSID == "" || w.Security == "" || (w.Mode != ModeStation && w.Mode != ModeHostAP) {
		return false
	}
	if w.Security == SecurityWPA2PSK && w.PSK == "" {
		return false
	}
	if kind == KindWLAN && w.Device == "" {
		return false
	}
	return true
}

// Missing describes what is still needed before Ready returns true.
func (w *Wireless) Missing(kind string) string {
	var m []string
	if kind == KindWLAN && w.Device == "" {
		m = append(m, "device")
	}
	if w.Mode != ModeStation && w.Mode != ModeHostAP {
		m = append(m, "mode")
	}
	if w.SSID == "" {
		m = append(m, "ssid")
	}
	if w.Security == "" {
		m = append(m, "security")
	}
	if w.Security == SecurityWPA2PSK && w.PSK == "" {
		m = append(m, "psk")
	}
	return strings.Join(m, ", ")
}

func (c *Config) validatePool(name string, p *DHCPPool, add func(string, ...any)) {
	if !ValidName(name) {
		add("dhcp pool name %q is invalid", name)
	}
	subnet, err := ParsePrefix(p.Network, false)
	if p.Network == "" {
		add("dhcp pool %s: set the network first", name)
		return
	}
	if err != nil || !subnet.Addr().Is4() {
		add("dhcp pool %s: network %q is invalid", name, p.Network)
		return
	}
	if subnet != subnet.Masked() {
		add("dhcp pool %s: network %s has host bits set", name, p.Network)
		subnet = subnet.Masked()
	}
	inNet := func(what, s string) {
		a, err := netip.ParseAddr(s)
		if err != nil {
			add("dhcp pool %s: %s %q is not an address", name, what, s)
		} else if !subnet.Contains(a) {
			add("dhcp pool %s: %s %s is outside %s", name, what, s, subnet)
		}
	}
	if (p.RangeStart == "") != (p.RangeEnd == "") {
		add("dhcp pool %s: range needs both start and end", name)
	} else if p.RangeStart != "" {
		inNet("range start", p.RangeStart)
		inNet("range end", p.RangeEnd)
		a, _ := netip.ParseAddr(p.RangeStart)
		b, _ := netip.ParseAddr(p.RangeEnd)
		if a.IsValid() && b.IsValid() && b.Less(a) {
			add("dhcp pool %s: range end is before range start", name)
		}
	}
	for _, r := range p.DefaultRouter {
		inNet("default-router", r)
	}
	for _, d := range p.DNSServers {
		if _, err := netip.ParseAddr(d); err != nil {
			add("dhcp pool %s: dns-server %q is invalid", name, d)
		}
	}
	for _, h := range p.Hosts {
		if !ValidName(h.Name) {
			add("dhcp pool %s: host name %q is invalid", name, h.Name)
		}
		if hw, err := parseMAC(h.MAC); err != nil || hw == "" {
			add("dhcp pool %s: host %s has invalid MAC %q", name, h.Name, h.MAC)
		}
		inNet("host "+h.Name, h.Address)
	}
}

func parseMAC(s string) (string, error) {
	hw, err := net.ParseMAC(s)
	if err != nil || len(hw) != 6 {
		return "", fmt.Errorf("invalid MAC %q", s)
	}
	return hw.String(), nil
}

// NormalizeMAC returns the canonical lower case colon form of a MAC.
func NormalizeMAC(s string) (string, error) { return parseMAC(s) }

// Validate checks one access-list entry.
func (e ACE) Validate() error {
	if e.Seq < 1 || e.Seq > 2147483647 {
		return fmt.Errorf("invalid sequence number")
	}
	if e.Action != "permit" && e.Action != "deny" {
		return fmt.Errorf("action must be permit or deny")
	}
	ports := false
	switch e.Protocol {
	case "ip", "icmp", "icmpv6":
	case "tcp", "udp":
		ports = true
	default:
		n, err := strconv.Atoi(e.Protocol)
		if err != nil || n < 0 || n > 255 {
			return fmt.Errorf("unknown protocol %q", e.Protocol)
		}
	}
	if !ports && (e.SourcePort != "" || e.DestinationPort != "") {
		return fmt.Errorf("ports are only valid for tcp and udp")
	}
	for _, p := range []string{e.SourcePort, e.DestinationPort} {
		if p != "" {
			if _, _, err := ParsePortSpec(p); err != nil {
				return err
			}
		}
	}
	if e.ICMPType != nil && e.Protocol != "icmp" && e.Protocol != "icmpv6" {
		return fmt.Errorf("icmp type is only valid for icmp")
	}
	if e.ICMPType != nil && (*e.ICMPType < 0 || *e.ICMPType > 255) {
		return fmt.Errorf("invalid icmp type")
	}
	fam := 0
	for _, a := range []string{e.Source, e.Destination} {
		if a == "any" {
			continue
		}
		p, err := ParsePrefix(a, false)
		if err != nil {
			return err
		}
		f := 6
		if p.Addr().Is4() {
			f = 4
		}
		if fam != 0 && fam != f {
			return fmt.Errorf("source and destination are different address families")
		}
		fam = f
	}
	if e.Protocol == "icmp" && fam == 6 {
		return fmt.Errorf("use icmpv6 for IPv6 addresses")
	}
	if e.Protocol == "icmpv6" && fam == 4 {
		return fmt.Errorf("icmpv6 cannot match IPv4 addresses")
	}
	if e.Stateless && e.Action != "permit" {
		return fmt.Errorf("stateless only applies to permit entries")
	}
	return nil
}
