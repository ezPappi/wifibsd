package apply

import (
	"strings"
	"testing"

	"github.com/netcli/netcli/internal/config"
	"github.com/netcli/netcli/internal/system"
)

func base() *config.Config {
	c := config.New()
	c.Hostname = "gw"
	c.IPRouting = true
	c.Interfaces["wm0"] = &config.Interface{DHCP: true}
	c.Interfaces["wm1"] = &config.Interface{IPv4: []string{"192.168.1.1/24"}}
	c.Routes = []config.Route{{Prefix: "10.0.0.0/8", NextHop: "192.168.1.254"}}
	c.NAT = []config.NATRule{{Type: config.NATOverload, Interface: "wm0", Source: "192.168.1.0/24"}}
	return c
}

func newApplier(t *testing.T) (*Applier, *system.Dry) {
	d := &system.Dry{Sandbox: t.TempDir()}
	return &Applier{Exec: d, Paths: system.Paths{RunDir: "/run", DHCPLeases: "/leases"}}, d
}

func has(log []string, want string) bool {
	for _, l := range log {
		if l == want {
			return true
		}
	}
	return false
}

func TestBootApply(t *testing.T) {
	a, d := newApplier(t)
	if err := a.Apply(nil, base(), true); err != nil {
		t.Fatal(err)
	}
	log := d.TakeLog()
	for _, want := range []string{
		"hostname gw",
		"sysctl -w net.inet.ip.forwarding=1",
		"ifconfig wm0 up",
		"dhcpcd -b wm0",
		"ifconfig wm1 inet 192.168.1.1 netmask 255.255.255.0 alias",
		"route -n -q add -inet 10.0.0.0/8 192.168.1.254",
		"npfctl validate /run/npf.conf",
		"npfctl reload /run/npf.conf",
	} {
		if !has(log, want) {
			t.Errorf("missing %q in\n%s", want, strings.Join(log, "\n"))
		}
	}
}

func TestIncrementalChangeIsMinimal(t *testing.T) {
	a, d := newApplier(t)
	old := base()
	next := base()
	next.Interfaces["wm1"].IPv4 = []string{"192.168.1.2/24"}
	next.Interfaces["wm1"].Description = "LAN"
	if err := a.Apply(old, next, false); err != nil {
		t.Fatal(err)
	}
	got := d.TakeLog()
	want := []string{
		"ifconfig wm1 inet 192.168.1.1 -alias",
		"ifconfig wm1 inet 192.168.1.2 netmask 255.255.255.0 alias",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestShutdownAndRoutes(t *testing.T) {
	a, d := newApplier(t)
	old := base()
	next := base()
	next.Interfaces["wm0"].Shutdown = true
	next.Routes = []config.Route{{Prefix: "0.0.0.0/0", NextHop: "null0"}}
	if err := a.Apply(old, next, false); err != nil {
		t.Fatal(err)
	}
	log := d.TakeLog()
	for _, want := range []string{
		"dhcpcd -x wm0",
		"ifconfig wm0 down",
		"route -n -q delete -inet 10.0.0.0/8",
		"route -n -q add -inet default 127.0.0.1 -blackhole",
	} {
		if !has(log, want) {
			t.Errorf("missing %q in\n%s", want, strings.Join(log, "\n"))
		}
	}
}

func TestIncompleteWirelessNotStarted(t *testing.T) {
	a, d := newApplier(t)
	old := base()
	next := base()
	next.Interfaces["ath0"] = &config.Interface{Wireless: &config.Wireless{Mode: config.ModeHostAP, SSID: "x", Security: config.SecurityWPA2PSK}}
	next.Interfaces["wlan0"] = &config.Interface{Wireless: &config.Wireless{Mode: config.ModeStation}}
	if err := a.Apply(old, next, false); err != nil {
		t.Fatal(err)
	}
	for _, l := range d.TakeLog() {
		if strings.Contains(l, "hostapd") && !strings.HasPrefix(l, "stop") || strings.Contains(l, "wlan0") {
			t.Errorf("unexpected action for incomplete wireless: %s", l)
		}
	}
	// Completing the PSK starts hostapd.
	done := next.Clone()
	done.Interfaces["ath0"].Wireless.PSK = "longenough"
	if err := a.Apply(next, done, false); err != nil {
		t.Fatal(err)
	}
	log := d.TakeLog()
	if !has(log, "ifconfig ath0 mediaopt hostap") || !has(log, "hostapd -B -P /run/hostapd.ath0.pid /run/hostapd.ath0.conf") {
		t.Errorf("hostapd not started:\n%s", strings.Join(log, "\n"))
	}
}

func TestVAPRecreatedOnModeChange(t *testing.T) {
	a, d := newApplier(t)
	c1 := base()
	c1.Interfaces["wlan0"] = &config.Interface{Wireless: &config.Wireless{Device: "ath0", Mode: config.ModeStation, SSID: "up", Security: config.SecurityOpen}}
	c2 := c1.Clone()
	c2.Interfaces["wlan0"].Wireless.Mode = config.ModeHostAP
	if err := a.Apply(c1, c2, false); err != nil {
		t.Fatal(err)
	}
	log := d.TakeLog()
	if !has(log, "ifconfig wlan0 destroy") || !has(log, "ifconfig wlan0 create wlandev ath0 wlanmode hostap") {
		t.Errorf("VAP not recreated:\n%s", strings.Join(log, "\n"))
	}
}
