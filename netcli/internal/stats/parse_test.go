package stats

import "testing"

const netstatBytes = `Name  Mtu   Network       Address              Ibytes     Obytes
wm0   1500  <Link>        00:0d:b9:41:2c:a0  123456789   98765432
wm0   1500  192.168.1/24  192.168.1.1        123456789   98765432
wm1*  1500  <Link#2>      00:0d:b9:41:2c:a1          0          0
lo0   33624 <Link>                                 4321       4321
`

const netstatPkts = `Name  Mtu   Network       Address              Ipkts Ierrs    Opkts Oerrs Colls
wm0   1500  <Link>        00:0d:b9:41:2c:a0   1000     2     2000     3     0
lo0   33624 <Link>                              10     0       10     0     0
`

func TestParseNetstat(t *testing.T) {
	m := map[string]IfCounters{}
	parseNetstatBytes(netstatBytes, m)
	parseNetstatPackets(netstatPkts, m)
	if c := m["wm0"]; c.RxBytes != 123456789 || c.TxBytes != 98765432 || c.RxPkts != 1000 || c.RxErrs != 2 || c.TxPkts != 2000 || c.TxErrs != 3 {
		t.Errorf("wm0 = %+v", c)
	}
	if _, ok := m["wm1"]; !ok {
		t.Error("down interface (wm1*) missing")
	}
	if c := m["lo0"]; c.RxBytes != 4321 || c.TxPkts != 10 {
		t.Errorf("lo0 (no address column) = %+v", c)
	}
}

func TestParseCPTime(t *testing.T) {
	total, idle := parseCPTime("user = 100, nice = 0, sys = 50, intr = 10, idle = 840\n")
	if total != 1000 || idle != 840 {
		t.Errorf("total=%d idle=%d", total, idle)
	}
}
