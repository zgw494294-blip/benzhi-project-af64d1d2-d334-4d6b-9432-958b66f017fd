package application

import (
	"context"
	"tapemastergate/internal/domain"
)

type ReadinessReport struct {
	JobID               string                    `json:"jobId"`
	Version             int64                     `json:"version"`
	Status              domain.JobStatus          `json:"status"`
	ProfileValid        bool                      `json:"profileValid"`
	PreflightCompleted  bool                      `json:"preflightCompleted"`
	Preflight           *domain.PreflightCheck    `json:"preflight,omitempty"`
	Approval            domain.ApprovalAssessment `json:"approval"`
	CanRegisterCapture  bool                      `json:"canRegisterCapture"`
	CanFreeze           bool                      `json:"canFreeze"`
	CanIssueCertificate bool                      `json:"canIssueCertificate"`
	NextActions         []string                  `json:"nextActions"`
}

type readinessCacheEntry struct {
	version int64
	report  ReadinessReport
}

func (s *Service) cachedReadiness(jobID string, version int64) (ReadinessReport, bool) {
	s.readinessMu.RLock()
	defer s.readinessMu.RUnlock()
	entry, ok := s.readinessCache[jobID]
	if !ok || entry.version != version {
		return ReadinessReport{}, false
	}
	return entry.report, true
}

func (s *Service) rememberReadiness(report ReadinessReport) {
	s.readinessMu.Lock()
	defer s.readinessMu.Unlock()
	s.readinessCache[report.JobID] = readinessCacheEntry{version: report.Version, report: report}
}

func (s *Service) Readiness(ctx context.Context, jobID string) (ReadinessReport, error) {
	snap := s.store.Snapshot(ctx)
	job, err := getJob(snap, jobID)
	if err != nil {
		return ReadinessReport{}, err
	}
	if report, ok := s.cachedReadiness(jobID, job.Version); ok {
		return report, nil
	}
	profileValid := domain.ValidateProfile(job.CaptureProfile) == nil
	preflightCheck, hasPreflight := snap.Preflights[jobID]
	preflight := hasPreflight && preflightCheck.Ready && len(preflightCheck.CarrierChecks) == len(snap.JobCarriers(jobID))
	assessment := domain.AssessApproval(snap.JobCarriers(jobID), snap.JobCaptures(jobID), snap.JobFindings(jobID))
	report := ReadinessReport{JobID: jobID, Version: job.Version, Status: job.Status, ProfileValid: profileValid, PreflightCompleted: preflight, Approval: assessment, NextActions: nextActions(job, snap)}
	if hasPreflight {
		report.Preflight = &preflightCheck
	}
	report.CanRegisterCapture = job.Status == domain.StatusReady || job.Status == domain.StatusCapturing || job.Status == domain.StatusRemediation
	report.CanFreeze = job.Status == domain.StatusPendingApproval && assessment.Allowed
	report.CanIssueCertificate = job.Status == domain.StatusFrozen
	s.rememberReadiness(report)
	return report, nil
}
