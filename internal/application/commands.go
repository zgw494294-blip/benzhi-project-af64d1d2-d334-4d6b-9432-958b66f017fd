package application

import "tapemastergate/internal/domain"

type Meta struct {
	ExpectedVersion int64  `json:"expectedVersion"`
	IdempotencyKey  string `json:"idempotencyKey"`
	Actor           string `json:"actor"`
	Role            string `json:"role"`
}
type CreateJobCommand struct {
	Meta
	Title, CollectionRef string
	Profile              domain.CaptureProfile
}
type AddCarrierCommand struct {
	Meta
	JobID, CarrierCode, Format string
	ExpectedDurationMS         int64
	ConditionGrade             string
	CleaningRequired           bool
	AssessmentNote             string
}
type CompletePreflightCommand struct {
	Meta
	JobID                                string
	PlaybackCalibrated, StorageAvailable bool
	CarrierChecks                        []CarrierPreflightInput
	CarrierCleaned                       bool
}
type CarrierPreflightInput struct {
	CarrierID            string `json:"carrierId"`
	CleaningCompleted    bool   `json:"cleaningCompleted"`
	AppearancePassed     bool   `json:"appearancePassed"`
	PlaybackCompatible   bool   `json:"playbackCompatible"`
	DispositionNote      string `json:"dispositionNote"`
	DispositionCompleted bool   `json:"dispositionCompleted"`
}
type RegisterCaptureCommand struct {
	Meta
	JobID, CarrierID, SHA256, Operator, SupersedesID, ContentSummary, ResolutionNote string
	SampleRate, BitDepth, Channels                                                   int
	DurationMS                                                                       int64
	Metrics                                                                          domain.CaptureMetrics
}
type VoidCaptureCommand struct {
	Meta
	JobID, CaptureID, Reason string
}
type AddFindingCommand struct {
	Meta
	JobID, CaptureTakeID string
	Severity             domain.Severity
	StartMS, EndMS       int64
	Description          string
}
type ReviewFindingCommand struct {
	Meta
	JobID, FindingID, Decision, ResolutionNote string
	CarrierID, CaptureTakeID                   string
	RemediationRound                           int
}
type SubmitApprovalCommand struct {
	Meta
	JobID string
}
type FreezeManifestCommand struct {
	Meta
	JobID            string
	PreviewVersion   int64  `json:"previewVersion"`
	PreviewDigest    string `json:"previewDigest"`
	PreflightVersion int64  `json:"preflightVersion,omitempty"`
	PreflightDigest  string `json:"preflightDigest,omitempty"`
}
type IssueCertificateCommand struct {
	Meta
	JobID string
}
