package feishu

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zenith-wang/it-wiki/backend/internal/directory"
)

func TestDirectoryUsesUserTokenStableIDsAndPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer user-token" || r.URL.Query().Get("user_id_type") != "open_id" || r.URL.Query().Get("department_id_type") != "open_department_id" {
			t.Error("wrong principal or identifier type")
		}
		switch r.URL.Path {
		case "/open-apis/contact/v3/departments/0/children":
			if r.URL.Query().Get("fetch_child") != "false" {
				t.Error("unexpected recursive visibility query")
			}
			if r.URL.Query().Get("page_token") == "next" {
				_, _ = w.Write([]byte(`{"code":0,"data":{"items":null,"has_more":false}}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"name":"Engineering","open_department_id":"od-engineering"}],"has_more":true,"page_token":"next"}}`))
		case "/open-apis/contact/v3/users/find_by_department":
			if r.URL.Query().Get("department_id") == "od-empty" {
				_, _ = w.Write([]byte(`{"code":0,"data":{"items":null,"has_more":false}}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"open_id":"ou-visible","name":"Visible","department_ids":["od-secret"],"email":"private@example.test"}]}}`))
		case "/open-apis/contact/v3/departments/parent":
			switch r.URL.Query().Get("department_id") {
			case "od-child":
				if r.URL.Query().Get("page_token") == "" {
					_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"name":"Team","open_department_id":"od-team"}],"has_more":true,"page_token":"more"}}`))
					return
				}
				_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"name":"Division","open_department_id":"od-division"}],"has_more":false}}`))
			case "od-top":
				_, _ = w.Write([]byte(`{"code":0,"data":{"has_more":false}}`))
			default:
				w.WriteHeader(403)
				_, _ = w.Write([]byte(`{"code":40014,"msg":"SECRET no parent dept authority"}`))
			}
		case "/open-apis/contact/v3/users/ou-denied":
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"code":41050,"msg":"SECRET provider error"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := NewClient(ClientConfig{BaseURL: server.URL, MaxRetries: -1}, server.Client())
	ctx := context.Background()
	page, e := c.Departments(ctx, "user-token", "0", "")
	if e != nil || !page.HasMore || page.Items[0].ID != "od-engineering" {
		t.Fatal(page, e)
	}
	page, e = c.Departments(ctx, "user-token", "0", page.PageToken)
	if e != nil || page.HasMore || page.Items == nil {
		t.Fatal(page, e)
	}
	users, e := c.Users(ctx, "user-token", "0", "")
	if e != nil || users.Items[0].DepartmentIDs != nil {
		t.Fatal("picker leaked membership", e)
	}
	users, e = c.Users(ctx, "user-token", "od-empty", "")
	if e != nil || users.Items == nil || len(users.Items) != 0 {
		t.Fatal("empty picker must return an array", e)
	}
	if _, e = c.User(ctx, "user-token", "ou-denied"); !errors.Is(e, directory.ErrAuthorization) || e.Error() == "SECRET provider error" {
		t.Fatal("unsafe/unclassified provider error", e)
	}
	parents, e := c.ParentDepartments(ctx, "user-token", "od-child", "")
	if e != nil || !parents.HasMore || parents.Items[0].ID != "od-team" {
		t.Fatal("parent page", parents, e)
	}
	parents, e = c.ParentDepartments(ctx, "user-token", "od-child", parents.PageToken)
	if e != nil || parents.HasMore || parents.Items[0].ID != "od-division" {
		t.Fatal("parent next page", parents, e)
	}
	if parents, e = c.ParentDepartments(ctx, "user-token", "od-top", ""); e != nil || parents.Items == nil || len(parents.Items) != 0 {
		t.Fatal("top-level department must have no parents", parents, e)
	}
	if _, e = c.ParentDepartments(ctx, "user-token", "od-hidden", ""); !errors.Is(e, directory.ErrAuthorization) {
		t.Fatal("hidden parent not classified as denied", e)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, e = c.Departments(canceled, "user-token", "0", ""); !errors.Is(e, context.Canceled) {
		t.Fatal("cancellation lost", e)
	}
}
