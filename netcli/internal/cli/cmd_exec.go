package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/netcli/netcli/internal/apply"
	"github.com/netcli/netcli/internal/config"
	"github.com/netcli/netcli/internal/manager"
	"github.com/netcli/netcli/internal/render"
	"github.com/netcli/netcli/internal/system"
)

var (
	userTree *Node
	execTree *Node
)

func asParseError(err error, pe **ParseError) bool { return errors.As(err, pe) }

// RunCommand runs an external program for ping/traceroute. Interactive
// front ends replace it to attach the terminal.
var RunCommand = func(s *Session, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout, cmd.Stderr = s.Out, s.Out
	return cmd.Run()
}

func (s *Session) endConfig() {
	s.End()
	if d, ok := s.Mgr.RevertPending(); ok {
		s.printf("%% Rollback timer is armed: changes are reverted in %s unless you enter 'configure confirm'.\n",
			time.Until(d).Round(time.Second))
	}
}

func exitCmd() []*Node {
	quit := func(s *Session, _ *Args) error { s.quit = true; return nil }
	return []*Node{
		kw("exit", "Exit from the EXEC").run(quit),
		kw("logout", "Exit from the EXEC").run(quit),
		kw("quit", "Exit from the EXEC").run(quit).hidden(),
	}
}

func pingNodes() []*Node {
	ping := func(s *Session, a *Args) error {
		host := a.Get("host")
		count := 5
		if c := a.Get("count"); c != "" {
			count = atoi(c)
		}
		v6 := isIPv6(host)
		name, args := system.PingCommand(v6, count)
		return RunCommand(s, name, append(args, host)...)
	}
	return []*Node{
		kw("ping", "Send echo messages").add(
			arg("host", tHost).run(ping).add(
				kw("count", "Number of echo requests").add(arg("count", tNum(1, 1000, "Count")).run(ping)))),
		kw("traceroute", "Trace route to destination").add(
			arg("host", tHost).run(func(s *Session, a *Args) error {
				name := "traceroute"
				if isIPv6(a.Get("host")) {
					name = "traceroute6"
				}
				return RunCommand(s, name, "-n", a.Get("host"))
			})),
	}
}

func buildUserTree() *Node {
	r := root(exitCmd()...)
	r.add(kw("enable", "Turn on privileged commands").run(func(s *Session, a *Args) error {
		c := s.running()
		if c.EnableSecret != "" {
			if s.ReadLine == nil {
				return fmt.Errorf("enable secret is set; an interactive terminal is required")
			}
			pw, err := s.ReadLine("Password: ", false)
			if err != nil {
				return err
			}
			if !config.CheckSecret(c.EnableSecret, pw) {
				return fmt.Errorf("Bad secrets")
			}
		}
		s.Mode = ModeExec
		return nil
	}))
	r.add(showNode(false))
	r.add(pingNodes()...)
	return r
}

func buildExecTree() *Node {
	r := root(exitCmd()...)
	r.add(kw("enable", "Turn on privileged commands").run(func(s *Session, a *Args) error { return nil }))
	r.add(kw("disable", "Turn off privileged commands").run(func(s *Session, a *Args) error { s.Mode = ModeUser; return nil }))
	r.add(showNode(true))
	r.add(pingNodes()...)

	enterConfig := func(s *Session, a *Args) error {
		if m := a.Get("minutes"); m != "" {
			d, err := s.Mgr.ArmRevert(time.Duration(atoi(m))*time.Minute, s.RevertArgs)
			if err != nil {
				return err
			}
			s.printf("Rollback timer armed: the configuration reverts at %s unless confirmed with 'configure confirm'.\n",
				d.Format("15:04:05"))
		} else if d, ok := s.Mgr.RevertPending(); ok {
			s.printf("%% Note: rollback timer armed, expires in %s\n", time.Until(d).Round(time.Second))
		}
		if s.ReadLine != nil {
			s.println("Enter configuration commands, one per line.  End with CNTL/Z.")
		}
		s.Mode = ModeConfig
		return nil
	}
	r.add(kw("configure", "Enter configuration mode").add(
		kw("terminal", "Configure from the terminal").run(enterConfig).add(
			kw("revert", "Arm an automatic rollback").add(
				kw("timer", "Revert after a number of minutes unless confirmed").add(
					arg("minutes", tNum(1, 120, "Minutes until rollback")).run(enterConfig)))),
		kw("confirm", "Keep the changes made since 'configure terminal revert timer'").run(func(s *Session, a *Args) error {
			if !s.Mgr.ConfirmRevert() {
				return fmt.Errorf("no rollback timer is armed")
			}
			s.println("Configuration confirmed; rollback timer cancelled.")
			return nil
		}),
		kw("revert", "Roll back now").add(kw("now", "Restore the configuration saved when the timer was armed").run(func(s *Session, a *Args) error {
			if err := s.Mgr.RevertNow(); err != nil {
				return err
			}
			s.println("Configuration rolled back.")
			return nil
		})),
	))

	writeMem := func(s *Session, a *Args) error {
		s.println("Building configuration...")
		if err := s.Mgr.Save(); err != nil {
			return err
		}
		s.println("[OK]")
		return nil
	}
	erase := func(s *Session, a *Args) error {
		if s.ReadLine != nil {
			ans, err := s.ReadLine("Erasing the startup configuration. Continue? [confirm]", true)
			if err != nil || (ans != "" && !strings.HasPrefix(strings.ToLower(ans), "y")) {
				return fmt.Errorf("aborted")
			}
		}
		if err := s.Mgr.Erase(); err != nil {
			return err
		}
		s.println("[OK]")
		return nil
	}
	r.add(kw("write", "Write running configuration to memory or terminal").run(writeMem).add(
		kw("memory", "Write to the startup configuration").run(writeMem),
		kw("terminal", "Write to terminal").run(func(s *Session, a *Args) error { return showRun(s, "") }),
		kw("erase", "Erase the startup configuration").run(erase),
	))
	r.add(kw("erase", "Erase a filesystem").add(kw("startup-config", "Erase the startup configuration").run(erase)))
	r.add(kw("copy", "Copy configuration").add(
		kw("running-config", "Copy from the running configuration").add(
			kw("startup-config", "Copy to the startup configuration").run(writeMem)),
		kw("startup-config", "Copy from the startup configuration").add(
			kw("running-config", "Replace the running configuration").run(func(s *Session, a *Args) error {
				c, ok, err := s.Mgr.Store.Startup()
				if err != nil {
					return err
				}
				if !ok {
					return fmt.Errorf("startup-config is not present")
				}
				if _, err := s.Mgr.Replace(c); err != nil {
					return err
				}
				s.println("[OK]")
				return nil
			})),
	))
	r.add(kw("reload", "Halt and perform a cold restart").run(func(s *Session, a *Args) error {
		if s.ReadLine == nil {
			return fmt.Errorf("reload requires an interactive terminal")
		}
		c, _ := s.Mgr.Running()
		st, _ := s.Mgr.Startup()
		if !config.Equal(c, st) {
			ans, _ := s.ReadLine("System configuration has been modified. Save? [yes/no]: ", true)
			if strings.HasPrefix(strings.ToLower(ans), "y") {
				if err := s.Mgr.Save(); err != nil {
					return err
				}
			}
		}
		ans, err := s.ReadLine("Proceed with reload? [confirm]", true)
		if err != nil || (ans != "" && !strings.HasPrefix(strings.ToLower(ans), "y")) {
			return fmt.Errorf("reload aborted")
		}
		_, err = s.Mgr.Applier.Exec.Run("shutdown", "-r", "now")
		return err
	}))
	r.add(kw("debug", "Debugging functions").add(
		kw("apply", "Show the system commands executed for each change").run(func(s *Session, a *Args) error {
			s.DebugApply = true
			s.updateTrace()
			s.println("System command debugging is on")
			return nil
		})))
	r.add(kw("undebug", "Disable debugging functions").add(
		kw("all", "Disable all debugging").run(func(s *Session, a *Args) error {
			s.DebugApply = false
			s.updateTrace()
			s.println("All possible debugging has been turned off")
			return nil
		})))
	return r
}

func (s *Session) updateTrace() {
	if s.SetTrace == nil {
		return
	}
	if s.DebugApply {
		s.SetTrace(func(l string) { s.printf("  %s\n", l) })
	} else {
		s.SetTrace(nil)
	}
}

// ---- show ---------------------------------------------------------------------------

func showNode(priv bool) *Node {
	ifArg := func(run func(s *Session, a *Args) error) *Node { return arg("if", tIfName).run(run) }
	show := kw("show", "Show running system information")
	show.add(
		kw("interfaces", "Interface status and configuration").run(showInterfaces).add(ifArg(showInterfaces)),
		kw("ip", "IP information").add(
			kw("interface", "IP interface status and configuration").add(
				kw("brief", "Brief summary of IP status and configuration").run(showIPBrief)),
			kw("route", "IP routing table").run(func(s *Session, a *Args) error {
				s.printf("%s", system.RouteTable(false))
				return nil
			}),
			kw("dhcp", "Show DHCP server information").add(
				kw("binding", "DHCP address bindings").run(showDHCPBinding),
				kw("pool", "DHCP pools").run(showDHCPPool)),
			kw("nat", "IP NAT information").add(
				kw("rules", "Configured translations").run(showNAT))),
		kw("ipv6", "IPv6 information").add(
			kw("route", "IPv6 routing table").run(func(s *Session, a *Args) error {
				s.printf("%s", system.RouteTable(true))
				return nil
			})),
		kw("version", "System hardware and software status").run(showVersion),
		kw("clock", "Display the system clock").run(func(s *Session, a *Args) error {
			s.println(time.Now().Format("15:04:05.000 MST Mon Jan 2 2006"))
			return nil
		}),
		kw("history", "Display the session command history").run(func(s *Session, a *Args) error {
			for _, h := range s.History {
				s.printf("  %s\n", h)
			}
			return nil
		}),
		kw("wireless", "Wireless interfaces").run(showWireless).add(ifArg(showWireless)),
	)
	if !priv {
		return show
	}
	show.add(
		kw("running-config", "Current operating configuration").run(func(s *Session, a *Args) error { return showRun(s, "") }).add(
			kw("interface", "Show one interface").add(arg("if", tIfName).run(func(s *Session, a *Args) error { return showRun(s, a.Get("if")) })),
			kw("yaml", "Show the configuration in YAML").run(func(s *Session, a *Args) error {
				data, err := config.Marshal(s.running())
				if err != nil {
					return err
				}
				s.printf("%s", data)
				return nil
			})),
		kw("startup-config", "Contents of the startup configuration").run(func(s *Session, a *Args) error {
			c, ok, err := s.Mgr.Store.Startup()
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("startup-config is not present")
			}
			s.printf("%s", ConfigText(c, ""))
			return nil
		}),
		kw("archive", "Archive functions").add(
			kw("config", "Configuration").add(
				kw("differences", "Differences between startup and running configuration").run(showDiff))),
		kw("access-lists", "List access lists").run(showACLs).add(arg("name", tACLName).run(showACLs)),
		kw("firewall", "NPF firewall state").run(func(s *Session, a *Args) error {
			out, _ := system.Output("npfctl", "show")
			s.printf("%s", out)
			return nil
		}).add(
			kw("config", "Generated npf.conf").run(func(s *Session, a *Args) error {
				s.printf("%s", render.NPF(apply.Effective(s.running())))
				return nil
			}),
			kw("statistics", "NPF statistics").run(func(s *Session, a *Args) error {
				out, _ := system.Output("npfctl", "stats")
				s.printf("%s", out)
				return nil
			})),
		kw("rollback", "Rollback timer status").run(func(s *Session, a *Args) error {
			if d, ok := s.Mgr.RevertPending(); ok {
				s.printf("Rollback timer armed, the configuration reverts in %s (at %s)\n",
					time.Until(d).Round(time.Second), d.Format("15:04:05"))
			} else {
				s.println("No rollback timer is armed")
			}
			return nil
		}),
	)
	return show
}

func showRun(s *Session, only string) error {
	text := ConfigText(s.running(), only)
	if only != "" && text == "" {
		return fmt.Errorf("interface %s is not configured", only)
	}
	if only == "" {
		s.println("Building configuration...")
		s.println()
		s.printf("Current configuration : %d bytes\n", len(text))
	}
	s.printf("%s", text)
	return nil
}

func showDiff(s *Session, a *Args) error {
	st, ok, err := s.Mgr.Store.Startup()
	if err != nil {
		return err
	}
	if !ok {
		st = config.New()
	}
	d := manager.Diff(ConfigText(st, ""), ConfigText(s.running(), ""))
	if d == "" {
		s.println("!No changes were found")
		return nil
	}
	s.println("Contextual Config Diffs (- startup, + running):")
	s.printf("%s", d)
	return nil
}

func showVersion(s *Session, a *Args) error {
	s.printf("netcli version %s (%s, %s/%s)\n", s.Version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	s.println(system.Uname())
	if u := system.Uptime(); u != "" {
		s.printf("uptime: %s\n", u)
	}
	if s.Mgr.Applier.Exec.DryRun() {
		s.println("Mode: DRY RUN (system commands are logged, not executed)")
	}
	s.printf("Startup configuration: %s\n", s.Mgr.Store.StartupPath)
	return nil
}

type ifInfo struct {
	name    string
	sys     *net.Interface
	addrs   []string
	cfg     *config.Interface
	present bool
}

func gatherInterfaces(s *Session, only string) []ifInfo {
	c := s.running()
	seen := map[string]*ifInfo{}
	if ifs, err := net.Interfaces(); err == nil {
		for i := range ifs {
			ni := ifs[i]
			info := &ifInfo{name: ni.Name, sys: &ni, present: true}
			if addrs, err := ni.Addrs(); err == nil {
				for _, ad := range addrs {
					info.addrs = append(info.addrs, ad.String())
				}
			}
			seen[ni.Name] = info
		}
	}
	for n, ic := range c.Interfaces {
		if seen[n] == nil {
			seen[n] = &ifInfo{name: n}
		}
		seen[n].cfg = ic
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		if only == "" || n == only {
			names = append(names, n)
		}
	}
	config.SortInterfaceNames(names)
	out := make([]ifInfo, 0, len(names))
	for _, n := range names {
		out = append(out, *seen[n])
	}
	return out
}

func (i ifInfo) status() (string, string) {
	if i.cfg != nil && i.cfg.Shutdown {
		return "administratively down", "down"
	}
	if !i.present {
		return "not present", "down"
	}
	if i.sys.Flags&net.FlagUp == 0 {
		return "down", "down"
	}
	if i.sys.Flags&net.FlagRunning == 0 {
		return "up", "down"
	}
	return "up", "up"
}

func showInterfaces(s *Session, a *Args) error {
	list := gatherInterfaces(s, a.Get("if"))
	if len(list) == 0 {
		return fmt.Errorf("interface %s not found", a.Get("if"))
	}
	var sample = map[string]struct{ rb, tb, rp, tp, re, te uint64 }{}
	if s.Sample != nil {
		if sm, err := s.Sample(); err == nil && sm != nil {
			for n, c := range sm.Ifaces {
				sample[n] = struct{ rb, tb, rp, tp, re, te uint64 }{c.RxBytes, c.TxBytes, c.RxPkts, c.TxPkts, c.RxErrs, c.TxErrs}
			}
		}
	}
	for _, i := range list {
		st, proto := i.status()
		s.printf("%s is %s, line protocol is %s\n", i.name, st, proto)
		if i.cfg != nil && i.cfg.Description != "" {
			s.printf("  Description: %s\n", i.cfg.Description)
		}
		if i.sys != nil {
			if len(i.sys.HardwareAddr) > 0 {
				s.printf("  Hardware address is %s\n", i.sys.HardwareAddr)
			}
			s.printf("  MTU %d bytes\n", i.sys.MTU)
		}
		for _, ad := range i.addrs {
			if p, err := netip.ParsePrefix(ad); err == nil && p.Addr().Is4() {
				s.printf("  Internet address is %s\n", ad)
			} else {
				s.printf("  IPv6 address %s\n", ad)
			}
		}
		if i.cfg != nil {
			if i.cfg.DHCP {
				s.println("  Address determined by DHCP")
			}
			if i.cfg.VLAN != nil {
				s.printf("  Encapsulation 802.1Q VLAN %d on %s\n", i.cfg.VLAN.ID, i.cfg.VLAN.Parent)
			}
			if i.cfg.Bridge != nil {
				s.printf("  Bridge members: %s\n", strings.Join(i.cfg.Bridge.Members, ", "))
			}
			if i.cfg.AccessGroupIn != "" {
				s.printf("  Inbound access list is %s\n", i.cfg.AccessGroupIn)
			}
			if i.cfg.AccessGroupOut != "" {
				s.printf("  Outgoing access list is %s\n", i.cfg.AccessGroupOut)
			}
			if w := i.cfg.Wireless; w != nil {
				s.printf("  Wireless: %s\n", wirelessSummary(i.name, w))
			}
		}
		if c, ok := sample[i.name]; ok {
			s.printf("     %d packets input, %d bytes, %d input errors\n", c.rp, c.rb, c.re)
			s.printf("     %d packets output, %d bytes, %d output errors\n", c.tp, c.tb, c.te)
		}
	}
	return nil
}

func wirelessSummary(name string, w *config.Wireless) string {
	kind := config.InterfaceKind(name)
	parts := []string{w.Mode}
	if w.SSID != "" {
		parts = append(parts, fmt.Sprintf("SSID %q", w.SSID))
	}
	sec := w.Security
	if sec == "" {
		sec = "security not set"
	}
	parts = append(parts, sec)
	if w.Channel != 0 {
		parts = append(parts, fmt.Sprintf("channel %d", w.Channel))
	}
	if w.Device != "" {
		parts = append(parts, "radio "+w.Device)
	}
	if !w.Ready(kind) {
		parts = append(parts, "INCOMPLETE (missing "+w.Missing(kind)+")")
	}
	return strings.Join(parts, ", ")
}

func showIPBrief(s *Session, a *Args) error {
	s.printf("%-14s %-17s %-4s %-7s %-22s %s\n", "Interface", "IP-Address", "OK?", "Method", "Status", "Protocol")
	for _, i := range gatherInterfaces(s, "") {
		ip := "unassigned"
		for _, ad := range i.addrs {
			if p, err := netip.ParsePrefix(ad); err == nil && p.Addr().Is4() {
				ip = p.Addr().String()
				break
			}
		}
		method := "unset"
		if i.cfg != nil {
			if i.cfg.DHCP {
				method = "DHCP"
			} else if len(i.cfg.IPv4) > 0 {
				method = "manual"
			}
		}
		st, proto := i.status()
		s.printf("%-14s %-17s %-4s %-7s %-22s %s\n", i.name, ip, "YES", method, st, proto)
	}
	return nil
}

func showACLs(s *Session, a *Args) error {
	c := s.running()
	names := []string{}
	for n := range c.AccessLists {
		if a.Get("name") == "" || n == a.Get("name") {
			names = append(names, n)
		}
	}
	if len(names) == 0 && a.Get("name") != "" {
		return fmt.Errorf("access-list %s does not exist", a.Get("name"))
	}
	sort.Strings(names)
	for _, n := range names {
		s.printf("Extended IP access list %s\n", n)
		for _, e := range c.AccessLists[n].Entries {
			s.printf("    %d %s\n", e.Seq, formatACE(e))
		}
		var used []string
		for _, ifn := range c.InterfaceNames() {
			ic := c.Interfaces[ifn]
			if ic.AccessGroupIn == n {
				used = append(used, ifn+" in")
			}
			if ic.AccessGroupOut == n {
				used = append(used, ifn+" out")
			}
		}
		if len(used) > 0 {
			s.printf("    (applied: %s; implicit deny at the end)\n", strings.Join(used, ", "))
		} else {
			s.println("    (not applied to any interface)")
		}
	}
	return nil
}

func showNAT(s *Session, a *Args) error {
	c := s.running()
	if len(c.NAT) == 0 {
		s.println("No NAT rules configured")
		return nil
	}
	for _, n := range c.NAT {
		switch n.Type {
		case config.NATOverload:
			s.printf("overload  %-18s -> address of %s\n", n.Source, n.Interface)
		case config.NATStatic:
			s.printf("static    %s %s:%d <- %s port %d\n", n.Protocol, n.LocalAddr, n.LocalPort, n.Interface, n.PublicPort)
		}
	}
	return nil
}

func showDHCPPool(s *Session, a *Args) error {
	c := s.running()
	if len(c.DHCPPools) == 0 {
		s.println("No DHCP pools configured")
		return nil
	}
	_, ifs := render.Dhcpd(c)
	names := []string{}
	for n := range c.DHCPPools {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		p := c.DHCPPools[n]
		s.printf("Pool %s :\n", n)
		s.printf(" Network        : %s\n", p.Network)
		if p.RangeStart != "" {
			s.printf(" Range          : %s - %s\n", p.RangeStart, p.RangeEnd)
		} else {
			s.println(" Range          : (none, only reservations are served)")
		}
		s.printf(" Reservations   : %d\n", len(p.Hosts))
	}
	if len(ifs) == 0 {
		s.println("% No interface has an address in any pool network; dhcpd is not running.")
	} else {
		s.printf("Serving on: %s\n", strings.Join(ifs, ", "))
	}
	return nil
}

type lease struct {
	ip, mac, host, ends, state string
}

// parseLeases reads an ISC dhcpd lease database; later entries win.
func parseLeases(path string) ([]lease, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	byIP := map[string]*lease{}
	var cur *lease
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		line = strings.TrimSuffix(line, ";")
		f := strings.Fields(line)
		switch {
		case len(f) >= 2 && f[0] == "lease":
			cur = &lease{ip: f[1]}
		case cur == nil:
		case line == "}":
			byIP[cur.ip] = cur
			cur = nil
		case len(f) >= 3 && f[0] == "hardware" && f[1] == "ethernet":
			cur.mac = f[2]
		case len(f) >= 2 && f[0] == "client-hostname":
			cur.host = strings.Trim(strings.Join(f[1:], " "), "\"")
		case len(f) >= 3 && f[0] == "ends":
			cur.ends = strings.Join(f[2:], " ")
		case len(f) >= 3 && f[0] == "binding" && f[1] == "state":
			cur.state = f[2]
		}
	}
	var out []lease
	for _, l := range byIP {
		out = append(out, *l)
	}
	sort.Slice(out, func(i, j int) bool {
		a, _ := netip.ParseAddr(out[i].ip)
		b, _ := netip.ParseAddr(out[j].ip)
		return a.Less(b)
	})
	return out, sc.Err()
}

func showDHCPBinding(s *Session, a *Args) error {
	leases, err := parseLeases(s.Mgr.Paths.DHCPLeases)
	if err != nil {
		return fmt.Errorf("no lease database (%v)", err)
	}
	s.printf("%-16s %-18s %-20s %-8s %s\n", "IP address", "Hardware address", "Lease expiration", "State", "Hostname")
	for _, l := range leases {
		s.printf("%-16s %-18s %-20s %-8s %s\n", l.ip, l.mac, l.ends, l.state, l.host)
	}
	return nil
}

func showWireless(s *Session, a *Args) error {
	c := s.running()
	found := false
	for _, n := range c.InterfaceNames() {
		ic := c.Interfaces[n]
		if ic.Wireless == nil || (a.Get("if") != "" && a.Get("if") != n) {
			continue
		}
		found = true
		s.printf("%s: %s\n", n, wirelessSummary(n, ic.Wireless))
		if out, err := system.Output("ifconfig", n); err == nil {
			for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
				s.printf("    %s\n", l)
			}
		}
		if ic.Wireless.Mode == config.ModeHostAP && runtime.GOOS == "netbsd" {
			if out, err := system.Output("wlanctl", n); err == nil {
				s.println("  Associated stations (wlanctl):")
				for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
					s.printf("    %s\n", l)
				}
			}
		}
	}
	if !found {
		s.println("No wireless interfaces configured")
	}
	return nil
}
