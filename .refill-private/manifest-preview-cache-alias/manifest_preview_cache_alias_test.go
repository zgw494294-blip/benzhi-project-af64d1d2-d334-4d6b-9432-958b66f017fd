package manifest_preview_cache_alias_test

import (
	"context"
	"testing"

	"tapemastergate/internal/application"
	"tapemastergate/internal/domain"
)

type snapshotStore struct {
	snapshot application.Snapshot
}

func (s *snapshotStore) Snapshot(context.Context) application.Snapshot {
	return s.snapshot.Clone()
}

func (s *snapshotStore) Commit(context.Context, string, int64, string, []application.Event, application.CommitResult) (application.CommitResult, error) {
	panic("unexpected commit")
}

func (s *snapshotStore) IdempotentResult(context.Context, string) (application.CommitResult, bool) {
	return application.CommitResult{}, false
}

func (s *snapshotStore) Close() error { return nil }

func TestManifestPreviewCacheDoesNotLeakCallerMutation(t *testing.T) {
	snapshot := application.NewSnapshot()
	snapshot.Jobs["job-1"] = domain.DigitizationJob{
		ID:      "job-1",
		Version: 9,
		CaptureProfile: domain.CaptureProfile{
			SampleRate: 96000,
			BitDepth:   24,
			Channels:   2,
		},
	}
	snapshot.Carriers["carrier-1"] = domain.TapeCarrier{
		ID:          "carrier-1",
		JobID:       "job-1",
		CarrierCode: "TAPE-001",
	}
	snapshot.Captures["capture-1"] = domain.CaptureTake{
		ID:         "capture-1",
		JobID:      "job-1",
		CarrierID:  "carrier-1",
		Sequence:   1,
		SampleRate: 96000,
		BitDepth:   24,
		Channels:   2,
		SHA256:     "original-digest",
		Filename:   "TAPE-001.wav",
		Status:     domain.CaptureValid,
	}

	service := application.NewService(&snapshotStore{snapshot: snapshot})
	first, err := service.ManifestPreview(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("首次预检失败: %v", err)
	}
	if len(first.Items) != 1 {
		t.Fatalf("首次预检应包含一个母版，实际为 %d", len(first.Items))
	}
	first.Items[0].CaptureID = "caller-corrupted-capture"
	first.Items[0].SHA256 = "caller-corrupted-digest"

	second, err := service.ManifestPreview(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("缓存预检失败: %v", err)
	}
	if second.Items[0].CaptureID != "capture-1" || second.Items[0].SHA256 != "original-digest" {
		t.Fatalf("同版本缓存泄漏了调用方修改: %+v", second.Items[0])
	}
}
