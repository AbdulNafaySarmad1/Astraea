package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"nocturn.example/aegis-operations/internal/logsafe"
)

const maxLogReadBytes = 256 << 10
const maxPendingLogBatches = 3000

var localLogName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

type LogSource struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type LogBatch struct {
	TenantID    string    `json:"tenant_id"`
	ConnectorID string    `json:"connector_id"`
	BatchID     string    `json:"batch_id"`
	SourceName  string    `json:"source_name"`
	SourceKind  string    `json:"source_kind"`
	PathSHA256  string    `json:"path_sha256"`
	CollectedAt time.Time `json:"collected_at"`
	Lines       []string  `json:"lines"`
}

type logCursor struct {
	FileID string `json:"file_id"`
	Offset int64  `json:"offset"`
}

type LogSpool struct {
	dir string
}

func validateLogSource(source LogSource) error {
	if !localLogName.MatchString(source.Name) || source.Path == "" || !filepath.IsAbs(source.Path) || source.Path != filepath.Clean(source.Path) {
		return errors.New("log source requires a stable name and absolute file path")
	}
	if source.Kind != "postgres" && source.Kind != "valkey" && source.Kind != "vault" && source.Kind != "aegiscore" {
		return errors.New("unsupported service log kind")
	}
	return nil
}

func openLogSpool(base string) (*LogSpool, error) {
	spool, err := openSpool(filepath.Join(base, "service-logs"))
	if err != nil {
		return nil, err
	}
	return &LogSpool{dir: spool.dir}, nil
}

func (s *LogSpool) cursorPath(name string) string { return filepath.Join(s.dir, name+".cursor") }

func (s *LogSpool) cursor(name string) (logCursor, error) {
	raw, err := os.ReadFile(s.cursorPath(name))
	if errors.Is(err, os.ErrNotExist) {
		return logCursor{}, nil
	}
	if err != nil {
		return logCursor{}, err
	}
	var cursor logCursor
	if json.Unmarshal(raw, &cursor) != nil || cursor.FileID == "" || cursor.Offset < 0 {
		return logCursor{}, errors.New("invalid log cursor; operator intervention required")
	}
	return cursor, nil
}

func (s *LogSpool) saveCursor(name string, cursor logCursor) error {
	raw, _ := json.Marshal(cursor)
	return atomicPrivateFile(s.dir, name+".cursor", raw)
}

func (s *LogSpool) save(batch LogBatch) error {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	pending := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".batch") {
			pending++
			if strings.HasSuffix(entry.Name(), "_"+batch.BatchID+".batch") {
				return nil
			}
		}
	}
	if pending >= maxPendingLogBatches {
		return errors.New("service log spool full; operator intervention required")
	}
	raw, err := json.Marshal(batch)
	if err != nil || len(raw) > maxLogReadBytes+65536 {
		return errors.New("service log batch exceeds size limit")
	}
	name := time.Now().UTC().Format("20060102T150405.000000000Z") + "_" + batch.BatchID + ".batch"
	return atomicPrivateFile(s.dir, name, raw)
}

func atomicPrivateFile(dir, name string, raw []byte) error {
	temp, err := os.CreateTemp(dir, ".pending-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err = temp.Chmod(0600); err == nil {
		_, err = temp.Write(raw)
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(temp.Name(), filepath.Join(dir, name)); err != nil {
		return err
	}
	if opened, err := os.Open(dir); err == nil {
		_ = opened.Sync()
		_ = opened.Close()
	}
	return nil
}

func (s *LogSpool) oldest() (string, LogBatch, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return "", LogBatch{}, err
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".batch") {
			continue
		}
		if !entry.Type().IsRegular() {
			return "", LogBatch{}, errors.New("non-regular service log spool entry")
		}
		path := filepath.Join(s.dir, entry.Name())
		info, err := entry.Info()
		if err != nil || info.Size() > maxLogReadBytes+65536 {
			return "", LogBatch{}, errors.New("invalid service log spool entry size")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", LogBatch{}, err
		}
		var batch LogBatch
		if json.Unmarshal(raw, &batch) != nil || len(batch.Lines) == 0 || batch.BatchID == "" {
			return "", LogBatch{}, errors.New("invalid service log spool entry")
		}
		return path, batch, nil
	}
	return "", LogBatch{}, os.ErrNotExist
}

func collectLogs(c Config, spool *LogSpool) {
	for _, source := range c.LogSources {
		if err := collectLogSource(c, spool, source); err != nil {
			log.Printf("service log source %s deferred: %v", source.Name, err)
		}
	}
}

func collectLogSource(c Config, spool *LogSpool, source LogSource) error {
	resolved, err := filepath.EvalSymlinks(source.Path)
	if err != nil || resolved != source.Path {
		return errors.New("approved service log path must not traverse a symlink")
	}
	info, err := os.Lstat(source.Path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("approved service log path is not a regular file")
	}
	file, err := os.Open(source.Path)
	if err != nil {
		return err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		return errors.New("service log file changed during open")
	}
	id := logFileIdentity(openedInfo)
	cursor, err := spool.cursor(source.Name)
	if err != nil {
		return err
	}
	if cursor.FileID == "" {
		// No automatic historical backfill: start from the approved file's end.
		return spool.saveCursor(source.Name, logCursor{FileID: id, Offset: openedInfo.Size()})
	}
	if cursor.FileID != id || openedInfo.Size() < cursor.Offset {
		cursor = logCursor{FileID: id, Offset: 0}
	}
	if openedInfo.Size() == cursor.Offset {
		return nil
	}
	buffer := make([]byte, min(maxLogReadBytes, int(openedInfo.Size()-cursor.Offset)))
	n, err := file.ReadAt(buffer, cursor.Offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	buffer = buffer[:n]
	end := bytes.LastIndexByte(buffer, '\n')
	if end < 0 {
		return errors.New("service log line exceeds read window or is incomplete")
	}
	end++
	lines := []string{}
	processed := 0
	for _, raw := range bytes.SplitAfter(buffer[:end], []byte{'\n'}) {
		if len(raw) == 0 {
			continue
		}
		processed += len(raw)
		if line, keep := logsafe.FilterLine(source.Kind, string(raw)); keep {
			lines = append(lines, line)
			if len(lines) == 1024 {
				break
			}
		}
	}
	newOffset := cursor.Offset + int64(processed)
	if len(lines) > 0 {
		material := fmt.Sprintf("%s|%s|%s|%s|%d|%d", c.TenantID, c.ConnectorID, source.Name, id, cursor.Offset, newOffset)
		hash := sha256.Sum256([]byte(material))
		pathHash := sha256.Sum256([]byte(filepath.Clean(source.Path)))
		batch := LogBatch{TenantID: c.TenantID, ConnectorID: c.ConnectorID, BatchID: hex.EncodeToString(hash[:16]), SourceName: source.Name, SourceKind: source.Kind, PathSHA256: hex.EncodeToString(pathHash[:]), CollectedAt: time.Now().UTC(), Lines: lines}
		if err = spool.save(batch); err != nil {
			return err
		}
	}
	return spool.saveCursor(source.Name, logCursor{FileID: id, Offset: newOffset})
}

func flushLogs(ctx context.Context, client *http.Client, c Config, credential string, spool *LogSpool) {
	for i := 0; i < 20; i++ {
		path, batch, err := spool.oldest()
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		if err != nil {
			log.Printf("service log spool unavailable: %v", err)
			return
		}
		if batch.TenantID != c.TenantID || batch.ConnectorID != c.ConnectorID {
			log.Print("service log spool identity mismatch; operator intervention required")
			return
		}
		payload, _ := json.Marshal(batch)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.ControlPlaneURL, "/")+"/v1/connectors/logs", bytes.NewReader(payload))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+credential)
		response, err := client.Do(req)
		if err != nil {
			log.Printf("service log delivery deferred: %v", err)
			return
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			log.Printf("service log delivery deferred: HTTP %d", response.StatusCode)
			return
		}
		if err = os.Remove(path); err != nil {
			log.Printf("accepted service log batch remains queued: %v", err)
			return
		}
	}
}
