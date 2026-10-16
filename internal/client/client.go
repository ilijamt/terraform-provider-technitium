package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

type Error struct {
	Path    string
	Status  string
	Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s: %s", e.Path, e.Status, e.Message)
}

// IsDropped means the call may or may not have taken effect.
func IsDropped(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, syscall.ECONNRESET)
}

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
	zones     sync.Map
	// restart is held shared by each call while it is sent, and exclusively
	// by a call that restarts the web service until the service is back.
	restart sync.RWMutex
	timings restartTimings
	// moved is set once a settings change moved the web service away from
	// host, and fails every later call.
	moved atomic.Pointer[MovedError]
}

type restartTimings struct {
	wait, poll, timeout, probe time.Duration
}

// The server restarts its web service 2s after the call that changed it
// returns (Task.Delay(2000) in DnsWebService), and was seen to go down within
// 5ms of that, for a few ms. Polling sooner could find it up before it went
// down, so the wait adds a margin for a slow or busy server. A service that
// moved is polled for probe only, as it answers at once when it did not.
var defaultRestartTimings = restartTimings{wait: 2500 * time.Millisecond, poll: 250 * time.Millisecond, timeout: time.Minute, probe: 5 * time.Second}

// MovedError is the error of every call made after a settings change moved
// the web service away from the provider's host.
type MovedError struct {
	// To is where the service moved, when the change tells.
	To       string
	From     string
	Settings []string
}

func (e *MovedError) Error() string {
	if e.To != "" {
		return fmt.Sprintf("the web service moved to %s after a settings change; set the provider host to it and apply again", e.To)
	}
	return fmt.Sprintf("the web service stopped answering at %s after a change to %s; set the provider host to where it now listens and apply again", e.From, strings.Join(e.Settings, ", "))
}

func IsMoved(err error) bool {
	var e *MovedError
	return errors.As(err, &e)
}

func New(host, token, userAgent string, insecure bool, hc *http.Client) *Client {
	if hc == nil {
		tr := http.DefaultTransport.(*http.Transport).Clone()
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: insecure}
		hc = &http.Client{Transport: tr, Timeout: 2 * time.Minute, CheckRedirect: checkRedirect}
	}
	return &Client{host: strings.TrimRight(host, "/"), token: token, userAgent: userAgent, http: hc, timings: defaultRestartTimings}
}

// checkRedirect refuses redirects that turn the POST into a GET, which drops
// the form and lets a call like settings/set succeed without changing anything,
// and those that would resend the form, with its passwords, to another host or
// over plain http.
func checkRedirect(req *http.Request, via []*http.Request) error {
	from := via[0].URL
	if req.Method != via[0].Method || !strings.EqualFold(req.URL.Hostname(), from.Hostname()) ||
		from.Scheme == "https" && req.URL.Scheme != "https" {
		return fmt.Errorf("redirected to %s, set host to that address", req.URL.Redacted())
	}
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	return nil
}

func (c *Client) Login(ctx context.Context, username, password string) error {
	resp, err := c.retry(ctx, "/api/user/login", url.Values{"user": {username}, "pass": {password}}, true)
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

// busy is the server's answer to a read while another request changes the
// same data. A write can get it after part of it took effect, such as a
// record deleted before the zone was unsigned under it, so writes are not
// retried but serialized per zone instead.
const busy = "Operation is not valid due to the current state of the object."

// Call retries a busy read and any call made while the server restarts its
// web service, as it does after some settings change. A call cut off mid-way
// is retried only when repeating it is harmless, which an add, create, sign,
// install or convert is not.
func (c *Client) Call(ctx context.Context, path string, params url.Values) (map[string]any, error) {
	return c.retry(ctx, path, params, false)
}

type onceKey struct{}

// Once marks ctx so a call cut off mid-way is not sent again, for a call
// that does something else when repeated.
func Once(ctx context.Context) context.Context {
	return context.WithValue(ctx, onceKey{}, true)
}

type restartsKey struct{}

// Restarts marks ctx for a call that makes the server restart its web
// service about 2s after it replies, dropping the calls in flight then. The
// call waits for the calls in flight, and holds new ones until the service
// answers again.
func Restarts(ctx context.Context) context.Context {
	return context.WithValue(ctx, restartsKey{}, true)
}

func IsRestart(ctx context.Context) bool {
	return ctx.Value(restartsKey{}) != nil
}

func (c *Client) retry(ctx context.Context, path string, params url.Values, root bool) (map[string]any, error) {
	read := strings.HasSuffix(path, "/get") || strings.HasSuffix(path, "/list")
	creates := unrepeatable(path) || ctx.Value(onceKey{}) != nil
	if zone := params.Get("zone"); zone != "" && !read {
		mu, _ := c.zones.LoadOrStore(strings.ToLower(strings.TrimSuffix(zone, ".")), &sync.Mutex{})
		mu.(*sync.Mutex).Lock()
		defer mu.(*sync.Mutex).Unlock()
	}
	// The zone lock comes first: a call waiting for it must not hold the
	// restart lock, or it could wait on a call that waits on the restart.
	restarts := IsRestart(ctx)
	if restarts {
		c.restart.Lock()
		defer c.restart.Unlock()
	}
	for attempt := 1; ; attempt++ {
		resp, err := c.attempt(ctx, path, params, root, restarts)
		var e *Error
		retry := read && errors.As(err, &e) && e.Message == busy ||
			errors.Is(err, syscall.ECONNREFUSED) ||
			!creates && IsDropped(err)
		if !retry || attempt == 10 {
			if restarts && (err == nil || IsDropped(err)) {
				if werr := c.awaitRestart(ctx, path, params); werr != nil {
					return nil, werr
				}
			}
			return resp, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt) * 200 * time.Millisecond):
		}
	}
}

// attempt holds the restart lock shared only while the call is sent, never
// across retries or nested calls, so a restart waits just for those in flight.
func (c *Client) attempt(ctx context.Context, path string, params url.Values, root, exclusive bool) (map[string]any, error) {
	if !exclusive {
		c.restart.RLock()
		defer c.restart.RUnlock()
	}
	if m := c.moved.Load(); m != nil {
		return nil, fmt.Errorf("%s: %w", path, m)
	}
	return c.call(ctx, path, params, root)
}

// awaitRestart waits until the restart has begun, then until the web service
// answers again. A call that turned out not to restart it waits out the
// first part only.
func (c *Client) awaitRestart(ctx context.Context, path string, params url.Values) error {
	tflog.Debug(ctx, "waiting for the web service to restart", map[string]any{"path": path})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(c.timings.wait):
	}
	moved := c.moves(ctx, params)
	timeout := c.timings.timeout
	if moved != nil {
		timeout = c.timings.probe
	}
	deadline := time.Now().Add(timeout)
	for {
		// The restart closed the pooled connections, and a fresh one shows
		// whether the service accepts new ones. A proxy in front may answer
		// for it, so only a reply to the token counts.
		c.http.CloseIdleConnections()
		pctx, cancel := context.WithTimeout(ctx, min(5*time.Second, timeout))
		_, err := c.call(pctx, "/api/user/session/get", nil, false)
		cancel()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return c.notBack(ctx, path, moved, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(c.timings.poll):
		}
	}
}

// notBack is the error for a web service that did not answer again after
// the call to path restarted it. One that moved took the change, so the call
// succeeds and the calls after it fail.
func (c *Client) notBack(ctx context.Context, path string, moved *MovedError, err error) error {
	if moved != nil {
		tflog.Warn(ctx, moved.Error(), map[string]any{"path": path})
		c.moved.Store(moved)
		return nil
	}
	return fmt.Errorf("%s: the web service at %s did not answer within %s of restarting: %w", path, c.host, c.timings.timeout, err)
}

// moves tells whether params stop the web service from answering at host,
// and where it went when they tell. Each value is checked, not only the
// changed ones, so a host behind a proxy may look moved; the probe then
// finds it still answering.
func (c *Client) moves(ctx context.Context, params url.Values) *MovedError {
	u, err := url.Parse(c.host)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	tls := u.Scheme == "https"
	port := u.Port()
	if port == "" && tls {
		port = "443"
	} else if port == "" {
		port = "80"
	}
	scheme, host := u.Scheme, u.Hostname()
	var settings []string
	moved := func(param, newScheme, newPort string) {
		settings = append(settings, settingName(param))
		scheme, port = newScheme, newPort
	}
	switch {
	case tls && params.Get("webServiceEnableTls") == "false":
		moved("webServiceEnableTls", "http", params.Get("webServiceHttpPort"))
	case tls && params.Has("webServiceTlsPort") && params.Get("webServiceTlsPort") != port:
		moved("webServiceTlsPort", "https", params.Get("webServiceTlsPort"))
	case !tls && params.Get("webServiceHttpToTlsRedirect") == "true":
		moved("webServiceHttpToTlsRedirect", "https", params.Get("webServiceTlsPort"))
	case !tls && params.Has("webServiceHttpPort") && params.Get("webServiceHttpPort") != port:
		moved("webServiceHttpPort", "http", params.Get("webServiceHttpPort"))
	}
	if params.Has("webServiceLocalAddresses") {
		addrs := strings.Split(params.Get("webServiceLocalAddresses"), ",")
		if !covers(ctx, u.Hostname(), addrs) {
			settings = append(settings, settingName("webServiceLocalAddresses"))
			host = ""
			if len(addrs) == 1 {
				host = strings.Trim(addrs[0], "[] ")
			}
		}
	}
	if len(settings) == 0 {
		return nil
	}
	m := &MovedError{From: c.host, Settings: settings}
	if host != "" && port != "" {
		m.To = scheme + "://" + net.JoinHostPort(host, port)
	}
	return m
}

// covers tells whether the web service listening on addrs answers at
// hostname. An empty list is the server's default of every address, and a
// name that does not resolve is given the benefit of the doubt.
func covers(ctx context.Context, hostname string, addrs []string) bool {
	var want []net.IP
	if ip := net.ParseIP(hostname); ip != nil {
		want = []net.IP{ip}
	} else {
		lctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		found, err := net.DefaultResolver.LookupIPAddr(lctx, hostname)
		if err != nil {
			return true
		}
		for _, a := range found {
			want = append(want, a.IP)
		}
	}
	for _, a := range addrs {
		a = strings.Trim(a, "[] ")
		if a == "" {
			return len(addrs) == 1
		}
		ip := net.ParseIP(a)
		if ip == nil {
			continue
		}
		if ip.IsUnspecified() || slices.ContainsFunc(want, ip.Equal) {
			return true
		}
	}
	return false
}

// settingName turns a settings parameter into its attribute name, such as
// webServiceHttpPort into web_service_http_port.
func settingName(param string) string {
	var b strings.Builder
	for _, r := range param {
		if unicode.IsUpper(r) {
			b.WriteByte('_')
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// unrepeatable matches the last path segment loosely, so a new endpoint such
// as createToken or addReservedLease is never resent by accident.
func unrepeatable(path string) bool {
	seg := strings.ToLower(path[strings.LastIndex(path, "/")+1:])
	for _, s := range []string{"add", "create", "install", "sign", "convert"} {
		if strings.Contains(seg, s) {
			return true
		}
	}
	return false
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
