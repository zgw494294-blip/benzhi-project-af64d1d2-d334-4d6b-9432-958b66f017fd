package eventbatch_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"tapemastergate/internal/application"
	"tapemastergate/internal/domain"
)

type fixedClock struct{}

func (fixedClock) Now() time.Time {
	return time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
}

type sequentialIDs struct {
	mu   sync.Mutex
	next int
}

func (g *sequentialIDs) NewID(prefix string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.next++
	return fmt.Sprintf("%s-%d", prefix, g.next)
}

type recordedCommit struct {
	jobID  string
	events []application.Event
}

type controlledStore struct {
	snapshot     application.Snapshot
	firstEntered chan struct{}
	releaseFirst chan struct{}

	mu      sync.Mutex
	calls   int
	records []recordedCommit
}

func (s *controlledStore) Snapshot(context.Context) application.Snapshot {
	return s.snapshot.Clone()
}

func (s *controlledStore) IdempotentResult(context.Context, string) (application.CommitResult, bool) {
	return application.CommitResult{}, false
}

func (s *controlledStore) Commit(_ context.Context, jobID string, _ int64, _ string, events []application.Event, result application.CommitResult) (application.CommitResult, error) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()

	if call == 1 {
		close(s.firstEntered)
		<-s.releaseFirst
	}

	batch := append([]application.Event(nil), events...)
	s.mu.Lock()
	s.records = append(s.records, recordedCommit{jobID: jobID, events: batch})
	s.mu.Unlock()
	return result, nil
}

func (s *controlledStore) Close() error { return nil }

func (s *controlledStore) commits() []recordedCommit {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedCommit(nil), s.records...)
}

func TestConcurrentEventBatchesRemainBoundToTheirJobs(t *testing.T) {
	snapshot := application.NewSnapshot()
	snapshot.Jobs["job-a"] = domain.DigitizationJob{ID: "job-a", Status: domain.StatusDraft, Version: 1}
	snapshot.Jobs["job-b"] = domain.DigitizationJob{ID: "job-b", Status: domain.StatusDraft, Version: 1}
	store := &controlledStore{
		snapshot:     snapshot,
		firstEntered: make(chan struct{}),
		releaseFirst: make(chan struct{}),
	}
	service := application.NewServiceWithDependencies(store, fixedClock{}, &sequentialIDs{})

	command := func(jobID, key, carrierCode string) application.AddCarrierCommand {
		return application.AddCarrierCommand{
			Meta: application.Meta{
				ExpectedVersion: 1,
				IdempotencyKey:  key,
				Actor:           "operator-1",
				Role:            "operator",
			},
			JobID:              jobID,
			CarrierCode:        carrierCode,
			Format:             "open-reel",
			ExpectedDurationMS: 60_000,
			ConditionGrade:     "good",
		}
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := service.AddCarrier(context.Background(), command("job-a", "key-a", "A-001"))
		firstDone <- err
	}()

	<-store.firstEntered
	if _, err := service.AddCarrier(context.Background(), command("job-b", "key-b", "B-001")); err != nil {
		t.Fatalf("second command failed: %v", err)
	}
	close(store.releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first command failed: %v", err)
	}

	records := store.commits()
	if len(records) != 2 {
		t.Fatalf("got %d commits, want 2", len(records))
	}
	for _, record := range records {
		if len(record.events) != 2 {
			t.Fatalf("commit for %s has %d events, want 2", record.jobID, len(record.events))
		}
		for _, event := range record.events {
			if event.JobID != record.jobID {
				t.Fatalf("commit for %s contains event owned by %s", record.jobID, event.JobID)
			}
		}
	}
}
