package playground

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestDiscoverModelsUsesDraftOrOwnedKeyWithoutSaving(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer secret-test-key" {
			t.Error("wrong discovery request")
		}
		fmt.Fprint(w, `{"data":[{"id":"beta"},{"id":"alpha"},{"id":"beta"},{"id":""}]}`)
	}))
	defer upstream.Close()
	client, _ := NewHTTPClient(upstream.URL)
	s, store := newTestService(t, client)
	input := ModelsRequest{BaseURL: upstream.URL + "/v1", APIKey: "secret-test-key"}
	models, err := s.DiscoverModels(context.Background(), "alice", input)
	if err != nil || !reflect.DeepEqual(models, []string{"alpha", "beta"}) || len(store.rows) != 0 {
		t.Fatalf("draft discovery: %v %v", models, err)
	}
	saveTestConnection(t, s, input.BaseURL)
	input.APIKey = ""
	if _, err := s.DiscoverModels(context.Background(), "alice", input); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DiscoverModels(context.Background(), "bob", input); !errors.Is(err, ErrInvalid) {
		t.Fatal("another user's key was reused")
	}
	input.BaseURL = upstream.URL + "/other"
	if _, err := s.DiscoverModels(context.Background(), "alice", input); !errors.Is(err, ErrInvalid) {
		t.Fatal("saved key sent to changed destination")
	}
}

func TestDiscoverModelsFailuresAndEmptyList(t *testing.T) {
	for _, body := range []string{`{"data":[]}`, `{"error":"secret-test-key"}`, `not json`, strings.Repeat("x", 1024*1024+1)} {
		t.Run(fmt.Sprintf("body length %d", len(body)), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer server.Close()
			client, _ := NewHTTPClient(server.URL)
			s, _ := newTestService(t, client)
			models, err := s.DiscoverModels(context.Background(), "alice", ModelsRequest{BaseURL: server.URL, APIKey: "secret-test-key"})
			if body == `{"data":[]}` {
				if err != nil || models == nil || len(models) != 0 {
					t.Fatal("empty list not represented correctly")
				}
			} else if !errors.Is(err, ErrUpstream) {
				t.Fatalf("provider error not sanitized: %v", err)
			}
		})
	}
}
