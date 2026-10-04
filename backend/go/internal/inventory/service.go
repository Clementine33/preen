package inventory

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	pipelineBatchSize      = 1000
	taskQueueSize          = 100 // Buffered channel for backpressure
	DefaultScanWorkers     = 32
	progressEntryThreshold = 500                    // emit progress at most every 500 entries
	progressTimeInterval   = 100 * time.Millisecond // emit progress at most every 100ms
)

// Progress reports scan progress. The final report of a full scan also carries
// the binding classifications, which are known only once the merge has run.
type Progress struct {
	FilesScanned int
	DirsScanned  int

	// DirsWithMultipleNumbers counts the directories outside any work whose
	// names yielded more than one work number. A name with none is no work
	// root: the walk descends into it, so it needs no report.
	DirsWithMultipleNumbers int
	ExampleDirs             []string
	ExamplesTruncated       bool
}

type scanOptions struct {
	progress  func(Progress)
	libraryID string
}

// ScanOption configures a scan operation.
type ScanOption func(*scanOptions)

// WithProgress registers a progress callback. The callback receives throttled
// progress (at most every 500 entries or 100ms) followed by a final report
// with the complete counts when the pipeline finishes.
func WithProgress(cb func(Progress)) ScanOption {
	return func(o *scanOptions) { o.progress = cb }
}

// WithLibrary names the library a full scan belongs to. It is what lets the
// merge reconcile that library's directory bindings in the same transaction.
func WithLibrary(libraryID string) ScanOption {
	return func(o *scanOptions) { o.libraryID = libraryID }
}

// resolvePipelineError applies the pipeline failure precedence shared by the
// root and folder scans:
//   - an independent walk error (not caused by our own cancellation) is
//     primary and reported as SCAN_WALK_FAILED, even if staging also failed;
//   - otherwise a staging-write failure beats the walker's context.Canceled,
//     which is the symptom of the consumer cancelling the walk, not a cause.
func resolvePipelineError(producerErr, consumerErr error) (code string, primary error) {
	if producerErr != nil && !errors.Is(producerErr, context.Canceled) {
		return "SCAN_WALK_FAILED", producerErr
	}
	if consumerErr != nil {
		return "STAGING_WRITE_FAILED", consumerErr
	}
	return "SCAN_WALK_FAILED", producerErr
}

// progressTracker counts scanned entries and emits throttled progress.
// The first report is emitted immediately (zero-value lastReportedAt makes
// the time check true), later reports at most every progressEntryThreshold
// entries or progressTimeInterval, and finish() always emits a final report
// with the complete counts.
type progressTracker struct {
	cb             func(Progress)
	files, dirs    int
	last           Progress
	lastReportedAt time.Time
}

func newProgressTracker(cb func(Progress)) *progressTracker {
	return &progressTracker{cb: cb}
}

// record counts one scanned entry and emits throttled progress.
func (t *progressTracker) record(entry StagingEntry) {
	if t.cb == nil {
		return
	}
	if entry.IsDir {
		t.dirs++
	} else {
		t.files++
	}
	if t.files-t.last.FilesScanned >= progressEntryThreshold || time.Since(t.lastReportedAt) >= progressTimeInterval {
		t.emit()
	}
}

// finish emits a final progress report with the complete counts.
func (t *progressTracker) finish() {
	if t.cb == nil {
		return
	}
	t.emit()
}

func (t *progressTracker) emit() {
	t.last = Progress{FilesScanned: t.files, DirsScanned: t.dirs}
	t.lastReportedAt = time.Now()
	t.cb(t.last)
}

// MergeOutcome is what one merge did beyond moving rows: how many entries it
// merged, and which directories outside any work the reconciliation could not
// bind. The classification is the scan's answer for a directory whose name
// carries more than one work number - the scan does not guess, so it reports.
type MergeOutcome struct {
	Merged                  int
	DirsWithMultipleNumbers int
	ExampleDirs             []string
	ExamplesTruncated       bool
}

// StagingStore is the staging and session storage one scan needs, declared
// here by its consumer and satisfied by the SQLite adapter.
//
// MergeStaging takes the library because a full scan is what reconciles that
// library's directory bindings, and the reconciliation has to happen inside the
// same transaction as the merge. An empty library means the merge is a member
// refresh, which leaves every binding alone.
type StagingStore interface {
	WriteStagingEntries(sessionID string, entries []StagingEntry) error
	MergeStaging(sessionID, libraryID, rootPath string) (MergeOutcome, error)
	CreateScanSession(session *ScanSession) error
	UpdateScanSessionStatus(sessionID, status, errorCode, errorMessage string) error
}

// StagingEntry represents an entry to be written to staging.
type StagingEntry struct {
	SessionID  string
	Path       string
	RootPath   string
	ParentPath string
	Name       string
	IsDir      bool
	IsWorkRoot bool
	Size       int64
	Mtime      int64
	Format     string
}

func detectFormatFromPath(path string, isDir bool) string {
	if isDir {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return ""
	}
	if mt := mime.TypeByExtension(ext); mt != "" {
		if before, _, ok := strings.Cut(mt, ";"); ok {
			return strings.TrimSpace(before)
		}
		return mt
	}
	// Fallbacks for common audio extensions not always present in MIME db.
	switch ext {
	case ".mp3":
		return "audio/mpeg"
	case ".flac":
		return "audio/flac"
	case ".wav":
		return "audio/wav"
	case ".m4a":
		return "audio/mp4"
	case ".aac":
		return "audio/aac"
	case ".ogg":
		return "audio/ogg"
	case ".opus":
		return "audio/opus"
	default:
		return ""
	}
}

// Pipeline runs one scan: it walks a root (or one member directory) through
// the injected traversal, stages every observed entry through the store, and
// merges the staged rows into the inventory. The traversal is a port so this
// pipeline never reads the filesystem itself. merged is told after every
// successful merge, which is how the media enricher learns there may be facts
// to measure (enrich.go); nil when unwired.
type Pipeline struct {
	repo            StagingStore
	walkRoot        WalkRoot
	walkMember      WalkMember
	merged          func()
	rootConcurrency int
}

// NewPipeline creates a scan pipeline. walkRoot is the full-root traversal and
// walkMember the single-directory one; both are wired from the filesystem
// adapter. merged (may be nil) is called after every successful merge.
func NewPipeline(repo StagingStore, walkRoot WalkRoot, walkMember WalkMember, merged func()) *Pipeline {
	return &Pipeline{
		repo:            repo,
		walkRoot:        walkRoot,
		walkMember:      walkMember,
		merged:          merged,
		rootConcurrency: DefaultScanWorkers,
	}
}

// ScanRoot performs a full scan of the given root path using
// bounded work scanning + inline metadata + staging pipeline.
//
// Invariant: Root = bounded work scanning + inline metadata + pipeline.
func (s *Pipeline) ScanRoot(rootPath string) (string, error) {
	return s.ScanRootCtx(context.Background(), rootPath)
}

// ScanRootCtx performs a full scan of the given root path using
// bounded work scanning + inline metadata + staging pipeline.
// It is context-aware: on ctx cancellation the merge is skipped, the scan
// session is marked canceled (error_code SCAN_CANCELLED), and staging rows
// are cleaned up best-effort.
//
// Invariant: Root = bounded work scanning + inline metadata + pipeline.
//
//nolint:gocognit,funlen // producer/consumer pipeline with cancellation has many coordinated branches
func (s *Pipeline) ScanRootCtx(ctx context.Context, rootPath string, opts ...ScanOption) (string, error) {
	var o scanOptions
	for _, opt := range opts {
		opt(&o)
	}

	sessionID := uuid.New().String()

	// Normalize root_path to POSIX for consistent storage and matching
	normalizedRootPath := filepath.ToSlash(rootPath)

	// Create scan session
	session := &ScanSession{
		SessionID: sessionID,
		RootPath:  normalizedRootPath,
		Kind:      "full",
		Status:    "running",
		StartedAt: time.Now(),
	}

	if s.repo != nil {
		if err := s.repo.CreateScanSession(session); err != nil {
			return "", fmt.Errorf("create scan session: %w", err)
		}
	}

	// Run pipeline: walk + batch write to staging
	//
	// CONTRACT:
	// - producer emits to entryCh (buffered with backpressure)
	// - consumer batches and writes (batchSize=1000)
	// - producer closes entryCh when done
	// - consumer exits on entryCh close or ctx cancellation
	// - first error from either triggers ctx.cancel and stops both
	// - after waiting both, if any error, cleanup staging and mark failed
	// - on caller ctx cancellation: skip merge, mark canceled, cleanup staging best-effort
	baseCtx := ctx
	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	entryCh := make(chan StagingEntry, taskQueueSize)
	var producerErr, consumerErr error
	var wg sync.WaitGroup

	// Producer: walk and emit to channel
	wg.Go(func() {
		defer close(entryCh) // Only close when producer exits

		emitFn := func(entry DirEntry) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case entryCh <- DirEntryToStagingEntry(entry, sessionID, normalizedRootPath):
				return nil
			}
		}

		if err := s.walkRoot(ctx, rootPath, rootPath, s.rootConcurrency, emitFn); err != nil {
			producerErr = err
			cancel()
		}
	})

	// Consumer: batch writes from channel
	var cleanupFn func(string) error
	if pipeline, ok := s.repo.(pipelineRepo); ok {
		cleanupFn = pipeline.CleanupStagingSession
	}

	// The consumer's final counts outlive its goroutine: the report after the
	// merge repeats them so the caller sees one event carrying both the scan's
	// own counts and what the merge classified.
	var scanned Progress

	wg.Go(func() {
		batch := make([]StagingEntry, 0, pipelineBatchSize)
		progress := newProgressTracker(o.progress)

		for entry := range entryCh {
			progress.record(entry)

			batch = append(batch, entry)

			if len(batch) >= pipelineBatchSize {
				// Flush batch
				if s.repo != nil {
					if pipeline, ok := s.repo.(pipelineRepo); ok {
						if err := pipeline.WriteStagingBatch(sessionID, batch); err != nil {
							consumerErr = err
							cancel()
							return
						}
					} else {
						consumerErr = fmt.Errorf("repo does not support pipeline batch writing")
						cancel()
						return
					}
				}
				batch = batch[:0] // Clear batch for reuse
			}
		}

		// Flush final batch
		if len(batch) > 0 && s.repo != nil {
			if pipeline, ok := s.repo.(pipelineRepo); ok {
				if err := pipeline.WriteStagingBatch(sessionID, batch); err != nil {
					consumerErr = err
				}
			}
		}

		// Final progress report with complete counts
		progress.finish()
		scanned = progress.last
	})

	// Wait for both producer and consumer to finish
	wg.Wait()

	// Cancellation (caller context): skip merge, mark canceled, cleanup staging.
	if err := baseCtx.Err(); err != nil {
		if s.repo != nil {
			_ = s.repo.UpdateScanSessionStatus(sessionID, "canceled", "SCAN_CANCELLED", err.Error())
		}
		if cleanupFn != nil {
			_ = cleanupFn(sessionID) // Best-effort cleanup
		}
		return "", err
	}

	// Check for errors from either
	if producerErr != nil || consumerErr != nil {
		// Try to cleanup staging on failure
		// Note: Partial persistence is expected - cleanup is best-effort
		var cleanupFailure error
		if cleanupFn != nil {
			cleanupFailure = cleanupFn(sessionID)
		}

		// Update session status to failed
		code, primaryErr := resolvePipelineError(producerErr, consumerErr)
		if s.repo != nil {
			_ = s.repo.UpdateScanSessionStatus(sessionID, "failed", code, primaryErr.Error())
		}

		// Return primary error (not cleanup failure). A staging-write failure
		// wins over a concurrently observed walker cancellation: the consumer
		// cancels the derived context to stop the walk, so the producer
		// commonly records context.Canceled that is a symptom, not the cause.
		// An independent walk error (not caused by our cancellation) stays
		// primary even when staging also failed.
		_ = cleanupFailure // We could log this, but primary error is what's expected
		if errors.Is(primaryErr, consumerErr) {
			return "", fmt.Errorf("write staging: %w", consumerErr)
		}
		return "", fmt.Errorf("walk directory: %w", producerErr)
	}

	// Both succeeded - proceed to merge
	if s.repo != nil {
		// Transition to merging status before merge
		if err := s.repo.UpdateScanSessionStatus(sessionID, "merging", "", ""); err != nil {
			return "", fmt.Errorf("update session status to merging: %w", err)
		}

		// Merge into main catalog
		outcome, err := s.repo.MergeStaging(sessionID, o.libraryID, normalizedRootPath)
		if err != nil {
			_ = s.repo.UpdateScanSessionStatus(sessionID, "failed", "MERGE_FAILED", err.Error())
			return "", fmt.Errorf("merge staging: %w", err)
		}

		// The merged entries' unknown media facts are measured in the
		// background, after this scan releases the slot (ADR 0015 §2).
		s.notifyMerged()

		// The classifications are only known once the bindings have been
		// reconciled, so they travel in a report of their own rather than in the
		// per-entry progress above.
		if o.progress != nil {
			o.progress(Progress{
				FilesScanned:            scanned.FilesScanned,
				DirsScanned:             scanned.DirsScanned,
				DirsWithMultipleNumbers: outcome.DirsWithMultipleNumbers,
				ExampleDirs:             outcome.ExampleDirs,
				ExamplesTruncated:       outcome.ExamplesTruncated,
			})
		}

		if err := s.repo.UpdateScanSessionStatus(sessionID, "completed", "", ""); err != nil {
			return "", fmt.Errorf("update session status to completed: %w", err)
		}
	}

	return sessionID, nil
}

// ScanFolder performs a scoped scan of a single folder using
// single-enumerator directory discovery + inline metadata + staging pipeline.
//
// Invariant: Folder = single-enumerator directory discovery + inline metadata + pipeline.
func (s *Pipeline) ScanFolder(folderPath, rootPath string) (string, error) {
	return s.ScanFolderCtx(context.Background(), folderPath, rootPath)
}

// ScanFolderCtx performs a scoped scan of a single folder using
// single-enumerator directory discovery + inline metadata + staging pipeline.
// It is context-aware: on ctx cancellation the merge is skipped, the scan
// session is marked canceled (error_code SCAN_CANCELLED), and staging rows
// are cleaned up best-effort.
//
// Invariant: Folder = single-enumerator directory discovery + inline metadata + pipeline.
//
//nolint:gocognit,funlen // mirrors the root-scan pipeline branches (folder-scoped)
func (s *Pipeline) ScanFolderCtx(
	ctx context.Context,
	folderPath, rootPath string,
	opts ...ScanOption,
) (string, error) {
	var o scanOptions
	for _, opt := range opts {
		opt(&o)
	}

	sessionID := uuid.New().String()

	// Normalize paths to POSIX for consistent storage and matching
	normalizedRootPath := filepath.ToSlash(rootPath)
	scopePath := filepath.ToSlash(folderPath)
	session := &ScanSession{
		SessionID: sessionID,
		RootPath:  normalizedRootPath,
		ScopePath: &scopePath,
		Kind:      "folder",
		Status:    "running",
		StartedAt: time.Now(),
	}

	if s.repo != nil {
		if err := s.repo.CreateScanSession(session); err != nil {
			return "", fmt.Errorf("create scan session: %w", err)
		}
	}

	// Run pipeline: walk + batch write to staging
	//
	// CONTRACT (Folder-specific):
	// - producer emits to entryCh (buffered with backpressure)
	// - Must NOT pre-aggregate into []DirEntry - must stream immediately
	// - producer is the ONLY close(entryCh) caller
	// - writer triggers cancel() on first error
	// - producer respects ctx.Done() and stops emitting
	// - main waits for both before merge decision
	baseCtx := ctx
	ctx, cancel := context.WithCancel(baseCtx)
	defer cancel()

	entryCh := make(chan StagingEntry, taskQueueSize)
	var producerErr, consumerErr error
	var wg sync.WaitGroup

	// Producer: walk and emit to channel (STREAMING, NO PRE-AGGREGATION)
	wg.Go(func() {
		defer close(entryCh) // Producer owns close

		emitFn := func(entry DirEntry) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case entryCh <- DirEntryToStagingEntry(entry, sessionID, normalizedRootPath):
				return nil
			}
		}

		if err := s.walkMember(ctx, folderPath, rootPath, emitFn); err != nil {
			producerErr = err
			cancel()
		}
	})

	// Consumer: batch writes from channel
	var cleanupFn func(string) error
	if pipeline, ok := s.repo.(pipelineRepo); ok {
		cleanupFn = pipeline.CleanupStagingSession
	}

	wg.Go(func() {
		batch := make([]StagingEntry, 0, pipelineBatchSize)
		progress := newProgressTracker(o.progress)

		for entry := range entryCh {
			progress.record(entry)

			batch = append(batch, entry)

			if len(batch) >= pipelineBatchSize {
				if s.repo != nil {
					if pipeline, ok := s.repo.(pipelineRepo); ok {
						if err := pipeline.WriteStagingBatch(sessionID, batch); err != nil {
							consumerErr = fmt.Errorf("write staging batch: %w", err)
							cancel()
							return
						}
					} else {
						consumerErr = fmt.Errorf("repo does not support pipeline batch writing")
						cancel()
						return
					}
				}
				batch = batch[:0] // Clear for reuse
			}
		}

		// Flush final batch
		if len(batch) > 0 && s.repo != nil {
			if pipeline, ok := s.repo.(pipelineRepo); ok {
				if err := pipeline.WriteStagingBatch(sessionID, batch); err != nil {
					consumerErr = fmt.Errorf("write final staging batch: %w", err)
				}
			}
		}

		// Final progress report with complete counts
		progress.finish()
	})

	// Wait for both producer and consumer
	wg.Wait()

	// Cancellation (caller context): skip merge, mark canceled, cleanup staging.
	if err := baseCtx.Err(); err != nil {
		if s.repo != nil {
			_ = s.repo.UpdateScanSessionStatus(sessionID, "canceled", "SCAN_CANCELLED", err.Error())
		}
		if cleanupFn != nil {
			_ = cleanupFn(sessionID) // Best-effort cleanup
		}
		return "", err
	}

	// Check for errors
	if producerErr != nil || consumerErr != nil {
		// Cleanup staging on failure
		var cleanupFailure error
		if cleanupFn != nil {
			cleanupFailure = cleanupFn(sessionID)
		}

		// Update session status
		code, primaryErr := resolvePipelineError(producerErr, consumerErr)
		if s.repo != nil {
			_ = s.repo.UpdateScanSessionStatus(sessionID, "failed", code, primaryErr.Error())
		}

		_ = cleanupFailure // Best effort cleanup
		if errors.Is(primaryErr, consumerErr) {
			return "", consumerErr
		}
		return "", producerErr
	}

	// Both succeeded - merge
	if s.repo != nil {
		if err := s.repo.UpdateScanSessionStatus(sessionID, "merging", "", ""); err != nil {
			return "", err
		}

		// A member refresh reconciles nothing: it is scoped to one subtree, and
		// the library's work roots are not its to rewrite.
		if _, err := s.repo.MergeStaging(sessionID, "", normalizedRootPath); err != nil {
			_ = s.repo.UpdateScanSessionStatus(sessionID, "failed", "MERGE_FAILED", err.Error())
			return "", err
		}
		s.notifyMerged()

		if err := s.repo.UpdateScanSessionStatus(sessionID, "completed", "", ""); err != nil {
			return "", err
		}
	}

	return sessionID, nil
}

// notifyMerged tells the wired listener that the inventory changed.
func (s *Pipeline) notifyMerged() {
	if s.merged != nil {
		s.merged()
	}
}

// DirEntryToStagingEntry converts a DirEntry to StagingEntry with format detection.
func DirEntryToStagingEntry(entry DirEntry, sessionID, rootPath string) StagingEntry {
	return StagingEntry{
		SessionID:  sessionID,
		Path:       filepath.ToSlash(entry.Path),
		RootPath:   filepath.ToSlash(rootPath),
		ParentPath: filepath.ToSlash(entry.ParentPath),
		Name:       entry.Name,
		IsDir:      entry.IsDir,
		IsWorkRoot: entry.IsWorkRoot,
		Size:       entry.Size,
		Mtime:      entry.Mtime,
		Format:     detectFormatFromPath(entry.Path, entry.IsDir),
	}
}
