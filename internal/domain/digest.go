package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"
)

func BuildManifest(jobID string, revision int, takes []CaptureTake, approvedBy string, now time.Time, id string) DeliveryManifest {
	copyTakes := append([]CaptureTake(nil), takes...)
	sort.Slice(copyTakes, func(i, j int) bool {
		if copyTakes[i].CarrierID == copyTakes[j].CarrierID {
			return copyTakes[i].Sequence < copyTakes[j].Sequence
		}
		return copyTakes[i].CarrierID < copyTakes[j].CarrierID
	})
	ids := make([]string, 0, len(copyTakes))
	parts := []string{"tapemaster-manifest-v2", jobID, strconv.Itoa(revision)}
	for _, t := range copyTakes {
		ids = append(ids, t.ID)
		parts = append(parts, t.CarrierID, t.ID, t.Filename, t.SHA256, strconv.Itoa(t.SampleRate), strconv.Itoa(t.BitDepth), strconv.Itoa(t.Channels), strconv.FormatInt(t.DurationMS, 10))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return DeliveryManifest{ID: id, JobID: jobID, Revision: revision, CaptureTakeIDs: ids, ManifestDigest: hex.EncodeToString(sum[:]), ApprovedBy: approvedBy, FrozenAt: now.UTC()}
}

func BuildCertificate(manifest DeliveryManifest, issuedBy string, now time.Time, number string) ReleaseCertificate {
	payload := strings.Join([]string{"tapemaster-certificate-v1", number, manifest.ID, manifest.ManifestDigest, now.UTC().Format(time.RFC3339Nano)}, "\n")
	sum := sha256.Sum256([]byte(payload))
	return ReleaseCertificate{CertificateNo: number, JobID: manifest.JobID, ManifestID: manifest.ID, ManifestDigest: manifest.ManifestDigest, IssuedBy: issuedBy, IssuedAt: now.UTC(), VerificationCode: hex.EncodeToString(sum[:])}
}
