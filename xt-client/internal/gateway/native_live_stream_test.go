package gateway

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNativeStreamFlushesTextBeforeUpstreamCompletes(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	svc, id := setupProtocolGateway(t, "openai-responses", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range protocolStreams["openai-responses"][:2] {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
		}
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		for _, event := range protocolStreams["openai-responses"][2:] {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
		}
	})
	defer unblock()
	server := httptest.NewServer(http.HandlerFunc(svc.HandleMessages))
	defer server.Close()
	request, err := http.NewRequest("POST", server.URL+"/v1/messages", strings.NewReader(inboundProtocolBody("anthropic-messages", id, true)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("x-api-key", "local-key")
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	seen := false
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "hello") {
			seen = true
			break
		}
	}
	unblock()
	if !seen {
		t.Fatal("text was buffered until upstream completion", scanner.Err())
	}
	rest, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	// Scanner may already hold the rest of the current frame; terminal events were
	// gated upstream and therefore are still on the network when release closes.
	if !strings.Contains(string(rest), "event: message_stop") {
		t.Fatal("missing final native event", string(rest))
	}
}
