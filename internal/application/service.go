package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"tapemastergate/internal/domain"
)

type Service struct {
	store          Store
	clock          Clock
	ids            IDGenerator
	queryMu        sync.Mutex
	jobDetailCalls map[string]*jobDetailCall
}
type randomIDs struct{}

func (randomIDs) NewID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func NewService(store Store) *Service {
	return &Service{store: store, clock: realClock{}, ids: randomIDs{}, jobDetailCalls: map[string]*jobDetailCall{}}
}
func NewServiceWithDependencies(store Store, clock Clock, ids IDGenerator) *Service {
	return &Service{store: store, clock: clock, ids: ids, jobDetailCalls: map[string]*jobDetailCall{}}
}

func requireMeta(m Meta, roles ...string) error {
	if strings.TrimSpace(m.IdempotencyKey) == "" {
		return domain.Invalid("idempotencyKey", "不能为空")
	}
	if strings.TrimSpace(m.Actor) == "" {
		return domain.Invalid("actor", "不能为空")
	}
	for _, r := range roles {
		if m.Role == r {
			return nil
		}
	}
	return domain.ErrForbidden
}
func (s *Service) prior(ctx context.Context, key string) (CommitResult, bool) {
	return s.store.IdempotentResult(ctx, key)
}
func getJob(snap Snapshot, id string) (domain.DigitizationJob, error) {
	v, ok := snap.Jobs[id]
	if !ok {
		return v, domain.ErrNotFound
	}
	return v, nil
}
func requireMutable(job domain.DigitizationJob) error { return domain.Mutable(job) }

func (s *Service) CreateJob(ctx context.Context, c CreateJobCommand) (CommitResult, error) {
	if err := requireMeta(c.Meta, "operator", "reviewer", "manager"); err != nil {
		return CommitResult{}, err
	}
	if prior, ok := s.prior(ctx, c.IdempotencyKey); ok {
		return prior, nil
	}
	if strings.TrimSpace(c.Title) == "" {
		return CommitResult{}, domain.Invalid("title", "不能为空")
	}
	if strings.TrimSpace(c.CollectionRef) == "" {
		return CommitResult{}, domain.Invalid("collectionRef", "不能为空")
	}
	if err := domain.ValidateProfile(c.Profile); err != nil {
		return CommitResult{}, err
	}
	now := s.clock.Now()
	id := s.ids.NewID("job")
	job := domain.DigitizationJob{ID: id, Title: c.Title, CollectionRef: c.CollectionRef, Status: domain.StatusDraft, CaptureProfile: c.Profile, Version: 1, CreatedAt: now, UpdatedAt: now}
	r := CommitResult{JobID: id, Version: 1, Status: job.Status, ResourceID: id}
	return s.store.Commit(ctx, id, 0, c.IdempotencyKey, []Event{NewEvent("job.created", id, c.Actor, now, job)}, r)
}

func (s *Service) AddCarrier(ctx context.Context, c AddCarrierCommand) (CommitResult, error) {
	if err := requireMeta(c.Meta, "operator"); err != nil {
		return CommitResult{}, err
	}
	if p, ok := s.prior(ctx, c.IdempotencyKey); ok {
		return p, nil
	}
	snap := s.store.Snapshot(ctx)
	job, err := getJob(snap, c.JobID)
	if err != nil {
		return CommitResult{}, err
	}
	if err = requireMutable(job); err != nil {
		return CommitResult{}, err
	}
	if job.Status != domain.StatusDraft && job.Status != domain.StatusReady {
		return CommitResult{}, domain.ErrInvalidTransition
	}
	carrier := domain.TapeCarrier{ID: s.ids.NewID("tape"), JobID: c.JobID, CarrierCode: c.CarrierCode, Format: c.Format, ExpectedDurationMS: c.ExpectedDurationMS, ConditionGrade: c.ConditionGrade, CleaningRequired: c.CleaningRequired, AssessmentNote: c.AssessmentNote}
	if err = domain.ValidateCarrier(carrier); err != nil {
		return CommitResult{}, err
	}
	for _, v := range snap.JobCarriers(c.JobID) {
		if v.CarrierCode == carrier.CarrierCode {
			return CommitResult{}, domain.Invalid("carrierCode", "同一作业内必须唯一")
		}
	}
	job.Version++
	job.UpdatedAt = s.clock.Now()
	events := []Event{NewEvent("carrier.added", c.JobID, c.Actor, job.UpdatedAt, carrier)}
	if job.Status == domain.StatusReady {
		job.Status = domain.StatusDraft
		events = append(events, NewEvent("preflight.invalidated", c.JobID, c.Actor, job.UpdatedAt, struct{}{}))
	}
	events = append(events, NewEvent("job.status_changed", c.JobID, c.Actor, job.UpdatedAt, job))
	r := CommitResult{JobID: c.JobID, Version: job.Version, Status: job.Status, ResourceID: carrier.ID}
	return s.store.Commit(ctx, c.JobID, c.ExpectedVersion, c.IdempotencyKey, events, r)
}

func (s *Service) CompletePreflight(ctx context.Context, c CompletePreflightCommand) (CommitResult, error) {
	if err := requireMeta(c.Meta, "operator"); err != nil {
		return CommitResult{}, err
	}
	if p, ok := s.prior(ctx, c.IdempotencyKey); ok {
		return p, nil
	}
	snap := s.store.Snapshot(ctx)
	job, err := getJob(snap, c.JobID)
	if err != nil {
		return CommitResult{}, err
	}
	if err = requireMutable(job); err != nil {
		return CommitResult{}, err
	}
	if job.Status != domain.StatusDraft {
		return CommitResult{}, domain.ErrInvalidTransition
	}
	carriers := snap.JobCarriers(c.JobID)
	if len(carriers) == 0 {
		return CommitResult{}, domain.Invalid("carriers", "至少登记一盘载体")
	}
	inputs := c.CarrierChecks
	if len(inputs) == 0 && c.CarrierCleaned {
		for _, carrier := range carriers {
			inputs = append(inputs, CarrierPreflightInput{CarrierID: carrier.ID, CleaningCompleted: true, AppearancePassed: true, PlaybackCompatible: true, DispositionNote: "兼容旧版作业级前检", DispositionCompleted: true})
		}
	}
	carrierChecks := make([]domain.CarrierPreflightCheck, 0, len(inputs))
	for _, input := range inputs {
		carrierChecks = append(carrierChecks, domain.CarrierPreflightCheck{CarrierID: input.CarrierID, CleaningCompleted: input.CleaningCompleted, AppearancePassed: input.AppearancePassed, PlaybackCompatible: input.PlaybackCompatible, DispositionNote: input.DispositionNote, DispositionCompleted: input.DispositionCompleted})
	}
	now := s.clock.Now()
	check, err := domain.EvaluatePreflight(carriers, carrierChecks, c.PlaybackCalibrated, c.StorageAvailable, c.Actor, now)
	if err != nil {
		return CommitResult{}, err
	}
	if check.Ready {
		if err = domain.Transition(&job, domain.StatusReady); err != nil {
			return CommitResult{}, err
		}
	}
	job.Version++
	job.UpdatedAt = now
	data := struct {
		JobID string                `json:"jobId"`
		Check domain.PreflightCheck `json:"check"`
	}{c.JobID, check}
	events := []Event{NewEvent("preflight.completed", c.JobID, c.Actor, check.CheckedAt, data), NewEvent("job.status_changed", c.JobID, c.Actor, check.CheckedAt, job)}
	r := CommitResult{JobID: c.JobID, Version: job.Version, Status: job.Status, Preflight: &check}
	return s.store.Commit(ctx, c.JobID, c.ExpectedVersion, c.IdempotencyKey, events, r)
}

func latestValidForCarrier(s Snapshot, carrierID string) (domain.CaptureTake, bool) {
	var found domain.CaptureTake
	ok := false
	for _, v := range s.Captures {
		if v.CarrierID == carrierID && v.Status == domain.CaptureValid && (!ok || v.Sequence > found.Sequence) {
			found = v
			ok = true
		}
	}
	return found, ok
}
func latestForCarrier(s Snapshot, carrierID string) (domain.CaptureTake, bool) {
	var found domain.CaptureTake
	ok := false
	for _, v := range s.Captures {
		if v.CarrierID == carrierID && (!ok || v.Sequence > found.Sequence) {
			found = v
			ok = true
		}
	}
	return found, ok
}
func allCovered(s Snapshot, jobID string) ([]domain.CaptureTake, bool) {
	var takes []domain.CaptureTake
	for _, c := range s.JobCarriers(jobID) {
		t, ok := latestValidForCarrier(s, c.ID)
		if !ok {
			return nil, false
		}
		takes = append(takes, t)
	}
	return takes, len(takes) > 0
}

func remediationRound(s Snapshot, carrierID string) int {
	round := 0
	for _, remediation := range s.Remediations {
		if remediation.CarrierID == carrierID && remediation.Round > round {
			round = remediation.Round
		}
	}
	return round
}

func relatedOpenFindings(s Snapshot, jobID, carrierID string) []domain.QualityFinding {
	var out []domain.QualityFinding
	for _, finding := range s.JobFindings(jobID) {
		if finding.CarrierID == carrierID && finding.Status != domain.FindingClosed {
			out = append(out, finding)
		}
	}
	return out
}

func findRemediation(s Snapshot, id string) (domain.RemediationSubmission, bool) {
	v, ok := s.Remediations[id]
	return v, ok
}

func (s *Service) RegisterCapture(ctx context.Context, c RegisterCaptureCommand) (CommitResult, error) {
	if err := requireMeta(c.Meta, "operator"); err != nil {
		return CommitResult{}, err
	}
	if p, ok := s.prior(ctx, c.IdempotencyKey); ok {
		return p, nil
	}
	snap := s.store.Snapshot(ctx)
	job, err := getJob(snap, c.JobID)
	if err != nil {
		return CommitResult{}, err
	}
	if err = requireMutable(job); err != nil {
		return CommitResult{}, err
	}
	if job.Status != domain.StatusReady && job.Status != domain.StatusCapturing && job.Status != domain.StatusRemediation {
		return CommitResult{}, domain.ErrInvalidTransition
	}
	carrier, ok := snap.Carriers[c.CarrierID]
	if !ok || carrier.JobID != c.JobID {
		return CommitResult{}, domain.Invalid("carrierId", "载体不属于该作业")
	}
	seq := 1
	latest, hasHistory := latestForCarrier(snap, c.CarrierID)
	old, hasActive := latestValidForCarrier(snap, c.CarrierID)
	if hasHistory {
		seq = latest.Sequence + 1
		if hasActive {
			if c.SupersedesID == "" {
				return CommitResult{}, domain.Invalid("supersedesId", "已有有效采集时必须指定被替代版本")
			}
			if old.ID != latest.ID || c.SupersedesID != old.ID {
				return CommitResult{}, domain.InvalidBecause("supersedesId", "只能指向该载体最新有效版本", domain.ErrLineageConflict)
			}
		} else if c.SupersedesID != "" {
			return CommitResult{}, domain.InvalidBecause("supersedesId", "当前无有效采集，重新采集不能指向已作废版本", domain.ErrLineageConflict)
		}
	} else if c.SupersedesID != "" {
		return CommitResult{}, domain.InvalidBecause("supersedesId", "首次采集不能指定替代版本", domain.ErrLineageConflict)
	}
	now := s.clock.Now()
	filename, err := domain.RenderCaptureFilename(job.CaptureProfile.NamingRule, carrier.CarrierCode, job.CollectionRef, seq)
	if err != nil {
		return CommitResult{}, err
	}
	take := domain.CaptureTake{ID: s.ids.NewID("take"), JobID: c.JobID, CarrierID: c.CarrierID, Sequence: seq, SampleRate: c.SampleRate, BitDepth: c.BitDepth, Channels: c.Channels, DurationMS: c.DurationMS, SHA256: strings.ToLower(c.SHA256), Operator: c.Operator, Status: domain.CaptureValid, SupersedesID: c.SupersedesID, ContentSummary: c.ContentSummary, Filename: filename, Metrics: c.Metrics, ResolutionNote: strings.TrimSpace(c.ResolutionNote), CreatedAt: now}
	if err = domain.ValidateCapture(take); err != nil {
		return CommitResult{}, err
	}
	if hasActive && take.SHA256 == old.SHA256 {
		return CommitResult{}, domain.InvalidBecause("sha256", "替代版本的内容摘要不能与被替代版本相同", domain.ErrLineageConflict)
	}
	for _, existing := range snap.JobCaptures(c.JobID) {
		if existing.Status != domain.CaptureValid || existing.ID == c.SupersedesID {
			continue
		}
		otherCarrier := snap.Carriers[existing.CarrierID]
		if existing.SHA256 == take.SHA256 {
			return CommitResult{}, domain.InvalidBecause("sha256", fmt.Sprintf("与采集 %s（载体 %s）重复", existing.ID, otherCarrier.CarrierCode), domain.ErrDuplicateCapture)
		}
		if strings.EqualFold(strings.TrimSpace(existing.Filename), strings.TrimSpace(take.Filename)) {
			return CommitResult{}, domain.InvalidBecause("filename", fmt.Sprintf("与采集 %s（载体 %s）的规范化母版文件名碰撞", existing.ID, otherCarrier.CarrierCode), domain.ErrFilenameCollision)
		}
	}
	events := []Event{}
	if take.SupersedesID != "" {
		old.Status = domain.CaptureVoided
		old.VoidReason = "由采集 " + take.ID + " 替代"
		events = append(events, NewEvent("capture.voided", c.JobID, c.Actor, now, old))
	}
	events = append(events, NewEvent("capture.registered", c.JobID, c.Actor, now, take))
	evaluation := domain.EvaluateCapture(job, carrier, take, now, func() string { return s.ids.NewID("finding") })
	for _, f := range evaluation.Findings {
		events = append(events, NewEvent("finding.created", c.JobID, c.Actor, now, f))
	}
	if hasHistory {
		related := relatedOpenFindings(snap, c.JobID, c.CarrierID)
		round := remediationRound(snap, c.CarrierID) + 1
		previous := latest
		if hasActive {
			previous = old
		}
		remediation, remediationErr := domain.NewRemediation(s.ids.NewID("remediation"), previous, take, c.ResolutionNote, c.Actor, now, round, related, evaluation.Findings)
		if remediationErr != nil {
			return CommitResult{}, remediationErr
		}
		events = append(events, NewEvent("remediation.submitted", c.JobID, c.Actor, now, remediation))
		for _, finding := range related {
			finding.Status = domain.FindingPending
			finding.CurrentCaptureTakeID = take.ID
			finding.RemediationRound = round
			finding.RemediationID = remediation.ID
			events = append(events, NewEvent("finding.remediation_submitted", c.JobID, c.Actor, now, finding))
		}
	}
	if job.Status == domain.StatusReady {
		_ = domain.Transition(&job, domain.StatusCapturing)
	}
	if len(evaluation.Findings) > 0 && job.Status == domain.StatusCapturing {
		_ = domain.Transition(&job, domain.StatusRemediation)
	}
	job.Version++
	job.UpdatedAt = now
	events = append(events, NewEvent("job.status_changed", c.JobID, c.Actor, now, job))
	r := CommitResult{JobID: c.JobID, Version: job.Version, Status: job.Status, ResourceID: take.ID}
	return s.store.Commit(ctx, c.JobID, c.ExpectedVersion, c.IdempotencyKey, events, r)
}

func (s *Service) VoidCapture(ctx context.Context, c VoidCaptureCommand) (CommitResult, error) {
	if err := requireMeta(c.Meta, "operator"); err != nil {
		return CommitResult{}, err
	}
	if p, ok := s.prior(ctx, c.IdempotencyKey); ok {
		return p, nil
	}
	snap := s.store.Snapshot(ctx)
	job, err := getJob(snap, c.JobID)
	if err != nil {
		return CommitResult{}, err
	}
	if err = requireMutable(job); err != nil {
		return CommitResult{}, err
	}
	take, ok := snap.Captures[c.CaptureID]
	if !ok || take.JobID != c.JobID {
		return CommitResult{}, domain.ErrNotFound
	}
	if take.Status == domain.CaptureVoided {
		return CommitResult{}, domain.Invalid("captureId", "采集已作废")
	}
	if strings.TrimSpace(c.Reason) == "" {
		return CommitResult{}, domain.Invalid("reason", "不能为空")
	}
	take.Status = domain.CaptureVoided
	take.VoidReason = strings.TrimSpace(c.Reason)
	now := s.clock.Now()
	if job.Status == domain.StatusPendingApproval {
		if transitionErr := domain.Transition(&job, domain.StatusRemediation); transitionErr != nil {
			return CommitResult{}, transitionErr
		}
	}
	job.Version++
	job.UpdatedAt = now
	events := []Event{NewEvent("capture.voided", c.JobID, c.Actor, now, take), NewEvent("job.status_changed", c.JobID, c.Actor, now, job)}
	r := CommitResult{JobID: c.JobID, Version: job.Version, Status: job.Status, ResourceID: take.ID}
	return s.store.Commit(ctx, c.JobID, c.ExpectedVersion, c.IdempotencyKey, events, r)
}

func (s *Service) AddManualFinding(ctx context.Context, c AddFindingCommand) (CommitResult, error) {
	if err := requireMeta(c.Meta, "reviewer"); err != nil {
		return CommitResult{}, err
	}
	if p, ok := s.prior(ctx, c.IdempotencyKey); ok {
		return p, nil
	}
	snap := s.store.Snapshot(ctx)
	job, err := getJob(snap, c.JobID)
	if err != nil {
		return CommitResult{}, err
	}
	if err = requireMutable(job); err != nil {
		return CommitResult{}, err
	}
	take, ok := snap.Captures[c.CaptureTakeID]
	if !ok || take.JobID != c.JobID {
		return CommitResult{}, domain.ErrNotFound
	}
	now := s.clock.Now()
	f := domain.QualityFinding{ID: s.ids.NewID("finding"), JobID: c.JobID, CaptureTakeID: c.CaptureTakeID, CarrierID: take.CarrierID, CurrentCaptureTakeID: c.CaptureTakeID, Source: "manual", RuleCode: "MANUAL_REVIEW", Severity: c.Severity, StartMS: c.StartMS, EndMS: c.EndMS, Description: c.Description, Status: domain.FindingOpen, CreatedAt: now}
	if err = domain.ValidateFinding(f); err != nil {
		return CommitResult{}, err
	}
	if f.EndMS > take.DurationMS {
		return CommitResult{}, domain.Invalid("timecode", "发现时间码超出采集时长")
	}
	if job.Status == domain.StatusCapturing {
		_ = domain.Transition(&job, domain.StatusRemediation)
	}
	job.Version++
	job.UpdatedAt = now
	events := []Event{NewEvent("finding.created", c.JobID, c.Actor, now, f), NewEvent("job.status_changed", c.JobID, c.Actor, now, job)}
	r := CommitResult{JobID: c.JobID, Version: job.Version, Status: job.Status, ResourceID: f.ID}
	return s.store.Commit(ctx, c.JobID, c.ExpectedVersion, c.IdempotencyKey, events, r)
}

func (s *Service) ReviewFinding(ctx context.Context, c ReviewFindingCommand) (CommitResult, error) {
	if err := requireMeta(c.Meta, "reviewer"); err != nil {
		return CommitResult{}, err
	}
	if p, ok := s.prior(ctx, c.IdempotencyKey); ok {
		return p, nil
	}
	snap := s.store.Snapshot(ctx)
	job, err := getJob(snap, c.JobID)
	if err != nil {
		return CommitResult{}, err
	}
	if err = requireMutable(job); err != nil {
		return CommitResult{}, err
	}
	f, ok := snap.Findings[c.FindingID]
	if !ok || f.JobID != c.JobID {
		return CommitResult{}, domain.ErrNotFound
	}
	if f.Status == domain.FindingClosed {
		return CommitResult{}, domain.Invalid("findingId", "发现已经复核")
	}
	if strings.TrimSpace(c.ResolutionNote) == "" {
		return CommitResult{}, domain.Invalid("resolutionNote", "不能为空")
	}
	if c.Decision != "close" && c.Decision != "reject" {
		return CommitResult{}, domain.Invalid("decision", "必须是 close 或 reject")
	}
	if f.Status != domain.FindingPending {
		return CommitResult{}, domain.Invalid("findingId", "发现尚无可复核的替代采集")
	}
	if c.RemediationRound != f.RemediationRound {
		return CommitResult{}, domain.Invalid("remediationRound", fmt.Sprintf("当前整改轮次为 %d", f.RemediationRound))
	}
	if c.CarrierID != "" && c.CarrierID != f.CarrierID {
		return CommitResult{}, domain.Invalid("carrierId", "复核载体与发现关联载体不一致")
	}
	if c.CaptureTakeID != "" && c.CaptureTakeID != f.CurrentCaptureTakeID {
		return CommitResult{}, domain.Invalid("captureTakeId", "复核采集版本与当前整改版本不一致")
	}
	current, ok := snap.Captures[f.CurrentCaptureTakeID]
	if !ok || current.CarrierID != f.CarrierID || current.Status != domain.CaptureValid {
		return CommitResult{}, domain.Invalid("captureTakeId", "当前整改采集无效或关联载体不一致")
	}
	active, activeOK := latestValidForCarrier(snap, f.CarrierID)
	if !activeOK || active.ID != current.ID {
		return CommitResult{}, domain.Invalid("captureTakeId", "复核采集不是该载体最新有效版本")
	}
	remediation, ok := findRemediation(snap, f.RemediationID)
	if !ok || remediation.Round != f.RemediationRound || remediation.ReplacementCaptureID != current.ID {
		return CommitResult{}, domain.Invalid("remediationRound", "整改记录与当前轮次不一致")
	}
	if c.Decision == "close" && f.Source == "rule" {
		passed := false
		found := false
		for _, result := range remediation.RerunResults {
			if result.RuleCode == f.RuleCode {
				found = true
				passed = result.Passed
				break
			}
		}
		if !found {
			return CommitResult{}, domain.Invalid("decision", "规则发现缺少关联替代版本的复跑结果")
		}
		if !passed {
			return CommitResult{}, domain.Invalid("decision", "关联替代版本的同一规则仍未通过")
		}
	}
	if f.EndMS > current.DurationMS {
		return CommitResult{}, domain.Invalid("timecode", "发现时间码超出当前整改采集时长")
	}
	if c.Decision == "close" {
		f.Status = domain.FindingClosed
	} else {
		f.Status = domain.FindingRejected
		if job.Status == domain.StatusPendingApproval {
			if transitionErr := domain.Transition(&job, domain.StatusRemediation); transitionErr != nil {
				return CommitResult{}, transitionErr
			}
		}
	}
	now := s.clock.Now()
	f.ResolutionNote = c.ResolutionNote
	f.Reviewer = c.Actor
	f.ReviewedAt = &now
	f.ReviewHistory = append(f.ReviewHistory, domain.FindingReviewRecord{Round: f.RemediationRound, Decision: c.Decision, Note: strings.TrimSpace(c.ResolutionNote), Reviewer: c.Actor, CaptureID: current.ID, ReviewedAt: now})
	job.Version++
	job.UpdatedAt = now
	events := []Event{NewEvent("finding.reviewed", c.JobID, c.Actor, now, f), NewEvent("job.status_changed", c.JobID, c.Actor, now, job)}
	r := CommitResult{JobID: c.JobID, Version: job.Version, Status: job.Status, ResourceID: f.ID}
	return s.store.Commit(ctx, c.JobID, c.ExpectedVersion, c.IdempotencyKey, events, r)
}

func (s *Service) SubmitApproval(ctx context.Context, c SubmitApprovalCommand) (CommitResult, error) {
	if err := requireMeta(c.Meta, "reviewer"); err != nil {
		return CommitResult{}, err
	}
	if p, ok := s.prior(ctx, c.IdempotencyKey); ok {
		return p, nil
	}
	snap := s.store.Snapshot(ctx)
	job, err := getJob(snap, c.JobID)
	if err != nil {
		return CommitResult{}, err
	}
	if job.Status != domain.StatusCapturing && job.Status != domain.StatusRemediation {
		return CommitResult{}, domain.ErrInvalidTransition
	}
	if _, ok := allCovered(snap, c.JobID); !ok {
		return CommitResult{}, domain.ErrIncompleteCoverage
	}
	if domain.HasOpenSevere(snap.JobFindings(c.JobID)) {
		return CommitResult{}, domain.ErrOpenSevereFinding
	}
	if err = domain.Transition(&job, domain.StatusPendingApproval); err != nil {
		return CommitResult{}, err
	}
	now := s.clock.Now()
	job.Version++
	job.UpdatedAt = now
	events := []Event{NewEvent("job.status_changed", c.JobID, c.Actor, now, job)}
	r := CommitResult{JobID: c.JobID, Version: job.Version, Status: job.Status}
	return s.store.Commit(ctx, c.JobID, c.ExpectedVersion, c.IdempotencyKey, events, r)
}

func (s *Service) FreezeManifest(ctx context.Context, c FreezeManifestCommand) (CommitResult, error) {
	if err := requireMeta(c.Meta, "manager"); err != nil {
		return CommitResult{}, err
	}
	if p, ok := s.prior(ctx, c.IdempotencyKey); ok {
		return p, nil
	}
	snap := s.store.Snapshot(ctx)
	job, err := getJob(snap, c.JobID)
	if err != nil {
		return CommitResult{}, err
	}
	previewVersion := c.PreviewVersion
	previewDigest := c.PreviewDigest
	if previewVersion == 0 {
		previewVersion = c.PreflightVersion
	}
	if previewDigest == "" {
		previewDigest = c.PreflightDigest
	}
	revision := 1
	if old, exists := snap.Manifests[c.JobID]; exists {
		revision = old.Revision + 1
	}
	preview := domain.BuildManifestPreview(job, snap.JobCarriers(c.JobID), snap.JobCaptures(c.JobID), snap.JobFindings(c.JobID), revision)
	if previewVersion != job.Version || previewDigest == "" || previewDigest != preview.ProposedDigest {
		return CommitResult{}, domain.ErrStalePreview
	}
	if job.Status != domain.StatusPendingApproval {
		return CommitResult{}, domain.ErrInvalidTransition
	}
	if !preview.CanFreeze {
		return CommitResult{}, domain.ErrManifestBlocked
	}
	takes, ok := allCovered(snap, c.JobID)
	if !ok {
		return CommitResult{}, domain.ErrIncompleteCoverage
	}
	now := s.clock.Now()
	manifest := domain.BuildManifest(c.JobID, revision, takes, c.Actor, now, s.ids.NewID("manifest"))
	if manifest.ManifestDigest != preview.ProposedDigest {
		return CommitResult{}, domain.ErrStalePreview
	}
	if err = domain.Transition(&job, domain.StatusFrozen); err != nil {
		return CommitResult{}, err
	}
	job.Version++
	job.UpdatedAt = now
	events := []Event{NewEvent("manifest.frozen", c.JobID, c.Actor, now, manifest), NewEvent("job.status_changed", c.JobID, c.Actor, now, job)}
	r := CommitResult{JobID: c.JobID, Version: job.Version, Status: job.Status, ResourceID: manifest.ID}
	return s.store.Commit(ctx, c.JobID, c.ExpectedVersion, c.IdempotencyKey, events, r)
}

func (s *Service) IssueCertificate(ctx context.Context, c IssueCertificateCommand) (CommitResult, error) {
	if err := requireMeta(c.Meta, "manager"); err != nil {
		return CommitResult{}, err
	}
	if p, ok := s.prior(ctx, c.IdempotencyKey); ok {
		return p, nil
	}
	snap := s.store.Snapshot(ctx)
	job, err := getJob(snap, c.JobID)
	if err != nil {
		return CommitResult{}, err
	}
	if job.Status == domain.StatusCertified {
		for _, cert := range snap.Certificates {
			if cert.JobID == c.JobID {
				return CommitResult{JobID: c.JobID, Version: job.Version, Status: job.Status, ResourceID: cert.CertificateNo}, nil
			}
		}
	}
	if job.Status != domain.StatusFrozen {
		return CommitResult{}, domain.ErrInvalidTransition
	}
	manifest, ok := snap.Manifests[c.JobID]
	if !ok {
		return CommitResult{}, domain.ErrNotFound
	}
	now := s.clock.Now()
	number := fmt.Sprintf("TMG-%s-%s", now.Format("20060102"), strings.ToUpper(s.ids.NewID("")[0:8]))
	cert := domain.BuildCertificate(manifest, c.Actor, now, number)
	if err = domain.Transition(&job, domain.StatusCertified); err != nil {
		return CommitResult{}, err
	}
	job.Version++
	job.UpdatedAt = now
	events := []Event{NewEvent("certificate.issued", c.JobID, c.Actor, now, cert), NewEvent("job.status_changed", c.JobID, c.Actor, now, job)}
	r := CommitResult{JobID: c.JobID, Version: job.Version, Status: job.Status, ResourceID: cert.CertificateNo}
	return s.store.Commit(ctx, c.JobID, c.ExpectedVersion, c.IdempotencyKey, events, r)
}
