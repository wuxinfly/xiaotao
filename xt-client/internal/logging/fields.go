package logging

import (
	"fmt"
	"strconv"
	"strings"
)

// Field represents a key-value pair in a structured log line.
type Field struct {
	Key   string
	Value string
}

// F constructs a Field. Value supports string, int, int64, float64, bool, error.
// error uses Error(); nil becomes empty string; other types use fmt.Sprintf("%v").
func F(key string, value any) Field {
	var strVal string
	switch v := value.(type) {
	case nil:
		strVal = ""
	case string:
		strVal = v
	case int:
		strVal = strconv.Itoa(v)
	case int64:
		strVal = strconv.FormatInt(v, 10)
	case float64:
		strVal = strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		strVal = strconv.FormatBool(v)
	case error:
		if v != nil {
			strVal = v.Error()
		}
	default:
		strVal = fmt.Sprintf("%v", v)
	}
	return Field{Key: key, Value: strVal}
}

// formatFields merges duplicate keys in call order and quotes values if needed.
func formatFields(fields []Field) string {
	if len(fields) == 0 {
		return ""
	}
	type item struct {
		key string
		val string
	}
	var ordered []item
	indices := make(map[string]int, len(fields))

	for _, f := range fields {
		if idx, exists := indices[f.Key]; exists {
			ordered[idx].val += "," + f.Value
		} else {
			indices[f.Key] = len(ordered)
			ordered = append(ordered, item{key: f.Key, val: f.Value})
		}
	}

	var b strings.Builder
	for i, it := range ordered {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(it.key)
		b.WriteByte('=')
		b.WriteString(quoteValue(it.val))
	}
	return b.String()
}

// quoteValue quotes a value with strconv.Quote if it contains space, '=', '"', '\n', or '\r'.
func quoteValue(s string) string {
	if strings.ContainsAny(s, " =\n\r\"") {
		return strconv.Quote(s)
	}
	return s
}
