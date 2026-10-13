package framework

import (
	"context"
	"net/url"
)

// API is what the resources need from the Technitium client: post params to
// an endpoint and get the "response" object back. Errors the server reports
// are *client.Error, so client.IsNotFound works on them.
type API interface {
	Call(ctx context.Context, path string, params url.Values) (map[string]any, error)
}
