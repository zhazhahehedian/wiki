package knowledge

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

type Service struct {
	Optimizer QueryOptimizer
	Repo      Repository
	storage   ports.ObjectStorage
	embed     ports.Embedder
	minScore  float32
	mu        sync.Mutex
	limits    map[string]*limit
}
type limit struct {
	window           time.Time
	requests, active int
}

func New(r Repository, s ports.ObjectStorage, e ports.Embedder, min float32) *Service {
	return &Service{Repo: r, storage: s, embed: e, minScore: min, limits: map[string]*limit{}}
}
func (s *Service) SaveKB(ctx context.Context, p Principal, id string, in KB) (KB, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len([]rune(in.Name)) > 120 || len(in.Description) > 4000 || len(in.Members) > 100 {
		return KB{}, ErrInvalid
	}
	switch in.Visibility {
	case "private", "company", "department", "members":
	default:
		return KB{}, ErrInvalid
	}
	if in.Visibility == "department" && strings.TrimSpace(in.Department) == "" {
		return KB{}, ErrInvalid
	}
	for _, m := range in.Members {
		if m == "" || len(m) > 128 {
			return KB{}, ErrInvalid
		}
	}
	if s.embed == nil {
		return KB{}, ErrUnavailable
	}
	return s.Repo.SaveKB(ctx, p, id, in, s.embed.Model(), s.embed.Dim())
}
func (s *Service) Upload(ctx context.Context, p Principal, kbID, docID, name string, data []byte) (Document, error) {
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if !Supported(name) || len(name) > 240 || len(data) == 0 || len(data) > MaxUpload {
		return Document{}, ErrInvalid
	}
	if s.embed == nil {
		return Document{}, ErrUnavailable
	}
	// Authorize management before any object writes (readiness is irrelevant).
	bases, e := s.Repo.ListKB(ctx, p)
	if e != nil {
		return Document{}, e
	}
	ok := false
	for _, b := range bases {
		if b.ID == kbID && b.Manage {
			ok = true
		}
	}
	if !ok {
		return Document{}, ErrForbidden
	}
	sum := sha256.Sum256(data)
	v := Version{ID: uuid.NewString(), Title: name, Model: s.embed.Model(), Dim: s.embed.Dim(), Checksum: hex.EncodeToString(sum[:])}
	v.ObjectKey = "knowledge/" + kbID + "/" + v.ID
	if e = s.storage.Put(ctx, v.ObjectKey, bytes.NewReader(data), int64(len(data)), "application/octet-stream"); e != nil {
		return Document{}, ErrUnavailable
	}
	d, e := s.Repo.Stage(ctx, p, kbID, docID, v, int64(len(data)))
	if e != nil {
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.storage.Delete(clean, v.ObjectKey)
	}
	return d, e
}
func (s *Service) Process(ctx context.Context, id string) error {
	v, e := s.Repo.LoadVersion(ctx, id)
	if errors.Is(e, ErrNotFound) {
		return nil
	}
	if e != nil {
		return e
	}
	if v.Status == "ready" {
		return nil
	}
	fail := func(code string, transient bool) error {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if e := s.Repo.Fail(c, id, code); e != nil {
			return ErrUnavailable
		}
		if transient {
			return ErrUnavailable
		}
		return nil
	}
	if s.embed == nil || v.Model != s.embed.Model() || v.Dim != s.embed.Dim() {
		return fail("embedding_configuration_changed", false)
	}
	r, e := s.storage.Get(ctx, v.ObjectKey)
	if e != nil {
		return fail("storage_unavailable", true)
	}
	data, e := io.ReadAll(io.LimitReader(r, MaxUpload+1))
	r.Close()
	if e != nil {
		return fail("storage_read_failed", true)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != v.Checksum {
		return fail("checksum_mismatch", false)
	}
	preview := Parse(ctx, v.Title, data)
	if e = s.Repo.RecordPreview(ctx, id, preview); e != nil {
		return fail("preview_write_failed", true)
	}
	if !preview.Valid() {
		return fail("parse_quality_blocked", false)
	}
	blocks, e := Split(preview)
	if e != nil {
		diagnostic(&preview, "structure_too_large", "表格或结构超过安全切分上限，无法保证表头、单位和上下文完整；请拆分源文档", 0, true)
		if e = s.Repo.RecordPreview(ctx, id, preview); e != nil {
			return fail("preview_write_failed", true)
		}
		return fail("structure_too_large", false)
	}
	vectors := make([][]float32, 0, len(blocks))
	for i := 0; i < len(blocks); i += 16 {
		end := i + 16
		if end > len(blocks) {
			end = len(blocks)
		}
		texts := []string{}
		for _, b := range blocks[i:end] {
			texts = append(texts, b.Locator.Heading+"\n"+b.Content)
		}
		batch, e := s.embed.Embed(ctx, texts)
		if e != nil || len(batch) != len(texts) {
			return fail("embedding_unavailable", true)
		}
		for _, vec := range batch {
			if !validVector(vec, v.Dim) {
				return fail("embedding_invalid", false)
			}
		}
		vectors = append(vectors, batch...)
	}
	e = s.Repo.Publish(ctx, v, blocks, vectors)
	if errors.Is(e, ErrConflict) || errors.Is(e, ErrNotFound) {
		return nil
	}
	if e != nil {
		return fail("index_publish_failed", true)
	}
	return nil
}
func validVector(vec []float32, dim int) bool {
	if len(vec) != dim {
		return false
	}
	sum := float64(0)
	for _, x := range vec {
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
			return false
		}
		sum += float64(x) * float64(x)
	}
	return sum > 0
}
func ValidateSearch(in *SearchRequest) error {
	in.Query = strings.TrimSpace(in.Query)
	if !utf8.ValidString(in.Query) || in.Query == "" || utf8.RuneCountInString(in.Query) > 4000 || len(in.KBIDs) == 0 || len(in.KBIDs) > 10 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	ids := []string{}
	for _, id := range in.KBIDs {
		v, e := uuid.Parse(id)
		if e != nil {
			return ErrInvalid
		}
		id = v.String()
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	in.KBIDs = ids
	if in.TopK == 0 {
		in.TopK = 5
	}
	if in.TopK < 1 || in.TopK > 20 {
		return ErrInvalid
	}
	return nil
}
func (s *Service) acquire(p Principal) (func(), error) {
	key := p.ApplicationID
	if key == "" {
		key = "user:" + p.UserID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, l := range s.limits {
		if l.active == 0 && now.Sub(l.window) > 2*time.Minute {
			delete(s.limits, id)
		}
	}
	l := s.limits[key]
	if l == nil {
		l = &limit{window: now}
		s.limits[key] = l
	}
	if now.Sub(l.window) >= time.Minute {
		l.requests = 0
		l.window = now
	}
	if l.active >= 4 || l.requests >= 60 {
		return nil, ErrLimited
	}
	l.active++
	l.requests++
	return func() { s.mu.Lock(); l.active--; s.mu.Unlock() }, nil
}
func (s *Service) Search(ctx context.Context, p Principal, in SearchRequest) ([]Match, error) {
	return s.SearchDetailed(ctx, p, in, &RetrievalInfo{})
}
func (s *Service) SearchDetailed(ctx context.Context, p Principal, in SearchRequest, info *RetrievalInfo) ([]Match, error) {
	*info = RetrievalInfo{Strategy: "none", Rewrite: "skipped", Rerank: "skipped", Warnings: []string{}}
	if e := ValidateSearch(&in); e != nil {
		return nil, e
	}
	release, e := s.acquire(p)
	if e != nil {
		return nil, e
	}
	defer release()
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	bases, e := s.Repo.Authorize(ctx, p, in.KBIDs)
	if e != nil {
		return nil, e
	}
	ready := false
	for _, b := range bases {
		ready = ready || b.HasActive
	}
	if !ready {
		return s.Repo.Recheck(ctx, p, in.KBIDs, []Match{})
	}
	if s.embed == nil {
		return nil, ErrUnavailable
	}
	for _, b := range bases {
		if b.HasActive && (b.Model != s.embed.Model() || b.Dim != s.embed.Dim()) {
			return nil, ErrNotReady
		}
	}
	return s.retrieve(ctx, p, in, info)
}
func (s *Service) Authenticate(ctx context.Context, token string) (Principal, error) {
	if !strings.HasPrefix(token, "kh_") || len(token) != 46 {
		return Principal{}, ErrUnauthenticated
	}
	sum := sha256.Sum256([]byte(token))
	return s.Repo.Authenticate(ctx, sum[:])
}
func (s *Service) Rotate(ctx context.Context, p Principal, id string) (Credential, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return Credential{}, e
	}
	token := "kh_" + base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(token))
	out := Credential{Token: token, Prefix: token[:11], ExpiresAt: time.Now().Add(90 * 24 * time.Hour)}
	if e := s.Repo.Rotate(ctx, p, id, sum[:], out.Prefix, out.ExpiresAt); e != nil {
		return Credential{}, e
	}
	return out, nil
}
func (s *Service) SaveApplication(ctx context.Context, p Principal, id string, in ApplicationInput) (Application, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 120 || len(in.KBIDs) > 100 {
		return Application{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range in.KBIDs {
		if _, e := uuid.Parse(id); e != nil || seen[id] {
			return Application{}, ErrInvalid
		}
		seen[id] = true
	}
	return s.Repo.SaveApplication(ctx, p, id, in)
}
