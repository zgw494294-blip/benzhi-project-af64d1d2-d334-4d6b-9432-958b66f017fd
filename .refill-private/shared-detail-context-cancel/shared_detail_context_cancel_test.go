package shared_detail_context_cancel_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"tapemastergate/internal/application"
	"tapemastergate/internal/domain"
)

type blockingStore struct {
	snapshot application.Snapshot
	entered  chan struct{}
	release  chan struct{}
	calls    atomic.Int32
}

func (s *blockingStore) Snapshot(ctx context.Context) application.Snapshot {
	if s.calls.Add(1) != 1 {
		return s.snapshot
	}
	close(s.entered)
	select {
	case <-ctx.Done():
		return application.NewSnapshot()
	case <-s.release:
		return s.snapshot
	}
}

func (s *blockingStore) Commit(context.Context, string, int64, string, []application.Event, application.CommitResult) (application.CommitResult, error) {
	return application.CommitResult{}, errors.New("测试未使用 Commit")
}

func (s *blockingStore) IdempotentResult(context.Context, string) (application.CommitResult, bool) {
	return application.CommitResult{}, false
}

func (s *blockingStore) Close() error { return nil }

type observedContext struct {
	context.Context
	observed chan struct{}
	once     sync.Once
}

func (c *observedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.observed) })
	return c.Context.Done()
}

type detailResult struct {
	detail application.JobDetail
	err    error
}

func TestSharedJobDetailLoadSurvivesLeaderCancellation(t *testing.T) {
	snapshot := application.NewSnapshot()
	snapshot.Jobs["job-1"] = domain.DigitizationJob{ID: "job-1", Title: "共享读取", Status: domain.StatusDraft, Version: 1}
	store := &blockingStore{snapshot: snapshot, entered: make(chan struct{}), release: make(chan struct{})}
	service := application.NewService(store)

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderResult := make(chan detailResult, 1)
	go func() {
		detail, err := service.GetJob(leaderCtx, "job-1")
		leaderResult <- detailResult{detail: detail, err: err}
	}()
	<-store.entered

	followerCtx := &observedContext{Context: context.Background(), observed: make(chan struct{})}
	followerResult := make(chan detailResult, 1)
	go func() {
		detail, err := service.GetJob(followerCtx, "job-1")
		followerResult <- detailResult{detail: detail, err: err}
	}()
	<-followerCtx.observed

	cancelLeader()
	<-leaderResult
	close(store.release)

	got := <-followerResult
	if got.err != nil {
		t.Fatalf("仍然有效的等待请求不应继承首请求的取消结果: %v", got.err)
	}
	if got.detail.Job.ID != "job-1" {
		t.Fatalf("等待请求返回了错误作业: %q", got.detail.Job.ID)
	}
}
