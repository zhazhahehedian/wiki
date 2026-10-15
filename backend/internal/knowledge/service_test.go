package knowledge

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type searchRepo struct {
	Repository
	deny    error
	revoke  error
	calls   int
	matches []Match
}

func (r *searchRepo) Authorize(context.Context, Principal, []string) ([]KB, error) {
	r.calls++
	return []KB{{Model: "test", Dim: 2, HasActive: true}}, r.deny
}
func (r *searchRepo) Search(context.Context, []string, []float32, int, float32) ([]Match, error) {
	return r.matches, nil
}
func (r *searchRepo) LexicalSearch(context.Context, []string, []string, int) ([]Match, error) {
	return nil, nil
}
func (r *searchRepo) Recheck(context.Context, Principal, []string, []Match) ([]Match, error) {
	return r.matches, r.revoke
}

type testEmbed struct {
	calls     atomic.Int32
	wait      bool
	malformed bool
	texts     []string // embedded inputs, for sequential tests
}

func (*testEmbed) Model() string { return "test" }
func (*testEmbed) Dim() int      { return 2 }
func (e *testEmbed) Embed(ctx context.Context, s []string) ([][]float32, error) {
	e.calls.Add(1)
	e.texts = append(e.texts, s...)
	if e.wait {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if e.malformed {
		return [][]float32{{0, 0}}, nil
	}
	return [][]float32{{1, 0}}, nil
}
func TestSearchAuthorizationAndRecheck(t *testing.T) {
	req := SearchRequest{KBIDs: []string{"00000000-0000-0000-0000-000000000001"}, Query: "业务问题"}
	r := &searchRepo{deny: ErrForbidden}
	e := &testEmbed{}
	s := New(r, nil, e, 0.5)
	if _, _, err := s.Search(context.Background(), Principal{ApplicationID: "a"}, req); !errors.Is(err, ErrForbidden) || e.calls.Load() != 0 {
		t.Fatal("unauthorized question sent to embedding")
	}
	r.deny = nil
	r.revoke = ErrUnauthenticated
	if _, _, err := s.Search(context.Background(), Principal{ApplicationID: "a"}, req); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("revocation ignored")
	}
	r.revoke = nil
	e.malformed = true
	if _, _, err := s.Search(context.Background(), Principal{ApplicationID: "a"}, req); !errors.Is(err, ErrUnavailable) {
		t.Fatal("invalid provider vector accepted")
	}
}
func TestSearchCancellation(t *testing.T) {
	r := &searchRepo{}
	e := &testEmbed{wait: true}
	s := New(r, nil, e, 0.5)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, _, err := s.Search(ctx, Principal{UserID: "u"}, SearchRequest{KBIDs: []string{"00000000-0000-0000-0000-000000000001"}, Query: "question"})
	if !errors.Is(err, context.Canceled) || time.Since(start) > time.Second {
		t.Fatal("cancellation not propagated", err)
	}
}
func TestApplicationLimitsIndependentAndReleased(t *testing.T) {
	s := New(nil, nil, nil, 0.5)
	a := Principal{ApplicationID: "a"}
	releases := []func(){}
	for i := 0; i < 4; i++ {
		f, e := s.acquire(a)
		if e != nil {
			t.Fatal(e)
		}
		releases = append(releases, f)
	}
	if _, e := s.acquire(a); !errors.Is(e, ErrLimited) {
		t.Fatal("concurrency not limited")
	}
	f, e := s.acquire(Principal{ApplicationID: "b"})
	if e != nil {
		t.Fatal("applications share quota")
	}
	f()
	for _, f := range releases {
		f()
	}
	for i := 0; i < 56; i++ {
		f, e = s.acquire(a)
		if e != nil {
			t.Fatal(i, e)
		}
		f()
	}
	if _, e = s.acquire(a); !errors.Is(e, ErrLimited) {
		t.Fatal("minute quota not limited")
	}
}

type credentialRepo struct {
	Repository
	digest []byte
	prefix string
}

func (r *credentialRepo) Rotate(_ context.Context, _ Principal, _ string, d []byte, p string, _ time.Time) error {
	r.digest = append([]byte{}, d...)
	r.prefix = p
	return nil
}
func TestCredentialsAreOneTimeRandomDigests(t *testing.T) {
	r := &credentialRepo{}
	s := New(r, nil, nil, 0.5)
	first, e := s.Rotate(context.Background(), Principal{}, "app")
	if e != nil {
		t.Fatal(e)
	}
	second, e := s.Rotate(context.Background(), Principal{}, "app")
	if e != nil || first.Token == second.Token || len(second.Token) != 46 || len(r.digest) != 32 || strings.Contains(string(r.digest), second.Token) || len(r.prefix) != 11 {
		t.Fatal("credential generation invalid")
	}
}
