package application_test

import (
	"context"
	"errors"
	"tapemastergate/internal/application"
	"tapemastergate/internal/domain"
	"tapemastergate/internal/journal"
	"testing"
)

func createJob(t *testing.T, svc *application.Service, key, rule string) application.CommitResult {
	t.Helper()
	r, err := svc.CreateJob(context.Background(), application.CreateJobCommand{Meta: meta(0, key+"-create", "采集员", "operator"), Title: "扩展流程测试", CollectionRef: "COLL", Profile: domain.CaptureProfile{SampleRate: 96000, BitDepth: 24, Channels: 2, NamingRule: rule}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func addCarrier(t *testing.T, svc *application.Service, job string, version int64, key, code, grade string, cleaning bool) application.CommitResult {
	t.Helper()
	r, err := svc.AddCarrier(context.Background(), application.AddCarrierCommand{Meta: meta(version, key, "采集员", "operator"), JobID: job, CarrierCode: code, Format: "Cassette", ExpectedDurationMS: 60000, ConditionGrade: grade, CleaningRequired: cleaning, AssessmentNote: "载体评估"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func completeChecks(t *testing.T, svc *application.Service, job string, version int64, key string, carriers ...string) application.CommitResult {
	t.Helper()
	checks := make([]application.CarrierPreflightInput, 0, len(carriers))
	for _, id := range carriers {
		checks = append(checks, application.CarrierPreflightInput{CarrierID: id, CleaningCompleted: true, AppearancePassed: true, PlaybackCompatible: true, DispositionNote: "已完成清洁与处置", DispositionCompleted: true})
	}
	r, err := svc.CompletePreflight(context.Background(), application.CompletePreflightCommand{Meta: meta(version, key, "采集员", "operator"), JobID: job, PlaybackCalibrated: true, StorageAvailable: true, CarrierChecks: checks})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func capture(t *testing.T, svc *application.Service, job, carrier string, version int64, key, digest, supersedes, resolution string, peak float64) application.CommitResult {
	t.Helper()
	r, err := svc.RegisterCapture(context.Background(), application.RegisterCaptureCommand{Meta: meta(version, key, "采集员", "operator"), JobID: job, CarrierID: carrier, SampleRate: 96000, BitDepth: 24, Channels: 2, DurationMS: 60000, SHA256: digest, Operator: "采集员", SupersedesID: supersedes, ResolutionNote: resolution, ContentSummary: "测试音频", Metrics: domain.CaptureMetrics{PeakDBFS: peak}})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCarrierPreflightBlockersAndReplay(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := journal.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc := application.NewService(store)
	job := createJob(t, svc, "pf", "{carrierCode}.wav")
	first := addCarrier(t, svc, job.JobID, job.Version, "pf-c1", "T-01", "poor", true)
	second := addCarrier(t, svc, job.JobID, first.Version, "pf-c2", "T-02", "good", false)
	failed, err := svc.CompletePreflight(ctx, application.CompletePreflightCommand{Meta: meta(second.Version, "pf-failed", "采集员", "operator"), JobID: job.JobID, PlaybackCalibrated: true, StorageAvailable: true, CarrierChecks: []application.CarrierPreflightInput{{CarrierID: first.ResourceID, CleaningCompleted: false, AppearancePassed: true, PlaybackCompatible: true}, {CarrierID: second.ResourceID, CleaningCompleted: true, AppearancePassed: true, PlaybackCompatible: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != domain.StatusDraft || failed.Preflight == nil || failed.Preflight.Ready || len(failed.Preflight.Blockers) == 0 || failed.Preflight.Blockers[0].CarrierCode != "T-01" {
		t.Fatalf("未通过的逐盘前检没有返回可定位阻断: %+v", failed)
	}
	ready := completeChecks(t, svc, job.JobID, failed.Version, "pf-ready", first.ResourceID, second.ResourceID)
	if ready.Status != domain.StatusReady {
		t.Fatalf("完整前检后未进入就绪: %+v", ready)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = journal.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	detail, err := application.NewService(store).GetJob(ctx, job.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Preflight == nil || len(detail.Preflight.CarrierChecks) != 2 || detail.Preflight.CarrierChecks[0].CheckedBy != "采集员" || detail.Preflight.CarrierChecks[0].CheckedAt.IsZero() {
		t.Fatalf("重建后逐盘检查事实不完整: %+v", detail.Preflight)
	}
	added := addCarrier(t, application.NewService(store), job.JobID, detail.Job.Version, "pf-c3", "T-03", "good", false)
	if added.Status != domain.StatusDraft {
		t.Fatalf("就绪后新增载体必须使评估失效并回到建档状态: %+v", added)
	}
	detail, err = application.NewService(store).GetJob(ctx, job.JobID)
	if err != nil || detail.Preflight != nil {
		t.Fatalf("新增载体后不应继续返回旧的作业级前检: %+v, %v", detail.Preflight, err)
	}
}

func TestCaptureUniquenessAndLineage(t *testing.T) {
	ctx := context.Background()
	store, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := application.NewService(store)
	job := createJob(t, svc, "lineage", "{carrierCode}.wav")
	firstCarrier := addCarrier(t, svc, job.JobID, job.Version, "lineage-c1", "A B", "good", false)
	secondCarrier := addCarrier(t, svc, job.JobID, firstCarrier.Version, "lineage-c2", "A@B", "good", false)
	ready := completeChecks(t, svc, job.JobID, secondCarrier.Version, "lineage-pf", firstCarrier.ResourceID, secondCarrier.ResourceID)
	firstTake := capture(t, svc, job.JobID, firstCarrier.ResourceID, ready.Version, "lineage-t1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "", "", -3)
	_, err = svc.RegisterCapture(ctx, application.RegisterCaptureCommand{Meta: meta(firstTake.Version, "lineage-dup", "采集员", "operator"), JobID: job.JobID, CarrierID: secondCarrier.ResourceID, SampleRate: 96000, BitDepth: 24, Channels: 2, DurationMS: 60000, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Operator: "采集员", Metrics: domain.CaptureMetrics{PeakDBFS: -3}})
	if !errors.Is(err, domain.ErrDuplicateCapture) {
		t.Fatalf("重复摘要应返回稳定冲突，得到 %v", err)
	}
	_, err = svc.RegisterCapture(ctx, application.RegisterCaptureCommand{Meta: meta(firstTake.Version, "lineage-name", "采集员", "operator"), JobID: job.JobID, CarrierID: secondCarrier.ResourceID, SampleRate: 96000, BitDepth: 24, Channels: 2, DurationMS: 60000, SHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Operator: "采集员", Metrics: domain.CaptureMetrics{PeakDBFS: -3}})
	if !errors.Is(err, domain.ErrFilenameCollision) {
		t.Fatalf("规范化文件名碰撞应返回稳定冲突，得到 %v", err)
	}
	detail, err := svc.GetJob(ctx, job.JobID)
	if err != nil || detail.Job.Version != firstTake.Version || len(detail.Captures) != 1 {
		t.Fatalf("冲突登记不应改变投影或作业版本: %+v, %v", detail, err)
	}
}

func TestRemediationRejectionCanEnterNextRound(t *testing.T) {
	ctx := context.Background()
	store, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := application.NewService(store)
	job := createJob(t, svc, "rem", "{carrierCode}_{sequence}.wav")
	carrier := addCarrier(t, svc, job.JobID, job.Version, "rem-c", "R-01", "good", false)
	ready := completeChecks(t, svc, job.JobID, carrier.Version, "rem-pf", carrier.ResourceID)
	first := capture(t, svc, job.JobID, carrier.ResourceID, ready.Version, "rem-t1", "1111111111111111111111111111111111111111111111111111111111111111", "", "", 0)
	second := capture(t, svc, job.JobID, carrier.ResourceID, first.Version, "rem-t2", "2222222222222222222222222222222222222222222222222222222222222222", first.ResourceID, "第一次重新调电平", 0)
	detail, err := svc.GetJob(ctx, job.JobID)
	if err != nil {
		t.Fatal(err)
	}
	var original domain.QualityFinding
	for _, finding := range detail.Findings {
		if finding.CaptureTakeID == first.ResourceID && finding.RuleCode == "CLIPPING" {
			original = finding
		}
	}
	_, err = svc.ReviewFinding(ctx, application.ReviewFindingCommand{Meta: meta(second.Version, "rem-close-fail", "审校员", "reviewer"), JobID: job.JobID, FindingID: original.ID, Decision: "close", ResolutionNote: "尝试关闭", CarrierID: carrier.ResourceID, CaptureTakeID: second.ResourceID, RemediationRound: 1})
	if err == nil {
		t.Fatal("同规则复跑仍失败时不应允许关闭")
	}
	rejected, err := svc.ReviewFinding(ctx, application.ReviewFindingCommand{Meta: meta(second.Version, "rem-reject", "审校员", "reviewer"), JobID: job.JobID, FindingID: original.ID, Decision: "reject", ResolutionNote: "削波仍存在，请继续调整", CarrierID: carrier.ResourceID, CaptureTakeID: second.ResourceID, RemediationRound: 1})
	if err != nil {
		t.Fatal(err)
	}
	third := capture(t, svc, job.JobID, carrier.ResourceID, rejected.Version, "rem-t3", "3333333333333333333333333333333333333333333333333333333333333333", second.ResourceID, "第二次降低增益", -3)
	detail, err = svc.GetJob(ctx, job.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.CaptureLineages) != 1 || len(detail.CaptureLineages[0].Versions) != 3 || !detail.CaptureLineages[0].Versions[2].CurrentMaster || detail.CaptureLineages[0].Versions[0].VoidReason == "" || detail.CaptureLineages[0].Versions[1].VoidReason == "" {
		t.Fatalf("三代采集谱系或当前母版标识不完整: %+v", detail.CaptureLineages)
	}
	for _, finding := range detail.Findings {
		if finding.ID == original.ID {
			original = finding
		}
	}
	if original.Status != domain.FindingPending || original.RemediationRound != 2 || len(original.ReviewHistory) != 1 || original.ReviewHistory[0].Decision != "reject" {
		t.Fatalf("驳回记录或下一整改轮次未保留: %+v", original)
	}
	_, err = svc.ReviewFinding(ctx, application.ReviewFindingCommand{Meta: meta(third.Version, "rem-close", "审校员", "reviewer"), JobID: job.JobID, FindingID: original.ID, Decision: "close", ResolutionNote: "第二轮规则复跑通过", CarrierID: carrier.ResourceID, CaptureTakeID: third.ResourceID, RemediationRound: 2})
	if err != nil {
		t.Fatalf("下一轮复跑通过后应允许关闭: %v", err)
	}
}

func TestManifestPreviewIsReadOnlyAndBoundToVersion(t *testing.T) {
	ctx := context.Background()
	store, err := journal.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	svc := application.NewService(store)
	job := createJob(t, svc, "manifest", "{carrierCode}_{sequence}.wav")
	carrier := addCarrier(t, svc, job.JobID, job.Version, "manifest-c", "M-01", "good", false)
	ready := completeChecks(t, svc, job.JobID, carrier.Version, "manifest-pf", carrier.ResourceID)
	first := capture(t, svc, job.JobID, carrier.ResourceID, ready.Version, "manifest-t1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "", "", -3)
	submitted, err := svc.SubmitApproval(ctx, application.SubmitApprovalCommand{Meta: meta(first.Version, "manifest-submit", "审校员", "reviewer"), JobID: job.JobID})
	if err != nil {
		t.Fatal(err)
	}
	auditCount := len(svc.AuditTimeline(ctx, job.JobID))
	preview, err := svc.ManifestPreview(ctx, job.JobID)
	if err != nil || !preview.CanFreeze || preview.JobVersion != submitted.Version {
		t.Fatalf("无阻断预检应可冻结: %+v, %v", preview, err)
	}
	if len(svc.AuditTimeline(ctx, job.JobID)) != auditCount {
		t.Fatal("只读预检不应追加审计事件")
	}
	voided, err := svc.VoidCapture(ctx, application.VoidCaptureCommand{Meta: meta(submitted.Version, "manifest-void", "采集员", "operator"), JobID: job.JobID, CaptureID: first.ResourceID, Reason: "文件损坏"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.FreezeManifest(ctx, application.FreezeManifestCommand{Meta: meta(voided.Version, "manifest-stale", "负责人", "manager"), JobID: job.JobID, PreviewVersion: preview.JobVersion, PreviewDigest: preview.ProposedDigest})
	if !errors.Is(err, domain.ErrStalePreview) {
		t.Fatalf("数据变化后旧预检应稳定冲突，得到 %v", err)
	}
	second := capture(t, svc, job.JobID, carrier.ResourceID, voided.Version, "manifest-t2", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "", "重新采集损坏文件", -3)
	resubmitted, err := svc.SubmitApproval(ctx, application.SubmitApprovalCommand{Meta: meta(second.Version, "manifest-resubmit", "审校员", "reviewer"), JobID: job.JobID})
	if err != nil {
		t.Fatal(err)
	}
	preview, err = svc.ManifestPreview(ctx, job.JobID)
	if err != nil || !preview.CanFreeze {
		t.Fatalf("补齐后预检应通过: %+v, %v", preview, err)
	}
	_, err = svc.FreezeManifest(ctx, application.FreezeManifestCommand{Meta: meta(resubmitted.Version, "manifest-freeze", "负责人", "manager"), JobID: job.JobID, PreviewVersion: preview.JobVersion, PreviewDigest: preview.ProposedDigest})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := svc.GetJob(ctx, job.JobID)
	if err != nil || detail.Manifest == nil || detail.Manifest.ManifestDigest != preview.ProposedDigest {
		t.Fatalf("冻结清单必须与确认预检一致: %+v, %v", detail.Manifest, err)
	}
}
