package application

import (
	"context"
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
