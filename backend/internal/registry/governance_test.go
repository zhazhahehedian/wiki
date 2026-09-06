package registry

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPublishedVisibility(t *testing.T) {
	for _, tc := range []struct {
		name string
		who  Identity
		live bool
		meta PublishedMetadata
		want bool
	}{
		{"anonymous org", Identity{}, true, PublishedMetadata{Visibility: "org"}, false},
		{"org member", Identity{OpenID: "bob"}, true, PublishedMetadata{Visibility: "org"}, true},
		{"offline admin", Identity{OpenID: "admin", IsAdmin: true}, false, PublishedMetadata{Visibility: "org"}, false},
		{"missing departments", Identity{OpenID: "bob"}, true, PublishedMetadata{Visibility: "department"}, false},
		{"untrusted department absent", Identity{OpenID: "bob"}, true, PublishedMetadata{Visibility: "department", Department: "Engineering"}, false},
		{"matching department", Identity{OpenID: "bob", Department: "Engineering"}, true, PublishedMetadata{Visibility: "department", Department: "Engineering"}, true},
		{"different department", Identity{OpenID: "bob", Department: "Sales"}, true, PublishedMetadata{Visibility: "department", Department: "Engineering"}, false},
		{"listed", Identity{OpenID: "bob"}, true, PublishedMetadata{Visibility: "allowlist", Allowlist: []string{"bob"}}, true},
		{"unlisted", Identity{OpenID: "eve"}, true, PublishedMetadata{Visibility: "allowlist", Allowlist: []string{"bob"}}, false},
		{"owner", Identity{OpenID: "owner"}, true, PublishedMetadata{Visibility: "allowlist"}, true},
		{"admin", Identity{OpenID: "admin", IsAdmin: true}, true, PublishedMetadata{Visibility: "allowlist"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanViewPublished(tc.who, "owner", tc.live, tc.meta); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
func TestPublicationSkillFormatAndReferences(t *testing.T) {
	valid := "---\nname: example\ndescription: Prepare a report\n---\n# Instructions\nUse [template](references/template.md)."
	fixtures := []File{{Name: "SKILL.md", Data: []byte(valid)}, {Name: "references/template.md", Data: []byte("[Root](../SKILL.md)")}}
	if e := ValidateSkillPublication("example", fixtures); e != nil {
		t.Fatal(e)
	}
	for name, body := range map[string]string{
		"no frontmatter": "# Instructions", "wrong name": strings.Replace(valid, "name: example", "name: spoof", 1),
		"missing description": strings.Replace(valid, "description: Prepare a report", "description: ''", 1),
		"wrong type":          strings.Replace(valid, "description: Prepare a report", "description: [one, two]", 1),
		"duplicate field":     strings.Replace(valid, "name: example", "name: example\nname: other", 1),
		"missing file":        strings.Replace(valid, "references/template.md", "references/absent.md", 1),
		"escape":              strings.Replace(valid, "references/template.md", "../outside.md", 1),
		"encoded escape":      strings.Replace(valid, "references/template.md", "%2e%2e/outside.md", 1),
		"empty body":          "---\nname: example\ndescription: Test\n---\n",
	} {
		t.Run(name, func(t *testing.T) {
			files := append([]File{}, fixtures...)
			files[0].Data = []byte(body)
			if ValidateSkillPublication("example", files) == nil {
				t.Fatal("accepted invalid publication")
			}
		})
	}
}

type reviewRaceRepo struct {
	Repository
	reads int
}

func (r *reviewRaceRepo) Identity(context.Context, string) (Identity, error) {
	return Identity{OpenID: "owner"}, nil
}
func (r *reviewRaceRepo) Read(context.Context, string, string, bool) (Detail, error) {
	r.reads++
	return Detail{Capability: Capability{Slug: "example", Type: "skill", OwnerOpenID: "owner", Status: "draft", Revision: int64(r.reads), DraftVersionID: "11111111-1111-1111-1111-111111111111"}, Versions: []Version{{ID: "11111111-1111-1111-1111-111111111111", BundleKey: "old-key", HasBundle: true}}}, nil
}
func TestSubmitReportsConcurrentBundleReplacementAsConflict(t *testing.T) {
	repo := &reviewRaceRepo{}
	service := New(repo, &recordingStorage{}) // the replaced bundle is no longer present
	_, e := service.Act(context.Background(), "user", "example", "submit", ActionRequest{Revision: 1, VersionID: "11111111-1111-1111-1111-111111111111"})
	if !errors.Is(e, ErrConflict) || repo.reads != 2 {
		t.Fatalf("stale bundle review should conflict: %v (reads=%d)", e, repo.reads)
	}
}
