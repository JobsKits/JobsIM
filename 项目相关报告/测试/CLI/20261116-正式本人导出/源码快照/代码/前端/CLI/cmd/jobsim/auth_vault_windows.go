//go:build windows

// Created by Jobs on 2026年11月15日，星期日.
package main

import (
	"encoding/hex"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WinCred's generic blob limit is smaller than the protocol's 8 KiB ceiling.
const windowsVaultLimit = 2560

type windowsCredential struct {
	Flags              uint32
	Type               uint32
	TargetName         *uint16
	Comment            *uint16
	LastWritten        windows.Filetime
	CredentialBlobSize uint32
	CredentialBlob     *byte
	Persist            uint32
	AttributeCount     uint32
	Attributes         uintptr
	TargetAlias        *uint16
	UserName           *uint16
}

type windowsCredentialVault struct {
	target *uint16
	read   *windows.LazyProc
	write  *windows.LazyProc
	delete *windows.LazyProc
	free   *windows.LazyProc
}

func openCredentialVault(slot string) (credentialVault, error) {
	decoded, err := hex.DecodeString(slot)
	if err != nil || len(decoded) != 32 || strings.ToLower(slot) != slot {
		return nil, errVaultUnavailable
	}
	target, err := windows.UTF16PtrFromString("JobsIM.CLI/" + slot)
	if err != nil {
		return nil, errVaultUnavailable
	}
	dll := windows.NewLazySystemDLL("advapi32.dll")
	v := &windowsCredentialVault{target: target, read: dll.NewProc("CredReadW"), write: dll.NewProc("CredWriteW"), delete: dll.NewProc("CredDeleteW"), free: dll.NewProc("CredFree")}
	for _, procedure := range []*windows.LazyProc{v.read, v.write, v.delete, v.free} {
		if procedure.Find() != nil {
			return nil, errVaultUnavailable
		}
	}
	return v, nil
}

func (v *windowsCredentialVault) Read() ([]byte, error) {
	var credential *windowsCredential
	ok, _, callErr := v.read.Call(uintptr(unsafe.Pointer(v.target)), 1, 0, uintptr(unsafe.Pointer(&credential)))
	runtime.KeepAlive(v)
	if ok == 0 {
		if callErr == windows.ERROR_NOT_FOUND {
			return nil, errVaultNotFound
		}
		return nil, errVaultUnavailable
	}
	if credential == nil {
		return nil, errVaultUnavailable
	}
	defer v.free.Call(uintptr(unsafe.Pointer(credential)))
	if credential.Type != 1 || credential.Persist != 2 || credential.CredentialBlobSize == 0 || credential.CredentialBlobSize > windowsVaultLimit || credential.CredentialBlob == nil {
		return nil, errVaultUnavailable
	}
	value := append([]byte(nil), unsafe.Slice(credential.CredentialBlob, int(credential.CredentialBlobSize))...)
	return value, nil
}

func (v *windowsCredentialVault) Write(value []byte) error {
	if len(value) == 0 || len(value) > windowsVaultLimit {
		return errVaultUnavailable
	}
	credential := windowsCredential{Type: 1, TargetName: v.target, CredentialBlobSize: uint32(len(value)), CredentialBlob: &value[0], Persist: 2}
	ok, _, _ := v.write.Call(uintptr(unsafe.Pointer(&credential)), 0)
	runtime.KeepAlive(value)
	runtime.KeepAlive(v)
	if ok == 0 {
		return errVaultUnavailable
	}
	return nil
}

func (v *windowsCredentialVault) Delete() error {
	ok, _, callErr := v.delete.Call(uintptr(unsafe.Pointer(v.target)), 1, 0)
	runtime.KeepAlive(v)
	if ok == 0 && callErr != windows.ERROR_NOT_FOUND {
		return errVaultUnavailable
	}
	return nil
}
