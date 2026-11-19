package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"xiaotao/xt-client/internal/modelsource"
	"xiaotao/xt-client/internal/wireprotocol"
)

const maxCatalogBytes = 2 << 20

type CatalogModel struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
}

type catalogPage struct {
	Data    []CatalogModel `json:"data"`
	HasMore bool           `json:"has_more"`
	LastID  string         `json:"last_id"`
}

func (s *Service) DiscoverModels(ctx context.Context, baseURL, protocol, key string) ([]CatalogModel, error) {
	endpoint, err := modelsource.ModelsURL(baseURL, protocol)
	key = strings.TrimSpace(key)
	if err != nil || key == "" || strings.ContainsAny(key, "\r\n") {
		return nil, modelsource.ErrInvalid
	}
	models := []CatalogModel{}
	seen := map[string]bool{}
	cursors := map[string]bool{}
	pageURL := endpoint
	remaining := maxCatalogBytes
	for range 100 {
		page, err := s.readCatalogPage(ctx, pageURL, protocol, key)
		if err != nil {
			return nil, err
		}
		for _, model := range page.Data {
			model.ID = strings.TrimSpace(model.ID)
			if model.ID != "" && !seen[model.ID] {
				remaining -= len(model.ID) + len(model.DisplayName)
				if remaining < 0 || len(models) >= 10_000 {
					return nil, errors.New("upstream catalog too large")
				}
				seen[model.ID] = true
				models = append(models, model)
			}
		}
		if protocol != wireprotocol.Messages || !page.HasMore {
			return models, nil
		}
		if page.LastID == "" || cursors[page.LastID] {
			return nil, errors.New("invalid catalog cursor")
		}
		cursors[page.LastID] = true
		pageURL = endpoint + "?after_id=" + url.QueryEscape(page.LastID)
	}
	return nil, errors.New("catalog pagination limit exceeded")
}

func (s *Service) readCatalogPage(ctx context.Context, endpoint, protocol, key string) (catalogPage, error) {
	var page catalogPage
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return page, err
	}
	setUpstreamAuth(req, protocol, key)
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return page, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return page, errors.New("upstream rejected catalog request")
	}
	body := &io.LimitedReader{R: resp.Body, N: maxCatalogBytes + 1}
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&page); err != nil || page.Data == nil || decoder.Decode(new(any)) != io.EOF || body.N == 0 {
		return page, errors.New("invalid upstream catalog response")
	}
	return page, nil
}
