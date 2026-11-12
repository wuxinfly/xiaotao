package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"xiaotao/xt-client/internal/wireprotocol"
)

type protocolRequestError struct{ error }

func adaptUpstream(resp *http.Response, protocol string, stream bool) error {
	if protocol == wireprotocol.Chat || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil
	}
	original := resp.Body
	if !stream {
		defer original.Close()
		raw, err := io.ReadAll(io.LimitReader(original, maxBody+1))
		if err != nil || len(raw) > maxBody {
			return errors.New("invalid upstream response size")
		}
		var native map[string]any
		if json.Unmarshal(raw, &native) != nil {
			return errors.New("invalid upstream JSON")
		}
		canonical, err := wireprotocol.ToChatResponse(protocol, native)
		if err != nil {
			return err
		}
		raw, err = json.Marshal(canonical)
		if err != nil {
			return err
		}
		resp.Body = io.NopCloser(bytes.NewReader(raw))
		return nil
	}
	reader, writer := io.Pipe()
	resp.Body = reader
	go func() {
		defer original.Close()
		decoder := wireprotocol.Decoder{Protocol: protocol}
		err := readEvents(original, func(data string) error {
			var native map[string]any
			if json.Unmarshal([]byte(data), &native) != nil {
				return errors.New("invalid upstream protocol event")
			}
			chunks, err := decoder.Event(native)
			if err != nil {
				return err
			}
			for _, chunk := range chunks {
				raw, err := json.Marshal(chunk)
				if err != nil {
					return err
				}
				if _, err = fmt.Fprintf(writer, "data: %s\n\n", raw); err != nil {
					return err
				}
			}
			if decoder.Done {
				if _, err = io.WriteString(writer, "data: [DONE]\n\n"); err != nil {
					return err
				}
				return errStreamDone
			}
			return nil
		})
		if err == nil && !decoder.Done {
			err = errors.New("upstream stream ended without terminal event")
		}
		_ = writer.CloseWithError(err)
	}()
	return nil
}
