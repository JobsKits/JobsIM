package jobsim

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// A deployment approves an exact native build and its storage verification
// report. A client declaration alone never upgrades a transport registration.
type ReceiptProfile struct {
	ID                 string `json:"id"`
	BuildDigest        string `json:"buildDigest"`
	VerificationDigest string `json:"verificationDigest"`
	StorageContract    string `json:"storageContract"`
}

func LoadReceiptProfiles(path string) ([]ReceiptProfile, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("receipt storage policy unavailable")
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return nil, fmt.Errorf("receipt storage policy exceeds the bounded input")
	}
	var document struct {
		Profiles []ReceiptProfile `json:"profiles"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("invalid receipt storage policy")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return nil, fmt.Errorf("receipt storage policy must contain one document")
	}
	if err = validateReceiptProfiles(document.Profiles); err != nil {
		return nil, err
	}
	return document.Profiles, nil
}

func validateReceiptProfiles(profiles []ReceiptProfile) error {
	if len(profiles) > 64 {
		return fmt.Errorf("too many receipt storage profiles")
	}
	seen := map[string]bool{}
	for _, profile := range profiles {
		if len(profile.ID) < 8 || len(profile.ID) > 128 || !syncValidationDigest(profile.BuildDigest) || !syncValidationDigest(profile.VerificationDigest) || profile.StorageContract != "native_sqlite_full_v1" {
			return fmt.Errorf("receipt policy requires an exact native build and verification digest")
		}
		for _, letter := range profile.ID {
			if !(letter >= 'a' && letter <= 'z' || letter >= '0' && letter <= '9' || letter == '-' || letter == '_' || letter == '.') {
				return fmt.Errorf("receipt profile ID must be canonical ASCII")
			}
		}
		key := profile.ID + ":" + profile.BuildDigest
		if seen[key] {
			return fmt.Errorf("duplicate receipt storage profile")
		}
		seen[key] = true
	}
	return nil
}

func (a *App) receiptProfile(id, build, verification string) (ReceiptProfile, bool) {
	for _, profile := range a.Config.ReceiptProfiles {
		if profile.ID == id && profile.BuildDigest == build && profile.VerificationDigest == verification && profile.StorageContract == "native_sqlite_full_v1" {
			return profile, true
		}
	}
	return ReceiptProfile{}, false
}
