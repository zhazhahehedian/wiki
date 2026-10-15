// evalknowledge queries an already prepared isolated evaluation knowledge base.
// Credentials stay in the environment; this command never changes data.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	k "github.com/zenith-wang/it-wiki/backend/internal/knowledge"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	base, token, ids := os.Getenv("KNOWLEDGE_API_BASE"), os.Getenv("KNOWLEDGE_APPLICATION_TOKEN"), os.Getenv("KNOWLEDGE_EVAL_BASE_IDS")
	if base == "" || token == "" || ids == "" {
		return fmt.Errorf("set KNOWLEDGE_API_BASE, KNOWLEDGE_APPLICATION_TOKEN and KNOWLEDGE_EVAL_BASE_IDS")
	}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	stages := []k.RetrievalInfo{}
	suite := os.Getenv("KNOWLEDGE_EVAL_SUITE")
	if suite == "" {
		suite = "baseline"
	}
	report, e := k.EvaluateSuite(context.Background(), func(ctx context.Context, query string) ([]k.Match, error) {
		raw, _ := json.Marshal(k.SearchRequest{KBIDs: strings.Split(ids, ","), Query: query, TopK: 5})
		req, e := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(base, "/")+"/api/v1/knowledge/search", bytes.NewReader(raw))
		if e != nil {
			return nil, e
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		res, e := client.Do(req)
		if e != nil {
			return nil, fmt.Errorf("retrieval connection failed")
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return nil, fmt.Errorf("retrieval status %d", res.StatusCode)
		}
		var body struct {
			Matches   []k.Match       `json:"matches"`
			Retrieval k.RetrievalInfo `json:"retrieval"`
		}
		e = json.NewDecoder(io.LimitReader(res.Body, 128<<10)).Decode(&body)
		stages = append(stages, body.Retrieval)
		if e == nil && os.Getenv("KNOWLEDGE_EVAL_REQUIRE_OPTIMIZER") == "1" {
			if body.Retrieval.Strategy != "hybrid_rrf" || (body.Retrieval.Rewrite != "applied" && body.Retrieval.Rewrite != "unchanged") || len(body.Retrieval.Warnings) > 0 || (len(body.Matches) > 0 && body.Retrieval.Rerank != "model") {
				return nil, fmt.Errorf("optimizer acceptance requires enabled model stages without fallback")
			}
		}
		return body.Matches, e
	}, suite)
	if e != nil {
		return e
	}
	report.Mode = "external-api"
	report.Retrieval = stages
	report.OptimizerModel = os.Getenv("KNOWLEDGE_OPTIMIZER_MODEL")
	report.Model = os.Getenv("EMBEDDING_MODEL")
	report.MinScore = 0.5
	if raw := os.Getenv("KNOWLEDGE_MIN_SCORE"); raw != "" {
		v, err := strconv.ParseFloat(raw, 32)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1 {
			return fmt.Errorf("invalid KNOWLEDGE_MIN_SCORE for evaluation report")
		}
		report.MinScore = float32(v)
	}
	out, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(out))
	if !report.Passed {
		return fmt.Errorf("knowledge evaluation thresholds not met")
	}
	return nil
}
