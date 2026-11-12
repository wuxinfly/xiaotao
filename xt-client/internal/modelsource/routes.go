package modelsource

import (
	"errors"
	"maps"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

// CallID encodes components independently; lookup compares the full ID.
func CallID(source, key string) string {
	escape := func(v string) string { return strings.ReplaceAll(url.QueryEscape(v), "+", "%20") }
	return escape(source) + "/" + escape(key)
}

// Removed identities stay reserved, preventing old callers from binding to new models.
func allocateRoutes(previous map[string]string, entries []entry) map[string]string {
	routes := maps.Clone(previous)
	if routes == nil {
		routes = map[string]string{}
	}
	used := map[string]bool{}
	for _, value := range routes {
		used[value] = true
	}
	reserve := func(id, base string) string {
		if value := routes[id]; value != "" {
			return value
		}
		value := base
		for n := 2; used[value]; n++ {
			value = base + "~" + strconv.Itoa(n)
		}
		routes[id], used[value] = value, true
		return value
	}
	for _, e := range entries {
		prefix := reserve(e.Source.ID, routeName(e.Source.Label))
		for _, m := range e.Source.Models {
			reserve(m.ID, CallID(prefix, m.UpstreamModelID))
		}
	}
	return routes
}

func routeName(label string) string {
	// Plugin IDs require dots; manual prefixes exclude them to keep namespaces disjoint.
	name := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return '-'
	}, label)
	name = strings.Join(strings.FieldsFunc(name, func(r rune) bool { return r == '-' }), "-")
	if name == "" {
		return "api"
	}
	return name
}

func validateRoutes(routes map[string]string, entries []entry) error {
	used := map[string]bool{}
	for id, value := range routes {
		if value == "" || used[value] {
			return errors.New("invalid model call routes")
		}
		used[value] = true
		switch {
		case strings.HasPrefix(id, "source_"):
			base := strings.Split(value, "~")[0]
			if base == "" || routeName(base) != base || strings.ContainsAny(value, "/.%") {
				return errors.New("invalid source call prefix")
			}
		case strings.HasPrefix(id, "model_"):
			parts := strings.Split(value, "/")
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return errors.New("invalid model call ID")
			}
		default:
			return errors.New("invalid call route identity")
		}
	}
	for _, e := range entries {
		for _, m := range e.Source.Models {
			if value := routes[m.ID]; value != "" {
				prefix := routes[e.Source.ID]
				if prefix == "" || !strings.HasPrefix(value, CallID(prefix, "")) {
					return errors.New("model call route belongs to another source")
				}
			}
		}
	}
	return nil
}
