package certificateverificationcacherace

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"tapemastergate/internal/application"
	"tapemastergate/internal/domain"
	"testing"
)

const concurrentVerifications = 64

type barrierStore struct {
	snapshot application.Snapshot
	arrived  sync.WaitGroup
	release  chan struct{}
}

func newBarrierStore(snapshot application.Snapshot) *barrierStore {
	store := &barrierStore{snapshot: snapshot, release: make(chan struct{})}
	store.arrived.Add(concurrentVerifications)
	return store
}

func (s *barrierStore) Snapshot(context.Context) application.Snapshot {
	s.arrived.Done()
	<-s.release
	return s.snapshot
}

func (*barrierStore) Commit(context.Context, string, int64, string, []application.Event, application.CommitResult) (application.CommitResult, error) {
	panic("私有复现不执行写入")
}

func (*barrierStore) IdempotentResult(context.Context, string) (application.CommitResult, bool) {
	return application.CommitResult{}, false
}

func (*barrierStore) Close() error { return nil }

func TestConcurrentCertificateVerificationCacheIsSynchronized(t *testing.T) {
	previousProcs := runtime.GOMAXPROCS(8)
	defer runtime.GOMAXPROCS(previousProcs)

	snapshot := application.NewSnapshot()
	for i := 0; i < concurrentVerifications; i++ {
		jobID := fmt.Sprintf("job-%02d", i)
		manifestID := fmt.Sprintf("manifest-%02d", i)
		number := fmt.Sprintf("TMG-CACHE-%02d", i)
		digest := fmt.Sprintf("digest-%02d", i)
		snapshot.Manifests[jobID] = domain.DeliveryManifest{
			ID: manifestID, JobID: jobID, ManifestDigest: digest,
		}
		snapshot.Certificates[number] = domain.ReleaseCertificate{
			CertificateNo:    number,
			JobID:            jobID,
			ManifestID:       manifestID,
			ManifestDigest:   digest,
			VerificationCode: fmt.Sprintf("code-%02d", i),
		}
	}

	store := newBarrierStore(snapshot)
	service := application.NewService(store)
	results := make(chan bool, concurrentVerifications)
	var callers sync.WaitGroup
	callers.Add(concurrentVerifications)
	for i := 0; i < concurrentVerifications; i++ {
		go func(index int) {
			defer callers.Done()
			number := fmt.Sprintf("TMG-CACHE-%02d", index)
			code := fmt.Sprintf("code-%02d", index)
			results <- service.VerifyCertificate(context.Background(), number, code).Valid
		}(i)
	}

	store.arrived.Wait()
	close(store.release)
	callers.Wait()
	close(results)
	for valid := range results {
		if !valid {
			t.Fatal("合法凭据的并发验证结果应保持有效")
		}
	}
}
