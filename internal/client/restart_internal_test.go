package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// restartingServer goes down for down, starting delay after it answers a
// settings/set, as the web service does after a change that restarts it.
type restartingServer struct {
	*httptest.Server
	delay, down time.Duration
	restarts    bool
	mu          sync.Mutex
	downFrom    time.Time
	downTo      time.Time
	sets, polls int
	inFlight    int
	most        int
	arrived     []time.Time
	hold        chan struct{}
}

func newRestartingServer(t *testing.T, restarts bool, down time.Duration) *restartingServer {
	s := &restartingServer{delay: 50 * time.Millisecond, down: down, restarts: restarts}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		s.mu.Lock()
		isDown := !s.downFrom.IsZero() && now.After(s.downFrom) && now.Before(s.downTo)
		switch r.URL.Path {
		case "/api/user/session/get":
			s.polls++
		case "/api/settings/set":
			s.sets++
		default:
			s.arrived = append(s.arrived, now)
		}
		s.mu.Unlock()
		if isDown {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		switch r.URL.Path {
		case "/api/settings/set":
			if s.restarts {
				s.mu.Lock()
				s.downFrom = now.Add(s.delay)
				s.downTo = s.downFrom.Add(s.down)
				s.mu.Unlock()
			}
		case "/api/user/session/get":
		default:
			s.mu.Lock()
			s.inFlight++
			s.most = max(s.most, s.inFlight)
			hold := s.hold
			s.mu.Unlock()
			if hold != nil {
				<-hold
			} else {
				time.Sleep(20 * time.Millisecond)
			}
			s.mu.Lock()
			s.inFlight--
			s.mu.Unlock()
		}
		_, _ = w.Write([]byte(`{"status":"ok","response":{}}`))
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *restartingServer) client() *Client {
	c := New(s.URL, "t0k", "test", false, nil)
	c.timings = restartTimings{wait: 100 * time.Millisecond, poll: 10 * time.Millisecond, timeout: 500 * time.Millisecond, probe: 200 * time.Millisecond}
	return c
}

func (s *restartingServer) stats() (sets, polls, most int, arrived []time.Time, downTo time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sets, s.polls, s.most, append([]time.Time(nil), s.arrived...), s.downTo
}

func noLeaks(t *testing.T) {
	before := runtime.NumGoroutine()
	t.Cleanup(func() {
		for range 100 {
			if runtime.NumGoroutine() <= before {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		assert.LessOrEqual(t, runtime.NumGoroutine(), before, "goroutines left behind")
	})
}

func TestRestartHoldsCalls(t *testing.T) {
	noLeaks(t)
	s := newRestartingServer(t, true, 100*time.Millisecond)
	c := s.client()
	defer c.http.CloseIdleConnections()
	ctx := context.Background()
	s.mu.Lock()
	s.hold = make(chan struct{})
	s.mu.Unlock()

	var wg sync.WaitGroup
	add := func(zone string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Call(ctx, "/api/zones/records/add", url.Values{"zone": {zone}})
			assert.NoError(t, err)
		}()
	}
	for _, zone := range []string{"a.com", "b.com", "c.com"} {
		add(zone)
	}
	require.Eventually(t, func() bool { _, _, most, _, _ := s.stats(); return most == 3 }, time.Second, 5*time.Millisecond, "calls run in parallel")

	var replied atomic.Bool
	done := make(chan error)
	go func() {
		_, err := c.Call(Restarts(ctx), "/api/settings/set", nil)
		replied.Store(true)
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	sets, _, _, _, _ := s.stats()
	assert.Zero(t, sets, "the restarting call waits for the calls in flight")

	s.mu.Lock()
	close(s.hold)
	s.hold = nil
	s.mu.Unlock()
	require.Eventually(t, func() bool { sets, _, _, _, _ := s.stats(); return sets == 1 }, time.Second, 5*time.Millisecond)

	// Calls made while the restart is pending are held until it is over.
	for _, zone := range []string{"a.com", "b.com", "c.com", "d.com"} {
		add(zone)
	}
	require.NoError(t, <-done)
	wg.Wait()
	_, polls, _, arrived, downTo := s.stats()
	assert.Greater(t, polls, 1, "the service was polled until it was back")
	require.Len(t, arrived, 7)
	for _, at := range arrived[3:] {
		assert.True(t, at.After(downTo), "a held call went out after the restart")
	}
}

func TestRestartThatDoesNotHappen(t *testing.T) {
	noLeaks(t)
	s := newRestartingServer(t, false, 0)
	c := s.client()
	defer c.http.CloseIdleConnections()

	start := time.Now()
	_, err := c.Call(Restarts(context.Background()), "/api/settings/set", nil)
	require.NoError(t, err)
	took := time.Since(start)
	_, polls, _, _, _ := s.stats()
	assert.Equal(t, 1, polls)
	assert.GreaterOrEqual(t, took, c.timings.wait)
	assert.Less(t, took, c.timings.wait+200*time.Millisecond)

	start = time.Now()
	_, err = c.Call(context.Background(), "/api/settings/set", nil)
	require.NoError(t, err)
	assert.Less(t, time.Since(start), c.timings.wait, "an unmarked call does not wait")
}

func TestRestartFailedCallDoesNotWait(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"error","errorMessage":"Invalid value"}`))
	}))
	defer srv.Close()
	c := New(srv.URL, "t0k", "test", false, nil)
	defer c.http.CloseIdleConnections()

	start := time.Now()
	_, err := c.Call(Restarts(context.Background()), "/api/settings/set", nil)
	require.Error(t, err)
	assert.Less(t, time.Since(start), time.Second)
}

func TestRestartNotBack(t *testing.T) {
	noLeaks(t)
	s := newRestartingServer(t, true, time.Hour)
	c := s.client()
	defer c.http.CloseIdleConnections()

	start := time.Now()
	_, err := c.Call(Restarts(context.Background()), "/api/settings/set", nil)
	require.ErrorContains(t, err, "/api/settings/set: the web service at "+s.URL+" did not answer within 500ms of restarting")
	assert.GreaterOrEqual(t, time.Since(start), c.timings.wait+c.timings.timeout)

	c = newRestartingServer(t, false, 0).client()
	defer c.http.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), c.timings.wait/2)
	defer cancel()
	_, err = c.Call(Restarts(ctx), "/api/settings/set", nil)
	require.ErrorIs(t, err, context.DeadlineExceeded, "a cancelled call stops waiting")
}

func TestRestartWithZoneLocks(t *testing.T) {
	noLeaks(t)
	s := newRestartingServer(t, true, 20*time.Millisecond)
	c := s.client()
	c.timings.wait = 60 * time.Millisecond
	defer c.http.CloseIdleConnections()
	ctx := context.Background()

	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for i := range 40 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				zone := url.Values{"zone": {[]string{"a.com", "b.com"}[i%2]}}
				if i%10 == 0 {
					_, _ = c.Call(Restarts(ctx), "/api/settings/set", zone)
					return
				}
				_, err := c.Call(ctx, "/api/zones/records/delete", zone)
				assert.NoError(t, err)
			}()
		}
		wg.Wait()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("deadlock")
	}
}

func TestMoves(t *testing.T) {
	v := func(kv ...string) url.Values {
		p := url.Values{}
		for i := 0; i < len(kv); i += 2 {
			p.Set(kv[i], kv[i+1])
		}
		return p
	}
	for _, tc := range []struct {
		name, host string
		params     url.Values
		to         string
		settings   []string
	}{
		{"no params", "http://10.0.0.1:5380", nil, "", nil},
		{"same http port", "http://10.0.0.1:5380", v("webServiceHttpPort", "5380", "webServiceTlsPort", "53443"), "", nil},
		{"http port", "http://10.0.0.1:5380", v("webServiceHttpPort", "5381"), "http://10.0.0.1:5381", []string{"web_service_http_port"}},
		{"http port, no port in host", "http://dns.example", v("webServiceHttpPort", "5380"), "http://dns.example:5380", []string{"web_service_http_port"}},
		{"tls port on http host", "http://10.0.0.1:5380", v("webServiceTlsPort", "443"), "", nil},
		{"tls port", "https://10.0.0.1:53443", v("webServiceTlsPort", "53444"), "https://10.0.0.1:53444", []string{"web_service_tls_port"}},
		{"same tls port, no port in host", "https://10.0.0.1", v("webServiceTlsPort", "443"), "", nil},
		{"http port on https host", "https://10.0.0.1:53443", v("webServiceHttpPort", "80", "webServiceTlsPort", "53443"), "", nil},
		{"tls off", "https://10.0.0.1:53443", v("webServiceEnableTls", "false", "webServiceHttpPort", "5380"), "http://10.0.0.1:5380", []string{"web_service_enable_tls"}},
		{"tls off, port unknown", "https://10.0.0.1:53443", v("webServiceEnableTls", "false"), "", []string{"web_service_enable_tls"}},
		{"tls on for http host", "http://10.0.0.1:5380", v("webServiceEnableTls", "true", "webServiceHttpPort", "5380"), "", nil},
		{"redirect", "http://10.0.0.1:5380", v("webServiceHttpToTlsRedirect", "true", "webServiceTlsPort", "53443"), "https://10.0.0.1:53443", []string{"web_service_http_to_tls_redirect"}},
		{"redirect off", "http://10.0.0.1:5380", v("webServiceHttpToTlsRedirect", "false"), "", nil},
		{"redirect on https host", "https://10.0.0.1:53443", v("webServiceHttpToTlsRedirect", "true", "webServiceTlsPort", "53443"), "", nil},
		{"any ipv4", "http://10.0.0.1:5380", v("webServiceLocalAddresses", "0.0.0.0"), "", nil},
		{"any ipv6", "http://10.0.0.1:5380", v("webServiceLocalAddresses", "[::]"), "", nil},
		{"default addresses", "http://10.0.0.1:5380", v("webServiceLocalAddresses", ""), "", nil},
		{"host listed", "http://10.0.0.1:5380", v("webServiceLocalAddresses", "10.0.0.2,10.0.0.1"), "", nil},
		{"one other address", "http://10.0.0.1:5380", v("webServiceLocalAddresses", "10.0.0.2", "webServiceHttpPort", "5380"), "http://10.0.0.2:5380", []string{"web_service_local_addresses"}},
		{"other addresses", "http://10.0.0.1:5380", v("webServiceLocalAddresses", "10.0.0.2,10.0.0.3"), "", []string{"web_service_local_addresses"}},
		{"other ipv6 address and port", "http://[fd00::1]:5380", v("webServiceLocalAddresses", "fd00::2", "webServiceHttpPort", "8080"), "http://[fd00::2]:8080", []string{"web_service_http_port", "web_service_local_addresses"}},
		{"name resolving to a listed address", "http://localhost:5380", v("webServiceLocalAddresses", "127.0.0.1,::1"), "", nil},
		{"name resolving elsewhere", "http://localhost:5380", v("webServiceLocalAddresses", "10.0.0.2"), "http://10.0.0.2:5380", []string{"web_service_local_addresses"}},
		{"unrelated settings", "http://10.0.0.1:5380", v("ssoEnabled", "true"), "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := New(tc.host, "", "test", false, nil)
			m := c.moves(context.Background(), tc.params)
			if tc.settings == nil {
				assert.Nil(t, m)
				return
			}
			require.NotNil(t, m)
			assert.Equal(t, tc.to, m.To)
			assert.Equal(t, tc.settings, m.Settings)
			assert.Equal(t, tc.host, m.From)
		})
	}
}

func TestMovedErrorMessage(t *testing.T) {
	assert.EqualError(t, &MovedError{To: "http://dns:5381", From: "http://dns:5380", Settings: []string{"web_service_http_port"}},
		"the web service moved to http://dns:5381 after a settings change; set the provider host to it and apply again")
	assert.EqualError(t, &MovedError{From: "http://dns:5380", Settings: []string{"web_service_local_addresses"}},
		"the web service stopped answering at http://dns:5380 after a change to web_service_local_addresses; set the provider host to where it now listens and apply again")
}

func TestRestartMoved(t *testing.T) {
	noLeaks(t)
	s := newRestartingServer(t, true, time.Hour)
	c := s.client()
	defer c.http.CloseIdleConnections()
	ctx := context.Background()

	var wg sync.WaitGroup
	held := make(chan error, 3)
	start := time.Now()
	done := make(chan error)
	go func() {
		_, err := c.Call(Restarts(ctx), "/api/settings/set", url.Values{"webServiceHttpPort": {"1"}})
		done <- err
	}()
	require.Eventually(t, func() bool { sets, _, _, _, _ := s.stats(); return sets == 1 }, time.Second, 5*time.Millisecond)
	for _, zone := range []string{"a.com", "b.com", "c.com"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.Call(ctx, "/api/zones/records/add", url.Values{"zone": {zone}})
			held <- err
		}()
	}
	require.NoError(t, <-done, "the change took effect")
	assert.Less(t, time.Since(start), c.timings.wait+c.timings.probe+300*time.Millisecond, "a move is not waited out")
	wg.Wait()
	close(held)
	for err := range held {
		assert.True(t, IsMoved(err), "a held call fails: %v", err)
	}

	start = time.Now()
	_, err := c.Call(ctx, "/api/settings/get", nil)
	assert.Less(t, time.Since(start), 50*time.Millisecond)
	require.EqualError(t, err, "/api/settings/get: the web service moved to http://127.0.0.1:1 after a settings change; set the provider host to it and apply again")
	_, err = c.Call(Restarts(ctx), "/api/settings/set", url.Values{"webServiceHttpPort": {"2"}})
	require.True(t, IsMoved(err))
	_, _, _, arrived, _ := s.stats()
	assert.Empty(t, arrived, "no call reached the old host after the move")
}

func TestRestartMovedButStillAnswers(t *testing.T) {
	noLeaks(t)
	s := newRestartingServer(t, true, 20*time.Millisecond)
	c := s.client()
	defer c.http.CloseIdleConnections()
	ctx := context.Background()

	_, err := c.Call(Restarts(ctx), "/api/settings/set", url.Values{"webServiceHttpPort": {"1"}})
	require.NoError(t, err)
	assert.Nil(t, c.moved.Load())
	_, err = c.Call(ctx, "/api/settings/get", nil)
	require.NoError(t, err)
}
