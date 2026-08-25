package domain

import (
	"encoding/hex"
	"strings"
)

func ValidateProfile(p CaptureProfile) error {
	if p.SampleRate != 44100 && p.SampleRate != 48000 && p.SampleRate != 96000 && p.SampleRate != 192000 {
		return Invalid("sampleRate", "必须是 44100、48000、96000 或 192000")
	}
	if p.BitDepth != 16 && p.BitDepth != 24 && p.BitDepth != 32 {
		return Invalid("bitDepth", "必须是 16、24 或 32")
	}
	if p.Channels < 1 || p.Channels > 8 {
		return Invalid("channels", "必须在 1 到 8 之间")
	}
	return ValidateNamingRule(p.NamingRule)
}

func ValidateCarrier(c TapeCarrier) error {
	if strings.TrimSpace(c.CarrierCode) == "" {
		return Invalid("carrierCode", "不能为空")
	}
	if strings.TrimSpace(c.Format) == "" {
		return Invalid("format", "不能为空")
	}
	if c.ExpectedDurationMS <= 0 {
		return Invalid("expectedDurationMs", "必须大于零")
	}
	if c.ConditionGrade != "good" && c.ConditionGrade != "fair" && c.ConditionGrade != "poor" {
		return Invalid("conditionGrade", "必须是 good、fair 或 poor")
	}
	return nil
}

func ValidateCapture(t CaptureTake) error {
	if t.CarrierID == "" {
		return Invalid("carrierId", "不能为空")
	}
	if t.DurationMS <= 0 {
		return Invalid("durationMs", "必须大于零")
	}
	if strings.TrimSpace(t.Operator) == "" {
		return Invalid("operator", "不能为空")
	}
	b, err := hex.DecodeString(t.SHA256)
	if err != nil || len(b) != 32 {
		return Invalid("sha256", "必须是 64 位十六进制 SHA-256")
	}
	if t.Sequence < 1 {
		return Invalid("sequence", "必须大于零")
	}
	return nil
}

func ValidateFinding(f QualityFinding) error {
	if f.CaptureTakeID == "" {
		return Invalid("captureTakeId", "不能为空")
	}
	if f.Severity != SeverityInfo && f.Severity != SeverityMinor && f.Severity != SeverityMajor && f.Severity != SeverityCritical {
		return Invalid("severity", "级别无效")
	}
	if f.StartMS < 0 || f.EndMS < f.StartMS {
		return Invalid("timecode", "时间码范围无效")
	}
	if strings.TrimSpace(f.Description) == "" {
		return Invalid("description", "不能为空")
	}
	return nil
}
