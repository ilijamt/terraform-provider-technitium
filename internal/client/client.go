// Package client calls the Technitium DNS Server HTTP API.
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Error is a non "ok" status returned by the API.
type Error struct {
	Path    string
	Status  string
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s: %s", e.Path, e.Status, e.Message)
}

// IsNotFound reports whether the API said the object does not exist.
func IsNotFound(err error) bool {
	e, ok := err.(*Error)
	if !ok {
		return false
	}
	m := strings.ToLower(e.Message)
	return strings.Contains(m, "no such ") || strings.Contains(m, "not found") || strings.Contains(m, "does not exist")
}

type Client struct {
	host      string
	token     string
	userAgent string
	http      *http.Client
}

// New returns a client for host (e.g. http://localhost:5380). When hc is nil
// a default client is used; insecure skips TLS verification.
func New(host, token, userAgent string, insecure bool, hc *http.Client) *Client {
	if hc == nil {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: insecure}
		hc = &http.Client{Transport: tr, Timeout: 2 * time.Minute, CheckRedirect: checkRedirect}
	}
	return &Client{host: strings.TrimRight(host, "/"), token: token, userAgent: userAgent, http: hc}
}

// checkRedirect refuses redirects that turn the POST into a GET, which drops
// the form and lets a call like settings/set succeed without changing anything.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if req.Method != via[0].Method {
		return fmt.Errorf("redirected to %s, set host to that address", req.URL.Redacted())
	}
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	return nil
}

// Login exchanges a username and password for a session token.
func (c *Client) Login(ctx context.Context, username, password string) error {
	resp, err := c.call(ctx, "/api/user/login", url.Values{"user": {username}, "pass": {password}}, true)
	if err != nil {
		return err
	}
	token, _ := resp["token"].(string)
	if token == "" {
		return fmt.Errorf("login: no token in response")
	}
	c.token = token
	return nil
}

// busy is the error the server answers a read with while another request
// changes the same data. Reading again shortly after succeeds.
const busy = "Operation is not valid due to the current state of the object."

// Call posts params to path and returns the "response" object. A read (get or
// list) that fails because the server is busy is tried again; writes are not.
func (c *Client) Call(ctx context.Context, path string, params url.Values) (map[string]any, error) {
	read := strings.HasSuffix(path, "/get") || strings.HasSuffix(path, "/list")
	for attempt := 1; ; attempt++ {
		resp, err := c.call(ctx, path, params, false)
		var e *Error
		if !read || attempt == 5 || !errors.As(err, &e) || e.Message != busy {
			return resp, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt) * 100 * time.Millisecond):
		}
	}
}

func (c *Client) call(ctx context.Context, path string, params url.Values, root bool) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+path, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", c.userAgent)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	tflog.Trace(ctx, "technitium api call", map[string]any{"path": path, "status": res.StatusCode})
	if res.StatusCode != http.StatusOK {
		return nil, &Error{Path: path, Status: res.Status, Message: string(body)}
	}

	var out map[string]any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err = dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("%s: decode response: %w", path, err)
	}
	if status, _ := out["status"].(string); status != "ok" {
		msg, _ := out["errorMessage"].(string)
		return nil, &Error{Path: path, Status: status, Message: msg}
	}
	if root {
		return out, nil
	}
	resp, _ := out["response"].(map[string]any)
	if resp == nil {
		resp = map[string]any{}
	}
	return resp, nil
}
