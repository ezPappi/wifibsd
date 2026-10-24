// Package config defines the netcli configuration model. The same model is
// used for the startup configuration (persisted as YAML) and the running
// configuration (the state currently applied to the system).
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sort"

	"gopkg.in/yaml.v3"
)

// Config is the complete router configuration.
type Config struct {
	Hostname    string   `yaml:"hostname,omitempty" json:"hostname,omitempty"`
	DomainName  string   `yaml:"domain-name,omitempty" json:"domain-name,omitempty"`
	NameServers []string `yaml:"name-servers,omitempty" json:"name-servers,omitempty"`
	Banner      string   `yaml:"banner,omitempty" json:"banner,omitempty"`

	// IPRouting enables IPv4 forwarding (net.inet.ip.forwarding).
	IPRouting bool `yaml:"ip-routing,omitempty" json:"ip-routing,omitempty"`
	// IPv6Routing enables IPv6 forwarding (net.inet6.ip6.forwarding).
	IPv6Routing bool `yaml:"ipv6-routing,omitempty" json:"ipv6-routing,omitempty"`

	// EnableSecret is a password hash (see HashSecret) required by the
	// CLI "enable" command. Empty means no password.
	EnableSecret string `yaml:"enable-secret,omitempty" json:"enable-secret,omitempty"`
	// Users maps user names to password hashes. Used by the web service.
	Users map[string]string `yaml:"users,omitempty" json:"users,omitempty"`

	Interfaces  map[string]*Interface  `yaml:"interfaces,omitempty" json:"interfaces,omitempty"`
	Routes      []Route                `yaml:"routes,omitempty" json:"routes,omitempty"`
	AccessLists map[string]*AccessList `yaml:"access-lists,omitempty" json:"access-lists,omitempty"`
	NAT         []NATRule              `yaml:"nat,omitempty" json:"nat,omitempty"`
	DHCPPools   map[string]*DHCPPool   `yaml:"dhcp-pools,omitempty" json:"dhcp-pools,omitempty"`

	Web WebConfig `yaml:"web,omitempty" json:"web,omitempty"`
}

// Interface is the configuration of one network interface. Physical
// interfaces (wm0, re0, ath0 ...) and cloned interfaces (vlanN, bridgeN,
// wlanN) share this structure.
type Interface struct {
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
	Shutdown    bool   `yaml:"shutdown,omitempty" json:"shutdown,omitempty"`
	MTU         int    `yaml:"mtu,omitempty" json:"mtu,omitempty"`
	// Media is passed to "ifconfig <if> media", e.g. "1000baseT".
	Media string `yaml:"media,omitempty" json:"media,omitempty"`

	// IPv4 holds addresses in CIDR notation; the first is the primary.
	IPv4 []string `yaml:"ipv4,omitempty" json:"ipv4,omitempty"`
	// DHCP obtains an IPv4 address with dhcpcd(8).
	DHCP bool `yaml:"dhcp,omitempty" json:"dhcp,omitempty"`
	// IPv6 holds addresses in CIDR notation.
	IPv6 []string `yaml:"ipv6,omitempty" json:"ipv6,omitempty"`

	AccessGroupIn  string `yaml:"access-group-in,omitempty" json:"access-group-in,omitempty"`
	AccessGroupOut string `yaml:"access-group-out,omitempty" json:"access-group-out,omitempty"`

	VLAN     *VLAN     `yaml:"vlan,omitempty" json:"vlan,omitempty"`
	Bridge   *Bridge   `yaml:"bridge,omitempty" json:"bridge,omitempty"`
	Wireless *Wireless `yaml:"wireless,omitempty" json:"wireless,omitempty"`
}

// VLAN configures an 802.1Q interface (vlanN).
type VLAN struct {
	ID     int    `yaml:"id" json:"id"`
	Parent string `yaml:"parent" json:"parent"`
}

// Bridge configures a bridge interface (bridgeN).
type Bridge struct {
	Members []string `yaml:"members,omitempty" json:"members,omitempty"`
}

// Wireless modes and security settings.
const (
	ModeStation = "station"
	ModeHostAP  = "hostap"

	SecurityOpen    = "open"
	SecurityWPA2PSK = "wpa2-psk"
)

// Wireless configures an 802.11 interface.
//
// Two NetBSD wifi stacks exist. With the legacy stack (NetBSD 11 and
// earlier) the physical interface (ath0, athn0 ...) is configured directly
// and Device is empty. With the new VAP-based stack (NetBSD 12) the
// interface is a cloned wlanN and Device names the radio (e.g. "ath0").
type Wireless struct {
	Mode     string `yaml:"mode" json:"mode"`
	Device   string `yaml:"device,omitempty" json:"device,omitempty"`
	SSID     string `yaml:"ssid" json:"ssid"`
	Security string `yaml:"security,omitempty" json:"security,omitempty"`
	PSK      string `yaml:"psk,omitempty" json:"psk,omitempty"`
	Channel  int    `yaml:"channel,omitempty" json:"channel,omitempty"`
	HWMode   string `yaml:"hw-mode,omitempty" json:"hw-mode,omitempty"`
	Country  string `yaml:"country,omitempty" json:"country,omitempty"`
	Hidden   bool   `yaml:"hidden,omitempty" json:"hidden,omitempty"`
}

// Route is a static route. NextHop is an IP address or "null0" for a
// blackhole route.
type Route struct {
	Prefix  string `yaml:"prefix" json:"prefix"`
	NextHop string `yaml:"next-hop" json:"next-hop"`
}

// AccessList is an ordered list of entries, evaluated first match wins.
// Traffic not matched by any entry is denied (implicit deny), as on IOS.
type AccessList struct {
	Entries []ACE `yaml:"entries,omitempty" json:"entries,omitempty"`
}

// ACE is a single access-list entry.
type ACE struct {
	Seq      int    `yaml:"seq" json:"seq"`
	Action   string `yaml:"action" json:"action"`     // permit | deny
	Protocol string `yaml:"protocol" json:"protocol"` // ip | tcp | udp | icmp | icmpv6 | <number>
	Source   string `yaml:"source" json:"source"`     // any | prefix
	// SourcePort / DestinationPort: "22", "1024-65535".
	SourcePort      string `yaml:"source-port,omitempty" json:"source-port,omitempty"`
	Destination     string `yaml:"destination" json:"destination"`
	DestinationPort string `yaml:"destination-port,omitempty" json:"destination-port,omitempty"`
	ICMPType        *int   `yaml:"icmp-type,omitempty" json:"icmp-type,omitempty"`
	Log             bool   `yaml:"log,omitempty" json:"log,omitempty"`
	// Stateless disables connection tracking for a permit entry. By
	// default permitted traffic is tracked so replies are allowed.
	Stateless bool `yaml:"stateless,omitempty" json:"stateless,omitempty"`
}

// NAT rule types.
const (
	NATOverload = "overload" // source NAT / masquerade
	NATStatic   = "static"   // port forward
)

// NATRule describes source NAT (overload) or a static port forward.
type NATRule struct {
	Type      string `yaml:"type" json:"type"`
	Interface string `yaml:"interface" json:"interface"` // outside interface
	// Overload: inside network to translate.
	Source string `yaml:"source,omitempty" json:"source,omitempty"`
	// Static: protocol, inside address/port and public port.
	Protocol   string `yaml:"protocol,omitempty" json:"protocol,omitempty"`
	LocalAddr  string `yaml:"local-address,omitempty" json:"local-address,omitempty"`
	LocalPort  int    `yaml:"local-port,omitempty" json:"local-port,omitempty"`
	PublicPort int    `yaml:"public-port,omitempty" json:"public-port,omitempty"`
}

// DHCPPool is a DHCP server scope served by dhcpd(8).
type DHCPPool struct {
	Network       string     `yaml:"network" json:"network"`
	RangeStart    string     `yaml:"range-start,omitempty" json:"range-start,omitempty"`
	RangeEnd      string     `yaml:"range-end,omitempty" json:"range-end,omitempty"`
	DefaultRouter []string   `yaml:"default-router,omitempty" json:"default-router,omitempty"`
	DNSServers    []string   `yaml:"dns-servers,omitempty" json:"dns-servers,omitempty"`
	DomainName    string     `yaml:"domain-name,omitempty" json:"domain-name,omitempty"`
	LeaseSeconds  int        `yaml:"lease-seconds,omitempty" json:"lease-seconds,omitempty"`
	Hosts         []DHCPHost `yaml:"hosts,omitempty" json:"hosts,omitempty"`
}

// DHCPHost is a static DHCP reservation.
type DHCPHost struct {
	Name    string `yaml:"name" json:"name"`
	MAC     string `yaml:"mac" json:"mac"`
	Address string `yaml:"address" json:"address"`
}

// WebConfig configures the built-in web service.
type WebConfig struct {
	Enabled bool `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	TLS     bool `yaml:"tls,omitempty" json:"tls,omitempty"`
	// Port defaults to 443 with TLS and 80 without.
	Port int `yaml:"port,omitempty" json:"port,omitempty"`
	// Bind restricts the listener to one address, e.g. the LAN address.
	Bind string `yaml:"bind,omitempty" json:"bind,omitempty"`
}

// New returns an empty configuration with sensible defaults.
func New() *Config {
	c := &Config{Hostname: "router"}
	c.Normalize()
	return c
}

// Normalize initialises nil maps and sorts ordered data so that two
// equivalent configurations marshal identically.
func (c *Config) Normalize() {
	if c.Interfaces == nil {
		c.Interfaces = map[string]*Interface{}
	}
	if c.AccessLists == nil {
		c.AccessLists = map[string]*AccessList{}
	}
	if c.DHCPPools == nil {
		c.DHCPPools = map[string]*DHCPPool{}
	}
	if c.Users == nil {
		c.Users = map[string]string{}
	}
	for name, ifc := range c.Interfaces {
		if ifc == nil {
			c.Interfaces[name] = &Interface{}
		}
	}
	for name, acl := range c.AccessLists {
		if acl == nil {
			acl = &AccessList{}
			c.AccessLists[name] = acl
		}
		sort.SliceStable(acl.Entries, func(i, j int) bool { return acl.Entries[i].Seq < acl.Entries[j].Seq })
	}
	for name, p := range c.DHCPPools {
		if p == nil {
			c.DHCPPools[name] = &DHCPPool{}
		}
	}
}

// Interface returns the named interface, creating it if needed.
func (c *Config) Interface(name string) *Interface {
	c.Normalize()
	ifc, ok := c.Interfaces[name]
	if !ok {
		ifc = &Interface{}
		c.Interfaces[name] = ifc
	}
	return ifc
}

// InterfaceNames returns interface names in a stable, human friendly order.
func (c *Config) InterfaceNames() []string {
	names := make([]string, 0, len(c.Interfaces))
	for n := range c.Interfaces {
		names = append(names, n)
	}
	SortInterfaceNames(names)
	return names
}

// Clone returns a deep copy of the configuration.
func (c *Config) Clone() *Config {
	data, err := Marshal(c)
	if err != nil {
		panic(fmt.Sprintf("config clone marshal: %v", err))
	}
	n, err := Unmarshal(data)
	if err != nil {
		panic(fmt.Sprintf("config clone unmarshal: %v", err))
	}
	return n
}

// Equal reports whether two configurations are equivalent.
func Equal(a, b *Config) bool {
	ad, err1 := Marshal(a)
	bd, err2 := Marshal(b)
	return err1 == nil && err2 == nil && bytes.Equal(ad, bd)
}

// Marshal encodes the configuration as YAML.
func Marshal(c *Config) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("# netcli configuration\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Unmarshal decodes YAML, rejecting unknown keys so typos are caught.
func Unmarshal(data []byte) (*Config, error) {
	c := &Config{}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(c); err != nil {
		if errors.Is(err, io.EOF) {
			return New(), nil
		}
		return nil, err
	}
	c.Normalize()
	return c, nil
}
