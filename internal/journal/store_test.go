package journal

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"tapemastergate/internal/application"
	"tapemastergate/internal/domain"
	"testing"
	"time"
)

func TestCommitReplayAndIdempotency(t *testing.T) {
	dir := t.TempDir()
	store, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(20, 0)
	job := domain.DigitizationJob{ID: "job", Title: "测试", CollectionRef: "C", Status: domain.StatusDraft, CaptureProfile: domain.CaptureProfile{SampleRate: 96000, BitDepth: 24, Channels: 2, NamingRule: "x"}, Version: 1, CreatedAt: now, UpdatedAt: now}
	want := application.CommitResult{JobID: "job", Version: 1, Status: domain.StatusDraft}
	got, err := store.Commit(context.Background(), "job", 0, "key", []application.Event{application.NewEvent("job.created", "job", "测试员", now, job)}, want)
	if err != nil {
		t.Fatal(err)
	}
	again, err := store.Commit(context.Background(), "job", 999, "key", nil, application.CommitResult{})
	if err != nil || again != got {
		t.Fatalf("幂等重试未返回原结果: %+v %v", again, err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	snap := reopened.Snapshot(context.Background())
	if snap.Jobs["job"].Title != "测试" {
		t.Fatalf("投影未从日志重建")
	}
	if prior, ok := reopened.IdempotentResult(context.Background(), "key"); !ok || prior.Version != 1 {
		t.Fatalf("幂等结果未持久化")
	}
}
func TestTruncatedTailDiagnosed(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(`{"schemaVersion":1`), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := Open(dir)
	if err == nil || !strings.Contains(err.Error(), "截断尾帧") {
		t.Fatalf("应明确诊断截断尾帧，得到 %v", err)
	}
}
