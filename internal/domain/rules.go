package domain

import (
	"fmt"
	"math"
	"time"
)

type RuleEvaluation struct{ Findings []QualityFinding }

func EvaluateCapture(job DigitizationJob, carrier TapeCarrier, take CaptureTake, now time.Time, id func() string) RuleEvaluation {
	result := RuleEvaluation{}
	add := func(code string, severity Severity, start, end int64, description string) {
		result.Findings = append(result.Findings, QualityFinding{ID: id(), JobID: job.ID, CaptureTakeID: take.ID, CarrierID: carrier.ID, CurrentCaptureTakeID: take.ID, Source: "rule", RuleCode: code, Severity: severity, StartMS: start, EndMS: end, Description: description, Status: FindingOpen, CreatedAt: now})
	}
	if take.SampleRate != job.CaptureProfile.SampleRate || take.BitDepth != job.CaptureProfile.BitDepth || take.Channels != job.CaptureProfile.Channels {
		add("PARAMETER_MISMATCH", SeverityMajor, 0, 0, "采集技术参数与作业规格不一致")
	}
	deviation := math.Abs(float64(take.DurationMS-carrier.ExpectedDurationMS)) / float64(carrier.ExpectedDurationMS)
	if deviation > 0.05 {
		add("DURATION_DEVIATION", SeverityMajor, 0, take.DurationMS, fmt.Sprintf("时长与预期偏差 %.1f%%", deviation*100))
	}
	if take.Metrics.PeakDBFS >= -0.1 {
		add("CLIPPING", SeverityCritical, 0, take.DurationMS, "峰值接近或达到数字满刻度，疑似削波")
	}
	if take.Metrics.DropoutCount > 0 {
		add("DROPOUT", SeverityMajor, 0, take.DurationMS, fmt.Sprintf("检测到 %d 处掉音", take.Metrics.DropoutCount))
	}
	if take.Metrics.LongestSilenceMS > 30000 {
		add("LONG_SILENCE", SeverityMinor, 0, take.Metrics.LongestSilenceMS, "检测到超过 30 秒的连续静音")
	}
	return result
}

func HasOpenSevere(findings []QualityFinding) bool {
	for _, f := range findings {
		if f.Status != FindingClosed && (f.Severity == SeverityMajor || f.Severity == SeverityCritical) {
			return true
		}
	}
	return false
}
