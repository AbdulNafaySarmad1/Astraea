package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const maxPendingBatches = 672 // Seven days at the default 15-minute interval.
const maxBatchBytes = 512 << 10

type Spool struct{ dir string }

func openSpool(path string) (*Spool, error) {
	dir, err := filepath.Abs(path)
	if err != nil || dir == "" {
		return nil, errors.New("invalid telemetry spool path")
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("telemetry spool must be a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("telemetry spool is accessible to other users")
	}
	return &Spool{dir: dir}, nil
}

func (s *Spool) Save(batch Heartbeat) error {
	if batch.TenantID == "" || batch.ConnectorID == "" || len(batch.Components) == 0 || len(batch.BatchID) != 32 {
		return errors.New("telemetry batch requires tenant, connector, ID, and components")
	}
	if _, err := hex.DecodeString(batch.BatchID); err != nil {
		return errors.New("invalid telemetry batch ID")
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	pending := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			pending++
		}
	}
	if pending >= maxPendingBatches {
		return fmt.Errorf("telemetry spool full (%d batches); operator intervention required", pending)
	}
	raw, err := json.Marshal(batch)
	if err != nil || len(raw) > maxBatchBytes {
		return errors.New("telemetry batch exceeds size limit")
	}
	file, err := os.CreateTemp(s.dir, ".pending-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(raw)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	final := filepath.Join(s.dir, time.Now().UTC().Format("20060102T150405.000000000Z")+"_"+batch.BatchID+".json")
	if err = os.Rename(name, final); err != nil {
		return err
	}
	if dir, err := os.Open(s.dir); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func (s *Spool) Oldest() (string, Heartbeat, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return "", Heartbeat{}, err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		if !entry.Type().IsRegular() {
			return "", Heartbeat{}, errors.New("telemetry spool contains non-regular file")
		}
		path := filepath.Join(s.dir, entry.Name())
		info, err := entry.Info()
		if err != nil || info.Size() > maxBatchBytes {
			return "", Heartbeat{}, errors.New("telemetry spool entry exceeds size limit")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", Heartbeat{}, err
		}
		var batch Heartbeat
		if json.Unmarshal(raw, &batch) != nil || batch.BatchID == "" || len(batch.Components) == 0 {
			return "", Heartbeat{}, errors.New("telemetry spool entry is invalid")
		}
		return path, batch, nil
	}
	return "", Heartbeat{}, os.ErrNotExist
}

func (s *Spool) Remove(path string) error {
	if filepath.Dir(path) != s.dir || !strings.HasSuffix(path, ".json") {
		return errors.New("telemetry spool path is outside spool directory")
	}
	return os.Remove(path)
}
