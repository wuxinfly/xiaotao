package main

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestBrowserLoginRequiresMatchingCallbackState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	token, err := waitForBrowserLogin(ctx, func(raw string) error {
		login, err := url.Parse(raw)
		if err != nil {
			return err
		}
		q := login.Query()
		callback, err := url.Parse(q.Get("auth_callback_url"))
		if err != nil {
			return err
		}
		for _, attempt := range []struct {
			state  string
			status int
		}{{"wrong", 400}, {q.Get("login_trace_id"), 200}} {
			callback.RawQuery = url.Values{"state": {attempt.state}, "refreshToken": {"test-token"}}.Encode()
			req, _ := http.NewRequestWithContext(ctx, "GET", callback.String(), nil)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			res.Body.Close()
			if res.StatusCode != attempt.status {
				t.Fatalf("callback status: %d, want %d", res.StatusCode, attempt.status)
			}
		}
		return nil
	})
	if err != nil || token != "test-token" {
		t.Fatalf("callback did not return test credential: %v", err)
	}
}
