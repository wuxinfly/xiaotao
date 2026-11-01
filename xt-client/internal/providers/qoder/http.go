package qoder

import (
	"bytes"
	"context"
	"io"
	"net/http"
)

// HTTPClient performs signed requests against Qoder endpoints.
type HTTPClient struct {
	Do func(req *http.Request) (*http.Response, error)
}

func (c HTTPClient) do(req *http.Request) (*http.Response, error) {
	if c.Do != nil {
		return c.Do(req)
	}
	return http.DefaultClient.Do(req)
}

// SignedGet issues a COSY-signed GET without encoding the empty body.
func (c HTTPClient) SignedGet(ctx context.Context, region Region, path, uid, token, machineID string, opts SignOptions) (*http.Response, error) {
	u := region.InferBase + path
	opts.Encode = boolRef(false)
	signed, err := Sign(u, "", uid, token, machineID, opts)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range signed.Headers {
		req.Header.Set(k, v)
	}
	return c.do(req)
}

// SignedPost issues a COSY-signed POST with the encoded plaintext body.
func (c HTTPClient) SignedPost(ctx context.Context, region Region, path, body, uid, token, machineID string, opts SignOptions) (*http.Response, error) {
	u := region.InferBase + path
	opts.Encode = boolRef(true)
	signed, err := Sign(u, body, uid, token, machineID, opts)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader([]byte(signed.Body)))
	if err != nil {
		return nil, err
	}
	for k, v := range signed.Headers {
		req.Header.Set(k, v)
	}
	return c.do(req)
}

// CloseBody drains and closes the response to allow connection reuse.
func CloseBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
}

func boolRef(v bool) *bool { return &v }
