package regression_test

import (
	"context"
	"tapemastergate/internal/application"
	"tapemastergate/internal/domain"
	"tapemastergate/internal/journal"
	"testing"
)

func TestIdempotencyKeyReuseWithDifferentPayloadIsRejected(t *testing.T) {
	store, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := application.NewService(store)
	profile := domain.CaptureProfile{SampleRate: 96000, BitDepth: 24, Channels: 2, NamingRule: "{carrierCode}.wav"}
	first := application.CreateJobCommand{
		Meta:          application.Meta{ExpectedVersion: 0, IdempotencyKey: "reused-key", Actor: "采集员", Role: "operator"},
		Title:         "第一项作业",
		CollectionRef: "COLLECTION-A",
		Profile:       profile,
	}
	created, err := svc.CreateJob(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.Title = "完全不同的第二项作业"
	second.CollectionRef = "COLLECTION-B"
	result, err := svc.CreateJob(context.Background(), second)
	if err == nil {
		t.Fatalf("复用同一 idempotencyKey 提交不同载荷应被拒绝，却返回了首个作业结果 %+v（首个结果 %+v）", result, created)
	}
	if got := len(store.Snapshot(context.Background()).Jobs); got != 1 {
		t.Fatalf("冲突的幂等请求不应产生额外作业，实际为 %d", got)
	}
}
