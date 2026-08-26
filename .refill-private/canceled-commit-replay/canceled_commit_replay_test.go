package canceled_commit_replay_test

import (
	"context"
	"errors"
	"tapemastergate/internal/application"
	"tapemastergate/internal/domain"
	"tapemastergate/internal/journal"
	"testing"
	"time"
)

func TestCanceledCommitDoesNotReappearAfterRestart(t *testing.T) {
	dir := t.TempDir()
	store, err := journal.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Unix(1_700_000_000, 0).UTC()
	job := domain.DigitizationJob{
		ID:            "job-canceled",
		Title:         "已取消的建档",
		CollectionRef: "ARCHIVE-CANCELED",
		Status:        domain.StatusDraft,
		CaptureProfile: domain.CaptureProfile{
			SampleRate: 96000,
			BitDepth:   24,
			Channels:   2,
			NamingRule: "{collectionRef}_{carrierCode}_{sequence}",
		},
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	result := application.CommitResult{
		JobID:      job.ID,
		Version:    job.Version,
		ResourceID: job.ID,
		Status:     job.Status,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = store.Commit(ctx, job.ID, 0, "canceled-create", []application.Event{
		application.NewEvent("job.created", job.ID, "采集员", now, job),
	}, result)
	if !errors.Is(err, context.Canceled) {
		_ = store.Close()
		t.Fatalf("预期已取消的提交返回 context.Canceled，得到 %v", err)
	}
	if _, exists := store.Snapshot(context.Background()).Jobs[job.ID]; exists {
		_ = store.Close()
		t.Fatal("已取消的提交不应进入当前内存投影")
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := journal.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if _, exists := reopened.Snapshot(context.Background()).Jobs[job.ID]; exists {
		t.Fatal("TestCanceledCommitDoesNotReappearAfterRestart: 已取消且对调用方报告失败的提交在重启回放后重新出现")
	}
}
