//go:build windows

package engram

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

const lockfileExclusiveLock = 0x00000002

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = kernel32.NewProc("LockFileEx")
	procUnlockFileEx = kernel32.NewProc("UnlockFileEx")
)

type processFileLock struct {
	file       *os.File
	overlapped syscall.Overlapped
}

func acquireProcessFileLock(path string) (*processFileLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	lock := &processFileLock{file: file}
	result, _, callErr := procLockFileEx.Call(
		file.Fd(),
		uintptr(lockfileExclusiveLock),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&lock.overlapped)),
	)
	if result == 0 {
		_ = file.Close()
		if callErr != syscall.Errno(0) {
			return nil, callErr
		}
		return nil, errors.New("LockFileEx failed")
	}
	return lock, nil
}

func (lock *processFileLock) release() error {
	result, _, callErr := procUnlockFileEx.Call(
		lock.file.Fd(),
		0,
		1,
		0,
		uintptr(unsafe.Pointer(&lock.overlapped)),
	)
	closeErr := lock.file.Close()
	if result == 0 {
		if callErr != syscall.Errno(0) {
			return callErr
		}
		return errors.New("UnlockFileEx failed")
	}
	return closeErr
}
