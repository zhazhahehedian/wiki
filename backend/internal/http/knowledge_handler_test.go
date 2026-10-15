package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	k "github.com/zenith-wang/it-wiki/backend/internal/knowledge"
)

type auditedSearchRepo struct {
	k.Repository
	status string
	count  int
	ids    []string
}

func (*auditedSearchRepo) Authenticate(context.Context, []byte) (k.Principal, error) {
	return k.Principal{ApplicationID: "app", CredentialID: "credential"}, nil
}
func (*auditedSearchRepo) Authorize(context.Context, k.Principal, []string) ([]k.KB, error) {
	return []k.KB{{HasActive: true, Model: "fixture", Dim: 2}}, nil
}
func (*auditedSearchRepo) Search(context.Context, []string, []float32, int, float32) ([]k.Match, error) {
	return []k.Match{{ChunkID: "c", DocumentID: "d", VersionID: "v"}}, nil
}
func (*auditedSearchRepo) LexicalSearch(context.Context, []string, []string, int) ([]k.Match, error) {
	return nil, nil
}
func (*auditedSearchRepo) Recheck(_ context.Context, _ k.Principal, _ []string, m []k.Match) ([]k.Match, error) {
	return m, nil
}
func (r *auditedSearchRepo) Audit(_ context.Context, _ k.Principal, ids []string, _ string, status string, count int, _ time.Duration) error {
	r.ids, r.status, r.count = ids, status, count
	return nil
}

type fixtureEmbedder struct{}

func (fixtureEmbedder) Model() string { return "fixture" }
func (fixtureEmbedder) Dim() int      { return 2 }
func (fixtureEmbedder) Embed(_ context.Context, in []string) ([][]float32, error) {
	out := make([][]float32, len(in))
	for i := range out {
		out[i] = []float32{1, 0}
	}
	return out, nil
}

// The audit row must describe exactly the response: same outcome, same result
// count and the canonical, deduplicated knowledge base IDs.
func TestKnowledgeSearchAuditMatchesResponse(t *testing.T) {
	repo := &auditedSearchRepo{}
	h := NewKnowledgeHandler(k.New(repo, nil, fixtureEmbedder{}, 0.5))
	search := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/knowledge/search", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer kh_"+strings.Repeat("a", 43))
		rec := httptest.NewRecorder()
		h.MachineSearch(rec, req)
		return rec
	}
	id := "0000000A-0000-0000-0000-000000000001"
	rec := search(`{"knowledge_base_ids":["` + id + `","` + strings.ToLower(id) + `"],"query":"q"}`)
	var body struct {
		Matches []k.Match `json:"matches"`
	}
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &body) != nil {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if repo.status != "ok" || repo.count != len(body.Matches) || len(body.Matches) != 1 || !reflect.DeepEqual(repo.ids, []string{strings.ToLower(id)}) {
		t.Fatal("audit disagrees with response", repo.status, repo.count, repo.ids, len(body.Matches))
	}
	if rec = search(`{"knowledge_base_ids":["` + id + `"],"query":"q","top_k":99}`); rec.Code != 400 || repo.status != "invalid_request" || repo.count != 0 || len(repo.ids) != 0 {
		t.Fatal("invalid search audit", rec.Code, repo.status, repo.ids)
	}
}
