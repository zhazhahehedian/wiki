package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/zenith-wang/it-wiki/backend/internal/config"
	"github.com/zenith-wang/it-wiki/backend/internal/repo/migrations"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: migrate <up|down|status|reset>")
	}
	cmd := os.Args[1]

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	goose.SetBaseFS(migrations.EmbedMigrations)
	if err := goose.SetDialect("postgres"); err != nil {
		log.Fatalf("set dialect: %v", err)
	}

	ctx := context.Background()
	switch cmd {
	case "up":
		if err := goose.UpContext(ctx, db, "."); err != nil {
			log.Fatalf("goose up: %v", err)
		}
	case "down":
		if err := goose.DownContext(ctx, db, "."); err != nil {
			log.Fatalf("goose down: %v", err)
		}
	case "status":
		if err := goose.StatusContext(ctx, db, "."); err != nil {
			log.Fatalf("goose status: %v", err)
		}
	case "reset":
		if err := goose.ResetContext(ctx, db, "."); err != nil {
			log.Fatalf("goose reset: %v", err)
		}
	default:
		log.Fatalf("unknown command: %s", cmd)
	}

	fmt.Println("done.")
}
