package platform

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const maxLogPlaintext = 320 << 10

type logStorage struct {
	hot           string
	archive       string
	key           []byte
	hotMarker     string
	archiveMarker string
}

type logManifest struct {
	TenantID        string    `json:"tenant_id"`
	ConnectorID     string    `json:"connector_id"`
	BatchID         string    `json:"batch_id"`
	SourceName      string    `json:"source_name"`
	ReceivedAt      time.Time `json:"received_at"`
	PlaintextSHA256 string    `json:"plaintext_sha256"`
	ArchiveSHA256   string    `json:"archive_sha256"`
	ArchiveBytes    int       `json:"archive_bytes"`
	Compression     string    `json:"compression"`
	Encryption      string    `json:"encryption"`
}

func openLogStorage(c Config) (*logStorage, error) {
	s, err := openHotLogStorage(c)
	if err != nil {
		return nil, err
	}
	if c.LogArchiveDir == "" {
		return nil, errors.New("service log archive directory is not configured")
	}
	if pathsOverlap(c.LogHotDir, c.LogArchiveDir) || c.LogHotMarker == c.LogArchiveMarker {
		return nil, errors.New("hot and archive stores must have distinct paths and identities")
	}
	if err = checkStoreRoot(c.LogArchiveDir, c.LogArchiveMarker); err != nil {
		return nil, err
	}
	s.archive = c.LogArchiveDir
	s.archiveMarker = c.LogArchiveMarker
	return s, nil
}

func openHotLogStorage(c Config) (*logStorage, error) {
	if c.LogHotDir == "" || c.LogKeyB64 == "" {
		return nil, errors.New("service log storage is not configured")
	}
	key, err := base64.StdEncoding.DecodeString(c.LogKeyB64)
	if err != nil || len(key) != 32 {
		return nil, errors.New("invalid service log encryption key")
	}
	if err = checkStoreRoot(c.LogHotDir, c.LogHotMarker); err != nil {
		return nil, err
	}
	return &logStorage{hot: c.LogHotDir, key: key, hotMarker: c.LogHotMarker}, nil
}

func validStoreMarker(value string) bool {
	if len(value) < 8 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if char != '-' && char != '_' && (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') {
			return false
		}
	}
	return true
}

func checkStoreRoot(path, marker string) error {
	if !validStoreMarker(marker) || !filepath.IsAbs(path) {
		return errors.New("service log store marker or path invalid")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("service log store root missing or not a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return errors.New("service log store root accessible to other users")
	}
	actual, err := os.ReadFile(filepath.Join(path, ".aegisops-store-id"))
	if err != nil || string(bytes.TrimSpace(actual)) != marker {
		return errors.New("service log store identity marker unavailable or mismatched")
	}
	return nil
}

func ensurePrivateDir(path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("service log storage path must be absolute")
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("service log storage path must be a real directory")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return errors.New("service log storage directory is accessible to other users")
	}
	return nil
}

func logDigest(raw []byte) string {
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

func readRegularBounded(path string, maxBytes int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxBytes {
		return nil, errors.New("log storage object unavailable or invalid")
	}
	return os.ReadFile(path)
}

func (s *logStorage) hotPath(tenant, connector, batch string) string {
	return filepath.Join(s.hot, tenant, connector, batch+".jsonl.enc")
}

func (s *logStorage) archivePath(tenant, source, batch string, received time.Time) string {
	return filepath.Join(s.archive, tenant, source, received.UTC().Format("2006/01/02"), batch+".jsonl.gz.enc")
}

func (s *logStorage) encrypt(raw []byte, associated string) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, raw, []byte(associated)), nil
}

func (s *logStorage) decrypt(raw []byte, associated string) ([]byte, error) {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(raw) < gcm.NonceSize() {
		return nil, errors.New("invalid encrypted log batch")
	}
	return gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte(associated))
}

func writePrivateAtomic(path string, raw []byte) error {
	dir := filepath.Dir(path)
	if err := ensurePrivateDir(dir); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
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
	if err = os.Rename(temp.Name(), path); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	opened, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer opened.Close()
	return opened.Sync()
}

func (s *logStorage) saveHot(tenant, connector, batch string, raw []byte) error {
	if len(raw) == 0 || len(raw) > maxLogPlaintext {
		return errors.New("service log batch size invalid")
	}
	path := s.hotPath(tenant, connector, batch)
	if _, err := os.Lstat(path); err == nil {
		stored, err := s.readHot(tenant, connector, batch)
		if err != nil || !bytes.Equal(raw, stored) {
			return errors.New("existing service log batch differs")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	encrypted, err := s.encrypt(raw, tenant+"|"+connector+"|"+batch+"|hot")
	if err != nil {
		return err
	}
	return writePrivateAtomic(path, encrypted)
}

func (s *logStorage) readHot(tenant, connector, batch string) ([]byte, error) {
	path := s.hotPath(tenant, connector, batch)
	raw, err := readRegularBounded(path, maxLogPlaintext+64)
	if err != nil {
		return nil, err
	}
	return s.decrypt(raw, tenant+"|"+connector+"|"+batch+"|hot")
}

func gzipLog(raw []byte) ([]byte, error) {
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(raw); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (s *logStorage) archiveAndVerify(tenant, connector, source, batch string, received time.Time, plaintextHash string, raw []byte) (string, error) {
	if logDigest(raw) != plaintextHash {
		return "", errors.New("hot log checksum mismatch")
	}
	path := s.archivePath(tenant, source, batch, received)
	compressed, err := gzipLog(raw)
	if err != nil {
		return "", err
	}
	if _, err = os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		sealed, e := s.encrypt(compressed, tenant+"|"+connector+"|"+batch+"|archive")
		if e != nil {
			return "", e
		}
		if err = writePrivateAtomic(path, sealed); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	stored, err := readRegularBounded(path, maxLogPlaintext+1024)
	if err != nil {
		return "", errors.New("archive unreadable or oversized")
	}
	archiveHash := logDigest(stored)
	opened, err := s.decrypt(stored, tenant+"|"+connector+"|"+batch+"|archive")
	if err != nil {
		return "", err
	}
	reader, err := gzip.NewReader(bytes.NewReader(opened))
	if err != nil {
		return "", err
	}
	restored, err := io.ReadAll(io.LimitReader(reader, maxLogPlaintext+1))
	closeErr := reader.Close()
	if err != nil || closeErr != nil || len(restored) > maxLogPlaintext || !bytes.Equal(restored, raw) {
		return "", errors.New("archive restore verification failed")
	}
	manifest := logManifest{TenantID: tenant, ConnectorID: connector, BatchID: batch, SourceName: source, ReceivedAt: received, PlaintextSHA256: plaintextHash, ArchiveSHA256: archiveHash, ArchiveBytes: len(stored), Compression: "gzip", Encryption: "AES-256-GCM"}
	manifestPath := path + ".manifest.json"
	if _, err = os.Lstat(manifestPath); errors.Is(err, os.ErrNotExist) {
		body, _ := json.Marshal(manifest)
		if err = writePrivateAtomic(manifestPath, body); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	manifestBytes, err := readRegularBounded(manifestPath, 4096)
	var existing logManifest
	if err != nil || json.Unmarshal(manifestBytes, &existing) != nil || existing.TenantID != manifest.TenantID || existing.ConnectorID != manifest.ConnectorID || existing.BatchID != manifest.BatchID || existing.SourceName != manifest.SourceName || !existing.ReceivedAt.Equal(manifest.ReceivedAt) || existing.PlaintextSHA256 != manifest.PlaintextSHA256 || existing.ArchiveSHA256 != manifest.ArchiveSHA256 || existing.ArchiveBytes != manifest.ArchiveBytes || existing.Compression != manifest.Compression || existing.Encryption != manifest.Encryption {
		return "", errors.New("archive manifest mismatch")
	}
	if err = checkStoreRoot(s.archive, s.archiveMarker); err != nil {
		return "", err
	}
	return archiveHash, nil
}

func (s *logStorage) verifyArchive(tenant, connector, source, batch string, received time.Time, plaintextHash, archiveHash string) error {
	path := s.archivePath(tenant, source, batch, received)
	stored, err := readRegularBounded(path, maxLogPlaintext+1024)
	if err != nil || logDigest(stored) != archiveHash {
		return errors.New("archive checksum mismatch")
	}
	opened, err := s.decrypt(stored, tenant+"|"+connector+"|"+batch+"|archive")
	if err != nil {
		return err
	}
	reader, err := gzip.NewReader(bytes.NewReader(opened))
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxLogPlaintext+1))
	reader.Close()
	if err != nil || len(raw) > maxLogPlaintext || logDigest(raw) != plaintextHash {
		return fmt.Errorf("archive content verification failed")
	}
	manifestRaw, err := readRegularBounded(path+".manifest.json", 4096)
	var manifest logManifest
	if err != nil || json.Unmarshal(manifestRaw, &manifest) != nil || manifest.TenantID != tenant || manifest.ConnectorID != connector || manifest.BatchID != batch || manifest.SourceName != source || !manifest.ReceivedAt.Equal(received) || manifest.PlaintextSHA256 != plaintextHash || manifest.ArchiveSHA256 != archiveHash || manifest.Compression != "gzip" || manifest.Encryption != "AES-256-GCM" {
		return errors.New("archive manifest verification failed")
	}
	if err = checkStoreRoot(s.archive, s.archiveMarker); err != nil {
		return err
	}
	return nil
}
