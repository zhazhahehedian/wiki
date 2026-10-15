package http

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/zenith-wang/it-wiki/backend/internal/domain"
	"github.com/zenith-wang/it-wiki/backend/internal/registry"
	"github.com/zenith-wang/it-wiki/backend/internal/skillbuilder"
)

type templateRepo struct {
	registry.Repository
	detail registry.Detail
	err    error
	user   string
	reads  int
}

func (r *templateRepo) Read(_ context.Context, user, slug string, published bool) (registry.Detail, error) {
	if !published || slug != r.detail.Slug {
		panic("template read bypassed published projection")
	}
	r.user = user
	r.reads++
	return r.detail, r.err
}

type templateStorage struct {
	data  []byte
	reads int
}

func (s *templateStorage) Get(_ context.Context, key string) (io.ReadCloser, error) {
	if key != "published-bundle" {
		panic("draft bundle leaked")
	}
	s.reads++
	return io.NopCloser(bytes.NewReader(s.data)), nil
}
func (*templateStorage) Put(context.Context, string, io.Reader, int64, string) error {
	panic("template must not save")
}
func (*templateStorage) Delete(context.Context, string) error {
	panic("template must not mutate source")
}

func TestSkillTemplatePublishedAuthorizationAndVersionBoundary(t *testing.T) {
	for _, scenario := range []string{"reader", "owner", "admin", "anonymous", "hidden", "changed version", "missing version", "unsupported"} {
		t.Run(scenario, func(t *testing.T) {
			user := uuid.NewString()
			auth := newTestAuthHandler(t, &fakeAuthFlow{}, &fakeSessionStore{session: domain.Session{UserID: user}}, fakeUserResolver{user: domain.User{ID: user}})
			repository := &templateRepo{detail: registry.Detail{Capability: registry.Capability{Type: "skill", Slug: "source", Name: "Published name", IsLive: true, CurrentVersionID: "published", DraftVersionID: "draft"}, Versions: []registry.Version{{ID: "draft", BundleKey: "secret-draft", HasBundle: true}, {ID: "published", Version: "1.0.0", BundleKey: "published-bundle", HasBundle: true}}}}
			files := []registry.File{{Name: "SKILL.md", Data: []byte("---\nname: source\ndescription: example\n---\n# Steps\n[Reference](references/data.md)")}, {Name: "references/data.md", Data: []byte("full reference")}}
			if scenario == "unsupported" {
				files = append(files, registry.File{Name: "scripts/run.py", Data: []byte("print('hello')")})
			}
			data, err := registry.PackSkill("source", files)
			if err != nil {
				t.Fatal(err)
			}
			storage := &templateStorage{data: data}
			if scenario == "hidden" {
				repository.err = registry.ErrNotFound
			}
			if scenario == "owner" {
				repository.detail.IsOwner = true
			}
			if scenario == "admin" {
				repository.detail.IsAdmin = true
			}
			version := "published"
			if scenario == "changed version" {
				version = "old"
			}
			if scenario == "missing version" {
				version = ""
			}
			router := NewCapabilityHubRouter(Handlers{Auth: auth, Registry: NewRegistryHandler(registry.New(repository, storage))})
			r := httptest.NewRequest("GET", "/api/v1/catalog/source/template?version_id="+version, nil)
			if scenario != "anonymous" {
				r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "session"})
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			want := map[string]int{"anonymous": 401, "hidden": 404, "changed version": 409, "missing version": 400, "unsupported": 422}[scenario]
			if want == 0 {
				want = 200
			}
			if w.Code != want {
				t.Fatalf("got %d want %d: %s", w.Code, want, w.Body.String())
			}
			if want == 200 {
				var result struct {
					Draft  skillbuilder.Draft
					Source struct{ Slug, Version string }
				}
				if json.Unmarshal(w.Body.Bytes(), &result) != nil || result.Draft.Slug == "source" || len(result.Draft.Files) != 1 || result.Draft.Files[0].Content != "full reference" || result.Source.Version != "1.0.0" || repository.user != user || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("copy or identity lost", w.Body.String())
				}
			} else if want != 422 && storage.reads != 0 {
				t.Fatal("unauthorized or stale source reached storage")
			}
		})
	}
}
