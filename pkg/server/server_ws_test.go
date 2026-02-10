package server

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWebSocketRegister(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to reserve port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- Start(ctx, addr) }()

	// allow server to start
	time.Sleep(50 * time.Millisecond)

	u := "ws://" + addr + "/ws/register"
	dialer := websocket.Dialer{}
	conn, resp, err := dialer.Dial(u, http.Header{})
	if err != nil {
		t.Fatalf("websocket dial failed: %v (status %v)", err, resp)
	}
	defer conn.Close()

	reg := map[string]any{"id": "ws-agent-1", "name": "wsnode", "addr": "10.0.0.8:2375", "apps": []string{"demo"}}
	if err := conn.WriteJSON(reg); err != nil {
		t.Fatalf("failed to send register JSON: %v", err)
	}

	// allow handler to process register
	time.Sleep(50 * time.Millisecond)

	// query /agents
	resp2, err := http.Get("http://" + addr + "/agents")
	if err != nil {
		t.Fatalf("failed to GET agents: %v", err)
	}
	var agents []map[string]any
	if err := json.NewDecoder(resp2.Body).Decode(&agents); err != nil {
		t.Fatalf("failed to decode agents: %v", err)
	}
	_ = resp2.Body.Close()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}

	// allow server to exit via context
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("server returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not exit in time")
	}
}
