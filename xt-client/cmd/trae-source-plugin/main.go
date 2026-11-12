// trae-source-plugin is executable code installed separately from the core.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"
	"xiaotao/xt-client/internal/sourceplugin"
)

func main() {
	http.DefaultClient.Timeout = 30 * time.Minute
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), sourceplugin.MaxMessage)
	encoder := json.NewEncoder(os.Stdout)
	emit := func(value any) {
		if encoder.Encode(value) != nil {
			os.Exit(1)
		}
	}
	for scanner.Scan() {
		var msg sourceplugin.Message
		if json.Unmarshal(scanner.Bytes(), &msg) != nil || msg.JSONRPC != "2.0" || msg.ID == 0 {
			return
		}
		if msg.Method == "initialize" {
			// Answer with the newest contract this build implements; the host
			// accepts anything between its minimum and maximum.
			raw, _ := json.Marshal(map[string]any{"id": "xiaotao.trae", "contractVersion": 2})
			emit(sourceplugin.Message{JSONRPC: "2.0", ID: msg.ID, Result: raw})
			continue
		}
		var op sourceplugin.Operation
		if json.Unmarshal(msg.Params, &op) != nil {
			return
		}
		result, err := operate(context.Background(), msg.Method, op, func(chunk map[string]any) {
			raw, _ := json.Marshal(chunk)
			emit(sourceplugin.Message{JSONRPC: "2.0", Method: "chat/delta", Params: raw})
		})
		response := sourceplugin.Message{JSONRPC: "2.0", ID: msg.ID}
		if err != nil {
			response.Error = rpcErrorFor(err)
		} else {
			response.Result, _ = json.Marshal(result)
		}
		emit(response)
	}
}
