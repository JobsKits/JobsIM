//go:build darwin || linux

// Created by Jobs on 2026年11月15日，星期日.
package main

import (
	"errors"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

func lockAuthDirectory(path string) (func(), error) {
	failure := errors.New("authentication state is locked by another process or unavailable")
	if os.MkdirAll(path, 0700) != nil {
		return nil, failure
	}
	directory, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, failure
	}
	defer unix.Close(directory)
	var info unix.Stat_t
	if unix.Fstat(directory, &info) != nil || info.Uid != uint32(os.Geteuid()) || info.Mode&0077 != 0 {
		return nil, failure
	}
	file, err := unix.Openat(directory, ".auth.lock", unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, failure
	}
	if unix.Fstat(file, &info) != nil || info.Mode&unix.S_IFMT != unix.S_IFREG || info.Uid != uint32(os.Geteuid()) || info.Mode&0077 != 0 || info.Nlink != 1 {
		unix.Close(file)
		return nil, failure
	}
	if unix.Flock(file, unix.LOCK_EX|unix.LOCK_NB) != nil {
		unix.Close(file)
		return nil, failure
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = unix.Flock(file, unix.LOCK_UN)
			_ = unix.Close(file)
		})
	}, nil
}
