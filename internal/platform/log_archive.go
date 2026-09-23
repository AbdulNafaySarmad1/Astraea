package platform

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
)

type archivedBatch struct {
	tenant, connector, batch, source, plaintextHash, archiveHash string
	received                                                     time.Time
	attempts                                                     int
	cleanupAttempts                                              int
}

// RunArchive is deployed separately from the ordinary job worker. It alone
// needs the archive mount and permission to delete verified hot copies.
func (w *Worker) RunArchive(ctx context.Context) error {
	storage, err := openLogStorage(w.Config)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			w.processLogArchive(ctx, storage)
		}
	}
}

func (w *Worker) archiveOne(ctx context.Context, storage *logStorage) error {
	tx, err := w.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var record archivedBatch
	err = tx.QueryRow(ctx, `SELECT tenant_id,connector_id,batch_id,source_name,plaintext_sha256,received_at,archive_attempts
 FROM log_batches WHERE status='hot' AND received_at<=now()-interval '365 days' AND archive_retry_at<=now()
 ORDER BY received_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&record.tenant, &record.connector, &record.batch, &record.source, &record.plaintextHash, &record.received, &record.attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !logSourceName.MatchString(record.source) {
		return w.deferArchive(ctx, tx, record, errors.New("archive metadata invalid"))
	}
	raw, err := storage.readHot(record.tenant, record.connector, record.batch)
	if err != nil {
		return w.deferArchive(ctx, tx, record, err)
	}
	record.archiveHash, err = storage.archiveAndVerify(record.tenant, record.connector, record.source, record.batch, record.received, record.plaintextHash, raw)
	if err != nil {
		return w.deferArchive(ctx, tx, record, err)
	}
	_, err = tx.Exec(ctx, "UPDATE log_batches SET status='archived',archive_sha256=$4,archived_at=now(),archive_last_error=NULL WHERE tenant_id=$1 AND connector_id=$2 AND batch_id=$3", record.tenant, record.connector, record.batch, record.archiveHash)
	if err == nil {
		_, err = w.Store.AuditTx(ctx, tx, &record.tenant, "worker:log-archive", "service_logs.archived", record.source, record.batch, "success", "", map[string]any{"batch_id": record.batch, "archive_sha256": record.archiveHash}, false)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *Worker) deferArchive(ctx context.Context, tx pgx.Tx, record archivedBatch, reason error) error {
	attempts := record.attempts + 1
	status := "hot"
	if attempts >= 8 {
		status = "blocked"
	}
	delay := time.Duration(1<<min(attempts, 8)) * time.Minute
	_, err := tx.Exec(ctx, `UPDATE log_batches SET archive_attempts=$4,status=$5,archive_retry_at=now()+$6::interval,archive_last_error=$7
 WHERE tenant_id=$1 AND connector_id=$2 AND batch_id=$3`, record.tenant, record.connector, record.batch, attempts, status, fmt.Sprintf("%d seconds", int(delay.Seconds())), truncate(reason.Error(), 250))
	if err == nil {
		_, err = w.Store.AuditTx(ctx, tx, &record.tenant, "worker:log-archive", "service_logs.archive_deferred", record.source, record.batch, status, "", map[string]any{"attempt": attempts, "status": status}, false)
	}
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return reason
}

func (w *Worker) cleanupArchivedOne(ctx context.Context, storage *logStorage) error {
	tx, err := w.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var record archivedBatch
	err = tx.QueryRow(ctx, `SELECT tenant_id,connector_id,batch_id,source_name,plaintext_sha256,archive_sha256,received_at,cleanup_attempts
 FROM log_batches WHERE status='archived' AND legal_hold=false AND hot_deleted_at IS NULL AND cleanup_retry_at<=now()
 ORDER BY archived_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&record.tenant, &record.connector, &record.batch, &record.source, &record.plaintextHash, &record.archiveHash, &record.received, &record.cleanupAttempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if !logSourceName.MatchString(record.source) || record.archiveHash == "" {
		return w.deferCleanup(ctx, tx, record, errors.New("archive cleanup metadata invalid"))
	}
	if err = storage.verifyArchive(record.tenant, record.connector, record.source, record.batch, record.received, record.plaintextHash, record.archiveHash); err != nil {
		return w.deferCleanup(ctx, tx, record, err)
	}
	if err = checkStoreRoot(storage.hot, storage.hotMarker); err != nil {
		return w.deferCleanup(ctx, tx, record, err)
	}
	path := storage.hotPath(record.tenant, record.connector, record.batch)
	if err = os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return w.deferCleanup(ctx, tx, record, err)
	}
	if err = checkStoreRoot(storage.hot, storage.hotMarker); err != nil {
		return w.deferCleanup(ctx, tx, record, err)
	}
	_, err = tx.Exec(ctx, "UPDATE log_batches SET hot_deleted_at=now(),cleanup_last_error=NULL WHERE tenant_id=$1 AND connector_id=$2 AND batch_id=$3", record.tenant, record.connector, record.batch)
	if err == nil {
		_, err = w.Store.AuditTx(ctx, tx, &record.tenant, "worker:log-archive", "service_logs.hot_copy_deleted", record.source, record.batch, "success", "", map[string]any{"batch_id": record.batch, "archive_sha256": record.archiveHash}, false)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *Worker) deferCleanup(ctx context.Context, tx pgx.Tx, record archivedBatch, reason error) error {
	attempts := min(record.cleanupAttempts+1, 1000000)
	delay := time.Duration(1<<min(attempts, 6)) * time.Minute
	_, err := tx.Exec(ctx, `UPDATE log_batches SET cleanup_attempts=$4,cleanup_retry_at=now()+$5::interval,cleanup_last_error=$6
 WHERE tenant_id=$1 AND connector_id=$2 AND batch_id=$3`, record.tenant, record.connector, record.batch, attempts, fmt.Sprintf("%d seconds", int(delay.Seconds())), truncate(reason.Error(), 250))
	if err == nil {
		_, err = w.Store.AuditTx(ctx, tx, &record.tenant, "worker:log-archive", "service_logs.hot_cleanup_deferred", record.source, record.batch, "retry", "", map[string]any{"attempt": attempts}, false)
	}
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	return reason
}

func (w *Worker) processLogArchive(ctx context.Context, storage *logStorage) {
	workCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := checkStoreRoot(storage.hot, storage.hotMarker); err != nil {
		log.Printf("service log hot store unavailable: %v", err)
		return
	}
	if err := checkStoreRoot(storage.archive, storage.archiveMarker); err != nil {
		log.Printf("service log archive mount unavailable: %v", err)
		return
	}
	if err := w.archiveOne(workCtx, storage); err != nil {
		log.Printf("service log archive deferred: %v", err)
	}
	if err := w.cleanupArchivedOne(workCtx, storage); err != nil {
		log.Printf("service log hot-copy cleanup deferred: %v", err)
	}
}
