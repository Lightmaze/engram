//go:build !linux && !darwin && !windows

package engram

import (
	"fmt"
	"runtime"
)

type processFileLock struct{}

func acquireProcessFileLock(string) (*processFileLock, error) {
	return nil, fmt.Errorf("cross-process Journal locking is not supported on %s", runtime.GOOS)
}

func (*processFileLock) release() error { return nil }
