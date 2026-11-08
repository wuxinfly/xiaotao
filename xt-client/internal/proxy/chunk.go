package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// writeSSEChunk marshals and flushes one OpenAI chunk to the SSE stream.
func writeSSEChunk(w http.ResponseWriter, flusher http.Flusher, payload map[string]any) {
	b, _ := json.Marshal(payload)
	fmt.Fprintf(w, "data: %s\n\n", b)
	if flusher != nil {
		flusher.Flush()
	}
}
