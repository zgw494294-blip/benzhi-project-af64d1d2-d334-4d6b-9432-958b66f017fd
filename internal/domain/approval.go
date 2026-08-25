package domain

import "sort"

type CarrierCoverage struct {
	CarrierID       string `json:"carrierId"`
	CarrierCode     string `json:"carrierCode"`
	Covered         bool   `json:"covered"`
	ActiveCaptureID string `json:"activeCaptureId,omitempty"`
	ActiveSequence  int    `json:"activeSequence,omitempty"`
}

type ApprovalAssessment struct {
	Allowed              bool              `json:"allowed"`
	CarrierCount         int               `json:"carrierCount"`
	CoveredCarrierCount  int               `json:"coveredCarrierCount"`
	OpenFindingCount     int               `json:"openFindingCount"`
	BlockingFindingCount int               `json:"blockingFindingCount"`
	Coverage             []CarrierCoverage `json:"coverage"`
	Blockers             []string          `json:"blockers"`
}

func AssessApproval(carriers []TapeCarrier, captures []CaptureTake, findings []QualityFinding) ApprovalAssessment {
	assessment := ApprovalAssessment{CarrierCount: len(carriers)}
	active := map[string]CaptureTake{}
	for _, take := range captures {
		if take.Status != CaptureValid {
			continue
		}
		old, ok := active[take.CarrierID]
		if !ok || take.Sequence > old.Sequence {
			active[take.CarrierID] = take
		}
	}
	ordered := append([]TapeCarrier(nil), carriers...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].CarrierCode < ordered[j].CarrierCode })
	for _, carrier := range ordered {
		coverage := CarrierCoverage{CarrierID: carrier.ID, CarrierCode: carrier.CarrierCode}
		if take, ok := active[carrier.ID]; ok {
			coverage.Covered = true
			coverage.ActiveCaptureID = take.ID
			coverage.ActiveSequence = take.Sequence
			assessment.CoveredCarrierCount++
		}
		assessment.Coverage = append(assessment.Coverage, coverage)
	}
	for _, finding := range findings {
		if finding.Status == FindingOpen {
			assessment.OpenFindingCount++
		}
		if finding.Status != FindingClosed && (finding.Severity == SeverityMajor || finding.Severity == SeverityCritical) {
			assessment.BlockingFindingCount++
		}
	}
	if assessment.CarrierCount == 0 {
		assessment.Blockers = append(assessment.Blockers, "至少需要一盘载体")
	}
	if assessment.CoveredCarrierCount < assessment.CarrierCount {
		assessment.Blockers = append(assessment.Blockers, "仍有载体缺少有效采集")
	}
	if assessment.BlockingFindingCount > 0 {
		assessment.Blockers = append(assessment.Blockers, "仍有未关闭的严重质量发现")
	}
	assessment.Allowed = len(assessment.Blockers) == 0
	return assessment
}

func ActiveMasterTakes(carriers []TapeCarrier, captures []CaptureTake) ([]CaptureTake, bool) {
	active := map[string]CaptureTake{}
	for _, take := range captures {
		if take.Status == CaptureValid {
			old, ok := active[take.CarrierID]
			if !ok || take.Sequence > old.Sequence {
				active[take.CarrierID] = take
			}
		}
	}
	masters := make([]CaptureTake, 0, len(carriers))
	for _, carrier := range carriers {
		take, ok := active[carrier.ID]
		if !ok {
			return nil, false
		}
		masters = append(masters, take)
	}
	sort.Slice(masters, func(i, j int) bool {
		if masters[i].CarrierID == masters[j].CarrierID {
			return masters[i].Sequence < masters[j].Sequence
		}
		return masters[i].CarrierID < masters[j].CarrierID
	})
	return masters, len(masters) > 0
}
