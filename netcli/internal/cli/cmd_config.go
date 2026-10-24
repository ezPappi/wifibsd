package cli

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/netcli/netcli/internal/config"
	"github.com/netcli/netcli/internal/manager"
)

var (
	configTree    *Node
	interfaceTree *Node
	aclTree       *Node
	dhcpTree      *Node
)

func init() {
	configTree = buildConfigTree()
	interfaceTree = buildInterfaceTree()
	aclTree = buildACLTree()
	dhcpTree = buildDHCPTree()
	userTree = buildUserTree()
	execTree = buildExecTree()
}

// commonSubmode are the commands available in every configuration mode.
func commonSubmode() []*Node {
	return []*Node{
		kw("exit", "Exit from the current mode").run(func(s *Session, _ *Args) error { s.exitMode(); return nil }),
		kw("end", "Exit to privileged EXEC mode").run(func(s *Session, _ *Args) error { s.endConfig(); return nil }),
		kw("do", "Run an EXEC command").add(arg("cmd", &ArgType{Name: "LINE", Help: "EXEC command", Rest: true}).run(doExec)),
	}
}

func doExec(s *Session, a *Args) error {
	saved := s.Mode
	s.Mode = ModeExec
	defer func() {
		if s.Mode == ModeExec {
			s.Mode = saved
		}
	}()
	line := a.Get("cmd")
	toks, err := tokenize(line)
	if err != nil {
		return err
	}
	if len(toks) > 0 && strings.HasPrefix("configure", strings.ToLower(toks[0].text)) {
		return fmt.Errorf("already in configuration mode")
	}
	n, args, err := walk(execTree, line, toks)
	if err != nil {
		var pe *ParseError
		if ok := asParseError(err, &pe); ok {
			return fmt.Errorf("%s", pe.Msg)
		}
		return err
	}
	if n.Run == nil {
		return errIncomplete
	}
	return n.Run(s, args)
}

// ---- global configuration ------------------------------------------------------

func buildConfigTree() *Node {
	r := root(commonSubmode()...)

	r.add(kw("hostname", "Set the system's network name").add(
		arg("name", &ArgType{Name: "WORD", Help: "This system's network name", Match: func(s string) bool { return s != "" }}).run(
			func(s *Session, a *Args) error {
				return s.change(func(c *config.Config) error { c.Hostname = a.Get("name"); return nil })
			})).no(func(s *Session, a *Args) error {
		return s.change(func(c *config.Config) error { c.Hostname = "router"; return nil })
	}))

	ip := kw("ip", "Global IP configuration subcommands")
	r.add(ip)
	ip.add(kw("domain-name", "Define the default domain name").add(
		arg("name", &ArgType{Name: "WORD", Help: "Default domain name", Match: func(s string) bool { return s != "" }}).run(
			func(s *Session, a *Args) error {
				return s.change(func(c *config.Config) error { c.DomainName = a.Get("name"); return nil })
			})).no(func(s *Session, a *Args) error {
		return s.change(func(c *config.Config) error { c.DomainName = ""; return nil })
	}))

	nsArg := &ArgType{Name: "A.B.C.D", Help: "Domain server IP address (maximum 6)", Match: func(s string) bool { return isIPv4(s) || isIPv6(s) }}
	ip.add(kw("name-server", "Specify address of name server to use").add(
		repeat("ns", nsArg, func(s *Session, a *Args) error {
			return s.change(func(c *config.Config) error {
				for _, ns := range a.All("ns") {
					if !slices.Contains(c.NameServers, ns) {
						c.NameServers = append(c.NameServers, ns)
					}
				}
				if len(c.NameServers) > 6 {
					return fmt.Errorf("at most 6 name servers")
				}
				return nil
			})
		}).no(func(s *Session, a *Args) error {
			return s.change(func(c *config.Config) error {
				c.NameServers = slices.DeleteFunc(c.NameServers, func(x string) bool { return slices.Contains(a.All("ns"), x) })
				return nil
			})
		})).no(func(s *Session, a *Args) error {
		return s.change(func(c *config.Config) error { c.NameServers = nil; return nil })
	}))

	ip.add(kw("routing", "Enable IP routing (forwarding)").run(func(s *Session, a *Args) error {
		return s.change(func(c *config.Config) error { c.IPRouting = true; return nil })
	}).no(func(s *Session, a *Args) error {
		return s.change(func(c *config.Config) error { c.IPRouting = false; return nil })
	}))

	ip.add(routeNode(false))
	ip.add(kw("access-list", "Named access-list").add(
		kw("extended", "Extended access list").add(
			arg("name", tACLName).run(func(s *Session, a *Args) error {
				name := a.Get("name")
				err := s.change(func(c *config.Config) error {
					if _, ok := c.AccessLists[name]; ok {
						return errNoChange
					}
					c.AccessLists[name] = &config.AccessList{}
					return nil
				})
				if err != nil {
					return err
				}
				s.ACL, s.Mode = name, ModeACL
				return nil
			}).no(func(s *Session, a *Args) error {
				return s.change(func(c *config.Config) error {
					if _, ok := c.AccessLists[a.Get("name")]; !ok {
						return errNoChange
					}
					delete(c.AccessLists, a.Get("name"))
					return nil
				})
			}))))
	ip.add(natNode())
	ip.add(kw("dhcp", "Configure DHCP server").add(
		kw("pool", "Configure DHCP address pools").add(
			arg("name", tPoolName).run(func(s *Session, a *Args) error {
				s.Pool, s.Mode = a.Get("name"), ModeDHCP
				return nil
			}).no(func(s *Session, a *Args) error {
				return s.change(func(c *config.Config) error {
					if _, ok := c.DHCPPools[a.Get("name")]; !ok {
						return errNoChange
					}
					delete(c.DHCPPools, a.Get("name"))
					return nil
				})
			}))))
	ip.add(httpNode())

	ipv6 := kw("ipv6", "Global IPv6 configuration subcommands")
	r.add(ipv6)
	ipv6.add(kw("unicast-routing", "Enable unicast routing (forwarding)").run(func(s *Session, a *Args) error {
		return s.change(func(c *config.Config) error { c.IPv6Routing = true; return nil })
	}).no(func(s *Session, a *Args) error {
		return s.change(func(c *config.Config) error { c.IPv6Routing = false; return nil })
	}))
	ipv6.add(routeNode(true))

	r.add(kw("interface", "Select an interface to configure").add(
		arg("if", tIfName).run(func(s *Session, a *Args) error {
			name := a.Get("if")
			if config.InterfaceKind(name) == config.KindLoopback {
				return fmt.Errorf("the loopback interface is managed by the system")
			}
			if !config.IsCloned(name) {
				// Physical interfaces get a (possibly empty) entry at once,
				// so they can be referenced as vlan parents and bridge members.
				if err := s.change(func(c *config.Config) error {
					if _, ok := c.Interfaces[name]; ok {
						return errNoChange
					}
					c.Interface(name)
					return nil
				}); err != nil {
					return err
				}
			}
			s.Iface, s.Mode = name, ModeInterface
			return nil
		}).no(func(s *Session, a *Args) error {
			return s.change(func(c *config.Config) error {
				if _, ok := c.Interfaces[a.Get("if")]; !ok {
					return errNoChange
				}
				delete(c.Interfaces, a.Get("if"))
				return nil
			})
		})))

	secretNode := func(key string, set func(c *config.Config, a *Args, hash string) error) *Node {
		apply := func(hashed bool) func(s *Session, a *Args) error {
			return func(s *Session, a *Args) error {
				v := a.Get(key)
				if hashed {
					if !config.IsHashedSecret(v) {
						return fmt.Errorf("type 8 secrets must look like pbkdf2-sha256$...")
					}
				} else {
					h, err := config.HashSecret(v)
					if err != nil {
						return err
					}
					v = h
				}
				return s.change(func(c *config.Config) error { return set(c, a, v) })
			}
		}
		return kw("secret", "Specify a secret").add(
			kw("0", "Specifies an UNENCRYPTED secret will follow").add(arg(key, tSecret).run(apply(false))),
			kw("8", "Specifies a PBKDF2 HASHED secret will follow").add(arg(key, &ArgType{Name: "LINE", Help: "The hashed secret", Match: config.IsHashedSecret}).run(apply(true))),
			arg(key, tSecret).run(apply(false)),
		)
	}

	r.add(kw("enable", "Modify enable password parameters").add(
		secretNode("secret", func(c *config.Config, a *Args, h string) error { c.EnableSecret = h; return nil }).no(
			func(s *Session, a *Args) error {
				return s.change(func(c *config.Config) error { c.EnableSecret = ""; return nil })
			})))

	r.add(kw("username", "Establish User Name Authentication").add(
		arg("user", tUserName).add(
			secretNode("secret", func(c *config.Config, a *Args, h string) error {
				c.Users[a.Get("user")] = h
				return nil
			})).no(func(s *Session, a *Args) error {
			return s.change(func(c *config.Config) error {
				if _, ok := c.Users[a.Get("user")]; !ok {
					return errNoChange
				}
				delete(c.Users, a.Get("user"))
				return nil
			})
		})))

	r.add(kw("banner", "Define a login banner").add(
		kw("motd", "Set Message of the Day banner").add(
			arg("text", tLine).run(func(s *Session, a *Args) error {
				t := a.Get("text")
				if len(t) >= 2 && t[0] == t[len(t)-1] && strings.ContainsRune("^#%$", rune(t[0])) {
					t = strings.TrimSpace(t[1 : len(t)-1])
				}
				return s.change(func(c *config.Config) error { c.Banner = t; return nil })
			})).no(func(s *Session, a *Args) error {
			return s.change(func(c *config.Config) error { c.Banner = ""; return nil })
		})))
	return r
}

// errNoChange tells the manager that nothing needs to be applied.
var errNoChange = manager.ErrNoChange

// routeNode builds "ip route" or "ipv6 route".
func routeNode(v6 bool) *Node {
	add := func(s *Session, prefix, nh string) error {
		p, err := netip.ParsePrefix(prefix)
		if err != nil {
			return err
		}
		if p != p.Masked() {
			return fmt.Errorf("inconsistent address and mask (did you mean %s?)", p.Masked())
		}
		r := config.Route{Prefix: p.String(), NextHop: nh}
		return s.change(func(c *config.Config) error {
			for _, x := range c.Routes {
				if x.Prefix == r.Prefix && strings.EqualFold(x.NextHop, r.NextHop) {
					return errNoChange
				}
			}
			c.Routes = append(c.Routes, r)
			return nil
		})
	}
	del := func(s *Session, prefix, nh string) error {
		p, err := netip.ParsePrefix(prefix)
		if err != nil {
			return err
		}
		return s.change(func(c *config.Config) error {
			n := len(c.Routes)
			c.Routes = slices.DeleteFunc(c.Routes, func(x config.Route) bool {
				return x.Prefix == p.Masked().String() && (nh == "" || strings.EqualFold(x.NextHop, nh))
			})
			if len(c.Routes) == n {
				return fmt.Errorf("route not found")
			}
			return nil
		})
	}
	prefixOf := func(a *Args) string {
		if v6 {
			return a.Get("prefix")
		}
		return prefixFromArgs(a, "prefix", "addr", "mask")
	}
	nexthops := func() []*Node {
		nhType := tIPv4
		if v6 {
			nhType = tIPv6
		}
		return []*Node{
			kw("Null0", "Discard traffic (blackhole route)"),
			arg("nh", nhType).help("Forwarding router's address"),
		}
	}
	bind := func(nodes []*Node) []*Node {
		for _, n := range nodes {
			n.Run = func(s *Session, a *Args) error {
				nh := a.Get("nh")
				if a.Has("Null0") {
					nh = "null0"
				}
				return add(s, prefixOf(a), nh)
			}
			n.NoRun = func(s *Session, a *Args) error {
				nh := a.Get("nh")
				if a.Has("Null0") {
					nh = "null0"
				}
				return del(s, prefixOf(a), nh)
			}
		}
		return nodes
	}
	noPrefix := func(s *Session, a *Args) error { return del(s, prefixOf(a), "") }

	if v6 {
		return kw("route", "Establish static routes").add(
			arg("prefix", tPrefix6).no(noPrefix).add(bind(nexthops())...))
	}
	return kw("route", "Establish static routes").add(
		arg("addr", &ArgType{Name: "A.B.C.D", Help: "Destination prefix", Match: isIPv4}).add(
			arg("mask", &ArgType{Name: "A.B.C.D", Help: "Destination prefix mask", Match: isMask}).no(noPrefix).add(bind(nexthops())...)),
		arg("prefix", tPrefix4).no(noPrefix).add(bind(nexthops())...),
	)
}

func natNode() *Node {
	overload := func(s *Session, a *Args) error {
		src := prefixFromArgs(a, "prefix", "addr", "mask")
		p, err := netip.ParsePrefix(src)
		if err != nil {
			return err
		}
		if p != p.Masked() {
			return fmt.Errorf("inconsistent address and mask (did you mean %s?)", p.Masked())
		}
		r := config.NATRule{Type: config.NATOverload, Source: p.String(), Interface: a.Get("if")}
		return s.change(func(c *config.Config) error {
			if slices.Contains(c.NAT, r) {
				return errNoChange
			}
			c.NAT = append(c.NAT, r)
			return nil
		})
	}
	unOverload := func(s *Session, a *Args) error {
		src := prefixFromArgs(a, "prefix", "addr", "mask")
		p, _ := netip.ParsePrefix(src)
		r := config.NATRule{Type: config.NATOverload, Source: p.Masked().String(), Interface: a.Get("if")}
		return s.change(func(c *config.Config) error {
			n := len(c.NAT)
			c.NAT = slices.DeleteFunc(c.NAT, func(x config.NATRule) bool { return x == r })
			if len(c.NAT) == n {
				return fmt.Errorf("NAT rule not found")
			}
			return nil
		})
	}
	ovTail := func() *Node {
		return kw("interface", "Translate to the address of an interface").add(
			arg("if", tIfName).add(kw("overload", "Overload (PAT) the interface address").run(overload).no(unOverload)))
	}
	static := func(s *Session, a *Args, del bool) error {
		proto := "tcp"
		if a.Has("udp") {
			proto = "udp"
		}
		r := config.NATRule{Type: config.NATStatic, Protocol: proto, LocalAddr: a.Get("local"),
			LocalPort: atoi(a.Get("lport")), Interface: a.Get("if"), PublicPort: atoi(a.Get("gport"))}
		return s.change(func(c *config.Config) error {
			if del {
				n := len(c.NAT)
				c.NAT = slices.DeleteFunc(c.NAT, func(x config.NATRule) bool { return x == r })
				if len(c.NAT) == n {
					return fmt.Errorf("NAT rule not found")
				}
				return nil
			}
			for _, x := range c.NAT {
				if x.Type == config.NATStatic && x.Interface == r.Interface && x.Protocol == r.Protocol && x.PublicPort == r.PublicPort {
					return fmt.Errorf("%s port %d on %s is already forwarded", proto, r.PublicPort, r.Interface)
				}
			}
			c.NAT = append(c.NAT, r)
			return nil
		})
	}
	tPort := tNum(1, 65535, "Port number")
	staticTail := func() *Node {
		return arg("local", &ArgType{Name: "A.B.C.D", Help: "Inside local IP address", Match: isIPv4}).add(
			arg("lport", tPort).help("Local port").add(
				kw("interface", "Specify interface for global address").add(
					arg("if", tIfName).add(
						arg("gport", tPort).help("Global port").run(func(s *Session, a *Args) error { return static(s, a, false) }).no(
							func(s *Session, a *Args) error { return static(s, a, true) })))))
	}
	return kw("nat", "NAT configuration commands").add(
		kw("inside", "Inside address translation").add(
			kw("source", "Source address translation").add(
				kw("network", "Translate an inside network").add(
					arg("addr", &ArgType{Name: "A.B.C.D", Help: "Network address", Match: isIPv4}).add(
						arg("mask", &ArgType{Name: "A.B.C.D", Help: "Network mask", Match: isMask}).add(ovTail())),
					arg("prefix", tPrefix4).add(ovTail())),
				kw("static", "Static port forwarding").add(
					kw("tcp", "Transmission Control Protocol").add(staticTail()),
					kw("udp", "User Datagram Protocol").add(staticTail())))))
}

func httpNode() *Node {
	set := func(f func(w *config.WebConfig)) func(s *Session, a *Args) error {
		return func(s *Session, a *Args) error {
			return s.change(func(c *config.Config) error { f(&c.Web); return nil })
		}
	}
	return kw("http", "HTTP server configuration").add(
		kw("server", "Enable the web server (plain HTTP)").run(set(func(w *config.WebConfig) { w.Enabled, w.TLS = true, false })).
			no(set(func(w *config.WebConfig) {
				if !w.TLS {
					w.Enabled = false
				}
			})),
		kw("secure-server", "Enable the web server with HTTPS").run(set(func(w *config.WebConfig) { w.Enabled, w.TLS = true, true })).
			no(set(func(w *config.WebConfig) {
				if w.TLS {
					w.Enabled = false
				}
			})),
		kw("port", "HTTP(S) server port").add(arg("port", tNum(1, 65535, "Port number")).run(func(s *Session, a *Args) error {
			return set(func(w *config.WebConfig) { w.Port = atoi(a.Get("port")) })(s, a)
		})).no(set(func(w *config.WebConfig) { w.Port = 0 })),
		kw("bind", "Listen only on this address").add(arg("addr", &ArgType{Name: "A.B.C.D", Help: "Local address", Match: func(s string) bool { return isIPv4(s) || isIPv6(s) }}).run(func(s *Session, a *Args) error {
			return set(func(w *config.WebConfig) { w.Bind = a.Get("addr") })(s, a)
		})).no(set(func(w *config.WebConfig) { w.Bind = "" })),
	)
}

// ---- interface configuration ----------------------------------------------------

func (s *Session) changeIf(f func(c *config.Config, ifc *config.Interface) error) error {
	name := s.Iface
	return s.change(func(c *config.Config) error { return f(c, c.Interface(name)) })
}

func buildInterfaceTree() *Node {
	r := root(commonSubmode()...)
	r.add(kw("description", "Interface specific description").add(
		arg("text", &ArgType{Name: "LINE", Help: "Up to 240 characters describing this interface", Rest: true}).run(
			func(s *Session, a *Args) error {
				t := a.Get("text")
				if len(t) > 240 {
					return fmt.Errorf("description is too long")
				}
				return s.changeIf(func(_ *config.Config, i *config.Interface) error { i.Description = t; return nil })
			})).no(func(s *Session, a *Args) error {
		return s.changeIf(func(_ *config.Config, i *config.Interface) error { i.Description = ""; return nil })
	}))

	r.add(kw("shutdown", "Shutdown the selected interface").run(func(s *Session, a *Args) error {
		return s.changeIf(func(_ *config.Config, i *config.Interface) error { i.Shutdown = true; return nil })
	}).no(func(s *Session, a *Args) error {
		return s.changeIf(func(_ *config.Config, i *config.Interface) error { i.Shutdown = false; return nil })
	}))

	r.add(kw("mtu", "Set the interface Maximum Transmission Unit (MTU)").add(
		arg("mtu", tNum(576, 9216, "MTU size in bytes")).run(func(s *Session, a *Args) error {
			return s.changeIf(func(_ *config.Config, i *config.Interface) error { i.MTU = atoi(a.Get("mtu")); return nil })
		})).no(func(s *Session, a *Args) error {
		return s.changeIf(func(_ *config.Config, i *config.Interface) error { i.MTU = 0; return nil })
	}))

	r.add(kw("media", "Set media type (passed to ifconfig media)").add(
		arg("media", &ArgType{Name: "LINE", Help: "Media, e.g. 1000baseT full-duplex", Rest: true}).run(func(s *Session, a *Args) error {
			return s.changeIf(func(_ *config.Config, i *config.Interface) error { i.Media = a.Get("media"); return nil })
		})).no(func(s *Session, a *Args) error {
		return s.changeIf(func(_ *config.Config, i *config.Interface) error { i.Media = ""; return nil })
	}))

	setAddr := func(s *Session, a *Args) error {
		p := prefixFromArgs(a, "prefix", "addr", "mask")
		pr, err := netip.ParsePrefix(p)
		if err != nil {
			return err
		}
		if pr.Bits() < 31 && (pr.Addr() == pr.Masked().Addr()) {
			return fmt.Errorf("%s is the network address of %s", pr.Addr(), pr.Masked())
		}
		return s.changeIf(func(_ *config.Config, i *config.Interface) error {
			i.DHCP = false
			if a.Has("secondary") {
				if len(i.IPv4) == 0 {
					return fmt.Errorf("set a primary address first")
				}
				if !slices.Contains(i.IPv4, p) {
					i.IPv4 = append(i.IPv4, p)
				}
				return nil
			}
			i.IPv4 = slices.DeleteFunc(i.IPv4, func(x string) bool { return x == p })
			if len(i.IPv4) == 0 {
				i.IPv4 = []string{p}
			} else {
				i.IPv4[0] = p
			}
			return nil
		})
	}
	delAddr := func(s *Session, a *Args) error {
		p := prefixFromArgs(a, "prefix", "addr", "mask")
		return s.changeIf(func(_ *config.Config, i *config.Interface) error {
			n := len(i.IPv4)
			i.IPv4 = slices.DeleteFunc(i.IPv4, func(x string) bool { return x == p })
			if len(i.IPv4) == n {
				return fmt.Errorf("address %s not found on %s", p, s.Iface)
			}
			return nil
		})
	}
	secondary := func() *Node {
		return kw("secondary", "Make this IP address a secondary address").run(setAddr).no(delAddr)
	}
	ip := kw("ip", "Interface Internet Protocol config commands")
	r.add(ip)
	ip.add(kw("address", "Set the IP address of an interface").add(
		kw("dhcp", "IP address negotiated via DHCP").run(func(s *Session, a *Args) error {
			return s.changeIf(func(_ *config.Config, i *config.Interface) error { i.DHCP, i.IPv4 = true, nil; return nil })
		}).no(func(s *Session, a *Args) error {
			return s.changeIf(func(_ *config.Config, i *config.Interface) error { i.DHCP = false; return nil })
		}),
		arg("addr", &ArgType{Name: "A.B.C.D", Help: "IP address", Match: isIPv4}).add(
			arg("mask", &ArgType{Name: "A.B.C.D", Help: "IP subnet mask", Match: isMask}).run(setAddr).no(delAddr).add(secondary())),
		arg("prefix", tPrefix4).run(setAddr).no(delAddr).add(secondary()),
	).no(func(s *Session, a *Args) error {
		return s.changeIf(func(_ *config.Config, i *config.Interface) error { i.IPv4, i.DHCP = nil, false; return nil })
	}))

	group := func(s *Session, a *Args, del bool) error {
		return s.changeIf(func(_ *config.Config, i *config.Interface) error {
			target := &i.AccessGroupIn
			if a.Has("out") {
				target = &i.AccessGroupOut
			}
			if del {
				if a.Get("acl") != "" && *target != a.Get("acl") {
					return fmt.Errorf("access-list %s is not applied %s", a.Get("acl"), map[bool]string{true: "out", false: "in"}[a.Has("out")])
				}
				*target = ""
				return nil
			}
			*target = a.Get("acl")
			return nil
		})
	}
	dirs := func(withACL bool) []*Node {
		var out []*Node
		for _, d := range []struct{ w, h string }{{"in", "Inbound packets"}, {"out", "Outbound packets"}} {
			n := kw(d.w, d.h).no(func(s *Session, a *Args) error { return group(s, a, true) })
			if withACL {
				n.Run = func(s *Session, a *Args) error { return group(s, a, false) }
			}
			out = append(out, n)
		}
		return out
	}
	ip.add(kw("access-group", "Specify access control for packets").add(
		arg("acl", tACLName).add(dirs(true)...)).add(dirs(false)...))

	ipv6 := kw("ipv6", "IPv6 interface subcommands")
	r.add(ipv6)
	ipv6.add(kw("address", "Configure IPv6 address on interface").add(
		arg("prefix", tPrefix6).run(func(s *Session, a *Args) error {
			p := a.Get("prefix")
			return s.changeIf(func(_ *config.Config, i *config.Interface) error {
				if slices.Contains(i.IPv6, p) {
					return errNoChange
				}
				i.IPv6 = append(i.IPv6, p)
				return nil
			})
		}).no(func(s *Session, a *Args) error {
			p := a.Get("prefix")
			return s.changeIf(func(_ *config.Config, i *config.Interface) error {
				n := len(i.IPv6)
				i.IPv6 = slices.DeleteFunc(i.IPv6, func(x string) bool { return x == p })
				if len(i.IPv6) == n {
					return fmt.Errorf("address %s not found", p)
				}
				return nil
			})
		})).no(func(s *Session, a *Args) error {
		return s.changeIf(func(_ *config.Config, i *config.Interface) error { i.IPv6 = nil; return nil })
	}))

	r.add(kw("encapsulation", "Set encapsulation type for an interface").add(
		kw("dot1q", "IEEE 802.1Q Virtual LAN").add(
			arg("vid", tNum(1, 4094, "IEEE 802.1Q VLAN ID")).add(
				kw("interface", "Parent (trunk) interface").add(
					arg("parent", tIfName).run(func(s *Session, a *Args) error {
						if config.InterfaceKind(s.Iface) != config.KindVLAN {
							return fmt.Errorf("802.1Q encapsulation needs a vlanN interface (e.g. interface vlan%s)", a.Get("vid"))
						}
						return s.changeIf(func(_ *config.Config, i *config.Interface) error {
							i.VLAN = &config.VLAN{ID: atoi(a.Get("vid")), Parent: a.Get("parent")}
							return nil
						})
					})))).no(func(s *Session, a *Args) error {
			return s.changeIf(func(_ *config.Config, i *config.Interface) error { i.VLAN = nil; return nil })
		})))

	r.add(kw("bridge", "Bridge configuration").add(
		kw("member", "Add an interface to this bridge").add(
			arg("member", tIfName).run(func(s *Session, a *Args) error {
				if config.InterfaceKind(s.Iface) != config.KindBridge {
					return fmt.Errorf("bridge members can only be added to a bridgeN interface")
				}
				m := a.Get("member")
				return s.changeIf(func(c *config.Config, i *config.Interface) error {
					for bn, b := range c.Interfaces {
						if bn != s.Iface && b.Bridge != nil && slices.Contains(b.Bridge.Members, m) {
							return fmt.Errorf("%s is already a member of %s", m, bn)
						}
					}
					if i.Bridge == nil {
						i.Bridge = &config.Bridge{}
					}
					if slices.Contains(i.Bridge.Members, m) {
						return errNoChange
					}
					i.Bridge.Members = append(i.Bridge.Members, m)
					return nil
				})
			}).no(func(s *Session, a *Args) error {
				m := a.Get("member")
				return s.changeIf(func(_ *config.Config, i *config.Interface) error {
					if i.Bridge == nil || !slices.Contains(i.Bridge.Members, m) {
						return fmt.Errorf("%s is not a member", m)
					}
					i.Bridge.Members = slices.DeleteFunc(i.Bridge.Members, func(x string) bool { return x == m })
					if len(i.Bridge.Members) == 0 {
						i.Bridge = nil
					}
					return nil
				})
			}))))

	r.add(wirelessNode())
	return r
}

func wirelessNode() *Node {
	set := func(f func(w *config.Wireless, a *Args) error) func(s *Session, a *Args) error {
		return func(s *Session, a *Args) error {
			kind := config.InterfaceKind(s.Iface)
			if kind != config.KindPhysical && kind != config.KindWLAN {
				return fmt.Errorf("wireless settings apply to a radio (ath0, athn0 ...) or a wlanN interface")
			}
			return s.changeIf(func(_ *config.Config, i *config.Interface) error {
				if i.Wireless == nil {
					i.Wireless = &config.Wireless{Mode: config.ModeStation}
				}
				return f(i.Wireless, a)
			})
		}
	}
	str := func(get func(w *config.Wireless) *string, key string) func(s *Session, a *Args) error {
		return set(func(w *config.Wireless, a *Args) error { *get(w) = a.Get(key); return nil })
	}
	clear := func(f func(w *config.Wireless)) func(s *Session, a *Args) error {
		return func(s *Session, a *Args) error {
			return s.changeIf(func(_ *config.Config, i *config.Interface) error {
				if i.Wireless == nil {
					return errNoChange
				}
				f(i.Wireless)
				return nil
			})
		}
	}
	anyText := &ArgType{Name: "WORD", Help: "Text (use quotes for spaces)", Match: func(s string) bool { return s != "" }}

	return kw("wireless", "IEEE 802.11 settings").add(
		kw("mode", "Operating mode").add(
			kw("station", "Join a network as a client").run(set(func(w *config.Wireless, _ *Args) error { w.Mode = config.ModeStation; return nil })),
			kw("hostap", "Act as an access point").run(set(func(w *config.Wireless, _ *Args) error { w.Mode = config.ModeHostAP; return nil })),
		),
		kw("device", "Radio for a wlanN virtual interface (NetBSD 12 wifi stack)").add(
			arg("dev", tIfName).run(set(func(w *config.Wireless, a *Args) error { w.Device = a.Get("dev"); return nil }))).
			no(clear(func(w *config.Wireless) { w.Device = "" })),
		kw("ssid", "Network name").add(arg("ssid", anyText).help("SSID, up to 32 bytes").run(str(func(w *config.Wireless) *string { return &w.SSID }, "ssid"))).
			no(clear(func(w *config.Wireless) { w.SSID = "" })),
		kw("security", "Authentication and encryption").add(
			kw("open", "No encryption").run(set(func(w *config.Wireless, _ *Args) error { w.Security = config.SecurityOpen; return nil })),
			kw("wpa2-psk", "WPA2 with a pre-shared key").run(set(func(w *config.Wireless, _ *Args) error { w.Security = config.SecurityWPA2PSK; return nil })),
		).no(clear(func(w *config.Wireless) { w.Security = "" })),
		kw("psk", "WPA2 pre-shared key (passphrase)").add(arg("psk", &ArgType{Name: "WORD", Help: "8-63 characters", Match: func(s string) bool { return s != "" }}).run(
			str(func(w *config.Wireless) *string { return &w.PSK }, "psk"))).
			no(clear(func(w *config.Wireless) { w.PSK = "" })),
		kw("channel", "Radio channel (access point mode)").add(arg("ch", tNum(1, 196, "Channel number")).run(
			set(func(w *config.Wireless, a *Args) error { w.Channel = atoi(a.Get("ch")); return nil }))).
			no(clear(func(w *config.Wireless) { w.Channel = 0 })),
		kw("hw-mode", "Radio band / standard").add(
			kw("a", "802.11a (5 GHz)").run(set(func(w *config.Wireless, _ *Args) error { w.HWMode = "a"; return nil })),
			kw("b", "802.11b (2.4 GHz)").run(set(func(w *config.Wireless, _ *Args) error { w.HWMode = "b"; return nil })),
			kw("g", "802.11g (2.4 GHz)").run(set(func(w *config.Wireless, _ *Args) error { w.HWMode = "g"; return nil })),
		).no(clear(func(w *config.Wireless) { w.HWMode = "" })),
		kw("country", "Regulatory country code").add(arg("cc", &ArgType{Name: "CC", Help: "Two letter country code, e.g. NO", Match: func(s string) bool { return len(s) == 2 }}).run(
			set(func(w *config.Wireless, a *Args) error { w.Country = strings.ToUpper(a.Get("cc")); return nil }))).
			no(clear(func(w *config.Wireless) { w.Country = "" })),
		kw("hidden-ssid", "Do not broadcast / actively probe for the SSID").run(
			set(func(w *config.Wireless, _ *Args) error { w.Hidden = true; return nil })).
			no(clear(func(w *config.Wireless) { w.Hidden = false })),
	).no(func(s *Session, a *Args) error {
		return s.changeIf(func(_ *config.Config, i *config.Interface) error { i.Wireless = nil; return nil })
	})
}

// ---- access-list configuration ----------------------------------------------------

func buildACLTree() *Node {
	r := root(commonSubmode()...)
	rule := &ArgType{Name: "RULE", Help: aclRuleHelp, Rest: true}

	addEntry := func(s *Session, a *Args, action string) error {
		e, err := parseACE(action, a.Get("rule"))
		if err != nil {
			return err
		}
		name := s.ACL
		return s.change(func(c *config.Config) error {
			acl, ok := c.AccessLists[name]
			if !ok {
				acl = &config.AccessList{}
				c.AccessLists[name] = acl
			}
			if seq := a.Get("seq"); seq != "" {
				e.Seq = atoi(seq)
				for _, x := range acl.Entries {
					if x.Seq == e.Seq {
						return fmt.Errorf("duplicate sequence number %d", e.Seq)
					}
				}
			} else {
				last := 0
				for _, x := range acl.Entries {
					if sameRule(x, e) {
						return errNoChange
					}
					last = max(last, x.Seq)
				}
				e.Seq = (last/10 + 1) * 10
			}
			acl.Entries = append(acl.Entries, e)
			return nil
		})
	}
	delEntry := func(s *Session, a *Args, action string) error {
		e, err := parseACE(action, a.Get("rule"))
		if err != nil {
			return err
		}
		return s.change(func(c *config.Config) error {
			acl := c.AccessLists[s.ACL]
			if acl == nil {
				return fmt.Errorf("entry not found")
			}
			n := len(acl.Entries)
			acl.Entries = slices.DeleteFunc(acl.Entries, func(x config.ACE) bool { return sameRule(x, e) })
			if len(acl.Entries) == n {
				return fmt.Errorf("entry not found")
			}
			return nil
		})
	}
	entry := func() []*Node {
		return []*Node{
			kw("permit", "Specify packets to forward").add(arg("rule", rule).
				run(func(s *Session, a *Args) error { return addEntry(s, a, "permit") }).
				no(func(s *Session, a *Args) error { return delEntry(s, a, "permit") })),
			kw("deny", "Specify packets to reject").add(arg("rule", rule).
				run(func(s *Session, a *Args) error { return addEntry(s, a, "deny") }).
				no(func(s *Session, a *Args) error { return delEntry(s, a, "deny") })),
		}
	}
	r.add(entry()...)
	r.add(arg("seq", tNum(1, 2147483647, "Sequence Number")).add(entry()...).no(func(s *Session, a *Args) error {
		seq := atoi(a.Get("seq"))
		return s.change(func(c *config.Config) error {
			acl := c.AccessLists[s.ACL]
			if acl == nil {
				return fmt.Errorf("entry %d not found", seq)
			}
			n := len(acl.Entries)
			acl.Entries = slices.DeleteFunc(acl.Entries, func(x config.ACE) bool { return x.Seq == seq })
			if len(acl.Entries) == n {
				return fmt.Errorf("entry %d not found", seq)
			}
			return nil
		})
	}))
	r.add(kw("resequence", "Renumber entries 10, 20, 30 ...").run(func(s *Session, a *Args) error {
		return s.change(func(c *config.Config) error {
			acl := c.AccessLists[s.ACL]
			if acl == nil {
				return errNoChange
			}
			for i := range acl.Entries {
				acl.Entries[i].Seq = (i + 1) * 10
			}
			return nil
		})
	}))
	return r
}

// ---- DHCP pool configuration --------------------------------------------------------

func (s *Session) changePool(f func(p *config.DHCPPool) error) error {
	name := s.Pool
	return s.change(func(c *config.Config) error {
		p, ok := c.DHCPPools[name]
		if !ok {
			p = &config.DHCPPool{}
			c.DHCPPools[name] = p
		}
		return f(p)
	})
}

func buildDHCPTree() *Node {
	r := root(commonSubmode()...)
	setNet := func(s *Session, a *Args) error {
		p := prefixFromArgs(a, "prefix", "addr", "mask")
		pr, err := netip.ParsePrefix(p)
		if err != nil {
			return err
		}
		return s.changePool(func(pool *config.DHCPPool) error { pool.Network = pr.Masked().String(); return nil })
	}
	r.add(kw("network", "Network number and mask").add(
		arg("addr", &ArgType{Name: "A.B.C.D", Help: "Network number in dotted-decimal notation", Match: isIPv4}).add(
			arg("mask", &ArgType{Name: "A.B.C.D", Help: "Network mask", Match: isMask}).run(setNet)),
		arg("prefix", tPrefix4).run(setNet)))

	r.add(kw("range", "Range of addresses to lease").add(
		arg("start", tIPv4).help("First address").add(
			arg("end", tIPv4).help("Last address").run(func(s *Session, a *Args) error {
				return s.changePool(func(p *config.DHCPPool) error {
					p.RangeStart, p.RangeEnd = a.Get("start"), a.Get("end")
					return nil
				})
			}))).no(func(s *Session, a *Args) error {
		return s.changePool(func(p *config.DHCPPool) error { p.RangeStart, p.RangeEnd = "", ""; return nil })
	}))

	r.add(kw("default-router", "Default routers").add(
		repeat("gw", tIPv4, func(s *Session, a *Args) error {
			return s.changePool(func(p *config.DHCPPool) error { p.DefaultRouter = a.All("gw"); return nil })
		})).no(func(s *Session, a *Args) error {
		return s.changePool(func(p *config.DHCPPool) error { p.DefaultRouter = nil; return nil })
	}))

	r.add(kw("dns-server", "DNS servers").add(
		repeat("dns", &ArgType{Name: "A.B.C.D", Help: "Server address", Match: func(s string) bool { return isIPv4(s) || isIPv6(s) }},
			func(s *Session, a *Args) error {
				return s.changePool(func(p *config.DHCPPool) error { p.DNSServers = a.All("dns"); return nil })
			})).no(func(s *Session, a *Args) error {
		return s.changePool(func(p *config.DHCPPool) error { p.DNSServers = nil; return nil })
	}))

	r.add(kw("domain-name", "Domain name").add(
		arg("dn", tWord).help("Domain name").run(func(s *Session, a *Args) error {
			return s.changePool(func(p *config.DHCPPool) error { p.DomainName = a.Get("dn"); return nil })
		})).no(func(s *Session, a *Args) error {
		return s.changePool(func(p *config.DHCPPool) error { p.DomainName = ""; return nil })
	}))

	lease := func(s *Session, a *Args) error {
		secs := atoi(a.Get("d"))*86400 + atoi(a.Get("h"))*3600 + atoi(a.Get("m"))*60
		if secs < 60 {
			return fmt.Errorf("lease must be at least one minute")
		}
		return s.changePool(func(p *config.DHCPPool) error { p.LeaseSeconds = secs; return nil })
	}
	r.add(kw("lease", "Address lease time").add(
		arg("d", tNum(0, 365, "Days")).run(lease).add(
			arg("h", tNum(0, 23, "Hours")).run(lease).add(
				arg("m", tNum(0, 59, "Minutes")).run(lease)))).no(func(s *Session, a *Args) error {
		return s.changePool(func(p *config.DHCPPool) error { p.LeaseSeconds = 0; return nil })
	}))

	r.add(kw("host", "Static address reservation").add(
		arg("name", &ArgType{Name: "WORD", Help: "Host name", Match: config.ValidName}).add(
			arg("mac", tMAC).add(
				arg("ip", tIPv4).help("Reserved address").run(func(s *Session, a *Args) error {
					mac, _ := config.NormalizeMAC(a.Get("mac"))
					h := config.DHCPHost{Name: a.Get("name"), MAC: mac, Address: a.Get("ip")}
					return s.changePool(func(p *config.DHCPPool) error {
						p.Hosts = slices.DeleteFunc(p.Hosts, func(x config.DHCPHost) bool { return x.Name == h.Name })
						for _, x := range p.Hosts {
							if x.MAC == h.MAC || x.Address == h.Address {
								return fmt.Errorf("conflicts with reservation %s", x.Name)
							}
						}
						p.Hosts = append(p.Hosts, h)
						return nil
					})
				}))).no(func(s *Session, a *Args) error {
			return s.changePool(func(p *config.DHCPPool) error {
				n := len(p.Hosts)
				p.Hosts = slices.DeleteFunc(p.Hosts, func(x config.DHCPHost) bool { return x.Name == a.Get("name") })
				if len(p.Hosts) == n {
					return fmt.Errorf("reservation %s not found", a.Get("name"))
				}
				return nil
			})
		})))
	return r
}
