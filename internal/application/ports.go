package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"tapemastergate/internal/domain"
	"time"
)

type Event struct {
	Type  string          `json:"type"`
	JobID string          `json:"jobId"`
	Actor string          `json:"actor"`
	At    time.Time       `json:"at"`
	Data  json.RawMessage `json:"data"`
}

func NewEvent(kind, jobID, actor string, at time.Time, value any) Event {
	b, _ := json.Marshal(value)
	return Event{Type: kind, JobID: jobID, Actor: actor, At: at.UTC(), Data: b}
}

type CommitResult struct {
	JobID      string                 `json:"jobId"`
	Version    int64                  `json:"version"`
	ResourceID string                 `json:"resourceId,omitempty"`
	Status     domain.JobStatus       `json:"status"`
	Preflight  *domain.PreflightCheck `json:"preflight,omitempty"`
	// Fingerprint records the operation and normalized payload digest that
	// first claimed this idempotency key. It is bookkeeping metadata only and
	// is excluded from JSON serialization so it never leaks into API responses;
	// the journal frame persists it separately so it survives replay.
	Fingerprint RequestFingerprint `json:"-"`
}

// RequestFingerprint identifies the logical request bound to an idempotency key.
// Two requests reuse an idempotency key only when they carry the same operation
// and the same normalized payload digest; any divergence must surface as a
// stable conflict rather than replaying the prior result.
type RequestFingerprint struct {
	Operation string `json:"operation"`
	PayloadDigest string `json:"payloadDigest"`
}

// Fingerprint computes a stable digest for the normalized payload of a write
// request. The operation label disambiguates command kinds, while the payload
// digest folds the command-specific fields (excluding volatile Meta such as
// expectedVersion, which legitimately changes across retries) into a canonical
// SHA-256 digest.
func Fingerprint(operation string, payload any) RequestFingerprint {
	b, _ := json.Marshal(payload)
	sum := sha256.Sum256(b)
	return RequestFingerprint{Operation: operation, PayloadDigest: hex.EncodeToString(sum[:])}
}

type Store interface {
	Snapshot(context.Context) Snapshot
	Commit(context.Context, string, int64, string, []Event, CommitResult) (CommitResult, error)
	IdempotentResult(context.Context, string) (CommitResult, bool)
	Close() error
}

type Clock interface{ Now() time.Time }
type IDGenerator interface{ NewID(prefix string) string }

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }
