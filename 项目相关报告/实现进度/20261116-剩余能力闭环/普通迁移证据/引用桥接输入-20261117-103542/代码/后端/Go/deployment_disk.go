package jobsim

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func deploymentRequireSpace(ctx context.Context, path string, additional int64) error {
	var raw []byte
	var err error
	var free uint64
	if runtime.GOOS == "windows" {
		literal := strings.ReplaceAll(filepath.Clean(path), "'", "''")
		raw, err = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "[System.IO.DriveInfo]::new([System.IO.Path]::GetPathRoot('"+literal+"')).AvailableFreeSpace").Output()
		if err == nil {
			free, err = strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
		}
	} else {
		raw, err = exec.CommandContext(ctx, "df", "-Pk", path).Output()
		if err == nil {
			lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
			parts := strings.Fields(lines[len(lines)-1])
			if len(parts) < 4 {
				err = fmt.Errorf("unknown disk capacity response")
			} else {
				free, err = strconv.ParseUint(parts[3], 10, 64)
				free *= 1024
			}
		}
	}
	if err != nil {
		return fmt.Errorf("cannot verify the 4 GiB restore disk reserve")
	}
	if additional < 0 || free < 4<<30 || uint64(additional) > free-(4<<30) {
		return fmt.Errorf("restore requires an untouched 4 GiB disk reserve")
	}
	return nil
}
