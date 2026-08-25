package domain

import (
	"sort"
	"strings"
	"time"
)

type RemediationSubmission struct {
	ID                   string            `json:"id"`
	JobID                string            `json:"jobId"`
	CarrierID            string            `json:"carrierId"`
	PreviousCaptureID    string            `json:"previousCaptureId"`
	ReplacementCaptureID string            `json:"replacementCaptureId"`
	Explanation          string            `json:"explanation"`
	SubmittedBy          string            `json:"submittedBy"`
	SubmittedAt          time.Time         `json:"submittedAt"`
	RerunRuleCodes       []string          `json:"rerunRuleCodes"`
	Round                int               `json:"round"`
	RerunResults         []RuleRerunResult `json:"rerunResults"`
}

type RuleRerunResult struct {
	RuleCode  string `json:"ruleCode"`
	Passed    bool   `json:"passed"`
	FindingID string `json:"findingId,omitempty"`
}

func NewRemediation(id string, previous, replacement CaptureTake, explanation, actor string, at time.Time, round int, related []QualityFinding, rerunFindings []QualityFinding) (RemediationSubmission, error) {
	if previous.JobID == "" || previous.JobID != replacement.JobID {
		return RemediationSubmission{}, Invalid("supersedesId", "替代版本必须属于同一作业")
	}
	if previous.CarrierID != replacement.CarrierID {
		return RemediationSubmission{}, Invalid("supersedesId", "替代版本必须属于同一载体")
	}
	if strings.TrimSpace(explanation) == "" {
		return RemediationSubmission{}, Invalid("resolutionNote", "替代采集必须说明修复措施")
	}
	rules := make([]string, 0, len(related))
	seen := map[string]bool{}
	for _, finding := range related {
		if finding.Source == "rule" && finding.RuleCode != "" && !seen[finding.RuleCode] {
			seen[finding.RuleCode] = true
			rules = append(rules, finding.RuleCode)
		}
	}
	sort.Strings(rules)
	results := make([]RuleRerunResult, 0, len(rules))
	for _, code := range rules {
		result := RuleRerunResult{RuleCode: code, Passed: true}
		for _, finding := range rerunFindings {
			if finding.Source == "rule" && finding.RuleCode == code {
				result.Passed = false
				result.FindingID = finding.ID
				break
			}
		}
		results = append(results, result)
	}
	return RemediationSubmission{ID: id, JobID: replacement.JobID, CarrierID: replacement.CarrierID, PreviousCaptureID: previous.ID, ReplacementCaptureID: replacement.ID, Explanation: strings.TrimSpace(explanation), SubmittedBy: actor, SubmittedAt: at.UTC(), RerunRuleCodes: rules, Round: round, RerunResults: results}, nil
}
