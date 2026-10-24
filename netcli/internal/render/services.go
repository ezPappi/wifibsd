package render

import (
	"encoding/hex"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/netcli/netcli/internal/config"
)

// Hostapd renders hostapd.conf for an access point interface. The bsd
// driver is the hostapd backend for the NetBSD net80211 stack.
func Hostapd(ifname string, w *config.Wireless, bridge string) string {
	var b strings.Builder
	b.WriteString(header)
	fmt.Fprintf(&b, "interface=%s\n", ifname)
	if bridge != "" {
		fmt.Fprintf(&b, "bridge=%s\n", bridge)
	}
	b.WriteString("driver=bsd\n")
	b.WriteString("ctrl_interface=/var/run/hostapd\n")
	b.WriteString("ctrl_interface_group=0\n")
	fmt.Fprintf(&b, "ssid2=%s\n", hostapdSSID(w.SSID))
	hw := w.HWMode
	if hw == "" {
		hw = "g"
		if w.Channel > 14 {
			hw = "a"
		}
	}
	fmt.Fprintf(&b, "hw_mode=%s\n", hw)
	ch := w.Channel
	if ch == 0 {
		ch = 6
		if hw == "a" {
			ch = 36
		}
	}
	fmt.Fprintf(&b, "channel=%d\n", ch)
	if w.Country != "" {
		fmt.Fprintf(&b, "country_code=%s\nieee80211d=1\n", w.Country)
	}
	if w.Hidden {
		b.WriteString("ignore_broadcast_ssid=1\n")
	}
	if w.Security == config.SecurityWPA2PSK {
		b.WriteString("wpa=2\nwpa_key_mgmt=WPA-PSK\nrsn_pairwise=CCMP\n")
		fmt.Fprintf(&b, "wpa_passphrase=%s\n", w.PSK)
	}
	return b.String()
}

// hostapdSSID encodes an SSID for the ssid2 option: quoted when printable,
// otherwise as hex.
func hostapdSSID(s string) string {
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == '"' {
			return hex.EncodeToString([]byte(s))
		}
	}
	return "\"" + s + "\""
}

// WPASupplicant renders wpa_supplicant.conf for a station interface.
func WPASupplicant(w *config.Wireless) string {
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("ctrl_interface=/var/run/wpa_supplicant\n")
	b.WriteString("ctrl_interface_group=0\n")
	if w.Country != "" {
		fmt.Fprintf(&b, "country=%s\n", w.Country)
	}
	b.WriteString("\nnetwork={\n")
	fmt.Fprintf(&b, "\tssid=%s\n", hostapdSSID(w.SSID))
	if w.Hidden {
		b.WriteString("\tscan_ssid=1\n")
	}
	if w.Security == config.SecurityWPA2PSK {
		b.WriteString("\tkey_mgmt=WPA-PSK\n\tproto=RSN\n")
		fmt.Fprintf(&b, "\tpsk=\"%s\"\n", w.PSK)
	} else {
		b.WriteString("\tkey_mgmt=NONE\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// Dhcpd renders dhcpd.conf and returns the interfaces it must listen on.
func Dhcpd(c *config.Config) (string, []string) {
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("authoritative;\nddns-update-style none;\n\n")

	names := make([]string, 0, len(c.DHCPPools))
	for n := range c.DHCPPools {
		names = append(names, n)
	}
	sort.Strings(names)

	ifset := map[string]bool{}
	for _, name := range names {
		p := c.DHCPPools[name]
		subnet, err := netip.ParsePrefix(p.Network)
		if err != nil {
			continue
		}
		subnet = subnet.Masked()
		for ifn, ifc := range c.Interfaces {
			for _, a := range ifc.IPv4 {
				if ap, err := netip.ParsePrefix(a); err == nil && ap.Masked() == subnet {
					ifset[ifn] = true
				}
			}
		}
		fmt.Fprintf(&b, "# pool %s\n", name)
		fmt.Fprintf(&b, "subnet %s netmask %s {\n", subnet.Addr(), bitsToMask(subnet.Bits()))
		if p.RangeStart != "" {
			fmt.Fprintf(&b, "\trange %s %s;\n", p.RangeStart, p.RangeEnd)
		}
		if len(p.DefaultRouter) > 0 {
			fmt.Fprintf(&b, "\toption routers %s;\n", strings.Join(p.DefaultRouter, ", "))
		}
		if len(p.DNSServers) > 0 {
			fmt.Fprintf(&b, "\toption domain-name-servers %s;\n", strings.Join(p.DNSServers, ", "))
		}
		if p.DomainName != "" {
			fmt.Fprintf(&b, "\toption domain-name \"%s\";\n", p.DomainName)
		}
		if p.LeaseSeconds > 0 {
			fmt.Fprintf(&b, "\tdefault-lease-time %d;\n\tmax-lease-time %d;\n", p.LeaseSeconds, p.LeaseSeconds*2)
		}
		for _, h := range p.Hosts {
			mac, _ := config.NormalizeMAC(h.MAC)
			fmt.Fprintf(&b, "\thost %s {\n\t\thardware ethernet %s;\n\t\tfixed-address %s;\n\t}\n", h.Name, mac, h.Address)
		}
		b.WriteString("}\n\n")
	}
	ifs := sortedKeys(ifset)
	config.SortInterfaceNames(ifs)
	return b.String(), ifs
}

func bitsToMask(bits int) string {
	var m uint32
	if bits > 0 {
		m = ^uint32(0) << (32 - bits)
	}
	return fmt.Sprintf("%d.%d.%d.%d", m>>24, (m>>16)&0xff, (m>>8)&0xff, m&0xff)
}

// BitsToMask converts a prefix length to a dotted IPv4 netmask.
func BitsToMask(bits int) string { return bitsToMask(bits) }

// Resolv renders resolv.conf.
func Resolv(c *config.Config) string {
	var b strings.Builder
	b.WriteString(header)
	if c.DomainName != "" {
		fmt.Fprintf(&b, "search %s\n", c.DomainName)
	}
	for _, ns := range c.NameServers {
		fmt.Fprintf(&b, "nameserver %s\n", ns)
	}
	return b.String()
}
