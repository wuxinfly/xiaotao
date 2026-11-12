package gateway

import (
	"strings"

	"xiaotao/xt-client/internal/modelsource"
)

func (s *Service) callSources() []modelsource.Source {
	if s.store == nil {
		return nil
	}
	sources := s.store.List()
	if s.plugins == nil {
		return sources
	}
	for _, src := range s.plugins.Sources() {
		prefix, err := s.store.PluginPrefix(src.ID)
		if err == nil && prefix != src.ID {
			for i := range src.Models {
				_, key, _ := strings.Cut(src.Models[i].CallID, "/")
				src.Models[i].CallID = prefix + "/" + key
			}
		}
		sources = append(sources, src)
	}
	return sources
}

func (s *Service) resolveCall(id string) (modelsource.Source, modelsource.Model, string, error) {
	// Translate the catalog alias back to an internal identity before routing.
	for _, src := range s.callSources() {
		if !src.Enabled || !src.HasAPIKey {
			continue
		}
		for _, m := range src.Models {
			if m.CallID != modelsource.ReadableCallID(id) {
				continue
			}
			if src.Kind == "plugin" {
				source, model, err := s.plugins.Resolve(m.ID)
				return source, model, "", err
			}
			return s.store.Resolve(m.ID)
		}
	}
	// Previously published full plugin names, encoded Unicode, and internal IDs stay valid.
	if s.store != nil {
		src, model, key, err := s.store.Resolve(id)
		if err != modelsource.ErrNotFound {
			return src, model, key, err
		}
	}
	if s.plugins != nil {
		src, model, err := s.plugins.Resolve(id)
		return src, model, "", err
	}
	return modelsource.Source{}, modelsource.Model{}, "", modelsource.ErrNotFound
}
