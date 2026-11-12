package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
	"xiaotao/xt-client/internal/modelsource"
	"xiaotao/xt-client/internal/sourceplugin"
)

func (s *Service) SetPlugins(plugins *sourceplugin.Manager) { s.plugins = plugins }
func (s *Service) pluginSend(ctx context.Context, src modelsource.Source, body map[string]any) (*http.Response, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(ctx, totalTimeout)
	accountID := s.plugins.PrimaryAccount(src.ID)
	if s.plugins.Supports(src.ID, "refresh") {
		if _, err := s.plugins.RunForAccount(ctx, src.ID, accountID, "credentials/refresh", nil, nil, "", nil); err != nil {
			cancel()
			return nil, func() {}, err
		}
	}
	reader, writer := io.Pipe()
	timer := time.AfterFunc(idleTimeout, func() { cancel(); _ = writer.CloseWithError(context.DeadlineExceeded) })
	stream, _ := body["stream"].(bool)
	go func() {
		defer timer.Stop()
		defer writer.Close()
		result, err := s.plugins.RunForAccount(ctx, src.ID, accountID, "chat/start", nil, body, body["model"].(string), func(msg sourceplugin.Message) {
			if msg.Method != "chat/delta" || !stream {
				return
			}
			timer.Reset(idleTimeout)
			_, writeErr := fmt.Fprintf(writer, "data: %s\n\n", msg.Params)
			if writeErr != nil {
				cancel()
			}
		})
		if err != nil {
			_ = writer.CloseWithError(err)
			return
		}
		if stream {
			_, _ = io.WriteString(writer, "data: [DONE]\n\n")
		} else {
			if !json.Valid(result.Value) {
				_ = writer.CloseWithError(io.ErrUnexpectedEOF)
				return
			}
			_, _ = writer.Write(result.Value)
		}
	}()
	return &http.Response{StatusCode: 200, Body: reader, Header: make(http.Header)}, func() { timer.Stop(); cancel(); reader.Close() }, nil
}
