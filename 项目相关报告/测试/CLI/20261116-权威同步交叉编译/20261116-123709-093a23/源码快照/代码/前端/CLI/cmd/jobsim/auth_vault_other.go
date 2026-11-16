//go:build !darwin && !linux && !windows

// Created by Jobs on 2026年11月15日，星期日.
package main

func openCredentialVault(slot string) (credentialVault, error) {
	return nil, errVaultUnavailable
}

func lockAuthDirectory(path string) (func(), error) {
	return nil, errVaultUnavailable
}
