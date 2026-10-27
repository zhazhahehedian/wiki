package feishu

import (
	"context"
	"errors"
	"net/url"

	"github.com/zenith-wang/it-wiki/backend/internal/directory"
	"github.com/zenith-wang/it-wiki/backend/internal/domain/ports"
)

func directoryQuery(page string) url.Values {
	q := url.Values{"user_id_type": {"open_id"}, "department_id_type": {"open_department_id"}, "page_size": {"50"}}
	if page != "" {
		q.Set("page_token", page)
	}
	return q
}
func directoryError(e error) error {
	if e == nil {
		return nil
	}
	if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
		return e
	}
	// Provider bodies and tokens are deliberately never included in public errors.
	var source *ports.SourceLoadError
	if errors.As(e, &source) {
		switch source.Code {
		case ports.SourceLoadForbidden:
			return directory.ErrAuthorization
		case ports.SourceLoadNotFound:
			return directory.ErrInvalid
		}
	}
	return directory.ErrUnavailable
}
func (c *Client) Departments(ctx context.Context, token, parent, page string) (directory.Page[directory.Department], error) {
	out := directory.Page[directory.Department]{Items: []directory.Department{}}
	q := directoryQuery(page)
	q.Set("fetch_child", "false")
	e := c.Get(ctx, token, "/open-apis/contact/v3/departments/"+url.PathEscape(parent)+"/children", q, &out)
	if out.Items == nil {
		out.Items = []directory.Department{}
	}
	if e == nil && out.HasMore && (out.PageToken == "" || out.PageToken == page) {
		return out, directory.ErrUnavailable
	}
	return out, directoryError(e)
}
func (c *Client) Users(ctx context.Context, token, department, page string) (directory.Page[directory.User], error) {
	out := directory.Page[directory.User]{Items: []directory.User{}}
	q := directoryQuery(page)
	q.Set("department_id", department)
	e := c.Get(ctx, token, "/open-apis/contact/v3/users/find_by_department", q, &out)
	if out.Items == nil {
		out.Items = []directory.User{}
	}
	if e == nil && out.HasMore && (out.PageToken == "" || out.PageToken == page) {
		return out, directory.ErrUnavailable
	}
	// A picker needs only names and IDs, not membership or other personal fields.
	for i := range out.Items {
		out.Items[i].DepartmentIDs = nil
	}
	return out, directoryError(e)
}
func (c *Client) Department(ctx context.Context, token, id string) (directory.Department, error) {
	var out struct {
		Department directory.Department `json:"department"`
	}
	e := c.Get(ctx, token, "/open-apis/contact/v3/departments/"+url.PathEscape(id), directoryQuery(""), &out)
	return out.Department, directoryError(e)
}
func (c *Client) User(ctx context.Context, token, id string) (directory.User, error) {
	var out struct {
		User directory.User `json:"user"`
	}
	e := c.Get(ctx, token, "/open-apis/contact/v3/users/"+url.PathEscape(id), directoryQuery(""), &out)
	return out.User, directoryError(e)
}
