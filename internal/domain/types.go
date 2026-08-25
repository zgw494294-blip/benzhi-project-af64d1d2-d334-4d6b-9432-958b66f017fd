package domain

import "time"

type JobStatus string

const (
	StatusDraft           JobStatus = "draft"
	StatusReady           JobStatus = "ready"
	StatusCapturing       JobStatus = "capturing"
	StatusRemediation     JobStatus = "remediation"
	StatusPendingApproval JobStatus = "pending_approval"
	StatusFrozen          JobStatus = "frozen"
	StatusCertified       JobStatus = "certified"
)

type CaptureStatus string

const (
	CaptureValid  CaptureStatus = "valid"
	CaptureVoided CaptureStatus = "voided"
)

type FindingStatus string

const (
	FindingOpen     FindingStatus = "open"
	FindingPending  FindingStatus = "pending_review"
	FindingClosed   FindingStatus = "closed"
	FindingRejected FindingStatus = "rejected"
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityMinor    Severity = "minor"
	SeverityMajor    Severity = "major"
	SeverityCritical Severity = "critical"
)

type CaptureProfile struct {
	SampleRate int    `json:"sampleRate"`
	BitDepth   int    `json:"bitDepth"`
	Channels   int    `json:"channels"`
	NamingRule string `json:"namingRule"`
}

type DigitizationJob struct {
	ID             string         `json:"id"`
	Title          string         `json:"title"`
	CollectionRef  string         `json:"collectionRef"`
	Status         JobStatus      `json:"status"`
	CaptureProfile CaptureProfile `json:"captureProfile"`
	Version        int64          `json:"version"`
	CreatedAt      time.Time      `json:"createdAt"`
	UpdatedAt      time.Time      `json:"updatedAt"`
}

type TapeCarrier struct {
	ID                 string `json:"id"`
	JobID              string `json:"jobId"`
	CarrierCode        string `json:"carrierCode"`
	Format             string `json:"format"`
	ExpectedDurationMS int64  `json:"expectedDurationMs"`
	ConditionGrade     string `json:"conditionGrade"`
	CleaningRequired   bool   `json:"cleaningRequired"`
	AssessmentNote     string `json:"assessmentNote"`
}

type PreflightCheck struct {
	PlaybackCalibrated bool                    `json:"playbackCalibrated"`
	StorageAvailable   bool                    `json:"storageAvailable"`
	CheckedBy          string                  `json:"checkedBy"`
	CheckedAt          time.Time               `json:"checkedAt"`
	CarrierChecks      []CarrierPreflightCheck `json:"carrierChecks"`
	Ready              bool                    `json:"ready"`
	Blockers           []PreflightBlocker      `json:"blockers"`
}

type CarrierPreflightCheck struct {
	CarrierID            string    `json:"carrierId"`
	CarrierCode          string    `json:"carrierCode"`
	CleaningCompleted    bool      `json:"cleaningCompleted"`
	AppearancePassed     bool      `json:"appearancePassed"`
	PlaybackCompatible   bool      `json:"playbackCompatible"`
	DispositionNote      string    `json:"dispositionNote,omitempty"`
	DispositionCompleted bool      `json:"dispositionCompleted"`
	CheckedBy            string    `json:"checkedBy"`
	CheckedAt            time.Time `json:"checkedAt"`
	Passed               bool      `json:"passed"`
	Blockers             []string  `json:"blockers"`
}

type PreflightBlocker struct {
	Code        string `json:"code"`
	CarrierID   string `json:"carrierId,omitempty"`
	CarrierCode string `json:"carrierCode,omitempty"`
	Message     string `json:"message"`
}

type CaptureMetrics struct {
	PeakDBFS         float64 `json:"peakDbfs"`
	DropoutCount     int     `json:"dropoutCount"`
	LongestSilenceMS int64   `json:"longestSilenceMs"`
}

type CaptureTake struct {
	ID             string         `json:"id"`
	JobID          string         `json:"jobId"`
	CarrierID      string         `json:"carrierId"`
	Sequence       int            `json:"sequence"`
	SampleRate     int            `json:"sampleRate"`
	BitDepth       int            `json:"bitDepth"`
	Channels       int            `json:"channels"`
	DurationMS     int64          `json:"durationMs"`
	SHA256         string         `json:"sha256"`
	Operator       string         `json:"operator"`
	Status         CaptureStatus  `json:"status"`
	SupersedesID   string         `json:"supersedesId,omitempty"`
	ContentSummary string         `json:"contentSummary"`
	Filename       string         `json:"filename"`
	Metrics        CaptureMetrics `json:"metrics"`
	VoidReason     string         `json:"voidReason,omitempty"`
	ResolutionNote string         `json:"resolutionNote,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
}

type FindingReviewRecord struct {
	Round      int       `json:"round"`
	Decision   string    `json:"decision"`
	Note       string    `json:"note"`
	Reviewer   string    `json:"reviewer"`
	CaptureID  string    `json:"captureId"`
	ReviewedAt time.Time `json:"reviewedAt"`
}

type QualityFinding struct {
	ID                   string                `json:"id"`
	JobID                string                `json:"jobId"`
	CaptureTakeID        string                `json:"captureTakeId"`
	CarrierID            string                `json:"carrierId"`
	CurrentCaptureTakeID string                `json:"currentCaptureTakeId"`
	Source               string                `json:"source"`
	RuleCode             string                `json:"ruleCode"`
	Severity             Severity              `json:"severity"`
	StartMS              int64                 `json:"startMs"`
	EndMS                int64                 `json:"endMs"`
	Description          string                `json:"description"`
	Status               FindingStatus         `json:"status"`
	ResolutionNote       string                `json:"resolutionNote,omitempty"`
	Reviewer             string                `json:"reviewer,omitempty"`
	CreatedAt            time.Time             `json:"createdAt"`
	ReviewedAt           *time.Time            `json:"reviewedAt,omitempty"`
	RemediationRound     int                   `json:"remediationRound"`
	RemediationID        string                `json:"remediationId,omitempty"`
	ReviewHistory        []FindingReviewRecord `json:"reviewHistory"`
}

type CaptureLineageEntry struct {
	CaptureTake
	SupersededByID string                 `json:"supersededById,omitempty"`
	CurrentMaster  bool                   `json:"currentMaster"`
	Remediation    *RemediationSubmission `json:"remediation,omitempty"`
}

type CarrierCaptureLineage struct {
	CarrierID       string                `json:"carrierId"`
	CarrierCode     string                `json:"carrierCode"`
	HasActiveMaster bool                  `json:"hasActiveMaster"`
	CurrentMasterID string                `json:"currentMasterId,omitempty"`
	Versions        []CaptureLineageEntry `json:"versions"`
}

type DeliveryManifest struct {
	ID             string    `json:"id"`
	JobID          string    `json:"jobId"`
	Revision       int       `json:"revision"`
	CaptureTakeIDs []string  `json:"captureTakeIds"`
	ManifestDigest string    `json:"manifestDigest"`
	ApprovedBy     string    `json:"approvedBy"`
	FrozenAt       time.Time `json:"frozenAt"`
}

type ManifestPreviewItem struct {
	CarrierID   string `json:"carrierId"`
	CarrierCode string `json:"carrierCode"`
	CaptureID   string `json:"captureId"`
	Sequence    int    `json:"sequence"`
	Filename    string `json:"filename"`
	SHA256      string `json:"sha256"`
	SampleRate  int    `json:"sampleRate"`
	BitDepth    int    `json:"bitDepth"`
	Channels    int    `json:"channels"`
}

type ManifestBlocker struct {
	Code        string `json:"code"`
	CarrierID   string `json:"carrierId,omitempty"`
	CarrierCode string `json:"carrierCode,omitempty"`
	CaptureID   string `json:"captureId,omitempty"`
	FindingID   string `json:"findingId,omitempty"`
	Message     string `json:"message"`
}

type ManifestPreview struct {
	JobID            string                `json:"jobId"`
	JobVersion       int64                 `json:"jobVersion"`
	ProposedRevision int                   `json:"proposedRevision"`
	ProposedDigest   string                `json:"proposedDigest"`
	Items            []ManifestPreviewItem `json:"items"`
	Blockers         []ManifestBlocker     `json:"blockers"`
	CanFreeze        bool                  `json:"canFreeze"`
}

type ReleaseCertificate struct {
	CertificateNo    string    `json:"certificateNo"`
	JobID            string    `json:"jobId"`
	ManifestID       string    `json:"manifestId"`
	ManifestDigest   string    `json:"manifestDigest"`
	IssuedBy         string    `json:"issuedBy"`
	IssuedAt         time.Time `json:"issuedAt"`
	VerificationCode string    `json:"verificationCode"`
}

type AuditEntry struct {
	Sequence int64     `json:"sequence"`
	JobID    string    `json:"jobId"`
	Action   string    `json:"action"`
	Actor    string    `json:"actor"`
	At       time.Time `json:"at"`
	Detail   string    `json:"detail"`
}
