//go:build darwin || linux

// Created by Jobs on 2026年11月18日，星期三.
package jobsim

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDeploymentDiskReserveIgnoresShadowedDF(t *testing.T) {
	directory := t.TempDir()
	var filesystem unix.Statfs_t
	if err := unix.Statfs(directory, &filesystem); err != nil {
		t.Fatal(err)
	}
	if uint64(filesystem.Bavail)*uint64(filesystem.Bsize) < 4<<30 {
		t.Skip("positive reserve check requires at least 4 GiB on the test filesystem")
	}
	// A failing executable named df must not replace the system capacity probe.
	if err := os.Symlink("/usr/bin/false", filepath.Join(directory, "df")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory)
	if err := deploymentRequireSpace(context.Background(), directory, 0); err != nil {
		t.Fatalf("PATH shadowing broke the available system disk probe: %v", err)
	}
	for _, additional := range []int64{-1, math.MaxInt64} {
		if err := deploymentRequireSpace(context.Background(), directory, additional); err == nil {
			t.Fatal("invalid additional space bypassed the protected reserve")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := deploymentRequireSpace(ctx, directory, 0); err == nil {
		t.Fatal("cancelled capacity probe was accepted")
	}
}
