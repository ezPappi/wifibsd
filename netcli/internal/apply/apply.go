// Package apply moves the system from one configuration to another with the
// smallest set of changes: only interfaces, routes and services whose
// configuration changed are touched.
package apply

import (
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/netcli/netcli/internal/config"
	"github.com/netcli/netcli/internal/render"
	"github.com/netcli/netcli/internal/system"
)

// Applier applies configuration changes through an Executor.
type Applier struct {
	Exec  system.Executor
	Paths system.Paths
}

type step struct {
	a    *Applier
	boot bool
	errs []error
}

// run executes a command; tolerated errors (an object that already exists
// at boot, or one that is already gone when deleting) are ignored.
func (s *step) run(tolerate []string, name string, args ...string) bool {
	_, err := s.a.Exec.Run(name, args...)
	if err == nil {
		return true
	}
	msg := strings.ToLower(err.Error())
	for _, t := range tolerate {
		if strings.Contains(msg, t) {
			return true
		}
	}
	s.errs = append(s.errs, err)
	return false
}

func (s *step) must(name string, args ...string) bool { return s.run(nil, name, args...) }

func (s *step) fail(err error) {
	if err != nil {
		s.errs = append(s.errs, err)
	}
}

var (
	tolExists  = []string{"exists"}
	tolMissing = []string{"not found", "no such", "can't assign", "not in table", "does not exist", "not running"}
)

// Apply moves the system from old to new. With boot set, old is treated
// as an empty configuration and "already exists" errors are tolerated.
func (a *Applier) Apply(old, new *config.Config, boot bool) error {
	if old == nil || boot {
		old = config.New()
		old.Hostname = ""
	}
	old, new = Effective(old), Effective(new)
	s := &step{a: a, boot: boot}

	a.system(s, old, new)
	a.interfaces(s, old, new)
	a.routes(s, old, new)
	a.firewall(s, old, new)
	a.dhcpServer(s, old, new)
	return errors.Join(s.errs...)
}

func (a *Applier) system(s *step, old, new *config.Config) {
	if new.Hostname != "" && new.Hostname != old.Hostname {
		s.must("hostname", new.Hostname)
	}
	if new.DomainName != old.DomainName || !slices.Equal(new.NameServers, old.NameServers) {
		if len(new.NameServers) > 0 {
			s.fail(a.Exec.WriteFile("/etc/resolv.conf", []byte(render.Resolv(new)), 0o644))
		}
	}
	if new.IPRouting != old.IPRouting || s.boot {
		s.must("sysctl", "-w", "net.inet.ip.forwarding="+b01(new.IPRouting))
	}
	if new.IPv6Routing != old.IPv6Routing || s.boot {
		s.must("sysctl", "-w", "net.inet6.ip6.forwarding="+b01(new.IPv6Routing))
	}
}

func b01(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// Effective returns the configuration as it should be applied: wireless
// settings that are still incomplete are left out, and wlanN interfaces
// whose radio or mode is not known yet are not created.
func Effective(c *config.Config) *config.Config {
	e := c.Clone()
	for n, ifc := range e.Interfaces {
		kind := config.InterfaceKind(n)
		if kind == config.KindWLAN && !ifc.Wireless.Ready(kind) {
			delete(e.Interfaces, n)
			continue
		}
		if ifc.Wireless != nil && !ifc.Wireless.Ready(kind) {
			ifc.Wireless = nil
		}
	}
	for _, ifc := range e.Interfaces {
		if ifc.Bridge != nil {
			var m []string
			for _, x := range ifc.Bridge.Members {
				if _, ok := e.Interfaces[x]; ok {
					m = append(m, x)
				}
			}
			ifc.Bridge.Members = m
		}
	}
	return e
}

// ---- interfaces ----------------------------------------------------------

func (a *Applier) interfaces(s *step, old, new *config.Config) {
	// Tear down removed interfaces, bridges first so members are released.
	removed := []string{}
	for _, n := range old.InterfaceNames() {
		if _, ok := new.Interfaces[n]; !ok {
			removed = append(removed, n)
		}
	}
	slices.Reverse(removed)
	for _, n := range removed {
		a.removeInterface(s, n, old.Interfaces[n])
	}

	// Configure in dependency order: physical, vlan, wlan, bridge.
	for _, n := range new.InterfaceNames() {
		ni := new.Interfaces[n]
		oi, existed := old.Interfaces[n]
		if !existed {
			oi = nil
		}
		a.configureInterface(s, n, oi, ni, bridgeOf(new, n), bridgeOf(old, n))
	}
}

// bridgeOf returns the bridge an interface is a member of.
func bridgeOf(c *config.Config, name string) string {
	for bn, b := range c.Interfaces {
		if b.Bridge != nil && slices.Contains(b.Bridge.Members, name) {
			return bn
		}
	}
	return ""
}

func (a *Applier) pidfile(kind, ifn string) string { return a.Paths.File(kind + "." + ifn + ".pid") }
func (a *Applier) conffile(kind, ifn string) string {
	return a.Paths.File(kind + "." + ifn + ".conf")
}

func (a *Applier) stopWireless(s *step, ifn string) {
	s.fail(a.Exec.StopPidfile(a.pidfile("hostapd", ifn)))
	s.fail(a.Exec.StopPidfile(a.pidfile("wpa_supplicant", ifn)))
}

func (a *Applier) removeInterface(s *step, n string, oi *config.Interface) {
	if oi.Wireless != nil {
		a.stopWireless(s, n)
	}
	if oi.DHCP {
		s.run(tolMissing, "dhcpcd", "-x", n)
	}
	if config.IsCloned(n) {
		s.run(tolMissing, "ifconfig", n, "destroy")
		return
	}
	for _, addr := range oi.IPv4 {
		delAddr(s, n, addr)
	}
	for _, addr := range oi.IPv6 {
		delAddr(s, n, addr)
	}
	if oi.Wireless != nil && oi.Wireless.Mode == config.ModeHostAP && oi.Wireless.Device == "" {
		s.run(tolMissing, "ifconfig", n, "-mediaopt", "hostap")
	}
	s.run(tolMissing, "ifconfig", n, "down")
}

func addAddr(s *step, ifn, cidr string) {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		s.fail(err)
		return
	}
	if p.Addr().Is4() {
		s.run(tolExists, "ifconfig", ifn, "inet", p.Addr().String(), "netmask", render.BitsToMask(p.Bits()), "alias")
	} else {
		s.run(tolExists, "ifconfig", ifn, "inet6", p.Addr().String(), "prefixlen", strconv.Itoa(p.Bits()), "alias")
	}
}

func delAddr(s *step, ifn, cidr string) {
	p, err := netip.ParsePrefix(cidr)
	if err != nil {
		return
	}
	fam := "inet"
	if p.Addr().Is6() {
		fam = "inet6"
	}
	s.run(tolMissing, "ifconfig", ifn, fam, p.Addr().String(), "-alias")
}

func (a *Applier) configureInterface(s *step, n string, oi, ni *config.Interface, newBridge, oldBridge string) {
	fresh := oi == nil
	if fresh {
		oi = &config.Interface{}
	}
	kind := config.InterfaceKind(n)

	// A VAP's radio and mode are fixed at creation: recreate on change.
	if kind == config.KindWLAN && !fresh && wlanIdentity(oi) != wlanIdentity(ni) {
		a.removeInterface(s, n, oi)
		oi, fresh = &config.Interface{}, true
	}

	if fresh && config.IsCloned(n) {
		if kind == config.KindWLAN {
			mode := "sta"
			if ni.Wireless.Mode == config.ModeHostAP {
				mode = "hostap"
			}
			s.run(tolExists, "ifconfig", n, "create", "wlandev", ni.Wireless.Device, "wlanmode", mode)
		} else {
			s.run(tolExists, "ifconfig", n, "create")
		}
	}

	if kind == config.KindVLAN && !reflect.DeepEqual(oi.VLAN, ni.VLAN) {
		if oi.VLAN != nil {
			s.run(tolMissing, "ifconfig", n, "-vlanif")
		}
		if ni.VLAN != nil {
			s.must("ifconfig", n, "vlan", strconv.Itoa(ni.VLAN.ID), "vlanif", ni.VLAN.Parent)
		}
	}

	// Legacy wifi stack: the radio itself switches into access point mode.
	if kind == config.KindPhysical {
		wasAP := oi.Wireless != nil && oi.Wireless.Mode == config.ModeHostAP
		isAP := ni.Wireless != nil && ni.Wireless.Mode == config.ModeHostAP
		if isAP && (!wasAP || fresh) {
			s.must("ifconfig", n, "mediaopt", "hostap")
		} else if !isAP && wasAP {
			s.must("ifconfig", n, "-mediaopt", "hostap")
		}
	}

	if ni.MTU != 0 && (ni.MTU != oi.MTU || fresh) {
		s.must("ifconfig", n, "mtu", strconv.Itoa(ni.MTU))
	}
	if ni.Media != oi.Media || (fresh && ni.Media != "") {
		media := ni.Media
		if media == "" {
			media = "autoselect"
		}
		args := append([]string{n, "media"}, strings.Fields(media)...)
		s.must("ifconfig", args...)
	}

	// Addresses: remove what disappeared, add what is new.
	for _, addr := range minus(oi.IPv4, ni.IPv4) {
		delAddr(s, n, addr)
	}
	for _, addr := range minus(oi.IPv6, ni.IPv6) {
		delAddr(s, n, addr)
	}
	for _, addr := range minus(ni.IPv4, oi.IPv4) {
		addAddr(s, n, addr)
	}
	for _, addr := range minus(ni.IPv6, oi.IPv6) {
		addAddr(s, n, addr)
	}

	if kind == config.KindBridge {
		var om, nm []string
		if oi.Bridge != nil {
			om = oi.Bridge.Members
		}
		if ni.Bridge != nil {
			nm = ni.Bridge.Members
		}
		for _, m := range minus(om, nm) {
			s.run(tolMissing, "brconfig", n, "delete", m)
		}
		for _, m := range minus(nm, om) {
			s.run(tolExists, "brconfig", n, "add", m)
		}
	}

	if ni.Shutdown {
		if !oi.Shutdown || fresh {
			if ni.Wireless != nil {
				a.stopWireless(s, n)
			}
			if oi.DHCP && !fresh {
				s.run(tolMissing, "dhcpcd", "-x", n)
			}
			s.must("ifconfig", n, "down")
		}
		return
	}
	if oi.Shutdown || fresh {
		s.must("ifconfig", n, "up")
	}

	cameUp := oi.Shutdown || fresh
	switch {
	case ni.DHCP && (!oi.DHCP || cameUp):
		s.run(tolExists, "dhcpcd", "-b", n)
	case !ni.DHCP && oi.DHCP && !oi.Shutdown:
		s.run(tolMissing, "dhcpcd", "-x", n)
	}

	hasWifi := oi.Wireless != nil || ni.Wireless != nil
	if hasWifi && (!reflect.DeepEqual(oi.Wireless, ni.Wireless) || newBridge != oldBridge || cameUp) {
		a.stopWireless(s, n)
		if ni.Wireless != nil {
			a.startWireless(s, n, ni.Wireless, newBridge)
		}
	}
}

func wlanIdentity(i *config.Interface) string {
	if i.Wireless == nil {
		return ""
	}
	return i.Wireless.Device + "/" + i.Wireless.Mode
}

func (a *Applier) startWireless(s *step, n string, w *config.Wireless, bridge string) {
	if w.Mode == config.ModeHostAP {
		conf := a.conffile("hostapd", n)
		if err := a.Exec.WriteFile(conf, []byte(render.Hostapd(n, w, bridge)), 0o600); err != nil {
			s.fail(err)
			return
		}
		s.must("hostapd", "-B", "-P", a.pidfile("hostapd", n), conf)
		return
	}
	conf := a.conffile("wpa_supplicant", n)
	if err := a.Exec.WriteFile(conf, []byte(render.WPASupplicant(w)), 0o600); err != nil {
		s.fail(err)
		return
	}
	s.must("wpa_supplicant", "-B", "-i", n, "-c", conf, "-P", a.pidfile("wpa_supplicant", n))
}

// minus returns the elements of a that are not in b, keeping order.
func minus(a, b []string) []string {
	var r []string
	for _, x := range a {
		if !slices.Contains(b, x) {
			r = append(r, x)
		}
	}
	return r
}

// ---- routes --------------------------------------------------------------

func routeKey(r config.Route) string { return r.Prefix + " " + strings.ToLower(r.NextHop) }

func routeArgs(verb string, r config.Route) []string {
	p, _ := netip.ParsePrefix(r.Prefix)
	fam := "-inet"
	if p.Addr().Is6() {
		fam = "-inet6"
	}
	dst := r.Prefix
	if p.Bits() == 0 {
		dst = "default"
	}
	args := []string{"-n", "-q", verb, fam, dst}
	if strings.EqualFold(r.NextHop, "null0") {
		lo := "127.0.0.1"
		if fam == "-inet6" {
			lo = "::1"
		}
		if verb == "add" {
			return append(args, lo, "-blackhole")
		}
		return args
	}
	if verb == "add" {
		args = append(args, r.NextHop)
	}
	return args
}

func (a *Applier) routes(s *step, old, new *config.Config) {
	oldKeys := map[string]bool{}
	for _, r := range old.Routes {
		oldKeys[routeKey(r)] = true
	}
	newKeys := map[string]bool{}
	for _, r := range new.Routes {
		newKeys[routeKey(r)] = true
	}
	for _, r := range old.Routes {
		if !newKeys[routeKey(r)] {
			s.run(tolMissing, "route", routeArgs("delete", r)...)
		}
	}
	for _, r := range new.Routes {
		if !oldKeys[routeKey(r)] {
			s.run(tolExists, "route", routeArgs("add", r)...)
		}
	}
}

// ---- firewall -------------------------------------------------------------

// NPFConfPath is where the generated npf.conf is written.
func (a *Applier) NPFConfPath() string { return a.Paths.File("npf.conf") }

func (a *Applier) firewall(s *step, old, new *config.Config) {
	newConf := render.NPF(new)
	if !s.boot && newConf == render.NPF(old) {
		return
	}
	path := a.NPFConfPath()
	prev, _ := a.Exec.ReadFile(path)
	if err := a.Exec.WriteFile(path, []byte(newConf), 0o600); err != nil {
		s.fail(err)
		return
	}
	if !s.must("npfctl", "validate", path) {
		// Keep the last good ruleset on disk.
		if prev != nil {
			a.Exec.WriteFile(path, prev, 0o600)
		}
		return
	}
	if s.must("npfctl", "reload", path) {
		s.run([]string{"already"}, "npfctl", "start")
	}
}

// ---- dhcp server ------------------------------------------------------------

func (a *Applier) dhcpServer(s *step, old, new *config.Config) {
	newConf, newIfs := render.Dhcpd(new)
	oldConf, oldIfs := render.Dhcpd(old)
	if !s.boot && newConf == oldConf && slices.Equal(newIfs, oldIfs) {
		return
	}
	pid := a.Paths.File("dhcpd.pid")
	s.fail(a.Exec.StopPidfile(pid))
	if len(new.DHCPPools) == 0 || len(newIfs) == 0 {
		return
	}
	conf := a.Paths.File("dhcpd.conf")
	if err := a.Exec.WriteFile(conf, []byte(newConf), 0o644); err != nil {
		s.fail(err)
		return
	}
	if _, err := a.Exec.ReadFile(a.Paths.DHCPLeases); err != nil {
		s.fail(a.Exec.WriteFile(a.Paths.DHCPLeases, nil, 0o644))
	}
	args := []string{"-q", "-cf", conf, "-pf", pid, "-lf", a.Paths.DHCPLeases}
	s.must("dhcpd", append(args, newIfs...)...)
}

// Describe returns a short human readable summary of an error list.
func Describe(err error) string {
	if err == nil {
		return ""
	}
	var b strings.Builder
	for _, line := range strings.Split(err.Error(), "\n") {
		fmt.Fprintf(&b, "%% %s\n", line)
	}
	return b.String()
}
