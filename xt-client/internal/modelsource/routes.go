package modelsource

import (
	"errors"
	"maps"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// CallID preserves readable Unicode and escapes structural characters in each component.
func CallID(source, key string) string { return ReadableCallID(legacyCallID(source, key)) }

func legacyCallID(source, key string) string {
	escape := func(v string) string { return strings.ReplaceAll(url.QueryEscape(v), "+", "%20") }
	return escape(source) + "/" + escape(key)
}

var encodedBytes = regexp.MustCompile(`(?:%[0-9a-fA-F]{2})+`)

// ReadableCallID decodes only visible Unicode. ASCII escapes stay escaped, so
// a literal "%E4%B8%AD" cannot collide with the character "中", nor can / cross components.
func ReadableCallID(id string) string {
	return encodedBytes.ReplaceAllStringFunc(id, func(group string) string {
		decoded, err := url.PathUnescape(group)
		if err != nil || !utf8.ValidString(decoded) {
			return group
		}
		var result strings.Builder
		for i, r := range decoded {
			size := utf8.RuneLen(r)
			if r > 127 && !unicode.IsControl(r) && !unicode.IsSpace(r) {
				result.WriteRune(r)
			} else {
				result.WriteString(group[i*3 : (i+size)*3])
			}
		}
		return result.String()
	})
}

// Removed identities stay reserved, preventing old callers from binding to new models.
func allocateRoutes(previous map[string]string, entries []entry) map[string]string {
	routes := maps.Clone(previous)
	if routes == nil {
		routes = map[string]string{}
	}
	used := map[string]bool{}
	for _, value := range routes {
		used[ReadableCallID(value)] = true
	}
	reserve := func(id, base string) string {
		if value := routes[id]; value != "" {
			return value
		}
		value := base
		for n := 2; used[ReadableCallID(value)]; n++ {
			value = base + "~" + strconv.Itoa(n)
		}
		routes[id], used[ReadableCallID(value)] = value, true
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
	// Full plugin IDs contain dots; short plugin prefixes are reserved separately.
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
		if value == "" || used[ReadableCallID(value)] {
			return errors.New("invalid model call routes")
		}
		used[ReadableCallID(value)] = true
		switch {
		case strings.HasPrefix(id, "plugin:"):
			pluginID := strings.TrimPrefix(id, "plugin:")
			if !strings.Contains(pluginID, ".") || (value != pluginID && value != strings.TrimPrefix(pluginID, "xiaotao.")) {
				return errors.New("invalid plugin call prefix")
			}
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
				if prefix == "" || !strings.HasPrefix(ReadableCallID(value), CallID(prefix, "")) {
					return errors.New("model call route belongs to another source")
				}
			}
		}
	}
	return nil
}
