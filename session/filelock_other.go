//go:build !(darwin || dragonfly || freebsd || linux || netbsd || openbsd || windows)

package session

import "os"

func tryLockFile(f *os.File) (bool, error) { return false, errNoFileLock }

func unlockFile(f *os.File) error { return errNoFileLock }
