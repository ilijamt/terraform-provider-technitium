package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ilijamt/terraform-provider-technitium/internal/client"
)

func TestClient(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		got = r
		switch r.URL.Path {
		case "/api/user/login":
			if r.Form.Get("pass") != "secret" {
				_, _ = w.Write([]byte(`{"status":"error","errorMessage":"Invalid username or password"}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":"ok","token":"t0k"}`))
		case "/api/ok":
			_, _ = w.Write([]byte(`{"status":"ok","response":{"ttl":300}}`))
		case "/api/empty":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case "/api/missing":
			_, _ = w.Write([]byte(`{"status":"error","errorMessage":"No such zone was found: a.com"}`))
		case "/api/expired":
			_, _ = w.Write([]byte(`{"status":"invalid-token","errorMessage":"Invalid token or session expired."}`))
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	ctx := context.Background()

	c := client.New(srv.URL+"/", "", "test", false, nil)
	require.Error(t, c.Login(ctx, "admin", "wrong"))
	require.NoError(t, c.Login(ctx, "admin", "secret"))

	resp, err := c.Call(ctx, "/api/ok", url.Values{"zone": {"a.com"}})
	require.NoError(t, err)
	assert.Equal(t, json.Number("300"), resp["ttl"], "numbers decode as json.Number")
	assert.Equal(t, "Bearer t0k", got.Header.Get("Authorization"))
	assert.Equal(t, "a.com", got.PostForm.Get("zone"))

	resp, err = c.Call(ctx, "/api/empty", nil)
	require.NoError(t, err)
	assert.Empty(t, resp)

	_, err = c.Call(ctx, "/api/missing", nil)
	assert.True(t, client.IsNotFound(err))

	_, err = c.Call(ctx, "/api/expired", nil)
	var apiErr *client.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "invalid-token", apiErr.Status)
	assert.False(t, client.IsNotFound(err))

	_, err = c.Call(ctx, "/api/broken", nil)
	assert.ErrorAs(t, err, &apiErr)
}

func TestIsNotFound(t *testing.T) {
	for msg, want := range map[string]bool{
		"No such zone was found: a.com":                true,
		"Cannot delete record: no such record exists.": true,
		"DNS application was not found: x":             true,
		"Zone already exists: a.com":                   false,
		"Record already exists. Use overwrite option.": false,
	} {
		assert.Equal(t, want, client.IsNotFound(&client.Error{Message: msg}), msg)
	}
	assert.False(t, client.IsNotFound(assert.AnError))
}

func TestRedirect(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.URL.Path {
		case "/api/found":
			http.Redirect(w, r, "/api/target", http.StatusFound)
		case "/api/temporary":
			http.Redirect(w, r, "/api/target", http.StatusTemporaryRedirect)
		case "/api/target":
			got = r.Form
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	defer srv.Close()
	c := client.New(srv.URL, "t0k", "test", false, nil)
	params := url.Values{"ttl": {"60"}}

	_, err := c.Call(context.Background(), "/api/temporary", params)
	require.NoError(t, err)
	assert.Equal(t, "60", got.Get("ttl"), "307 keeps the form")

	got = nil
	_, err = c.Call(context.Background(), "/api/found", params)
	require.Error(t, err, "302 turns the call into a GET without the form")
	assert.Nil(t, got)
}

func TestRetryBusyReads(t *testing.T) {
	calls := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		if calls[r.URL.Path] < 3 {
			_, _ = w.Write([]byte(`{"status":"error","errorMessage":"Operation is not valid due to the current state of the object."}`))
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","response":{}}`))
	}))
	defer srv.Close()
	c := client.New(srv.URL, "t0k", "test", false, nil)

	_, err := c.Call(context.Background(), "/api/zones/records/get", nil)
	require.NoError(t, err)
	assert.Equal(t, 3, calls["/api/zones/records/get"], "a busy read is tried again")

	_, err = c.Call(context.Background(), "/api/zones/records/add", nil)
	require.Error(t, err)
	assert.Equal(t, 1, calls["/api/zones/records/add"], "a write is not")
}

func TestRetryDroppedConnections(t *testing.T) {
	calls := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls[r.URL.Path]++
		if calls[r.URL.Path] < 2 {
			// Drop the connection, as a restarting web service does.
			conn, _, err := w.(http.Hijacker).Hijack()
			require.NoError(t, err)
			_ = conn.Close()
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","response":{}}`))
	}))
	defer srv.Close()
	c := client.New(srv.URL, "t0k", "test", false, nil)

	_, err := c.Call(context.Background(), "/api/admin/groups/delete", nil)
	require.NoError(t, err)
	assert.Equal(t, 2, calls["/api/admin/groups/delete"], "a delete is safe to repeat")

	_, err = c.Call(context.Background(), "/api/zones/records/add", nil)
	require.Error(t, err)
	assert.Equal(t, 1, calls["/api/zones/records/add"], "an add is not")

	_, err = c.Call(context.Background(), "/api/apps/downloadAndInstall", nil)
	require.Error(t, err)
	assert.Equal(t, 1, calls["/api/apps/downloadAndInstall"], "nor is an install")
}

func TestWritesSerializedPerZone(t *testing.T) {
	var mu sync.Mutex
	inFlight, most := map[string]int{}, map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		key := strings.TrimSuffix(strings.ToLower(r.PostForm.Get("zone")), ".")
		if strings.HasSuffix(r.URL.Path, "/get") {
			key += " read"
		}
		mu.Lock()
		for _, k := range []string{key, ""} {
			inFlight[k]++
			most[k] = max(most[k], inFlight[k])
		}
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		inFlight[key]--
		inFlight[""]--
		mu.Unlock()
		_, _ = w.Write([]byte(`{"status":"ok","response":{}}`))
	}))
	defer srv.Close()
	c := client.New(srv.URL, "t0k", "test", false, nil)

	var wg sync.WaitGroup
	for _, call := range []struct{ path, zone string }{
		{"/api/zones/dnssec/unsign", "a.com"},
		{"/api/zones/records/delete", "a.com"},
		{"/api/zones/records/delete", "A.com."},
		{"/api/zones/records/add", "a.com"},
		{"/api/zones/records/get", "a.com"},
		{"/api/zones/records/get", "a.com"},
		{"/api/zones/records/delete", "b.com"},
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Call(context.Background(), call.path, url.Values{"zone": {call.zone}})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
	assert.Equal(t, 1, most["a.com"], "writes to one zone wait for each other")
	assert.Equal(t, 2, most["a.com read"], "reads do not")
	assert.Greater(t, most[""], 2, "nor do writes to other zones")
}
