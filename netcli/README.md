# netcli

A Cisco IOS style command line and web interface for NetBSD routers.
It configures wired and wireless interfaces, VLANs, bridges, static
routes, the NPF firewall, NAT and a DHCP server, and stores everything in
one YAML file.

```
gw# configure terminal
gw(config)# interface wm1
gw(config-if)# ip address 192.168.1.1 255.255.255.0
gw(config-if)# no shutdown
gw(config-if)# end
gw# write memory
Building configuration...
[OK]
```

* One static Go binary (amd64, i386 incl. AMD Geode, arm64, armv6), no cgo,
  no runtime dependencies beyond the NetBSD base system.
* Running vs startup configuration, as on IOS: commands take effect
  immediately; `write memory` makes them permanent.
* Changes are applied as a minimal diff: changing one address runs two
  `ifconfig` commands, not a full network restart.
* Every change is validated before it is applied. If applying fails, the
  previous state is restored.
* `configure terminal revert timer N` arms an automatic rollback that runs
  in a separate process, so a firewall rule that cuts off your SSH session
  is undone after N minutes unless you `configure confirm`.
* Web service with live graphs (CPU, memory, per-interface traffic), an
  interface overview, a YAML editor with validation and diff, and a web
  terminal running the same CLI.

## Layout

| Path | Purpose |
|---|---|
| `/etc/netcli/startup.yaml` | startup configuration (`write memory`) |
| `/etc/netcli/web.crt`, `web.key` | TLS certificate, self-signed on first start |
| `/var/run/netcli/running.yaml` | running configuration |
| `/var/run/netcli/npf.conf` | generated firewall rules |
| `/var/run/netcli/hostapd.<if>.conf`, `wpa_supplicant.<if>.conf`, `dhcpd.conf` | generated service configs |

Generated files live in `/var/run`, so normal operation does not write to
flash. Only `write memory` touches `/etc`.

## Building

```sh
make test          # vet (also for GOOS=netbsd) and unit tests
make netbsd        # binaries in dist/
```

Requires Go 1.24 or newer. The i386 build uses `GO386=softfloat`, which is
needed for CPUs without SSE2 such as the AMD Geode LX in a Soekris net5501.

## Installing on NetBSD

```sh
install -m 755 netcli-netbsd-amd64 /usr/local/sbin/netcli
install -m 755 contrib/rc.d/netcli contrib/rc.d/netclid /etc/rc.d/
mkdir -p /etc/netcli
cp examples/startup.yaml /etc/netcli/   # optional starting point
```

In `/etc/rc.conf` enable netcli and disable what it takes over:

```sh
netcli=YES
netclid=YES     # web service
npf=NO          # netcli loads the NPF rules itself
dhcpcd=NO       # netcli starts dhcpcd per interface ("ip address dhcp")
# remove ifconfig_*, defaultroute, dhcpd, hostapd and wpa_supplicant entries
```

Then run `netcli` as root. A web user is needed before you can log in:

```
gw(config)# username admin secret <password>
gw(config)# ip http secure-server
gw(config)# ip http bind 192.168.1.1
```

## Trying it without a router

On anything other than NetBSD netcli starts in dry-run mode: system commands
are printed instead of executed, and generated files are written to a
sandbox directory.

```sh
go build ./cmd/netcli
./netcli -etc /tmp/nc/etc -run /tmp/nc/run shell
./netcli -etc /tmp/nc/etc -run /tmp/nc/run serve -listen 127.0.0.1:8080
```

## Command reference

Abbreviations work everywhere (`sh ip int br`, `conf t`), `?` lists what
can follow, Tab completes, Ctrl-Z returns to privileged mode and Ctrl-C
clears the line. Global configuration commands are accepted inside
sub-modes and return to global configuration mode, as on IOS. Prefix a
configuration command with `no` to remove it.

### EXEC

```
enable / disable / exit
configure terminal [revert timer <1-120>]
configure confirm | configure revert now
show running-config [interface IF | yaml]
show startup-config
show archive config differences
show interfaces [IF] | show ip interface brief
show ip route | show ipv6 route
show access-lists [NAME] | show ip nat rules
show firewall [config | statistics]
show wireless [IF]
show ip dhcp pool | show ip dhcp binding
show rollback | show version | show clock | show history
write [memory | terminal | erase]
copy running-config startup-config | copy startup-config running-config
ping HOST [count N] | traceroute HOST
debug apply | undebug all          (print every system command executed)
reload
```

### Global configuration

```
hostname NAME
ip domain-name NAME
ip name-server ADDR...
ip routing | ipv6 unicast-routing
ip route A.B.C.D MASK (NEXTHOP | Null0)      also A.B.C.D/nn
ipv6 route X:X::X/nn (NEXTHOP | Null0)
ip access-list extended NAME
ip nat inside source network A.B.C.D MASK interface IF overload
ip nat inside source static (tcp|udp) LOCAL-ADDR LOCAL-PORT interface IF PUBLIC-PORT
ip dhcp pool NAME
ip http (server | secure-server | port N | bind ADDR)
interface IF
enable secret [0|8] SECRET
username NAME secret [0|8] SECRET
banner motd TEXT
```

### Interface

```
description TEXT
ip address A.B.C.D MASK [secondary] | ip address A.B.C.D/nn [secondary] | ip address dhcp
ipv6 address X:X::X/nn
ip access-group ACL (in | out)
mtu N | media TEXT | shutdown
encapsulation dot1q VID interface PARENT     (on vlanN)
bridge member IF                             (on bridgeN)
wireless mode (station | hostap)
wireless device RADIO                        (wlanN, NetBSD 12 wifi stack)
wireless ssid "Name with spaces"
wireless security (open | wpa2-psk)
wireless psk "passphrase"
wireless channel N | wireless hw-mode (a|b|g) | wireless country CC | wireless hidden-ssid
```

### Access lists

```
[SEQ] (permit | deny) PROTO SRC [PORTS] DST [PORTS] [ICMP-TYPE] [log] [stateless]
  PROTO   ip | tcp | udp | icmp | icmpv6 | 0-255
  SRC/DST any | host A.B.C.D | A.B.C.D WILDCARD | A.B.C.D/nn | X:X::X/nn
  PORTS   eq P | range P1 P2 | gt P | lt P      (numbers or names: www, ssh, domain ...)
no SEQ | no permit ... | resequence
```

Entries are evaluated in sequence order, first match wins, with an implicit
deny at the end, as on IOS. Unlike IOS, permitted traffic is tracked
statefully by default, so replies are allowed without extra rules; add
`stateless` to turn that off for an entry. On interfaces with an inbound
access list netcli also lets in IPv6 neighbour discovery and, for DHCP
client interfaces, DHCP replies.

### DHCP pool

```
network A.B.C.D MASK | network A.B.C.D/nn
range FIRST LAST
default-router ADDR...
dns-server ADDR...
domain-name NAME
lease DAYS [HOURS [MINUTES]]
host NAME MAC ADDR
```

## How configuration maps to NetBSD

| netcli | NetBSD |
|---|---|
| interface addresses, MTU, media, shutdown | `ifconfig` |
| `ip address dhcp` | `dhcpcd -b IF` |
| VLANs | `ifconfig vlanN create; ifconfig vlanN vlan ID vlanif PARENT` |
| bridges | `ifconfig bridgeN create; brconfig bridgeN add IF` |
| wireless station | `wpa_supplicant -B -i IF -c ...` |
| wireless access point | `ifconfig IF mediaopt hostap`, `hostapd -B ...` (driver=bsd) |
| wlanN (NetBSD 12) | `ifconfig wlanN create wlandev RADIO wlanmode sta/hostap` |
| static routes | `route add` |
| access lists, NAT | generated `npf.conf`, `npfctl validate` then `npfctl reload` |
| DHCP pools | generated `dhcpd.conf`, `dhcpd` on the interfaces in the pool networks |
| statistics | `netstat -ibn`, `netstat -in`, `sysctl kern.cp_time`, `vmstat -s` |

NAT uses `ifaddrs(IF)` for the outside address, which tracks address
changes, so a WAN address learned by DHCP later works without a reload.

## Status and things to verify on real hardware

netcli has been developed and unit tested on Linux in dry-run mode and
cross-compiled for NetBSD; it has not yet been run on a NetBSD router. The
generated commands and files follow the NetBSD manual pages, but these
points deserve a check on the first real install:

* `npfctl validate` accepting the generated `npf.conf` (in particular
  `icmp-type ... all` rules and `apply "log"`), and NPF's handling of
  inbound port forwards against an inbound access list (whether rules see
  the public or the translated address).
* `hostapd` with `driver=bsd` on your Atheros card, and the legacy
  `mediaopt hostap` sequence.
* `dhcpd`: this uses ISC dhcpd from the base system; if your NetBSD
  version no longer ships it, install `net/isc-dhcpd` or adjust the
  command in `internal/apply`.
* The counters parsed from `netstat`, `sysctl kern.cp_time` and `vmstat -s`.

Use `debug apply` in the CLI to see exactly what is executed, and arm a
rollback timer (`configure terminal revert timer 5`) while experimenting
over the network.

## Code layout

```
cmd/netcli          main: shell, exec, apply, serve, revert-timer
internal/config     YAML model, validation, store with file locking, password hashing
internal/render     npf.conf, hostapd.conf, wpa_supplicant.conf, dhcpd.conf
internal/apply      diff based applier
internal/manager    lock → load → change → validate → apply → save; rollback timer
internal/cli        IOS style parser, command trees, show commands, terminal
internal/stats      counter sampling and history for the graphs
internal/web        JSON API, server-sent events, embedded UI
contrib/rc.d        NetBSD rc.d scripts
```
