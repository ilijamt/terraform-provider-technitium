package framework

import (
	"context"
	"net/url"
)

type API interface {
	Call(ctx context.Context, path string, params url.Values) (map[string]any, error)
}
