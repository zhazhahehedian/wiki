package playground

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
)

type ModelsRequest struct {
	Protocol string `json:"protocol"`
	BaseURL  string `json:"baseUrl"`
	APIKey   string `json:"apiKey"`
}

// DiscoverModels uses the form draft without persisting it. A blank key can
// reuse only this user's saved key at the same normalized destination.
func (s *Service) DiscoverModels(ctx context.Context, user string, input ModelsRequest) ([]string, error) {
	input.Protocol = protocolName(input.Protocol)
	base, err := NormalizeBaseURL(input.BaseURL, input.Protocol)
	if err != nil {
		return nil, ErrInvalid
	}
	key := strings.TrimSpace(input.APIKey)
	if key == "" {
		connection, err := s.store.Get(ctx, user)
		if err != nil || connection.BaseURL != base || protocolName(connection.Protocol) != input.Protocol {
			return nil, ErrInvalid
		}
		key, err = s.key(connection)
		if err != nil {
			return nil, err
		}
	}
	if key == "" || len(key) > 8192 || strings.IndexFunc(key, unicode.IsSpace) >= 0 || strings.IndexFunc(key, unicode.IsControl) >= 0 {
		return nil, ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	models := []string{}
	seen := map[string]bool{}
	cursor := ""
	for pageNumber := 0; pageNumber < 5; pageNumber++ {
		path := "/models"
		if input.Protocol == ProtocolAnthropic {
			path = "/v1/models?limit=1000"
			if cursor != "" {
				path += "&after_id=" + url.QueryEscape(cursor)
			}
		}
		page, err := s.modelPage(ctx, base+path, input.Protocol, key)
		if err != nil {
			return nil, err
		}
		for _, model := range page.Data {
			id := strings.TrimSpace(model.ID)
			if id == "" || len(id) > 200 || strings.IndexFunc(id, unicode.IsControl) >= 0 {
				continue
			}
			if !seen[id] {
				models = append(models, id)
				seen[id] = true
			}
		}
		if len(models) > 5000 {
			return nil, ErrUpstream
		}
		if input.Protocol != ProtocolAnthropic || !page.HasMore {
			sort.Strings(models)
			return models, nil
		}
		if page.LastID == "" || page.LastID == cursor {
			return nil, ErrUpstream
		}
		cursor = page.LastID
	}
	return nil, ErrUpstream
}

type modelPage struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
	HasMore bool   `json:"has_more"`
	LastID  string `json:"last_id"`
}

func (s *Service) modelPage(ctx context.Context, endpoint, protocol, key string) (modelPage, error) {
	var result modelPage
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return result, ErrInvalid
	}
	setModelAuthentication(req, protocol, key)
	req.Header.Set("Accept", "application/json")
	response, err := s.client.Do(req)
	if err != nil {
		return result, ErrUpstream
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return result, ErrUpstream
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1024*1024+1))
	if err != nil || len(body) > 1024*1024 {
		return result, ErrUpstream
	}
	if json.Unmarshal(body, &result) != nil || result.Data == nil || len(result.Data) > 5000 {
		return result, ErrUpstream
	}
	return result, nil
}
