// Package stats samples interface counters, CPU and memory and keeps a
// short history for the live graphs in the web interface.
package stats

import (
	"sort"
	"sync"
	"time"
)

// IfCounters are cumulative interface counters.
type IfCounters struct {
	RxBytes, TxBytes uint64
	RxPkts, TxPkts   uint64
	RxErrs, TxErrs   uint64
}

// Sample is one raw reading of the system counters.
type Sample struct {
	Time     time.Time
	CPUTotal uint64 // sum of all CPU tick counters
	CPUIdle  uint64
	MemTotal uint64 // bytes
	MemFree  uint64 // bytes
	Ifaces   map[string]IfCounters
}

// Rate is the per-second rate of an interface between two samples.
type Rate struct {
	RxBps float64 `json:"rx_bps"`
	TxBps float64 `json:"tx_bps"`
	RxPps float64 `json:"rx_pps"`
	TxPps float64 `json:"tx_pps"`
}

// Point is one entry in the history.
type Point struct {
	T      int64           `json:"t"`   // unix milliseconds
	CPU    float64         `json:"cpu"` // percent busy
	Mem    float64         `json:"mem"` // percent used
	Ifaces map[string]Rate `json:"ifaces"`
}

// Collector samples periodically and keeps a ring buffer of points.
type Collector struct {
	Interval time.Duration
	Size     int
	read     func() (*Sample, error)

	mu      sync.Mutex
	last    *Sample
	history []Point
	subs    map[chan Point]struct{}
	lastErr error
}

// NewCollector creates a collector using the platform sampler.
func NewCollector(interval time.Duration, size int) *Collector {
	return &Collector{Interval: interval, Size: size, read: Read, subs: map[chan Point]struct{}{}}
}

// Run samples until stop is closed.
func (c *Collector) Run(stop <-chan struct{}) {
	t := time.NewTicker(c.Interval)
	defer t.Stop()
	c.tick()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			c.tick()
		}
	}
}

func (c *Collector) tick() {
	s, err := c.read()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastErr = err
	if err != nil {
		return
	}
	prev := c.last
	c.last = s
	if prev == nil {
		return
	}
	p := Delta(prev, s)
	c.history = append(c.history, p)
	if len(c.history) > c.Size {
		c.history = c.history[len(c.history)-c.Size:]
	}
	for ch := range c.subs {
		select {
		case ch <- p:
		default: // slow consumer: drop the point
		}
	}
}

// Delta computes rates between two samples.
func Delta(a, b *Sample) Point {
	dt := b.Time.Sub(a.Time).Seconds()
	if dt <= 0 {
		dt = 1
	}
	p := Point{T: b.Time.UnixMilli(), Ifaces: map[string]Rate{}}
	if tot := sub(b.CPUTotal, a.CPUTotal); tot > 0 {
		idle := sub(b.CPUIdle, a.CPUIdle)
		p.CPU = round1(100 * float64(tot-min(idle, tot)) / float64(tot))
	}
	if b.MemTotal > 0 {
		p.Mem = round1(100 * float64(b.MemTotal-min(b.MemFree, b.MemTotal)) / float64(b.MemTotal))
	}
	for name, nc := range b.Ifaces {
		oc, ok := a.Ifaces[name]
		if !ok {
			continue
		}
		p.Ifaces[name] = Rate{
			RxBps: round1(8 * float64(sub(nc.RxBytes, oc.RxBytes)) / dt),
			TxBps: round1(8 * float64(sub(nc.TxBytes, oc.TxBytes)) / dt),
			RxPps: round1(float64(sub(nc.RxPkts, oc.RxPkts)) / dt),
			TxPps: round1(float64(sub(nc.TxPkts, oc.TxPkts)) / dt),
		}
	}
	return p
}

// sub handles counter resets (e.g. an interface recreated) gracefully.
func sub(a, b uint64) uint64 {
	if a < b {
		return 0
	}
	return a - b
}

func round1(f float64) float64 { return float64(int64(f*10+0.5)) / 10 }

// History returns a copy of the recorded points.
func (c *Collector) History() []Point {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Point(nil), c.history...)
}

// Latest returns the most recent raw sample.
func (c *Collector) Latest() (*Sample, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last, c.lastErr
}

// Subscribe returns a channel receiving new points; call the returned
// function to unsubscribe.
func (c *Collector) Subscribe() (<-chan Point, func()) {
	ch := make(chan Point, 16)
	c.mu.Lock()
	c.subs[ch] = struct{}{}
	c.mu.Unlock()
	return ch, func() {
		c.mu.Lock()
		delete(c.subs, ch)
		c.mu.Unlock()
	}
}

// Names returns the interface names in a sample, sorted.
func (s *Sample) Names() []string {
	n := make([]string, 0, len(s.Ifaces))
	for k := range s.Ifaces {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}
