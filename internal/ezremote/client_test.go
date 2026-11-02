package ezremote

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	p, _ := strconv.Atoi(port)
	return New(host, p, srv.Client())
}

func TestInstallURLSendsExpectedPayload(t *testing.T) {
	var got installRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/__local__/install_url" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		w.Write([]byte(`{ "result": { "success": true } }`))
	})

	if err := c.InstallURL(context.Background(), "http://nas:8080/a%20b.pkg"); err != nil {
		t.Fatal(err)
	}
	want := installRequest{Action: "installUrl", URL: "http://nas:8080/a%20b.pkg", EnableRPI: true}
	if got != want {
		t.Errorf("payload = %+v, want %+v", got, want)
	}
}

func TestInstallURLReportsRejection(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{ "result": { "success": false, "error": "Invalid URL" } }`))
	})
	err := c.InstallURL(context.Background(), "http://x/y.pkg")
	if !errors.Is(err, ErrRejected) {
		t.Fatalf("err = %v, want ErrRejected", err)
	}
}

// The queue retries network errors and fails answers, so a timeout must stay
// recognizable as a net.Error through our wrapping.
func TestInstallURLTimeoutIsNetworkError(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)

	host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	p, _ := strconv.Atoi(port)
	c := New(host, p, &http.Client{Timeout: 50 * time.Millisecond})

	err := c.InstallURL(context.Background(), "http://x/y.pkg")
	var ne net.Error
	if !errors.As(err, &ne) || !ne.Timeout() {
		t.Fatalf("err = %v, want a net.Error timeout", err)
	}
	if errors.Is(err, ErrRejected) {
		t.Fatal("a timeout is not a rejection")
	}
}

func TestInstallURLRejectionIsNotNetworkError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{ "result": { "success": false, "error": "x" } }`))
	})
	err := c.InstallURL(context.Background(), "http://x/y.pkg")
	var ne net.Error
	if errors.As(err, &ne) {
		t.Fatalf("rejection %v must not look like a network error", err)
	}
}

func TestInstallURLHTTPError(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	if err := c.InstallURL(context.Background(), "http://x/y.pkg"); err == nil {
		t.Fatal("expected error")
	}
}
