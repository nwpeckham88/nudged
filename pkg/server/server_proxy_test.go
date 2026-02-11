package server

import (
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"nudged/internal/hub"
)

// Test that when the reverse proxy cannot reach the agent, the server
// publishes a wake event for the requested app.
func TestProxyWakePublishesWakeEvent(t *testing.T) {
	h := hub.New()

	// simple in-test registry type mirroring server.Registry/Agent
	type Agent struct {
		ID       string
		Name     string
		Addr     string
		Apps     []string
		LastSeen int64
		// Conn omitted for this test
	}

	reg := &struct {
		mu     sync.RWMutex
		agents map[string]*Agent
	}{agents: make(map[string]*Agent)}

	// register agent advertising 'myapp' but pointing to an unreachable addr
	reg.agents["a1"] = &Agent{ID: "a1", Addr: "127.0.0.1:59999", Apps: []string{"myapp"}}

	mux := http.NewServeMux()

	// replicate only the minimal handlers needed: / and publish behavior
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if idx := strings.Index(host, ":"); idx != -1 {
			host = host[:idx]
		}
		parts := strings.Split(host, ".")
		if len(parts) == 0 {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		app := parts[0]

		reg.mu.RLock()
		var target *Agent
		for _, a := range reg.agents {
			for _, ap := range a.Apps {
				if ap == app {
					target = a
					break
				}
			}
			if target != nil {
				break
			}
		}
		reg.mu.RUnlock()

		if target == nil {
			http.Error(w, "service not found", http.StatusNotFound)
			return
		}

		targetURL := &url.URL{Scheme: "http", Host: target.Addr}
		proxy := httputil.NewSingleHostReverseProxy(targetURL)
		proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
			wakeMsg := map[string]any{"type": "wake", "app": app}
			h.Publish(hub.Message{Topic: "wake:" + app, Payload: wakeMsg, From: "test"})
			rw.Header().Set("Content-Type", "text/html; charset=utf-8")
			rw.WriteHeader(http.StatusOK)
			_, _ = rw.Write([]byte("splash"))
		}
		proxy.ServeHTTP(w, r)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// subscribe to the wake topic
	wakeSub, unsub := h.Subscribe("wake:myapp")
	defer unsub()

	// issue a request with Host header set to myapp.example.com
	req, err := http.NewRequest("GET", ts.URL+"/", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Host = "myapp.example.com"

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// Expect the proxy to respond with the splash HTML
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from splash, got %d", resp.StatusCode)
	}

	// Wait for a wake event to be published (with timeout)
	select {
	case msg := <-wakeSub:
		// verify payload contains expected app name
		m, ok := msg.Payload.(map[string]any)
		if !ok {
			t.Fatalf("unexpected payload type: %T", msg.Payload)
		}
		if m["app"] != "myapp" {
			t.Fatalf("expected wake for myapp, got %v", m["app"])
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timed out waiting for wake event")
	}
}
