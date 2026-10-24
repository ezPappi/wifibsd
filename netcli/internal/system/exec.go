// Package system wraps everything that touches the host: running
// commands, writing generated files and signalling daemons. A dry-run
// executor lets netcli be developed and tested on machines that are not
// NetBSD routers.
package system

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Paths holds the locations netcli reads and writes.
type Paths struct {
	EtcDir     string // startup config, TLS material
	RunDir     string // running config, generated files, pid files
	DHCPLeases string // dhcpd lease database
}

// DefaultPaths returns the standard NetBSD locations.
func DefaultPaths() Paths {
	return Paths{
		EtcDir:     "/etc/netcli",
		RunDir:     "/var/run/netcli",
		DHCPLeases: "/var/db/dhcpd.leases",
	}
}

// File returns a path inside the run directory.
func (p Paths) File(name string) string { return filepath.Join(p.RunDir, name) }

// Executor performs host side effects.
type Executor interface {
	// Run executes a command and returns its combined output.
	Run(name string, args ...string) (string, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
	ReadFile(path string) ([]byte, error)
	Remove(path string) error
	// StopPidfile sends SIGTERM to the process recorded in a pid file and
	// waits briefly for it to exit.
	StopPidfile(path string) error
	DryRun() bool
}

// Tracer receives a human readable line for every side effect.
type Tracer func(line string)

// Quote renders a command line for display.
func Quote(name string, args ...string) string {
	parts := []string{name}
	for _, a := range args {
		if a == "" || strings.ContainsAny(a, " \t\"'$\\") {
			a = strconv.Quote(a)
		}
		parts = append(parts, a)
	}
	return strings.Join(parts, " ")
}

// Real executes commands on the host.
type Real struct {
	mu    sync.Mutex
	Trace Tracer
}

func (r *Real) trace(s string) {
	r.mu.Lock()
	t := r.Trace
	r.mu.Unlock()
	if t != nil {
		t(s)
	}
}

// SetTrace replaces the tracer.
func (r *Real) SetTrace(t Tracer) { r.mu.Lock(); r.Trace = t; r.mu.Unlock() }

func (r *Real) Run(name string, args ...string) (string, error) {
	r.trace(Quote(name, args...))
	cmd := exec.Command(name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Stdin = nil
	err := cmd.Run()
	s := strings.TrimSpace(out.String())
	if err != nil {
		if s != "" {
			return s, fmt.Errorf("%s: %s", Quote(name, args...), s)
		}
		return s, fmt.Errorf("%s: %w", Quote(name, args...), err)
	}
	return s, nil
}

func (r *Real) WriteFile(path string, data []byte, perm os.FileMode) error {
	r.trace("write " + path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".new"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (r *Real) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (r *Real) Remove(path string) error {
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (r *Real) StopPidfile(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 1 {
		os.Remove(path)
		return nil
	}
	r.trace(fmt.Sprintf("kill %d (%s)", pid, filepath.Base(path)))
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		os.Remove(path)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return err
	}
	for i := 0; i < 30; i++ {
		if syscall.Kill(pid, 0) != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	os.Remove(path)
	return nil
}

func (r *Real) DryRun() bool { return false }

// Dry logs commands instead of running them. Files are written below
// Sandbox so the generated configuration can be inspected.
type Dry struct {
	mu      sync.Mutex
	Sandbox string
	Trace   Tracer
	Log     []string
}

func (d *Dry) record(s string) {
	d.mu.Lock()
	d.Log = append(d.Log, s)
	t := d.Trace
	d.mu.Unlock()
	if t != nil {
		t("[dry-run] " + s)
	}
}

// SetTrace replaces the tracer.
func (d *Dry) SetTrace(t Tracer) { d.mu.Lock(); d.Trace = t; d.mu.Unlock() }

// TakeLog returns and clears the recorded actions.
func (d *Dry) TakeLog() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	l := d.Log
	d.Log = nil
	return l
}

func (d *Dry) path(p string) string { return filepath.Join(d.Sandbox, p) }

func (d *Dry) Run(name string, args ...string) (string, error) {
	d.record(Quote(name, args...))
	return "", nil
}

func (d *Dry) WriteFile(path string, data []byte, perm os.FileMode) error {
	d.record("write " + path)
	p := d.path(path)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, data, perm)
}

func (d *Dry) ReadFile(path string) ([]byte, error) { return os.ReadFile(d.path(path)) }

func (d *Dry) Remove(path string) error {
	d.record("rm " + path)
	err := os.Remove(d.path(path))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (d *Dry) StopPidfile(path string) error {
	d.record("stop process from " + path)
	return nil
}

func (d *Dry) DryRun() bool { return true }

// IsNetBSD reports whether we run on NetBSD.
func IsNetBSD() bool { return runtime.GOOS == "netbsd" }

// Output runs a read-only command for display purposes ("show" commands).
func Output(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}
