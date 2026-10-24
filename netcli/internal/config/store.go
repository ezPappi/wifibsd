package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// Store gives access to the startup configuration (persistent, normally
// /etc/netcli/startup.yaml) and the running configuration (what is applied
// now, normally /var/run/netcli/running.yaml). The CLI and the web service
// both go through the store, serialised by an exclusive file lock, so they
// never work on stale copies of each other's changes.
type Store struct {
	StartupPath string
	RunningPath string
	LockPath    string
}

// NewStore builds a store from a configuration directory and run directory.
func NewStore(etcDir, runDir string) *Store {
	return &Store{
		StartupPath: filepath.Join(etcDir, "startup.yaml"),
		RunningPath: filepath.Join(runDir, "running.yaml"),
		LockPath:    filepath.Join(runDir, "netcli.lock"),
	}
}

// Lock takes the store lock and returns a function that releases it.
func (s *Store) Lock() (func(), error) {
	if err := os.MkdirAll(filepath.Dir(s.LockPath), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(s.LockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("lock %s: %w", s.LockPath, err)
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

// LoadFile reads a configuration file. A missing file yields an empty
// configuration and ok=false.
func LoadFile(path string) (c *Config, ok bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return New(), false, nil
	}
	if err != nil {
		return nil, false, err
	}
	c, err = Unmarshal(data)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", path, err)
	}
	return c, true, nil
}

// SaveFile writes a configuration atomically (temp file, fsync, rename).
func SaveFile(path string, c *Config) error {
	data, err := Marshal(c)
	if err != nil {
		return err
	}
	return WriteFileAtomic(path, data, 0o600)
}

// WriteFileAtomic writes data to path via a temporary file in the same
// directory, so a power loss never leaves a half written file behind.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	if d, err := os.Open(dir); err == nil {
		d.Sync()
		d.Close()
	}
	return nil
}

// Startup loads the startup configuration.
func (s *Store) Startup() (*Config, bool, error) { return LoadFile(s.StartupPath) }

// Running loads the running configuration. Before anything has been
// applied (no running file yet) the startup configuration is returned
// together with applied=false.
func (s *Store) Running() (c *Config, applied bool, err error) {
	c, ok, err := LoadFile(s.RunningPath)
	if err != nil || ok {
		return c, ok, err
	}
	c, _, err = s.Startup()
	return c, false, err
}

// SaveRunning records the configuration that is now applied.
func (s *Store) SaveRunning(c *Config) error { return SaveFile(s.RunningPath, c) }

// SaveStartup persists the configuration ("write memory").
func (s *Store) SaveStartup(c *Config) error { return SaveFile(s.StartupPath, c) }
