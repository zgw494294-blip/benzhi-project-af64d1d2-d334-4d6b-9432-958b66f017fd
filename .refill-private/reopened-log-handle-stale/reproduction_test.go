package reopenedlog_test

import (
	"context"
	"tapemastergate/internal/application"
	"tapemastergate/internal/domain"
	"tapemastergate/internal/journal"
	"testing"
	"time"
)

func TestReopenedStoreOwnsWritableLogHandle(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()
	job := domain.DigitizationJob{
		ID:            "job-reopen",
		Title:         "重启句柄复现",
		CollectionRef: "COL-REOPEN",
		Status:        domain.StatusDraft,
		CaptureProfile: domain.CaptureProfile{
			SampleRate: 96000,
			BitDepth:   24,
			Channels:   2,
			NamingRule: "{carrierCode}_{sequence}.wav",
		},
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}

	first, err := journal.Open(dir)
	if err != nil {
		t.Fatalf("首次打开存储失败: %v", err)
	}
	created := application.NewEvent("job.created", job.ID, "operator-a", now, job)
	if _, err = first.Commit(ctx, job.ID, 0, "create-before-reopen", []application.Event{created}, application.CommitResult{JobID: job.ID, Version: 1, Status: job.Status}); err != nil {
		t.Fatalf("首次提交失败: %v", err)
	}
	reopened, err := journal.Open(dir)
	if err != nil {
		t.Fatalf("滚动切换时打开新存储失败: %v", err)
	}
	if err = first.Close(); err != nil {
		t.Fatalf("关闭旧存储失败: %v", err)
	}
	defer reopened.Close()
	job.Status = domain.StatusReady
	job.Version = 2
	job.UpdatedAt = now.Add(time.Minute)
	changed := application.NewEvent("job.status_changed", job.ID, "operator-b", job.UpdatedAt, job)
	if _, err = reopened.Commit(ctx, job.ID, 1, "commit-after-reopen", []application.Event{changed}, application.CommitResult{JobID: job.ID, Version: 2, Status: job.Status}); err != nil {
		t.Fatalf("重新打开后的存储必须拥有可写日志句柄: %v", err)
	}
}
