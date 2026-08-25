package journal

import (
	"fmt"
	"tapemastergate/internal/application"
)

type Diagnostics struct {
	SchemaVersion    int    `json:"schemaVersion"`
	Sequence         int64  `json:"sequence"`
	LastDigest       string `json:"lastDigest"`
	JobCount         int    `json:"jobCount"`
	CarrierCount     int    `json:"carrierCount"`
	CaptureCount     int    `json:"captureCount"`
	FindingCount     int    `json:"findingCount"`
	ManifestCount    int    `json:"manifestCount"`
	CertificateCount int    `json:"certificateCount"`
	IdempotencyCount int    `json:"idempotencyCount"`
}

func (s *Store) Diagnostics() Diagnostics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Diagnostics{SchemaVersion: schemaVersion, Sequence: s.sequence, LastDigest: s.lastDigest, JobCount: len(s.projection.Jobs), CarrierCount: len(s.projection.Carriers), CaptureCount: len(s.projection.Captures), FindingCount: len(s.projection.Findings), ManifestCount: len(s.projection.Manifests), CertificateCount: len(s.projection.Certificates), IdempotencyCount: len(s.idempotency)}
}

func validateProjection(s application.Snapshot) error {
	for id, carrier := range s.Carriers {
		if _, ok := s.Jobs[carrier.JobID]; !ok {
			return fmt.Errorf("投影载体 %s 引用了不存在的作业 %s", id, carrier.JobID)
		}
	}
	for jobID, preflight := range s.Preflights {
		if _, ok := s.Jobs[jobID]; !ok {
			return fmt.Errorf("投影前检引用了不存在的作业 %s", jobID)
		}
		for _, check := range preflight.CarrierChecks {
			carrier, ok := s.Carriers[check.CarrierID]
			if !ok || carrier.JobID != jobID {
				return fmt.Errorf("投影前检的载体 %s 不属于作业 %s", check.CarrierID, jobID)
			}
		}
	}
	for id, capture := range s.Captures {
		job, jobOK := s.Jobs[capture.JobID]
		carrier, carrierOK := s.Carriers[capture.CarrierID]
		if !jobOK {
			return fmt.Errorf("投影采集 %s 引用了不存在的作业 %s", id, capture.JobID)
		}
		if !carrierOK {
			return fmt.Errorf("投影采集 %s 引用了不存在的载体 %s", id, capture.CarrierID)
		}
		if carrier.JobID != job.ID {
			return fmt.Errorf("投影采集 %s 的载体跨作业", id)
		}
		if capture.SupersedesID != "" {
			previous, ok := s.Captures[capture.SupersedesID]
			if !ok {
				return fmt.Errorf("投影采集 %s 引用了不存在的替代前版本", id)
			}
			if previous.CarrierID != capture.CarrierID {
				return fmt.Errorf("投影采集 %s 的替代关系跨载体", id)
			}
		}
	}
	for id, finding := range s.Findings {
		capture, ok := s.Captures[finding.CaptureTakeID]
		if !ok {
			return fmt.Errorf("投影发现 %s 引用了不存在的采集", id)
		}
		if capture.JobID != finding.JobID {
			return fmt.Errorf("投影发现 %s 与采集跨作业", id)
		}
		if finding.CarrierID != "" && finding.CarrierID != capture.CarrierID {
			return fmt.Errorf("投影发现 %s 与关联载体不一致", id)
		}
		if finding.CurrentCaptureTakeID != "" {
			current, currentOK := s.Captures[finding.CurrentCaptureTakeID]
			if !currentOK || current.CarrierID != capture.CarrierID {
				return fmt.Errorf("投影发现 %s 的当前整改采集不一致", id)
			}
		}
	}
	for id, remediation := range s.Remediations {
		previous, previousOK := s.Captures[remediation.PreviousCaptureID]
		replacement, replacementOK := s.Captures[remediation.ReplacementCaptureID]
		if !previousOK || !replacementOK {
			return fmt.Errorf("投影整改 %s 的采集版本不完整", id)
		}
		if previous.CarrierID != replacement.CarrierID || replacement.JobID != remediation.JobID {
			return fmt.Errorf("投影整改 %s 的替代关系不一致", id)
		}
	}
	for jobID, manifest := range s.Manifests {
		if manifest.JobID != jobID {
			return fmt.Errorf("投影清单 %s 的作业索引不一致", manifest.ID)
		}
		if _, ok := s.Jobs[jobID]; !ok {
			return fmt.Errorf("投影清单 %s 引用了不存在的作业", manifest.ID)
		}
		for _, captureID := range manifest.CaptureTakeIDs {
			capture, ok := s.Captures[captureID]
			if !ok || capture.JobID != jobID {
				return fmt.Errorf("投影清单 %s 包含无效采集 %s", manifest.ID, captureID)
			}
		}
	}
	for number, certificate := range s.Certificates {
		manifest, ok := s.Manifests[certificate.JobID]
		if !ok {
			return fmt.Errorf("投影凭据 %s 缺少冻结清单", number)
		}
		if manifest.ID != certificate.ManifestID || manifest.ManifestDigest != certificate.ManifestDigest {
			return fmt.Errorf("投影凭据 %s 与冻结清单摘要不一致", number)
		}
	}
	return nil
}
