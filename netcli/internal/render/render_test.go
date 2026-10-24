package render

import (
	"strings"
	"testing"

	"github.com/netcli/netcli/internal/config"
)

func TestNPF(t *testing.T) {
	c := config.New()
	eight := 8
	c.AccessLists["IN"] = &config.AccessList{Entries: []config.ACE{
		{Seq: 10, Action: "permit", Protocol: "icmp", Source: "any", Destination: "any", ICMPType: &eight},
		{Seq: 20, Action: "permit", Protocol: "tcp", Source: "any", Destination: "192.0.2.1/32", DestinationPort: "22", Log: true},
		{Seq: 30, Action: "deny", Protocol: "udp", Source: "10.0.0.0/8", SourcePort: "1-1023", Destination: "any"},
		{Seq: 40, Action: "permit", Protocol: "udp", Source: "any", Destination: "any", Stateless: true},
	}}
	c.Interfaces["wm0"] = &config.Interface{DHCP: true, IPv6: []string{"2001:db8::1/64"}, AccessGroupIn: "IN"}
	c.NAT = []config.NATRule{
		{Type: config.NATOverload, Interface: "wm0", Source: "192.168.1.0/24"},
		{Type: config.NATStatic, Interface: "wm0", Protocol: "tcp", LocalAddr: "192.168.1.5", LocalPort: 80, PublicPort: 8080},
	}
	got := NPF(c)
	for _, want := range []string{
		"procedure \"log\" {\n\tlog: npflog0\n}",
		"map wm0 dynamic 192.168.1.0/24 -> ifaddrs(wm0)",
		"map wm0 dynamic proto tcp 192.168.1.5 port 80 <- ifaddrs(wm0) port 8080",
		"group \"wm0\" on wm0 {",
		"pass in final proto udp from any port 67 to any port 68",
		"pass in final family inet6 proto ipv6-icmp icmp-type 135 all",
		"pass stateful in final proto icmp icmp-type 8 all",
		"pass stateful in final proto tcp from any to 192.0.2.1 port 22 apply \"log\"",
		"block in final proto udp from 10.0.0.0/8 port 1-1023 to any",
		"pass in final proto udp all",
		"block in final all",
		"pass stateful out final all",
		"group default {",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	// Order matters: entries keep their sequence order before the implicit deny.
	if strings.Index(got, "icmp-type 8") > strings.Index(got, "block in final all") {
		t.Error("implicit deny before entries")
	}
}

func TestHostapdHexSSID(t *testing.T) {
	w := &config.Wireless{Mode: config.ModeHostAP, SSID: `a"b`, Security: config.SecurityOpen}
	if got := Hostapd("ath0", w, ""); !strings.Contains(got, "ssid2=612262\n") {
		t.Errorf("expected hex ssid:\n%s", got)
	}
}

func TestDhcpdInterfaces(t *testing.T) {
	c := config.New()
	c.Interfaces["bridge0"] = &config.Interface{IPv4: []string{"192.168.1.1/24"}}
	c.DHCPPools["LAN"] = &config.DHCPPool{Network: "192.168.1.0/24", RangeStart: "192.168.1.50", RangeEnd: "192.168.1.99", LeaseSeconds: 3600}
	conf, ifs := Dhcpd(c)
	if len(ifs) != 1 || ifs[0] != "bridge0" {
		t.Errorf("interfaces = %v", ifs)
	}
	if !strings.Contains(conf, "subnet 192.168.1.0 netmask 255.255.255.0 {") || !strings.Contains(conf, "range 192.168.1.50 192.168.1.99;") {
		t.Errorf("dhcpd.conf:\n%s", conf)
	}
}
