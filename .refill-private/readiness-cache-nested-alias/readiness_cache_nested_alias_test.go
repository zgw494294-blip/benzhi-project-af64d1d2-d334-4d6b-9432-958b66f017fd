package readiness_cache_nested_alias_test

import (
	"context"
	"tapemastergate/internal/application"
	"tapemastergate/internal/domain"
	"tapemastergate/internal/journal"
	"testing"
)

func TestReadinessCacheDoesNotLeakCallerMutation(t *testing.T) {
	ctx := context.Background()
	store, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := application.NewService(store)

	created, err := svc.CreateJob(ctx, application.CreateJobCommand{
		Meta:          application.Meta{ExpectedVersion: 0, IdempotencyKey: "create", Actor: "采集员", Role: "operator"},
		Title:         "缓存隔离复现",
		CollectionRef: "COLL-CACHE",
		Profile:       domain.CaptureProfile{SampleRate: 96000, BitDepth: 24, Channels: 2, NamingRule: "{carrierCode}.wav"},
	})
	if err != nil {
		t.Fatal(err)
	}
	carrier, err := svc.AddCarrier(ctx, application.AddCarrierCommand{
		Meta:               application.Meta{ExpectedVersion: created.Version, IdempotencyKey: "carrier", Actor: "采集员", Role: "operator"},
		JobID:              created.JobID,
		CarrierCode:        "TAPE-ORIGINAL",
		Format:             "Cassette",
		ExpectedDurationMS: 60000,
		ConditionGrade:     "good",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.CompletePreflight(ctx, application.CompletePreflightCommand{
		Meta:               application.Meta{ExpectedVersion: carrier.Version, IdempotencyKey: "preflight", Actor: "采集员", Role: "operator"},
		JobID:              created.JobID,
		PlaybackCalibrated: true,
		StorageAvailable:   true,
		CarrierChecks: []application.CarrierPreflightInput{{
			CarrierID:            carrier.ResourceID,
			CleaningCompleted:    true,
			AppearancePassed:     true,
			PlaybackCompatible:   true,
			DispositionNote:      "已完成检查",
			DispositionCompleted: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	first, err := svc.Readiness(ctx, created.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Preflight == nil || len(first.Preflight.CarrierChecks) != 1 {
		t.Fatalf("前检报告不完整: %+v", first.Preflight)
	}
	first.Preflight.CarrierChecks[0].CarrierCode = "TAPE-FORGED"
	first.Preflight.CarrierChecks[0].DispositionNote = "调用方伪造内容"

	second, err := svc.Readiness(ctx, created.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if second.Preflight == nil || len(second.Preflight.CarrierChecks) != 1 {
		t.Fatalf("再次查询的前检报告不完整: %+v", second.Preflight)
	}
	if second.Preflight.CarrierChecks[0].CarrierCode != "TAPE-ORIGINAL" || second.Preflight.CarrierChecks[0].DispositionNote != "已完成检查" {
		t.Fatalf("缓存泄漏了调用方修改: %+v", second.Preflight.CarrierChecks[0])
	}
}
