//go:build !windows

package jobsim

import (
	"os"
	"syscall"
)

func syncPrivateTokenFile(info os.FileInfo, _ *os.File) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == uint32(os.Geteuid()) && info.Mode().Perm() == 0600
}
