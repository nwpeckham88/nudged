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

// Test that when a client requests an app whose backend is unreachable,
// the hub sends a WAKE message to the registered agent over its control websocket.
func TestAgentReceivesWakeOnProxyError(t *testing.T) {
	// Reserve a free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// start server
	go Start(ctx, addr)
	// give it a moment
	time.Sleep(50 * time.Millisecond)

	// connect as agent via websocket
	dialer := websocket.Dialer{}
	wsURL := "ws://" + addr + "/ws/register"
	conn, resp, err := dialer.Dial(wsURL, nil)
	if err != nil {
		if resp != nil {
			t.Fatalf("dial ws failed: %v status=%d", err, resp.StatusCode)
		}
		t.Fatalf("dial ws failed: %v", err)
	}
	defer conn.Close()

	// send identity (agent advertises myapp)
	ident := map[string]any{"id": "agent1", "name": "agent1", "addr": "127.0.0.1:59999", "apps": []string{"myapp"}}
	if err := conn.WriteJSON(ident); err != nil {
		t.Fatalf("send ident: %v", err)
	}

	// issue HTTP request that triggers proxy to target.Addr (unreachable)
	req, _ := http.NewRequest("GET", "http://"+addr+"/", nil)
	req.Host = "myapp.example.com"
	client := &http.Client{Timeout: 2 * time.Second}
	resp2, err := client.Do(req)
	if err != nil {
		t.Fatalf("http request failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		buf := make([]byte, 1024)
		n, _ := resp2.Body.Read(buf)
		t.Fatalf("expected splash 200, got %d; body=%s", resp2.StatusCode, string(buf[:n]))
	}

	// read message from agent websocket (wake)
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	var msg map[string]any
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read wake from agent ws: %v", err)
	}
	if msg["type"] != "wake" || msg["app"] != "myapp" {
		t.Fatalf("unexpected wake msg: %v", msg)
	}
}

// Test that an agent's STATUS message is forwarded to splash clients
func TestNotifyWebsocketForwardsStatus(t *testing.T) {
	// Reserve a free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// start server
	go Start(ctx, addr)
	// give it a moment
	time.Sleep(50 * time.Millisecond)

	// connect as agent websocket
	dialer := websocket.Dialer{}
	agentWS, resp, err := dialer.Dial("ws://"+addr+"/ws/register", nil)
	if err != nil {
		if resp != nil {
			t.Fatalf("dial agent ws failed: %v status=%d", err, resp.StatusCode)
		}
		t.Fatalf("dial agent ws failed: %v", err)
	}
	defer agentWS.Close()
	ident := map[string]any{"id": "agent2", "name": "agent2", "addr": "127.0.0.1:59998", "apps": []string{"myapp"}}
	if err := agentWS.WriteJSON(ident); err != nil {
		t.Fatalf("send ident: %v", err)
	}

	// connect as splash client to /ws/notify?app=myapp
	notifyWS, resp, err := dialer.Dial("ws://"+addr+"/ws/notify?app=myapp", nil)
	if err != nil {
		if resp != nil {
			t.Fatalf("dial notify ws failed: %v status=%d", err, resp.StatusCode)
		}
		t.Fatalf("dial notify ws failed: %v", err)
	}
	defer notifyWS.Close()

	// send status from agent
	status := map[string]any{"type": "status", "app": "myapp", "state": "READY"}
	if err := agentWS.WriteJSON(status); err != nil {
		t.Fatalf("agent send status: %v", err)
	}

	// read from notifyWS
	notifyWS.SetReadDeadline(time.Now().Add(1 * time.Second))
	var notified map[string]any
	if err := notifyWS.ReadJSON(&notified); err != nil {
		t.Fatalf("read notify ws: %v", err)
	}
	if notified["state"] != "READY" {
		t.Fatalf("unexpected notify payload: %v", notified)
	}
}
