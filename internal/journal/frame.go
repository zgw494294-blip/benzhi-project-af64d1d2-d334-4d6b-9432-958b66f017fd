package journal

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"tapemastergate/internal/application"
	"time"
)

const schemaVersion = 1

type frame struct {
	SchemaVersion  int                      `json:"schemaVersion"`
	Sequence       int64                    `json:"sequence"`
	PreviousDigest string                   `json:"previousDigest"`
	CommittedAt    time.Time                `json:"committedAt"`
	JobID          string                   `json:"jobId"`
	IdempotencyKey string                   `json:"idempotencyKey"`
	Events         []application.Event      `json:"events"`
	Result         application.CommitResult `json:"result"`
	Fingerprint    *application.RequestFingerprint `json:"fingerprint,omitempty"`
	Checksum       string                   `json:"checksum"`
}

func (f frame) canonical() ([]byte, error) { f.Checksum = ""; return json.Marshal(f) }
func (f frame) digest() (string, error) {
	b, err := f.canonical()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
func (f *frame) seal() error {
	sum, err := f.digest()
	if err != nil {
		return err
	}
	f.Checksum = sum
	return nil
}
func (f frame) verify() error {
	expected := f.Checksum
	actual, err := f.digest()
	if err != nil {
		return err
	}
	if expected != actual {
		return ErrChecksum
	}
	return nil
}
