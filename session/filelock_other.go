//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd || windows)

package session

import (
	"errors"
	"os"
)

// errNoFileLock is returned by a FileStore claim on a platform without a
// file lock the store can use.
var errNoFileLock = errors.New("session: FileStore claims are not supported on this platform")

func tryLockFile(f *os.File) (bool, error) { return false, errNoFileLock }

func unlockFile(f *os.File) error { return errNoFileLock }
