//go:build windows

package session

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// fileLocksSupported reports whether FileStore claims work here.
const fileLocksSupported = true

// tryLockFile takes an exclusive lock on f without waiting, reporting false
// when another open file holds it.
func tryLockFile(f *os.File) (bool, error) {
	var ol windows.Overlapped
	err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &ol)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return false, nil
	}
	return err == nil, err
}

// unlockFile releases the lock tryLockFile took.
func unlockFile(f *os.File) error {
	var ol windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &ol)
}
