package cli

import (
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"github.com/netcli/netcli/internal/config"
	"github.com/netcli/netcli/internal/render"
)

// ConfigText renders a configuration as CLI commands, the way IOS shows its
// running configuration. Feeding the text back into the CLI recreates the
// configuration. If only is set, just that interface is shown.
func ConfigText(c *config.Config, only string) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format+"\n", a...) }

	if only != "" {
		ifc, ok := c.Interfaces[only]
		if !ok {
			return ""
		}
		interfaceText(&b, only, ifc)
		w("end")
		return b.String()
	}

	w("!")
	if c.Hostname != "" {
		w("hostname %s", c.Hostname)
	}
	if c.DomainName != "" {
		w("ip domain-name %s", c.DomainName)
	}
	if len(c.NameServers) > 0 {
		w("ip name-server %s", strings.Join(c.NameServers, " "))
	}
	if c.IPRouting {
		w("ip routing")
	}
	if c.IPv6Routing {
		w("ipv6 unicast-routing")
	}
	w("!")
	if c.EnableSecret != "" {
		w("enable secret 8 %s", c.EnableSecret)
	}
	users := make([]string, 0, len(c.Users))
	for u := range c.Users {
		users = append(users, u)
	}
	sort.Strings(users)
	for _, u := range users {
		w("username %s secret 8 %s", u, c.Users[u])
	}
	if c.Banner != "" {
		w("banner motd %s", c.Banner)
	}
	if c.EnableSecret != "" || len(users) > 0 || c.Banner != "" {
		w("!")
	}

	// Access lists come before the interfaces that reference them so the
	// text can be pasted back in.
	acls := make([]string, 0, len(c.AccessLists))
	for n := range c.AccessLists {
		acls = append(acls, n)
	}
	sort.Strings(acls)
	for _, n := range acls {
		w("ip access-list extended %s", n)
		for _, e := range c.AccessLists[n].Entries {
			w(" %d %s", e.Seq, formatACE(e))
		}
		w("!")
	}

	for _, n := range c.InterfaceNames() {
		interfaceText(&b, n, c.Interfaces[n])
		w("!")
	}

	for _, r := range c.Routes {
		p, err := netip.ParsePrefix(r.Prefix)
		if err != nil {
			continue
		}
		nh := r.NextHop
		if strings.EqualFold(nh, "null0") {
			nh = "Null0"
		}
		if p.Addr().Is4() {
			w("ip route %s %s %s", p.Addr(), render.BitsToMask(p.Bits()), nh)
		} else {
			w("ipv6 route %s %s", p, nh)
		}
	}
	if len(c.Routes) > 0 {
		w("!")
	}

	for _, n := range c.NAT {
		switch n.Type {
		case config.NATOverload:
			p, _ := netip.ParsePrefix(n.Source)
			w("ip nat inside source network %s %s interface %s overload", p.Addr(), render.BitsToMask(p.Bits()), n.Interface)
		case config.NATStatic:
			w("ip nat inside source static %s %s %d interface %s %d", n.Protocol, n.LocalAddr, n.LocalPort, n.Interface, n.PublicPort)
		}
	}
	if len(c.NAT) > 0 {
		w("!")
	}

	pools := make([]string, 0, len(c.DHCPPools))
	for n := range c.DHCPPools {
		pools = append(pools, n)
	}
	sort.Strings(pools)
	for _, n := range pools {
		p := c.DHCPPools[n]
		w("ip dhcp pool %s", n)
		if pr, err := netip.ParsePrefix(p.Network); err == nil {
			w(" network %s %s", pr.Addr(), render.BitsToMask(pr.Bits()))
		}
		if p.RangeStart != "" {
			w(" range %s %s", p.RangeStart, p.RangeEnd)
		}
		if len(p.DefaultRouter) > 0 {
			w(" default-router %s", strings.Join(p.DefaultRouter, " "))
		}
		if len(p.DNSServers) > 0 {
			w(" dns-server %s", strings.Join(p.DNSServers, " "))
		}
		if p.DomainName != "" {
			w(" domain-name %s", p.DomainName)
		}
		if p.LeaseSeconds > 0 {
			s := p.LeaseSeconds
			w(" lease %d %d %d", s/86400, (s%86400)/3600, (s%3600)/60)
		}
		for _, h := range p.Hosts {
			w(" host %s %s %s", h.Name, h.MAC, h.Address)
		}
		w("!")
	}

	if c.Web.Enabled {
		if c.Web.TLS {
			w("ip http secure-server")
		} else {
			w("ip http server")
		}
	}
	if c.Web.Port != 0 {
		w("ip http port %d", c.Web.Port)
	}
	if c.Web.Bind != "" {
		w("ip http bind %s", c.Web.Bind)
	}
	if c.Web.Enabled || c.Web.Port != 0 || c.Web.Bind != "" {
		w("!")
	}
	w("end")
	return b.String()
}

func interfaceText(b *strings.Builder, n string, ifc *config.Interface) {
	w := func(format string, a ...any) { fmt.Fprintf(b, format+"\n", a...) }
	w("interface %s", n)
	if ifc.Description != "" {
		w(" description %s", ifc.Description)
	}
	if v := ifc.VLAN; v != nil {
		w(" encapsulation dot1q %d interface %s", v.ID, v.Parent)
	}
	if br := ifc.Bridge; br != nil {
		for _, m := range br.Members {
			w(" bridge member %s", m)
		}
	}
	if wl := ifc.Wireless; wl != nil {
		if wl.Device != "" {
			w(" wireless device %s", wl.Device)
		}
		if wl.Mode != "" {
			w(" wireless mode %s", wl.Mode)
		}
		if wl.SSID != "" {
			w(" wireless ssid %s", quote(wl.SSID))
		}
		if wl.Security != "" {
			w(" wireless security %s", wl.Security)
		}
		if wl.PSK != "" {
			w(" wireless psk %s", quote(wl.PSK))
		}
		if wl.Channel != 0 {
			w(" wireless channel %d", wl.Channel)
		}
		if wl.HWMode != "" {
			w(" wireless hw-mode %s", wl.HWMode)
		}
		if wl.Country != "" {
			w(" wireless country %s", wl.Country)
		}
		if wl.Hidden {
			w(" wireless hidden-ssid")
		}
	}
	if ifc.MTU != 0 {
		w(" mtu %d", ifc.MTU)
	}
	if ifc.Media != "" {
		w(" media %s", ifc.Media)
	}
	if ifc.DHCP {
		w(" ip address dhcp")
	}
	for i, a := range ifc.IPv4 {
		p, err := netip.ParsePrefix(a)
		if err != nil {
			continue
		}
		sec := ""
		if i > 0 {
			sec = " secondary"
		}
		w(" ip address %s %s%s", p.Addr(), render.BitsToMask(p.Bits()), sec)
	}
	for _, a := range ifc.IPv6 {
		w(" ipv6 address %s", a)
	}
	if ifc.AccessGroupIn != "" {
		w(" ip access-group %s in", ifc.AccessGroupIn)
	}
	if ifc.AccessGroupOut != "" {
		w(" ip access-group %s out", ifc.AccessGroupOut)
	}
	if ifc.Shutdown {
		w(" shutdown")
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
