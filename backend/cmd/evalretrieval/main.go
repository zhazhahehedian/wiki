package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/zenith-wang/it-wiki/backend/internal/eval"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/embedder"
	"github.com/zenith-wang/it-wiki/backend/internal/infra/vectorstore"
	"github.com/zenith-wang/it-wiki/backend/internal/repo"
	"github.com/zenith-wang/it-wiki/backend/internal/service"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "[fatal] %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	kbID := flag.String("kb", "", "knowledge base id (required)")
	file := flag.String("file", "../docs/eval/retrieval-eval.yaml", "eval set yaml path")
	outDir := flag.String("out", "../docs/eval/results", "directory for result JSON")
	label := flag.String("label", "run", "label recorded in result filename/JSON, e.g. baseline")
	flag.Parse()
	if *kbID == "" {
		return fmt.Errorf("-kb is required")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	dim, err := strconv.Atoi(getEnv("EMBEDDING_DIM", "1024"))
	if err != nil {
		return fmt.Errorf("invalid EMBEDDING_DIM: %w", err)
	}
	topK, _ := strconv.Atoi(getEnv("RAG_TOP_K", "8"))
	if topK < 5 {
		topK = 5 // Hit@5 至少需要 5 个命中
	}

	cases, err := eval.LoadCases(*file)
	if err != nil {
		return err
	}

	ctx := context.Background()
	pool, err := repo.NewPool(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("pgx pool: %w", err)
	}
	defer pool.Close()

	embed := embedder.New(embedder.Config{
		BaseURL: os.Getenv("EMBEDDING_BASE_URL"),
		APIKey:  os.Getenv("EMBEDDING_API_KEY"),
		Model:   os.Getenv("EMBEDDING_MODEL"),
		Dim:     dim,
	})
	// minScore 传 0: 评测要看原始排名, 不做证据过滤
	retrieval := service.NewRetrieval(embed, vectorstore.New(pool), topK, 0)

	results := make([]eval.CaseResult, 0, len(cases))
	for _, c := range cases {
		started := time.Now()
		res, err := retrieval.Retrieve(ctx, *kbID, c.Question)
		if err != nil {
			return fmt.Errorf("case %s: %w", c.ID, err)
		}
		hits := make([]eval.Hit, 0, len(res.Hits))
		var topScore float32
		for i, h := range res.Hits {
			if i == 0 {
				topScore = h.Score
			}
			hits = append(hits, eval.Hit{DocumentTitle: h.DocumentTitle, Content: h.Content})
		}
		r := eval.ScoreCase(c, hits, topScore)
		fmt.Printf("%-10s docRank=%d passageHit=%-5v topScore=%.3f %4dms %s\n",
			r.ID, r.DocRank, r.PassageHit, r.TopScore,
			time.Since(started).Milliseconds(), c.Question)
		results = append(results, r)
	}

	summary := eval.Summarize(results)
	fmt.Printf("\ncases=%d docHit@1=%.2f docHit@3=%.2f docHit@5=%.2f passageHit@5=%.2f MRR=%.3f\n",
		summary.Cases, summary.DocHitAt1, summary.DocHitAt3, summary.DocHitAt5,
		summary.PassageHitAt5, summary.MRR)

	out := map[string]any{
		"timestamp":       time.Now().Format(time.RFC3339),
		"label":           *label,
		"kb_id":           *kbID,
		"top_k":           topK,
		"embedding_model": os.Getenv("EMBEDDING_MODEL"),
		"chunk_size":      getEnv("CHUNK_SIZE", "(default)"),
		"chunk_overlap":   getEnv("CHUNK_OVERLAP", "(default)"),
		"summary":         summary,
		"cases":           results,
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		return err
	}
	name := filepath.Join(*outDir,
		fmt.Sprintf("%s-%s.json", time.Now().Format("20060102-150405"), *label))
	buf, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(name, buf, 0o644); err != nil {
		return err
	}
	fmt.Printf("result written to %s\n", name)
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
