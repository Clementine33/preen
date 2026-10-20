// Package app is the assembly root. It is the one place that knows every
// adapter at once — the repository, the filesystem walker, the encoder, the
// settings file and the HTTP transport — and it owns the process's lifetime:
// startup recovery, the background loops, the listener and the shutdown order.
// Nothing below it depends on it.
package app

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Clementine33/preen/backend/internal/adapters/coverhttp"
	"github.com/Clementine33/preen/backend/internal/adapters/ffmpeg"
	"github.com/Clementine33/preen/backend/internal/adapters/filesystem"
	"github.com/Clementine33/preen/backend/internal/adapters/httpapi"
	"github.com/Clementine33/preen/backend/internal/adapters/media"
	"github.com/Clementine33/preen/backend/internal/adapters/scraper"
	appconfig "github.com/Clementine33/preen/backend/internal/adapters/settings"
	"github.com/Clementine33/preen/backend/internal/adapters/sqlite"
	"github.com/Clementine33/preen/backend/internal/admission"
	"github.com/Clementine33/preen/backend/internal/collections"
	"github.com/Clementine33/preen/backend/internal/conversion"
	"github.com/Clementine33/preen/backend/internal/conversion/execute"
	"github.com/Clementine33/preen/backend/internal/fileops"
	"github.com/Clementine33/preen/backend/internal/inventory"
	"github.com/Clementine33/preen/backend/internal/library"
	"github.com/Clementine33/preen/backend/internal/notify"
	"github.com/Clementine33/preen/backend/internal/works/coverdelivery"
	"github.com/Clementine33/preen/backend/internal/works/refresh"

	"github.com/Clementine33/preen/backend/internal/dirrename"
	"github.com/Clementine33/preen/backend/internal/thumbnail"
	"github.com/Clementine33/preen/backend/internal/works"
	"github.com/Clementine33/preen/backend/internal/workset"
)

// Run starts the process: it opens the repository, assembles the services,
// serves until ctx is done, and returns once everything it started has stopped.
// Startup failures are fatal (log.Fatalf) so CI and dev surface them.
func Run(ctx context.Context, cfg Config) {
	if err := appconfig.SeedDefaults(cfg.ConfigDir); err != nil {
		log.Fatalf("initial configuration: %v", err)
	}
	server, err := appconfig.NewReader(cfg.ConfigDir).Server(cfg.Token, cfg.AllowLAN, cfg.Port)
	if err != nil {
		log.Fatalf("server configuration: %v", err)
	}
	var webRoot *os.Root
	if cfg.WebDir != "" {
		webRoot, err = os.OpenRoot(cfg.WebDir)
		if err != nil {
			log.Fatalf("web directory: %v", err)
		}
		defer webRoot.Close()
		index, openErr := webRoot.Open("index.html")
		if openErr != nil {
			log.Fatalf("web directory index.html: %v", openErr)
		}
		info, statErr := index.Stat()
		_ = index.Close()
		if statErr != nil || !info.Mode().IsRegular() {
			log.Fatal("web directory must contain a readable index.html file")
		}
	}
	dbPath := filepath.Join(cfg.DataDir, "cache.db")
	if ensureErr := sqlite.EnsureDBPath(dbPath); ensureErr != nil {
		log.Fatalf("ensure db path: %v", ensureErr)
	}

	openRepository := sqlite.NewRepository
	if IsDevelopmentVersion(cfg.Version) {
		openRepository = sqlite.NewDevelopmentRepository
	}
	repo, err := openRepository(dbPath)
	if err != nil {
		log.Fatalf("open repository: %v", err)
	}

	// Route std logger to stdout so host-side stdout drain also covers logs.
	log.SetOutput(os.Stdout)

	// Whether this database can give freed space back is worth one line at
	// startup: the answer is fixed when the file is created, and reclamation
	// cannot change it later (ADR 0001).
	logAutoVacuumMode(ctx, repo)

	runServer(ctx, repo, cfg, server, webRoot)
}

// Config is what the process is started with: where its data and configuration
// live, the build's version, and optional listener, token and web overrides.
type Config struct {
	DataDir   string
	ConfigDir string
	Token     string
	Version   string
	AllowLAN  *bool
	Port      *int
	WebDir    string
}

// ParseCORSOrigins splits a comma-separated allowlist into a slice, trimming
// whitespace and dropping empty segments. An empty input disables cross-origin access.
func ParseCORSOrigins(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// retentionWindows are how long the idle-time maintenance pass keeps finished
// session records: scans are disposable once their inventory has been merged,
// while a planning session is the record of a generation key and has to outlive
// the idempotency-key guarantee the workset service makes (ADR 0001).
const (
	scanRetention       = 7 * 24 * time.Hour
	generationRetention = 30 * 24 * time.Hour
)

// logAutoVacuumMode says once whether this database can return freed pages to
// the filesystem at all. A file created before incremental auto-vacuum existed
// keeps them: the mode is fixed when the file is created, and reclamation
// cannot turn it on, so the operator has to point PREEN_DATA_DIR somewhere new.
func logAutoVacuumMode(ctx context.Context, repo *sqlite.Repository) {
	mode, err := repo.AutoVacuumMode(ctx)
	if err != nil {
		log.Printf("auto_vacuum probe failed: %v", err)
		return
	}
	if mode != sqlite.AutoVacuumIncremental {
		log.Printf(
			"auto_vacuum=%d: reclaimed pages cannot be returned to the filesystem; "+
				"point PREEN_DATA_DIR at a new directory for a database that can",
			mode,
		)
	}
}

// services is the wired core: one gate, one scanning entry, one library entry
// and the workset service. It exists so the listener setup below reads as
// startup, not as assembly.
type services struct {
	gate    *admission.Gate
	library *library.Service
	scan    inventory.Service
	fileOps *fileops.Service
	workset workset.Service
	// catalog is the global policy catalog: the three policy slots and the
	// custom classifier tag library the settings pages edit.
	catalog *conversion.Catalog
	// works is the logical work catalog the library's directories bind to.
	works *works.Service
	// mediaFacts measures the inventory's unknown media facts in the
	// background, after scans (ADR 0013).
	mediaFacts *inventory.MediaEnricher
	// worksChanged tells the root stream's readers that committed catalog
	// changes (metadata, inventory, bindings) may alter what they list.
	worksChanged *notify.Hub
	// Metadata execution belongs to worksets; cover delivery owns its background queue.
	metadata *refresh.Service
	covers   *coverdelivery.Service
	rename   *dirrename.Task
}

// buildServices wires the process. Direct file management and the managed task
// paths share one admission control: a file operation is refused while a scan,
// planning session or execution is running, and starting one of those is
// refused while a file operation holds the slot (ADR 0002).
//
// Library scans, manual member refresh and inventory-based tasks share one
// pipeline. Directory-only tasks bypass it; covers synchronize exact file deltas
// and metadata's completed cover queue triggers one selected batch reconciliation.
func buildServices(repo *sqlite.Repository, configDir, dataDir string) services {
	worksChanged := &notify.Hub{}
	repo.OnCatalogChanged = worksChanged.Notify
	gate := admission.NewGate(repo.HasActiveSession)
	// The background media pass holds the gate's preemptible slot, so it
	// never reads alongside a scan's walk; every merge kicks it.
	prober := media.New("")
	mediaFacts := inventory.NewMediaEnricher(sqlite.NewScanStaging(repo), prober, gate.BeginBackground)
	librarySvc := library.NewService(repo, gate, repo, filesystem.ResolveMember)
	scanPipeline := inventory.NewPipeline(
		sqlite.NewScanStaging(repo),
		filesystem.WalkRootEntriesParallel,
		filesystem.WalkFolderEntries,
		mediaFacts.Kick,
	)
	scanSvc := inventory.NewService(
		scanPipeline,
		repo,
		gate,
		librarySvc.RecordScanState,
	)
	// Task registration: every kind a workset operation can carry. Creation
	// materializes only the requested operation and its seed draft.
	//
	// Generation refreshes inventory; conversion execution checks disk facts
	// without writing inventory. Rename also reads current
	// catalog metadata and immediate directory occupancy.
	outboundProxy := newOutboundProxy(repo)
	worksSvc := works.NewService(
		outboundProxy, repo, filesystem.ResolveMember, filesystem.ResolveMemberFile, media.ThumbEngineVersion,
	)
	scraperClient := scraper.NewProxied(outboundProxy.Resolve)
	metadataSvc := refresh.NewService(worksSvc, repo, scraperClient, scraperClient)
	coverStage, err := filesystem.NewCoverStaging(filepath.Join(dataDir, "cover-staging"))
	if err != nil {
		log.Fatalf("cover staging: %v", err)
	}
	coverSvc, err := coverdelivery.New(
		coverdelivery.Dependencies{
			Store:           repo,
			Downloader:      coverhttp.NewProxied(outboundProxy.Resolve),
			Source:          scraperClient,
			Files:           filesystem.CoverFiles{},
			Staging:         coverStage,
			Acquire:         gate.BeginManual,
			SyncInventory:   coverInventorySync(repo),
			RefreshSelected: metadataInventoryRefresh(scanPipeline),
		},
	)
	if err != nil {
		log.Fatalf("cover staging: %v", err)
	}
	metadataExecutor := refresh.Executor{Metadata: metadataSvc, Covers: coverSvc}
	renameTask := dirrename.New(repo, filesystem.ReadRenameDirectory, dirrename.ExecutionIO{
		ResolveDirectory: filesystem.ResolveMember,
		Rename:           filesystem.RenameNoReplace,
		Relocate:         repo.RelocateDirectoryAs,
		PendingCovers:    repo.HasPendingRenameCovers,
	})
	worksetSvc := workset.NewService(
		repo,
		0,
		0,
		[]workset.Task{
			conversion.New(
				repo,
				appconfig.NewReader(configDir),
				func() execute.Encoder { return ffmpeg.New(ffmpeg.ToolsConfig{}) },
				media.ProbeBitrate,
				filesystem.Recover,
				filesystem.IsResolvedWithinRoot,
				conversionIO(scanPipeline),
			),
			renameTask,
			refresh.NewTask(repo, filesystem.CheckWorkDirectory),
		},
		memberInventoryRefresh(scanSvc),
		gate.Enqueue,
		map[string]workset.BatchExecutor{refresh.Kind: metadataExecutor.Run},
	)
	worksetSvc.SetExclusiveEnqueueGuard(gate.EnqueueExclusive)
	return services{
		gate:    gate,
		rename:  renameTask,
		library: librarySvc,
		scan:    scanSvc,
		fileOps: fileops.NewService(
			gate,
			scanSvc.RefreshMember,
			filesystem.Recover,
			filesystem.RenameNoReplace,
			filesystem.IsResolvedWithinRoot,
		),
		workset:      worksetSvc,
		catalog:      conversion.NewCatalog(repo, repo, appconfig.NewReader(configDir)),
		works:        worksSvc,
		metadata:     metadataSvc,
		covers:       coverSvc,
		mediaFacts:   mediaFacts,
		worksChanged: worksChanged,
	}
}

// memberInventoryRefresh registers task-owned member walks in the shared gate.
func memberInventoryRefresh(scanSvc inventory.Service) workset.FolderScan {
	return func(ctx context.Context, folderPath, rootPath string) error {
		release, err := scanSvc.AdmitMemberRefresh(rootPath)
		if err != nil {
			return err
		}
		defer release()
		rel, err := filepath.Rel(filepath.FromSlash(rootPath), filepath.FromSlash(folderPath))
		if err != nil {
			return err
		}
		folder, err := filesystem.ResolveMember(rootPath, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		return scanSvc.RefreshMember(ctx, folder, rootPath)
	}
}

// coverInventorySync translates cover publication facts into exact inventory
// updates, without invoking scans or changing bindings and audio media facts.
func coverInventorySync(
	repo *sqlite.Repository,
) func(context.Context, coverdelivery.Target, coverdelivery.Publication) error {
	return func(_ context.Context, target coverdelivery.Target, result coverdelivery.Publication) error {
		var changed []inventory.InventoryFile
		if result.File != nil {
			changed = append(changed, *result.File)
		}
		return repo.SyncObservedInventory(
			target.LibraryRoot,
			result.Removed,
			changed,
			nil,
		)
	}
}

// startCoverWorkers restores the persistent queue before accepting requests.
func startCoverWorkers(ctx context.Context, service *coverdelivery.Service) <-chan struct{} {
	if err := service.Recover(ctx); err != nil {
		log.Fatalf("recover cover queue: %v", err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := service.Run(ctx); err != nil && ctx.Err() == nil {
			log.Printf("cover workers stopped: %v", err)
		}
	}()
	return done
}

// runServer starts the HTTP listener and blocks until the process is killed.
// Startup failures are fatal (log.Fatalf) so CI/dev surfaces them.
func runServer(
	ctx context.Context,
	repo *sqlite.Repository,
	cfg Config,
	server appconfig.ServerConfig,
	webRoot *os.Root,
) {
	httpListener, err := net.Listen("tcp", server.ListenAddress())
	if err != nil {
		log.Fatalf("http listen: %v", err)
	}
	httpPort := httpListener.Addr().(*net.TCPAddr).Port

	// The HTTP API is the only client surface. The workset service owns the
	// async planning dispatcher.
	services := buildServices(repo, cfg.ConfigDir, cfg.DataDir)

	// Startup recovery: any session left queued/running by a previous process
	// is marked interrupted before the dispatcher starts from an empty queue.
	interruptStaleSessions(repo)
	for _, err := range services.rename.RecoverInterrupted(ctx, repo) {
		log.Printf("rename startup repair failed: %v", err)
	}
	services.workset.DispatcherHandle().Start()

	// Maintenance excludes scans, sessions and file management through their gate.
	// Personal-data edits can still write concurrently. The loop ends with ctx.
	maintenanceLoop := NewMaintenanceLoop(repo, services.gate.BeginMaintenance, Options{
		ScanRetention:       scanRetention,
		GenerationRetention: generationRetention,
	})
	// The startup pass runs to completion here, before the server accepts
	// anything: nothing else is alive yet, so it is the cheapest pass the
	// process will ever run, and no client can be refused by it.
	maintenanceLoop.Pass(ctx)
	coversDone := startCoverWorkers(ctx, services.covers)
	maintenanceDone := make(chan struct{})
	go func() {
		defer close(maintenanceDone)
		maintenanceLoop.Run(ctx)
	}()
	// The startup kick picks up whatever an earlier process left unmeasured.
	services.mediaFacts.Kick()
	mediaFactsDone := make(chan struct{})
	go func() {
		defer close(mediaFactsDone)
		services.mediaFacts.Run(ctx)
	}()

	handlers := newActiveHandlers(httpapi.NewServer(httpapi.Dependencies{

		Library:        services.library,
		Catalog:        services.catalog,
		Token:          server.Token,
		CORSOrigins:    ParseCORSOrigins(os.Getenv("PREEN_CORS_ORIGINS")),
		Version:        cfg.Version,
		WebRoot:        webRoot,
		Inventory:      services.scan,
		MediaFacts:     services.mediaFacts,
		WorksChanged:   services.worksChanged,
		LocalCovers:    repo,
		WorksetService: services.workset,
		FileOps:        services.fileOps,
		Works:          services.works,
		Collections:    collections.NewService(repo),
		// A disposable cache (ADR 0013): one computation per CPU, a
		// bounded queue behind them, about 512 MiB on disk.
		Thumbs: thumbnail.New(
			media.ThumbEngine{}, filepath.Join(cfg.DataDir, "thumbs"), 512<<20,
			runtime.NumCPU(), 4*runtime.NumCPU(),
		),
		Covers: services.covers,
	}))
	requestCtx, cancelRequests := context.WithCancel(context.Background())
	defer cancelRequests()
	httpSrv := &http.Server{
		Handler:           handlers,
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return requestCtx },
	}
	httpSrv.RegisterOnShutdown(cancelRequests)
	go func() {
		if err := httpSrv.Serve(httpListener); err != nil && err != http.ErrServerClosed && ctx.Err() == nil {
			log.Fatalf("http serve: %v", err)
		}
	}()

	// Print the ready handshake BEFORE blocking — the dev and e2e launchers
	// read this line to learn the HTTP port
	//nolint:forbidigo // stdout handshake is a wire protocol for the host
	fmt.Println(BuildHandshakeLine(cfg.Version, httpPort))

	// Block until killed
	log.Printf("preen listening on http %s (data=%s)", httpListener.Addr(), cfg.DataDir)
	<-ctx.Done()
	shutdownBackend(httpSrv, handlers, services.workset.DispatcherHandle(), map[string]<-chan struct{}{
		"maintenance": maintenanceDone, "media facts": mediaFactsDone, "covers": coversDone,
	}, repo.Close, os.Exit)
}

// interruptStaleSessions marks leftover queued/running sessions of a previous
// process as interrupted (releasing generation idempotency keys). Scan sessions
// are finalized here as well: their non-terminal states make
// HasActiveScanForRoot refuse planning and execution against the root, and
// retention deletes terminal rows only, so a leftover row would otherwise never
// clear. Execution sessions keep their partial report; nothing is resumed,
// re-encoded or re-deleted.
func interruptStaleSessions(repo *sqlite.Repository) {
	if err := repo.InterruptStaleGenerations(); err != nil {
		log.Printf("interrupt stale generations failed: %v", err)
	}
	if scans, err := repo.InterruptStaleScanSessions(); err != nil {
		log.Printf("interrupt stale scan sessions failed: %v", err)
	} else if scans > 0 {
		log.Printf("interrupted %d stale scan session(s)", scans)
	}
	n, err := repo.InterruptStaleExecutions()
	if err != nil {
		log.Printf("interrupt stale executions failed: %v", err)
		return
	}
	if n > 0 {
		log.Printf("interrupted %d stale execution session(s)", n)
	}
}

// DrainHTTPServer shuts the HTTP server down gracefully; at the deadline (ctx
// done) it force-closes so the drain always returns.
func DrainHTTPServer(
	ctx context.Context,
	httpShutdown func(context.Context) error,
	httpClose func() error,
) {
	done := make(chan error, 1)
	go func() { done <- httpShutdown(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			log.Printf("http shutdown: %v", err)
		}
		if ctx.Err() != nil {
			_ = httpClose()
		}
	case <-ctx.Done():
		_ = httpClose()
	}
}

func conversionIO(scanPipeline *inventory.Pipeline) conversion.ExecutionIO {
	return conversion.ExecutionIO{
		Walk: func(ctx context.Context, roots []string, base string, emit func(string, inventory.DirEntry) error) error {
			return filesystem.WalkSelectedEntriesParallel(
				ctx,
				roots,
				base,
				inventory.DefaultScanWorkers,
				emit,
			)
		},
		Moves: filesystem.ConversionMoves{},
		Files: filesystem.ConversionHandles{},
		Scan: func(ctx context.Context, folders []string, root string, checkCancel func()) error {
			for _, folder := range folders {
				rel, relErr := filepath.Rel(filepath.FromSlash(root), filepath.FromSlash(folder))
				if relErr != nil {
					return relErr
				}
				if _, resolveErr := filesystem.ResolveMember(root, filepath.ToSlash(rel)); resolveErr != nil {
					return resolveErr
				}
			}
			_, err := scanPipeline.ScanFoldersCtx(
				ctx,
				folders,
				root,
				inventory.DefaultScanWorkers,
				inventory.WithProgress(func(inventory.Progress) { checkCancel() }),
			)
			return err
		},
	}
}
