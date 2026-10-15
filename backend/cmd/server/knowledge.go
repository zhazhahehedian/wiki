package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
	httpx "github.com/zenith-wang/it-wiki/backend/internal/http"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/embedder"
	"github.com/zenith-wang/it-wiki/backend/internal/knowledge"
	"github.com/zenith-wang/it-wiki/backend/internal/repo"
)

func buildKnowledge(pool *pgxpool.Pool, storage ports.ObjectStorage) (*httpx.KnowledgeHandler, *river.Client[pgx.Tx], error) {
	var embedding ports.Embedder
	model, base, key := os.Getenv("EMBEDDING_MODEL"), os.Getenv("EMBEDDING_BASE_URL"), os.Getenv("EMBEDDING_API_KEY")
	if model != "" || base != "" || key != "" {
		dim, e := strconv.Atoi(os.Getenv("EMBEDDING_DIM"))
		if e != nil || dim < 1 || dim > 16000 || model == "" || base == "" || key == "" {
			return nil, nil, fmt.Errorf("knowledge requires complete platform EMBEDDING configuration")
		}
		embedding = embedder.New(embedder.Config{BaseURL: strings.TrimRight(base, "/"), APIKey: key, Model: model, Dim: dim})
		var actual string
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if e := pool.QueryRow(ctx, `SELECT format_type(atttypid,atttypmod) FROM pg_attribute WHERE attrelid='knowledge_chunks'::regclass AND attname='embedding'`).Scan(&actual); e != nil {
			return nil, nil, fmt.Errorf("knowledge vector schema unavailable")
		}
		if actual != fmt.Sprintf("vector(%d)", dim) {
			return nil, nil, fmt.Errorf("knowledge embedding dimension differs from schema; explicit rebuild migration required")
		}
	}
	min := float32(0.5)
	if raw := os.Getenv("KNOWLEDGE_MIN_SCORE"); raw != "" {
		v, e := strconv.ParseFloat(raw, 32)
		if e != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return nil, nil, fmt.Errorf("invalid KNOWLEDGE_MIN_SCORE")
		}
		min = float32(v)
	}
	r := repo.NewKnowledgeRepository(pool)
	s := knowledge.New(r, storage, embedding, min)
	optBase, optKey, optModel := os.Getenv("KNOWLEDGE_OPTIMIZER_BASE_URL"), os.Getenv("KNOWLEDGE_OPTIMIZER_API_KEY"), os.Getenv("KNOWLEDGE_OPTIMIZER_MODEL")
	if optBase != "" || optKey != "" || optModel != "" {
		optimizer, err := knowledge.NewModelOptimizer(optBase, optKey, optModel)
		if err != nil {
			return nil, nil, fmt.Errorf("knowledge requires complete valid KNOWLEDGE_OPTIMIZER configuration")
		}
		s.Optimizer = optimizer
	}
	workers := river.NewWorkers()
	river.AddWorker(workers, &knowledge.Worker{Service: s})
	rc, e := river.NewClient(riverpgxv5.New(pool), &river.Config{Workers: workers, Queues: map[string]river.QueueConfig{"knowledge": {MaxWorkers: 4}}})
	if e != nil {
		return nil, nil, e
	}
	r.Enqueue = func(ctx context.Context, tx pgx.Tx, id string) error {
		_, e := rc.InsertTx(ctx, tx, knowledge.Job{VersionID: id}, &river.InsertOpts{Queue: "knowledge", MaxAttempts: 3})
		return e
	}
	return httpx.NewKnowledgeHandler(s), rc, nil
}
