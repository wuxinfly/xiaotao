package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"xiaotao/xt-client/internal/sourceplugin"
)

func TestWritePluginErrorMapsCodesToHTTP(t *testing.T) {
	cases := []struct {
		err      error
		status   int
		wantCode string
	}{
		{sourceplugin.NewError(sourceplugin.CodeAuthExpired, ""), http.StatusUnauthorized, sourceplugin.CodeAuthExpired},
		{sourceplugin.NewError(sourceplugin.CodeRateLimited, ""), http.StatusTooManyRequests, sourceplugin.CodeRateLimited},
		{sourceplugin.NewError(sourceplugin.CodeUnsupported, ""), http.StatusBadRequest, sourceplugin.CodeUnsupported},
		{sourceplugin.NewError(sourceplugin.CodeInvalid, ""), http.StatusBadRequest, sourceplugin.CodeInvalid},
		{sourceplugin.NewError(sourceplugin.CodeUnavailable, ""), http.StatusServiceUnavailable, sourceplugin.CodeUnavailable},
		{sourceplugin.NewError(sourceplugin.CodeUpstream, ""), http.StatusBadGateway, sourceplugin.CodeUpstream},
		{context.DeadlineExceeded, http.StatusGatewayTimeout, sourceplugin.CodeTimeout},
		{sourceplugin.NewError(sourceplugin.CodePlugin, ""), http.StatusBadGateway, sourceplugin.CodePlugin},
	}
	for _, tc := range cases {
		recorder := httptest.NewRecorder()
		writePluginError(recorder, tc.err)
		if recorder.Code != tc.status {
			t.Fatalf("%v status = %d, want %d", tc.err, recorder.Code, tc.status)
		}
		var body struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(recorder.Body.Bytes(), &body) != nil || body.Error.Code != tc.wantCode || body.Error.Message == "" {
			t.Fatalf("%v body = %s", tc.err, recorder.Body.String())
		}
	}
}
