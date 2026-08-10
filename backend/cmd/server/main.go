package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	stdhttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/zenith-wang/it-wiki/backend/internal/agent"
	agenttools "github.com/zenith-wang/it-wiki/backend/internal/agent/tools"
	"github.com/zenith-wang/it-wiki/backend/internal/config"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	httpx "github.com/zenith-wang/it-wiki/backend/internal/http"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/embedder"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/feishu"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/llm"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/parser"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/splitter"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/storage"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/tokenizer"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/vectorstore"
	"github.com/zenith-wang/it-wiki/backend/internal/repo"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/generated"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/migrations"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
	"github.com/zenith-wang/it-wiki/backend/internal/worker"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "[fatal] %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	bootCtx, bootCancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer bootCancel()

	if err := runGooseMigrations(bootCtx, cfg.DatabaseURL); err != nil {
		return fmt.Errorf("goose migrations: %w", err)
	}

	pool, err := repo.NewPool(bootCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("pgx pool: %w", err)
	}
	defer pool.Close()

	rmgr, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("rivermigrate new: %w", err)
	}
	if _, err := rmgr.Migrate(bootCtx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("rivermigrate up: %w", err)
	}

	if err := checkEmbeddingDim(bootCtx, pool, cfg.EmbeddingDim); err != nil {
		return fmt.Errorf("embedding dim check: %w", err)
	}
	if err := service.NewOwnershipBootstrap(repo.NewOwnershipBootstrapRepository(pool)).Run(bootCtx, cfg.BootstrapOwnerFeishuOpenID); err != nil {
		return fmt.Errorf("ownership bootstrap: %w", err)
	}

	mc, err := storage.NewMinioClient(bootCtx, storage.MinioConfig{
		Endpoint:     cfg.S3Endpoint,
		AccessKey:    cfg.S3AccessKey,
		SecretKey:    cfg.S3SecretKey,
		Region:       cfg.S3Region,
		Bucket:       cfg.S3Bucket,
		UsePathStyle: cfg.S3UsePathStyle,
	})
	if err != nil {
		return fmt.Errorf("minio client: %w", err)
	}
	if err := mc.EnsureBucket(bootCtx); err != nil {
		return fmt.Errorf("ensure bucket: %w", err)
	}
	fmt.Printf("[main] minio bucket %q ready\n", cfg.S3Bucket)

	if err := tokenizer.Init(cfg.TokenizerEncoding); err != nil {
		return fmt.Errorf("tokenizer init: %w", err)
	}

	queries := generated.New(pool)
	authHandler, authService, err := buildAuthRuntime(cfg, pool, stdhttp.DefaultClient)
	if err != nil {
		return fmt.Errorf("build auth runtime: %w", err)
	}
	parserDispatcher := parser.NewDispatcher()
	split := splitter.New()
	embed := embedder.New(embedder.Config{
		BaseURL: cfg.EmbeddingBaseURL,
		APIKey:  cfg.EmbeddingAPIKey,
		Model:   cfg.EmbeddingModel,
		Dim:     cfg.EmbeddingDim,
	})
	vstore := vectorstore.New(pool)
	llmClient := llm.New(llm.Config{
		BaseURL: cfg.LLMBaseURL,
		APIKey:  cfg.LLMAPIKey,
		Model:   cfg.LLMModel,
	})

	ingestionWorker := worker.NewIngestionWorker(worker.WorkerDeps{
		Pool: pool, Queries: queries, Storage: mc,
		Parser:   parserDispatcher,
		Splitter: split, Embedder: embed, VStore: vstore, StagedVStore: vstore,
		ChunkSize: cfg.ChunkSize, Overlap: cfg.ChunkOverlap, BatchSize: cfg.EmbedBatchSize,
		JobTimeout: cfg.IngestionJobTimeout,
	})
	var rclient *worker.Client
	riverConfig := worker.RiverClientConfig{
		IngestionWorker: ingestionWorker, MaxWorkers: cfg.RiverMaxWorkers,
		RescueStuckJobsAfter: cfg.RiverRescueStuckJobsAfter,
	}
	if authService != nil {
		apiClient := feishu.NewClient(feishu.ClientConfig{}, stdhttp.DefaultClient)
		docxLoader := feishu.NewDocxLoader(apiClient)
		sheetLoader := feishu.NewSheetLoader(apiClient)
		bitableLoader := feishu.NewBitableLoader(apiClient, feishu.BitableConfig{})
		wikiLoader := feishu.NewWikiLoader(apiClient, docxLoader, sheetLoader, bitableLoader)
		syncRepository := worker.NewSQLFeishuSyncRepository(pool)
		queueForwarder := worker.StagedIngestionEnqueuerFuncs{
			EnqueueFunc: func(ctx context.Context, snapshot worker.PendingFeishuSnapshot) error {
				if rclient == nil {
					return errors.New("River client unavailable")
				}
				return rclient.EnqueueStagedIngestion(ctx, snapshot)
			},
			EnqueueTxFunc: func(ctx context.Context, tx pgx.Tx, snapshot worker.PendingFeishuSnapshot) error {
				if rclient == nil {
					return errors.New("River client unavailable")
				}
				return rclient.EnqueueStagedIngestionTx(ctx, tx, snapshot)
			},
			EnqueueFeishuSyncFunc: func(ctx context.Context, documentID, revision string) error {
				if rclient == nil {
					return errors.New("River client unavailable")
				}
				return rclient.EnqueueFeishuSync(ctx, documentID, revision)
			},
			EnqueueMetadataOnlyFunc: func(ctx context.Context, snapshot worker.PendingFeishuSnapshot) error {
				if rclient == nil {
					return errors.New("River client unavailable")
				}
				return rclient.EnqueueMetadataOnlyIngestion(ctx, snapshot)
			},
		}
		feishuSyncWorker := worker.NewFeishuSyncWorker(worker.FeishuSyncWorkerDeps{
			Repository: syncRepository,
			Resolver:   feishu.NewURLResolver(),
			Loaders: map[domain.ResourceType]ports.SourceLoader{
				domain.ResourceDocx: docxLoader, domain.ResourceSheet: sheetLoader,
				domain.ResourceBitable: bitableLoader, domain.ResourceWiki: wikiLoader,
			},
			Tokens: authService, Storage: mc,
			CitationStore: vstore,
			Ingestion:     queueForwarder, JobTimeout: cfg.FeishuSyncJobTimeout, SyncLease: cfg.FeishuSyncLease,
			Logger: slog.Default(),
		})
		reconcileWorker := worker.NewFeishuReconcileWorker(worker.FeishuReconcileWorkerDeps{
			Repository: syncRepository, Queue: queueForwarder, SyncLease: cfg.FeishuSyncLease,
			JobTimeout: cfg.FeishuReconcileJobTimeout, BatchSize: cfg.FeishuReconcileBatchSize,
			MaxBatches: cfg.FeishuReconcileMaxBatches, Logger: slog.Default(),
		})
		riverConfig.FeishuSyncWorker = feishuSyncWorker
		riverConfig.ReconcileWorker = reconcileWorker
		riverConfig.ReconcileInterval = cfg.FeishuReconcileInterval
	}
	rclient, err = worker.NewConfiguredClient(bootCtx, pool, riverConfig)
	if err != nil {
		return fmt.Errorf("river client: %w", err)
	}

	kbSvc := service.NewKB(queries, cfg.EmbeddingModel, cfg.EmbeddingDim)
	docSvc := service.NewDocument(queries, mc)
	ingestionSvc := service.NewIngestion(queries, mc, rclient)
	retrievalSvc := service.NewRetrieval(embed, vstore, cfg.RAGTopK, cfg.RAGMinScore)
	runnerFactory := agent.NewFactory()
	if err := runnerFactory.Register(ports.DefaultAgentID, func() (ports.AgentRunner, error) {
		return agent.New(llmClient, cfg.LLMModel, 5), nil
	}); err != nil {
		return fmt.Errorf("register knowledge-rag runner: %w", err)
	}
	agentRegistry, err := runnerFactory.BuildRegistry()
	if err != nil {
		return fmt.Errorf("build agent registry: %w", err)
	}
	toolRegistry := agent.NewToolRegistry()
	if err := toolRegistry.Register("kb_retrieval", func(ctx context.Context, kbID string, callback ports.RetrievalCallback) (ports.Tool, error) {
		return agenttools.NewKBRetrieval(retrievalSvc, service.OwnerIDFromContext(ctx), kbID, callback), nil
	}); err != nil {
		return fmt.Errorf("register kb_retrieval tool: %w", err)
	}
	if err := toolRegistry.Register("list_documents", func(ctx context.Context, kbID string, _ ports.RetrievalCallback) (ports.Tool, error) {
		return agenttools.NewListDocuments(docSvc, service.OwnerIDFromContext(ctx), kbID), nil
	}); err != nil {
		return fmt.Errorf("register list_documents tool: %w", err)
	}
	if err := toolRegistry.RegisterAgent(ports.DefaultAgentID, "kb_retrieval", "list_documents"); err != nil {
		return fmt.Errorf("register knowledge-rag tools: %w", err)
	}
	chatSvc := service.NewChat(queries, retrievalSvc, llmClient, cfg.LLMModel, cfg.RAGHistoryMessages, agentRegistry, toolRegistry)
	feishuHandler := httpx.NewFeishuHandler(
		service.NewFeishuAccounts(queries),
		service.NewFeishuImport(feishu.NewURLResolver(), service.NewSQLFeishuImportRepository(pool), rclient),
		service.NewFeishuSync(queries, rclient),
	)
	router := httpx.NewRouter(httpx.Handlers{
		KB:     httpx.NewKBHandler(kbSvc),
		Doc:    httpx.NewDocumentHandler(docSvc, ingestionSvc, cfg.UploadMaxBytes),
		Chunk:  httpx.NewChunkHandler(vstore, docSvc),
		Chat:   httpx.NewChatHandler(chatSvc),
		Auth:   authHandler,
		Feishu: feishuHandler,
	})

	runCtx, runCancel := context.WithCancel(context.Background())
	defer runCancel()
	if err := rclient.Start(runCtx); err != nil {
		return fmt.Errorf("river start: %w", err)
	}

	srv := &stdhttp.Server{
		Addr:              ":" + cfg.Port,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serveErr := make(chan error, 1)
	go func() {
		fmt.Printf("[server] listening on :%s\n", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			serveErr <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-stop:
		fmt.Printf("[server] received %v, shutting down\n", sig)
	case err := <-serveErr:
		return fmt.Errorf("http listen: %w", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	_ = srv.Shutdown(shutdownCtx)
	_ = rclient.Stop(shutdownCtx)
	return nil
}

func runGooseMigrations(ctx context.Context, dbURL string) error {
	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		return err
	}
	defer db.Close()

	goose.SetBaseFS(migrations.EmbedMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.UpContext(ctx, db, ".")
}

func checkEmbeddingDim(ctx context.Context, pool *pgxpool.Pool, want int) error {
	var attmod int
	row := pool.QueryRow(ctx, `
		SELECT atttypmod FROM pg_attribute
		 WHERE attrelid = 'chunks'::regclass
		   AND attname  = 'embedding'`)
	if err := row.Scan(&attmod); err != nil {
		return fmt.Errorf("query atttypmod: %w", err)
	}
	if attmod != want {
		return fmt.Errorf("chunks.embedding dim = %d, expected EMBEDDING_DIM = %d (run a new migration to recreate)", attmod, want)
	}
	return nil
}
