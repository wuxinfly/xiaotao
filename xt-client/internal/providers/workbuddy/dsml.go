package workbuddy

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"
)

var dsmlVariants = []string{"｜｜DSML｜｜", "||DSML||", "|DSML|"}

var attrPattern = regexp.MustCompile(`([a-z0-9_:-]+)\s*=\s*["']([^"']*)["']`)

type dsmlTag struct {
	start       int
	end         int
	name        string
	closing     bool
	selfClosing bool
	attrs       string
	dsmlLike    bool
}

func canonicalToolName(raw string, dsmlLike bool) (string, bool) {
	switch raw {
	case "tool_calls", "tool-calls", "toolcalls":
		if raw != "tool_calls" && !dsmlLike {
			return raw, false
		}
		return "tool_calls", true
	case "calls":
		return "tool_calls", true
	default:
		return raw, true
	}
}

func scanDSMLTag(text string, start int) (dsmlTag, bool) {
	if start >= len(text) || text[start] != '<' {
		return dsmlTag{}, false
	}
	idx := start + 1
	closing := false
	if idx < len(text) && text[idx] == '/' {
		closing = true
		idx++
	}
	for idx < len(text) && (text[idx] == ' ' || text[idx] == '\t' || text[idx] == '\r' || text[idx] == '\n') {
		idx++
	}
	if idx >= len(text) {
		return dsmlTag{}, false
	}
	dsmlLike := false
	for _, variant := range dsmlVariants {
		if strings.HasPrefix(text[idx:], variant) {
			idx += len(variant)
			dsmlLike = true
			break
		}
	}
	if dsmlLike {
		for idx < len(text) {
			r, size := utf8.DecodeRuneInString(text[idx:])
			if r != ' ' && r != '　' && r != '\t' && r != '\r' && r != '\n' {
				break
			}
			idx += size
		}
	}
	nameStart := idx
	for idx < len(text) && (isAlphaNum(text[idx]) || text[idx] == '_' || text[idx] == '-') {
		idx++
	}
	if idx == nameStart {
		return dsmlTag{}, false
	}
	name, ok := canonicalToolName(strings.ToLower(text[nameStart:idx]), dsmlLike)
	if !ok {
		return dsmlTag{}, false
	}
	attrStart := idx
	selfClosing := false
	for idx < len(text) {
		if text[idx] == '>' {
			return dsmlTag{start: start, end: idx, name: name, closing: closing, attrs: strings.TrimSpace(text[attrStart:idx]), dsmlLike: dsmlLike}, true
		}
		if text[idx] == '/' && idx+1 < len(text) && text[idx+1] == '>' {
			selfClosing = true
			return dsmlTag{start: start, end: idx + 1, name: name, closing: closing, selfClosing: selfClosing, attrs: strings.TrimSpace(text[attrStart:idx]), dsmlLike: dsmlLike}, true
		}
		idx++
	}
	return dsmlTag{}, false
}

func isAlphaNum(ch byte) bool {
	return ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z'
}

func findDSMLTag(text string, start int) (dsmlTag, bool) {
	i := max(start, 0)
	for i < len(text) {
		if text[i] == '`' {
			end := strings.IndexByte(text[i+1:], '`')
			if end < 0 {
				return dsmlTag{}, false
			}
			i += end + 2
			continue
		}
		if strings.HasPrefix(text[i:], "<![CDATA[") {
			end := strings.Index(text[i:], "]]>")
			if end < 0 {
				return dsmlTag{}, false
			}
			i += end + 3
			continue
		}
		if atLineStart(text, i) && (strings.HasPrefix(text[i:], "```") || strings.HasPrefix(text[i:], "~~~")) {
			end := findFenceEnd(text, i)
			if end < 0 {
				return dsmlTag{}, false
			}
			i = end
			continue
		}
		if text[i] == '<' {
			if tag, ok := scanDSMLTag(text, i); ok {
				return tag, true
			}
		}
		i++
	}
	return dsmlTag{}, false
}

func atLineStart(text string, pos int) bool {
	return pos == 0 || text[pos-1] == '\n'
}

func findFenceEnd(text string, start int) int {
	lineEnd := strings.IndexByte(text[start:], '\n')
	if lineEnd < 0 {
		return -1
	}
	pos := start + lineEnd + 1
	for pos < len(text) {
		nextEnd := strings.IndexByte(text[pos:], '\n')
		line := text[pos:]
		if nextEnd >= 0 {
			line = text[pos : pos+nextEnd]
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if nextEnd < 0 {
				return len(text)
			}
			return pos + nextEnd + 1
		}
		if nextEnd < 0 {
			return -1
		}
		pos += nextEnd + 1
	}
	return -1
}

func findMatchingDSMLClose(text string, open dsmlTag) (dsmlTag, bool) {
	depth := 1
	i := open.end + 1
	for i < len(text) {
		tag, ok := findDSMLTag(text, i)
		if !ok {
			break
		}
		if tag.name == open.name {
			if tag.closing {
				depth--
				if depth == 0 {
					return tag, true
				}
			} else {
				depth++
			}
		}
		i = tag.end + 1
	}
	return dsmlTag{}, false
}

func parseXMLAttrs(raw string) map[string]string {
	out := map[string]string{}
	for _, match := range attrPattern.FindAllStringSubmatch(raw, -1) {
		out[strings.ToLower(match[1])] = html.UnescapeString(match[2])
	}
	return out
}

type dsmlInvoke struct {
	name string
	body string
}

func findInvokeBlocks(text string) []dsmlInvoke {
	var blocks []dsmlInvoke
	i := 0
	for i < len(text) {
		tag, ok := findDSMLTag(text, i)
		if !ok {
			break
		}
		if tag.name == "invoke" && !tag.closing {
			if close, found := findMatchingDSMLClose(text, tag); found {
				attrs := parseXMLAttrs(tag.attrs)
				blocks = append(blocks, dsmlInvoke{name: attrs["name"], body: text[tag.end+1 : close.start]})
				i = close.end + 1
				continue
			}
		}
		i = tag.end + 1
	}
	return blocks
}

func parseInvokeParameters(body string) map[string]any {
	params := map[string]any{}
	i := 0
	for i < len(body) {
		tagStart := strings.IndexByte(body[i:], '<')
		if tagStart < 0 {
			break
		}
		tagStart += i
		tag, ok := scanDSMLTag(body, tagStart)
		if !ok || tag.closing || tag.selfClosing {
			i = tagStart + 1
			continue
		}
		close, found := findMatchingDSMLClose(body, tag)
		if !found {
			i = tag.end + 1
			continue
		}
		value := strings.TrimSpace(body[tag.end+1 : close.start])
		name := ""
		if tag.name == "parameter" {
			name = parseXMLAttrs(tag.attrs)["name"]
		} else {
			name = tag.name
		}
		if name != "" {
			if strings.HasPrefix(value, "<![CDATA[") && strings.HasSuffix(value, "]]>") {
				params[name] = value[9 : len(value)-3]
			} else if nested := parseInvokeParameters(value); tag.name != "parameter" && len(nested) > 0 {
				params[name] = nested
			} else {
				params[name] = html.UnescapeString(value)
			}
		}
		i = close.end + 1
	}
	return params
}

func parseDSMLToolCalls(text string) []map[string]any {
	calls := []map[string]any{}
	for _, block := range findInvokeBlocks(text) {
		if block.name == "" {
			continue
		}
		args, err := json.Marshal(parseInvokeParameters(block.body))
		if err != nil {
			args = []byte("{}")
		}
		calls = append(calls, map[string]any{
			"id":   "call_" + randHex(),
			"type": "function",
			"function": map[string]any{
				"name":      block.name,
				"arguments": string(args),
			},
		})
	}
	return calls
}

func possibleDSMLStart(tail string) bool {
	if !strings.HasPrefix(tail, "<") || strings.Contains(tail, ">") {
		return false
	}
	body := tail[1:]
	for _, variant := range dsmlVariants {
		if strings.HasPrefix(body, variant) {
			body = body[len(variant):]
			break
		}
		if strings.HasPrefix(variant, body) {
			return true
		}
	}
	body = strings.ToLower(strings.TrimLeft(body, " \t\r\n"))
	if body == "" {
		return true
	}
	for _, opener := range []string{"tool", "invoke", "tool_calls", "tool-calls", "toolcalls", "tool_call", "tool-call", "calls"} {
		if strings.HasPrefix(opener, body) || strings.HasPrefix(body, opener) {
			return true
		}
	}
	return false
}

func lastUnclosedCodeSpan(text string) int {
	count := strings.Count(text, "`")
	if count == 0 || count%2 == 0 {
		return -1
	}
	return strings.LastIndexByte(text, '`')
}

type dsmlBuffer struct {
	buffer          string
	detectedCalls   []map[string]any
	callsEmitted    bool
	nativeToolCalls bool
}

func (b *dsmlBuffer) addChunk(content string) (string, []map[string]any) {
	b.buffer += content
	i := 0
	for i < len(b.buffer) {
		tag, ok := findDSMLTag(b.buffer, i)
		if !ok {
			break
		}
		if !tag.closing && (tag.name == "tool_calls" || tag.name == "invoke") {
			close, found := findMatchingDSMLClose(b.buffer, tag)
			if !found {
				return "", nil
			}
			block := b.buffer[tag.start : close.end+1]
			calls := parseDSMLToolCalls(block)
			if len(calls) > 0 {
				b.detectedCalls = calls
				b.callsEmitted = true
				prefix := strings.TrimSpace(b.buffer[:tag.start])
				b.buffer = b.buffer[close.end+1:]
				return prefix, calls
			}
			b.buffer = b.buffer[:tag.start] + b.buffer[close.end+1:]
			i = tag.start
			continue
		}
		i = tag.end + 1
	}
	holdFrom := len(b.buffer)
	if last := strings.LastIndexByte(b.buffer, '<'); last >= 0 && possibleDSMLStart(b.buffer[last:]) {
		holdFrom = last
	}
	if last := lastUnclosedCodeSpan(b.buffer); last >= 0 {
		holdFrom = min(holdFrom, last)
	}
	if holdFrom >= len(b.buffer) {
		output := b.buffer
		b.buffer = ""
		return output, nil
	}
	prefix := b.buffer[:holdFrom]
	b.buffer = b.buffer[holdFrom:]
	return prefix, nil
}

func (b *dsmlBuffer) flush() string {
	leftover := b.buffer
	b.buffer = ""
	return leftover
}

func (b *dsmlBuffer) shouldEmitToolCalls() bool { return b.callsEmitted }

func getDSMLBuffer(model string) *dsmlBuffer {
	if buffer, ok := dsmlBuffers[model]; ok {
		return buffer
	}
	buffer := &dsmlBuffer{}
	dsmlBuffers[model] = buffer
	return buffer
}
