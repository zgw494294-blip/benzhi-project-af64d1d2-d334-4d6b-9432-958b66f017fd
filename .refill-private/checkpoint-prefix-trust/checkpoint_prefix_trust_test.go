package checkpoint_prefix_trust_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"tapemastergate/internal/application"
	"tapemastergate/internal/domain"
	"tapemastergate/internal/journal"
	"testing"
)

func TestCheckpointPrefixCorruptionIsRejected(t *testing.T) {
	dir := t.TempDir()
	store, err := journal.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc := application.NewService(store)
	_, err = svc.CreateJob(context.Background(), application.CreateJobCommand{
		Meta: application.Meta{
			ExpectedVersion: 0,
			IdempotencyKey:  "checkpoint-prefix-create",
			Actor:           "采集员",
			Role:            "operator",
		},
		Title:         "trusted-title",
		CollectionRef: "ARCHIVE-001",
		Profile: domain.CaptureProfile{
			SampleRate: 96000,
			BitDepth:   24,
			Channels:   2,
			NamingRule: "{carrierCode}.wav",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(dir, "events.jsonl")
	contents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := bytes.Replace(contents, []byte("trusted-title"), []byte("altered-title"), 1)
	if bytes.Equal(contents, tampered) {
		t.Fatal("测试夹具未找到待篡改的事件载荷")
	}
	if err = os.WriteFile(logPath, tampered, 0600); err != nil {
		t.Fatal(err)
	}

	reopened, err := journal.Open(dir)
	if err == nil {
		_ = reopened.Close()
		t.Fatal("检查点覆盖的日志前缀已损坏，重启却仍然成功")
	}
}
