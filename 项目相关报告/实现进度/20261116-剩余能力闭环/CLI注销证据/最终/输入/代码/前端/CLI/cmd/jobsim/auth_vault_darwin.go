//go:build darwin

// Created by Jobs on 2026年11月15日，星期日.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const darwinVaultLimit = 8 * 1024

type darwinCredentialVault struct {
	account string
	helper  string
	digest  [sha256.Size]byte
}

type darwinVaultOutput struct {
	data  []byte
	limit int
}

func (b *darwinVaultOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-len(b.data) {
		return 0, errVaultUnavailable
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func openCredentialVault(slot string) (credentialVault, error) {
	decoded, err := hex.DecodeString(slot)
	if err != nil || len(decoded) != sha256.Size || strings.ToLower(slot) != slot {
		return nil, errVaultUnavailable
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, errVaultUnavailable
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, errVaultUnavailable
	}
	directory := filepath.Join(filepath.Dir(executable), "Helpers")
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0022 != 0 {
		return nil, errVaultUnavailable
	}
	manifest := filepath.Join(directory, "SHA256SUMS")
	info, err = os.Lstat(manifest)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Size() > 4096 {
		return nil, errVaultUnavailable
	}
	content, err := os.ReadFile(manifest)
	if err != nil {
		return nil, errVaultUnavailable
	}
	fields := strings.Fields(string(content))
	if len(fields) != 2 || fields[1] != "JobsIMKeychain" {
		return nil, errVaultUnavailable
	}
	digest, err := hex.DecodeString(fields[0])
	if err != nil || len(digest) != sha256.Size {
		return nil, errVaultUnavailable
	}
	v := &darwinCredentialVault{account: slot, helper: filepath.Join(directory, "JobsIMKeychain")}
	copy(v.digest[:], digest)
	if err = v.verify(); err != nil {
		return nil, err
	}
	return v, nil
}

func (v *darwinCredentialVault) verify() error {
	info, err := os.Lstat(v.helper)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Mode().Perm()&0111 == 0 || info.Size() > 32*1024*1024 {
		return errVaultUnavailable
	}
	f, err := os.Open(v.helper)
	if err != nil {
		return errVaultUnavailable
	}
	h := sha256.New()
	n, readErr := io.Copy(h, io.LimitReader(f, 32*1024*1024+1))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil || n != info.Size() || !bytes.Equal(h.Sum(nil), v.digest[:]) {
		return errVaultUnavailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "/usr/bin/codesign", "--verify", "--strict", v.helper)
	command.Env = []string{"PATH=/usr/bin:/bin"}
	command.Stdout, command.Stderr = io.Discard, io.Discard
	if command.Run() != nil {
		return errVaultUnavailable
	}
	return nil
}

func (v *darwinCredentialVault) request(operation string, value []byte) ([]byte, error) {
	if len(value) > darwinVaultLimit || operation == "write" && len(value) == 0 {
		return nil, errVaultUnavailable
	}
	if err := v.verify(); err != nil {
		return nil, err
	}
	request, err := json.Marshal(struct {
		Operation string `json:"operation"`
		Account   string `json:"account"`
		Data      []byte `json:"data,omitempty"`
	}{operation, v.account, value})
	if err != nil {
		return nil, errVaultUnavailable
	}
	defer erase(request)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, v.helper)
	command.Env = []string{"PATH=/usr/bin:/bin"}
	command.Stdin, command.Stderr = bytes.NewReader(request), io.Discard
	output := &darwinVaultOutput{limit: 12 * 1024}
	defer func() { erase(output.data) }()
	command.Stdout = output
	if command.Run() != nil {
		return nil, errVaultUnavailable
	}
	var response struct {
		Status string `json:"status"`
		Data   []byte `json:"data,omitempty"`
	}
	if json.Unmarshal(output.data, &response) != nil {
		erase(response.Data)
		return nil, errVaultUnavailable
	}
	if response.Status == "not_found" {
		erase(response.Data)
		return nil, errVaultNotFound
	}
	if response.Status != "ok" || len(response.Data) > darwinVaultLimit || operation == "read" && len(response.Data) == 0 || operation != "read" && len(response.Data) != 0 {
		erase(response.Data)
		return nil, errVaultUnavailable
	}
	return response.Data, nil
}

func (v *darwinCredentialVault) Read() ([]byte, error) {
	return v.request("read", nil)
}

func (v *darwinCredentialVault) Write(value []byte) error {
	_, err := v.request("write", value)
	return err
}

func (v *darwinCredentialVault) Delete() error {
	_, err := v.request("delete", nil)
	if err == errVaultNotFound {
		return nil
	}
	return err
}
