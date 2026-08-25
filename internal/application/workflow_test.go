package application_test

import (
	"context"
	"tapemastergate/internal/application"
	"tapemastergate/internal/domain"
	"tapemastergate/internal/journal"
	"testing"
)

func meta(v int64, key, actor, role string) application.Meta {
	return application.Meta{ExpectedVersion: v, IdempotencyKey: key, Actor: actor, Role: role}
}
func TestCompleteReleaseWorkflow(t *testing.T) {
	ctx := context.Background()
	store, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := application.NewService(store)
	created, err := svc.CreateJob(ctx, application.CreateJobCommand{Meta: meta(0, "1", "采集员", "operator"), Title: "测试作业", CollectionRef: "COLL", Profile: domain.CaptureProfile{SampleRate: 96000, BitDepth: 24, Channels: 2, NamingRule: "{carrierCode}.wav"}})
	if err != nil {
		t.Fatal(err)
	}
	job := created.JobID
	carrier, err := svc.AddCarrier(ctx, application.AddCarrierCommand{Meta: meta(created.Version, "2", "采集员", "operator"), JobID: job, CarrierCode: "T-1", Format: "Cassette", ExpectedDurationMS: 60000, ConditionGrade: "good"})
	if err != nil {
		t.Fatal(err)
	}
	ready, err := svc.CompletePreflight(ctx, application.CompletePreflightCommand{Meta: meta(carrier.Version, "3", "采集员", "operator"), JobID: job, CarrierCleaned: true, PlaybackCalibrated: true, StorageAvailable: true})
	if err != nil {
		t.Fatal(err)
	}
	captured, err := svc.RegisterCapture(ctx, application.RegisterCaptureCommand{Meta: meta(ready.Version, "4", "采集员", "operator"), JobID: job, CarrierID: carrier.ResourceID, SampleRate: 96000, BitDepth: 24, Channels: 2, DurationMS: 60000, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Operator: "采集员", Metrics: domain.CaptureMetrics{PeakDBFS: -3}})
	if err != nil {
		t.Fatal(err)
	}
	submitted, err := svc.SubmitApproval(ctx, application.SubmitApprovalCommand{Meta: meta(captured.Version, "5", "审校员", "reviewer"), JobID: job})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := svc.ManifestPreview(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := svc.FreezeManifest(ctx, application.FreezeManifestCommand{Meta: meta(submitted.Version, "6", "负责人", "manager"), JobID: job, PreviewVersion: preview.JobVersion, PreviewDigest: preview.ProposedDigest})
	if err != nil {
		t.Fatal(err)
	}
	issued, err := svc.IssueCertificate(ctx, application.IssueCertificateCommand{Meta: meta(frozen.Version, "7", "负责人", "manager"), JobID: job})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := svc.GetJob(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Job.Status != domain.StatusCertified || detail.Certificate == nil {
		t.Fatalf("作业未完成发证: %+v", detail)
	}
	verification := svc.VerifyCertificate(ctx, detail.Certificate.CertificateNo, detail.Certificate.VerificationCode)
	if !verification.Valid {
		t.Fatalf("凭据验证失败: %+v", verification)
	}
	if issued.ResourceID != detail.Certificate.CertificateNo {
		t.Fatalf("签发结果编号不一致")
	}
}
func TestExpectedVersionConflict(t *testing.T) {
	ctx := context.Background()
	store, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := application.NewService(store)
	created, err := svc.CreateJob(ctx, application.CreateJobCommand{Meta: meta(0, "c1", "甲", "operator"), Title: "并发测试", CollectionRef: "C", Profile: domain.CaptureProfile{SampleRate: 48000, BitDepth: 24, Channels: 2, NamingRule: "{carrierCode}.wav"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AddCarrier(ctx, application.AddCarrierCommand{Meta: meta(created.Version-1, "c2", "甲", "operator"), JobID: created.JobID, CarrierCode: "A", Format: "Cassette", ExpectedDurationMS: 1, ConditionGrade: "good"})
	if err == nil {
		t.Fatal("陈旧版本写入应冲突")
	}
}
