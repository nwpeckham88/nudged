package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestAgentsAndAppsAPI(t *testing.T) {
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

	// register an agent
	agent := map[string]any{
		"id":   "agent-1",
		"name": "node-1",
		"addr": "10.0.0.5:2375",
		"apps": []string{"plex", "webapp"},
	}
	b, _ := json.Marshal(agent)
	resp, err := http.Post("http://"+addr+"/agents", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatalf("failed to POST agent: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status on POST agent: %v", resp.StatusCode)
	}

	// GET /agents
	resp, err = http.Get("http://" + addr + "/agents")
	if err != nil {
		t.Fatalf("failed to GET agents: %v", err)
	}
	var agents []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		t.Fatalf("failed to decode agents: %v", err)
	}
	_ = resp.Body.Close()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}

	// GET /apps
	resp, err = http.Get("http://" + addr + "/apps")
	if err != nil {
		t.Fatalf("failed to GET apps: %v", err)
	}
	var apps map[string][]string
	if err := json.NewDecoder(resp.Body).Decode(&apps); err != nil {
		t.Fatalf("failed to decode apps: %v", err)
	}
	_ = resp.Body.Close()
	if len(apps) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(apps))
	}

	// ensure Start returns on context cancel
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("server returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not exit in time")
	}
}
