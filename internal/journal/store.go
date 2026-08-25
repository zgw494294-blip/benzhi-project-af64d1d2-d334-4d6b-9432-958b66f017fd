package journal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"tapemastergate/internal/application"
	"tapemastergate/internal/domain"
	"time"
)

var ErrChecksum = errors.New("事件帧校验和不匹配")

var appendFileCache = struct {
	sync.Mutex
	files map[string]*os.File
}{files: map[string]*os.File{}}

func openAppendFile(path string) (*os.File, error) {
	appendFileCache.Lock()
	defer appendFileCache.Unlock()
	if file := appendFileCache.files[path]; file != nil {
		return file, nil
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	appendFileCache.files[path] = file
	return file, nil
}

func forgetAppendFile(path string, file *os.File) {
	appendFileCache.Lock()
	defer appendFileCache.Unlock()
	if appendFileCache.files[path] == file {
		delete(appendFileCache.files, path)
	}
}

type Store struct {
	mu                           sync.RWMutex
	dir, logPath, checkpointPath string
	file                         *os.File
	projection                   application.Snapshot
	idempotency                  map[string]application.CommitResult
	sequence                     int64
	lastDigest                   string
	closed                       bool
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, logPath: filepath.Join(dir, "events.jsonl"), checkpointPath: filepath.Join(dir, "checkpoint.json"), projection: application.NewSnapshot(), idempotency: map[string]application.CommitResult{}}
	cp, err := loadCheckpoint(s.checkpointPath)
	if err != nil {
		return nil, err
	}
	if err = s.replay(); err != nil {
		return nil, err
	}
	if cp.Sequence > s.sequence {
		return nil, fmt.Errorf("检查点序号 %d 超过事件日志序号 %d", cp.Sequence, s.sequence)
	}
	if cp.Sequence == s.sequence && cp.Sequence > 0 && cp.LastDigest != s.lastDigest {
		return nil, fmt.Errorf("检查点摘要与事件日志不一致")
	}
	if err = validateProjection(s.projection); err != nil {
		return nil, fmt.Errorf("事件投影完整性校验失败: %w", err)
	}
	s.file, err = openAppendFile(s.logPath)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) replay() error {
	b, err := os.ReadFile(s.logPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(b) == 0 {
		return nil
	}
	if b[len(b)-1] != '\n' {
		return fmt.Errorf("事件日志存在截断尾帧，最后一个已确认帧之后缺少换行")
	}
	lines := bytes.Split(b[:len(b)-1], []byte{'\n'})
	previous := ""
	var seq int64
	for i, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			return fmt.Errorf("事件日志第 %d 行为空", i+1)
		}
		var f frame
		if err = json.Unmarshal(line, &f); err != nil {
			return fmt.Errorf("事件日志第 %d 帧无法解析: %w", i+1, err)
		}
		if f.SchemaVersion != schemaVersion {
			return fmt.Errorf("事件日志第 %d 帧 schemaVersion 不受支持", i+1)
		}
		if f.Sequence != seq+1 {
			return fmt.Errorf("事件日志序号不连续: 期望 %d，得到 %d", seq+1, f.Sequence)
		}
		if f.PreviousDigest != previous {
			return fmt.Errorf("事件日志第 %d 帧前序摘要不匹配", i+1)
		}
		if err = f.verify(); err != nil {
			return fmt.Errorf("事件日志第 %d 帧: %w", i+1, err)
		}
		for eventIndex, ev := range f.Events {
			auditSequence := f.Sequence*1000 + int64(eventIndex+1)
			if err = s.projection.Apply(ev, auditSequence); err != nil {
				return fmt.Errorf("重建投影失败: %w", err)
			}
		}
		s.idempotency[f.IdempotencyKey] = f.Result
		seq = f.Sequence
		previous = f.Checksum
	}
	s.sequence = seq
	s.lastDigest = previous
	return nil
}

func (s *Store) Snapshot(context.Context) application.Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.projection.Clone()
}
func (s *Store) IdempotentResult(_ context.Context, key string) (application.CommitResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.idempotency[key]
	return v, ok
}

func (s *Store) Commit(_ context.Context, jobID string, expected int64, key string, events []application.Event, result application.CommitResult) (application.CommitResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return application.CommitResult{}, errors.New("事件日志已经关闭")
	}
	if prior, ok := s.idempotency[key]; ok {
		return prior, nil
	}
	current := int64(0)
	if job, ok := s.projection.Jobs[jobID]; ok {
		current = job.Version
	}
	if current != expected {
		return application.CommitResult{}, fmt.Errorf("%w: 当前版本 %d，期望版本 %d", domain.ErrConflict, current, expected)
	}
	f := frame{SchemaVersion: schemaVersion, Sequence: s.sequence + 1, PreviousDigest: s.lastDigest, CommittedAt: time.Now().UTC(), JobID: jobID, IdempotencyKey: key, Events: events, Result: result}
	if err := f.seal(); err != nil {
		return application.CommitResult{}, err
	}
	line, err := json.Marshal(f)
	if err != nil {
		return application.CommitResult{}, err
	}
	line = append(line, '\n')
	if _, err = s.file.Write(line); err != nil {
		return application.CommitResult{}, err
	}
	if err = s.file.Sync(); err != nil {
		return application.CommitResult{}, err
	}
	next := s.projection.Clone()
	auditSequence := s.sequence * 1000
	for i, ev := range events {
		if err = next.Apply(ev, auditSequence+int64(i+1)); err != nil {
			return application.CommitResult{}, err
		}
	}
	s.projection = next
	s.sequence = f.Sequence
	s.lastDigest = f.Checksum
	s.idempotency[key] = result
	cp := checkpoint{SchemaVersion: schemaVersion, Sequence: s.sequence, LastDigest: s.lastDigest, Projection: s.projection, Idempotency: s.idempotency}
	if err = saveCheckpoint(s.checkpointPath, cp); err != nil {
		return application.CommitResult{}, fmt.Errorf("事件已确认但检查点更新失败: %w", err)
	}
	return result, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.file == nil {
		return nil
	}
	forgetAppendFile(s.logPath, s.file)
	return s.file.Close()
}
