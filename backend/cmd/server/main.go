package main

import (
	"context"
	"errors"
	"fmt"
	stdhttp "net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	authstore "github.com/zenith-wang/it-wiki/backend/internal/auth"
	"github.com/zenith-wang/it-wiki/backend/internal/config"
	httpx "github.com/zenith-wang/it-wiki/backend/internal/http"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/storage"
	"github.com/zenith-wang/it-wiki/backend/internal/playground"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"github.com/zenith-wang/it-wiki/backend/internal/repo"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/migrations"
	"github.com/zenith-wang/it-wiki/backend/internal/skillbuilder"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "[fatal] %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadPlatform()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	bootCtx, bootCancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer bootCancel()

	if err := runGooseMigrations(bootCtx, cfg.DatabaseURL, cfg.DatabaseProxyURL); err != nil {
		return fmt.Errorf("goose migrations: %w", err)
	}

	pool, err := repo.NewPool(bootCtx, cfg.DatabaseURL, cfg.DatabaseProxyURL)
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

	authHandler, err := buildAuthHandler(cfg, pool)
	if err != nil {
		return fmt.Errorf("build auth runtime: %w", err)
	}
	protector, err := authstore.NewAESGCMProtector([]byte(cfg.OAuthEncryptionKey))
	if err != nil {
		return fmt.Errorf("model key encryption: %w", err)
	}
	modelClient, err := playground.NewHTTPClient(os.Getenv("PLAYGROUND_TRUSTED_ORIGINS"))
	if err != nil {
		return err
	}
	modelService := playground.New(repo.NewPlaygroundRepository(pool), protector, modelClient)
	registryRepo := repo.NewRegistryRepository(pool)
	if err := registryRepo.BootstrapAdmin(bootCtx, cfg.BootstrapAdminFeishuOpenID); err != nil {
		return fmt.Errorf("bootstrap platform admin: %w", err)
	}
	registryService := registry.New(registryRepo, mc)
	router := httpx.NewCapabilityHubRouter(httpx.Handlers{
		Auth:         authHandler,
		Playground:   httpx.NewPlaygroundHandler(modelService),
		Registry:     httpx.NewRegistryHandler(registryService),
		SkillBuilder: httpx.NewSkillBuilderHandler(skillbuilder.New(modelService, registryService)),
	})

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
	return nil
}

func runGooseMigrations(ctx context.Context, dbURL string, proxyURL ...string) error {
	proxy := ""
	if len(proxyURL) > 0 {
		proxy = proxyURL[0]
	}
	db, err := repo.OpenDatabase(dbURL, proxy)
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
