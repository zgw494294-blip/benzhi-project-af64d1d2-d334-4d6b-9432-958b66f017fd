package application

import (
	"context"
	"sort"
	"tapemastergate/internal/domain"
)

type JobDetail struct {
	Job             domain.DigitizationJob         `json:"job"`
	Carriers        []domain.TapeCarrier           `json:"carriers"`
	Preflight       *domain.PreflightCheck         `json:"preflight,omitempty"`
	Captures        []domain.CaptureTake           `json:"captures"`
	Findings        []domain.QualityFinding        `json:"findings"`
	Remediations    []domain.RemediationSubmission `json:"remediations"`
	CaptureLineages []domain.CarrierCaptureLineage `json:"captureLineages"`
	FindingQueue    FindingQueueResult             `json:"findingQueue"`
	Manifest        *domain.DeliveryManifest       `json:"manifest,omitempty"`
	Certificate     *domain.ReleaseCertificate     `json:"certificate,omitempty"`
	NextActions     []string                       `json:"nextActions"`
}

func (s *Service) ListJobs(ctx context.Context) []domain.DigitizationJob {
	snap := s.store.Snapshot(ctx)
	out := make([]domain.DigitizationJob, 0, len(snap.Jobs))
	for _, v := range snap.Jobs {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}
func (s *Service) GetJob(ctx context.Context, id string) (JobDetail, error) {
	snap := s.store.Snapshot(ctx)
	job, err := getJob(snap, id)
	if err != nil {
		return JobDetail{}, err
	}
	d := JobDetail{Job: job, Carriers: snap.JobCarriers(id), Captures: snap.JobCaptures(id), Findings: snap.JobFindings(id), Remediations: snap.JobRemediations(id), NextActions: nextActions(job, snap)}
	d.CaptureLineages = buildCaptureLineages(d.Carriers, d.Captures, d.Remediations)
	d.FindingQueue = buildFindingQueue(d.Findings, FindingQueueFilter{})
	if p, ok := snap.Preflights[id]; ok {
		d.Preflight = &p
	}
	if m, ok := snap.Manifests[id]; ok {
		d.Manifest = &m
	}
	for _, c := range snap.Certificates {
		if c.JobID == id {
			v := c
			d.Certificate = &v
			break
		}
	}
	return d, nil
}
func nextActions(job domain.DigitizationJob, snap Snapshot) []string {
	switch job.Status {
	case domain.StatusDraft:
		return []string{"add_carrier", "complete_preflight"}
	case domain.StatusReady:
		return []string{"register_capture"}
	case domain.StatusCapturing:
		return []string{"register_capture", "add_finding", "submit_approval"}
	case domain.StatusRemediation:
		return []string{"register_replacement", "review_findings", "submit_approval"}
	case domain.StatusPendingApproval:
		return []string{"preview_manifest", "freeze_manifest"}
	case domain.StatusFrozen:
		return []string{"issue_certificate"}
	default:
		return []string{"verify_certificate", "view_audit"}
	}
}

type FindingQueueFilter struct {
	Severity  string
	Source    string
	CarrierID string
}

type FindingQueueSummary struct {
	Open          int `json:"open"`
	PendingReview int `json:"pendingReview"`
	Closed        int `json:"closed"`
	Rejected      int `json:"rejected"`
}

type FindingQueueResult struct {
	Summary  FindingQueueSummary     `json:"summary"`
	Findings []domain.QualityFinding `json:"findings"`
}

func buildFindingQueue(findings []domain.QualityFinding, filter FindingQueueFilter) FindingQueueResult {
	result := FindingQueueResult{Findings: []domain.QualityFinding{}}
	for _, f := range findings {
		switch f.Status {
		case domain.FindingOpen:
			result.Summary.Open++
		case domain.FindingPending:
			result.Summary.PendingReview++
		case domain.FindingClosed:
			result.Summary.Closed++
		case domain.FindingRejected:
			result.Summary.Rejected++
		}
		if filter.Severity != "" && string(f.Severity) != filter.Severity {
			continue
		}
		if filter.Source != "" && f.Source != filter.Source {
			continue
		}
		if filter.CarrierID != "" && f.CarrierID != filter.CarrierID {
			continue
		}
		result.Findings = append(result.Findings, f)
	}
	severityRank := map[domain.Severity]int{domain.SeverityCritical: 0, domain.SeverityMajor: 1, domain.SeverityMinor: 2, domain.SeverityInfo: 3}
	statusRank := map[domain.FindingStatus]int{domain.FindingPending: 0, domain.FindingOpen: 1, domain.FindingRejected: 2, domain.FindingClosed: 3}
	sort.SliceStable(result.Findings, func(i, j int) bool {
		a, b := result.Findings[i], result.Findings[j]
		if a.CarrierID != b.CarrierID {
			return a.CarrierID < b.CarrierID
		}
		if a.CurrentCaptureTakeID != b.CurrentCaptureTakeID {
			return a.CurrentCaptureTakeID < b.CurrentCaptureTakeID
		}
		if severityRank[a.Severity] != severityRank[b.Severity] {
			return severityRank[a.Severity] < severityRank[b.Severity]
		}
		if statusRank[a.Status] != statusRank[b.Status] {
			return statusRank[a.Status] < statusRank[b.Status]
		}
		return a.CreatedAt.Before(b.CreatedAt)
	})
	return result
}

func (s *Service) FindingQueue(ctx context.Context, jobID string, filter FindingQueueFilter) (FindingQueueResult, error) {
	snap := s.store.Snapshot(ctx)
	if _, err := getJob(snap, jobID); err != nil {
		return FindingQueueResult{}, err
	}
	return buildFindingQueue(snap.JobFindings(jobID), filter), nil
}

func buildCaptureLineages(carriers []domain.TapeCarrier, captures []domain.CaptureTake, remediations []domain.RemediationSubmission) []domain.CarrierCaptureLineage {
	byCarrier := map[string][]domain.CaptureTake{}
	byReplacement := map[string]domain.RemediationSubmission{}
	successor := map[string]string{}
	for _, take := range captures {
		byCarrier[take.CarrierID] = append(byCarrier[take.CarrierID], take)
	}
	for _, remediation := range remediations {
		byReplacement[remediation.ReplacementCaptureID] = remediation
		successor[remediation.PreviousCaptureID] = remediation.ReplacementCaptureID
	}
	ordered := append([]domain.TapeCarrier(nil), carriers...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].CarrierCode < ordered[j].CarrierCode })
	out := make([]domain.CarrierCaptureLineage, 0, len(ordered))
	for _, carrier := range ordered {
		lineage := domain.CarrierCaptureLineage{CarrierID: carrier.ID, CarrierCode: carrier.CarrierCode, Versions: []domain.CaptureLineageEntry{}}
		versions := byCarrier[carrier.ID]
		sort.Slice(versions, func(i, j int) bool { return versions[i].Sequence < versions[j].Sequence })
		for _, take := range versions {
			entry := domain.CaptureLineageEntry{CaptureTake: take}
			if next, ok := successor[take.ID]; ok {
				entry.SupersededByID = next
			}
			if remediation, ok := byReplacement[take.ID]; ok {
				copy := remediation
				entry.Remediation = &copy
			}
			if take.Status == domain.CaptureValid {
				entry.CurrentMaster = true
				lineage.HasActiveMaster = true
				lineage.CurrentMasterID = take.ID
			}
			lineage.Versions = append(lineage.Versions, entry)
		}
		out = append(out, lineage)
	}
	return out
}

func (s *Service) ManifestPreview(ctx context.Context, jobID string) (domain.ManifestPreview, error) {
	snap := s.store.Snapshot(ctx)
	job, err := getJob(snap, jobID)
	if err != nil {
		return domain.ManifestPreview{}, err
	}
	s.previewMu.RLock()
	cached, ok := s.manifestCache[jobID]
	s.previewMu.RUnlock()
	if ok && cached.jobVersion == job.Version {
		return cloneManifestPreview(cached.preview), nil
	}
	revision := 1
	if old, exists := snap.Manifests[jobID]; exists {
		revision = old.Revision + 1
	}
	preview := domain.BuildManifestPreview(job, snap.JobCarriers(jobID), snap.JobCaptures(jobID), snap.JobFindings(jobID), revision)
	s.previewMu.Lock()
	s.manifestCache[jobID] = cachedManifestPreview{jobVersion: job.Version, preview: cloneManifestPreview(preview)}
	s.previewMu.Unlock()
	return cloneManifestPreview(preview), nil
}

// cloneManifestPreview returns a preview whose slice fields own independent
// backing arrays, so callers cannot mutate cached entries through the returned
// value and corrupt subsequent queries for the same job version.
func cloneManifestPreview(p domain.ManifestPreview) domain.ManifestPreview {
	if p.Items != nil {
		items := make([]domain.ManifestPreviewItem, len(p.Items))
		copy(items, p.Items)
		p.Items = items
	}
	if p.Blockers != nil {
		blockers := make([]domain.ManifestBlocker, len(p.Blockers))
		copy(blockers, p.Blockers)
		p.Blockers = blockers
	}
	return p
}
func (s *Service) AuditTimeline(ctx context.Context, jobID string) []domain.AuditEntry {
	snap := s.store.Snapshot(ctx)
	var out []domain.AuditEntry
	for _, a := range snap.Audits {
		if a.JobID == jobID {
			out = append(out, a)
		}
	}
	return out
}

type CertificateVerification struct {
	Valid       bool                       `json:"valid"`
	Certificate *domain.ReleaseCertificate `json:"certificate,omitempty"`
	Manifest    *domain.DeliveryManifest   `json:"manifest,omitempty"`
	Reason      string                     `json:"reason,omitempty"`
}

func (s *Service) VerifyCertificate(ctx context.Context, number, code string) CertificateVerification {
	snap := s.store.Snapshot(ctx)
	cert, ok := snap.Certificates[number]
	if !ok {
		return CertificateVerification{Reason: "凭据不存在"}
	}
	if cert.VerificationCode != code {
		return CertificateVerification{Reason: "校验码不匹配"}
	}
	manifest, ok := snap.Manifests[cert.JobID]
	if !ok || manifest.ID != cert.ManifestID || manifest.ManifestDigest != cert.ManifestDigest {
		return CertificateVerification{Reason: "凭据与冻结清单不一致"}
	}
	return CertificateVerification{Valid: true, Certificate: &cert, Manifest: &manifest}
}
