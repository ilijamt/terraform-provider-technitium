package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
)

func TestNoRetryDroppedNonIdempotent(t *testing.T) {
	calls := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		conn, _, err := w.(http.Hijacker).Hijack()
		require.NoError(t, err)
		_ = conn.Close()
	}))
	defer srv.Close()
	c := client.New(srv.URL, "t0k", "test", false, nil)

	for _, p := range []string{
		"/api/admin/sessions/createToken",
		"/api/zones/dnssec/sign",
		"/api/zones/create",
		"/api/admin/users/create",
		"/api/zones/records/add",
		"/api/dhcp/scopes/addReservedLease",
		"/api/apps/downloadAndInstall",
	} {
		_, err := c.Call(context.Background(), p, nil)
		require.Error(t, err, p)
		assert.Equal(t, 1, calls[p], "%s is sent once", p)
	}
}

func TestNoRetryDroppedConvertOrOnce(t *testing.T) {
	calls := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		conn, _, err := w.(http.Hijacker).Hijack()
		require.NoError(t, err)
		_ = conn.Close()
	}))
	defer srv.Close()
	c := client.New(srv.URL, "t0k", "test", false, nil)

	for _, p := range []string{
		"/api/zones/convert",
		"/api/zones/dnssec/properties/convertToNSEC",
		"/api/zones/dnssec/properties/convertToNSEC3",
	} {
		_, err := c.Call(context.Background(), p, nil)
		require.Error(t, err, p)
		assert.Equal(t, 1, calls[p], "%s is sent once", p)
	}

	_, err := c.Call(client.Once(context.Background()), "/api/zones/records/update", nil)
	require.Error(t, err)
	assert.Equal(t, 1, calls["/api/zones/records/update"], "a call marked Once is sent once")

	_, err = c.Call(context.Background(), "/api/zones/options/set", nil)
	require.Error(t, err)
	assert.Greater(t, calls["/api/zones/options/set"], 1, "an unmarked set is still retried")
}

func TestRedirectToAnotherHost(t *testing.T) {
	var leaked bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		leaked = true
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, strings.Replace(other.URL, "127.0.0.1", "localhost", 1)+"/api/target", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()

	_, err := client.New(srv.URL, "t0k", "test", false, nil).Call(context.Background(), "/api/user/login", nil)
	require.Error(t, err)
	assert.False(t, leaked, "the form is not sent to another host")
}
