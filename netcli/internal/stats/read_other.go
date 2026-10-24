//go:build !netbsd && !linux

package stats

import (
	"errors"
	"runtime"
)

// Read is not implemented on this platform.
func Read() (*Sample, error) {
	return nil, errors.New("statistics are not supported on " + runtime.GOOS)
}
