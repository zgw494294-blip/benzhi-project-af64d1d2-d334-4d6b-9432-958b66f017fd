package domain

import (
	"testing"
	"time"
)

func TestFrozenCanOnlyBecomeCertified(t *testing.T) {
	job := DigitizationJob{Status: StatusFrozen}
	if err := Transition(&job, StatusCertified); err != nil {
		t.Fatalf("冻结作业应能发证: %v", err)
	}
	if job.Status != StatusCertified {
		t.Fatalf("状态未更新")
	}
	if err := Transition(&job, StatusDraft); err != ErrFrozen {
		t.Fatalf("发证后应不可变，得到 %v", err)
	}
}
func TestEvaluateCaptureDeterministicRules(t *testing.T) {
	job := DigitizationJob{ID: "j", CaptureProfile: CaptureProfile{SampleRate: 96000, BitDepth: 24, Channels: 2}}
	carrier := TapeCarrier{ID: "c", ExpectedDurationMS: 100000}
	take := CaptureTake{ID: "t", SampleRate: 48000, BitDepth: 16, Channels: 1, DurationMS: 80000, Metrics: CaptureMetrics{PeakDBFS: 0, DropoutCount: 2, LongestSilenceMS: 40000}}
	n := 0
	r := EvaluateCapture(job, carrier, take, time.Unix(0, 0), func() string { n++; return string(rune('a' + n)) })
	if len(r.Findings) != 5 {
		t.Fatalf("期望 5 个规则发现，得到 %d", len(r.Findings))
	}
	if r.Findings[0].RuleCode != "PARAMETER_MISMATCH" || r.Findings[4].RuleCode != "LONG_SILENCE" {
		t.Fatalf("规则输出顺序不稳定")
	}
}
func TestManifestDigestStableAcrossInputOrder(t *testing.T) {
	now := time.Unix(10, 0)
	a := CaptureTake{ID: "a", CarrierID: "2", Sequence: 1, SHA256: "aa"}
	b := CaptureTake{ID: "b", CarrierID: "1", Sequence: 1, SHA256: "bb"}
	m1 := BuildManifest("job", 1, []CaptureTake{a, b}, "负责人", now, "m1")
	m2 := BuildManifest("job", 1, []CaptureTake{b, a}, "负责人", now, "m2")
	if m1.ManifestDigest != m2.ManifestDigest {
		t.Fatalf("相同母版集合摘要应稳定")
	}
	if m1.CaptureTakeIDs[0] != "b" {
		t.Fatalf("清单未按载体稳定排序")
	}
}
func TestApprovalRejectsUnclosedSevereFinding(t *testing.T) {
	carriers := []TapeCarrier{{ID: "c", CarrierCode: "01"}}
	captures := []CaptureTake{{ID: "t", CarrierID: "c", Status: CaptureValid}}
	findings := []QualityFinding{{Severity: SeverityMajor, Status: FindingRejected}}
	r := AssessApproval(carriers, captures, findings)
	if r.Allowed || r.BlockingFindingCount != 1 {
		t.Fatalf("被驳回的严重发现仍必须阻止批准: %+v", r)
	}
	findings[0].Status = FindingClosed
	r = AssessApproval(carriers, captures, findings)
	if !r.Allowed {
		t.Fatalf("覆盖完整且严重发现关闭后应允许批准: %+v", r)
	}
}
