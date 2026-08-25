package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"tapemastergate/internal/application"
)

type checkpoint struct {
	SchemaVersion int                                 `json:"schemaVersion"`
	Sequence      int64                               `json:"sequence"`
	LastDigest    string                              `json:"lastDigest"`
	Projection    application.Snapshot                `json:"projection"`
	Idempotency   map[string]application.CommitResult `json:"idempotency"`
}

func saveCheckpoint(path string, cp checkpoint) error {
	b, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "checkpoint-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	ok := false
	defer func() {
		_ = tmp.Close()
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if _, err = tmp.Write(b); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	ok = true
	return nil
}

func loadCheckpoint(path string) (checkpoint, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return checkpoint{}, nil
	}
	if err != nil {
		return checkpoint{}, err
	}
	var cp checkpoint
	if err = json.Unmarshal(b, &cp); err != nil {
		return checkpoint{}, fmt.Errorf("检查点损坏: %w", err)
	}
	if cp.SchemaVersion != schemaVersion {
		return checkpoint{}, fmt.Errorf("不支持的检查点 schemaVersion %d", cp.SchemaVersion)
	}
	return cp, nil
}
