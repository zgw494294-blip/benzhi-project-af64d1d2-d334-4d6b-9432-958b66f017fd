package application

import (
	"encoding/json"
	"sort"
	"tapemastergate/internal/domain"
)

type Snapshot struct {
	Jobs         map[string]domain.DigitizationJob       `json:"jobs"`
	Carriers     map[string]domain.TapeCarrier           `json:"carriers"`
	Preflights   map[string]domain.PreflightCheck        `json:"preflights"`
	Captures     map[string]domain.CaptureTake           `json:"captures"`
	Findings     map[string]domain.QualityFinding        `json:"findings"`
	Manifests    map[string]domain.DeliveryManifest      `json:"manifests"`
	Certificates map[string]domain.ReleaseCertificate    `json:"certificates"`
	Remediations map[string]domain.RemediationSubmission `json:"remediations"`
	Audits       []domain.AuditEntry                     `json:"audits"`
}

func NewSnapshot() Snapshot {
	return Snapshot{Jobs: map[string]domain.DigitizationJob{}, Carriers: map[string]domain.TapeCarrier{}, Preflights: map[string]domain.PreflightCheck{}, Captures: map[string]domain.CaptureTake{}, Findings: map[string]domain.QualityFinding{}, Manifests: map[string]domain.DeliveryManifest{}, Certificates: map[string]domain.ReleaseCertificate{}, Remediations: map[string]domain.RemediationSubmission{}}
}

func (s Snapshot) Clone() Snapshot {
	b, _ := json.Marshal(s)
	n := NewSnapshot()
	_ = json.Unmarshal(b, &n)
	return n
}

func (s *Snapshot) Apply(ev Event, sequence int64) error {
	var err error
	switch ev.Type {
	case "job.created", "job.profile_set", "job.status_changed":
		var v domain.DigitizationJob
		err = json.Unmarshal(ev.Data, &v)
		if err == nil {
			s.Jobs[v.ID] = v
		}
	case "carrier.added":
		var v domain.TapeCarrier
		err = json.Unmarshal(ev.Data, &v)
		if err == nil {
			s.Carriers[v.ID] = v
		}
	case "preflight.completed":
		var v struct {
			JobID string                `json:"jobId"`
			Check domain.PreflightCheck `json:"check"`
		}
		err = json.Unmarshal(ev.Data, &v)
		if err == nil {
			s.Preflights[v.JobID] = v.Check
		}
	case "preflight.invalidated":
		delete(s.Preflights, ev.JobID)
	case "capture.registered", "capture.voided":
		var v domain.CaptureTake
		err = json.Unmarshal(ev.Data, &v)
		if err == nil {
			s.Captures[v.ID] = v
		}
	case "finding.created", "finding.reviewed", "finding.remediation_submitted":
		var v domain.QualityFinding
		err = json.Unmarshal(ev.Data, &v)
		if err == nil {
			s.Findings[v.ID] = v
		}
	case "manifest.frozen":
		var v domain.DeliveryManifest
		err = json.Unmarshal(ev.Data, &v)
		if err == nil {
			s.Manifests[v.JobID] = v
		}
	case "certificate.issued":
		var v domain.ReleaseCertificate
		err = json.Unmarshal(ev.Data, &v)
		if err == nil {
			s.Certificates[v.CertificateNo] = v
		}
	case "remediation.submitted":
		var v domain.RemediationSubmission
		err = json.Unmarshal(ev.Data, &v)
		if err == nil {
			s.Remediations[v.ID] = v
		}
	}
	if err == nil {
		s.Audits = append(s.Audits, domain.AuditEntry{Sequence: sequence, JobID: ev.JobID, Action: ev.Type, Actor: ev.Actor, At: ev.At, Detail: eventDetail(ev.Type)})
	}
	return err
}

func eventDetail(kind string) string {
	labels := map[string]string{"job.created": "创建数字化作业", "job.profile_set": "设定采集规格", "carrier.added": "登记磁带载体", "preflight.completed": "提交逐盘载体前检", "preflight.invalidated": "新增载体使前检失效", "job.status_changed": "作业状态变更", "capture.registered": "登记采集版本", "capture.voided": "作废失败采集", "finding.created": "生成质量发现", "finding.reviewed": "复核质量发现", "finding.remediation_submitted": "质量发现进入下一整改轮次", "remediation.submitted": "提交修复版本并复跑规则", "manifest.frozen": "批准并冻结交付清单", "certificate.issued": "签发交付凭据"}
	if v := labels[kind]; v != "" {
		return v
	}
	return kind
}

func (s Snapshot) JobRemediations(jobID string) []domain.RemediationSubmission {
	var out []domain.RemediationSubmission
	for _, v := range s.Remediations {
		if v.JobID == jobID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SubmittedAt.Before(out[j].SubmittedAt) })
	return out
}

func (s Snapshot) JobCarriers(jobID string) []domain.TapeCarrier {
	var out []domain.TapeCarrier
	for _, v := range s.Carriers {
		if v.JobID == jobID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CarrierCode < out[j].CarrierCode })
	return out
}
func (s Snapshot) JobCaptures(jobID string) []domain.CaptureTake {
	var out []domain.CaptureTake
	for _, v := range s.Captures {
		if v.JobID == jobID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CarrierID == out[j].CarrierID {
			return out[i].Sequence < out[j].Sequence
		}
		return out[i].CarrierID < out[j].CarrierID
	})
	return out
}
func (s Snapshot) JobFindings(jobID string) []domain.QualityFinding {
	var out []domain.QualityFinding
	for _, v := range s.Findings {
		if v.JobID == jobID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out
}
