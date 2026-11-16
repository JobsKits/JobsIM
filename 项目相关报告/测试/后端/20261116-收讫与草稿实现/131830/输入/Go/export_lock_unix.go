//go:build darwin || linux || freebsd || openbsd || netbsd

package jobsim

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"
)

func lockExportFile(ctx context.Context, file *os.File, exclusive bool) error {
	mode := syscall.LOCK_SH
	if exclusive {
		mode = syscall.LOCK_EX
	}
	for {
		e := syscall.Flock(int(file.Fd()), mode|syscall.LOCK_NB)
		if e == nil {
			return nil
		}
		if !errors.Is(e, syscall.EWOULDBLOCK) && !errors.Is(e, syscall.EAGAIN) {
			return e
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}
func unlockExportFile(file *os.File) { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }
