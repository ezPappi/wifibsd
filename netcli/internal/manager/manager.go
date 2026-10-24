// Package manager ties the configuration store to the applier. Every change,
// whether it comes from the CLI, the web service or the revert timer, goes
// through Change or Replace: lock, load the current running configuration,
// modify, validate, apply the difference and record the new running state.
package manager

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/netcli/netcli/internal/apply"
	"github.com/netcli/netcli/internal/config"
	"github.com/netcli/netcli/internal/system"
)

// Manager coordinates configuration changes.
type Manager struct {
	Store   *config.Store
	Applier *apply.Applier
	Paths   system.Paths
}

// New creates a manager for the given paths and executor.
func New(paths system.Paths, exec system.Executor) *Manager {
	return &Manager{
		Store:   config.NewStore(paths.EtcDir, paths.RunDir),
		Applier: &apply.Applier{Exec: exec, Paths: paths},
		Paths:   paths,
	}
}

// ErrNoChange is returned by a mutate function to skip applying.
var ErrNoChange = errors.New("no change")

// Running returns the current running configuration.
func (m *Manager) Running() (*config.Config, error) {
	c, _, err := m.Store.Running()
	return c, err
}

// Startup returns the startup configuration.
func (m *Manager) Startup() (*config.Config, error) {
	c, _, err := m.Store.Startup()
	return c, err
}

// Change modifies the running configuration and applies the result.
func (m *Manager) Change(mutate func(c *config.Config) error) (*config.Config, error) {
	unlock, err := m.Store.Lock()
	if err != nil {
		return nil, err
	}
	defer unlock()

	old, _, err := m.Store.Running()
	if err != nil {
		return nil, err
	}
	next := old.Clone()
	if err := mutate(next); err != nil {
		if errors.Is(err, ErrNoChange) {
			return old, nil
		}
		return old, err
	}
	return m.commit(old, next)
}

// Replace applies a complete configuration.
func (m *Manager) Replace(next *config.Config) (*config.Config, error) {
	unlock, err := m.Store.Lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	old, _, err := m.Store.Running()
	if err != nil {
		return nil, err
	}
	return m.commit(old, next.Clone())
}

func (m *Manager) commit(old, next *config.Config) (*config.Config, error) {
	next.Normalize()
	if err := next.Validate(); err != nil {
		return old, err
	}
	if config.Equal(old, next) {
		return old, nil
	}
	if err := m.Applier.Apply(old, next, false); err != nil {
		// Roll back to the previous state so the system and the recorded
		// running configuration stay in agreement.
		if rerr := m.Applier.Apply(next, old, false); rerr != nil {
			m.Store.SaveRunning(next)
			return next, fmt.Errorf("%w\nrollback also failed (running config reflects the attempted change):\n%v", err, rerr)
		}
		return old, fmt.Errorf("%w\nchange rolled back", err)
	}
	if err := m.Store.SaveRunning(next); err != nil {
		return next, err
	}
	return next, nil
}

// Boot applies the startup configuration from scratch.
func (m *Manager) Boot() error {
	unlock, err := m.Store.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	c, ok, err := m.Store.Startup()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no startup configuration at %s", m.Store.StartupPath)
	}
	if err := c.Validate(); err != nil {
		return fmt.Errorf("startup configuration is invalid:\n%w", err)
	}
	aerr := m.Applier.Apply(nil, c, true)
	if err := m.Store.SaveRunning(c); err != nil {
		return err
	}
	return aerr
}

// Save copies the running configuration to the startup configuration.
func (m *Manager) Save() error {
	unlock, err := m.Store.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	c, _, err := m.Store.Running()
	if err != nil {
		return err
	}
	return m.Store.SaveStartup(c)
}

// Erase removes the startup configuration.
func (m *Manager) Erase() error {
	unlock, err := m.Store.Lock()
	if err != nil {
		return err
	}
	defer unlock()
	err = os.Remove(m.Store.StartupPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ---- revert timer ------------------------------------------------------------
//
// "configure terminal revert timer N" snapshots the running configuration
// and starts a detached watchdog process. Unless "configure confirm" is
// issued in time, the watchdog restores the snapshot. Because it runs as a
// separate process it still fires when the SSH session that made a bad
// firewall change has been cut off.

type revertState struct {
	Deadline time.Time `json:"deadline"`
	PID      int       `json:"pid"`
}

func (m *Manager) revertSnapshot() string { return m.Paths.File("revert.yaml") }
func (m *Manager) revertState() string    { return m.Paths.File("revert.json") }

// RevertPending returns the deadline of an armed revert timer.
func (m *Manager) RevertPending() (time.Time, bool) {
	data, err := os.ReadFile(m.revertState())
	if err != nil {
		return time.Time{}, false
	}
	var st revertState
	if json.Unmarshal(data, &st) != nil {
		return time.Time{}, false
	}
	if st.PID > 0 && syscall.Kill(st.PID, 0) != nil {
		m.clearRevert()
		return time.Time{}, false
	}
	return st.Deadline, true
}

// ArmRevert snapshots the running configuration and starts the watchdog.
// If a timer is already armed its snapshot is kept and only the deadline
// is extended.
func (m *Manager) ArmRevert(d time.Duration, extraArgs []string) (time.Time, error) {
	if _, ok := m.RevertPending(); ok {
		m.stopWatchdog()
	} else {
		c, err := m.Running()
		if err != nil {
			return time.Time{}, err
		}
		if err := config.SaveFile(m.revertSnapshot(), c); err != nil {
			return time.Time{}, err
		}
	}
	deadline := time.Now().Add(d).Truncate(time.Second)
	self, err := os.Executable()
	if err != nil {
		return time.Time{}, err
	}
	args := append([]string{}, extraArgs...)
	args = append(args, "revert-timer", strconv.FormatInt(deadline.Unix(), 10))
	cmd := exec.Command(self, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	devnull, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devnull, devnull, devnull
	if err := cmd.Start(); err != nil {
		return time.Time{}, err
	}
	st := revertState{Deadline: deadline, PID: cmd.Process.Pid}
	data, _ := json.Marshal(st)
	if err := config.WriteFileAtomic(m.revertState(), data, 0o600); err != nil {
		cmd.Process.Kill()
		return time.Time{}, err
	}
	cmd.Process.Release()
	return deadline, nil
}

func (m *Manager) stopWatchdog() {
	data, err := os.ReadFile(m.revertState())
	if err != nil {
		return
	}
	var st revertState
	if json.Unmarshal(data, &st) == nil && st.PID > 1 && st.PID != os.Getpid() {
		syscall.Kill(st.PID, syscall.SIGTERM)
	}
}

func (m *Manager) clearRevert() {
	os.Remove(m.revertState())
	os.Remove(m.revertSnapshot())
}

// ConfirmRevert cancels an armed revert timer, keeping the changes.
func (m *Manager) ConfirmRevert() bool {
	if _, ok := m.RevertPending(); !ok {
		return false
	}
	m.stopWatchdog()
	m.clearRevert()
	return true
}

// RevertNow restores the snapshot immediately.
func (m *Manager) RevertNow() error {
	c, ok, err := config.LoadFile(m.revertSnapshot())
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no revert snapshot")
	}
	m.stopWatchdog()
	_, err = m.Replace(c)
	if err == nil {
		m.clearRevert()
	}
	return err
}

// RunRevertWatchdog is the body of the detached watchdog process.
func (m *Manager) RunRevertWatchdog(deadline time.Time) error {
	time.Sleep(time.Until(deadline))
	data, err := os.ReadFile(m.revertState())
	if err != nil {
		return nil // confirmed meanwhile
	}
	var st revertState
	if json.Unmarshal(data, &st) == nil && st.PID != os.Getpid() {
		return nil // superseded by a newer timer
	}
	c, ok, err := config.LoadFile(m.revertSnapshot())
	if err != nil || !ok {
		return fmt.Errorf("revert snapshot unavailable: %v", err)
	}
	_, err = m.Replace(c)
	m.clearRevert()
	return err
}

// Diff returns a simple line diff between two texts ("+" added, "-" removed).
func Diff(a, b string) string {
	al := strings.Split(strings.TrimRight(a, "\n"), "\n")
	bl := strings.Split(strings.TrimRight(b, "\n"), "\n")
	// Longest common subsequence table.
	n, mm := len(al), len(bl)
	t := make([][]int, n+1)
	for i := range t {
		t[i] = make([]int, mm+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := mm - 1; j >= 0; j-- {
			if al[i] == bl[j] {
				t[i][j] = t[i+1][j+1] + 1
			} else if t[i+1][j] >= t[i][j+1] {
				t[i][j] = t[i+1][j]
			} else {
				t[i][j] = t[i][j+1]
			}
		}
	}
	// Changed lines inside a section (indented) are preceded by the
	// section header, e.g. "interface wm1", so the diff reads on its own.
	var out strings.Builder
	header, printed := "", ""
	emit := func(sign, line string) {
		if !strings.HasPrefix(line, " ") {
			header = line
		} else if header != "" && header != printed {
			out.WriteString("  " + header + "\n")
			printed = header
		}
		out.WriteString(sign + " " + line + "\n")
	}
	i, j := 0, 0
	for i < n && j < mm {
		switch {
		case al[i] == bl[j]:
			if !strings.HasPrefix(al[i], " ") {
				header = al[i]
			}
			i++
			j++
		case t[i+1][j] >= t[i][j+1]:
			emit("-", al[i])
			i++
		default:
			emit("+", bl[j])
			j++
		}
	}
	for ; i < n; i++ {
		emit("-", al[i])
	}
	for ; j < mm; j++ {
		emit("+", bl[j])
	}
	return out.String()
}
