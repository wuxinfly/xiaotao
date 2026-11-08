package logging

import "context"

type reqIDKey struct{}

// WithReqID returns a new context containing the given request ID.
func WithReqID(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, reqIDKey{}, id)
}

// ReqID returns the request ID from ctx, or an empty string if not found.
func ReqID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(reqIDKey{}).(string); ok {
		return id
	}
	return ""
}
