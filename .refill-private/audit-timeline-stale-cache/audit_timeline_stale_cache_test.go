package audit_timeline_stale_cache_test

import (
	"context"
	"tapemastergate/internal/application"
	"tapemastergate/internal/domain"
	"tapemastergate/internal/journal"
	"testing"
)

func TestAuditTimelineRefreshesAfterCommittedMutation(t *testing.T) {
	ctx := context.Background()
	store, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	writer := application.NewService(store)
	reader := application.NewService(store)

	created, err := writer.CreateJob(ctx, application.CreateJobCommand{
		Meta:  application.Meta{ExpectedVersion: 0, IdempotencyKey: "audit-create", Actor: "采集员", Role: "operator"},
		Title: "审计缓存复现", CollectionRef: "AUDIT-CACHE",
		Profile: domain.CaptureProfile{SampleRate: 48000, BitDepth: 24, Channels: 2, NamingRule: "{carrierCode}.wav"},
	})
	if err != nil {
		t.Fatal(err)
	}
	before := reader.AuditTimeline(ctx, created.JobID)
	if len(before) != 1 {
		t.Fatalf("首次审计时间线应包含建档事件，实际为 %d 条", len(before))
	}

	_, err = writer.AddCarrier(ctx, application.AddCarrierCommand{
		Meta:  application.Meta{ExpectedVersion: created.Version, IdempotencyKey: "audit-carrier", Actor: "采集员", Role: "operator"},
		JobID: created.JobID, CarrierCode: "TAPE-AUDIT-1", Format: "Cassette",
		ExpectedDurationMS: 60000, ConditionGrade: "good", AssessmentNote: "状态稳定",
	})
	if err != nil {
		t.Fatal(err)
	}

	after := reader.AuditTimeline(ctx, created.JobID)
	if len(after) <= len(before) {
		t.Fatalf("成功提交载体后审计时间线仍停留在 %d 条，未包含新确认事件", len(after))
	}
}
