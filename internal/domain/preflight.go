package domain

import (
	"sort"
	"strings"
	"time"
)

// EvaluatePreflight produces a deterministic, carrier-addressable readiness result.
func EvaluatePreflight(carriers []TapeCarrier, checks []CarrierPreflightCheck, playbackCalibrated, storageAvailable bool, actor string, at time.Time) (PreflightCheck, error) {
	result := PreflightCheck{PlaybackCalibrated: playbackCalibrated, StorageAvailable: storageAvailable, CheckedBy: actor, CheckedAt: at.UTC()}
	byCarrier := make(map[string]CarrierPreflightCheck, len(checks))
	known := make(map[string]TapeCarrier, len(carriers))
	for _, carrier := range carriers {
		known[carrier.ID] = carrier
	}
	for i, check := range checks {
		carrier, ok := known[check.CarrierID]
		if !ok {
			return PreflightCheck{}, Invalid("carrierChecks["+itoa(i)+"].carrierId", "载体不属于当前作业")
		}
		if _, duplicate := byCarrier[check.CarrierID]; duplicate {
			return PreflightCheck{}, Invalid("carrierChecks["+itoa(i)+"].carrierId", "同一载体只能提交一次前检结论")
		}
		check.CarrierCode = carrier.CarrierCode
		check.CheckedBy = actor
		check.CheckedAt = at.UTC()
		check.Blockers = carrierPreflightBlockers(carrier, check)
		check.Passed = len(check.Blockers) == 0
		byCarrier[check.CarrierID] = check
	}
	ordered := append([]TapeCarrier(nil), carriers...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].CarrierCode < ordered[j].CarrierCode })
	for _, carrier := range ordered {
		check, ok := byCarrier[carrier.ID]
		if !ok {
			result.Blockers = append(result.Blockers, PreflightBlocker{Code: "carrier_check_missing", CarrierID: carrier.ID, CarrierCode: carrier.CarrierCode, Message: "载体缺少前检结论"})
			continue
		}
		result.CarrierChecks = append(result.CarrierChecks, check)
		for _, message := range check.Blockers {
			result.Blockers = append(result.Blockers, PreflightBlocker{Code: "carrier_check_failed", CarrierID: carrier.ID, CarrierCode: carrier.CarrierCode, Message: message})
		}
	}
	if !playbackCalibrated {
		result.Blockers = append(result.Blockers, PreflightBlocker{Code: "playback_not_calibrated", Message: "播放设备尚未完成校准"})
	}
	if !storageAvailable {
		result.Blockers = append(result.Blockers, PreflightBlocker{Code: "storage_unavailable", Message: "存储空间检查未通过"})
	}
	result.Ready = len(carriers) > 0 && len(result.Blockers) == 0
	return result, nil
}

func carrierPreflightBlockers(carrier TapeCarrier, check CarrierPreflightCheck) []string {
	var blockers []string
	if !check.CleaningCompleted {
		blockers = append(blockers, "清洁尚未完成")
	}
	if !check.AppearancePassed {
		blockers = append(blockers, "外观复核未通过")
	}
	if !check.PlaybackCompatible {
		blockers = append(blockers, "播放兼容性检查未通过")
	}
	requiresDisposition := carrier.CleaningRequired || carrier.ConditionGrade == "poor"
	if requiresDisposition && strings.TrimSpace(check.DispositionNote) == "" {
		blockers = append(blockers, "需要清洁或保存状况较差时必须填写处置说明")
	}
	if requiresDisposition && !check.DispositionCompleted {
		blockers = append(blockers, "载体处置尚未确认完成")
	}
	return blockers
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = byte('0' + v%10)
		v /= 10
	}
	return string(b[i:])
}
