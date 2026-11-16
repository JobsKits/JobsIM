//go:build linux

// Created by Jobs on 2026年11月15日，星期日.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const linuxVaultLimit = 8 * 1024

type linuxCredentialVault struct{ account string }

type linuxVaultOutput struct {
	data  []byte
	limit int
}

func (b *linuxVaultOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-len(b.data) {
		return 0, errVaultUnavailable
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func openCredentialVault(slot string) (credentialVault, error) {
	decoded, err := hex.DecodeString(slot)
	if err != nil || len(decoded) != 32 || strings.ToLower(slot) != slot || !linuxSecretToolTrusted() {
		return nil, errVaultUnavailable
	}
	return &linuxCredentialVault{account: slot}, nil
}

func linuxSecretToolTrusted() bool {
	info, err := os.Stat("/usr/bin/secret-tool")
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Mode().Perm()&0111 == 0 {
		return false
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	return ok && owner.Uid == 0
}

func (v *linuxCredentialVault) command(operation string, secret []byte) ([]byte, error) {
	if !linuxSecretToolTrusted() {
		return nil, errVaultUnavailable
	}
	args := []string{operation}
	if operation == "store" {
		args = append(args, "--label=JobsIM CLI")
	}
	args = append(args, "service", "JobsIM.CLI", "account", v.account)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/usr/bin/secret-tool", args...)
	command.Env = []string{"PATH=/usr/bin:/bin"}
	for _, key := range []string{"HOME", "DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS", "LANG", "LC_ALL"} {
		if value, present := os.LookupEnv(key); present {
			command.Env = append(command.Env, key+"="+value)
		}
	}
	command.Stdin = bytes.NewReader(secret)
	stdout := &linuxVaultOutput{limit: base64.StdEncoding.EncodedLen(linuxVaultLimit) + 2}
	stderr := &linuxVaultOutput{limit: 4096}
	defer func() { erase(stderr.data) }()
	command.Stdout, command.Stderr = stdout, stderr
	err := command.Run()
	if err != nil {
		empty := len(stdout.data) == 0 && len(stderr.data) == 0
		erase(stdout.data)
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 && empty && ctx.Err() == nil && operation != "store" {
			return nil, errVaultNotFound
		}
		return nil, errVaultUnavailable
	}
	if len(stderr.data) != 0 || operation != "lookup" && len(stdout.data) != 0 {
		erase(stdout.data)
		return nil, errVaultUnavailable
	}
	return stdout.data, nil
}

func (v *linuxCredentialVault) Read() ([]byte, error) {
	encoded, err := v.command("lookup", nil)
	if err != nil {
		return nil, err
	}
	defer erase(encoded)
	encoded = bytes.TrimSuffix(encoded, []byte{'\n'})
	if len(encoded) == 0 || len(encoded) > base64.StdEncoding.EncodedLen(linuxVaultLimit) {
		return nil, errVaultUnavailable
	}
	value := make([]byte, base64.StdEncoding.DecodedLen(len(encoded)))
	n, err := base64.StdEncoding.Strict().Decode(value, encoded)
	if err != nil || n == 0 || n > linuxVaultLimit {
		erase(value)
		return nil, errVaultUnavailable
	}
	return value[:n], nil
}

func (v *linuxCredentialVault) Write(value []byte) error {
	if len(value) == 0 || len(value) > linuxVaultLimit {
		return errVaultUnavailable
	}
	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(value)))
	base64.StdEncoding.Encode(encoded, value)
	defer erase(encoded)
	_, err := v.command("store", encoded)
	return err
}

func (v *linuxCredentialVault) Delete() error {
	_, err := v.command("clear", nil)
	if err == errVaultNotFound {
		return nil
	}
	return err
}
