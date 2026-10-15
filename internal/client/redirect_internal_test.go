package client

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckRedirect(t *testing.T) {
	req := func(method, u string) *http.Request {
		r, _ := http.NewRequest(method, u, nil)
		return r
	}
	from := []*http.Request{req(http.MethodPost, "https://dns.example:5380/api/user/login")}

	assert.NoError(t, checkRedirect(req(http.MethodPost, "https://dns.example:5380/api/x"), from))
	assert.Error(t, checkRedirect(req(http.MethodGet, "https://dns.example:5380/api/x"), from), "method change")
	assert.Error(t, checkRedirect(req(http.MethodPost, "http://dns.example:5380/api/x"), from), "https to http")
	assert.Error(t, checkRedirect(req(http.MethodPost, "https://evil.example:5380/api/x"), from), "another host")
	assert.NoError(t, checkRedirect(req(http.MethodPost, "https://DNS.example:53443/api/x"),
		[]*http.Request{req(http.MethodPost, "http://dns.example:5380/api/user/login")}), "http to the https port")
}
