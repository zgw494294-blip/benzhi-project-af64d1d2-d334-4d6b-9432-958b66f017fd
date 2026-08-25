package domain

import "sort"

func BuildManifestPreview(job DigitizationJob, carriers []TapeCarrier, captures []CaptureTake, findings []QualityFinding, revision int) ManifestPreview {
	preview := ManifestPreview{JobID: job.ID, JobVersion: job.Version, ProposedRevision: revision}
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
	masters := make([]CaptureTake, 0, len(ordered))
	for _, carrier := range ordered {
		take, ok := active[carrier.ID]
		if !ok {
			preview.Blockers = append(preview.Blockers, ManifestBlocker{Code: "missing_active_capture", CarrierID: carrier.ID, CarrierCode: carrier.CarrierCode, Message: "载体缺少有效采集"})
			continue
		}
		masters = append(masters, take)
		preview.Items = append(preview.Items, ManifestPreviewItem{CarrierID: carrier.ID, CarrierCode: carrier.CarrierCode, CaptureID: take.ID, Sequence: take.Sequence, Filename: take.Filename, SHA256: take.SHA256, SampleRate: take.SampleRate, BitDepth: take.BitDepth, Channels: take.Channels})
		if take.SampleRate != job.CaptureProfile.SampleRate || take.BitDepth != job.CaptureProfile.BitDepth || take.Channels != job.CaptureProfile.Channels {
			preview.Blockers = append(preview.Blockers, ManifestBlocker{Code: "parameter_mismatch", CarrierID: carrier.ID, CarrierCode: carrier.CarrierCode, CaptureID: take.ID, Message: "当前母版技术参数与作业规格不一致"})
		}
	}
	if len(ordered) == 0 {
		preview.Blockers = append(preview.Blockers, ManifestBlocker{Code: "no_carriers", Message: "作业尚未登记载体"})
	}
	for _, finding := range findings {
		if finding.Status != FindingClosed && (finding.Severity == SeverityMajor || finding.Severity == SeverityCritical) {
			preview.Blockers = append(preview.Blockers, ManifestBlocker{Code: "open_severe_finding", CarrierID: finding.CarrierID, CaptureID: finding.CurrentCaptureTakeID, FindingID: finding.ID, Message: "存在未关闭的严重质量发现"})
		}
	}
	manifest := BuildManifest(job.ID, revision, masters, "", job.UpdatedAt, "")
	preview.ProposedDigest = manifest.ManifestDigest
	preview.CanFreeze = len(preview.Blockers) == 0 && len(masters) == len(ordered) && len(ordered) > 0
	return preview
}
