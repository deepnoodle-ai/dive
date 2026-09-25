//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package session

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// fileLocksSupported reports whether FileStore claims work here.
const fileLocksSupported = true

// tryLockFile takes an exclusive lock on f without waiting, reporting false
// when another open file holds it.
func tryLockFile(f *os.File) (bool, error) {
	err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

// unlockFile releases the lock tryLockFile took.
func unlockFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}
