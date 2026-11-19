package gateway

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"xiaotao/xt-client/internal/wireprotocol"
)

type incomingContext struct {
	protocol  string
	codexID   string
	auxiliary bool
}

func codexRequestContext(r *http.Request, protocol string, body map[string]any) incomingContext {
	result := incomingContext{protocol: protocol}
	if protocol != wireprotocol.Responses {
		return result
	}
	meta, _ := body["client_metadata"].(map[string]any)
	var turn struct {
		Thread string `json:"thread_id"`
		Source string `json:"thread_source"`
		Parent string `json:"parent_thread_id"`
		Forked string `json:"forked_from_thread_id"`
	}
	switch value := meta["x-codex-turn-metadata"].(type) {
	case string:
		_ = json.Unmarshal([]byte(value), &turn)
	case map[string]any:
		if raw, err := json.Marshal(value); err == nil {
			_ = json.Unmarshal(raw, &turn)
		}
	default:
		_ = json.Unmarshal([]byte(r.Header.Get("x-codex-turn-metadata")), &turn)
	}
	kind := firstIdentifier(r.Header.Get("x-openai-subagent"), turn.Source, textValue(meta["thread_source"]))
	switch kind {
	case "thread_title", "thread_title_reconsideration", "title_generation", "thread_description", "thread_summary", "composer_predictions":
		result.codexID = firstIdentifier(turn.Parent, textValue(meta["parent_thread_id"]), r.Header.Get("x-codex-parent-thread-id"), turn.Forked, textValue(meta["forked_from_thread_id"]))
		if result.codexID != "" {
			result.auxiliary = true
			return result
		}
	}
	result.codexID = firstIdentifier(turn.Thread, textValue(meta["thread_id"]), r.Header.Get("thread-id"),
		r.Header.Get("session_id"), r.Header.Get("session-id"))
	// A general OpenAI cache key can be shared by many conversations. Use it
	// as a thread identifier only for clients identifying themselves as Codex.
	if result.codexID == "" && strings.HasPrefix(strings.ToLower(r.Header.Get("originator")), "codex") {
		result.codexID = identifier(textValue(body["prompt_cache_key"]))
	}
	return result
}

func firstIdentifier(values ...string) string {
	for _, value := range values {
		if id := identifier(value); id != "" {
			return id
		}
	}
	return ""
}

func textValue(value any) string { text, _ := value.(string); return text }

func firstUserTitle(messages []map[string]any) string {
	for _, msg := range messages {
		if msg["role"] != "user" {
			continue
		}
		text := textValue(msg["content"])
		if text == "" {
			for _, part := range objects(msg["content"]) {
				if text = textValue(part["text"]); text != "" {
					break
				}
			}
		}
		if strings.HasPrefix(strings.TrimSpace(text), "<environment_context>") {
			continue
		}
		if title := shortTitle(text); title != "" {
			return title
		}
	}
	return ""
}

func shortTitle(title string) string {
	title = strings.Join(strings.Fields(title), " ")
	letters := []rune(title)
	if len(letters) > 72 {
		return string(letters[:72]) + "…"
	}
	return title
}

// The only Codex file read is its local title index. Transcripts and
// credentials are outside the title lookup.
type codexTitles struct {
	mu                sync.Mutex
	path              string
	checked, modified time.Time
	size              int64
	titles            map[string]string
}

func (c *codexTitles) lookup(id string) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.checked) < 2*time.Second {
		return c.titles[id]
	}
	c.checked = time.Now()
	path := c.path
	if path == "" {
		dir := os.Getenv("CODEX_HOME")
		if dir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return ""
			}
			dir = filepath.Join(home, ".codex")
		}
		path = filepath.Join(dir, "session_index.jsonl")
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		c.titles = nil
		return ""
	}
	if c.titles != nil && info.Size() == c.size && info.ModTime().Equal(c.modified) {
		return c.titles[id]
	}
	file, err := os.Open(path)
	if err != nil {
		return c.titles[id]
	}
	defer file.Close()
	const maxIndexBytes = 8 << 20
	reader := bufio.NewReader(file)
	if info.Size() > maxIndexBytes {
		if _, err = file.Seek(-maxIndexBytes, io.SeekEnd); err != nil {
			return c.titles[id]
		}
		_, _ = reader.ReadString('\n')
	}
	titles := map[string]string{}
	scanner := bufio.NewScanner(io.LimitReader(reader, maxIndexBytes))
	scanner.Buffer(make([]byte, 4096), 256<<10)
	for scanner.Scan() {
		var entry struct {
			ID    string `json:"id"`
			Title string `json:"thread_name"`
		}
		if json.Unmarshal(scanner.Bytes(), &entry) == nil && entry.ID != "" && shortTitle(entry.Title) != "" {
			titles[entry.ID] = shortTitle(entry.Title)
		}
	}
	if scanner.Err() == nil {
		c.titles, c.modified, c.size = titles, info.ModTime(), info.Size()
	}
	return c.titles[id]
}
