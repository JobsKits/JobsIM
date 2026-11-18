//go:build windows

// Created by Jobs on 2026年11月15日，星期日.
package main

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/windows"
)

func lockAuthDirectory(path string) (func(), error) {
	failure := errors.New("authentication state is locked by another process or unavailable")
	if os.MkdirAll(path, 0700) != nil {
		return nil, failure
	}
	directory, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, failure
	}
	attributes, err := windows.GetFileAttributes(directory)
	if err != nil || attributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return nil, failure
	}
	name, err := windows.UTF16PtrFromString(filepath.Join(path, ".auth.lock"))
	if err != nil {
		return nil, failure
	}
	file, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, failure
	}
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(file, &info) != nil || info.FileAttributes&(windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_REPARSE_POINT) != 0 || info.NumberOfLinks != 1 {
		windows.CloseHandle(file)
		return nil, failure
	}
	var operation windows.Overlapped
	if windows.LockFileEx(file, windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &operation) != nil {
		windows.CloseHandle(file)
		return nil, failure
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = windows.UnlockFileEx(file, 0, 1, 0, &operation)
			_ = windows.CloseHandle(file)
		})
	}, nil
}
