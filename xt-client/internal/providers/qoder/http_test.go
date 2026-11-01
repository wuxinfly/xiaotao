package qoder

import (
	"context"
	"io"
	"net/http"
	"testing"
)

func TestHTTPSignedRequests(t *testing.T) {
	var gotPath, gotMethod, gotAuth, gotUser, gotBody string
	client := HTTPClient{Do: func(req *http.Request) (*http.Response, error) {
		gotMethod = req.Method
		gotPath = req.URL.Path
		gotAuth = req.Header.Get("Authorization")
		gotUser = req.Header.Get("Cosy-User")
		if req.Body != nil {
			raw, _ := io.ReadAll(req.Body)
			gotBody = string(raw)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: http.Header{}}, nil
	}}
	region := Region{InferBase: "https://inference.test", OpenAPIBase: "https://openapi.test"}

	resp, err := client.SignedGet(context.Background(), region, "/algo/api/v2/model/list?Encode=1", "u", "token", "machine", SignOptions{})
	if err != nil {
		t.Fatalf("SignedGet failed: %v", err)
	}
	CloseBody(resp)
	if gotMethod != http.MethodGet || gotPath != "/algo/api/v2/model/list" {
		t.Fatalf("unexpected GET request: %s %s", gotMethod, gotPath)
	}
	if gotAuth == "" || gotUser != "u" {
		t.Fatalf("missing COSY headers: auth=%q user=%q", gotAuth, gotUser)
	}

	resp, err = client.SignedPost(context.Background(), region, "/algo/chat", "plain", "u", "token", "machine", SignOptions{})
	if err != nil {
		t.Fatalf("SignedPost failed: %v", err)
	}
	CloseBody(resp)
	if gotMethod != http.MethodPost || gotPath != "/algo/chat" {
		t.Fatalf("unexpected POST request: %s %s", gotMethod, gotPath)
	}
	decoded, err := DecodeBody(gotBody)
	if err != nil || decoded != "plain" {
		t.Fatalf("POST body was not encoded as expected: %q, %v", gotBody, err)
	}
}

func TestHTTPClientCancellation(t *testing.T) {
	client := HTTPClient{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	region := Region{InferBase: "https://inference.test"}
	_, err := client.SignedGet(ctx, region, "/algo/api/v2/model/list", "u", "token", "machine", SignOptions{})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}
