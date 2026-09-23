package platform

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogArchiveVerifiesBeforeHotRemoval(t *testing.T) {
	root := t.TempDir()
	hot, archive := filepath.Join(root, "hot"), filepath.Join(root, "archive")
	for path, marker := range map[string]string{hot: "hot-store-test", archive: "archive-store-test"} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, ".aegisops-store-id"), []byte(marker+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	storage, err := openLogStorage(Config{LogHotDir: hot, LogArchiveDir: archive, LogKeyB64: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{42}, 32)), LogHotMarker: "hot-store-test", LogArchiveMarker: "archive-store-test"})
	if err != nil {
		t.Fatal(err)
	}
	tenant, connector, batch, source := "tenant-a", "connector-a", "0123456789abcdef0123456789abcdef", "primary_db"
	plain := []byte("\"service restarted\"\n")
	hash := logDigest(plain)
	if err := storage.saveHot(tenant, connector, batch, plain); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(storage.hotPath(tenant, connector, batch)); err != nil {
		t.Fatal(err)
	}
	received := time.Now().UTC().Truncate(time.Microsecond)
	archiveHash, err := storage.archiveAndVerify(tenant, connector, source, batch, received, hash, plain)
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.verifyArchive(tenant, connector, source, batch, received, hash, archiveHash); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(archive, ".aegisops-store-id")); err != nil {
		t.Fatal(err)
	}
	if err := storage.verifyArchive(tenant, connector, source, batch, received, hash, archiveHash); err == nil {
		t.Fatal("missing archive mount marker accepted")
	}
	if err := os.WriteFile(filepath.Join(archive, ".aegisops-store-id"), []byte("archive-store-test\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(storage.hotPath(tenant, connector, batch)); err != nil {
		t.Fatal("archive creation removed the hot copy before database confirmation")
	}
	archivePath := storage.archivePath(tenant, source, batch, received)
	if err := os.WriteFile(archivePath, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := storage.verifyArchive(tenant, connector, source, batch, received, hash, archiveHash); err == nil {
		t.Fatal("tampered archive passed verification")
	}
}

func TestEnrollmentScopeBindsLogSources(t *testing.T) {
	if validEnrollmentScope([]string{"service_logs"}, nil) {
		t.Fatal("unbounded log capability accepted")
	}
	if validEnrollmentScope([]string{"tcp_health"}, []logSourceApproval{{Name: "primary_db", Kind: "postgres", PathSHA256: strings.Repeat("a", 64)}}) {
		t.Fatal("log source accepted without log capability")
	}
	if !validEnrollmentScope([]string{"tcp_health", "service_logs"}, []logSourceApproval{{Name: "primary_db", Kind: "postgres", PathSHA256: strings.Repeat("a", 64)}}) {
		t.Fatal("reviewed log scope rejected")
	}
	if validEnrollmentScope([]string{"service_logs"}, []logSourceApproval{{Name: "../escape", Kind: "postgres", PathSHA256: strings.Repeat("a", 64)}}) {
		t.Fatal("unsafe log source name accepted")
	}
}

func TestLogStorageRejectsOverlappingRoots(t *testing.T) {
	root := t.TempDir()
	hot := filepath.Join(root, "hot")
	if err := os.Mkdir(hot, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hot, ".aegisops-store-id"), []byte("hot-store-test"), 0600); err != nil {
		t.Fatal(err)
	}
	config := Config{LogHotDir: hot, LogArchiveDir: filepath.Join(hot, "archive"), LogHotMarker: "hot-store-test", LogArchiveMarker: "archive-store-test", LogKeyB64: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{42}, 32))}
	if _, err := openLogStorage(config); err == nil {
		t.Fatal("nested archive root accepted")
	}
}
