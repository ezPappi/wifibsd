package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/netcli/netcli/internal/config"
	"github.com/netcli/netcli/internal/manager"
	"github.com/netcli/netcli/internal/system"
)

func newTestSession(t *testing.T) (*Session, *bytes.Buffer, *system.Dry) {
	t.Helper()
	dir := t.TempDir()
	paths := system.Paths{EtcDir: dir + "/etc", RunDir: dir + "/run", DHCPLeases: dir + "/leases"}
	dry := &system.Dry{Sandbox: dir + "/sandbox"}
	var out bytes.Buffer
	s := &Session{Mgr: manager.New(paths, dry), Out: &out, Mode: ModeExec}
	return s, &out, dry
}

func run(t *testing.T, s *Session, out *bytes.Buffer, lines string) {
	t.Helper()
	for _, l := range strings.Split(lines, "\n") {
		if !s.Execute(l, -1) {
			t.Fatalf("command failed: %q\n%s", l, out.String())
		}
	}
}

const sample = `configure terminal
hostname gw01
ip domain-name home.lan
ip name-server 1.1.1.1 2606:4700::1111
ip routing
ipv6 unicast-routing
enable secret letmein
username admin secret Passw0rd!
banner motd ^Authorised access only^
ip access-list extended WAN-IN
 permit icmp any any echo
 permit tcp any host 192.0.2.10 eq ssh log
 permit udp 10.0.0.0 0.0.0.255 gt 1023 any eq domain
 deny ip any any log
interface wm0
 description WAN uplink
 ip address dhcp
 ip access-group WAN-IN in
interface wm1
 ip address 192.168.1.1 255.255.255.0
 ip address 192.168.2.1/24 secondary
 ipv6 address 2001:db8:1::1/64
 mtu 1500
interface ath0
 wireless mode hostap
 wireless ssid "Home \"5\" net"
 wireless security wpa2-psk
 wireless psk "correct horse"
 wireless channel 11
 wireless country no
 wireless hidden-ssid
interface wlan0
 wireless device athn0
 wireless mode station
 wireless ssid Upstream
 wireless security open
interface vlan20
 encapsulation dot1q 20 interface wm1
 ip address 10.20.0.1 255.255.255.0
 shutdown
interface bridge0
 bridge member ath0
 description wifi bridge
ip route 0.0.0.0 0.0.0.0 192.0.2.1
ip route 10.99.0.0/16 Null0
ipv6 route 2001:db8:99::/48 2001:db8:1::fe
ip nat inside source network 192.168.1.0 255.255.255.0 interface wm0 overload
ip nat inside source static udp 192.168.1.20 51820 interface wm0 51820
ip dhcp pool LAN
 network 192.168.1.0 255.255.255.0
 range 192.168.1.100 192.168.1.200
 default-router 192.168.1.1
 dns-server 192.168.1.1
 domain-name home.lan
 lease 0 8
 host nas 00:11:22:33:44:55 192.168.1.10
ip http secure-server
ip http bind 192.168.1.1
end`

func TestRoundTrip(t *testing.T) {
	s, out, _ := newTestSession(t)
	run(t, s, out, sample)
	c1, _ := s.Mgr.Running()
	text := ConfigText(c1, "")

	s2, out2, _ := newTestSession(t)
	run(t, s2, out2, "configure terminal\n"+text)
	c2, _ := s2.Mgr.Running()
	if !config.Equal(c1, c2) {
		a, _ := config.Marshal(c1)
		b, _ := config.Marshal(c2)
		t.Fatalf("round trip differs:\n%s\n----\n%s\n----\n%s", text, a, b)
	}
	if c1.Interfaces["ath0"].Wireless.SSID != `Home "5" net` {
		t.Errorf("ssid = %q", c1.Interfaces["ath0"].Wireless.SSID)
	}
	if c1.Banner != "Authorised access only" {
		t.Errorf("banner = %q", c1.Banner)
	}
	if !config.CheckSecret(c1.EnableSecret, "letmein") {
		t.Error("enable secret does not verify")
	}
}

func TestErrorsAndAbbreviations(t *testing.T) {
	s, out, _ := newTestSession(t)
	cases := []struct{ line, want string }{
		{"conf t", ""},
		{"int wm1", ""},
		{"ip adress 1.2.3.4/24", "Invalid input"},
		{"i", "Ambiguous"},
		{"ip address", "Incomplete"},
		{"ip address 10.0.0.0/24", "network address"},
		{"ip address 10.0.0.1/24", ""},
		{"ip address 10.0.1.1/24 secondary", ""},
		{"no ip address 10.9.9.9/24", "not found"},
		{"exit", ""},
		{"ip route 10.0.0.1 255.0.0.0 1.2.3.4", "inconsistent"},
		{"interface vlan5", ""},
		{"description test", "missing encapsulation"},
		{"encapsulation dot1q 5 interface wm1", ""},
		{"description test", ""},
		{"exit", ""},
		{"no interface vlan5", ""},
		{"ip access-list extended X", ""},
		{"permit tcp any any neq 22", "neq"},
		{"permit tcp any any eq 22", ""},
		{"permit tcp any any eq 22", ""}, // duplicate is ignored
		{"no permit tcp any any eq 22", ""},
		{"no permit tcp any any eq 22", "not found"},
	}
	for _, c := range cases {
		out.Reset()
		ok := s.Execute(c.line, -1)
		if c.want == "" && !ok {
			t.Errorf("%q failed: %s", c.line, out.String())
		}
		if c.want != "" && !strings.Contains(out.String(), c.want) {
			t.Errorf("%q: want %q in output, got %q", c.line, c.want, out.String())
		}
	}
}

func TestSubmodeAcceptsGlobalCommands(t *testing.T) {
	s, out, _ := newTestSession(t)
	run(t, s, out, "configure terminal\ninterface wm1\nhostname edge")
	if s.Mode != ModeConfig {
		t.Errorf("mode = %v, want global config", s.Mode)
	}
	if c, _ := s.Mgr.Running(); c.Hostname != "edge" {
		t.Errorf("hostname = %q", c.Hostname)
	}
}

func TestHelpAndComplete(t *testing.T) {
	s, _, _ := newTestSession(t)
	h := s.Help("show ip ")
	for _, w := range []string{"interface", "route", "dhcp"} {
		if !strings.Contains(h, w) {
			t.Errorf("help missing %q:\n%s", w, h)
		}
	}
	if got := s.Complete("conf"); len(got) != 1 || got[0] != "configure" {
		t.Errorf("complete conf = %v", got)
	}
	s.Mode = ModeConfig
	if h := s.Help("no ip "); !strings.Contains(h, "routing") {
		t.Errorf("no-form help:\n%s", h)
	}
	if h := s.Help("hostname x"); !strings.Contains(h, "WORD") && !strings.Contains(h, "<cr>") {
		t.Errorf("help after argument:\n%s", h)
	}
}

func TestACEParse(t *testing.T) {
	cases := []struct{ in, want string }{
		{"tcp any host 192.0.2.1 eq www", "permit tcp any host 192.0.2.1 eq 80"},
		{"tcp 10.0.0.0 0.255.255.255 range 1000 2000 any log", "permit tcp 10.0.0.0 0.255.255.255 range 1000 2000 any log"},
		{"udp any lt 1024 10.1.0.0/16", "permit udp any range 1 1023 10.1.0.0 0.0.255.255"},
		{"icmp any any echo-reply", "permit icmp any any echo-reply"},
		{"icmpv6 any any 135", "permit icmpv6 any any nd-ns"},
		{"6 any any stateless", "permit tcp any any stateless"},
		{"ip 2001:db8::/32 any", "permit ip 2001:db8::/32 any"},
	}
	for _, c := range cases {
		e, err := parseACE("permit", c.in)
		if err != nil {
			t.Errorf("%q: %v", c.in, err)
			continue
		}
		if got := formatACE(e); got != c.want {
			t.Errorf("%q: got %q want %q", c.in, got, c.want)
		}
	}
	for _, bad := range []string{"tcp any", "icmp any any 2001:db8::/32", "tcp 10.0.0.1 0.0.0.255 any", "udp any any eq 70000", "ip any any eq 22", "icmp 2001:db8::/32 any"} {
		if _, err := parseACE("permit", bad); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}

func TestTokenizeQuote(t *testing.T) {
	for _, s := range []string{"plain", "with space", `quote " inside`, `back\slash`, ""} {
		toks, err := tokenize("x " + quote(s))
		if err != nil || len(toks) != 2 || toks[1].text != s {
			if !(s == "" && len(toks) == 2 && toks[1].text == "") {
				t.Errorf("quote(%q) -> %v %v", s, toks, err)
			}
		}
	}
}

func TestRevertTimerAndConfirm(t *testing.T) {
	s, out, _ := newTestSession(t)
	// ArmRevert spawns the test binary as watchdog; keep the deadline far
	// away and confirm immediately.
	s.RevertArgs = []string{"-test.run=^$"}
	run(t, s, out, "configure terminal revert timer 60\nhostname changed\nend")
	if _, ok := s.Mgr.RevertPending(); !ok {
		t.Fatalf("revert not pending: %s", out.String())
	}
	run(t, s, out, "configure revert now")
	if c, _ := s.Mgr.Running(); c.Hostname == "changed" {
		t.Error("revert did not restore the hostname")
	}
	if _, ok := s.Mgr.RevertPending(); ok {
		t.Error("revert still pending after revert now")
	}
}
